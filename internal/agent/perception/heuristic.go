package perception

import (
	"crypto/md5"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// HeuristicConfig defines the configuration for the heuristic pre-filter.
type HeuristicConfig struct {
	MinContentLength   int // minimum content length to pass
	MaxContentLength   int // maximum content length to pass
	FrequencyThreshold int // skip if seen >N times in window
	FrequencyWindowMin int // window size in minutes
}

// HeuristicResult represents the outcome of a heuristic evaluation.
type HeuristicResult struct {
	Pass      bool    // whether the event passes the heuristic filter
	Score     float32 // confidence score from 0.0 to 1.0
	Rationale string  // explanation of the result
}

// frequencyEntry tracks a content hash with its timestamp.
type frequencyEntry struct {
	hash      string
	timestamp time.Time
}

// HeuristicFilter implements the pre-filter logic for watcher events.
type HeuristicFilter struct {
	cfg       HeuristicConfig
	log       *slog.Logger
	mu        sync.RWMutex
	frequency map[string][]frequencyEntry // hash -> list of timestamps

	// Batch edit detection: track rapid sequential file edits
	recentEdits []recentEdit
	editMu      sync.Mutex

	// Recall-aware salience: files recently recalled via MCP get a boost
	recalledFiles map[string]time.Time // path -> last recall time
	recallMu      sync.RWMutex
}

// recentEdit tracks a file edit for batch detection.
type recentEdit struct {
	path      string
	timestamp time.Time
}

// NewHeuristicFilter creates a new HeuristicFilter with the given configuration.
func NewHeuristicFilter(cfg HeuristicConfig, log *slog.Logger) *HeuristicFilter {
	hf := &HeuristicFilter{
		cfg:           cfg,
		log:           log,
		frequency:     make(map[string][]frequencyEntry),
		recalledFiles: make(map[string]time.Time),
	}

	// Start a cleanup goroutine to periodically remove old entries
	go hf.cleanupLoop()

	return hf
}

// cleanupLoop periodically removes frequency entries older than the window.
func (h *HeuristicFilter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.cleanup()
	}
}

// cleanup removes frequency entries older than the frequency window.
func (h *HeuristicFilter) cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	windowDuration := time.Duration(h.cfg.FrequencyWindowMin) * time.Minute
	cutoffTime := time.Now().Add(-windowDuration)

	for hash, entries := range h.frequency {
		// Keep only recent entries
		var recentEntries []frequencyEntry
		for _, entry := range entries {
			if entry.timestamp.After(cutoffTime) {
				recentEntries = append(recentEntries, entry)
			}
		}

		if len(recentEntries) == 0 {
			delete(h.frequency, hash)
		} else {
			h.frequency[hash] = recentEntries
		}
	}
}

// Evaluate evaluates a watcher event against the heuristic filter.
func (h *HeuristicFilter) Evaluate(event Event) HeuristicResult {
	// 1. Content length check
	if len(event.Content) < h.cfg.MinContentLength {
		return HeuristicResult{
			Pass:      false,
			Score:     0.0,
			Rationale: fmt.Sprintf("content length %d below minimum %d", len(event.Content), h.cfg.MinContentLength),
		}
	}
	if len(event.Content) > h.cfg.MaxContentLength {
		return HeuristicResult{
			Pass:      false,
			Score:     0.0,
			Rationale: fmt.Sprintf("content length %d exceeds maximum %d", len(event.Content), h.cfg.MaxContentLength),
		}
	}

	// 2. Empty content check
	if strings.TrimSpace(event.Content) == "" {
		return HeuristicResult{
			Pass:      false,
			Score:     0.0,
			Rationale: "content is empty or whitespace only",
		}
	}

	// 3. Frequency check (deduplication)
	contentHash := h.hashContent(event.Content)
	if !h.checkAndRecordFrequency(contentHash) {
		return HeuristicResult{
			Pass:      false,
			Score:     0.0,
			Rationale: fmt.Sprintf("content seen more than %d times in last %d minutes", h.cfg.FrequencyThreshold, h.cfg.FrequencyWindowMin),
		}
	}

	// 4. Source-specific heuristics and base score
	score, sourceRationale, hardReject := h.evaluateSource(event.Source, event.Type, event.Path, event.Content)

	// Hard rejection: source-level filter says this event should never be remembered,
	// regardless of keyword content. Do not allow keyword scoring to override.
	if hardReject {
		return HeuristicResult{
			Pass:      false,
			Score:     0.0,
			Rationale: sourceRationale,
		}
	}

	// 4b. Recall-aware salience boost for filesystem events
	if event.Source == "filesystem" && event.Path != "" {
		recallBoost := h.GetRecallBoost(event.Path)
		if recallBoost > 0 {
			score += recallBoost
			sourceRationale += fmt.Sprintf("; recall boost +%.2f (recently accessed via MCP)", recallBoost)
		}
	}

	// 4c. Batch edit detection for filesystem events
	if event.Source == "filesystem" && event.Path != "" {
		isBatch, batchCount := h.IsBatchEdit(event.Path, event.Timestamp)
		if isBatch {
			sourceRationale += fmt.Sprintf("; batch edit detected (%d edits in window)", batchCount)
		}
	}

	// 5. Keyword scoring
	keywordScore, keywordMatches := h.scoreKeywords(event.Content)
	score += keywordScore

	// 6. Clamp score to [0.0, 1.0]
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	// 7. Pass threshold check (>= 0.2)
	rationale := sourceRationale
	if keywordMatches > 0 {
		rationale += fmt.Sprintf("; found %d high-signal keywords", keywordMatches)
	}

	passed := score >= 0.2
	return HeuristicResult{
		Pass:      passed,
		Score:     score,
		Rationale: rationale,
	}
}

// hashContent computes an MD5 hash of the content.
func (h *HeuristicFilter) hashContent(content string) string {
	hash := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// checkAndRecordFrequency checks if content has been seen too often and records it.
// Returns false if frequency threshold exceeded, true otherwise.
func (h *HeuristicFilter) checkAndRecordFrequency(contentHash string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	windowDuration := time.Duration(h.cfg.FrequencyWindowMin) * time.Minute
	cutoffTime := time.Now().Add(-windowDuration)

	// Get entries within the window
	var recentEntries []frequencyEntry
	if existingEntries, ok := h.frequency[contentHash]; ok {
		for _, entry := range existingEntries {
			if entry.timestamp.After(cutoffTime) {
				recentEntries = append(recentEntries, entry)
			}
		}
	}

	// Check threshold
	if len(recentEntries) >= h.cfg.FrequencyThreshold {
		return false // Threshold exceeded
	}

	// Record the new occurrence
	recentEntries = append(recentEntries, frequencyEntry{
		hash:      contentHash,
		timestamp: time.Now(),
	})
	h.frequency[contentHash] = recentEntries

	return true
}

// evaluateSource returns a base score, rationale, and whether the event should
// be hard-rejected (no keyword scoring can override it).
func (h *HeuristicFilter) evaluateSource(source, eventType, path, content string) (float32, string, bool) {
	switch source {
	case "filesystem":
		return h.evaluateFilesystem(path, content)
	case "terminal":
		return h.evaluateTerminal(content)
	case "clipboard":
		return h.evaluateClipboard(content)
	case "mcp":
		return h.evaluateMCP(eventType, content)
	default:
		// Unknown source: neutral score
		return 0.3, fmt.Sprintf("source=%s", source), false
	}
}

// evaluateFilesystem scores filesystem events.
func (h *HeuristicFilter) evaluateFilesystem(path, content string) (float32, string, bool) {
	// Skip if path contains ignored patterns — hard reject, no keyword override
	ignoredPatterns := []string{".git/", "node_modules/", "__pycache__/", ".DS_Store", "~", ".swp", ".tmp", ".xbel",
		"venv/", ".venv/", "site-packages/", ".tox/", ".mypy_cache/", ".ruff_cache/", ".pytest_cache/",
		".egg-info/", ".eggs/"}
	for _, pattern := range ignoredPatterns {
		if strings.Contains(path, pattern) {
			return 0.0, fmt.Sprintf("filesystem: ignored path pattern '%s'", pattern), true
		}
	}

	// Suppress application-internal state directories — hard reject
	appInternalDirs := []string{
		"/google-chrome/", "/chromium/", "/BraveSoftware/",
		"/LM Studio/", "/lm-studio/",
		"/Trash/", "/.local/share/Trash/",
		"/leveldb/", "/IndexedDB/", "/Local Storage/", "/Session Storage/",
		"/Cache/", "/GPUCache/", "/ShaderCache/", "/Code Cache/",
		"/dconf/", "/gconf/",
		"/pulse/", "/pipewire/", "/wireplumber/",
		"/gvfs-metadata/", "/tracker3/",
		"session_migration-",
		"/.copilot/", "/.github-copilot/",
		"/snap/", "/.snap/",
		"/.config/gtk-", "/.config/dbus-",
		"/.mnemonic/", "/.claude/",
	}
	lowerPathCheck := strings.ToLower(path)
	for _, dir := range appInternalDirs {
		if strings.Contains(lowerPathCheck, strings.ToLower(dir)) {
			return 0.0, fmt.Sprintf("filesystem: application-internal path '%s'", dir), true
		}
	}

	// Hard-reject sensitive files (defense-in-depth — watcher should block these first)
	sensitiveNames := []string{".env", "id_rsa", "id_ed25519", "id_ecdsa", ".pem", ".key",
		"credentials", "secret", ".keychain", ".keystore", ".netrc", ".htpasswd"}
	baseName := strings.ToLower(path)
	if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
		baseName = baseName[idx+1:]
	}
	for _, s := range sensitiveNames {
		if strings.Contains(baseName, s) {
			return 0.0, fmt.Sprintf("filesystem: sensitive file '%s'", s), true
		}
	}

	score := float32(0.3)
	rationale := "filesystem event"

	// Score boost for error logs and config files
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "error") || strings.Contains(lowerPath, ".log") {
		score += 0.2
		rationale += "; error/log file"
	} else if strings.HasSuffix(lowerPath, ".cfg") || strings.HasSuffix(lowerPath, ".conf") ||
		strings.HasSuffix(lowerPath, ".yaml") || strings.HasSuffix(lowerPath, ".json") ||
		strings.HasSuffix(lowerPath, ".toml") {
		score += 0.15
		rationale += "; config file"
	}

	// Score boost for source code
	sourceExtensions := []string{".go", ".py", ".js", ".ts", ".java", ".rs", ".cpp", ".c", ".h"}
	for _, ext := range sourceExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			score += 0.1
			rationale += "; source code"
			break
		}
	}

	return score, rationale, false
}

// evaluateTerminal scores terminal events.
func (h *HeuristicFilter) evaluateTerminal(content string) (float32, string, bool) {
	score := float32(0.3)
	rationale := "terminal event"

	command := strings.Fields(content)
	if len(command) == 0 {
		return score, rationale, false
	}

	cmd := strings.ToLower(command[0])

	// Skip trivial commands (only if they are just the command itself) — hard reject
	trivialCommands := map[string]bool{
		"cd": true, "ls": true, "pwd": true, "clear": true,
		"exit": true, "history": true, "which": true, "whoami": true,
		"echo": true,
	}

	if trivialCommands[cmd] && len(command) == 1 {
		return 0.0, fmt.Sprintf("terminal: trivial command '%s'", cmd), true
	}

	// Score boost for high-signal commands
	highSignalCommands := map[string]bool{
		"git": true, "make": true, "go": true, "npm": true, "docker": true,
		"kubectl": true, "ssh": true, "curl": true, "python": true, "node": true,
	}

	for signalCmd := range highSignalCommands {
		if strings.HasPrefix(cmd, signalCmd) {
			score += 0.25
			rationale += fmt.Sprintf("; high-signal command '%s'", cmd)
			break
		}
	}

	return score, rationale, false
}

// evaluateClipboard scores clipboard events.
func (h *HeuristicFilter) evaluateClipboard(content string) (float32, string, bool) {
	score := float32(0.3)
	rationale := "clipboard event"

	trimmed := strings.TrimSpace(content)

	// Skip if content looks like just a URL — hard reject
	if (strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")) &&
		!strings.ContainsAny(trimmed, " \t\n") {
		return 0.0, "clipboard: URL-only content", true
	}

	// Score boost for code snippets
	codeIndicators := []string{"{", "}", "function", "def", "class", "import", "package"}
	foundCodeIndicators := 0
	for _, indicator := range codeIndicators {
		if strings.Contains(content, indicator) {
			foundCodeIndicators++
		}
	}

	if foundCodeIndicators > 0 {
		score += 0.2
		rationale += fmt.Sprintf("; code snippet detected (%d indicators)", foundCodeIndicators)
	}

	return score, rationale, false
}

// scoreKeywords scans content for high-signal words and returns the score bonus and count.
func (h *HeuristicFilter) scoreKeywords(content string) (float32, int) {
	contentLower := strings.ToLower(content)
	score := float32(0.0)
	matchCount := 0

	// High signal keywords (0.15 each)
	highSignalKeywords := []string{
		"error", "bug", "fix", "todo", "hack",
		"important", "decision", "deadline", "meeting",
	}
	for _, keyword := range highSignalKeywords {
		if strings.Contains(contentLower, keyword) {
			score += 0.15
			matchCount++
		}
	}

	// Medium signal keywords (0.10 each)
	mediumSignalKeywords := []string{
		"config", "deploy", "release", "review",
		"merge", "refactor", "test", "fail",
	}
	for _, keyword := range mediumSignalKeywords {
		if strings.Contains(contentLower, keyword) {
			score += 0.10
			matchCount++
		}
	}

	// Low signal keywords (0.05 each)
	lowSignalKeywords := []string{
		"update", "change", "add", "remove", "create", "install",
	}
	for _, keyword := range lowSignalKeywords {
		if strings.Contains(contentLower, keyword) {
			score += 0.05
			matchCount++
		}
	}

	return score, matchCount
}

// evaluateMCP scores MCP-source events (from Claude Code tool calls).
// MCP events are high-signal — they represent explicit user/AI interaction.
func (h *HeuristicFilter) evaluateMCP(eventType, content string) (float32, string, bool) {
	score := float32(0.6) // High base score — MCP events are always intentional
	rationale := "mcp event (high-signal)"

	switch eventType {
	case "decision":
		score += 0.25
		rationale += "; decision type"
	case "error":
		score += 0.2
		rationale += "; error type"
	case "insight":
		score += 0.3
		rationale += "; insight type"
	case "learning":
		score += 0.2
		rationale += "; learning type"
	}

	return score, rationale, false
}

// IsBatchEdit checks if a filesystem event is part of a rapid batch of edits
// (e.g., AI-driven editing pattern) and returns true if it should be grouped
// rather than treated as an individual event. If batch detected, returns true
// and the count of edits in the batch window.
func (h *HeuristicFilter) IsBatchEdit(path string, timestamp time.Time) (bool, int) {
	h.editMu.Lock()
	defer h.editMu.Unlock()

	// Batch window: edits within 5 seconds are considered a batch
	batchWindow := 5 * time.Second
	cutoff := timestamp.Add(-batchWindow)

	// Clean old entries and count recent edits
	var recent []recentEdit
	for _, edit := range h.recentEdits {
		if edit.timestamp.After(cutoff) {
			recent = append(recent, edit)
		}
	}

	// Add current edit
	recent = append(recent, recentEdit{path: path, timestamp: timestamp})
	h.recentEdits = recent

	// If 3+ edits in the window, it's a batch
	batchThreshold := 3
	if len(recent) >= batchThreshold {
		return true, len(recent)
	}

	return false, len(recent)
}

// RecordRecalledFile marks a file as recently recalled via MCP,
// so future edits to it get a salience boost.
func (h *HeuristicFilter) RecordRecalledFile(path string) {
	h.recallMu.Lock()
	defer h.recallMu.Unlock()
	h.recalledFiles[path] = time.Now()
}

// GetRecallBoost returns a salience boost if the file was recently recalled
// via MCP (within the last 30 minutes). This detects the pattern where
// Claude recalls something, then the user edits the same file.
func (h *HeuristicFilter) GetRecallBoost(path string) float32 {
	if path == "" {
		return 0
	}

	h.recallMu.RLock()
	defer h.recallMu.RUnlock()

	recallTime, ok := h.recalledFiles[path]
	if !ok {
		return 0
	}

	// Boost decays over 30 minutes
	elapsed := time.Since(recallTime)
	if elapsed > 30*time.Minute {
		return 0
	}

	// Linear decay: 0.2 → 0 over 30 minutes
	boost := float32(0.2) * float32(1.0-elapsed.Seconds()/(30*60))
	return boost
}

// Event represents a watcher event.
type Event struct {
	ID        string
	Source    string
	Type      string
	Path      string
	Content   string
	Timestamp time.Time
	Metadata  map[string]interface{}
}
