package agentutil

import "math"

// CosineSimilarity computes cosine similarity between two embedding vectors.
// Returns 0 if vectors are different lengths, empty, or have zero magnitude.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// AverageVectors computes the element-wise average of a set of float32 vectors.
// All vectors must have the same dimension; mismatched vectors are skipped.
// Returns nil if the input is empty.
func AverageVectors(vecs [][]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	avg := make([]float32, dim)
	for _, v := range vecs {
		if len(v) != dim {
			continue
		}
		for i, val := range v {
			avg[i] += val
		}
	}
	n := float32(len(vecs))
	for i := range avg {
		avg[i] /= n
	}
	return avg
}
