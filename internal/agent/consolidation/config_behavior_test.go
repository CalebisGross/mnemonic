package consolidation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// ---------------------------------------------------------------------------
// Config Behavioral Tests — verify each config param affects behavior
// ---------------------------------------------------------------------------

func TestConfigDecayRateAffectsSalienceDrop(t *testing.T) {
	tests := []struct {
		name        string
		decayRate   float64
		wantMinDrop float32 // minimum salience drop expected
		wantMaxDrop float32 // maximum salience drop expected
	}{
		// DecayRate 0.5: salience * 0.5^(recencyFactor*accessBonus) — big drop
		{"aggressive_decay_0.5", 0.5, 0.2, 0.5},
		// DecayRate 0.99: salience barely moves
		{"gentle_decay_0.99", 0.99, 0.0, 0.02},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			initialSalience := float32(0.8)

			var capturedUpdates map[string]float32
			s := newMockStore()
			s.listMemoriesFn = func(_ context.Context, state string, limit, _ int) ([]store.Memory, error) {
				if state == "active" {
					return []store.Memory{
						{
							ID:           "m1",
							Salience:     initialSalience,
							LastAccessed: time.Now().Add(-200 * time.Hour), // >168h, full decay
							CreatedAt:    time.Now().Add(-200 * time.Hour),
						},
					}, nil
				}
				return nil, nil
			}
			s.batchUpdateSalienceFn = func(_ context.Context, updates map[string]float32) error {
				capturedUpdates = updates
				return nil
			}

			cfg := DefaultConfig()
			cfg.DecayRate = tc.decayRate
			agent := NewConsolidationAgent(s, nil, cfg, testLogger())

			_, _, err := agent.decaySalience(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			newSalience, ok := capturedUpdates["m1"]
			if !ok {
				t.Fatal("expected m1 in salience updates")
			}

			drop := initialSalience - newSalience
			if drop < tc.wantMinDrop || drop > tc.wantMaxDrop {
				t.Errorf("decayRate=%.2f: expected salience drop in [%.2f, %.2f], got %.4f (new=%.4f)",
					tc.decayRate, tc.wantMinDrop, tc.wantMaxDrop, drop, newSalience)
			}
		})
	}
}

func TestConfigFadeThresholdControlsTransition(t *testing.T) {
	tests := []struct {
		name          string
		memSalience   float32
		fadeThreshold float64
		expectFading  bool
	}{
		{"salience_0.35_threshold_0.3_stays_active", 0.35, 0.3, false},
		{"salience_0.35_threshold_0.4_becomes_fading", 0.35, 0.4, true},
		{"salience_0.25_threshold_0.3_becomes_fading", 0.25, 0.3, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newMockStore()
			s.listMemoriesFn = func(_ context.Context, state string, _, _ int) ([]store.Memory, error) {
				if state == "active" {
					return []store.Memory{
						{ID: "m1", Salience: tc.memSalience},
					}, nil
				}
				return nil, nil
			}

			cfg := DefaultConfig()
			cfg.FadeThreshold = tc.fadeThreshold
			cfg.ArchiveThreshold = 0.05 // very low so it doesn't interfere
			agent := NewConsolidationAgent(s, nil, cfg, testLogger())

			toFading, _, err := agent.transitionStates(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotFading := toFading > 0
			if gotFading != tc.expectFading {
				t.Errorf("salience=%.2f, fadeThreshold=%.2f: expected fading=%v, got %v (toFading=%d)",
					tc.memSalience, tc.fadeThreshold, tc.expectFading, gotFading, toFading)
			}
		})
	}
}

func TestConfigArchiveThresholdControlsTransition(t *testing.T) {
	tests := []struct {
		name             string
		memSalience      float32
		archiveThreshold float64
		expectArchived   bool
	}{
		{"salience_0.15_threshold_0.1_stays_fading", 0.15, 0.1, false},
		{"salience_0.15_threshold_0.2_becomes_archived", 0.15, 0.2, true},
		{"salience_0.05_threshold_0.1_becomes_archived", 0.05, 0.1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newMockStore()
			s.listMemoriesFn = func(_ context.Context, state string, _, _ int) ([]store.Memory, error) {
				if state == "fading" {
					return []store.Memory{
						{ID: "m1", Salience: tc.memSalience},
					}, nil
				}
				return nil, nil
			}

			cfg := DefaultConfig()
			cfg.ArchiveThreshold = tc.archiveThreshold
			agent := NewConsolidationAgent(s, nil, cfg, testLogger())

			_, toArchived, err := agent.transitionStates(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotArchived := toArchived > 0
			if gotArchived != tc.expectArchived {
				t.Errorf("salience=%.2f, archiveThreshold=%.2f: expected archived=%v, got %v",
					tc.memSalience, tc.archiveThreshold, tc.expectArchived, gotArchived)
			}
		})
	}
}

func TestConfigRetentionWindowAffectsDeletion(t *testing.T) {
	tests := []struct {
		name            string
		retentionWindow time.Duration
		expectDelete    bool
	}{
		// Memory archived 100 days ago
		{"90d_window_deletes", 90 * 24 * time.Hour, true},    // 100 > 90 → deleted
		{"120d_window_retains", 120 * 24 * time.Hour, false}, // 100 < 120 → retained
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedCutoff time.Time
			s := newMockStore()
			s.deleteOldArchivedFn = func(_ context.Context, olderThan time.Time) (int, error) {
				capturedCutoff = olderThan
				// Simulate: memory is 100 days old; delete if cutoff is after its creation
				memTime := time.Now().Add(-100 * 24 * time.Hour)
				if memTime.Before(olderThan) {
					return 1, nil
				}
				return 0, nil
			}

			cfg := DefaultConfig()
			cfg.RetentionWindow = tc.retentionWindow
			agent := NewConsolidationAgent(s, nil, cfg, testLogger())

			deleted, err := agent.deleteExpired(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !capturedCutoff.IsZero() {
				// Verify cutoff is approximately now - retentionWindow
				expectedCutoff := time.Now().Add(-tc.retentionWindow)
				diff := capturedCutoff.Sub(expectedCutoff)
				if diff < -time.Minute || diff > time.Minute {
					t.Errorf("cutoff time mismatch: expected ~%v, got %v", expectedCutoff, capturedCutoff)
				}
			}

			gotDeleted := deleted > 0
			if gotDeleted != tc.expectDelete {
				t.Errorf("retentionWindow=%v: expected deletion=%v, got %v (deleted=%d)",
					tc.retentionWindow, tc.expectDelete, gotDeleted, deleted)
			}
		})
	}
}

func TestConfigMaxMemoriesPerCycleLimitsProcessing(t *testing.T) {
	tests := []struct {
		name          string
		totalMemories int
		maxPerCycle   int
		wantProcessed int
	}{
		{"limit_10_of_50", 50, 10, 10},
		{"limit_100_of_50", 50, 100, 50},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newMockStore()
			s.listMemoriesFn = func(_ context.Context, state string, limit, _ int) ([]store.Memory, error) {
				if state == "active" {
					count := tc.totalMemories
					if limit < count {
						count = limit
					}
					memories := make([]store.Memory, count)
					for i := range memories {
						memories[i] = store.Memory{
							ID:           fmt.Sprintf("m%d", i),
							Salience:     0.8,
							LastAccessed: time.Now().Add(-200 * time.Hour),
							CreatedAt:    time.Now().Add(-200 * time.Hour),
						}
					}
					return memories, nil
				}
				return nil, nil
			}
			s.batchUpdateSalienceFn = func(_ context.Context, updates map[string]float32) error {
				return nil
			}

			cfg := DefaultConfig()
			cfg.MaxMemoriesPerCycle = tc.maxPerCycle
			agent := NewConsolidationAgent(s, nil, cfg, testLogger())

			_, processed, err := agent.decaySalience(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if processed != tc.wantProcessed {
				t.Errorf("maxPerCycle=%d, totalMemories=%d: expected %d processed, got %d",
					tc.maxPerCycle, tc.totalMemories, tc.wantProcessed, processed)
			}
		})
	}
}

func TestConfigMaxMergesPerCycleLimitsMerges(t *testing.T) {
	// Create enough identical-embedding memories to form multiple clusters
	emb := []float32{0.1, 0.2, 0.3, 0.4}

	tests := []struct {
		name         string
		maxMerges    int
		clusterCount int // how many clusters of 3 we'll create
		wantMerges   int
	}{
		{"limit_1_merge", 1, 3, 1},
		{"limit_5_merges", 5, 3, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create memories in clusters of 3 (identical embeddings within each cluster)
			var memories []store.Memory
			for c := 0; c < tc.clusterCount; c++ {
				clusterEmb := make([]float32, len(emb))
				copy(clusterEmb, emb)
				clusterEmb[0] = float32(c) * 0.01 // slightly different per cluster seed

				for i := 0; i < 3; i++ {
					memories = append(memories, store.Memory{
						ID:        fmt.Sprintf("c%d-m%d", c, i),
						Summary:   fmt.Sprintf("cluster %d memory %d", c, i),
						Salience:  0.8,
						Embedding: clusterEmb,
					})
				}
			}

			s := newMockStore()
			s.listMemoriesFn = func(_ context.Context, state string, _, _ int) ([]store.Memory, error) {
				if state == "active" {
					return memories, nil
				}
				return nil, nil
			}

			llmProv := newMockLLMProvider()
			llmProv.completeFn = func(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
				return llm.CompletionResponse{
					Content: `{"summary": "merged gist", "concepts": ["test"], "salience": 0.9}`,
				}, nil
			}
			llmProv.embedFn = func(_ context.Context, _ string) ([]float32, error) {
				return emb, nil
			}

			cfg := DefaultConfig()
			cfg.MaxMergesPerCycle = tc.maxMerges
			cfg.MinClusterSize = 3
			agent := NewConsolidationAgent(s, llmProv, cfg, testLogger())

			merges, err := agent.mergeClusters(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if merges > tc.maxMerges {
				t.Errorf("maxMerges=%d: expected at most %d merges, got %d",
					tc.maxMerges, tc.maxMerges, merges)
			}
		})
	}
}

func TestConfigMinClusterSizeFiltersMerge(t *testing.T) {
	// 2 memories with identical embeddings
	emb := []float32{0.1, 0.2, 0.3, 0.4}
	memories := []store.Memory{
		{ID: "m1", Summary: "mem 1", Salience: 0.8, Embedding: emb},
		{ID: "m2", Summary: "mem 2", Salience: 0.8, Embedding: emb},
	}

	tests := []struct {
		name           string
		minClusterSize int
		expectMerge    bool
	}{
		{"min_2_merges", 2, true},
		{"min_3_skips", 3, false}, // only 2 memories, can't form cluster of 3
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newMockStore()
			s.listMemoriesFn = func(_ context.Context, state string, _, _ int) ([]store.Memory, error) {
				if state == "active" {
					return memories, nil
				}
				return nil, nil
			}

			llmProv := newMockLLMProvider()
			llmProv.completeFn = func(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
				return llm.CompletionResponse{
					Content: `{"summary": "merged", "concepts": ["test"], "salience": 0.9}`,
				}, nil
			}
			llmProv.embedFn = func(_ context.Context, _ string) ([]float32, error) {
				return emb, nil
			}

			cfg := DefaultConfig()
			cfg.MinClusterSize = tc.minClusterSize
			agent := NewConsolidationAgent(s, llmProv, cfg, testLogger())

			merges, err := agent.mergeClusters(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotMerge := merges > 0
			if gotMerge != tc.expectMerge {
				t.Errorf("minClusterSize=%d with 2 memories: expected merge=%v, got %v (merges=%d)",
					tc.minClusterSize, tc.expectMerge, gotMerge, merges)
			}
		})
	}
}

func TestConfigAssocPruneThresholdPassedToStore(t *testing.T) {
	tests := []struct {
		name      string
		threshold float32
	}{
		{"threshold_0.05", 0.05},
		{"threshold_0.2", 0.2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newMockStore()
			cfg := DefaultConfig()
			cfg.AssocPruneThreshold = tc.threshold
			agent := NewConsolidationAgent(s, nil, cfg, testLogger())

			_, err := agent.pruneAssociations(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(s.pruneWeakAssocCalls) != 1 {
				t.Fatalf("expected 1 PruneWeakAssociations call, got %d", len(s.pruneWeakAssocCalls))
			}
			if s.pruneWeakAssocCalls[0] != tc.threshold {
				t.Errorf("expected threshold %.2f passed to store, got %.2f",
					tc.threshold, s.pruneWeakAssocCalls[0])
			}
		})
	}
}
