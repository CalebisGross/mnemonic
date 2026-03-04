package dreaming

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

type DreamingConfig struct {
	Interval               time.Duration
	BatchSize              int
	SalienceThreshold      float32
	AssociationBoostFactor float32
	NoisePruneThreshold    float32
}

type DreamingAgent struct {
	store       store.Store
	llmProvider llm.Provider
	config      DreamingConfig
	log         *slog.Logger
	bus         events.Bus
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopOnce    sync.Once
	triggerCh   chan struct{}
}

type DreamReport struct {
	Duration                 time.Duration
	MemoriesReplayed         int
	AssociationsStrengthened int
	NewAssociationsCreated   int
	CrossProjectLinks        int
	PatternLinks             int
	InsightsGenerated        int
	NoisyMemoriesDemoted     int
}

func NewDreamingAgent(s store.Store, llmProv llm.Provider, cfg DreamingConfig, log *slog.Logger) *DreamingAgent {
	return &DreamingAgent{
		store:       s,
		llmProvider: llmProv,
		config:      cfg,
		log:         log,
		triggerCh:   make(chan struct{}, 1),
	}
}

func (da *DreamingAgent) Name() string {
	return "dreaming-agent"
}

func (da *DreamingAgent) Start(ctx context.Context, bus events.Bus) error {
	da.ctx, da.cancel = context.WithCancel(ctx)
	da.bus = bus
	da.wg.Add(1)
	go da.loop()
	return nil
}

func (da *DreamingAgent) Stop() error {
	da.stopOnce.Do(func() {
		da.cancel()
	})
	da.wg.Wait()
	return nil
}

func (da *DreamingAgent) Health(ctx context.Context) error {
	_, err := da.store.CountMemories(ctx)
	return err
}

// GetTriggerChannel returns a channel that can be used to trigger an on-demand
// dream cycle (e.g. from the reactor after an episode closes).
func (da *DreamingAgent) GetTriggerChannel() chan<- struct{} {
	return da.triggerCh
}

func (da *DreamingAgent) RunOnce(ctx context.Context) (*DreamReport, error) {
	return da.runCycle(ctx)
}

func (da *DreamingAgent) loop() {
	defer da.wg.Done()

	// 90-second startup grace period
	startupTimer := time.NewTimer(90 * time.Second)
	defer startupTimer.Stop()

	ticker := time.NewTicker(da.config.Interval)
	defer ticker.Stop()

	runAndLog := func() {
		report, err := da.runCycle(da.ctx)
		if err != nil && da.ctx.Err() == nil {
			da.log.Error("dream cycle failed", "error", err)
		} else if report != nil {
			da.log.Info("dream cycle completed",
				"duration_ms", report.Duration.Milliseconds(),
				"memories_replayed", report.MemoriesReplayed,
				"associations_strengthened", report.AssociationsStrengthened,
				"new_associations_created", report.NewAssociationsCreated,
				"cross_project_links", report.CrossProjectLinks,
				"pattern_links", report.PatternLinks,
				"insights_generated", report.InsightsGenerated,
				"noisy_memories_demoted", report.NoisyMemoriesDemoted,
			)
		}
	}

	for {
		select {
		case <-da.ctx.Done():
			return
		case <-startupTimer.C:
			runAndLog()
		case <-ticker.C:
			runAndLog()
		case <-da.triggerCh:
			runAndLog()
		}
	}
}

func (da *DreamingAgent) runCycle(ctx context.Context) (*DreamReport, error) {
	// Gate on LLM availability — without LLM, dreaming blindly strengthens
	// associations without being able to generate insights or judge quality.
	if da.llmProvider != nil {
		if err := da.llmProvider.Health(ctx); err != nil {
			da.log.Warn("skipping dream cycle: LLM unavailable", "error", err)
			return nil, nil
		}
	}

	startTime := time.Now()
	report := &DreamReport{}

	// Phase 1: Replay memories by salience
	replayed, err := da.replayMemories(ctx, report)
	if err != nil && ctx.Err() == nil {
		da.log.Error("replay phase failed", "error", err)
	}

	// Phase 2: Strengthen associations for replayed memories
	if err := da.strengthenAssociations(ctx, replayed, report); err != nil && ctx.Err() == nil {
		da.log.Error("strengthen associations phase failed", "error", err)
	}

	// Phase 3: Cross-pollinate memories with shared concepts
	if err := da.crossPollinate(ctx, replayed, report); err != nil && ctx.Err() == nil {
		da.log.Error("cross-pollinate phase failed", "error", err)
	}

	// Phase 4: Cross-project linking — find similar memories across different projects
	if err := da.crossProjectLink(ctx, replayed, report); err != nil && ctx.Err() == nil {
		da.log.Error("cross-project linking phase failed", "error", err)
	}

	// Phase 5: Link replayed memories to matching patterns
	if err := da.linkToPatterns(ctx, replayed, report); err != nil && ctx.Err() == nil {
		da.log.Error("pattern linking phase failed", "error", err)
	}

	// Phase 6: Noise prune low-quality dead memories
	if err := da.noisePrune(ctx, report); err != nil && ctx.Err() == nil {
		da.log.Error("noise prune phase failed", "error", err)
	}

	// Phase 7: Generate insights from top replayed memories
	if err := da.generateInsights(ctx, replayed, report); err != nil && ctx.Err() == nil {
		da.log.Error("insight generation phase failed", "error", err)
	}

	report.Duration = time.Since(startTime)

	// Publish event
	if da.bus != nil {
		_ = da.bus.Publish(ctx, events.DreamCycleCompleted{
			MemoriesReplayed:         report.MemoriesReplayed,
			AssociationsStrengthened: report.AssociationsStrengthened,
			NewAssociationsCreated:   report.NewAssociationsCreated,
			CrossProjectLinks:        report.CrossProjectLinks,
			PatternLinks:             report.PatternLinks,
			InsightsGenerated:        report.InsightsGenerated,
			NoisyMemoriesDemoted:     report.NoisyMemoriesDemoted,
			DurationMs:               report.Duration.Milliseconds(),
			Ts:                       time.Now(),
		})
	}

	return report, nil
}

// replayMemories performs Phase 1: get top memories by salience and increment their access.
func (da *DreamingAgent) replayMemories(ctx context.Context, report *DreamReport) ([]store.Memory, error) {
	memories, err := da.store.ListMemories(ctx, "active", da.config.BatchSize, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list active memories: %w", err)
	}

	var replayed []store.Memory

	for _, mem := range memories {
		if mem.Salience >= da.config.SalienceThreshold {
			if err := da.store.IncrementAccess(ctx, mem.ID); err != nil {
				da.log.Warn("failed to increment access for memory", "memory_id", mem.ID, "error", err)
				continue
			}
			replayed = append(replayed, mem)
			report.MemoriesReplayed++
		}
	}

	return replayed, nil
}

// strengthenAssociations performs Phase 2: boost association strength for replayed memories.
func (da *DreamingAgent) strengthenAssociations(ctx context.Context, replayed []store.Memory, report *DreamReport) error {
	for _, mem := range replayed {
		assocs, err := da.store.GetAssociations(ctx, mem.ID)
		if err != nil {
			da.log.Warn("failed to get associations for memory", "memory_id", mem.ID, "error", err)
			continue
		}

		for _, assoc := range assocs {
			newStrength := assoc.Strength * da.config.AssociationBoostFactor
			if newStrength > 1.0 {
				newStrength = 1.0
			}

			if newStrength != assoc.Strength {
				if err := da.store.UpdateAssociationStrength(ctx, assoc.SourceID, assoc.TargetID, newStrength); err != nil {
					da.log.Warn("failed to update association strength", "source_id", assoc.SourceID, "target_id", assoc.TargetID, "error", err)
					continue
				}

				if err := da.store.ActivateAssociation(ctx, assoc.SourceID, assoc.TargetID); err != nil {
					da.log.Warn("failed to activate association", "source_id", assoc.SourceID, "target_id", assoc.TargetID, "error", err)
					continue
				}

				report.AssociationsStrengthened++
			}
		}
	}

	return nil
}

// crossPollinate performs Phase 3: find memories with shared concepts and create new associations.
func (da *DreamingAgent) crossPollinate(ctx context.Context, replayed []store.Memory, report *DreamReport) error {
	for _, mem := range replayed {
		if len(mem.Concepts) < 2 {
			continue
		}

		related, err := da.store.SearchByConcepts(ctx, mem.Concepts, 10)
		if err != nil {
			da.log.Warn("failed to search by concepts for memory", "memory_id", mem.ID, "error", err)
			continue
		}

		existingAssocs, err := da.store.GetAssociations(ctx, mem.ID)
		if err != nil {
			da.log.Warn("failed to get existing associations for memory", "memory_id", mem.ID, "error", err)
			continue
		}

		linkedIDs := make(map[string]bool)
		linkedIDs[mem.ID] = true
		for _, a := range existingAssocs {
			linkedIDs[a.SourceID] = true
			linkedIDs[a.TargetID] = true
		}

		for _, candidate := range related {
			if !linkedIDs[candidate.ID] && countSharedConcepts(mem.Concepts, candidate.Concepts) >= 2 {
				newAssoc := store.Association{
					SourceID:      mem.ID,
					TargetID:      candidate.ID,
					Strength:      0.3,
					RelationType:  "similar",
					CreatedAt:     time.Now(),
					LastActivated: time.Now(),
				}

				if err := da.store.CreateAssociation(ctx, newAssoc); err != nil {
					da.log.Warn("failed to create association", "source_id", mem.ID, "target_id", candidate.ID, "error", err)
					continue
				}

				report.NewAssociationsCreated++
			}
		}
	}

	return nil
}

// noisePrune performs Phase 4: reduce salience of low-quality dead memories.
func (da *DreamingAgent) noisePrune(ctx context.Context, report *DreamReport) error {
	deadMemories, err := da.store.GetDeadMemories(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to get dead memories: %w", err)
	}

	for _, mem := range deadMemories {
		if mem.Salience < da.config.NoisePruneThreshold {
			newSalience := mem.Salience * 0.8
			if err := da.store.UpdateSalience(ctx, mem.ID, newSalience); err != nil {
				da.log.Warn("failed to update salience for memory", "memory_id", mem.ID, "error", err)
				continue
			}
			report.NoisyMemoriesDemoted++
		}
	}

	return nil
}

// countSharedConcepts counts overlapping concepts between two concept lists (case-insensitive).
func countSharedConcepts(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	// Create a map of lowercase concepts from a
	aMap := make(map[string]bool)
	for _, concept := range a {
		aMap[strings.ToLower(concept)] = true
	}

	// Count matches in b
	count := 0
	for _, concept := range b {
		if aMap[strings.ToLower(concept)] {
			count++
		}
	}

	return count
}

// crossProjectLink searches for similar memories in other projects and creates cross-project associations.
func (da *DreamingAgent) crossProjectLink(ctx context.Context, replayed []store.Memory, report *DreamReport) error {
	for _, mem := range replayed {
		if mem.Project == "" || len(mem.Embedding) == 0 {
			continue
		}

		// Search by embedding across all memories — will include other projects
		similar, err := da.store.SearchByEmbedding(ctx, mem.Embedding, 10)
		if err != nil {
			da.log.Warn("cross-project search failed", "memory_id", mem.ID, "error", err)
			continue
		}

		// Get existing associations to avoid duplicates
		existingAssocs, err := da.store.GetAssociations(ctx, mem.ID)
		if err != nil {
			continue
		}
		linkedIDs := make(map[string]bool)
		linkedIDs[mem.ID] = true
		for _, a := range existingAssocs {
			linkedIDs[a.SourceID] = true
			linkedIDs[a.TargetID] = true
		}

		for _, result := range similar {
			// Only link memories from different projects
			if result.Memory.Project == "" || result.Memory.Project == mem.Project {
				continue
			}
			if linkedIDs[result.Memory.ID] {
				continue
			}
			if result.Score < 0.75 {
				continue
			}

			newAssoc := store.Association{
				SourceID:      mem.ID,
				TargetID:      result.Memory.ID,
				Strength:      result.Score * 0.5, // start weaker than same-project links
				RelationType:  "cross_project",
				CreatedAt:     time.Now(),
				LastActivated: time.Now(),
			}

			if err := da.store.CreateAssociation(ctx, newAssoc); err != nil {
				da.log.Warn("failed to create cross-project association",
					"source_id", mem.ID, "target_id", result.Memory.ID, "error", err)
				continue
			}
			report.CrossProjectLinks++
			linkedIDs[result.Memory.ID] = true
		}
	}
	return nil
}

// linkToPatterns links replayed memories to matching patterns via embedding similarity.
func (da *DreamingAgent) linkToPatterns(ctx context.Context, replayed []store.Memory, report *DreamReport) error {
	for _, mem := range replayed {
		if len(mem.Embedding) == 0 {
			continue
		}

		patterns, err := da.store.SearchPatternsByEmbedding(ctx, mem.Embedding, 3)
		if err != nil {
			da.log.Warn("pattern search failed", "memory_id", mem.ID, "error", err)
			continue
		}

		for _, pattern := range patterns {
			// Check if memory is already in pattern's evidence
			alreadyEvidence := false
			for _, eid := range pattern.EvidenceIDs {
				if eid == mem.ID {
					alreadyEvidence = true
					break
				}
			}
			if alreadyEvidence {
				continue
			}

			// Strengthen pattern by adding this memory as evidence
			pattern.EvidenceIDs = append(pattern.EvidenceIDs, mem.ID)
			// Boost strength slightly (capped at 1.0)
			pattern.Strength = pattern.Strength + 0.02
			if pattern.Strength > 1.0 {
				pattern.Strength = 1.0
			}
			pattern.UpdatedAt = time.Now()

			if err := da.store.UpdatePattern(ctx, pattern); err != nil {
				da.log.Warn("failed to update pattern with new evidence",
					"pattern_id", pattern.ID, "memory_id", mem.ID, "error", err)
				continue
			}
			report.PatternLinks++
		}
	}
	return nil
}

// generateInsights clusters the most-accessed replayed memories and asks LLM for higher-order insights.
// Stores results as Abstractions with level=2. Budget: max 2 per dream cycle.
func (da *DreamingAgent) generateInsights(ctx context.Context, replayed []store.Memory, report *DreamReport) error {
	if da.llmProvider == nil || len(replayed) < 3 {
		return nil
	}

	// Sort by access count (most accessed first)
	sorted := make([]store.Memory, len(replayed))
	copy(sorted, replayed)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].AccessCount > sorted[j].AccessCount
	})

	// Take top 5 most-accessed
	top := sorted
	if len(top) > 5 {
		top = top[:5]
	}

	// Cluster by concept overlap — find groups with >= 2 shared concepts
	clusters := clusterByConceptOverlap(top)
	if len(clusters) == 0 {
		return nil
	}

	insightsBudget := 2
	for _, cluster := range clusters {
		if insightsBudget <= 0 {
			break
		}
		if len(cluster) < 2 {
			continue
		}

		insight, err := da.synthesizeInsight(ctx, cluster)
		if err != nil {
			da.log.Warn("insight generation failed", "error", err)
			continue
		}
		if insight == nil {
			continue
		}

		if err := da.store.WriteAbstraction(ctx, *insight); err != nil {
			da.log.Warn("failed to store insight", "error", err)
			continue
		}

		report.InsightsGenerated++
		insightsBudget--
		da.log.Info("insight generated", "title", insight.Title, "level", insight.Level)
	}

	return nil
}

// clusterByConceptOverlap groups memories that share at least 2 concepts.
func clusterByConceptOverlap(memories []store.Memory) [][]store.Memory {
	if len(memories) == 0 {
		return nil
	}

	used := make([]bool, len(memories))
	var clusters [][]store.Memory

	for i := 0; i < len(memories); i++ {
		if used[i] {
			continue
		}
		cluster := []store.Memory{memories[i]}
		used[i] = true

		for j := i + 1; j < len(memories); j++ {
			if used[j] {
				continue
			}
			if countSharedConcepts(memories[i].Concepts, memories[j].Concepts) >= 2 {
				cluster = append(cluster, memories[j])
				used[j] = true
			}
		}

		if len(cluster) >= 2 {
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

type insightResponse struct {
	Title      string   `json:"title"`
	Insight    string   `json:"insight"`
	Concepts   []string `json:"concepts"`
	Confidence float64  `json:"confidence"`
	HasInsight bool     `json:"has_insight"`
}

// synthesizeInsight asks the LLM to identify a higher-order insight from a cluster of memories.
func (da *DreamingAgent) synthesizeInsight(ctx context.Context, cluster []store.Memory) (*store.Abstraction, error) {
	var summaries strings.Builder
	var memoryIDs []string
	var allConcepts []string

	for i, mem := range cluster {
		fmt.Fprintf(&summaries, "%d. [%s] %s\n   Concepts: %s\n",
			i+1, mem.Project, mem.Summary, strings.Join(mem.Concepts, ", "))
		memoryIDs = append(memoryIDs, mem.ID)
		allConcepts = append(allConcepts, mem.Concepts...)
	}

	prompt := fmt.Sprintf(`These memories keep surfacing — they're the ones this person's mind returns to most often. When you look at them together, what do they teach?

Is there a lesson here that's bigger than any single memory? A principle that connects them? Something this person has been learning, perhaps without realizing it?

Memories:
%s

Respond with ONLY a JSON object:
{
  "has_insight": true/false,
  "title": "a clear name for this insight",
  "insight": "the deeper lesson or principle, in 1-2 sentences — something genuinely useful",
  "concepts": ["key", "concepts"],
  "confidence": 0.0-1.0
}

Only share an insight if it's genuinely illuminating — something that makes you think "oh, that's interesting." If these memories are just individually notable without a connecting thread, set has_insight to false.`, summaries.String())

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are an insight generator. Find connections between memories. Output JSON only."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   200,
		Temperature: 0.4,
		ResponseFormat: &llm.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &llm.JSONSchema{
				Name:   "insight_response",
				Strict: true,
				Schema: json.RawMessage(`{"type":"object","properties":{"has_insight":{"type":"boolean"},"title":{"type":"string"},"insight":{"type":"string"},"concepts":{"type":"array","items":{"type":"string"}},"confidence":{"type":"number"}},"required":["has_insight","title","insight","concepts","confidence"],"additionalProperties":false}`),
			},
		},
	}

	resp, err := da.llmProvider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM insight generation failed: %w", err)
	}

	jsonStr := extractInsightJSON(resp.Content)
	var result insightResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse insight response: %w", err)
	}

	if !result.HasInsight || result.Title == "" || result.Insight == "" {
		return nil, nil
	}

	// Compute average embedding from cluster memories
	embedding := averageMemoryEmbedding(cluster)

	// Deduplicate concepts
	concepts := result.Concepts
	if len(concepts) == 0 {
		concepts = deduplicateConcepts(allConcepts)
	}

	confidence := float32(result.Confidence)
	if confidence <= 0 || confidence > 1.0 {
		confidence = 0.6
	}

	abstraction := &store.Abstraction{
		ID:              fmt.Sprintf("abs-%d", time.Now().UnixNano()),
		Level:           2, // principle level
		Title:           result.Title,
		Description:     result.Insight,
		SourceMemoryIDs: memoryIDs,
		Confidence:      confidence,
		Concepts:        concepts,
		Embedding:       embedding,
		State:           "active",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	return abstraction, nil
}

// averageMemoryEmbedding computes the element-wise average of memory embeddings.
func averageMemoryEmbedding(memories []store.Memory) []float32 {
	if len(memories) == 0 {
		return nil
	}

	var withEmb []store.Memory
	for _, m := range memories {
		if len(m.Embedding) > 0 {
			withEmb = append(withEmb, m)
		}
	}
	if len(withEmb) == 0 {
		return nil
	}

	dim := len(withEmb[0].Embedding)
	avg := make([]float32, dim)
	for _, m := range withEmb {
		if len(m.Embedding) != dim {
			continue
		}
		for i, v := range m.Embedding {
			avg[i] += v
		}
	}
	n := float32(len(withEmb))
	for i := range avg {
		avg[i] /= n
	}
	return avg
}

// extractInsightJSON extracts JSON from LLM response, handling markdown fences and prose.
func extractInsightJSON(s string) string {
	// Try to find JSON in markdown code fences
	if idx := strings.Index(s, "```json"); idx != -1 {
		start := idx + 7
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx != -1 {
		start := idx + 3
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	// Try to find raw JSON object
	if idx := strings.Index(s, "{"); idx != -1 {
		if end := strings.LastIndex(s, "}"); end > idx {
			return s[idx : end+1]
		}
	}
	return s
}

// deduplicateConcepts returns unique concepts (case-insensitive), preserving first occurrence's casing.
func deduplicateConcepts(concepts []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, c := range concepts {
		lower := strings.ToLower(c)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, c)
		}
	}
	return result
}
