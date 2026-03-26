//go:build llamacpp

package llamacpp

import (
	"context"
	"os"
	"testing"

	"github.com/appsprout-dev/mnemonic/internal/llm"
)

func findModel() string {
	// Check common locations
	paths := []string{
		"../../../models/felix-encoder-v1.gguf",
		"../../../models/felix-base-test.gguf",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func TestBackendLoadAndComplete(t *testing.T) {
	modelPath := findModel()
	if modelPath == "" {
		t.Skip("no GGUF model found in models/")
	}

	backend := NewBackend()
	if backend == nil {
		t.Fatal("NewBackend returned nil")
	}
	defer func() {
		if err := backend.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	err := backend.LoadModel(modelPath, llm.BackendOptions{
		ContextSize: 512,
		GPULayers:   0,
		Threads:     4,
		BatchSize:   256,
	})
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	// Test completion
	ctx := context.Background()
	resp, err := backend.Complete(ctx, llm.BackendCompletionRequest{
		Prompt:      "The capital of France is",
		MaxTokens:   20,
		Temperature: 0.3,
		TopP:        0.9,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	t.Logf("Completion: %q", resp.Text)
	t.Logf("Prompt tokens: %d, Completion tokens: %d", resp.PromptTokens, resp.CompletionTokens)

	if resp.PromptTokens == 0 {
		t.Error("expected non-zero prompt tokens")
	}
	if resp.CompletionTokens == 0 {
		t.Error("expected non-zero completion tokens")
	}
	if resp.Text == "" {
		t.Error("expected non-empty completion text")
	}
}

func TestBackendEmbed(t *testing.T) {
	modelPath := findModel()
	if modelPath == "" {
		t.Skip("no GGUF model found in models/")
	}

	backend := NewBackend()
	defer func() { _ = backend.Close() }()

	err := backend.LoadModel(modelPath, llm.BackendOptions{
		ContextSize: 512,
		GPULayers:   0,
		Threads:     4,
		BatchSize:   256,
	})
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	ctx := context.Background()
	embedding, err := backend.Embed(ctx, "Hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	t.Logf("Embedding dimensions: %d", len(embedding))

	if len(embedding) != 512 {
		t.Errorf("expected 512 dimensions, got %d", len(embedding))
	}

	// Check L2 norm is ~1.0 (we normalize in the bridge)
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	t.Logf("L2 norm: %.4f", norm)
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected L2 norm ~1.0, got %.4f", norm)
	}
}
