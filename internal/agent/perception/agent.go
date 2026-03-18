package perception

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/watcher"
	"github.com/google/uuid"
)

// PerceptionConfig configures the perception agent.
// ProjectResolver resolves file paths and names to canonical project names.
type ProjectResolver interface {
	Resolve(input string) string
}

type PerceptionConfig struct {
	HeuristicConfig       HeuristicConfig
	LLMGatingEnabled      bool            // if false, skip LLM and use heuristic score as salience
	LearnedExclusionsPath string          // file path for persisting learned watcher exclusions
	ProjectResolver       ProjectResolver // optional: maps paths to canonical project names
}

// recentContentTTL is how long to remember content hashes for dedup.
// This prevents duplicate raw memories when the filesystem watcher emits
// multiple events for the same file content within a short window
// (e.g., atomic saves producing Created+Renamed then Modified events).
const recentContentTTL = 5 * time.Second

// PerceptionAgent orchestrates the perception pipeline: watchers → heuristic → LLM → memory.
type PerceptionAgent struct {
	name             string
	watchers         []watcher.Watcher
	store            store.Store
	llmProvider      llm.Provider
	cfg              PerceptionConfig
	log              *slog.Logger
	heuristicFilter  *HeuristicFilter
	rejectionTracker *rejectionTracker
	bus              events.Bus
	mu               sync.RWMutex
	running          bool
	cancelFunc       context.CancelFunc
	processingWg     sync.WaitGroup
	watcherStopChans []chan struct{} // one per watcher goroutine

	// Content-hash dedup: prevents creating duplicate raw memories when the
	// watcher emits multiple events for the same file with identical content.
	recentContentMu sync.Mutex
	recentContent   map[string]time.Time // hash(source+path+content) → timestamp

	// Git-operation suppression: prevents encoding filesystem events that are
	// side-effects of git operations (pull, checkout, merge, rebase).
	gitOpCooldown time.Duration // how long after a git op to suppress fs events (default: 10s)
}

// NewPerceptionAgent creates a new perception agent with the given dependencies.
func NewPerceptionAgent(
	watchers []watcher.Watcher,
	s store.Store,
	llmProv llm.Provider,
	cfg PerceptionConfig,
	log *slog.Logger,
) *PerceptionAgent {
	return &PerceptionAgent{
		name:          "perception",
		watchers:      watchers,
		store:         s,
		llmProvider:   llmProv,
		cfg:           cfg,
		log:           log,
		gitOpCooldown: 10 * time.Second,
	}
}

// Name returns the agent's name.
func (pa *PerceptionAgent) Name() string {
	return pa.name
}

// Start initializes and starts the perception agent.
func (pa *PerceptionAgent) Start(ctx context.Context, bus events.Bus) error {
	pa.mu.Lock()
	if pa.running {
		pa.mu.Unlock()
		return fmt.Errorf("perception agent already running")
	}

	// Create a context with cancellation for this agent's lifetime
	ctx, cancelFunc := context.WithCancel(ctx)
	pa.cancelFunc = cancelFunc
	pa.bus = bus
	pa.running = true
	pa.recentContent = make(map[string]time.Time)
	pa.heuristicFilter = NewHeuristicFilter(pa.cfg.HeuristicConfig, pa.log)
	pa.rejectionTracker = newRejectionTracker(
		rejectionTrackerConfig{
			PersistPath: pa.cfg.LearnedExclusionsPath,
		},
		pa.log,
		pa.promoteExclusion,
	)
	pa.watcherStopChans = make([]chan struct{}, len(pa.watchers))
	pa.mu.Unlock()

	pa.log.Info("starting perception agent", "watcher_count", len(pa.watchers))

	// Start all watchers
	for i, w := range pa.watchers {
		if err := w.Start(ctx); err != nil {
			pa.log.Error("failed to start watcher", "index", i, "error", err)
			return fmt.Errorf("watcher %d failed to start: %w", i, err)
		}
	}

	// Apply any previously learned exclusions to watchers
	if pa.rejectionTracker != nil {
		for _, pattern := range pa.rejectionTracker.learnedExclusions() {
			pa.promoteExclusion(pattern)
		}
	}

	// Launch cleanup goroutine for content-hash dedup map
	pa.processingWg.Add(1)
	go pa.cleanupRecentContent(ctx)

	// Launch a processing goroutine for each watcher
	for i, w := range pa.watchers {
		stopChan := make(chan struct{})
		pa.watcherStopChans[i] = stopChan

		pa.processingWg.Add(1)
		go pa.processWatcherEvents(ctx, w, stopChan, i)
	}

	pa.log.Info("perception agent started")
	return nil
}

// Stop cleanly shuts down the perception agent.
func (pa *PerceptionAgent) Stop() error {
	pa.mu.Lock()
	if !pa.running {
		pa.mu.Unlock()
		return fmt.Errorf("perception agent not running")
	}
	pa.running = false
	pa.mu.Unlock()

	pa.log.Info("stopping perception agent")

	// Signal all processing goroutines to stop
	for _, stopChan := range pa.watcherStopChans {
		select {
		case stopChan <- struct{}{}:
		default:
		}
	}

	// Cancel the agent's context to signal watchers
	if pa.cancelFunc != nil {
		pa.cancelFunc()
	}

	// Stop all watchers
	for i, w := range pa.watchers {
		if err := w.Stop(); err != nil {
			pa.log.Error("failed to stop watcher", "index", i, "error", err)
		}
	}

	// Wait for all processing goroutines to finish
	pa.processingWg.Wait()

	pa.log.Info("perception agent stopped")
	return nil
}

// Health checks the health of the perception agent.
func (pa *PerceptionAgent) Health(ctx context.Context) error {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	if !pa.running {
		return fmt.Errorf("perception agent not running")
	}
	return nil
}

// processWatcherEvents reads events from a watcher and processes them through the pipeline.
func (pa *PerceptionAgent) processWatcherEvents(
	ctx context.Context,
	w watcher.Watcher,
	stopChan chan struct{},
	watcherIndex int,
) {
	defer pa.processingWg.Done()

	eventsChan := w.Events()
	watcherName := w.Name()

	for {
		select {
		case <-ctx.Done():
			pa.log.Debug("watcher processing context cancelled", "watcher", watcherName)
			return

		case <-stopChan:
			pa.log.Debug("watcher processing stopped", "watcher", watcherName)
			return

		case event, ok := <-eventsChan:
			if !ok {
				pa.log.Debug("watcher events channel closed", "watcher", watcherName)
				return
			}

			// Convert watcher.Event to our Event type
			evt := Event{
				ID:        event.ID,
				Source:    event.Source,
				Type:      event.Type,
				Path:      event.Path,
				Content:   event.Content,
				Timestamp: event.Timestamp,
				Metadata:  event.Metadata,
			}

			// Process the event through the pipeline
			pa.processEvent(ctx, evt)
		}
	}
}

// contentHash returns a dedup key for a filesystem event based on source, path, and content.
func contentHash(source, path, content string) string {
	h := sha256.New()
	h.Write([]byte(source))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil)[:16]) // 128 bits is plenty for dedup
}

// cleanupRecentContent periodically evicts expired entries from the dedup map.
func (pa *PerceptionAgent) cleanupRecentContent(ctx context.Context) {
	defer pa.processingWg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			pa.recentContentMu.Lock()
			for k, ts := range pa.recentContent {
				if now.Sub(ts) > recentContentTTL {
					delete(pa.recentContent, k)
				}
			}
			pa.recentContentMu.Unlock()
		}
	}
}

// processEvent processes a single event through the perception pipeline.
func (pa *PerceptionAgent) processEvent(ctx context.Context, event Event) {
	// 0. Content-hash dedup: skip if we recently created a raw memory with
	// identical source+path+content. This catches duplicate filesystem events
	// that slip through the watcher's debounce (e.g., atomic saves producing
	// multiple FSEvents batches).
	if event.Source == "filesystem" && event.Content != "" {
		hash := contentHash(event.Source, event.Path, event.Content)
		pa.recentContentMu.Lock()
		if _, exists := pa.recentContent[hash]; exists {
			pa.recentContentMu.Unlock()
			pa.log.Debug("dedup: skipping duplicate filesystem event",
				"path", event.Path, "type", event.Type)
			return
		}
		pa.recentContent[hash] = time.Now()
		pa.recentContentMu.Unlock()
	}

	// 0b. Git-operation suppression: if this filesystem event is inside a git repo
	// that had a recent git operation (pull, checkout, merge, rebase), suppress it.
	// The git watcher will handle it as a single "repo_changed" event instead.
	if event.Source == "filesystem" && pa.isRecentGitOp(event.Path) {
		pa.log.Debug("suppressed filesystem event during git operation",
			"path", event.Path, "type", event.Type)
		return
	}

	// 1. Run heuristic filter
	heuristicResult := pa.heuristicFilter.Evaluate(event)
	if !heuristicResult.Pass {
		pa.log.Info(
			"event failed heuristic filter",
			"source", event.Source,
			"path", event.Path,
			"rationale", heuristicResult.Rationale,
		)
		// Track filesystem rejections for adaptive exclusion learning
		if event.Source == "filesystem" && event.Path != "" && pa.rejectionTracker != nil {
			pa.rejectionTracker.recordRejection(event.Path)
		}
		return
	}

	salience := heuristicResult.Score

	// 2. LLM gating (if enabled)
	if pa.cfg.LLMGatingEnabled {
		llmResult, err := pa.callLLMGate(ctx, event, heuristicResult.Score)
		if err != nil {
			pa.log.Error(
				"LLM gating failed, falling back to heuristic",
				"error", err,
				"source", event.Source,
			)
			// Fall back to heuristic score
			salience = heuristicResult.Score
		} else if !llmResult.WorthRemembering {
			pa.log.Info(
				"event rejected by LLM gating",
				"source", event.Source,
				"path", event.Path,
				"reason", llmResult.Reason,
			)
			return
		} else {
			salience = llmResult.Salience
		}
	}

	// 3. Create a raw memory entry
	rawMemory := store.RawMemory{
		ID:              uuid.New().String(),
		Source:          event.Source,
		Type:            event.Type,
		Content:         pa.truncateContent(event.Content, 10000),
		Timestamp:       event.Timestamp,
		CreatedAt:       time.Now(),
		Metadata:        pa.mergeMetadata(event.Metadata, event.Path, heuristicResult.Score),
		HeuristicScore:  heuristicResult.Score,
		InitialSalience: salience,
		Processed:       false,
		Project:         pa.resolveProject(event.Path),
	}

	// 4. Write to store
	if err := pa.store.WriteRaw(ctx, rawMemory); err != nil {
		pa.log.Error(
			"failed to write raw memory",
			"error", err,
			"source", event.Source,
		)
		return
	}

	// 5. Publish event
	if err := pa.bus.Publish(ctx, events.RawMemoryCreated{
		ID:             rawMemory.ID,
		Source:         rawMemory.Source,
		HeuristicScore: heuristicResult.Score,
		Salience:       salience,
		Ts:             time.Now(),
	}); err != nil {
		pa.log.Error(
			"failed to publish RawMemoryCreated event",
			"error", err,
			"memory_id", rawMemory.ID,
		)
		// Don't return; the memory was already written to store
	}

	// 6. Log at info level
	pa.log.Info(
		"perceived new memory",
		"memory_id", rawMemory.ID,
		"source", rawMemory.Source,
		"type", rawMemory.Type,
		"salience", salience,
		"heuristic_score", heuristicResult.Score,
	)
}

// callLLMGate calls the LLM to determine if an event is worth remembering.
func (pa *PerceptionAgent) callLLMGate(
	ctx context.Context,
	event Event,
	heuristicScore float32,
) (*llmGateResult, error) {
	// Build the prompt
	contentSnippet := event.Content
	if len(contentSnippet) > 500 {
		contentSnippet = contentSnippet[:500]
	}

	prompt := fmt.Sprintf(`You are a memory perception system. Evaluate if this observation is worth remembering.

Source: %s
Type: %s
Content: %s

Respond in exactly this JSON format, nothing else:
{"worth_remembering": true, "salience": 0.7, "reason": "brief explanation"}

Salience 0.0-1.0: Higher for errors, decisions, insights, creative work. Lower for routine navigation, trivial commands.`,
		event.Source,
		event.Type,
		contentSnippet,
	)

	// Create LLM request with structured output to ensure valid JSON
	req := llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are a relevance filter. Decide if events are worth remembering. Output JSON only."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   200,
		Temperature: 0.5,
		ResponseFormat: &llm.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &llm.JSONSchema{
				Name:   "gate_response",
				Strict: true,
				Schema: json.RawMessage(`{"type":"object","properties":{"worth_remembering":{"type":"boolean"},"salience":{"type":"number"},"reason":{"type":"string"}},"required":["worth_remembering","salience","reason"],"additionalProperties":false}`),
			},
		},
	}

	// Call LLM with context timeout
	llmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := pa.llmProvider.Complete(llmCtx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	// Parse the JSON response
	var result llmGateResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		pa.log.Error(
			"failed to parse LLM response",
			"error", err,
			"response", resp.Content,
		)
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	// Clamp salience to [0.0, 1.0]
	if result.Salience < 0.0 {
		result.Salience = 0.0
	} else if result.Salience > 1.0 {
		result.Salience = 1.0
	}

	return &result, nil
}

// llmGateResult represents the LLM's decision on whether to remember an event.
type llmGateResult struct {
	WorthRemembering bool    `json:"worth_remembering"`
	Salience         float32 `json:"salience"`
	Reason           string  `json:"reason"`
}

// promoteExclusion pushes a learned exclusion pattern to all watchers that
// support runtime exclusion updates.
func (pa *PerceptionAgent) promoteExclusion(pattern string) {
	for _, w := range pa.watchers {
		if ew, ok := w.(watcher.ExcludableWatcher); ok {
			ew.AddExclusion(pattern)
			pa.log.Info("promoted learned exclusion to watcher",
				"pattern", pattern,
				"watcher", w.Name(),
			)
		}
	}
}

// truncateContent truncates content to a maximum length.
func (pa *PerceptionAgent) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}

// mergeMetadata merges event metadata with additional fields.
func (pa *PerceptionAgent) mergeMetadata(
	eventMetadata map[string]interface{},
	path string,
	heuristicScore float32,
) map[string]interface{} {
	if eventMetadata == nil {
		eventMetadata = make(map[string]interface{})
	}

	merged := make(map[string]interface{})
	for k, v := range eventMetadata {
		merged[k] = v
	}

	if path != "" {
		merged["path"] = path
	}
	merged["heuristic_score"] = heuristicScore

	return merged
}

// isRecentGitOp checks if the given file path is inside a git repo that had
// a recent git operation (pull, checkout, merge, rebase). It does this by
// walking up the directory tree to find .git/, then checking if sentinel files
// (.git/FETCH_HEAD, .git/HEAD, .git/ORIG_HEAD) were modified recently.
//
// This is lightweight (a few stat calls) and avoids encoding filesystem events
// that are side-effects of git operations — the git watcher handles those as
// a single "repo_changed" event instead.
func (pa *PerceptionAgent) isRecentGitOp(filePath string) bool {
	gitDir := findGitDir(filePath)
	if gitDir == "" {
		return false
	}

	cutoff := time.Now().Add(-pa.gitOpCooldown)

	// Check sentinel files that git updates during operations.
	// FETCH_HEAD: updated by git fetch/pull
	// ORIG_HEAD: updated by git merge/rebase/reset
	// HEAD: updated by git checkout/switch
	sentinels := []string{"FETCH_HEAD", "ORIG_HEAD", "HEAD"}
	for _, s := range sentinels {
		info, err := os.Stat(filepath.Join(gitDir, s))
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			return true
		}
	}
	return false
}

// findGitDir walks up from the given path to find the nearest .git directory.
// Returns the .git directory path, or empty string if not found.
func findGitDir(path string) string {
	dir := filepath.Dir(path)
	for {
		candidate := filepath.Join(dir, ".git")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached filesystem root
		}
		dir = parent
	}
}

// knownProjectParents are directory names that typically contain project directories.
var knownProjectParents = map[string]bool{
	"Projects":  true,
	"projects":  true,
	"src":       true,
	"repos":     true,
	"workspace": true,
	"Workspace": true,
}

// inferProjectFromPath extracts a project name from a file path by looking for
// known project parent directories (e.g., ~/Projects/felixlm/foo.go → "felixlm").
func inferProjectFromPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand ~ if present
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}

	parts := strings.Split(filepath.Clean(path), string(os.PathSeparator))
	for i, part := range parts {
		if knownProjectParents[part] && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// resolveProject uses the configured ProjectResolver if available, falling back to inferProjectFromPath.
func (pa *PerceptionAgent) resolveProject(path string) string {
	if pa.cfg.ProjectResolver != nil {
		return pa.cfg.ProjectResolver.Resolve(path)
	}
	return inferProjectFromPath(path)
}

// Ensure PerceptionAgent implements agent.Agent interface.
var _ agent.Agent = (*PerceptionAgent)(nil)
