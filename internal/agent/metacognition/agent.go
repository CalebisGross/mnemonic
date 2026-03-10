package metacognition

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/google/uuid"
)

type MetacognitionConfig struct {
	Interval time.Duration
}

type MetacognitionAgent struct {
	store       store.Store
	llmProvider llm.Provider
	config      MetacognitionConfig
	log         *slog.Logger
	bus         events.Bus
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopOnce    sync.Once
	triggerCh   chan struct{}
}

func NewMetacognitionAgent(s store.Store, llmProv llm.Provider, cfg MetacognitionConfig, log *slog.Logger) *MetacognitionAgent {
	return &MetacognitionAgent{
		store:       s,
		llmProvider: llmProv,
		config:      cfg,
		log:         log,
		triggerCh:   make(chan struct{}, 1),
	}
}

func (ma *MetacognitionAgent) Name() string {
	return "metacognition-agent"
}

func (ma *MetacognitionAgent) Start(ctx context.Context, bus events.Bus) error {
	ma.ctx, ma.cancel = context.WithCancel(ctx)
	ma.bus = bus
	ma.wg.Add(1)
	go ma.loop()
	return nil
}

func (ma *MetacognitionAgent) Stop() error {
	ma.stopOnce.Do(func() {
		ma.cancel()
	})
	ma.wg.Wait()
	return nil
}

func (ma *MetacognitionAgent) Health(ctx context.Context) error {
	_, err := ma.store.CountMemories(ctx)
	return err
}

// GetTriggerChannel returns a channel that can be used to trigger an on-demand
// metacognition cycle (e.g. from the reactor after consolidation completes).
func (ma *MetacognitionAgent) GetTriggerChannel() chan<- struct{} {
	return ma.triggerCh
}

type CycleReport struct {
	Duration         time.Duration
	Observations     []store.MetaObservation
	ActionsPerformed int
}

func (ma *MetacognitionAgent) RunOnce(ctx context.Context) (*CycleReport, error) {
	return ma.runCycle(ctx)
}

func (ma *MetacognitionAgent) loop() {
	defer ma.wg.Done()

	startupTimer := time.NewTimer(60 * time.Second)
	defer startupTimer.Stop()

	ticker := time.NewTicker(ma.config.Interval)
	defer ticker.Stop()

	runAndLog := func() {
		report, err := ma.runCycle(ma.ctx)
		if err != nil && ma.ctx.Err() == nil {
			ma.log.Error("metacognition cycle failed", "error", err)
		} else if report != nil {
			ma.log.Info("metacognition cycle completed", "duration_ms", report.Duration.Milliseconds(), "observations", len(report.Observations))
		}
	}

	for {
		select {
		case <-ma.ctx.Done():
			return
		case <-startupTimer.C:
			runAndLog()
		case <-ticker.C:
			runAndLog()
		case <-ma.triggerCh:
			runAndLog()
		}
	}
}

func (ma *MetacognitionAgent) runCycle(ctx context.Context) (*CycleReport, error) {
	startTime := time.Now()

	// Cleanup: remove meta observations older than 7 days to prevent stale triggers
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	if deleted, err := ma.store.DeleteOldMetaObservations(ctx, cutoff); err != nil {
		ma.log.Warn("failed to cleanup old meta observations", "error", err)
	} else if deleted > 0 {
		ma.log.Info("cleaned up stale meta observations", "deleted", deleted)
	}

	observations := []store.MetaObservation{}

	// --- Observation phase ---
	if obs := ma.auditMemoryQuality(ctx); obs != nil {
		observations = append(observations, *obs)
	}

	if obs := ma.analyzeSourceDistribution(ctx); obs != nil {
		observations = append(observations, *obs)
	}

	if obs := ma.analyzeRecallEffectiveness(ctx); obs != nil {
		observations = append(observations, *obs)
	}

	if obs := ma.checkConsolidationHealth(ctx); obs != nil {
		observations = append(observations, *obs)
	}

	if obs := ma.analyzeRetrievalFeedback(ctx); obs != nil {
		observations = append(observations, *obs)
	}

	for i := range observations {
		observations[i].ID = uuid.New().String()
		observations[i].CreatedAt = time.Now()
		if err := ma.store.WriteMetaObservation(ctx, observations[i]); err != nil {
			return nil, fmt.Errorf("failed to write observation: %w", err)
		}
	}

	// --- Feedback processing phase: adjust associations and salience based on feedback ---
	feedbackActions := ma.processFeedback(ctx)

	// --- Action phase: act on observations ---
	actionsPerformed := ma.actOnObservations(ctx, observations) + feedbackActions

	if ma.bus != nil {
		_ = ma.bus.Publish(ctx, events.MetaCycleCompleted{
			ObservationsLogged: len(observations),
			Ts:                 time.Now(),
		})
	}

	return &CycleReport{
		Duration:         time.Since(startTime),
		Observations:     observations,
		ActionsPerformed: actionsPerformed,
	}, nil
}

func (ma *MetacognitionAgent) auditMemoryQuality(ctx context.Context) *store.MetaObservation {
	stats, err := ma.store.GetStatistics(ctx)
	if err != nil {
		ma.log.Error("failed to get statistics", "error", err)
		return nil
	}

	memories, err := ma.store.ListMemories(ctx, "active", 500, 0)
	if err != nil {
		ma.log.Error("failed to list memories", "error", err)
		return nil
	}

	var noEmbedding, noCompression, shortSummary, longSummary int

	for _, mem := range memories {
		if len(mem.Embedding) == 0 {
			noEmbedding++
		}
		if mem.Content == mem.Summary {
			noCompression++
		}
		if len(mem.Summary) < 10 {
			shortSummary++
		}
		if len(mem.Summary) > 200 {
			longSummary++
		}
	}

	totalIssues := noEmbedding + noCompression + shortSummary + longSummary

	if totalIssues == 0 {
		return nil
	}

	severity := "info"
	threshold := stats.ActiveMemories / 20
	if threshold < 1 {
		threshold = 1
	}

	if totalIssues >= stats.ActiveMemories/5 {
		severity = "critical"
	} else if totalIssues >= threshold {
		severity = "warning"
	}

	return &store.MetaObservation{
		ObservationType: "quality_audit",
		Severity:        severity,
		Details: map[string]interface{}{
			"no_embedding":   noEmbedding,
			"no_compression": noCompression,
			"short_summary":  shortSummary,
			"long_summary":   longSummary,
			"total_issues":   totalIssues,
		},
	}
}

func (ma *MetacognitionAgent) analyzeSourceDistribution(ctx context.Context) *store.MetaObservation {
	distribution, err := ma.store.GetSourceDistribution(ctx)
	if err != nil {
		ma.log.Error("failed to get source distribution", "error", err)
		return nil
	}

	var total int
	var dominantSource string
	var dominantCount int

	for source, count := range distribution {
		total += count
		if count > dominantCount {
			dominantCount = count
			dominantSource = source
		}
	}

	if total == 0 {
		return nil
	}

	dominantRatio := float64(dominantCount) / float64(total)

	if dominantRatio <= 0.8 {
		return nil
	}

	return &store.MetaObservation{
		ObservationType: "source_balance",
		Severity:        "warning",
		Details: map[string]interface{}{
			"source_counts":   distribution,
			"dominant_source": dominantSource,
			"dominant_ratio":  dominantRatio,
		},
	}
}

func (ma *MetacognitionAgent) analyzeRecallEffectiveness(ctx context.Context) *store.MetaObservation {
	deadMemories, err := ma.store.GetDeadMemories(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		ma.log.Error("failed to get dead memories", "error", err)
		return nil
	}

	stats, err := ma.store.GetStatistics(ctx)
	if err != nil {
		ma.log.Error("failed to get statistics", "error", err)
		return nil
	}

	if stats.ActiveMemories == 0 {
		return nil
	}

	deadRatio := float64(len(deadMemories)) / float64(stats.ActiveMemories)

	if deadRatio <= 0.2 {
		return nil
	}

	severity := "warning"
	if deadRatio > 0.5 {
		severity = "critical"
	}

	return &store.MetaObservation{
		ObservationType: "recall_effectiveness",
		Severity:        severity,
		Details: map[string]interface{}{
			"dead_count":   len(deadMemories),
			"dead_ratio":   deadRatio,
			"total_active": stats.ActiveMemories,
		},
	}
}

func (ma *MetacognitionAgent) checkConsolidationHealth(ctx context.Context) *store.MetaObservation {
	record, err := ma.store.GetLastConsolidation(ctx)
	if err != nil || record.ID == "" {
		return &store.MetaObservation{
			ObservationType: "consolidation_health",
			Severity:        "warning",
			Details: map[string]interface{}{
				"message": "consolidation never run",
			},
		}
	}

	hoursSinceLastRun := time.Since(record.EndTime).Hours()

	if hoursSinceLastRun <= 48 {
		return nil
	}

	severity := "info"
	if hoursSinceLastRun > 72 {
		severity = "warning"
	}

	return &store.MetaObservation{
		ObservationType: "consolidation_health",
		Severity:        severity,
		Details: map[string]interface{}{
			"hours_since_last_run": hoursSinceLastRun,
			"last_run_time":        record.EndTime,
		},
	}
}

// analyzeRetrievalFeedback reads recent retrieval_feedback observations and computes quality metrics.
func (ma *MetacognitionAgent) analyzeRetrievalFeedback(ctx context.Context) *store.MetaObservation {
	feedbacks, err := ma.store.ListMetaObservations(ctx, "retrieval_feedback", 50)
	if err != nil {
		ma.log.Warn("failed to list retrieval feedback", "error", err)
		return nil
	}

	if len(feedbacks) < 5 {
		return nil // not enough data
	}

	var helpful, partial, irrelevant int
	for _, fb := range feedbacks {
		// MCP feedback stores quality in "quality" field
		quality := ""
		if q, ok := fb.Details["quality"].(string); ok {
			quality = q
		} else if q, ok := fb.Details["rating"].(string); ok {
			quality = q
		}
		switch quality {
		case "helpful":
			helpful++
		case "partial":
			partial++
		case "irrelevant":
			irrelevant++
		}
	}

	total := helpful + partial + irrelevant
	if total == 0 {
		return nil
	}

	irrelevantRatio := float64(irrelevant) / float64(total)
	helpfulRatio := float64(helpful) / float64(total)

	severity := "info"
	if irrelevantRatio > 0.3 {
		severity = "warning"
	}
	if irrelevantRatio > 0.5 {
		severity = "critical"
	}

	return &store.MetaObservation{
		ObservationType: "retrieval_quality",
		Severity:        severity,
		Details: map[string]interface{}{
			"helpful_count":    helpful,
			"partial_count":    partial,
			"irrelevant_count": irrelevant,
			"total_feedback":   total,
			"helpful_ratio":    helpfulRatio,
			"irrelevant_ratio": irrelevantRatio,
		},
	}
}

// actOnObservations reviews observations and takes corrective actions.
// Returns the number of actions performed.
func (ma *MetacognitionAgent) actOnObservations(ctx context.Context, observations []store.MetaObservation) int {
	actions := 0

	for _, obs := range observations {
		switch obs.ObservationType {
		case "recall_effectiveness":
			actions += ma.actOnHighDeadRatio(ctx, obs)
		case "quality_audit":
			actions += ma.actOnQualityIssues(ctx, obs)
		case "consolidation_health":
			actions += ma.actOnStaleConsolidation(ctx, obs)
		case "retrieval_quality":
			actions += ma.actOnPoorRetrieval(ctx, obs)
		}
	}

	return actions
}

// actOnHighDeadRatio: detects high dead memory ratio.
// Consolidation triggering, cooldowns, and action logging are now handled
// by the reactor engine's "meta_consolidation_on_dead_ratio" chain.
func (ma *MetacognitionAgent) actOnHighDeadRatio(_ context.Context, obs store.MetaObservation) int {
	if obs.Severity != "critical" && obs.Severity != "warning" {
		return 0
	}
	ma.log.Info("high dead memory ratio detected (reactor will handle consolidation request)",
		"severity", obs.Severity)
	return 0
}

// actOnQualityIssues: re-embed memories that are missing embeddings.
func (ma *MetacognitionAgent) actOnQualityIssues(ctx context.Context, obs store.MetaObservation) int {
	if ma.llmProvider == nil {
		return 0
	}

	noEmbedding, _ := obs.Details["no_embedding"].(int)
	if noEmbedding == 0 {
		// Try float64 (JSON numbers decode as float64)
		if f, ok := obs.Details["no_embedding"].(float64); ok {
			noEmbedding = int(f)
		}
	}
	if noEmbedding == 0 {
		return 0
	}

	// Fetch memories missing embeddings and re-embed them (up to 10 per cycle)
	memories, err := ma.store.ListMemories(ctx, "active", 100, 0)
	if err != nil {
		return 0
	}

	reembedded := 0
	for _, mem := range memories {
		if reembedded >= 10 {
			break
		}
		if len(mem.Embedding) > 0 {
			continue
		}

		text := mem.Summary
		if text == "" {
			text = mem.Content
		}
		if text == "" {
			continue
		}

		embedding, err := ma.llmProvider.Embed(ctx, text)
		if err != nil {
			ma.log.Warn("re-embedding failed", "memory_id", mem.ID, "error", err)
			continue
		}

		mem.Embedding = embedding
		mem.UpdatedAt = time.Now()
		if err := ma.store.UpdateMemory(ctx, mem); err != nil {
			ma.log.Warn("failed to update re-embedded memory", "memory_id", mem.ID, "error", err)
			continue
		}

		reembedded++
	}

	if reembedded > 0 {
		ma.log.Info("re-embedded memories missing embeddings", "count", reembedded)

		action := store.MetaObservation{
			ID:              uuid.New().String(),
			ObservationType: "autonomous_action",
			Severity:        "info",
			Details: map[string]interface{}{
				"action": "re_embedded_memories",
				"count":  reembedded,
			},
			CreatedAt: time.Now(),
		}
		if err := ma.store.WriteMetaObservation(ctx, action); err != nil {
			ma.log.Warn("failed to write meta observation", "error", err)
		}
	}

	return reembedded
}

// actOnStaleConsolidation: emit a warning event when consolidation hasn't run in too long.
func (ma *MetacognitionAgent) actOnStaleConsolidation(ctx context.Context, obs store.MetaObservation) int {
	if obs.Severity != "warning" {
		return 0
	}

	ma.log.Warn("consolidation is stale, publishing system health warning")

	if ma.bus != nil {
		_ = ma.bus.Publish(ctx, events.SystemHealth{
			LLMAvailable: true,
			StoreHealthy: true,
			Ts:           time.Now(),
		})
	}

	return 1
}

// actOnPoorRetrieval: log a suggestion to adjust retrieval params when feedback is poor.
func (ma *MetacognitionAgent) actOnPoorRetrieval(ctx context.Context, obs store.MetaObservation) int {
	if obs.Severity != "warning" && obs.Severity != "critical" {
		return 0
	}

	irrelevantRatio, _ := obs.Details["irrelevant_ratio"].(float64)

	ma.log.Warn("poor retrieval quality detected",
		"irrelevant_ratio", irrelevantRatio,
		"suggestion", "consider adjusting spread activation depth or similarity threshold")

	action := store.MetaObservation{
		ID:              uuid.New().String(),
		ObservationType: "autonomous_action",
		Severity:        "warning",
		Details: map[string]interface{}{
			"action":           "retrieval_quality_alert",
			"irrelevant_ratio": irrelevantRatio,
			"suggestion":       "adjust spread activation params or similarity threshold",
		},
		CreatedAt: time.Now(),
	}
	if err := ma.store.WriteMetaObservation(ctx, action); err != nil {
		ma.log.Warn("failed to write meta observation", "error", err)
	}

	return 1
}

// processFeedback reads recent unprocessed retrieval feedback and adjusts associations/salience.
// "helpful" → strengthen associations between the query concepts and returned memories.
// "irrelevant" → weaken associations, slightly lower salience.
func (ma *MetacognitionAgent) processFeedback(ctx context.Context) int {
	feedbacks, err := ma.store.ListMetaObservations(ctx, "retrieval_feedback", 20)
	if err != nil || len(feedbacks) == 0 {
		return 0
	}

	actions := 0

	for _, fb := range feedbacks {
		quality := ""
		if q, ok := fb.Details["quality"].(string); ok {
			quality = q
		}

		// Extract memory IDs from the feedback
		var memoryIDs []string
		if ids, ok := fb.Details["memory_ids"].([]interface{}); ok {
			for _, id := range ids {
				if s, ok := id.(string); ok {
					memoryIDs = append(memoryIDs, s)
				}
			}
		}

		if len(memoryIDs) == 0 {
			continue
		}

		switch quality {
		case "helpful":
			actions += ma.reinforceMemories(ctx, memoryIDs)
		case "irrelevant":
			actions += ma.weakenMemories(ctx, memoryIDs)
		}
	}

	if actions > 0 {
		ma.log.Info("feedback processing completed", "adjustments", actions)
	}

	return actions
}

// reinforceMemories strengthens associations and boosts salience for helpful memories.
func (ma *MetacognitionAgent) reinforceMemories(ctx context.Context, memoryIDs []string) int {
	adjusted := 0

	for _, memID := range memoryIDs {
		mem, err := ma.store.GetMemory(ctx, memID)
		if err != nil {
			continue
		}

		// Boost salience slightly (capped at 1.0)
		newSalience := mem.Salience + 0.02
		if newSalience > 1.0 {
			newSalience = 1.0
		}
		if newSalience != mem.Salience {
			if err := ma.store.UpdateSalience(ctx, memID, newSalience); err == nil {
				adjusted++
			}
		}

		// Strengthen all associations for this memory
		assocs, err := ma.store.GetAssociations(ctx, memID)
		if err != nil {
			continue
		}
		for _, assoc := range assocs {
			newStrength := assoc.Strength * 1.05
			if newStrength > 1.0 {
				newStrength = 1.0
			}
			if newStrength != assoc.Strength {
				if err := ma.store.UpdateAssociationStrength(ctx, assoc.SourceID, assoc.TargetID, newStrength); err != nil {
					ma.log.Warn("failed to update association strength", "source", assoc.SourceID, "target", assoc.TargetID, "error", err)
				}
			}
		}
	}

	return adjusted
}

// weakenMemories reduces salience and weakens associations for irrelevant memories.
func (ma *MetacognitionAgent) weakenMemories(ctx context.Context, memoryIDs []string) int {
	adjusted := 0

	for _, memID := range memoryIDs {
		mem, err := ma.store.GetMemory(ctx, memID)
		if err != nil {
			continue
		}

		// Lower salience slightly
		newSalience := mem.Salience - 0.01
		if newSalience < 0.05 {
			newSalience = 0.05
		}
		if newSalience != mem.Salience {
			if err := ma.store.UpdateSalience(ctx, memID, newSalience); err == nil {
				adjusted++
			}
		}

		// Weaken all associations for this memory
		assocs, err := ma.store.GetAssociations(ctx, memID)
		if err != nil {
			continue
		}
		for _, assoc := range assocs {
			newStrength := assoc.Strength * 0.95
			if newStrength < 0.05 {
				newStrength = 0.05
			}
			if newStrength != assoc.Strength {
				if err := ma.store.UpdateAssociationStrength(ctx, assoc.SourceID, assoc.TargetID, newStrength); err != nil {
					ma.log.Warn("failed to update association strength", "source", assoc.SourceID, "target", assoc.TargetID, "error", err)
				}
			}
		}
	}

	return adjusted
}
