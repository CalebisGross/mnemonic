package perception

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Config Behavioral Tests — verify each HeuristicConfig param affects behavior
// ---------------------------------------------------------------------------

func newConfigFilter(cfg HeuristicConfig) *HeuristicFilter {
	return NewHeuristicFilter(cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

// validSourceCodeEvent returns an event that would pass all source-specific
// and keyword checks — only config-driven checks should affect the outcome.
func validSourceCodeEvent(content string) Event {
	return Event{
		Source:    "filesystem",
		Type:      "file_modified",
		Path:      "/home/user/Projects/myapp/internal/handler.go",
		Content:   content,
		Timestamp: time.Now(),
	}
}

func TestConfigMinContentLengthFilters(t *testing.T) {
	shortContent := "x = 1" // 5 chars

	tests := []struct {
		name     string
		minLen   int
		wantPass bool
	}{
		{"min_3_passes", 3, true},
		{"min_10_rejects", 10, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := HeuristicConfig{
				MinContentLength:   tc.minLen,
				MaxContentLength:   100000,
				FrequencyThreshold: 100, // high so frequency doesn't interfere
				FrequencyWindowMin: 10,
			}
			hf := newConfigFilter(cfg)

			result := hf.Evaluate(validSourceCodeEvent(shortContent))
			if result.Pass != tc.wantPass {
				t.Errorf("minContentLength=%d, content len=%d: expected Pass=%v, got Pass=%v (score=%.2f, rationale=%q)",
					tc.minLen, len(shortContent), tc.wantPass, result.Pass, result.Score, result.Rationale)
			}
			if !tc.wantPass && result.Score != 0.0 {
				t.Errorf("expected score 0.0 on reject, got %.2f", result.Score)
			}
		})
	}
}

func TestConfigMaxContentLengthFilters(t *testing.T) {
	// Generate content of ~50K chars
	largeContent := strings.Repeat("func handler() { /* error handling code */ }\n", 1200) // ~52800 chars

	tests := []struct {
		name     string
		maxLen   int
		wantPass bool
	}{
		{"max_40000_rejects", 40000, false},
		{"max_60000_passes", 60000, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := HeuristicConfig{
				MinContentLength:   1,
				MaxContentLength:   tc.maxLen,
				FrequencyThreshold: 100,
				FrequencyWindowMin: 10,
			}
			hf := newConfigFilter(cfg)

			result := hf.Evaluate(validSourceCodeEvent(largeContent))
			if result.Pass != tc.wantPass {
				t.Errorf("maxContentLength=%d, content len=%d: expected Pass=%v, got Pass=%v (score=%.2f, rationale=%q)",
					tc.maxLen, len(largeContent), tc.wantPass, result.Pass, result.Score, result.Rationale)
			}
			if !tc.wantPass && result.Score != 0.0 {
				t.Errorf("expected score 0.0 on reject, got %.2f", result.Score)
			}
		})
	}
}

func TestConfigFrequencyThresholdBlocksRepetition(t *testing.T) {
	tests := []struct {
		name         string
		threshold    int
		submissions  int
		wantLastPass bool
	}{
		// Submit same content 3 times: threshold=2 means 3rd is blocked (seen >2)
		{"threshold_2_blocks_at_3", 2, 3, false},
		// Submit same content 3 times: threshold=5 means 3rd still passes (seen 3 <= 5)
		{"threshold_5_allows_3", 5, 3, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := HeuristicConfig{
				MinContentLength:   1,
				MaxContentLength:   100000,
				FrequencyThreshold: tc.threshold,
				FrequencyWindowMin: 60, // large window so entries don't expire
			}
			hf := newConfigFilter(cfg)

			content := "func handleRequest(w http.ResponseWriter, r *http.Request) { /* error */ }"
			event := validSourceCodeEvent(content)

			var lastResult HeuristicResult
			for i := 0; i < tc.submissions; i++ {
				lastResult = hf.Evaluate(event)
			}

			if lastResult.Pass != tc.wantLastPass {
				t.Errorf("threshold=%d after %d submissions: expected Pass=%v, got Pass=%v (score=%.2f, rationale=%q)",
					tc.threshold, tc.submissions, tc.wantLastPass, lastResult.Pass, lastResult.Score, lastResult.Rationale)
			}
		})
	}
}

func TestConfigFrequencyWindowMinControlsExpiry(t *testing.T) {
	// This test verifies that the frequency window controls which entries
	// are considered "recent". We can't easily manipulate time.Now() in the
	// production code, but we can verify the window is used by the cleanup logic.

	tests := []struct {
		name       string
		windowMin  int
		entryAge   time.Duration
		expectKept bool // whether the entry survives cleanup
	}{
		// Entry is 5 minutes old, window is 10 minutes: entry is kept
		{"10min_window_keeps_5min_old", 10, 5 * time.Minute, true},
		// Entry is 5 minutes old, window is 1 minute: entry is expired
		{"1min_window_expires_5min_old", 1, 5 * time.Minute, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := HeuristicConfig{
				MinContentLength:   1,
				MaxContentLength:   100000,
				FrequencyThreshold: 1, // block after first occurrence
				FrequencyWindowMin: tc.windowMin,
			}
			hf := newConfigFilter(cfg)

			content := "func handler() { return nil }"
			event := validSourceCodeEvent(content)

			// First evaluation records the frequency entry
			hf.Evaluate(event)

			// Manually backdate the frequency entries to simulate age
			hf.mu.Lock()
			for hash, entries := range hf.frequency {
				for i := range entries {
					entries[i].timestamp = time.Now().Add(-tc.entryAge)
				}
				hf.frequency[hash] = entries
			}
			hf.mu.Unlock()

			// Run cleanup (uses the configured window to prune old entries)
			hf.cleanup()

			// Check if entries survived
			hf.mu.RLock()
			totalEntries := 0
			for _, entries := range hf.frequency {
				totalEntries += len(entries)
			}
			hf.mu.RUnlock()

			gotKept := totalEntries > 0
			if gotKept != tc.expectKept {
				t.Errorf("windowMin=%d, entryAge=%v: expected kept=%v, got kept=%v (entries=%d)",
					tc.windowMin, tc.entryAge, tc.expectKept, gotKept, totalEntries)
			}
		})
	}
}
