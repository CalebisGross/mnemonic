package sqlite

import (
	"math"
	"sync"
	"testing"
)

func TestNewEmbeddingIndex(t *testing.T) {
	idx := newEmbeddingIndex()
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if idx.Len() != 0 {
		t.Errorf("expected empty index, got len %d", idx.Len())
	}
}

func TestEmbeddingIndexAddAndLen(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0, 0.0})
	idx.Add("mem-2", []float32{0.0, 1.0, 0.0})
	idx.Add("mem-3", []float32{0.0, 0.0, 1.0})

	if idx.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", idx.Len())
	}
}

func TestEmbeddingIndexAddReplace(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0, 0.0})
	idx.Add("mem-1", []float32{0.0, 1.0, 0.0}) // replace

	if idx.Len() != 1 {
		t.Errorf("expected 1 entry after replace, got %d", idx.Len())
	}

	// Search should find the updated embedding
	results := idx.Search([]float32{0.0, 1.0, 0.0}, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].id != "mem-1" {
		t.Errorf("expected mem-1, got %q", results[0].id)
	}
	if results[0].score < 0.99 {
		t.Errorf("expected score ~1.0 for identical vector, got %v", results[0].score)
	}
}

func TestEmbeddingIndexAddIgnoresEmpty(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{})
	idx.Add("mem-2", nil)

	if idx.Len() != 0 {
		t.Errorf("expected 0 entries for empty embeddings, got %d", idx.Len())
	}
}

func TestEmbeddingIndexAddIgnoresZeroVector(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{0.0, 0.0, 0.0})

	if idx.Len() != 0 {
		t.Errorf("expected 0 entries for zero vector, got %d", idx.Len())
	}
}

func TestEmbeddingIndexRemove(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0})
	idx.Add("mem-2", []float32{0.0, 1.0})
	idx.Add("mem-3", []float32{1.0, 1.0})

	idx.Remove("mem-2")

	if idx.Len() != 2 {
		t.Errorf("expected 2 entries after removal, got %d", idx.Len())
	}

	// Verify mem-2 is gone from search
	results := idx.Search([]float32{0.0, 1.0}, 10)
	for _, r := range results {
		if r.id == "mem-2" {
			t.Error("mem-2 should have been removed")
		}
	}
}

func TestEmbeddingIndexRemoveNonexistent(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0})
	idx.Remove("nonexistent") // should be a no-op

	if idx.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", idx.Len())
	}
}

func TestEmbeddingIndexRemoveLastElement(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0})
	idx.Remove("mem-1")

	if idx.Len() != 0 {
		t.Errorf("expected 0 entries, got %d", idx.Len())
	}
}

func TestEmbeddingIndexSearchIdentical(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0, 0.0})
	idx.Add("mem-2", []float32{0.0, 1.0, 0.0})

	results := idx.Search([]float32{1.0, 0.0, 0.0}, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].id != "mem-1" {
		t.Errorf("expected mem-1, got %q", results[0].id)
	}
	if results[0].score < 0.99 {
		t.Errorf("expected score ~1.0, got %v", results[0].score)
	}
}

func TestEmbeddingIndexSearchOrthogonal(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0})
	idx.Add("mem-2", []float32{0.0, 1.0})

	results := idx.Search([]float32{1.0, 0.0}, 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should be mem-1 (identical)
	if results[0].id != "mem-1" {
		t.Errorf("expected mem-1 first, got %q", results[0].id)
	}

	// Second result should be mem-2 (orthogonal, score ~0)
	if results[1].id != "mem-2" {
		t.Errorf("expected mem-2 second, got %q", results[1].id)
	}
	if math.Abs(float64(results[1].score)) > 0.01 {
		t.Errorf("expected score ~0 for orthogonal vectors, got %v", results[1].score)
	}
}

func TestEmbeddingIndexSearchLimitRespected(t *testing.T) {
	idx := newEmbeddingIndex()

	for i := 0; i < 20; i++ {
		emb := make([]float32, 3)
		emb[i%3] = 1.0
		idx.Add("mem-"+string(rune('a'+i)), emb)
	}

	results := idx.Search([]float32{1.0, 0.0, 0.0}, 5)
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestEmbeddingIndexSearchEmpty(t *testing.T) {
	idx := newEmbeddingIndex()

	results := idx.Search([]float32{1.0, 0.0}, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

func TestEmbeddingIndexSearchEmptyQuery(t *testing.T) {
	idx := newEmbeddingIndex()
	idx.Add("mem-1", []float32{1.0, 0.0})

	results := idx.Search([]float32{}, 5)
	if results != nil {
		t.Errorf("expected nil results for empty query, got %v", results)
	}
}

func TestEmbeddingIndexSearchZeroQuery(t *testing.T) {
	idx := newEmbeddingIndex()
	idx.Add("mem-1", []float32{1.0, 0.0})

	results := idx.Search([]float32{0.0, 0.0}, 5)
	if results != nil {
		t.Errorf("expected nil results for zero query vector, got %v", results)
	}
}

func TestEmbeddingIndexSearchDimensionMismatch(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("mem-1", []float32{1.0, 0.0, 0.0}) // 3D
	idx.Add("mem-2", []float32{1.0, 0.0})      // 2D

	results := idx.Search([]float32{1.0, 0.0, 0.0}, 10) // 3D query
	if len(results) != 1 {
		t.Fatalf("expected 1 result (only matching dimension), got %d", len(results))
	}
	if results[0].id != "mem-1" {
		t.Errorf("expected mem-1, got %q", results[0].id)
	}
}

func TestEmbeddingIndexSearchSorted(t *testing.T) {
	idx := newEmbeddingIndex()

	// Add vectors at varying angles to the query
	idx.Add("far", []float32{0.0, 1.0, 0.0})       // orthogonal
	idx.Add("close", []float32{0.9, 0.1, 0.0})     // close
	idx.Add("identical", []float32{1.0, 0.0, 0.0}) // identical
	idx.Add("moderate", []float32{0.5, 0.5, 0.0})  // moderate

	results := idx.Search([]float32{1.0, 0.0, 0.0}, 4)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Verify descending score order
	for i := 1; i < len(results); i++ {
		if results[i].score > results[i-1].score {
			t.Errorf("results not sorted: score[%d]=%v > score[%d]=%v",
				i, results[i].score, i-1, results[i-1].score)
		}
	}

	// First should be identical
	if results[0].id != "identical" {
		t.Errorf("expected 'identical' first, got %q", results[0].id)
	}
}

func TestEmbeddingIndexConcurrentAccess(t *testing.T) {
	idx := newEmbeddingIndex()

	var wg sync.WaitGroup
	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			emb := []float32{float32(i), float32(i + 1), float32(i + 2)}
			idx.Add("mem-"+string(rune(i)), emb)
		}(i)
	}

	// Concurrent reads while writing
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx.Search([]float32{1.0, 2.0, 3.0}, 5)
		}()
	}

	// Concurrent removes while writing
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			idx.Remove("mem-" + string(rune(i)))
		}(i)
	}

	wg.Wait()

	// Just verify it didn't panic or deadlock
	_ = idx.Len()
}

func TestL2Norm(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected float32
	}{
		{"unit x", []float32{1, 0, 0}, 1.0},
		{"unit y", []float32{0, 1, 0}, 1.0},
		{"3-4-5", []float32{3, 4}, 5.0},
		{"zero", []float32{0, 0, 0}, 0.0},
		{"empty", []float32{}, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := l2norm(tt.input)
			if math.Abs(float64(got-tt.expected)) > 0.001 {
				t.Errorf("l2norm(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEmbeddingIndexRemoveMiddle(t *testing.T) {
	idx := newEmbeddingIndex()

	idx.Add("a", []float32{1, 0})
	idx.Add("b", []float32{0, 1})
	idx.Add("c", []float32{1, 1})

	// Remove the middle element — tests the swap-with-last logic
	idx.Remove("b")

	if idx.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", idx.Len())
	}

	// Both remaining elements should be searchable
	results := idx.Search([]float32{1, 0}, 10)
	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.id] = true
	}
	if !ids["a"] {
		t.Error("expected 'a' in results")
	}
	if !ids["c"] {
		t.Error("expected 'c' in results")
	}
	if ids["b"] {
		t.Error("'b' should have been removed")
	}
}
