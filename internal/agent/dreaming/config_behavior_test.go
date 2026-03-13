package dreaming

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/store"
)

// configMockStore embeds the zero-value mockStore and overrides specific methods via callbacks.
type configMockStore struct {
	mockStore
	listMemoriesFn       func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error)
	getDeadMemoriesFn    func(ctx context.Context, cutoffDate time.Time) ([]store.Memory, error)
	updateSalienceFn     func(ctx context.Context, id string, salience float32) error
	getAssociationsFn    func(ctx context.Context, memoryID string) ([]store.Association, error)
	updateAssocStrFn     func(ctx context.Context, sourceID, targetID string, strength float32) error

	// Call tracking
	updateSalienceCalls  []updateSalienceCall
	updateAssocStrCalls  []updateAssocStrCall
}

type updateSalienceCall struct {
	ID       string
	Salience float32
}

type updateAssocStrCall struct {
	SourceID  string
	TargetID  string
	Strength  float32
}

func (m *configMockStore) ListMemories(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
	if m.listMemoriesFn != nil {
		return m.listMemoriesFn(ctx, state, limit, offset)
	}
	return nil, nil
}

func (m *configMockStore) GetDeadMemories(ctx context.Context, cutoffDate time.Time) ([]store.Memory, error) {
	if m.getDeadMemoriesFn != nil {
		return m.getDeadMemoriesFn(ctx, cutoffDate)
	}
	return nil, nil
}

func (m *configMockStore) UpdateSalience(ctx context.Context, id string, salience float32) error {
	m.updateSalienceCalls = append(m.updateSalienceCalls, updateSalienceCall{ID: id, Salience: salience})
	if m.updateSalienceFn != nil {
		return m.updateSalienceFn(ctx, id, salience)
	}
	return nil
}

func (m *configMockStore) GetAssociations(ctx context.Context, memoryID string) ([]store.Association, error) {
	if m.getAssociationsFn != nil {
		return m.getAssociationsFn(ctx, memoryID)
	}
	return nil, nil
}

func (m *configMockStore) UpdateAssociationStrength(ctx context.Context, sourceID, targetID string, strength float32) error {
	m.updateAssocStrCalls = append(m.updateAssocStrCalls, updateAssocStrCall{SourceID: sourceID, TargetID: targetID, Strength: strength})
	if m.updateAssocStrFn != nil {
		return m.updateAssocStrFn(ctx, sourceID, targetID, strength)
	}
	return nil
}

func cfgTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------------------------------
// Config Behavioral Tests
// ---------------------------------------------------------------------------

func TestConfigBatchSizeLimitsReplay(t *testing.T) {
	tests := []struct {
		name         string
		batchSize    int
		totalMemories int
		wantReplayed int
	}{
		{"batch_10_of_50", 10, 50, 10},
		{"batch_30_of_50", 30, 50, 30},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &configMockStore{
				listMemoriesFn: func(_ context.Context, _ string, limit, _ int) ([]store.Memory, error) {
					count := tc.totalMemories
					if limit < count {
						count = limit
					}
					memories := make([]store.Memory, count)
					for i := range memories {
						memories[i] = store.Memory{
							ID:       fmt.Sprintf("m%d", i),
							Salience: 0.8, // above any reasonable threshold
						}
					}
					return memories, nil
				},
			}

			cfg := DreamingConfig{
				Interval:               3 * time.Hour,
				BatchSize:              tc.batchSize,
				SalienceThreshold:      0.3,
				AssociationBoostFactor: 1.15,
				NoisePruneThreshold:    0.15,
			}
			agent := NewDreamingAgent(s, nil, cfg, cfgTestLogger())

			report := &DreamReport{}
			replayed, err := agent.replayMemories(context.Background(), report)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(replayed) != tc.wantReplayed {
				t.Errorf("batchSize=%d: expected %d replayed, got %d",
					tc.batchSize, tc.wantReplayed, len(replayed))
			}
		})
	}
}

func TestConfigSalienceThresholdFiltersReplay(t *testing.T) {
	memories := []store.Memory{
		{ID: "high", Salience: 0.8},
		{ID: "mid", Salience: 0.4},
		{ID: "low", Salience: 0.2},
	}

	tests := []struct {
		name           string
		threshold      float32
		wantReplayed   int
		expectIDs      []string
	}{
		{"threshold_0.1_all_pass", 0.1, 3, []string{"high", "mid", "low"}},
		{"threshold_0.3_filters_low", 0.3, 2, []string{"high", "mid"}},
		{"threshold_0.5_only_high", 0.5, 1, []string{"high"}},
		{"threshold_0.9_none_pass", 0.9, 0, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &configMockStore{
				listMemoriesFn: func(_ context.Context, _ string, _, _ int) ([]store.Memory, error) {
					return memories, nil
				},
			}

			cfg := DreamingConfig{
				BatchSize:              100,
				SalienceThreshold:      tc.threshold,
				AssociationBoostFactor: 1.15,
				NoisePruneThreshold:    0.15,
			}
			agent := NewDreamingAgent(s, nil, cfg, cfgTestLogger())

			report := &DreamReport{}
			replayed, err := agent.replayMemories(context.Background(), report)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(replayed) != tc.wantReplayed {
				t.Errorf("threshold=%.1f: expected %d replayed, got %d",
					tc.threshold, tc.wantReplayed, len(replayed))
			}
		})
	}
}

func TestConfigAssociationBoostFactorStrengthens(t *testing.T) {
	tests := []struct {
		name          string
		boostFactor   float32
		initialStr    float32
		wantStrMin    float32
		wantStrMax    float32
	}{
		// 0.5 * 1.0 = 0.5 (unchanged)
		{"boost_1.0_no_change", 1.0, 0.5, 0.5, 0.5},
		// 0.5 * 1.5 = 0.75
		{"boost_1.5_strengthens", 1.5, 0.5, 0.74, 0.76},
		// 0.9 * 1.5 = 1.35 → capped at 1.0
		{"boost_1.5_caps_at_1.0", 1.5, 0.9, 1.0, 1.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &configMockStore{
				getAssociationsFn: func(_ context.Context, _ string) ([]store.Association, error) {
					return []store.Association{
						{SourceID: "m1", TargetID: "m2", Strength: tc.initialStr, RelationType: "similar"},
					}, nil
				},
			}

			cfg := DreamingConfig{
				BatchSize:              100,
				SalienceThreshold:      0.1,
				AssociationBoostFactor: tc.boostFactor,
				NoisePruneThreshold:    0.15,
			}
			agent := NewDreamingAgent(s, nil, cfg, cfgTestLogger())

			replayed := []store.Memory{{ID: "m1", Salience: 0.8}}
			report := &DreamReport{}
			err := agent.strengthenAssociations(context.Background(), replayed, report)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.boostFactor == 1.0 {
				// No change expected — no UpdateAssociationStrength call
				if len(s.updateAssocStrCalls) != 0 {
					t.Errorf("boost=1.0: expected 0 update calls, got %d", len(s.updateAssocStrCalls))
				}
				return
			}

			if len(s.updateAssocStrCalls) != 1 {
				t.Fatalf("expected 1 update call, got %d", len(s.updateAssocStrCalls))
			}
			got := s.updateAssocStrCalls[0].Strength
			if got < tc.wantStrMin || got > tc.wantStrMax {
				t.Errorf("boost=%.1f, initial=%.1f: expected strength in [%.2f, %.2f], got %.4f",
					tc.boostFactor, tc.initialStr, tc.wantStrMin, tc.wantStrMax, got)
			}
		})
	}
}

func TestConfigNoisePruneThresholdDemotesLowSalience(t *testing.T) {
	tests := []struct {
		name           string
		threshold      float32
		memSalience    float32
		expectDemoted  bool
	}{
		// Memory salience 0.1, threshold 0.05 → not below threshold → not demoted
		{"salience_0.1_threshold_0.05_keeps", 0.05, 0.1, false},
		// Memory salience 0.1, threshold 0.2 → below threshold → demoted
		{"salience_0.1_threshold_0.2_demotes", 0.2, 0.1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &configMockStore{
				getDeadMemoriesFn: func(_ context.Context, _ time.Time) ([]store.Memory, error) {
					return []store.Memory{
						{ID: "dead1", Salience: tc.memSalience},
					}, nil
				},
			}

			cfg := DreamingConfig{
				BatchSize:              100,
				SalienceThreshold:      0.1,
				AssociationBoostFactor: 1.15,
				NoisePruneThreshold:    tc.threshold,
			}
			agent := NewDreamingAgent(s, nil, cfg, cfgTestLogger())

			report := &DreamReport{}
			err := agent.noisePrune(context.Background(), report)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotDemoted := report.NoisyMemoriesDemoted > 0
			if gotDemoted != tc.expectDemoted {
				t.Errorf("threshold=%.2f, salience=%.2f: expected demoted=%v, got %v",
					tc.threshold, tc.memSalience, tc.expectDemoted, gotDemoted)
			}

			if tc.expectDemoted {
				if len(s.updateSalienceCalls) != 1 {
					t.Fatalf("expected 1 UpdateSalience call, got %d", len(s.updateSalienceCalls))
				}
				// Salience should be reduced (multiplied by 0.8)
				expected := tc.memSalience * 0.8
				got := s.updateSalienceCalls[0].Salience
				if got < expected-0.001 || got > expected+0.001 {
					t.Errorf("expected new salience ~%.4f, got %.4f", expected, got)
				}
			}
		})
	}
}
