package episoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/appsprout-dev/mnemonic/internal/agent/agentutil"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// EpisodingConfig holds configuration for the episoding agent.
type EpisodingConfig struct {
	EpisodeWindowSizeMin int           // fixed window size in minutes (default 10)
	MinEventsPerEpisode  int           // minimum events to form an episode (default 2)
	PollingInterval      time.Duration // how often to check for new events (default 10s)
	StartupLookback      time.Duration // how far back to look on startup (default 1h)
	DefaultSalience      float32       // fallback salience for synthesized episodes (default 0.5)
}

// DefaultEpisodingConfig returns sensible defaults.
func DefaultEpisodingConfig() EpisodingConfig {
	return EpisodingConfig{
		EpisodeWindowSizeMin: 10,
		MinEventsPerEpisode:  2,
		PollingInterval:      10 * time.Second,
	}
}

// EpisodingAgent clusters raw memories into temporal episodes.
type EpisodingAgent struct {
	store       store.Store
	llmProvider llm.Provider
	config      EpisodingConfig
	log         *slog.Logger
	bus         events.Bus
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopOnce    sync.Once

	// Track which raw memories have been assigned to episodes
	lastProcessedTime time.Time
	// Set of raw memory IDs already assigned to an episode (avoids duplicates)
	assignedRawIDs map[string]bool
	assignedMu     sync.Mutex
}

// NewEpisodingAgent creates a new episoding agent.
func NewEpisodingAgent(s store.Store, llmProvider llm.Provider, log *slog.Logger, cfg EpisodingConfig) *EpisodingAgent {
	lookback := cfg.StartupLookback
	if lookback <= 0 {
		lookback = 1 * time.Hour
	}
	return &EpisodingAgent{
		store:             s,
		llmProvider:       llmProvider,
		config:            cfg,
		log:               log,
		lastProcessedTime: time.Now().Add(-lookback),
		assignedRawIDs:    make(map[string]bool),
	}
}

func (ea *EpisodingAgent) Name() string {
	return "episoding"
}

// defaultSalience returns the configured default salience, falling back to 0.5.
func (ea *EpisodingAgent) defaultSalience() float32 {
	if ea.config.DefaultSalience > 0 {
		return ea.config.DefaultSalience
	}
	return 0.5
}

func (ea *EpisodingAgent) Start(ctx context.Context, bus events.Bus) error {
	ea.ctx, ea.cancel = context.WithCancel(ctx)
	ea.bus = bus

	// Hydrate in-memory state from existing episodes so we don't re-process
	// raw memories that already belong to episodes (prevents duplicates on restart).
	ea.hydrateFromExistingEpisodes(ctx)

	ea.wg.Add(1)
	go ea.pollingLoop()
	ea.log.Info("episoding agent started",
		"window_size_min", ea.config.EpisodeWindowSizeMin,
	)
	return nil
}

// hydrateFromExistingEpisodes loads recent episodes from the DB and marks their
// raw memory IDs as already assigned, so the polling loop won't create duplicates.
func (ea *EpisodingAgent) hydrateFromExistingEpisodes(ctx context.Context) {
	// Load all episodes (both open and closed) from the last lookback window
	episodes, err := ea.store.ListEpisodes(ctx, "", 200, 0)
	if err != nil {
		ea.log.Warn("failed to hydrate episode state on startup", "error", err)
		return
	}

	ea.assignedMu.Lock()
	defer ea.assignedMu.Unlock()

	var latestTime time.Time
	for _, ep := range episodes {
		for _, rawID := range ep.RawMemoryIDs {
			ea.assignedRawIDs[rawID] = true
		}
		if ep.EndTime.After(latestTime) {
			latestTime = ep.EndTime
		}
	}

	// Advance the cursor past already-episoded events
	if !latestTime.IsZero() {
		ea.lastProcessedTime = latestTime
		ea.log.Info("hydrated episoding state from DB",
			"episodes", len(episodes),
			"assigned_raw_ids", len(ea.assignedRawIDs),
			"cursor", latestTime.Format(time.RFC3339),
		)
	}
}

func (ea *EpisodingAgent) Stop() error {
	ea.stopOnce.Do(func() {
		ea.cancel()
		ea.wg.Wait()
		ea.log.Info("episoding agent stopped")
	})
	return nil
}

func (ea *EpisodingAgent) Health(ctx context.Context) error {
	return nil
}

// ProcessAllPending processes all raw memories into episodes synchronously.
// This is intended for batch/benchmark usage where the polling loop is not running.
// It processes episodes, then forces closure of any remaining open episode.
func (ea *EpisodingAgent) ProcessAllPending(ctx context.Context) error {
	// Reset cursor to epoch so we pick up all raw memories.
	ea.lastProcessedTime = time.Time{}
	ea.assignedMu.Lock()
	ea.assignedRawIDs = make(map[string]bool)
	ea.assignedMu.Unlock()

	if err := ea.processEpisodes(ctx); err != nil {
		return fmt.Errorf("processing episodes: %w", err)
	}

	// Force-close any remaining open episode.
	openEp, err := ea.store.GetOpenEpisode(ctx)
	if err == nil && openEp.ID != "" {
		if err := ea.closeEpisode(ctx, &openEp); err != nil {
			return fmt.Errorf("closing open episode: %w", err)
		}
	}
	return nil
}

func (ea *EpisodingAgent) pollingLoop() {
	defer ea.wg.Done()

	ticker := time.NewTicker(ea.config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ea.ctx.Done():
			return
		case <-ticker.C:
			if err := ea.processEpisodes(ea.ctx); err != nil {
				ea.log.Error("episode processing failed", "error", err)
			}
		}
	}
}

// processEpisodes checks for raw memories that need episode assignment and manages episode lifecycle.
func (ea *EpisodingAgent) processEpisodes(ctx context.Context) error {
	// Use time-based cursor to find ALL raw memories since our last check,
	// regardless of the encoding agent's "processed" flag.
	allRaw, err := ea.store.ListRawMemoriesAfter(ctx, ea.lastProcessedTime, 200)
	if err != nil {
		return fmt.Errorf("failed to list raw memories: %w", err)
	}

	// Filter out raw memories we've already assigned to an episode
	ea.assignedMu.Lock()
	var newRaw []store.RawMemory
	for _, raw := range allRaw {
		if !ea.assignedRawIDs[raw.ID] {
			newRaw = append(newRaw, raw)
		}
	}
	ea.assignedMu.Unlock()

	if len(newRaw) == 0 {
		// No new raw memories — check if open episode should be closed due to idle
		return ea.checkIdleEpisode(ctx)
	}

	ea.log.Debug("episoding found new raw memories", "count", len(newRaw))

	// Get current open episode
	openEp, err := ea.store.GetOpenEpisode(ctx)
	hasOpenEpisode := err == nil && openEp.ID != ""

	windowSize := time.Duration(ea.config.EpisodeWindowSizeMin) * time.Minute

	for _, raw := range newRaw {
		if hasOpenEpisode {
			// Check if raw memory timestamp exceeds the fixed window
			if raw.Timestamp.Sub(openEp.StartTime) > windowSize {
				// Window exceeded — close current episode and open new one
				if err := ea.closeEpisode(ctx, &openEp); err != nil {
					ea.log.Error("failed to close episode", "id", openEp.ID, "error", err)
				}
				hasOpenEpisode = false
			}
		}

		if !hasOpenEpisode {
			// Create new episode
			openEp = store.Episode{
				ID:           uuid.New().String(),
				StartTime:    raw.Timestamp,
				EndTime:      raw.Timestamp,
				RawMemoryIDs: []string{},
				MemoryIDs:    []string{},
				State:        store.EpisodeStateOpen,
				Project:      raw.Project,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			if err := ea.store.CreateEpisode(ctx, openEp); err != nil {
				return fmt.Errorf("failed to create episode: %w", err)
			}
			hasOpenEpisode = true
			ea.log.Info("opened new episode", "id", openEp.ID, "start", raw.Timestamp)
		}

		// Add raw memory to episode
		openEp.RawMemoryIDs = append(openEp.RawMemoryIDs, raw.ID)
		if openEp.Project == "" && raw.Project != "" {
			openEp.Project = raw.Project
		}
		openEp.EndTime = raw.Timestamp
		openEp.DurationSec = int(openEp.EndTime.Sub(openEp.StartTime).Seconds())
		openEp.UpdatedAt = time.Now()

		if err := ea.store.UpdateEpisode(ctx, openEp); err != nil {
			ea.log.Error("failed to update episode", "id", openEp.ID, "error", err)
		}

		// Track this raw memory as assigned
		ea.assignedMu.Lock()
		ea.assignedRawIDs[raw.ID] = true
		ea.assignedMu.Unlock()

		// Advance the cursor to this raw memory's timestamp
		if raw.Timestamp.After(ea.lastProcessedTime) {
			ea.lastProcessedTime = raw.Timestamp
		}
	}

	// Check if open episode should be closed due to idle
	return ea.checkIdleEpisode(ctx)
}

// checkIdleEpisode closes the open episode if it exceeds the fixed window.
func (ea *EpisodingAgent) checkIdleEpisode(ctx context.Context) error {
	openEp, err := ea.store.GetOpenEpisode(ctx)
	if err != nil {
		return nil // no open episode
	}

	windowSize := time.Duration(ea.config.EpisodeWindowSizeMin) * time.Minute
	if time.Since(openEp.StartTime) > windowSize {
		if len(openEp.RawMemoryIDs) >= ea.config.MinEventsPerEpisode {
			return ea.closeEpisode(ctx, &openEp)
		}
		// Too few events — just close without synthesis
		if err := ea.store.CloseEpisode(ctx, openEp.ID); err != nil {
			ea.log.Warn("failed to close sparse episode", "id", openEp.ID, "error", err)
		}
		ea.log.Info("closed sparse episode without synthesis",
			"id", openEp.ID,
			"events", len(openEp.RawMemoryIDs),
		)
	}
	return nil
}

// closeEpisode synthesizes an episode using the LLM and closes it.
func (ea *EpisodingAgent) closeEpisode(ctx context.Context, ep *store.Episode) error {
	ea.log.Info("closing episode", "id", ep.ID, "events", len(ep.RawMemoryIDs))

	// Gather all raw events for synthesis and build enrichment data
	var eventTexts []string
	var timeline []store.EventEntry
	filesModified := make(map[string]bool)

	for _, rawID := range ep.RawMemoryIDs {
		raw, err := ea.store.GetRaw(ctx, rawID)
		if err != nil {
			continue
		}

		// Extract file path from metadata if available
		filePath := ""
		if path, ok := raw.Metadata["path"]; ok {
			if pathStr, ok := path.(string); ok {
				filePath = pathStr
				filesModified[filePath] = true
			}
		}

		// Include file path in event text sent to LLM
		text := fmt.Sprintf("[%s] [%s] [%s] [%s]: %s",
			raw.Timestamp.Format("15:04:05"),
			raw.Source,
			raw.Type,
			filePath,
			agentutil.Truncate(raw.Content, 2000),
		)
		eventTexts = append(eventTexts, text)

		// Build timeline entry
		brief := agentutil.Truncate(raw.Content, 80)
		entry := store.EventEntry{
			RawMemoryID: raw.ID,
			Timestamp:   raw.Timestamp,
			Source:      raw.Source,
			Type:        raw.Type,
			Brief:       brief,
			FilePath:    filePath,
		}
		timeline = append(timeline, entry)
	}

	// Deduplicate and populate files modified
	for file := range filesModified {
		ep.FilesModified = append(ep.FilesModified, file)
	}

	// Set event timeline
	ep.EventTimeline = timeline

	if len(eventTexts) == 0 {
		if err := ea.store.CloseEpisode(ctx, ep.ID); err != nil {
			ea.log.Warn("failed to close empty episode", "id", ep.ID, "error", err)
		}
		return nil
	}

	// Build LLM prompt for episode synthesis
	eventsStr := ""
	for _, t := range eventTexts {
		eventsStr += t + "\n\n"
	}

	// Detect if episode contains MCP-source events (Claude Code interaction)
	hasMCPEvents := false
	for _, rawID := range ep.RawMemoryIDs {
		raw, err := ea.store.GetRaw(ctx, rawID)
		if err != nil {
			continue
		}
		if raw.Source == "mcp" {
			hasMCPEvents = true
			break
		}
	}

	var prompt string
	if hasMCPEvents {
		// Claude-aware prompt: emphasize the collaborative creative journey
		prompt = fmt.Sprintf(`You're looking at a chapter from a creative collaboration — a human and AI building something together. What's the story of this session?

Look for the arc: What problem were they trying to solve? What did they decide to do? Did they hit any walls, and how did they get past them? What did they actually create or change? What's the most interesting thing that happened?

Events:
%s

Respond with ONLY a JSON object (no prose, no fences):
{"title":"a vivid, specific title for this session","summary":"1-2 sentences — the outcome and why it matters","narrative":"the story of what unfolded — decisions, breakthroughs, struggles, and what was learned","emotional_tone":"neutral|satisfying|frustrating|exciting|concerning","outcome":"success|failure|ongoing|unknown","concepts":["keyword1","keyword2"],"salience":0.7}`, eventsStr)
	} else {
		prompt = fmt.Sprintf(`You're looking at a stream of activity — moments from someone's work. What's the thread that connects them? What was this person doing, and what's worth remembering about it?

Events:
%s

Respond with ONLY a JSON object (no prose, no fences):
{"title":"a clear, specific title","summary":"1-2 sentences capturing what happened","narrative":"the story — what was this person working on, what did they accomplish, what's interesting about it","emotional_tone":"neutral|satisfying|frustrating|exciting|concerning","outcome":"success|failure|ongoing|unknown","concepts":["keyword1","keyword2"],"salience":0.5}`, eventsStr)
	}

	resp, err := ea.llmProvider.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are an episode synthesizer. Summarize groups of events into coherent episodes. Output JSON only."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
		ResponseFormat: &llm.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &llm.JSONSchema{
				Name:   "episode_synthesis",
				Strict: true,
				Schema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"summary":{"type":"string"},"narrative":{"type":"string"},"emotional_tone":{"type":"string"},"outcome":{"type":"string"},"concepts":{"type":"array","items":{"type":"string"}},"salience":{"type":"number"}},"required":["title","summary","narrative","emotional_tone","outcome","concepts","salience"],"additionalProperties":false}`),
			},
		},
	})

	if err != nil {
		ea.log.Warn("LLM episode synthesis failed, using fallback", "error", err)
		ep.Title = fmt.Sprintf("Session with %d events", len(ep.RawMemoryIDs))
		ep.Summary = ep.Title
		ep.Salience = ea.defaultSalience()
		ep.Concepts = []string{}
	} else {
		// Parse LLM response
		parsed := parseEpisodeSynthesis(resp.Content)
		ep.Title = parsed.Title
		ep.Summary = parsed.Summary
		ep.Narrative = parsed.Narrative
		ep.EmotionalTone = parsed.EmotionalTone
		ep.Outcome = parsed.Outcome
		ep.Concepts = parsed.Concepts
		ep.Salience = parsed.Salience
	}

	ep.State = store.EpisodeStateClosed
	ep.UpdatedAt = time.Now()

	if err := ea.store.UpdateEpisode(ctx, *ep); err != nil {
		return fmt.Errorf("failed to update closed episode: %w", err)
	}

	if err := ea.store.CloseEpisode(ctx, ep.ID); err != nil {
		ea.log.Warn("failed to close episode", "id", ep.ID, "error", err)
	}

	// Publish event
	if ea.bus != nil {
		_ = ea.bus.Publish(ctx, events.EpisodeClosed{
			EpisodeID:   ep.ID,
			Title:       ep.Title,
			EventCount:  len(ep.RawMemoryIDs),
			DurationSec: ep.DurationSec,
			Ts:          time.Now(),
		})
	}

	ea.log.Info("episode closed",
		"id", ep.ID,
		"title", ep.Title,
		"events", len(ep.RawMemoryIDs),
		"tone", ep.EmotionalTone,
		"concepts", len(ep.Concepts),
		"files", len(ep.FilesModified),
		"claude_session", hasMCPEvents,
	)
	return nil
}

// episodeSynthesis is the LLM response structure.
type episodeSynthesis struct {
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	Narrative     string   `json:"narrative"`
	EmotionalTone string   `json:"emotional_tone"`
	Outcome       string   `json:"outcome"`
	Concepts      []string `json:"concepts"`
	Salience      float32  `json:"salience"`
}

// parseEpisodeSynthesis extracts JSON from LLM response.
func parseEpisodeSynthesis(response string) episodeSynthesis {
	var result episodeSynthesis
	jsonStr := agentutil.ExtractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return episodeSynthesis{
			Title:         "Untitled session",
			Summary:       "Episode synthesis failed — LLM returned unparseable response.",
			Salience:      0.5,
			EmotionalTone: "neutral",
			Outcome:       "ongoing",
			Concepts:      []string{},
		}
	}
	// Guard against the LLM returning code or garbage in fields
	if len(result.Summary) > 500 {
		result.Summary = result.Summary[:500] + "..."
	}
	if len(result.Narrative) > 2000 {
		result.Narrative = result.Narrative[:2000] + "..."
	}
	// Validate fields
	if result.Title == "" {
		result.Title = "Untitled session"
	}
	if result.Salience <= 0 {
		result.Salience = 0.5
	}
	if result.EmotionalTone == "" {
		result.EmotionalTone = "neutral"
	}
	if result.Outcome == "" {
		result.Outcome = "ongoing"
	}
	if result.Concepts == nil {
		result.Concepts = []string{}
	}
	return result
}
