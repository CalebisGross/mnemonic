//go:build llamacpp

package llamacpp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/appsprout-dev/mnemonic/internal/llm"
)

func findModel() string {
	// Prefer Q8_0 quantized models (smaller, equivalent quality), latest version first
	paths := []string{
		"../../../models/felix-encoder-v2-q8_0.gguf",
		"../../../models/felix-encoder-v2.gguf",
		"../../../models/felix-encoder-v1-q8_0.gguf",
		"../../../models/felix-encoder-v1.gguf",
		"../../../models/felix-base-test-q8_0.gguf",
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
	t.Logf("Mean prob: %.4f, Min prob: %.4f", resp.MeanProb, resp.MinProb)

	if resp.PromptTokens == 0 {
		t.Error("expected non-zero prompt tokens")
	}
	if resp.CompletionTokens == 0 {
		t.Error("expected non-zero completion tokens")
	}
	if resp.Text == "" {
		t.Error("expected non-empty completion text")
	}
	if resp.MeanProb == 0 {
		t.Error("expected non-zero mean probability")
	}
}

func TestBackendGBNFGrammar(t *testing.T) {
	modelPath := findModel()
	if modelPath == "" {
		t.Skip("no GGUF model found in models/")
	}

	backend := NewBackend()
	defer func() { _ = backend.Close() }()

	err := backend.LoadModel(modelPath, llm.BackendOptions{
		ContextSize: 2048,
		GPULayers:   0,
		Threads:     4,
		BatchSize:   512,
	})
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	ctx := context.Background()

	// Test with the encoding-specific GBNF grammar
	prompt := `<|system|>
You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize, never chat. Just fill in the JSON fields based on the event data.
<|user|>
SOURCE: mcp
TYPE: general
CONTENT: User decided to use SQLite instead of Postgres for the mnemonic daemon because it's simpler and doesn't require a separate process.
<|assistant|>
`

	resp, err := backend.Complete(ctx, llm.BackendCompletionRequest{
		Prompt:      prompt,
		MaxTokens:   512,
		Temperature: 0.3,
		TopP:        0.9,
		Grammar:     llm.GBNFEncodingResponse,
	})
	if err != nil {
		t.Fatalf("Complete with GBNF grammar: %v", err)
	}

	t.Logf("Grammar-constrained output (%d tokens): %s", resp.CompletionTokens, resp.Text)
	t.Logf("Grammar completion confidence — Mean prob: %.4f, Min prob: %.6f", resp.MeanProb, resp.MinProb)

	// Verify the output is valid JSON with expected fields
	if resp.Text == "" {
		t.Fatal("expected non-empty completion text with grammar")
	}
	if resp.Text[0] != '{' {
		t.Errorf("expected JSON object starting with '{', got %q", resp.Text[:min(20, len(resp.Text))])
	}

	// Parse and verify required fields exist
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Text), &result); err != nil {
		t.Fatalf("grammar output is not valid JSON: %v\nOutput: %s", err, resp.Text)
	}

	requiredFields := []string{"gist", "summary", "content", "narrative", "concepts", "structured_concepts", "significance", "emotional_tone", "outcome", "salience"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("missing required field %q in grammar output", field)
		}
	}

	// Verify structured_concepts has its sub-fields
	if sc, ok := result["structured_concepts"].(map[string]interface{}); ok {
		for _, subfield := range []string{"topics", "entities", "actions", "causality"} {
			if _, ok := sc[subfield]; !ok {
				t.Errorf("missing structured_concepts.%s in grammar output", subfield)
			}
		}
	} else {
		t.Error("structured_concepts is not an object")
	}

	// Verify salience is a number
	if _, ok := result["salience"].(float64); !ok {
		t.Errorf("salience should be a number, got %T", result["salience"])
	}
}

func TestBackendContextWindowTruncation(t *testing.T) {
	modelPath := findModel()
	if modelPath == "" {
		t.Skip("no GGUF model found in models/")
	}

	backend := NewBackend()
	defer func() { _ = backend.Close() }()

	// Use a small context window to make truncation easy to trigger
	err := backend.LoadModel(modelPath, llm.BackendOptions{
		ContextSize: 256,
		GPULayers:   0,
		Threads:     4,
		BatchSize:   128,
	})
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	ctx := context.Background()

	// Generate a prompt that will definitely exceed 256 tokens
	longPrompt := "The following is a very long document about many topics. "
	for i := 0; i < 200; i++ {
		longPrompt += "This is sentence number " + string(rune('A'+i%26)) + " and it contains various words to fill up the token budget. "
	}

	resp, err := backend.Complete(ctx, llm.BackendCompletionRequest{
		Prompt:      longPrompt,
		MaxTokens:   32,
		Temperature: 0.3,
		TopP:        0.9,
	})
	if err != nil {
		t.Fatalf("Complete with oversized prompt should not crash: %v", err)
	}

	t.Logf("Truncated prompt tokens: %d (context: 256, max_tokens: 32, max_prompt: 224)", resp.PromptTokens)
	t.Logf("Completion tokens: %d", resp.CompletionTokens)
	t.Logf("Output: %q", resp.Text)

	// Prompt should have been truncated to fit: n_ctx(256) - max_tokens(32) = 224 max prompt tokens
	if resp.PromptTokens > 224 {
		t.Errorf("prompt tokens %d should be <= 224 (context 256 - max_tokens 32)", resp.PromptTokens)
	}
	if resp.PromptTokens == 0 {
		t.Error("expected non-zero prompt tokens after truncation")
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
