package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/google/uuid"
)

// RetrievalConfig holds configurable parameters for the retrieval agent.
type RetrievalConfig struct {
	MaxHops             int
	ActivationThreshold float32
	DecayFactor         float32
	MaxResults          int
	MaxToolCalls        int     // max tool invocations per synthesis
	SynthesisMaxTokens  int     // max tokens per synthesis LLM call
	MergeAlpha          float32 // weight of embedding vs FTS in score merge (0-1)
	DualHitBonus        float32 // bonus for memories found by both FTS and embedding
}

// DefaultConfig returns sensible defaults for retrieval configuration.
func DefaultConfig() RetrievalConfig {
	return RetrievalConfig{
		MaxHops:             3,
		ActivationThreshold: 0.1,
		DecayFactor:         0.7,
		MaxResults:          7,
		MaxToolCalls:        5,
		SynthesisMaxTokens:  1024,
		MergeAlpha:          0.6,
		DualHitBonus:        0.15,
	}
}

// QueryRequest is the input for a retrieval query.
type QueryRequest struct {
	Query               string
	MaxResults          int       // override config default, 0 = use default
	IncludeReasoning    bool      // if true, add explanation to each result
	Synthesize          bool      // if true, ask LLM to synthesize a narrative
	IncludePatterns     bool      // if true, search and include matching patterns
	IncludeAbstractions bool      // if true, search and include matching abstractions
	Project             string    // if set, filter to this project
	TimeFrom            time.Time // if set, filter memories created after this time
	TimeTo              time.Time // if set, filter memories created before this time
}

// QueryResponse is the output of a retrieval query.
type QueryResponse struct {
	QueryID         string                  `json:"query_id"`
	Memories        []store.RetrievalResult `json:"memories"`
	Patterns        []store.Pattern         `json:"patterns,omitempty"`
	Abstractions    []store.Abstraction     `json:"abstractions,omitempty"`
	Synthesis       string                  `json:"synthesis,omitempty"`
	TraversedAssocs []store.TraversedAssoc  `json:"traversed_assocs,omitempty"`
	TookMs          int64                   `json:"took_ms"`
}

// RetrievalAgent performs memory retrieval using full-text search, embeddings, and spread activation.
type RetrievalAgent struct {
	store  store.Store
	llm    llm.Provider
	config RetrievalConfig
	log    *slog.Logger
	mu     sync.RWMutex
	stats  *retrievalStats
}

// retrievalStats tracks retrieval performance metrics.
type retrievalStats struct {
	TotalQueries     int64
	TotalMemoriesHit int64
	AvgActivationMs  int64
	AvgSynthesisMs   int64
	LastQueryTime    time.Time
}

// NewRetrievalAgent creates a new retrieval agent with the given dependencies.
func NewRetrievalAgent(s store.Store, llmProv llm.Provider, cfg RetrievalConfig, log *slog.Logger) *RetrievalAgent {
	return &RetrievalAgent{
		store:  s,
		llm:    llmProv,
		config: cfg,
		log:    log,
		stats: &retrievalStats{
			TotalQueries: 0,
		},
	}
}

// Query executes a retrieval query and returns ranked results with optional synthesis.
func (ra *RetrievalAgent) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	startTime := time.Now()
	queryID := uuid.New().String()

	ra.log.Debug("starting retrieval query", "query_id", queryID, "query", req.Query, "synthesize", req.Synthesize)

	// Auto-detect temporal intent from query text when no explicit time range is set
	if req.TimeFrom.IsZero() && req.TimeTo.IsZero() {
		temporal := parseTemporalIntent(req.Query, time.Now())
		if temporal.Detected {
			req.TimeFrom = temporal.From
			req.TimeTo = temporal.To
			ra.log.Debug("temporal intent detected", "query_id", queryID, "from", req.TimeFrom, "to", req.TimeTo)
		}
	}

	// Determine max results
	maxResults := ra.config.MaxResults
	if req.MaxResults > 0 {
		maxResults = req.MaxResults
	}

	// Step 1: Parse the query to extract concepts
	concepts := parseQueryConcepts(req.Query)
	ra.log.Debug("query concepts extracted", "query_id", queryID, "concepts_count", len(concepts))

	// Step 2: Find entry points via full-text search
	ftsResults, err := ra.store.SearchByFullText(ctx, req.Query, 10)
	if err != nil {
		ra.log.Warn("full-text search failed", "query_id", queryID, "error", err)
		ftsResults = []store.Memory{}
	}
	ra.log.Debug("full-text search completed", "query_id", queryID, "results_count", len(ftsResults))

	// Step 3: Find entry points via embedding search
	var embeddingResults []store.RetrievalResult
	embedding, err := ra.llm.Embed(ctx, req.Query)
	if err != nil {
		ra.log.Warn("embedding generation failed", "query_id", queryID, "error", err)
	} else {
		embeddingResults, err = ra.store.SearchByEmbedding(ctx, embedding, 10)
		if err != nil {
			ra.log.Warn("embedding search failed", "query_id", queryID, "error", err)
			embeddingResults = []store.RetrievalResult{}
		}
		ra.log.Debug("embedding search completed", "query_id", queryID, "results_count", len(embeddingResults))
	}

	// Step 3b: When temporal intent is detected, also fetch memories by time range
	// to ensure time-relevant results are included even if text/embedding search misses them
	var timeRangeResults []store.Memory
	if !req.TimeFrom.IsZero() && !req.TimeTo.IsZero() {
		timeRangeResults, err = ra.store.ListMemoriesByTimeRange(ctx, req.TimeFrom, req.TimeTo, maxResults)
		if err != nil {
			ra.log.Warn("time range search failed", "query_id", queryID, "error", err)
		} else {
			ra.log.Debug("time range search completed", "query_id", queryID, "results_count", len(timeRangeResults))
		}
	}

	// Step 4: Merge and deduplicate entry points
	entryPoints := ra.mergeEntryPoints(ftsResults, embeddingResults)

	// Inject time-range results as additional entry points with a moderate base score
	for _, mem := range timeRangeResults {
		if _, exists := entryPoints[mem.ID]; !exists {
			entryPoints[mem.ID] = 0.3 + 0.2*mem.Salience
		}
	}
	ra.log.Debug("entry points merged and deduplicated", "query_id", queryID, "entry_points_count", len(entryPoints))

	// Step 5: Spread activation across the association graph
	activated, traversedAssocs := ra.spreadActivation(ctx, entryPoints)
	ra.log.Debug("spread activation completed", "query_id", queryID, "activated_memories_count", len(activated), "traversals", len(traversedAssocs))

	// Step 6: Rank results by combined score
	ranked := ra.rankResults(activated, req.IncludeReasoning)

	// Step 7: Apply project and time filters (before truncation so matching results aren't discarded)
	if req.Project != "" || !req.TimeFrom.IsZero() || !req.TimeTo.IsZero() {
		ranked = ra.applyFilters(ranked, req)
	}

	// Step 8: Constrain to maxResults
	if len(ranked) > maxResults {
		ranked = ranked[:maxResults]
	}

	// Step 9: Side effect - increment access counts for returned memories
	for _, result := range ranked {
		if err := ra.store.IncrementAccess(ctx, result.Memory.ID); err != nil {
			ra.log.Warn("failed to increment access count", "query_id", queryID, "memory_id", result.Memory.ID, "error", err)
		}
	}

	// Step 10: Search patterns and abstractions by embedding
	var matchedPatterns []store.Pattern
	var matchedAbstractions []store.Abstraction

	if embedding != nil {
		if req.IncludePatterns {
			patterns, err := ra.store.SearchPatternsByEmbedding(ctx, embedding, 5)
			if err != nil {
				ra.log.Warn("pattern search failed", "query_id", queryID, "error", err)
			} else {
				// Filter by project if specified
				for _, p := range patterns {
					if req.Project == "" || p.Project == "" || p.Project == req.Project {
						matchedPatterns = append(matchedPatterns, p)
					}
				}
			}
		}

		if req.IncludeAbstractions {
			abs, err := ra.store.SearchAbstractionsByEmbedding(ctx, embedding, 5)
			if err != nil {
				ra.log.Warn("abstraction search failed", "query_id", queryID, "error", err)
			} else {
				matchedAbstractions = abs
			}
		}
	}

	// Step 11: Optional synthesis (now includes patterns and abstractions)
	var synthesis string
	if req.Synthesize {
		synthStart := time.Now()
		synthesis, err = ra.synthesizeNarrative(ctx, req.Query, ranked, matchedPatterns, matchedAbstractions)
		if err != nil {
			ra.log.Warn("synthesis failed", "query_id", queryID, "error", err)
			synthesis = ""
		}
		synthesisMs := time.Since(synthStart).Milliseconds()
		ra.log.Debug("synthesis completed", "query_id", queryID, "synthesis_length", len(synthesis), "took_ms", synthesisMs)

		ra.mu.Lock()
		ra.stats.AvgSynthesisMs = (ra.stats.AvgSynthesisMs + synthesisMs) / 2
		ra.mu.Unlock()
	}

	// Calculate total time
	tookMs := time.Since(startTime).Milliseconds()

	// Update stats
	ra.mu.Lock()
	ra.stats.TotalQueries++
	ra.stats.TotalMemoriesHit += int64(len(ranked))
	ra.stats.LastQueryTime = startTime
	ra.mu.Unlock()

	ra.log.Info("retrieval query completed", "query_id", queryID, "results_count", len(ranked),
		"patterns", len(matchedPatterns), "abstractions", len(matchedAbstractions), "took_ms", tookMs)

	return QueryResponse{
		QueryID:         queryID,
		Memories:        ranked,
		Patterns:        matchedPatterns,
		Abstractions:    matchedAbstractions,
		Synthesis:       synthesis,
		TraversedAssocs: traversedAssocs,
		TookMs:          tookMs,
	}, nil
}

// mergeEntryPoints combines FTS and embedding results with a weighted blend.
// Memories found by both methods get a dual-hit bonus to reward convergent evidence.
func (ra *RetrievalAgent) mergeEntryPoints(ftsResults []store.Memory, embeddingResults []store.RetrievalResult) map[string]float32 {
	ftsScores := make(map[string]float32)
	embScores := make(map[string]float32)

	// FTS results: normalize salience into a bounded relevance signal
	for _, mem := range ftsResults {
		salience := mem.Salience
		if salience <= 0 {
			salience = 0.5
		}
		ftsScores[mem.ID] = 0.3 + 0.4*salience // maps [0,1] → [0.3, 0.7]
	}

	// Embedding results: use cosine similarity directly
	for _, result := range embeddingResults {
		embScores[result.Memory.ID] = result.Score
	}

	// Union all candidate IDs
	allIDs := make(map[string]bool)
	for id := range ftsScores {
		allIDs[id] = true
	}
	for id := range embScores {
		allIDs[id] = true
	}

	alpha := ra.config.MergeAlpha
	dualHitBonus := ra.config.DualHitBonus

	entryPoints := make(map[string]float32)
	for id := range allIDs {
		fts, hasFTS := ftsScores[id]
		emb, hasEmb := embScores[id]

		var score float32
		switch {
		case hasFTS && hasEmb:
			score = alpha*emb + (1-alpha)*fts + dualHitBonus
		case hasEmb:
			score = emb
		default:
			score = fts
		}
		entryPoints[id] = score
	}

	return entryPoints
}

// activationState tracks a memory's activation level during spread activation.
type activationState struct {
	activation      float32
	hopsReached     int
	activationCount int // cumulative activation_count from traversed associations
}

// getAssociationTypeWeight returns the weight multiplier for a given association relationship type.
func getAssociationTypeWeight(relationType string) float32 {
	switch relationType {
	case "caused_by":
		return 1.2
	case "part_of":
		return 1.15
	case "reinforces":
		return 1.1
	case "temporal":
		return 1.1
	case "similar":
		return 1.0
	case "contradicts":
		return 0.8
	default:
		return 1.0 // default weight
	}
}

// spreadActivation traverses the association graph using spread activation algorithm.
// Returns a map of memory IDs to their activation state and the list of associations traversed.
func (ra *RetrievalAgent) spreadActivation(ctx context.Context, entryPoints map[string]float32) (map[string]activationState, []store.TraversedAssoc) {
	activated := make(map[string]activationState)
	var traversed []store.TraversedAssoc

	// Initialize with entry points
	frontier := make(map[string]float32)
	for memID, score := range entryPoints {
		frontier[memID] = score
		activated[memID] = activationState{activation: score, hopsReached: 0}
	}

	// Spread activation for MaxHops iterations
	for hop := 0; hop < ra.config.MaxHops && len(frontier) > 0; hop++ {
		nextFrontier := make(map[string]float32)

		for memID, currentActivation := range frontier {
			// Get associations for this memory
			assocs, err := ra.store.GetAssociations(ctx, memID)
			if err != nil {
				ra.log.Warn("failed to get associations for memory", "memory_id", memID, "error", err)
				continue
			}

			// Propagate activation along associations
			for _, assoc := range assocs {
				// Calculate propagated activation with decay and type-based weight
				decayFactor := float32(math.Pow(float64(ra.config.DecayFactor), float64(hop+1)))
				typeWeight := getAssociationTypeWeight(assoc.RelationType)
				propagated := currentActivation * assoc.Strength * decayFactor * typeWeight

				// Only propagate if above threshold
				if propagated > ra.config.ActivationThreshold {
					// Record that this association was traversed (Hebbian activation)
					if err := ra.store.ActivateAssociation(ctx, memID, assoc.TargetID); err != nil {
						ra.log.Warn("failed to activate association", "src", memID, "tgt", assoc.TargetID, "error", err)
					}

					// Track traversal for feedback loop
					traversed = append(traversed, store.TraversedAssoc{
						SourceID: memID,
						TargetID: assoc.TargetID,
					})

					// Keep maximum activation if memory was seen before
					existing := activationState{}
					if state, ok := activated[assoc.TargetID]; ok {
						existing = state
					}

					if propagated > existing.activation {
						activated[assoc.TargetID] = activationState{
							activation:      propagated,
							hopsReached:     hop + 1,
							activationCount: assoc.ActivationCount,
						}
						nextFrontier[assoc.TargetID] = propagated
					}
				}
			}
		}

		frontier = nextFrontier
	}

	return activated, traversed
}

// rankResults sorts activated memories by a combined score and prepares results.
func (ra *RetrievalAgent) rankResults(activated map[string]activationState, includeReasoning bool) []store.RetrievalResult {
	type scoredMemory struct {
		id            string
		activation    float32
		finalScore    float32
		recencyBonus  float32
		activityBonus float32
	}

	scored := make([]scoredMemory, 0, len(activated))

	for memID, state := range activated {
		mem, err := ra.store.GetMemory(context.Background(), memID)
		if err != nil {
			ra.log.Warn("failed to fetch memory for ranking", "memory_id", memID, "error", err)
			continue
		}

		// Calculate recency bonus — use CreatedAt for never-accessed memories
		var daysSinceAccess float32
		if mem.LastAccessed.IsZero() {
			daysSinceAccess = float32(time.Since(mem.CreatedAt).Hours() / 24)
		} else {
			daysSinceAccess = float32(time.Since(mem.LastAccessed).Hours() / 24)
		}
		recencyBonus := 0.2 * float32(math.Exp(float64(-daysSinceAccess/30)))

		// Hebbian activity bonus — frequently traversed associations indicate relevance
		activityBonus := float32(math.Min(0.2, 0.02*math.Log1p(float64(state.activationCount))))

		// Combined score
		finalScore := state.activation * (1.0 + recencyBonus + activityBonus)

		// Valence boost for significant memories
		attrs, attrErr := ra.store.GetMemoryAttributes(context.Background(), memID)
		if attrErr == nil {
			switch attrs.Significance {
			case "critical":
				finalScore *= 1.2
			case "important":
				finalScore *= 1.1
			}
		}

		scored = append(scored, scoredMemory{
			id:            memID,
			activation:    state.activation,
			finalScore:    finalScore,
			recencyBonus:  recencyBonus,
			activityBonus: activityBonus,
		})
	}

	// Sort by final score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].finalScore > scored[j].finalScore
	})

	// Build results
	results := make([]store.RetrievalResult, len(scored))
	for i, sm := range scored {
		mem, _ := ra.store.GetMemory(context.Background(), sm.id)

		explanation := ""
		if includeReasoning {
			explanation = fmt.Sprintf(
				"activation: %.3f, recency_bonus: %.3f, activity_bonus: %.3f, combined_score: %.3f",
				sm.activation, sm.recencyBonus, sm.activityBonus, sm.finalScore,
			)
		}

		results[i] = store.RetrievalResult{
			Memory:      mem,
			Score:       sm.finalScore,
			Explanation: explanation,
		}
	}

	return results
}

// synthesisTools returns the read-only tools available to the LLM during synthesis.
func (ra *RetrievalAgent) synthesisTools() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "search_memories",
				Description: "Search for additional memories by keyword or phrase. Use this when you want to explore a topic mentioned in the existing memories.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {"type": "string", "description": "The search query — a keyword, phrase, or concept to look for"}
					},
					"required": ["query"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "get_related",
				Description: "Follow connections from a specific memory to find related ones. Use this when a memory seems important and you want to see what it connects to.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"memory_id": {"type": "string", "description": "The ID of the memory to explore connections from"}
					},
					"required": ["memory_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "get_details",
				Description: "Get the full detail of a specific memory — its narrative, context, and original observations. Use this when a summary isn't enough.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"memory_id": {"type": "string", "description": "The ID of the memory to get full details for"}
					},
					"required": ["memory_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "search_timeline",
				Description: "Find memories from a specific time period. Use this when the question involves 'recently', 'last week', or a specific date range.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"from": {"type": "string", "description": "Start date in YYYY-MM-DD format"},
						"to": {"type": "string", "description": "End date in YYYY-MM-DD format"}
					},
					"required": ["from", "to"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "get_project_context",
				Description: "Get an overview of a project — what's been happening, key themes, and activity summary. Use this for project-level questions.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project": {"type": "string", "description": "The project name to get context for"}
					},
					"required": ["project"]
				}`),
			},
		},
	}
}

// executeTool dispatches a tool call to the appropriate read-only Store method and returns the result as a string.
func (ra *RetrievalAgent) executeTool(ctx context.Context, tc llm.ToolCall) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	switch tc.Function.Name {
	case "search_memories":
		query, _ := args["query"].(string)
		if query == "" {
			return "Error: query is required"
		}
		memories, err := ra.store.SearchByFullText(ctx, query, 5)
		if err != nil {
			return fmt.Sprintf("Search failed: %v", err)
		}
		if len(memories) == 0 {
			return "No memories found matching that query."
		}
		var sb strings.Builder
		for i, mem := range memories {
			project := ""
			if mem.Project != "" {
				project = fmt.Sprintf(" [%s]", mem.Project)
			}
			fmt.Fprintf(&sb, "%d. (id:%s)%s %s\n   Concepts: %s\n", i+1, mem.ID, project, mem.Summary, strings.Join(mem.Concepts, ", "))
		}
		return sb.String()

	case "get_related":
		memoryID, _ := args["memory_id"].(string)
		if memoryID == "" {
			return "Error: memory_id is required"
		}
		assocs, err := ra.store.GetAssociations(ctx, memoryID)
		if err != nil {
			return fmt.Sprintf("Failed to get associations: %v", err)
		}
		if len(assocs) == 0 {
			return "This memory has no connections to other memories."
		}
		var sb strings.Builder
		for i, assoc := range assocs {
			if i >= 5 {
				break
			}
			targetMem, err := ra.store.GetMemory(ctx, assoc.TargetID)
			if err != nil {
				continue
			}
			fmt.Fprintf(&sb, "- (id:%s) %s [%s, strength: %.2f]\n", targetMem.ID, targetMem.Summary, assoc.RelationType, assoc.Strength)
		}
		if sb.Len() == 0 {
			return "Connected memories could not be loaded."
		}
		return sb.String()

	case "get_details":
		memoryID, _ := args["memory_id"].(string)
		if memoryID == "" {
			return "Error: memory_id is required"
		}
		res, err := ra.store.GetMemoryResolution(ctx, memoryID)
		if err != nil {
			return fmt.Sprintf("Failed to get details: %v", err)
		}
		return fmt.Sprintf("Gist: %s\n\nFull narrative: %s", res.Gist, res.Narrative)

	case "search_timeline":
		fromStr, _ := args["from"].(string)
		toStr, _ := args["to"].(string)
		from, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return fmt.Sprintf("Error parsing 'from' date: %v", err)
		}
		to, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return fmt.Sprintf("Error parsing 'to' date: %v", err)
		}
		// Include the entire 'to' day
		to = to.Add(24*time.Hour - time.Second)
		memories, err := ra.store.ListMemoriesByTimeRange(ctx, from, to, 5)
		if err != nil {
			return fmt.Sprintf("Timeline search failed: %v", err)
		}
		if len(memories) == 0 {
			return "No memories found in that time range."
		}
		var sb strings.Builder
		for i, mem := range memories {
			fmt.Fprintf(&sb, "%d. (id:%s) [%s] %s\n", i+1, mem.ID, mem.CreatedAt.Format("2006-01-02 15:04"), mem.Summary)
		}
		return sb.String()

	case "get_project_context":
		project, _ := args["project"].(string)
		if project == "" {
			return "Error: project is required"
		}
		summary, err := ra.store.GetProjectSummary(ctx, project)
		if err != nil {
			return fmt.Sprintf("Failed to get project context: %v", err)
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		return string(data)

	default:
		return fmt.Sprintf("Unknown tool: %s", tc.Function.Name)
	}
}

// synthesizeNarrative uses the LLM to create a reasoned response from retrieved memories, patterns, and abstractions.
// The LLM has access to read-only tools to pull in additional context during synthesis.
func (ra *RetrievalAgent) synthesizeNarrative(ctx context.Context, query string, results []store.RetrievalResult, patterns []store.Pattern, abstractions []store.Abstraction) (string, error) {
	if len(results) == 0 && len(patterns) == 0 && len(abstractions) == 0 {
		return "No relevant memories found.", nil
	}

	// Build the initial prompt with pre-fetched context
	var prompt strings.Builder
	prompt.WriteString("Someone is searching their memory. Help them remember — not just the facts, but the meaning. Draw on everything below to give them a thoughtful, useful answer.\n\n")
	prompt.WriteString("You have tools available to search for more context if the memories below don't fully answer the question. Use them if something seems incomplete or if you want to follow a thread. When you have enough context, respond with your final synthesis.\n\n")
	fmt.Fprintf(&prompt, "They're asking: %s\n\n", query)

	// Memories section — include IDs so the LLM can reference them with tools
	if len(results) > 0 {
		prompt.WriteString("Specific memories:\n")
		for i, result := range results {
			mem := result.Memory
			project := ""
			if mem.Project != "" {
				project = fmt.Sprintf(" [%s]", mem.Project)
			}
			detail := mem.Content
			if detail == "" {
				detail = mem.Summary
			}
			fmt.Fprintf(&prompt, "%d. (id:%s)%s %s\n   Detail: %s\n   Concepts: %v | Created: %s\n",
				i+1, mem.ID, project, mem.Summary, detail, mem.Concepts, mem.CreatedAt.Format("2006-01-02 15:04"))
		}
		prompt.WriteString("\n")
	}

	// Patterns section
	if len(patterns) > 0 {
		prompt.WriteString("Patterns you've noticed over time:\n")
		for i, p := range patterns {
			project := ""
			if p.Project != "" {
				project = fmt.Sprintf(" [%s]", p.Project)
			}
			fmt.Fprintf(&prompt, "- %s%s: %s (strength: %.2f)\n", p.Title, project, p.Description, p.Strength)
			if i >= 2 {
				break
			}
		}
		prompt.WriteString("\n")
	}

	// Abstractions section
	if len(abstractions) > 0 {
		prompt.WriteString("Deeper principles you've learned:\n")
		for i, a := range abstractions {
			levelLabel := "principle"
			if a.Level == 3 {
				levelLabel = "axiom"
			}
			fmt.Fprintf(&prompt, "- [%s] %s: %s (confidence: %.2f)\n", levelLabel, a.Title, a.Description, a.Confidence)
			if i >= 2 {
				break
			}
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("Weave these together into a clear, informative response. Reference specific memories when they illuminate the answer. Include concrete details — file names, what changed, why it matters. If patterns or principles apply, share that wisdom. Be honest about what you're confident in and what's fuzzy. Aim for a thorough summary — a short paragraph per major theme or activity, not just a couple of sentences.")

	// Build conversation history for the tool-use loop
	messages := []llm.Message{
		{Role: "user", Content: prompt.String()},
	}
	tools := ra.synthesisTools()
	toolCallCount := 0

	for {
		req := llm.CompletionRequest{
			Messages:    messages,
			MaxTokens:   ra.config.SynthesisMaxTokens,
			Temperature: 0.5,
		}

		// Only provide tools if we haven't exhausted the budget
		if toolCallCount < ra.config.MaxToolCalls {
			req.Tools = tools
		}

		resp, err := ra.llm.Complete(ctx, req)
		if err != nil {
			// If tool-use fails (e.g. model doesn't support it), fall back to no-tools synthesis
			if toolCallCount == 0 {
				ra.log.Warn("tool-use synthesis failed, falling back to plain synthesis", "error", err)
				req.Tools = nil
				resp, err = ra.llm.Complete(ctx, req)
				if err != nil {
					return "", fmt.Errorf("llm synthesis failed: %w", err)
				}
				return strings.TrimSpace(resp.Content), nil
			}
			return "", fmt.Errorf("llm synthesis failed during tool loop: %w", err)
		}

		// If the model returned text (no tool calls), we're done
		if len(resp.ToolCalls) == 0 {
			return strings.TrimSpace(resp.Content), nil
		}

		// The model wants to use tools — append its message to the conversation
		assistantMsg := llm.Message{
			Role:      "assistant",
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute each tool call and append results
		for _, tc := range resp.ToolCalls {
			ra.log.Debug("executing synthesis tool", "tool", tc.Function.Name, "args", tc.Function.Arguments, "call_number", toolCallCount+1)

			result := ra.executeTool(ctx, tc)

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		toolCallCount++

		// If we've hit the budget, the next iteration will send without tools,
		// forcing the model to produce a text response
		if toolCallCount >= ra.config.MaxToolCalls {
			ra.log.Debug("tool call budget exhausted, forcing final synthesis")
		}
	}
}

// parseQueryConcepts extracts meaningful tokens from the query text.
// This is a simple v1 implementation that splits on spaces and filters common words.
func parseQueryConcepts(query string) []string {
	commonWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
		"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "can": true, "this": true,
		"that": true, "these": true, "those": true, "i": true, "you": true, "he": true,
		"she": true, "it": true, "we": true, "they": true, "what": true, "when": true,
		"where": true, "why": true, "how": true, "which": true, "who": true,
	}

	tokens := strings.Fields(strings.ToLower(query))
	concepts := []string{}

	for _, token := range tokens {
		// Clean punctuation
		token = strings.Trim(token, ".,!?;:\"'")

		// Filter out common words and short tokens
		if len(token) > 2 && !commonWords[token] {
			concepts = append(concepts, token)
		}
	}

	return concepts
}

// GetStats returns retrieval statistics.
func (ra *RetrievalAgent) GetStats() map[string]interface{} {
	ra.mu.RLock()
	defer ra.mu.RUnlock()

	avgMemoriesPerQuery := float64(0)
	if ra.stats.TotalQueries > 0 {
		avgMemoriesPerQuery = float64(ra.stats.TotalMemoriesHit) / float64(ra.stats.TotalQueries)
	}

	return map[string]interface{}{
		"total_queries":            ra.stats.TotalQueries,
		"total_memories_retrieved": ra.stats.TotalMemoriesHit,
		"avg_memories_per_query":   avgMemoriesPerQuery,
		"avg_synthesis_ms":         ra.stats.AvgSynthesisMs,
		"last_query_time":          ra.stats.LastQueryTime,
	}
}

// ResetStats clears all recorded statistics.
func (ra *RetrievalAgent) ResetStats() {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	ra.stats = &retrievalStats{
		TotalQueries: 0,
	}
}

// applyFilters filters results by project and/or time range.
func (ra *RetrievalAgent) applyFilters(results []store.RetrievalResult, req QueryRequest) []store.RetrievalResult {
	var filtered []store.RetrievalResult
	for _, r := range results {
		if req.Project != "" && r.Memory.Project != req.Project {
			continue
		}
		if !req.TimeFrom.IsZero() && r.Memory.Timestamp.Before(req.TimeFrom) {
			continue
		}
		if !req.TimeTo.IsZero() && r.Memory.Timestamp.After(req.TimeTo) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}
