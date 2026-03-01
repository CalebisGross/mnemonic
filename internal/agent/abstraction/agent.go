package abstraction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

type AbstractionConfig struct {
	Interval    time.Duration
	MinStrength float32
	MaxLLMCalls int
}

type AbstractionAgent struct {
	store       store.Store
	llmProvider llm.Provider
	config      AbstractionConfig
	log         *slog.Logger
	bus         events.Bus
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopOnce    sync.Once
	triggerCh   chan struct{} // allows on-demand abstraction when patterns are discovered
}

type CycleReport struct {
	Duration            time.Duration
	PatternsEvaluated   int
	PrinciplesCreated   int
	AxiomsCreated       int
	AbstractionsDemoted int
}

func NewAbstractionAgent(s store.Store, llmProv llm.Provider, cfg AbstractionConfig, log *slog.Logger) *AbstractionAgent {
	ctx, cancel := context.WithCancel(context.Background())
	return &AbstractionAgent{
		store:       s,
		llmProvider: llmProv,
		config:      cfg,
		log:         log,
		ctx:         ctx,
		cancel:      cancel,
		triggerCh:   make(chan struct{}, 1),
	}
}

func (aa *AbstractionAgent) Name() string {
	return "abstraction-agent"
}

func (aa *AbstractionAgent) Start(ctx context.Context, bus events.Bus) error {
	aa.bus = bus

	// On-demand triggers (via triggerCh) are now managed by the reactor engine,
	// which subscribes to PatternDiscovered and sends signals here.

	aa.wg.Add(1)
	go aa.loop()
	return nil
}

// GetTriggerChannel returns a send-only reference to the on-demand trigger channel.
// Used by the reactor engine to send abstraction signals.
func (aa *AbstractionAgent) GetTriggerChannel() chan<- struct{} {
	return aa.triggerCh
}

func (aa *AbstractionAgent) Stop() error {
	aa.stopOnce.Do(func() {
		aa.cancel()
	})
	aa.wg.Wait()
	return nil
}

func (aa *AbstractionAgent) Health(ctx context.Context) error {
	_, err := aa.store.CountMemories(ctx)
	return err
}

func (aa *AbstractionAgent) RunOnce(ctx context.Context) (*CycleReport, error) {
	return aa.runCycle(ctx)
}

func (aa *AbstractionAgent) loop() {
	defer aa.wg.Done()

	// 5-minute startup grace period (runs less frequently than other agents)
	startupTimer := time.NewTimer(5 * time.Minute)
	defer startupTimer.Stop()

	ticker := time.NewTicker(aa.config.Interval)
	defer ticker.Stop()

	runAndLog := func() {
		report, err := aa.runCycle(aa.ctx)
		if err != nil && aa.ctx.Err() == nil {
			aa.log.Error("abstraction cycle failed", "error", err)
		} else if report != nil {
			aa.log.Info("abstraction cycle completed",
				"duration_ms", report.Duration.Milliseconds(),
				"patterns_evaluated", report.PatternsEvaluated,
				"principles_created", report.PrinciplesCreated,
				"axioms_created", report.AxiomsCreated,
				"abstractions_demoted", report.AbstractionsDemoted,
			)
		}
	}

	for {
		select {
		case <-aa.ctx.Done():
			return
		case <-startupTimer.C:
			runAndLog()
		case <-ticker.C:
			runAndLog()
		case <-aa.triggerCh:
			aa.log.Info("running on-demand abstraction cycle (pattern discovered)")
			runAndLog()
		}

		// Drain any pending trigger to prevent back-to-back on-demand runs.
		// If a PatternDiscovered event arrived during a cycle, discard the stacked trigger.
		select {
		case <-aa.triggerCh:
			aa.log.Debug("drained stacked abstraction trigger")
		default:
		}
	}
}

func (aa *AbstractionAgent) runCycle(ctx context.Context) (*CycleReport, error) {
	startTime := time.Now()
	report := &CycleReport{}

	// Step 1: Synthesize principles from strong patterns (level 2)
	if err := aa.synthesizePrinciples(ctx, report); err != nil && ctx.Err() == nil {
		aa.log.Error("principle synthesis failed", "error", err)
	}

	// Step 2: Synthesize axioms from principles (level 3)
	if err := aa.synthesizeAxioms(ctx, report); err != nil && ctx.Err() == nil {
		aa.log.Error("axiom synthesis failed", "error", err)
	}

	// Step 3: Verify grounding — demote abstractions with decayed evidence
	if err := aa.verifyGrounding(ctx, report); err != nil && ctx.Err() == nil {
		aa.log.Error("grounding verification failed", "error", err)
	}

	report.Duration = time.Since(startTime)
	return report, nil
}

// synthesizePrinciples loads strong patterns, clusters by embedding similarity, and asks LLM to synthesize principles.
func (aa *AbstractionAgent) synthesizePrinciples(ctx context.Context, report *CycleReport) error {
	patterns, err := aa.store.ListPatterns(ctx, "", 50) // all projects
	if err != nil {
		return fmt.Errorf("failed to list patterns: %w", err)
	}

	// Filter to strong patterns
	var strong []store.Pattern
	for _, p := range patterns {
		if p.Strength >= aa.config.MinStrength && p.State == "active" {
			strong = append(strong, p)
		}
	}
	report.PatternsEvaluated = len(strong)

	if len(strong) < 2 {
		return nil
	}

	// Cluster patterns by embedding similarity
	clusters := clusterPatterns(strong, 0.7)

	llmBudget := aa.config.MaxLLMCalls / 2 // reserve half for axioms
	if llmBudget < 1 {
		llmBudget = 1
	}

	// Load existing principles once for dedup checks
	existingPrinciples, _ := aa.store.ListAbstractions(ctx, 2, 200)

	for _, cluster := range clusters {
		if llmBudget <= 0 {
			break
		}
		if len(cluster) < 2 {
			continue
		}

		principle, err := aa.synthesizePrinciple(ctx, cluster)
		if err != nil {
			aa.log.Warn("principle synthesis failed for cluster", "error", err)
			llmBudget--
			continue
		}
		if principle == nil {
			llmBudget--
			continue
		}

		// Dedup: compare the synthesized principle's own embedding against existing ones.
		// Using 0.85 threshold since both are text-derived embeddings in the same space.
		if len(principle.Embedding) > 0 {
			if match := findSimilarAbstraction(existingPrinciples, principle.Embedding, 0.85); match != nil {
				// Strengthen the existing principle instead of creating a duplicate
				match.Confidence = min32(match.Confidence+0.05, 1.0)
				match.AccessCount++
				match.UpdatedAt = time.Now()
				if err := aa.store.UpdateAbstraction(ctx, *match); err != nil {
					aa.log.Warn("failed to strengthen existing principle", "id", match.ID, "error", err)
				} else {
					aa.log.Info("strengthened existing principle (dedup)",
						"id", match.ID, "title", match.Title, "confidence", match.Confidence)
				}
				llmBudget--
				continue
			}
		}

		if err := aa.store.WriteAbstraction(ctx, *principle); err != nil {
			aa.log.Warn("failed to store principle", "error", err)
			continue
		}

		// Track newly created principle for dedup within this cycle
		existingPrinciples = append(existingPrinciples, *principle)

		report.PrinciplesCreated++
		llmBudget--

		if aa.bus != nil {
			_ = aa.bus.Publish(ctx, events.AbstractionCreated{
				AbstractionID: principle.ID,
				Level:         2,
				Title:         principle.Title,
				SourceCount:   len(cluster),
				Ts:            time.Now(),
			})
		}
		aa.log.Info("principle synthesized", "title", principle.Title, "source_patterns", len(cluster))
	}

	return nil
}

// synthesizeAxioms clusters level-2 abstractions and synthesizes level-3 axioms.
func (aa *AbstractionAgent) synthesizeAxioms(ctx context.Context, report *CycleReport) error {
	principles, err := aa.store.ListAbstractions(ctx, 2, 50)
	if err != nil {
		return fmt.Errorf("failed to list principles: %w", err)
	}

	// Need at least 2 active principles
	var active []store.Abstraction
	for _, p := range principles {
		if p.State == "active" && p.Confidence >= 0.5 {
			active = append(active, p)
		}
	}

	if len(active) < 2 {
		return nil
	}

	clusters := clusterAbstractions(active, 0.75)

	llmBudget := aa.config.MaxLLMCalls / 2
	if llmBudget < 1 {
		llmBudget = 1
	}

	// Load existing axioms once for dedup checks
	existingAxioms, _ := aa.store.ListAbstractions(ctx, 3, 200)

	for _, cluster := range clusters {
		if llmBudget <= 0 {
			break
		}
		if len(cluster) < 2 {
			continue
		}

		axiom, err := aa.synthesizeAxiom(ctx, cluster)
		if err != nil {
			aa.log.Warn("axiom synthesis failed", "error", err)
			llmBudget--
			continue
		}
		if axiom == nil {
			llmBudget--
			continue
		}

		// Dedup: compare the synthesized axiom's own embedding against existing ones
		if len(axiom.Embedding) > 0 {
			if match := findSimilarAbstraction(existingAxioms, axiom.Embedding, 0.85); match != nil {
				match.Confidence = min32(match.Confidence+0.05, 1.0)
				match.AccessCount++
				match.UpdatedAt = time.Now()
				if err := aa.store.UpdateAbstraction(ctx, *match); err != nil {
					aa.log.Warn("failed to strengthen existing axiom", "id", match.ID, "error", err)
				} else {
					aa.log.Info("strengthened existing axiom (dedup)",
						"id", match.ID, "title", match.Title, "confidence", match.Confidence)
				}
				llmBudget--
				continue
			}
		}

		if err := aa.store.WriteAbstraction(ctx, *axiom); err != nil {
			aa.log.Warn("failed to store axiom", "error", err)
			continue
		}

		existingAxioms = append(existingAxioms, *axiom)

		report.AxiomsCreated++
		llmBudget--

		if aa.bus != nil {
			_ = aa.bus.Publish(ctx, events.AbstractionCreated{
				AbstractionID: axiom.ID,
				Level:         3,
				Title:         axiom.Title,
				SourceCount:   len(cluster),
				Ts:            time.Now(),
			})
		}
		aa.log.Info("axiom synthesized", "title", axiom.Title, "source_principles", len(cluster))
	}

	return nil
}

// verifyGrounding checks that abstractions still have active supporting evidence.
func (aa *AbstractionAgent) verifyGrounding(ctx context.Context, report *CycleReport) error {
	for _, level := range []int{2, 3} {
		abstractions, err := aa.store.ListAbstractions(ctx, level, 50)
		if err != nil {
			continue
		}

		for _, abs := range abstractions {
			if abs.State != "active" {
				continue
			}

			// Check source memories — if most are archived/fading, reduce confidence
			activeEvidence := 0
			totalEvidence := len(abs.SourceMemoryIDs) + len(abs.SourcePatternIDs)
			if totalEvidence == 0 {
				continue
			}

			for _, memID := range abs.SourceMemoryIDs {
				mem, err := aa.store.GetMemory(ctx, memID)
				if err == nil && (mem.State == "active" || mem.State == "fading") {
					activeEvidence++
				}
			}

			for _, patID := range abs.SourcePatternIDs {
				pat, err := aa.store.GetPattern(ctx, patID)
				if err == nil && pat.State == "active" {
					activeEvidence++
				}
			}

			groundingRatio := float32(activeEvidence) / float32(totalEvidence)

			// Access-count protection: frequently-retrieved abstractions resist decay
			if abs.AccessCount > 5 && groundingRatio >= 0.1 {
				continue
			}

			// Graduated grounding response
			switch {
			case groundingRatio >= 0.5:
				// Healthy grounding, no action needed
				continue
			case groundingRatio >= 0.3:
				// Moderate decay: reduce confidence slightly
				abs.Confidence *= 0.9
			case groundingRatio >= 0.1:
				// Significant decay: reduce confidence more
				abs.Confidence *= 0.7
				report.AbstractionsDemoted++
			default:
				// Nearly all evidence gone: aggressive demotion
				abs.Confidence *= 0.3
				if abs.Confidence < 0.1 {
					abs.State = "fading"
				}
				report.AbstractionsDemoted++
			}

			abs.UpdatedAt = time.Now()
			if err := aa.store.UpdateAbstraction(ctx, abs); err != nil {
				aa.log.Warn("failed to update abstraction grounding", "id", abs.ID, "error", err)
				continue
			}
			aa.log.Info("abstraction grounding adjusted",
				"id", abs.ID, "title", abs.Title, "grounding_ratio", groundingRatio, "new_confidence", abs.Confidence, "state", abs.State)
		}
	}

	return nil
}

type principleResponse struct {
	HasPrinciple bool     `json:"has_principle"`
	Title        string   `json:"title"`
	Principle    string   `json:"principle"`
	Concepts     []string `json:"concepts"`
	Confidence   float64  `json:"confidence"`
}

// synthesizePrinciple asks LLM to identify a principle from a cluster of patterns.
func (aa *AbstractionAgent) synthesizePrinciple(ctx context.Context, patterns []store.Pattern) (*store.Abstraction, error) {
	var descriptions strings.Builder
	var patternIDs []string
	var allConcepts []string

	for i, p := range patterns {
		fmt.Fprintf(&descriptions, "%d. [%s] %s: %s\n   Concepts: %s\n",
			i+1, p.PatternType, p.Title, p.Description, strings.Join(p.Concepts, ", "))
		patternIDs = append(patternIDs, p.ID)
		allConcepts = append(allConcepts, p.Concepts...)
	}

	prompt := fmt.Sprintf(`These patterns have been quietly emerging from someone's work — recurring themes discovered over time. Step back and look at the bigger picture. Is there a principle that ties them together?

Think of it like this: if these patterns are the "what," what's the "why"? What general rule or guiding truth would explain why these patterns keep showing up?

Patterns:
%s

Respond with ONLY a JSON object:
{
  "has_principle": true/false,
  "title": "a memorable name for this principle",
  "principle": "the unifying principle in 1-2 clear sentences — something genuinely actionable",
  "concepts": ["key", "concepts"],
  "confidence": 0.0-1.0
}

Only share a principle if it genuinely unifies these patterns in an insightful way. A good principle should make someone nod and say "yes, that's exactly right." If the patterns are just loosely related, set has_principle to false.`, descriptions.String())

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are a principle synthesizer. Extract general principles from patterns. Output JSON only."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   200,
		Temperature: 0.3,
		ResponseFormat: &llm.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &llm.JSONSchema{
				Name:   "principle_response",
				Strict: true,
				Schema: json.RawMessage(`{"type":"object","properties":{"has_principle":{"type":"boolean"},"title":{"type":"string"},"principle":{"type":"string"},"concepts":{"type":"array","items":{"type":"string"}},"confidence":{"type":"number"}},"required":["has_principle","title","principle","concepts","confidence"],"additionalProperties":false}`),
			},
		},
	}

	resp, err := aa.llmProvider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM principle synthesis failed: %w", err)
	}

	jsonStr := extractJSON(resp.Content)
	var result principleResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse principle response: %w", err)
	}

	if !result.HasPrinciple || result.Title == "" || result.Principle == "" {
		return nil, nil
	}

	// Generate embedding from the principle's own text (more precise than averaged pattern embeddings)
	principleText := result.Title + ": " + result.Principle
	embedding, embErr := aa.llmProvider.Embed(ctx, principleText)
	if embErr != nil {
		aa.log.Warn("failed to embed principle text, falling back to pattern average", "error", embErr)
		embedding = averagePatternEmbedding(patterns)
	}

	concepts := result.Concepts
	if len(concepts) == 0 {
		concepts = deduplicateConcepts(allConcepts)
	}

	confidence := float32(result.Confidence)
	if confidence <= 0 || confidence > 1.0 {
		confidence = 0.6
	}

	return &store.Abstraction{
		ID:               fmt.Sprintf("abs-%d", time.Now().UnixNano()),
		Level:            2,
		Title:            result.Title,
		Description:      result.Principle,
		SourcePatternIDs: patternIDs,
		Confidence:       confidence,
		Concepts:         concepts,
		Embedding:        embedding,
		State:            "active",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}, nil
}

type axiomResponse struct {
	HasAxiom   bool     `json:"has_axiom"`
	Title      string   `json:"title"`
	Axiom      string   `json:"axiom"`
	Concepts   []string `json:"concepts"`
	Confidence float64  `json:"confidence"`
}

// synthesizeAxiom asks LLM to identify a fundamental axiom from a cluster of principles.
func (aa *AbstractionAgent) synthesizeAxiom(ctx context.Context, principles []store.Abstraction) (*store.Abstraction, error) {
	var descriptions strings.Builder
	var sourceIDs []string
	var allConcepts []string

	for i, p := range principles {
		fmt.Fprintf(&descriptions, "%d. %s: %s\n   Concepts: %s\n",
			i+1, p.Title, p.Description, strings.Join(p.Concepts, ", "))
		sourceIDs = append(sourceIDs, p.ID)
		allConcepts = append(allConcepts, p.Concepts...)
	}

	prompt := fmt.Sprintf(`These principles have emerged from layers of experience — each one discovered through patterns, and each pattern built from real memories. Now zoom out one more level.

Is there a fundamental truth here — something almost axiomatic? The kind of insight that, once you see it, changes how you approach everything?

Principles:
%s

Respond with ONLY a JSON object:
{
  "has_axiom": true/false,
  "title": "a concise name for this truth",
  "axiom": "the fundamental insight in 1-2 sentences — something that feels deeply true",
  "concepts": ["key", "concepts"],
  "confidence": 0.0-1.0
}

This is the highest level of abstraction — only share an axiom if it's genuinely profound. If these principles are related but don't converge on a deeper truth, set has_axiom to false.`, descriptions.String())

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are an axiom synthesizer. Extract deep truths from principles. Output JSON only."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   200,
		Temperature: 0.3,
		ResponseFormat: &llm.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &llm.JSONSchema{
				Name:   "axiom_response",
				Strict: true,
				Schema: json.RawMessage(`{"type":"object","properties":{"has_axiom":{"type":"boolean"},"title":{"type":"string"},"axiom":{"type":"string"},"concepts":{"type":"array","items":{"type":"string"}},"confidence":{"type":"number"}},"required":["has_axiom","title","axiom","concepts","confidence"],"additionalProperties":false}`),
			},
		},
	}

	resp, err := aa.llmProvider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM axiom synthesis failed: %w", err)
	}

	jsonStr := extractJSON(resp.Content)
	var result axiomResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse axiom response: %w", err)
	}

	if !result.HasAxiom || result.Title == "" || result.Axiom == "" {
		return nil, nil
	}

	// Generate embedding from the axiom's own text (more precise than averaged principle embeddings)
	axiomText := result.Title + ": " + result.Axiom
	embedding, embErr := aa.llmProvider.Embed(ctx, axiomText)
	if embErr != nil {
		aa.log.Warn("failed to embed axiom text, falling back to principle average", "error", embErr)
		embedding = averageAbstractionEmbedding(principles)
	}

	concepts := result.Concepts
	if len(concepts) == 0 {
		concepts = deduplicateConcepts(allConcepts)
	}

	confidence := float32(result.Confidence)
	if confidence <= 0 || confidence > 1.0 {
		confidence = 0.5
	}

	return &store.Abstraction{
		ID:               fmt.Sprintf("axm-%d", time.Now().UnixNano()),
		Level:            3,
		Title:            result.Title,
		Description:      result.Axiom,
		SourcePatternIDs: sourceIDs, // these are actually principle IDs
		Confidence:       confidence,
		Concepts:         concepts,
		Embedding:        embedding,
		State:            "active",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}, nil
}

// --- Helper functions ---

// clusterPatterns groups patterns by embedding similarity (greedy clustering).
func clusterPatterns(patterns []store.Pattern, threshold float32) [][]store.Pattern {
	if len(patterns) == 0 {
		return nil
	}

	used := make([]bool, len(patterns))
	var clusters [][]store.Pattern

	for i := 0; i < len(patterns); i++ {
		if used[i] || len(patterns[i].Embedding) == 0 {
			continue
		}
		cluster := []store.Pattern{patterns[i]}
		used[i] = true

		for j := i + 1; j < len(patterns); j++ {
			if used[j] || len(patterns[j].Embedding) == 0 {
				continue
			}
			if cosineSimilarity(patterns[i].Embedding, patterns[j].Embedding) >= threshold {
				cluster = append(cluster, patterns[j])
				used[j] = true
			}
		}

		if len(cluster) >= 2 {
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

// clusterAbstractions groups abstractions by embedding similarity.
func clusterAbstractions(abstractions []store.Abstraction, threshold float32) [][]store.Abstraction {
	if len(abstractions) == 0 {
		return nil
	}

	used := make([]bool, len(abstractions))
	var clusters [][]store.Abstraction

	for i := 0; i < len(abstractions); i++ {
		if used[i] || len(abstractions[i].Embedding) == 0 {
			continue
		}
		cluster := []store.Abstraction{abstractions[i]}
		used[i] = true

		for j := i + 1; j < len(abstractions); j++ {
			if used[j] || len(abstractions[j].Embedding) == 0 {
				continue
			}
			if cosineSimilarity(abstractions[i].Embedding, abstractions[j].Embedding) >= threshold {
				cluster = append(cluster, abstractions[j])
				used[j] = true
			}
		}

		if len(cluster) >= 2 {
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

// findSimilarAbstraction returns the first existing abstraction with embedding similarity >= threshold, or nil.
func findSimilarAbstraction(existing []store.Abstraction, embedding []float32, threshold float32) *store.Abstraction {
	for i, abs := range existing {
		if len(abs.Embedding) > 0 && abs.State == "active" && cosineSimilarity(abs.Embedding, embedding) >= threshold {
			return &existing[i]
		}
	}
	return nil
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt32(normA) * sqrt32(normB))
}

func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton's method for float32 square root
	guess := x / 2
	for i := 0; i < 10; i++ {
		guess = (guess + x/guess) / 2
	}
	return guess
}

// averagePatternEmbedding computes the element-wise average of pattern embeddings.
func averagePatternEmbedding(patterns []store.Pattern) []float32 {
	var withEmb [][]float32
	for _, p := range patterns {
		if len(p.Embedding) > 0 {
			withEmb = append(withEmb, p.Embedding)
		}
	}
	return averageVectors(withEmb)
}

// averageAbstractionEmbedding computes the element-wise average of abstraction embeddings.
func averageAbstractionEmbedding(abstractions []store.Abstraction) []float32 {
	var withEmb [][]float32
	for _, a := range abstractions {
		if len(a.Embedding) > 0 {
			withEmb = append(withEmb, a.Embedding)
		}
	}
	return averageVectors(withEmb)
}

func averageVectors(vecs [][]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	avg := make([]float32, dim)
	for _, v := range vecs {
		if len(v) != dim {
			continue
		}
		for i, val := range v {
			avg[i] += val
		}
	}
	n := float32(len(vecs))
	for i := range avg {
		avg[i] /= n
	}
	return avg
}

// extractJSON extracts JSON from LLM response, handling markdown fences.
func extractJSON(s string) string {
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
	if idx := strings.Index(s, "{"); idx != -1 {
		if end := strings.LastIndex(s, "}"); end > idx {
			return s[idx : end+1]
		}
	}
	return s
}

// deduplicateConcepts returns unique concepts, case-insensitive.
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
