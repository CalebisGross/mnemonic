package agentutil

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float32
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0.0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1.0},
		{"empty", nil, nil, 0},
		{"different lengths", []float32{1}, []float32{1, 2}, 0},
		{"zero vector", []float32{0, 0}, []float32{1, 1}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 0.001 {
				t.Errorf("CosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAverageVectors(t *testing.T) {
	t.Run("single vector", func(t *testing.T) {
		got := AverageVectors([][]float32{{1, 2, 3}})
		if got[0] != 1 || got[1] != 2 || got[2] != 3 {
			t.Errorf("got %v, want [1 2 3]", got)
		}
	})

	t.Run("two vectors", func(t *testing.T) {
		got := AverageVectors([][]float32{{0, 0}, {2, 4}})
		if got[0] != 1 || got[1] != 2 {
			t.Errorf("got %v, want [1 2]", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := AverageVectors(nil)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
