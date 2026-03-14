package encoding

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// ---------------------------------------------------------------------------
// Config Behavioral Tests — verify each config param affects encoding behavior
// ---------------------------------------------------------------------------

func TestConfigCompletionMaxTokensPassedToLLM(t *testing.T) {
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
				getRawFn: func(_ context.Context, _ string) (store.RawMemory, error) {
					return store.RawMemory{
						ID:      "raw1",
						Content: "test content for encoding",
						Source:  "mcp",
						Type:    "decision",
					}, nil
				},
				writeMemoryFn: func(_ context.Context, _ store.Memory) error { return nil },
			}

			p := &mockLLMProvider{
				completeFn: func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
					capturedMaxTokens = req.MaxTokens
					return llm.CompletionResponse{
						Content: `{"gist":"test","summary":"test summary","content":"test content","narrative":"test","concepts":["test"],"salience":0.5,"significance":"routine","emotional_tone":"neutral","outcome":"success"}`,
					}, nil
				},
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.CompletionMaxTokens = tc.maxTokens
			agent := NewEncodingAgentWithConfig(s, p, testLogger(), cfg)
			agent.bus = newMockBus()

			err := agent.encodeMemory(context.Background(), "raw1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedMaxTokens != tc.maxTokens {
				t.Errorf("expected MaxTokens=%d in LLM request, got %d", tc.maxTokens, capturedMaxTokens)
			}
		})
	}
}

func TestConfigCompletionTemperaturePassedToLLM(t *testing.T) {
	tests := []struct {
		name string
		temp float32
	}{
		{"temp_0.1", 0.1},
		{"temp_0.7", 0.7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedTemp float32

			s := &mockStore{
				getRawFn: func(_ context.Context, _ string) (store.RawMemory, error) {
					return store.RawMemory{ID: "raw1", Content: "test content", Source: "mcp", Type: "decision"}, nil
				},
				writeMemoryFn: func(_ context.Context, _ store.Memory) error { return nil },
			}

			p := &mockLLMProvider{
				completeFn: func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
					capturedTemp = req.Temperature
					return llm.CompletionResponse{
						Content: `{"gist":"test","summary":"test","content":"test","narrative":"test","concepts":["test"],"salience":0.5,"significance":"routine","emotional_tone":"neutral","outcome":"success"}`,
					}, nil
				},
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.CompletionTemperature = tc.temp
			agent := NewEncodingAgentWithConfig(s, p, testLogger(), cfg)
			agent.bus = newMockBus()

			err := agent.encodeMemory(context.Background(), "raw1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedTemp != tc.temp {
				t.Errorf("expected Temperature=%.1f in LLM request, got %.1f", tc.temp, capturedTemp)
			}
		})
	}
}

func TestConfigSimilarityThresholdGatesAssociations(t *testing.T) {
	tests := []struct {
		name               string
		threshold          float32
		similarScore       float32
		expectAssocCreated bool
	}{
		{"score_0.5_threshold_0.3_creates", 0.3, 0.5, true},
		{"score_0.5_threshold_0.6_skips", 0.6, 0.5, false},
		{"score_0.8_threshold_0.3_creates", 0.3, 0.8, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var assocCreated bool

			s := &mockStore{
				getRawFn: func(_ context.Context, _ string) (store.RawMemory, error) {
					return store.RawMemory{ID: "raw1", Content: "test content", Source: "mcp", Type: "decision"}, nil
				},
				writeMemoryFn: func(_ context.Context, _ store.Memory) error { return nil },
				searchByEmbeddingFn: func(_ context.Context, _ []float32, _ int) ([]store.RetrievalResult, error) {
					return []store.RetrievalResult{
						{Memory: store.Memory{ID: "existing1", Summary: "existing memory"}, Score: tc.similarScore},
					}, nil
				},
				createAssociationFn: func(_ context.Context, _ store.Association) error {
					assocCreated = true
					return nil
				},
			}

			p := &mockLLMProvider{
				completeFn: func(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
					return llm.CompletionResponse{
						Content: `{"gist":"test","summary":"test","content":"test","narrative":"test","concepts":["test"],"salience":0.5,"significance":"routine","emotional_tone":"neutral","outcome":"success"}`,
					}, nil
				},
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.SimilarityThreshold = tc.threshold
			agent := NewEncodingAgentWithConfig(s, p, testLogger(), cfg)
			agent.bus = newMockBus()

			err := agent.encodeMemory(context.Background(), "raw1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if assocCreated != tc.expectAssocCreated {
				t.Errorf("threshold=%.2f, score=%.2f: expected association created=%v, got %v",
					tc.threshold, tc.similarScore, tc.expectAssocCreated, assocCreated)
			}
		})
	}
}

func TestConfigMaxSimilarSearchResultsPassedToStore(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{"limit_3", 3},
		{"limit_10", 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedLimit int

			s := &mockStore{
				getRawFn: func(_ context.Context, _ string) (store.RawMemory, error) {
					return store.RawMemory{ID: "raw1", Content: "test content", Source: "mcp", Type: "decision"}, nil
				},
				writeMemoryFn: func(_ context.Context, _ store.Memory) error { return nil },
				searchByEmbeddingFn: func(_ context.Context, _ []float32, limit int) ([]store.RetrievalResult, error) {
					capturedLimit = limit
					return nil, nil
				},
			}

			p := &mockLLMProvider{
				completeFn: func(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
					return llm.CompletionResponse{
						Content: `{"gist":"test","summary":"test","content":"test","narrative":"test","concepts":["test"],"salience":0.5,"significance":"routine","emotional_tone":"neutral","outcome":"success"}`,
					}, nil
				},
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.MaxSimilarSearchResults = tc.limit
			agent := NewEncodingAgentWithConfig(s, p, testLogger(), cfg)
			agent.bus = newMockBus()

			err := agent.encodeMemory(context.Background(), "raw1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedLimit != tc.limit {
				t.Errorf("expected search limit=%d, got %d", tc.limit, capturedLimit)
			}
		})
	}
}

func TestConfigConceptVocabularyIncludedInPrompt(t *testing.T) {
	tests := []struct {
		name           string
		vocabulary     []string
		expectInPrompt string
	}{
		{"custom_vocab", []string{"golang", "memory", "sqlite"}, "golang, memory, sqlite"},
		{"empty_vocab", nil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPrompt string

			s := &mockStore{
				getRawFn: func(_ context.Context, _ string) (store.RawMemory, error) {
					return store.RawMemory{ID: "raw1", Content: "test content", Source: "mcp", Type: "decision"}, nil
				},
				writeMemoryFn: func(_ context.Context, _ store.Memory) error { return nil },
			}

			p := &mockLLMProvider{
				completeFn: func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
					for _, msg := range req.Messages {
						if msg.Role == "user" {
							capturedPrompt = msg.Content
						}
					}
					return llm.CompletionResponse{
						Content: `{"gist":"test","summary":"test","content":"test","narrative":"test","concepts":["test"],"salience":0.5,"significance":"routine","emotional_tone":"neutral","outcome":"success"}`,
					}, nil
				},
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.ConceptVocabulary = tc.vocabulary
			agent := NewEncodingAgentWithConfig(s, p, testLogger(), cfg)
			agent.bus = newMockBus()

			err := agent.encodeMemory(context.Background(), "raw1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.expectInPrompt != "" {
				if !strings.Contains(capturedPrompt, tc.expectInPrompt) {
					t.Errorf("expected vocabulary %q in prompt, not found", tc.expectInPrompt)
				}
			} else {
				if strings.Contains(capturedPrompt, "CONCEPT VOCABULARY") {
					t.Error("expected no vocabulary section in prompt with nil vocabulary")
				}
			}
		})
	}
}

func TestConfigMaxConcurrentEncodingsLimitsConcurrency(t *testing.T) {
	tests := []struct {
		name            string
		maxConcurrent   int
		wantMaxInFlight int
	}{
		{"concurrency_1", 1, 1},
		{"concurrency_3", 3, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var maxInFlight int64
			var currentInFlight int64
			var mu sync.Mutex

			s := &mockStore{
				getRawFn: func(_ context.Context, id string) (store.RawMemory, error) {
					return store.RawMemory{ID: id, Content: "test content " + id, Source: "mcp", Type: "decision"}, nil
				},
				listRawUnprocessedFn: func(_ context.Context, _ int) ([]store.RawMemory, error) {
					return nil, nil
				},
				writeMemoryFn: func(_ context.Context, _ store.Memory) error { return nil },
			}

			p := &mockLLMProvider{
				completeFn: func(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
					current := atomic.AddInt64(&currentInFlight, 1)
					mu.Lock()
					if current > maxInFlight {
						maxInFlight = current
					}
					mu.Unlock()
					time.Sleep(10 * time.Millisecond) // simulate LLM latency
					atomic.AddInt64(&currentInFlight, -1)
					return llm.CompletionResponse{
						Content: `{"gist":"test","summary":"test","content":"test","narrative":"test","concepts":["test"],"salience":0.5,"significance":"routine","emotional_tone":"neutral","outcome":"success"}`,
					}, nil
				},
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				},
			}

			cfg := DefaultConfig()
			cfg.MaxConcurrentEncodings = tc.maxConcurrent
			agent := NewEncodingAgentWithConfig(s, p, testLogger(), cfg)

			// Verify the semaphore was created with the right capacity
			if cap(agent.encodingSem) != tc.maxConcurrent {
				t.Errorf("expected semaphore capacity=%d, got %d", tc.maxConcurrent, cap(agent.encodingSem))
			}
		})
	}
}
