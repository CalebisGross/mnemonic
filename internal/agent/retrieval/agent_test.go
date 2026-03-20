package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/store/storetest"
)

// ---------------------------------------------------------------------------
// Mock LLM Provider
// ---------------------------------------------------------------------------

// mockLLMProvider implements llm.Provider for testing.
type mockLLMProvider struct {
	completeFunc   func(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
	embedFunc      func(ctx context.Context, text string) ([]float32, error)
	batchEmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
	healthFunc     func(ctx context.Context) error
	modelInfoFunc  func(ctx context.Context) (llm.ModelMetadata, error)

	// Track calls for assertions
	completeCalls int
	embedCalls    int
}

func (m *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	m.completeCalls++
	if m.completeFunc != nil {
		return m.completeFunc(ctx, req)
	}
	return llm.CompletionResponse{Content: "synthesized response", TokensUsed: 10}, nil
}

func (m *mockLLMProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	m.embedCalls++
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

func (m *mockLLMProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.batchEmbedFunc != nil {
		return m.batchEmbedFunc(ctx, texts)
	}
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (m *mockLLMProvider) Health(ctx context.Context) error {
	if m.healthFunc != nil {
		return m.healthFunc(ctx)
	}
	return nil
}

func (m *mockLLMProvider) ModelInfo(ctx context.Context) (llm.ModelMetadata, error) {
	if m.modelInfoFunc != nil {
		return m.modelInfoFunc(ctx)
	}
	return llm.ModelMetadata{Name: "mock-model", ContextWindow: 4096, MaxTokens: 512}, nil
}

// ---------------------------------------------------------------------------
// Mock Store
// ---------------------------------------------------------------------------

// mockStore implements store.Store with configurable return values for testing.
type mockStore struct {
	storetest.MockStore

	// Configurable function fields for methods used by the retrieval agent.
	searchByFullTextFunc        func(ctx context.Context, query string, limit int) ([]store.Memory, error)
	searchByEmbeddingFunc       func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error)
	getAssociationsFunc         func(ctx context.Context, memoryID string) ([]store.Association, error)
	getMemoryFunc               func(ctx context.Context, id string) (store.Memory, error)
	incrementAccessFunc         func(ctx context.Context, id string) error
	getMemoryAttrsFunc          func(ctx context.Context, memoryID string) (store.MemoryAttributes, error)
	getMemoryFeedbackScoresFunc func(ctx context.Context, memoryIDs []string) (map[string]float32, error)

	// Call tracking
	incrementAccessCalls []string
}

func (m *mockStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	if m.getMemoryFunc != nil {
		return m.getMemoryFunc(ctx, id)
	}
	return store.Memory{ID: id, Summary: "memory " + id, Salience: 0.5, LastAccessed: time.Now()}, nil
}
func (m *mockStore) IncrementAccess(ctx context.Context, id string) error {
	m.incrementAccessCalls = append(m.incrementAccessCalls, id)
	if m.incrementAccessFunc != nil {
		return m.incrementAccessFunc(ctx, id)
	}
	return nil
}
func (m *mockStore) SearchByFullText(ctx context.Context, query string, limit int) ([]store.Memory, error) {
	if m.searchByFullTextFunc != nil {
		return m.searchByFullTextFunc(ctx, query, limit)
	}
	return nil, nil
}
func (m *mockStore) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
	if m.searchByEmbeddingFunc != nil {
		return m.searchByEmbeddingFunc(ctx, embedding, limit)
	}
	return nil, nil
}
func (m *mockStore) GetAssociations(ctx context.Context, memoryID string) ([]store.Association, error) {
	if m.getAssociationsFunc != nil {
		return m.getAssociationsFunc(ctx, memoryID)
	}
	return nil, nil
}
func (m *mockStore) GetOpenEpisode(ctx context.Context) (store.Episode, error) {
	return store.Episode{}, fmt.Errorf("no open episode")
}
func (m *mockStore) GetMemoryAttributes(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
	if m.getMemoryAttrsFunc != nil {
		return m.getMemoryAttrsFunc(ctx, memoryID)
	}
	return store.MemoryAttributes{}, fmt.Errorf("no attributes")
}
func (m *mockStore) GetMemoryFeedbackScores(ctx context.Context, memoryIDs []string) (map[string]float32, error) {
	if m.getMemoryFeedbackScoresFunc != nil {
		return m.getMemoryFeedbackScoresFunc(ctx, memoryIDs)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewRetrievalAgent(t *testing.T) {
	s := &mockStore{}
	p := &mockLLMProvider{}
	cfg := DefaultConfig()
	log := testLogger()

	agent := NewRetrievalAgent(s, p, cfg, log)

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.store != s {
		t.Error("expected store to be set")
	}
	if agent.llm != p {
		t.Error("expected llm provider to be set")
	}
	if agent.config.MaxHops != cfg.MaxHops {
		t.Errorf("expected MaxHops %d, got %d", cfg.MaxHops, agent.config.MaxHops)
	}
	if agent.config.ActivationThreshold != cfg.ActivationThreshold {
		t.Errorf("expected ActivationThreshold %f, got %f", cfg.ActivationThreshold, agent.config.ActivationThreshold)
	}
	if agent.config.DecayFactor != cfg.DecayFactor {
		t.Errorf("expected DecayFactor %f, got %f", cfg.DecayFactor, agent.config.DecayFactor)
	}
	if agent.config.MaxResults != cfg.MaxResults {
		t.Errorf("expected MaxResults %d, got %d", cfg.MaxResults, agent.config.MaxResults)
	}
	if agent.stats == nil {
		t.Error("expected stats to be initialized")
	}
	if agent.stats.TotalQueries != 0 {
		t.Errorf("expected initial TotalQueries 0, got %d", agent.stats.TotalQueries)
	}
}

func TestParseQueryConcepts(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "simple query with stop words",
			query:    "the go concurrency pattern",
			expected: []string{"concurrency", "pattern"},
		},
		{
			name:     "query with punctuation",
			query:    "what is the error in auth.go?",
			expected: []string{"error", "auth.go"},
		},
		{
			name:     "query with all stop words",
			query:    "the a an and or but",
			expected: []string{},
		},
		{
			name:     "empty query",
			query:    "",
			expected: []string{},
		},
		{
			name:     "short tokens filtered",
			query:    "go is ok to do it",
			expected: []string{},
		},
		{
			name:     "mixed case normalized",
			query:    "Docker KUBERNETES Helm",
			expected: []string{"docker", "kubernetes", "helm"},
		},
		{
			name:     "punctuation stripped from edges",
			query:    "debugging, testing, and refactoring!",
			expected: []string{"debugging", "testing", "refactoring"},
		},
		{
			name:     "preserves meaningful short-ish words",
			query:    "sql database query optimization",
			expected: []string{"sql", "database", "query", "optimization"},
		},
		{
			name:     "query with quotes and semicolons",
			query:    `"memory"; "retrieval"`,
			expected: []string{"memory", "retrieval"},
		},
		{
			name:     "single meaningful word",
			query:    "concurrency",
			expected: []string{"concurrency"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseQueryConcepts(tc.query)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d concepts %v, got %d concepts %v", len(tc.expected), tc.expected, len(got), got)
			}
			for i, want := range tc.expected {
				if got[i] != want {
					t.Errorf("concept[%d]: expected %q, got %q", i, want, got[i])
				}
			}
		})
	}
}

func TestGetAssociationTypeWeight(t *testing.T) {
	tests := []struct {
		relationType string
		expected     float32
	}{
		{"caused_by", 1.2},
		{"part_of", 1.15},
		{"reinforces", 1.1},
		{"temporal", 1.1},
		{"similar", 1.0},
		{"contradicts", 0.8},
		{"unknown_type", 1.0},
		{"", 1.0},
	}

	for _, tc := range tests {
		name := tc.relationType
		if name == "" {
			name = "empty_string"
		}
		t.Run(name, func(t *testing.T) {
			got := getAssociationTypeWeight(tc.relationType)
			if got != tc.expected {
				t.Errorf("getAssociationTypeWeight(%q) = %f, want %f", tc.relationType, got, tc.expected)
			}
		})
	}
}

func TestMergeEntryPoints(t *testing.T) {
	agent := NewRetrievalAgent(&mockStore{}, &mockLLMProvider{}, DefaultConfig(), testLogger())

	t.Run("FTS only", func(t *testing.T) {
		fts := []store.Memory{
			{ID: "m1", Salience: 0.9},
			{ID: "m2", Salience: 0.6},
		}
		result := agent.mergeEntryPoints(fts, nil)

		if len(result) != 2 {
			t.Fatalf("expected 2 entry points, got %d", len(result))
		}
		// FTS score = 0.7 * (1/(rank)) + 0.3 * salience
		expectedM1 := float32(0.7*1.0 + 0.3*0.9) // rank 1: 0.97
		if abs32(result["m1"]-expectedM1) > 0.001 {
			t.Errorf("expected m1 score ~%.3f, got %f", expectedM1, result["m1"])
		}
		expectedM2 := float32(0.7*0.5 + 0.3*0.6) // rank 2: 0.53
		if abs32(result["m2"]-expectedM2) > 0.001 {
			t.Errorf("expected m2 score ~%.3f, got %f", expectedM2, result["m2"])
		}
	})

	t.Run("Embedding only", func(t *testing.T) {
		emb := []store.RetrievalResult{
			{Memory: store.Memory{ID: "m3"}, Score: 0.85},
			{Memory: store.Memory{ID: "m4"}, Score: 0.72},
		}
		result := agent.mergeEntryPoints(nil, emb)

		if len(result) != 2 {
			t.Fatalf("expected 2 entry points, got %d", len(result))
		}
		if result["m3"] != 0.85 {
			t.Errorf("expected m3 score 0.85, got %f", result["m3"])
		}
		if result["m4"] != 0.72 {
			t.Errorf("expected m4 score 0.72, got %f", result["m4"])
		}
	})

	t.Run("overlap uses weighted blend with dual-hit bonus", func(t *testing.T) {
		fts := []store.Memory{
			{ID: "m1", Salience: 0.5},
			{ID: "m2", Salience: 0.9},
		}
		emb := []store.RetrievalResult{
			{Memory: store.Memory{ID: "m1"}, Score: 0.8},
			{Memory: store.Memory{ID: "m2"}, Score: 0.3},
			{Memory: store.Memory{ID: "m3"}, Score: 0.65}, // embedding only
		}
		result := agent.mergeEntryPoints(fts, emb)

		if len(result) != 3 {
			t.Fatalf("expected 3 entry points, got %d", len(result))
		}
		// m1 rank=1: fts = 0.7*1.0 + 0.3*0.5 = 0.85
		// dual-hit: 0.6*0.8 + 0.4*0.85 + 0.15 = 0.48 + 0.34 + 0.15 = 0.97
		ftsM1 := float32(0.7*1.0 + 0.3*0.5)
		expectedM1 := 0.6*float32(0.8) + 0.4*ftsM1 + 0.15
		if abs32(result["m1"]-expectedM1) > 0.001 {
			t.Errorf("expected m1 score ~%.3f (dual-hit blend), got %f", expectedM1, result["m1"])
		}
		// m2 rank=2: fts = 0.7*0.5 + 0.3*0.9 = 0.62
		// dual-hit: 0.6*0.3 + 0.4*0.62 + 0.15 = 0.18 + 0.248 + 0.15 = 0.578
		ftsM2 := float32(0.7*0.5 + 0.3*0.9)
		expectedM2 := 0.6*float32(0.3) + 0.4*ftsM2 + 0.15
		if abs32(result["m2"]-expectedM2) > 0.001 {
			t.Errorf("expected m2 score ~%.3f (dual-hit blend), got %f", expectedM2, result["m2"])
		}
		// m3: embedding only
		if result["m3"] != 0.65 {
			t.Errorf("expected m3 score 0.65 (embedding only), got %f", result["m3"])
		}
	})

	t.Run("FTS with zero salience gets default", func(t *testing.T) {
		fts := []store.Memory{
			{ID: "m1", Salience: 0.0},
		}
		result := agent.mergeEntryPoints(fts, nil)

		// Zero salience → default 0.5, rank 1: 0.7*1.0 + 0.3*0.5 = 0.85
		expected := float32(0.7*1.0 + 0.3*0.5)
		if abs32(result["m1"]-expected) > 0.001 {
			t.Errorf("expected default score ~%.3f for zero-salience FTS result, got %f", expected, result["m1"])
		}
	})

	t.Run("both empty", func(t *testing.T) {
		result := agent.mergeEntryPoints(nil, nil)
		if len(result) != 0 {
			t.Errorf("expected 0 entry points for empty inputs, got %d", len(result))
		}
	})
}

func TestSpreadActivation(t *testing.T) {
	t.Run("single hop with associations", func(t *testing.T) {
		s := &mockStore{
			getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
				switch memoryID {
				case "m1":
					return []store.Association{
						{SourceID: "m1", TargetID: "m2", Strength: 0.8, RelationType: "similar"},
						{SourceID: "m1", TargetID: "m3", Strength: 0.6, RelationType: "caused_by"},
					}, nil
				default:
					return nil, nil
				}
			},
		}

		cfg := RetrievalConfig{
			MaxHops:             1,
			ActivationThreshold: 0.1,
			DecayFactor:         0.7,
			MaxResults:          10,
		}
		agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

		entryPoints := map[string]float32{"m1": 1.0}
		result, _ := agent.spreadActivation(context.Background(), entryPoints)

		if result["m1"].activation != 1.0 {
			t.Errorf("expected m1 activation 1.0, got %f", result["m1"].activation)
		}
		if _, ok := result["m2"]; !ok {
			t.Fatal("expected m2 to be activated via association")
		}
		if _, ok := result["m3"]; !ok {
			t.Fatal("expected m3 to be activated via association")
		}
		// m2: 1.0 * 0.8 * 0.7 * 1.0 (similar) = 0.56
		expectedM2 := float32(1.0 * 0.8 * 0.7 * 1.0)
		if abs32(result["m2"].activation-expectedM2) > 0.001 {
			t.Errorf("expected m2 activation ~%.3f, got %.3f", expectedM2, result["m2"].activation)
		}
		// m3: 1.0 * 0.6 * 0.7 * 1.2 (caused_by) = 0.504
		expectedM3 := float32(1.0 * 0.6 * 0.7 * 1.2)
		if abs32(result["m3"].activation-expectedM3) > 0.001 {
			t.Errorf("expected m3 activation ~%.3f, got %.3f", expectedM3, result["m3"].activation)
		}
	})

	t.Run("multi-hop traversal", func(t *testing.T) {
		s := &mockStore{
			getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
				switch memoryID {
				case "m1":
					return []store.Association{
						{SourceID: "m1", TargetID: "m2", Strength: 0.9, RelationType: "similar"},
					}, nil
				case "m2":
					return []store.Association{
						{SourceID: "m2", TargetID: "m3", Strength: 0.9, RelationType: "similar"},
					}, nil
				case "m3":
					return []store.Association{
						{SourceID: "m3", TargetID: "m4", Strength: 0.9, RelationType: "similar"},
					}, nil
				default:
					return nil, nil
				}
			},
		}

		cfg := RetrievalConfig{
			MaxHops:             3,
			ActivationThreshold: 0.01, // low threshold so activation can propagate
			DecayFactor:         0.7,
			MaxResults:          10,
		}
		agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

		entryPoints := map[string]float32{"m1": 1.0}
		result, _ := agent.spreadActivation(context.Background(), entryPoints)

		// m1 should be present as entry point
		if _, ok := result["m1"]; !ok {
			t.Error("expected m1 in result")
		}
		// m2 should be reached at hop 1
		if _, ok := result["m2"]; !ok {
			t.Error("expected m2 in result (hop 1)")
		}
		// m3 should be reached at hop 2
		if _, ok := result["m3"]; !ok {
			t.Error("expected m3 in result (hop 2)")
		}
		// m4 should be reached at hop 3
		if _, ok := result["m4"]; !ok {
			t.Error("expected m4 in result (hop 3)")
		}

		// Verify decay: each hop should reduce activation
		if result["m2"].activation >= result["m1"].activation {
			t.Error("expected m2 activation < m1 due to decay")
		}
		if result["m3"].activation >= result["m2"].activation {
			t.Error("expected m3 activation < m2 due to decay")
		}
		if result["m4"].activation >= result["m3"].activation {
			t.Error("expected m4 activation < m3 due to decay")
		}
	})

	t.Run("below threshold not propagated", func(t *testing.T) {
		s := &mockStore{
			getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
				if memoryID == "m1" {
					return []store.Association{
						{SourceID: "m1", TargetID: "m2", Strength: 0.05, RelationType: "similar"},
					}, nil
				}
				return nil, nil
			},
		}

		cfg := RetrievalConfig{
			MaxHops:             2,
			ActivationThreshold: 0.5, // high threshold
			DecayFactor:         0.7,
			MaxResults:          10,
		}
		agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

		entryPoints := map[string]float32{"m1": 1.0}
		result, _ := agent.spreadActivation(context.Background(), entryPoints)

		if _, ok := result["m2"]; ok {
			t.Error("expected m2 to NOT be activated (below threshold)")
		}
	})

	t.Run("no associations returns only entry points", func(t *testing.T) {
		s := &mockStore{
			getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
				return nil, nil
			},
		}

		cfg := DefaultConfig()
		agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

		entryPoints := map[string]float32{"m1": 0.8, "m2": 0.5}
		result, _ := agent.spreadActivation(context.Background(), entryPoints)

		if len(result) != 2 {
			t.Fatalf("expected 2 results (entry points only), got %d", len(result))
		}
		if result["m1"].activation != 0.8 {
			t.Errorf("expected m1=0.8, got %f", result["m1"].activation)
		}
		if result["m2"].activation != 0.5 {
			t.Errorf("expected m2=0.5, got %f", result["m2"].activation)
		}
	})

	t.Run("association error is handled gracefully", func(t *testing.T) {
		s := &mockStore{
			getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
				return nil, fmt.Errorf("db error")
			},
		}

		cfg := DefaultConfig()
		agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

		entryPoints := map[string]float32{"m1": 1.0}
		result, _ := agent.spreadActivation(context.Background(), entryPoints)

		// Should still have the entry point even though associations failed
		if result["m1"].activation != 1.0 {
			t.Errorf("expected m1=1.0 despite association error, got %f", result["m1"].activation)
		}
	})
}

func TestQuery(t *testing.T) {
	now := time.Now()

	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "mem-1", Summary: "Go concurrency patterns", Salience: 0.8, LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return []store.RetrievalResult{
				{Memory: store.Memory{ID: "mem-2", Summary: "Channel usage in Go", Salience: 0.7, LastAccessed: now}, Score: 0.75},
			}, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil // no associations for simplicity
		},
		getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
			switch id {
			case "mem-1":
				return store.Memory{ID: "mem-1", Summary: "Go concurrency patterns", Salience: 0.8, LastAccessed: now}, nil
			case "mem-2":
				return store.Memory{ID: "mem-2", Summary: "Channel usage in Go", Salience: 0.7, LastAccessed: now}, nil
			default:
				return store.Memory{}, fmt.Errorf("not found: %s", id)
			}
		},
	}
	p := &mockLLMProvider{}
	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, p, cfg, testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query:      "Go concurrency",
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.QueryID == "" {
		t.Error("expected non-empty query ID")
	}
	if len(resp.Memories) == 0 {
		t.Error("expected at least one memory in results")
	}
	if resp.Synthesis != "" {
		t.Error("expected no synthesis when Synthesize=false")
	}
	if resp.TookMs < 0 {
		t.Error("expected non-negative TookMs")
	}

	// Verify access counts were incremented
	if len(s.incrementAccessCalls) != len(resp.Memories) {
		t.Errorf("expected %d IncrementAccess calls, got %d", len(resp.Memories), len(s.incrementAccessCalls))
	}

	// Verify embed was called for the query
	if p.embedCalls != 1 {
		t.Errorf("expected 1 embed call, got %d", p.embedCalls)
	}
}

func TestQueryWithSynthesis(t *testing.T) {
	now := time.Now()
	synthesisText := "Based on your memories, Go concurrency uses goroutines and channels."

	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "mem-1", Summary: "Go concurrency patterns", Salience: 0.8, LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
			return store.Memory{ID: id, Summary: "Go concurrency patterns", Salience: 0.8, LastAccessed: now}, nil
		},
	}

	p := &mockLLMProvider{
		completeFunc: func(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
			return llm.CompletionResponse{Content: synthesisText, TokensUsed: 25}, nil
		},
	}

	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, p, cfg, testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query:      "Go concurrency",
		Synthesize: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Synthesis != synthesisText {
		t.Errorf("expected synthesis %q, got %q", synthesisText, resp.Synthesis)
	}

	// Verify LLM Complete was called for synthesis
	if p.completeCalls != 1 {
		t.Errorf("expected 1 Complete call for synthesis, got %d", p.completeCalls)
	}
}

func TestQueryEmptyResults(t *testing.T) {
	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return nil, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
	}

	p := &mockLLMProvider{}
	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, p, cfg, testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query: "something with no matches",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(resp.Memories))
	}
	if resp.QueryID == "" {
		t.Error("expected non-empty query ID even with no results")
	}
}

func TestQueryEmptyResultsWithSynthesis(t *testing.T) {
	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return nil, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
	}

	p := &mockLLMProvider{}
	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, p, cfg, testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query:      "nothing here",
		Synthesize: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// synthesizeNarrative returns "No relevant memories found." for empty results
	if resp.Synthesis != "No relevant memories found." {
		t.Errorf("expected 'No relevant memories found.' synthesis, got %q", resp.Synthesis)
	}

	// LLM.Complete should NOT be called when there are no results
	if p.completeCalls != 0 {
		t.Errorf("expected 0 Complete calls for empty results synthesis, got %d", p.completeCalls)
	}
}

func TestQueryMaxResultsOverride(t *testing.T) {
	now := time.Now()

	// Return more results than the override limit
	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Salience: 0.9, Summary: "mem1", LastAccessed: now},
				{ID: "m2", Salience: 0.8, Summary: "mem2", LastAccessed: now},
				{ID: "m3", Salience: 0.7, Summary: "mem3", LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
			return store.Memory{ID: id, Summary: "mem " + id, Salience: 0.5, LastAccessed: now}, nil
		},
	}

	p := &mockLLMProvider{}
	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, p, cfg, testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query:      "test query",
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Memories) > 2 {
		t.Errorf("expected at most 2 results (MaxResults override), got %d", len(resp.Memories))
	}
}

func TestGetStats(t *testing.T) {
	s := &mockStore{}
	p := &mockLLMProvider{}
	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, p, cfg, testLogger())

	t.Run("initial stats are zeroed", func(t *testing.T) {
		stats := agent.GetStats()

		totalQueries, ok := stats["total_queries"]
		if !ok {
			t.Fatal("expected total_queries key in stats")
		}
		if totalQueries.(int64) != 0 {
			t.Errorf("expected total_queries=0, got %v", totalQueries)
		}

		totalRetrieved, ok := stats["total_memories_retrieved"]
		if !ok {
			t.Fatal("expected total_memories_retrieved key in stats")
		}
		if totalRetrieved.(int64) != 0 {
			t.Errorf("expected total_memories_retrieved=0, got %v", totalRetrieved)
		}

		avgPerQuery, ok := stats["avg_memories_per_query"]
		if !ok {
			t.Fatal("expected avg_memories_per_query key in stats")
		}
		if avgPerQuery.(float64) != 0 {
			t.Errorf("expected avg_memories_per_query=0, got %v", avgPerQuery)
		}
	})

	t.Run("stats updated after query", func(t *testing.T) {
		now := time.Now()
		s := &mockStore{
			searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
				return []store.Memory{
					{ID: "m1", Salience: 0.8, Summary: "test", LastAccessed: now},
				}, nil
			},
			searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
				return nil, nil
			},
			getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
				return nil, nil
			},
			getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
				return store.Memory{ID: id, Summary: "test", Salience: 0.8, LastAccessed: now}, nil
			},
		}
		agent := NewRetrievalAgent(s, &mockLLMProvider{}, DefaultConfig(), testLogger())

		_, err := agent.Query(context.Background(), QueryRequest{Query: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		stats := agent.GetStats()

		if stats["total_queries"].(int64) != 1 {
			t.Errorf("expected total_queries=1, got %v", stats["total_queries"])
		}
		if stats["total_memories_retrieved"].(int64) < 1 {
			t.Errorf("expected total_memories_retrieved >= 1, got %v", stats["total_memories_retrieved"])
		}
		if stats["last_query_time"].(time.Time).IsZero() {
			t.Error("expected last_query_time to be set after a query")
		}
	})
}

func TestResetStats(t *testing.T) {
	now := time.Now()
	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Salience: 0.8, Summary: "test", LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
			return store.Memory{ID: id, Summary: "test", Salience: 0.8, LastAccessed: now}, nil
		},
	}
	p := &mockLLMProvider{}
	agent := NewRetrievalAgent(s, p, DefaultConfig(), testLogger())

	// Run a query to populate stats
	_, err := agent.Query(context.Background(), QueryRequest{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify stats are non-zero
	statsBefore := agent.GetStats()
	if statsBefore["total_queries"].(int64) == 0 {
		t.Fatal("expected non-zero total_queries before reset")
	}

	// Reset
	agent.ResetStats()

	// Verify stats are zeroed
	statsAfter := agent.GetStats()
	if statsAfter["total_queries"].(int64) != 0 {
		t.Errorf("expected total_queries=0 after reset, got %v", statsAfter["total_queries"])
	}
	if statsAfter["total_memories_retrieved"].(int64) != 0 {
		t.Errorf("expected total_memories_retrieved=0 after reset, got %v", statsAfter["total_memories_retrieved"])
	}
	if statsAfter["avg_memories_per_query"].(float64) != 0 {
		t.Errorf("expected avg_memories_per_query=0 after reset, got %v", statsAfter["avg_memories_per_query"])
	}
	if !statsAfter["last_query_time"].(time.Time).IsZero() {
		t.Error("expected last_query_time to be zero after reset")
	}
}

func TestQueryIncludeReasoning(t *testing.T) {
	now := time.Now()

	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Salience: 0.8, Summary: "test memory", LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
			return store.Memory{ID: id, Summary: "test memory", Salience: 0.8, LastAccessed: now}, nil
		},
	}
	p := &mockLLMProvider{}
	agent := NewRetrievalAgent(s, p, DefaultConfig(), testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query:            "test",
		IncludeReasoning: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, mem := range resp.Memories {
		if mem.Explanation == "" {
			t.Errorf("expected non-empty explanation when IncludeReasoning=true for memory %s", mem.Memory.ID)
		}
	}
}

func TestQueryWithoutReasoning(t *testing.T) {
	now := time.Now()

	s := &mockStore{
		searchByFullTextFunc: func(ctx context.Context, query string, limit int) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Salience: 0.8, Summary: "test memory", LastAccessed: now},
			}, nil
		},
		searchByEmbeddingFunc: func(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
			return nil, nil
		},
		getAssociationsFunc: func(ctx context.Context, memoryID string) ([]store.Association, error) {
			return nil, nil
		},
		getMemoryFunc: func(ctx context.Context, id string) (store.Memory, error) {
			return store.Memory{ID: id, Summary: "test memory", Salience: 0.8, LastAccessed: now}, nil
		},
	}
	p := &mockLLMProvider{}
	agent := NewRetrievalAgent(s, p, DefaultConfig(), testLogger())

	resp, err := agent.Query(context.Background(), QueryRequest{
		Query:            "test",
		IncludeReasoning: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, mem := range resp.Memories {
		if mem.Explanation != "" {
			t.Errorf("expected empty explanation when IncludeReasoning=false, got %q", mem.Explanation)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxHops != 3 {
		t.Errorf("expected MaxHops=3, got %d", cfg.MaxHops)
	}
	if cfg.ActivationThreshold != 0.1 {
		t.Errorf("expected ActivationThreshold=0.1, got %f", cfg.ActivationThreshold)
	}
	if cfg.DecayFactor != 0.7 {
		t.Errorf("expected DecayFactor=0.7, got %f", cfg.DecayFactor)
	}
	if cfg.MaxResults != 7 {
		t.Errorf("expected MaxResults=7, got %d", cfg.MaxResults)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// ---------------------------------------------------------------------------
// Feedback-informed ranking tests
// ---------------------------------------------------------------------------

func TestRankResults_FeedbackInfluence(t *testing.T) {
	// Two memories with identical activation scores; feedback should differentiate them.
	memA := store.Memory{ID: "helpful-mem", Summary: "helpful memory", Salience: 0.5, Source: "mcp", LastAccessed: time.Now()}
	memB := store.Memory{ID: "irrelevant-mem", Summary: "irrelevant memory", Salience: 0.5, Source: "mcp", LastAccessed: time.Now()}

	s := &mockStore{
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			switch id {
			case memA.ID:
				return memA, nil
			case memB.ID:
				return memB, nil
			default:
				return store.Memory{}, fmt.Errorf("not found")
			}
		},
		getMemoryFeedbackScoresFunc: func(_ context.Context, ids []string) (map[string]float32, error) {
			return map[string]float32{
				"helpful-mem":    1.0,  // consistently helpful
				"irrelevant-mem": -1.0, // consistently irrelevant
			}, nil
		},
	}

	cfg := DefaultConfig()
	cfg.FeedbackWeight = 0.15
	agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

	activated := map[string]activationState{
		memA.ID: {activation: 0.8},
		memB.ID: {activation: 0.8},
	}

	results := agent.rankResults(context.Background(), activated, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// The helpful memory should rank higher
	if results[0].Memory.ID != "helpful-mem" {
		t.Errorf("expected helpful-mem to rank first, got %s", results[0].Memory.ID)
	}
	if results[1].Memory.ID != "irrelevant-mem" {
		t.Errorf("expected irrelevant-mem to rank second, got %s", results[1].Memory.ID)
	}

	// Score difference should be ~0.3 (0.15 * 1.0 - 0.15 * -1.0)
	diff := results[0].Score - results[1].Score
	if diff < 0.2 || diff > 0.4 {
		t.Errorf("expected score difference ~0.3 from feedback, got %.3f (scores: %.3f, %.3f)",
			diff, results[0].Score, results[1].Score)
	}
}

func TestRankResults_FeedbackErrorGraceful(t *testing.T) {
	// When feedback lookup fails, ranking should still work (graceful degradation).
	mem := store.Memory{ID: "m1", Summary: "test", Salience: 0.5, Source: "mcp", LastAccessed: time.Now()}

	s := &mockStore{
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			return mem, nil
		},
		getMemoryFeedbackScoresFunc: func(_ context.Context, _ []string) (map[string]float32, error) {
			return nil, fmt.Errorf("database error")
		},
	}

	cfg := DefaultConfig()
	agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

	activated := map[string]activationState{
		"m1": {activation: 0.7},
	}

	results := agent.rankResults(context.Background(), activated, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result despite feedback error, got %d", len(results))
	}
	if results[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", results[0].Score)
	}
}

// ---------------------------------------------------------------------------
// Source-weighted scoring tests
// ---------------------------------------------------------------------------

func TestRankResults_SourceWeighting(t *testing.T) {
	// Two memories with identical activation, different sources.
	now := time.Now()
	mcpMem := store.Memory{ID: "mcp-mem", Summary: "mcp memory", Salience: 0.5, Source: "mcp", LastAccessed: now}
	fsMem := store.Memory{ID: "fs-mem", Summary: "filesystem memory", Salience: 0.5, Source: "filesystem", LastAccessed: now}

	s := &mockStore{
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			switch id {
			case mcpMem.ID:
				return mcpMem, nil
			case fsMem.ID:
				return fsMem, nil
			default:
				return store.Memory{}, fmt.Errorf("not found")
			}
		},
	}

	cfg := DefaultConfig()
	cfg.SourceWeights = map[string]float32{
		"mcp":        1.0,
		"filesystem": 0.5,
	}
	agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

	activated := map[string]activationState{
		mcpMem.ID: {activation: 0.8},
		fsMem.ID:  {activation: 0.8},
	}

	results := agent.rankResults(context.Background(), activated, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// MCP memory (weight 1.0) should rank higher than filesystem (weight 0.5)
	if results[0].Memory.ID != "mcp-mem" {
		t.Errorf("expected mcp-mem to rank first, got %s", results[0].Memory.ID)
	}

	// The filesystem score should be roughly half the MCP score
	ratio := results[1].Score / results[0].Score
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("expected score ratio ~0.5, got %.3f (mcp=%.3f, fs=%.3f)",
			ratio, results[0].Score, results[1].Score)
	}
}

func TestRankResults_UnknownSourceGetsWeight1(t *testing.T) {
	// Unknown source should default to weight 1.0 (no penalty).
	now := time.Now()
	mem := store.Memory{ID: "m1", Summary: "test", Salience: 0.5, Source: "unknown_source", LastAccessed: now}

	s := &mockStore{
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			return mem, nil
		},
	}

	cfg := DefaultConfig()
	cfg.SourceWeights = map[string]float32{
		"mcp":        1.0,
		"filesystem": 0.5,
	}
	agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

	activated := map[string]activationState{
		"m1": {activation: 0.8},
	}

	results := agent.rankResults(context.Background(), activated, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// With source weight 1.0 (default) and activation 0.8, the base score should be
	// 0.8 * (1.0 + recency_bonus + activity_bonus) ~ 0.8 * (1.0 + ~0.2) = ~0.96
	// The important thing is it's not penalized.
	if results[0].Score < 0.7 {
		t.Errorf("expected score > 0.7 for unknown source (weight 1.0), got %f", results[0].Score)
	}
}

func TestRankResults_SourceAndFeedbackCombined(t *testing.T) {
	// Source weighting applies first (as multiplier), then feedback adjustment (additive).
	now := time.Now()
	fsMem := store.Memory{ID: "fs-helpful", Summary: "fs helpful", Salience: 0.5, Source: "filesystem", LastAccessed: now}
	mcpMem := store.Memory{ID: "mcp-irrelevant", Summary: "mcp irrelevant", Salience: 0.5, Source: "mcp", LastAccessed: now}

	s := &mockStore{
		getMemoryFunc: func(_ context.Context, id string) (store.Memory, error) {
			switch id {
			case fsMem.ID:
				return fsMem, nil
			case mcpMem.ID:
				return mcpMem, nil
			default:
				return store.Memory{}, fmt.Errorf("not found")
			}
		},
		getMemoryFeedbackScoresFunc: func(_ context.Context, _ []string) (map[string]float32, error) {
			return map[string]float32{
				"fs-helpful":     1.0,  // strong positive feedback
				"mcp-irrelevant": -1.0, // strong negative feedback
			}, nil
		},
	}

	cfg := DefaultConfig()
	cfg.SourceWeights = map[string]float32{
		"mcp":        1.0,
		"filesystem": 0.5,
	}
	cfg.FeedbackWeight = 0.3 // high weight to override source bias
	agent := NewRetrievalAgent(s, &mockLLMProvider{}, cfg, testLogger())

	activated := map[string]activationState{
		fsMem.ID:  {activation: 0.8},
		mcpMem.ID: {activation: 0.8},
	}

	results := agent.rankResults(context.Background(), activated, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// With high feedback weight, the filesystem memory (helpful, +0.3) should overcome
	// its 0.5x source penalty relative to the MCP memory (irrelevant, -0.3).
	// fs score: 0.8 * ~1.2 * 0.5 + 0.3 = ~0.78
	// mcp score: 0.8 * ~1.2 * 1.0 - 0.3 = ~0.66
	if results[0].Memory.ID != "fs-helpful" {
		t.Errorf("expected feedback to override source bias; got %s first", results[0].Memory.ID)
	}
}
