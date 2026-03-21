package retrieval

import (
	"sync"
	"time"
)

// activityTracker maintains a time-decaying map of concepts observed from
// recent watcher events (file edits, terminal commands, clipboard). The
// retrieval agent uses this to boost recall scores for memories whose
// concepts overlap with recent daemon activity.
type activityTracker struct {
	mu       sync.RWMutex
	concepts map[string]time.Time // concept → most recent observation time
	window   time.Duration        // how long concepts remain active
	maxBoost float32              // cap on the additive context boost
}

// newActivityTracker creates a tracker with the given decay window and max boost.
func newActivityTracker(windowMinutes int, maxBoost float32) *activityTracker {
	return &activityTracker{
		concepts: make(map[string]time.Time),
		window:   time.Duration(windowMinutes) * time.Minute,
		maxBoost: maxBoost,
	}
}

// windowMinutes returns the decay window in minutes.
func (at *activityTracker) windowMinutes() int {
	if at == nil {
		return 30
	}
	return int(at.window.Minutes())
}

// observe records that the given concepts were just seen in watcher activity.
// Upserts timestamps and lazily evicts expired entries when the map grows large.
func (at *activityTracker) observe(concepts []string) {
	at.mu.Lock()
	defer at.mu.Unlock()

	now := time.Now()
	for _, c := range concepts {
		at.concepts[c] = now
	}

	// Lazy cleanup: evict expired entries when map exceeds threshold.
	if len(at.concepts) > 500 {
		for k, ts := range at.concepts {
			if now.Sub(ts) > at.window {
				delete(at.concepts, k)
			}
		}
	}
}

// snapshot returns a copy of the current concept map, filtered to non-expired entries.
func (at *activityTracker) snapshot() map[string]time.Time {
	if at == nil {
		return nil
	}
	at.mu.RLock()
	defer at.mu.RUnlock()

	now := time.Now()
	out := make(map[string]time.Time, len(at.concepts))
	for k, ts := range at.concepts {
		if now.Sub(ts) < at.window {
			out[k] = ts
		}
	}
	return out
}

// loadSnapshot replaces the concept map with the provided snapshot.
// Used by MCP processes to sync activity state from the daemon.
func (at *activityTracker) loadSnapshot(snap map[string]time.Time) {
	if at == nil {
		return
	}
	at.mu.Lock()
	defer at.mu.Unlock()
	at.concepts = snap
}

// boostForMemory computes an additive score boost for a memory based on
// how many of its concepts overlap with recent watcher activity. The boost
// scales with overlap fraction and decays linearly over the window.
//
// Formula: sum(decayed_weight for matching concepts) / max(len(memoryConcepts), 1)
// Clamped to [0, maxBoost].
func (at *activityTracker) boostForMemory(memoryConcepts []string) float32 {
	if at == nil || len(memoryConcepts) == 0 {
		return 0
	}

	at.mu.RLock()
	defer at.mu.RUnlock()

	if len(at.concepts) == 0 {
		return 0
	}

	now := time.Now()
	var totalWeight float32
	for _, mc := range memoryConcepts {
		if ts, ok := at.concepts[mc]; ok {
			elapsed := now.Sub(ts)
			if elapsed < at.window {
				// Linear decay: 1.0 at time=0, 0.0 at time=window.
				weight := float32(1.0 - float64(elapsed)/float64(at.window))
				totalWeight += weight
			}
		}
	}

	boost := totalWeight / float32(len(memoryConcepts))
	if boost > at.maxBoost {
		boost = at.maxBoost
	}
	return boost
}
