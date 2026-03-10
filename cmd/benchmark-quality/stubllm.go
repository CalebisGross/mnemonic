package main

import (
	"context"
	"hash/fnv"
	"math"

	"github.com/appsprout/mnemonic/internal/llm"
)

const stubEmbDims = 64

// stubProvider implements llm.Provider with deterministic synthetic embeddings.
// Complete calls return empty responses (LLM-gated features are skipped).
type stubProvider struct{}

func (s *stubProvider) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: "", StopReason: "stub"}, nil
}

func (s *stubProvider) Embed(_ context.Context, text string) ([]float32, error) {
	return hashEmbedding(text, stubEmbDims), nil
}

func (s *stubProvider) BatchEmbed(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		results[i] = hashEmbedding(t, stubEmbDims)
	}
	return results, nil
}

func (s *stubProvider) Health(_ context.Context) error {
	return nil
}

func (s *stubProvider) ModelInfo(_ context.Context) (llm.ModelMetadata, error) {
	return llm.ModelMetadata{Name: "stub-benchmark"}, nil
}

// hashEmbedding creates a deterministic unit vector from text using FNV hash.
// Similar texts won't produce similar vectors (unlike real embeddings), but
// this ensures the retrieval pipeline doesn't crash on nil.
func hashEmbedding(text string, dims int) []float32 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	seed := h.Sum64()

	emb := make([]float32, dims)
	for i := range emb {
		// Simple deterministic pseudo-random based on seed and position.
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		emb[i] = float32(math.Sin(float64(seed)))
	}

	// Normalize to unit vector.
	var norm float64
	for _, v := range emb {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range emb {
			emb[i] = float32(float64(emb[i]) / norm)
		}
	}
	return emb
}
