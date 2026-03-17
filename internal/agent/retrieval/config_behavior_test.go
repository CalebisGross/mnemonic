package retrieval

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

func TestConfigMaxResultsLimitsOutput(t *testing.T) {
	now := time.Now()

	// Return 10 FTS results
	memories := make([]store.Memory, 10)
	for i := range memories {
		memories[i] = store.Memory{
			ID:           fmt.Sprintf("m%d", i),
			Summary:      fmt.Sprintf("memory %d", i),
			Salience:     0.9 - float32(i)*0.05,
			LastAccessed: now,
		}
	}

	s := &mockStore{
		searchByFullTextFunc: func(_ context.Context, _ string, _ int) ([]store.Memory, error) {
			return memories, nil
		},
		searchByEmbeddingFunc: func(_ context.Context, _ []float32, _ int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(_ context.Context, _ string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			for _, m := range memories {
				if m.ID == id {
					return m, nil
				}
			}
			return store.Memory{ID: id, Salience: 0.5, LastAccessed: now}, nil
		},
	}

	tests := []struct {
		name       string
		maxResults int
		wantAtMost int
	}{
		{"max_results=1", 1, 1},
		{"max_results=3", 3, 3},
		{"max_results=5", 5, 5},
		{"max_results=10", 10, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MaxResults = tc.maxResults
			agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

			resp, err := agent.Query(context.Background(), QueryRequest{Query: "test"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Memories) > tc.wantAtMost {
				t.Errorf("config MaxResults=%d but got %d results", tc.maxResults, len(resp.Memories))
			}
		})
	}
}

func TestConfigMaxHopsControlsGraphDepth(t *testing.T) {
	// Chain: m1 → m2 → m3 → m4 (each hop via strong association)
	s := &mockStore{
		getAssociationsFunc: func(_ context.Context, memoryID string) ([]store.Association, error) {
			chains := map[string]string{"m1": "m2", "m2": "m3", "m3": "m4"}
			if target, ok := chains[memoryID]; ok {
				return []store.Association{
					{SourceID: memoryID, TargetID: target, Strength: 0.9, RelationType: "similar"},
				}, nil
			}
			return nil, nil
		},
	}

	tests := []struct {
		name     string
		maxHops  int
		wantIDs  []string
		dontWant []string
	}{
		{"0_hops_entry_only", 0, []string{"m1"}, []string{"m2", "m3", "m4"}},
		{"1_hop_reaches_m2", 1, []string{"m1", "m2"}, []string{"m3", "m4"}},
		{"2_hops_reaches_m3", 2, []string{"m1", "m2", "m3"}, []string{"m4"}},
		{"3_hops_reaches_m4", 3, []string{"m1", "m2", "m3", "m4"}, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := RetrievalConfig{
				MaxHops:             tc.maxHops,
				ActivationThreshold: 0.01,
				DecayFactor:         0.9, // high so activation survives multiple hops
				MaxResults:          10,
			}
			agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

			entryPoints := map[string]float32{"m1": 1.0}
			result, _ := agent.spreadActivation(context.Background(), entryPoints)

			for _, id := range tc.wantIDs {
				if _, ok := result[id]; !ok {
					t.Errorf("expected %s to be activated with maxHops=%d", id, tc.maxHops)
				}
			}
			for _, id := range tc.dontWant {
				if _, ok := result[id]; ok {
					t.Errorf("expected %s NOT to be activated with maxHops=%d", id, tc.maxHops)
				}
			}
		})
	}
}

func TestConfigActivationThresholdPrunesWeak(t *testing.T) {
	// m1 has a weak association to m2 (strength 0.15)
	s := &mockStore{
		getAssociationsFunc: func(_ context.Context, memoryID string) ([]store.Association, error) {
			if memoryID == "m1" {
				return []store.Association{
					{SourceID: "m1", TargetID: "m2", Strength: 0.15, RelationType: "similar"},
				}, nil
			}
			return nil, nil
		},
	}

	tests := []struct {
		name      string
		threshold float32
		expectM2  bool
	}{
		// Propagated: 1.0 * 0.15 * 0.7^1 * 1.0 = 0.105
		{"threshold_0.1_propagates", 0.1, true},
		{"threshold_0.2_prunes", 0.2, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := RetrievalConfig{
				MaxHops:             1,
				ActivationThreshold: tc.threshold,
				DecayFactor:         0.7,
				MaxResults:          10,
			}
			agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

			entryPoints := map[string]float32{"m1": 1.0}
			result, _ := agent.spreadActivation(context.Background(), entryPoints)

			_, hasM2 := result["m2"]
			if hasM2 != tc.expectM2 {
				t.Errorf("threshold=%.2f: expected m2 activated=%v, got %v", tc.threshold, tc.expectM2, hasM2)
			}
		})
	}
}

func TestConfigDecayFactorAffectsActivationMagnitude(t *testing.T) {
	// 2-hop chain: m1 → m2 → m3
	s := &mockStore{
		getAssociationsFunc: func(_ context.Context, memoryID string) ([]store.Association, error) {
			chains := map[string]string{"m1": "m2", "m2": "m3"}
			if target, ok := chains[memoryID]; ok {
				return []store.Association{
					{SourceID: memoryID, TargetID: target, Strength: 1.0, RelationType: "similar"},
				}, nil
			}
			return nil, nil
		},
	}

	tests := []struct {
		name        string
		decayFactor float32
		wantM2Min   float32
		wantM2Max   float32
	}{
		// m2: 1.0 * 1.0 * 0.5^1 * 1.0 = 0.5
		{"decay_0.5_fast", 0.5, 0.49, 0.51},
		// m2: 1.0 * 1.0 * 0.9^1 * 1.0 = 0.9
		{"decay_0.9_slow", 0.9, 0.89, 0.91},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := RetrievalConfig{
				MaxHops:             2,
				ActivationThreshold: 0.01,
				DecayFactor:         tc.decayFactor,
				MaxResults:          10,
			}
			agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

			entryPoints := map[string]float32{"m1": 1.0}
			result, _ := agent.spreadActivation(context.Background(), entryPoints)

			m2 := result["m2"].activation
			if m2 < tc.wantM2Min || m2 > tc.wantM2Max {
				t.Errorf("decay=%.1f: expected m2 activation in [%.2f, %.2f], got %.4f",
					tc.decayFactor, tc.wantM2Min, tc.wantM2Max, m2)
			}
		})
	}
}

func TestConfigMergeAlphaWeightsFTSvsEmbedding(t *testing.T) {
	tests := []struct {
		name       string
		alpha      float32
		wantMinFTS bool // if true, FTS-dominated score should be lower bound
	}{
		{"alpha_0_fts_only", 0.0, true},
		{"alpha_1_embedding_only", 1.0, false},
	}

	fts := []store.Memory{
		{ID: "m1", Salience: 0.8}, // FTS score: 0.7*1.0 + 0.3*0.8 = 0.94 (rank 1)
	}
	emb := []store.RetrievalResult{
		{Memory: store.Memory{ID: "m1"}, Score: 0.3}, // embedding score: 0.3
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MergeAlpha = tc.alpha
			cfg.DualHitBonus = 0 // isolate alpha effect
			agent := NewRetrievalAgent(&mockStore{}, &mockLLMProvider{}, cfg, testLogger())

			result := agent.mergeEntryPoints(fts, emb)

			score := result["m1"]
			// FTS score for rank 1 with salience 0.8: 0.7*1.0 + 0.3*0.8 = 0.94
			// alpha=0: score = 0*0.3 + 1*0.94 + 0 = 0.94 (FTS dominated)
			// alpha=1: score = 1*0.3 + 0*0.94 + 0 = 0.30 (embedding dominated)
			if tc.alpha == 0.0 {
				expected := float32(0.94)
				if abs32(score-expected) > 0.01 {
					t.Errorf("alpha=0: expected score ~%.2f (FTS dominated), got %.4f", expected, score)
				}
			} else {
				expected := float32(0.3)
				if abs32(score-expected) > 0.01 {
					t.Errorf("alpha=1: expected score ~%.2f (embedding dominated), got %.4f", expected, score)
				}
			}
		})
	}
}

func TestConfigDualHitBonusAddsToScore(t *testing.T) {
	fts := []store.Memory{{ID: "m1", Salience: 0.5}}
	emb := []store.RetrievalResult{{Memory: store.Memory{ID: "m1"}, Score: 0.5}}

	tests := []struct {
		name  string
		bonus float32
	}{
		{"bonus_0.0", 0.0},
		{"bonus_0.15", 0.15},
		{"bonus_0.5", 0.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.DualHitBonus = tc.bonus
			agent := NewRetrievalAgent(&mockStore{}, &mockLLMProvider{}, cfg, testLogger())

			result := agent.mergeEntryPoints(fts, emb)

			// Score = alpha*emb + (1-alpha)*fts + bonus
			// FTS rank 1 with salience 0.5: 0.7*1.0 + 0.3*0.5 = 0.85
			ftsScore := float32(0.7*1.0 + 0.3*0.5) // 0.85
			expected := cfg.MergeAlpha*0.5 + (1-cfg.MergeAlpha)*ftsScore + tc.bonus
			score := result["m1"]
			if abs32(score-expected) > 0.001 {
				t.Errorf("bonus=%.2f: expected score %.4f, got %.4f", tc.bonus, expected, score)
			}
		})
	}
}

func TestConfigSynthesisMaxTokensPassedToLLM(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		maxTokens int
	}{
		{"tokens_256", 256},
		{"tokens_2048", 2048},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedMaxTokens int

			s := &mockStore{
				searchByFullTextFunc: func(_ context.Context, _ string, _ int) ([]store.Memory, error) {
					return []store.Memory{
						{ID: "m1", Summary: "test", Salience: 0.8, LastAccessed: now},
					}, nil
				},
				searchByEmbeddingFunc: func(_ context.Context, _ []float32, _ int) ([]store.RetrievalResult, error) {
					return nil, nil
				},
				getAssociationsFunc: func(_ context.Context, _ string) ([]store.Association, error) {
					return nil, nil
				},
				getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
					return store.Memory{ID: id, Summary: "test", Salience: 0.8, LastAccessed: now}, nil
				},
			}

			p := &mockLLMProvider{
				completeFunc: func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
					capturedMaxTokens = req.MaxTokens
					return llm.CompletionResponse{Content: "synthesis result", TokensUsed: 10}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.SynthesisMaxTokens = tc.maxTokens
			agent := NewRetrievalAgent(s, p, cfg, testLogger())

			_, err := agent.Query(context.Background(), QueryRequest{
				Query:      "test",
				Synthesize: true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedMaxTokens != tc.maxTokens {
				t.Errorf("expected MaxTokens=%d in LLM request, got %d", tc.maxTokens, capturedMaxTokens)
			}
		})
	}
}

func TestConfigMaxToolCallsLimitsSynthesisTools(t *testing.T) {
	now := time.Now()

	s := &mockStore{
		searchByFullTextFunc: func(_ context.Context, _ string, _ int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Summary: "test", Salience: 0.8, LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(_ context.Context, _ []float32, _ int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(_ context.Context, _ string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			return store.Memory{ID: id, Summary: "test", Salience: 0.8, LastAccessed: now}, nil
		},
	}

	tests := []struct {
		name         string
		maxToolCalls int
		wantCalls    int // expected total Complete() calls: 1 per tool round + 1 final
	}{
		// maxToolCalls=0: first call gets no tools, must produce text immediately → 1 call
		{"max_tool_calls_0", 0, 1},
		// maxToolCalls=2: up to 2 rounds of tool use + 1 final = 3 max calls
		{"max_tool_calls_2", 2, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			callCount := 0

			p := &mockLLMProvider{
				completeFunc: func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
					callCount++
					// If tools are available, make a tool call; otherwise return text
					if len(req.Tools) > 0 {
						return llm.CompletionResponse{
							ToolCalls: []llm.ToolCall{
								{
									ID: "call1",
									Function: llm.ToolCallFunction{
										Name:      "search_memories",
										Arguments: `{"query": "test"}`,
									},
								},
							},
						}, nil
					}
					return llm.CompletionResponse{Content: "final synthesis", TokensUsed: 10}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.MaxToolCalls = tc.maxToolCalls
			agent := NewRetrievalAgent(s, p, cfg, testLogger())

			_, err := agent.Query(context.Background(), QueryRequest{
				Query:      "test",
				Synthesize: true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if callCount > tc.wantCalls {
				t.Errorf("maxToolCalls=%d: expected at most %d Complete() calls, got %d",
					tc.maxToolCalls, tc.wantCalls, callCount)
			}
		})
	}
}
