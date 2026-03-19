package retrieval

import (
	"log/slog"
	"math"
	"os"
	"testing"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float32
	}{
		{
			name: "identical vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "similar vectors",
			a:    []float32{1, 1, 0},
			b:    []float32{1, 0.9, 0.1},
			want: 0.992, // approximately
		},
		{
			name: "empty vectors",
			a:    []float32{},
			b:    []float32{},
			want: 0.0,
		},
		{
			name: "mismatched lengths",
			a:    []float32{1, 0},
			b:    []float32{1, 0, 0},
			want: 0.0,
		},
		{
			name: "zero magnitude",
			a:    []float32{0, 0, 0},
			b:    []float32{1, 0, 0},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 0.01 {
				t.Errorf("cosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

// makeResult creates a RetrievalResult with the given score and embedding.
func makeResult(id string, score float32, embedding []float32) store.RetrievalResult {
	return store.RetrievalResult{
		Memory: store.Memory{
			ID:        id,
			Embedding: embedding,
		},
		Score: score,
	}
}

func newTestRetrievalAgent(cfg RetrievalConfig) *RetrievalAgent {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &RetrievalAgent{
		config: cfg,
		log:    log,
	}
}

func TestApplyDiversityFilter_DropsNearDuplicates(t *testing.T) {
	ra := newTestRetrievalAgent(RetrievalConfig{
		DiversityLambda:    0.7,
		DiversityThreshold: 0.85,
	})

	// 5 near-identical embeddings (score descending) + 2 diverse ones
	dup := []float32{1, 0, 0, 0}
	diverse1 := []float32{0, 1, 0, 0}
	diverse2 := []float32{0, 0, 1, 0}

	results := []store.RetrievalResult{
		makeResult("dup1", 1.0, dup),
		makeResult("dup2", 0.95, dup),
		makeResult("dup3", 0.90, dup),
		makeResult("diverse1", 0.80, diverse1),
		makeResult("dup4", 0.75, dup),
		makeResult("diverse2", 0.70, diverse2),
		makeResult("dup5", 0.65, dup),
	}

	filtered := ra.applyDiversityFilter(results)

	// Should keep dup1 (first pick), diverse1, diverse2 — and drop the other dups
	ids := make(map[string]bool)
	for _, r := range filtered {
		ids[r.Memory.ID] = true
	}

	if !ids["dup1"] {
		t.Error("expected dup1 (highest scored) to be kept")
	}
	if !ids["diverse1"] {
		t.Error("expected diverse1 to be kept")
	}
	if !ids["diverse2"] {
		t.Error("expected diverse2 to be kept")
	}

	// At most one of the duplicate cluster should survive
	dupCount := 0
	for _, id := range []string{"dup1", "dup2", "dup3", "dup4", "dup5"} {
		if ids[id] {
			dupCount++
		}
	}
	if dupCount > 1 {
		t.Errorf("expected at most 1 duplicate to survive, got %d", dupCount)
	}
}

func TestApplyDiversityFilter_PureRelevance(t *testing.T) {
	// Lambda=1.0 means no diversity penalty — order should be preserved
	ra := newTestRetrievalAgent(RetrievalConfig{
		DiversityLambda:    1.0,
		DiversityThreshold: 0.85,
	})

	dup := []float32{1, 0, 0}
	results := []store.RetrievalResult{
		makeResult("a", 1.0, dup),
		makeResult("b", 0.9, dup),
		makeResult("c", 0.8, dup),
	}

	filtered := ra.applyDiversityFilter(results)

	// Near-duplicates above threshold are still dropped even with lambda=1.0
	// because the threshold is a hard gate, not part of the MMR score
	if len(filtered) > 1 {
		t.Errorf("expected near-duplicates to be dropped, got %d results", len(filtered))
	}
}

func TestApplyDiversityFilter_EmptyAndSingle(t *testing.T) {
	ra := newTestRetrievalAgent(RetrievalConfig{
		DiversityLambda:    0.7,
		DiversityThreshold: 0.85,
	})

	// Empty
	got := ra.applyDiversityFilter(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(got))
	}

	// Single
	single := []store.RetrievalResult{makeResult("only", 1.0, []float32{1, 0})}
	got = ra.applyDiversityFilter(single)
	if len(got) != 1 || got[0].Memory.ID != "only" {
		t.Error("single result should pass through unchanged")
	}
}

func TestApplyDiversityFilter_NoEmbeddings(t *testing.T) {
	ra := newTestRetrievalAgent(RetrievalConfig{
		DiversityLambda:    0.7,
		DiversityThreshold: 0.85,
	})

	// Results with no embeddings should still be returned (can't compare for diversity)
	results := []store.RetrievalResult{
		makeResult("a", 1.0, nil),
		makeResult("b", 0.9, nil),
		makeResult("c", 0.8, nil),
	}

	filtered := ra.applyDiversityFilter(results)
	if len(filtered) != 3 {
		t.Errorf("expected all 3 results (no embeddings to compare), got %d", len(filtered))
	}
}
