package consolidation

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

// ---------------------------------------------------------------------------
// Mock Store
// ---------------------------------------------------------------------------

// mockStore is a configurable mock implementation of store.Store for testing.
// Each method delegates to a callback if set, otherwise returns zero values.
type mockStore struct {
	// Configurable callbacks
	listMemoriesFn          func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error)
	batchUpdateSalienceFn   func(ctx context.Context, updates map[string]float32) error
	updateStateFn           func(ctx context.Context, id string, state string) error
	pruneWeakAssociationsFn func(ctx context.Context, threshold float32) (int, error)
	deleteOldArchivedFn     func(ctx context.Context, olderThan time.Time) (int, error)
	batchMergeMemoriesFn    func(ctx context.Context, sourceIDs []string, gist store.Memory) error
	writeConsolidationFn    func(ctx context.Context, record store.ConsolidationRecord) error
	getMemoryAttributesFn   func(ctx context.Context, memoryID string) (store.MemoryAttributes, error)

	// Call tracking
	updateStateCalls         []updateStateCall
	batchUpdateSalienceCalls []map[string]float32
	pruneWeakAssocCalls      []float32
	deleteOldArchivedCalls   []time.Time
	batchMergeMemoriesCalls  []batchMergeCall
	writeConsolidationCalls  []store.ConsolidationRecord
}

type updateStateCall struct {
	ID    string
	State string
}

type batchMergeCall struct {
	SourceIDs []string
	Gist      store.Memory
}

func newMockStore() *mockStore {
	return &mockStore{}
}

// --- Raw memory operations ---
func (m *mockStore) WriteRaw(ctx context.Context, raw store.RawMemory) error {
	return nil
}
func (m *mockStore) GetRaw(ctx context.Context, id string) (store.RawMemory, error) {
	return store.RawMemory{}, nil
}
func (m *mockStore) ListRawUnprocessed(ctx context.Context, limit int) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) ListRawMemoriesAfter(ctx context.Context, after time.Time, limit int) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) MarkRawProcessed(ctx context.Context, id string) error {
	return nil
}

// --- Encoded memory operations ---
func (m *mockStore) WriteMemory(ctx context.Context, mem store.Memory) error {
	return nil
}
func (m *mockStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) GetMemoryByRawID(ctx context.Context, rawID string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) UpdateMemory(ctx context.Context, mem store.Memory) error {
	return nil
}
func (m *mockStore) UpdateSalience(ctx context.Context, id string, salience float32) error {
	return nil
}
func (m *mockStore) UpdateState(ctx context.Context, id string, state string) error {
	m.updateStateCalls = append(m.updateStateCalls, updateStateCall{ID: id, State: state})
	if m.updateStateFn != nil {
		return m.updateStateFn(ctx, id, state)
	}
	return nil
}
func (m *mockStore) IncrementAccess(ctx context.Context, id string) error {
	return nil
}
func (m *mockStore) ListMemories(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
	if m.listMemoriesFn != nil {
		return m.listMemoriesFn(ctx, state, limit, offset)
	}
	return nil, nil
}
func (m *mockStore) CountMemories(ctx context.Context) (int, error) {
	return 0, nil
}

// --- Search operations ---
func (m *mockStore) SearchByFullText(ctx context.Context, query string, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
	return nil, nil
}
func (m *mockStore) SearchByConcepts(ctx context.Context, concepts []string, limit int) ([]store.Memory, error) {
	return nil, nil
}

// --- Association graph operations ---
func (m *mockStore) CreateAssociation(ctx context.Context, assoc store.Association) error {
	return nil
}
func (m *mockStore) GetAssociations(ctx context.Context, memoryID string) ([]store.Association, error) {
	return nil, nil
}
func (m *mockStore) UpdateAssociationStrength(ctx context.Context, sourceID, targetID string, strength float32) error {
	return nil
}
func (m *mockStore) UpdateAssociationType(ctx context.Context, sourceID, targetID string, relationType string) error {
	return nil
}
func (m *mockStore) WriteRetrievalFeedback(ctx context.Context, fb store.RetrievalFeedback) error {
	return nil
}
func (m *mockStore) GetRetrievalFeedback(ctx context.Context, queryID string) (store.RetrievalFeedback, error) {
	return store.RetrievalFeedback{}, nil
}
func (m *mockStore) ActivateAssociation(ctx context.Context, sourceID, targetID string) error {
	return nil
}
func (m *mockStore) PruneWeakAssociations(ctx context.Context, strengthThreshold float32) (int, error) {
	m.pruneWeakAssocCalls = append(m.pruneWeakAssocCalls, strengthThreshold)
	if m.pruneWeakAssociationsFn != nil {
		return m.pruneWeakAssociationsFn(ctx, strengthThreshold)
	}
	return 0, nil
}

// --- Batch operations ---
func (m *mockStore) BatchUpdateSalience(ctx context.Context, updates map[string]float32) error {
	m.batchUpdateSalienceCalls = append(m.batchUpdateSalienceCalls, updates)
	if m.batchUpdateSalienceFn != nil {
		return m.batchUpdateSalienceFn(ctx, updates)
	}
	return nil
}
func (m *mockStore) BatchMergeMemories(ctx context.Context, sourceIDs []string, gist store.Memory) error {
	m.batchMergeMemoriesCalls = append(m.batchMergeMemoriesCalls, batchMergeCall{SourceIDs: sourceIDs, Gist: gist})
	if m.batchMergeMemoriesFn != nil {
		return m.batchMergeMemoriesFn(ctx, sourceIDs, gist)
	}
	return nil
}
func (m *mockStore) DeleteOldArchived(ctx context.Context, olderThan time.Time) (int, error) {
	m.deleteOldArchivedCalls = append(m.deleteOldArchivedCalls, olderThan)
	if m.deleteOldArchivedFn != nil {
		return m.deleteOldArchivedFn(ctx, olderThan)
	}
	return 0, nil
}

// --- Consolidation tracking ---
func (m *mockStore) WriteConsolidation(ctx context.Context, record store.ConsolidationRecord) error {
	m.writeConsolidationCalls = append(m.writeConsolidationCalls, record)
	if m.writeConsolidationFn != nil {
		return m.writeConsolidationFn(ctx, record)
	}
	return nil
}
func (m *mockStore) GetLastConsolidation(ctx context.Context) (store.ConsolidationRecord, error) {
	return store.ConsolidationRecord{}, nil
}

// --- Export/Backup operations ---
func (m *mockStore) ListAllAssociations(ctx context.Context) ([]store.Association, error) {
	return nil, nil
}
func (m *mockStore) ListAllRawMemories(ctx context.Context) ([]store.RawMemory, error) {
	return nil, nil
}

// --- Metacognition operations ---
func (m *mockStore) WriteMetaObservation(ctx context.Context, obs store.MetaObservation) error {
	return nil
}
func (m *mockStore) ListMetaObservations(ctx context.Context, observationType string, limit int) ([]store.MetaObservation, error) {
	return nil, nil
}
func (m *mockStore) GetDeadMemories(ctx context.Context, cutoffDate time.Time) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) GetSourceDistribution(ctx context.Context) (map[string]int, error) {
	return nil, nil
}

// --- Episode operations ---
func (m *mockStore) CreateEpisode(ctx context.Context, ep store.Episode) error { return nil }
func (m *mockStore) GetEpisode(ctx context.Context, id string) (store.Episode, error) {
	return store.Episode{}, nil
}
func (m *mockStore) UpdateEpisode(ctx context.Context, ep store.Episode) error { return nil }
func (m *mockStore) ListEpisodes(ctx context.Context, state string, limit, offset int) ([]store.Episode, error) {
	return nil, nil
}
func (m *mockStore) GetOpenEpisode(ctx context.Context) (store.Episode, error) {
	return store.Episode{}, fmt.Errorf("no open episode")
}
func (m *mockStore) CloseEpisode(ctx context.Context, id string) error { return nil }

// --- Multi-resolution operations ---
func (m *mockStore) WriteMemoryResolution(ctx context.Context, res store.MemoryResolution) error {
	return nil
}
func (m *mockStore) GetMemoryResolution(ctx context.Context, memoryID string) (store.MemoryResolution, error) {
	return store.MemoryResolution{}, nil
}

// --- Structured concept operations ---
func (m *mockStore) WriteConceptSet(ctx context.Context, cs store.ConceptSet) error { return nil }
func (m *mockStore) GetConceptSet(ctx context.Context, memoryID string) (store.ConceptSet, error) {
	return store.ConceptSet{}, nil
}
func (m *mockStore) SearchByEntity(ctx context.Context, name string, entityType string, limit int) ([]store.Memory, error) {
	return nil, nil
}

// --- Memory attribute operations ---
func (m *mockStore) WriteMemoryAttributes(ctx context.Context, attrs store.MemoryAttributes) error {
	return nil
}
func (m *mockStore) GetMemoryAttributes(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
	if m.getMemoryAttributesFn != nil {
		return m.getMemoryAttributesFn(ctx, memoryID)
	}
	return store.MemoryAttributes{}, fmt.Errorf("no attributes")
}

// --- Housekeeping ---
func (m *mockStore) GetStatistics(ctx context.Context) (store.StoreStatistics, error) {
	return store.StoreStatistics{}, nil
}

// --- Pattern operations ---
func (m *mockStore) WritePattern(ctx context.Context, p store.Pattern) error { return nil }
func (m *mockStore) GetPattern(ctx context.Context, id string) (store.Pattern, error) {
	return store.Pattern{}, nil
}
func (m *mockStore) UpdatePattern(ctx context.Context, p store.Pattern) error { return nil }
func (m *mockStore) ListPatterns(ctx context.Context, project string, limit int) ([]store.Pattern, error) {
	return nil, nil
}
func (m *mockStore) SearchPatternsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.Pattern, error) {
	return nil, nil
}
func (m *mockStore) ArchiveAllPatterns(ctx context.Context) (int, error) {
	return 0, nil
}

// --- Abstraction operations ---
func (m *mockStore) WriteAbstraction(ctx context.Context, a store.Abstraction) error { return nil }
func (m *mockStore) GetAbstraction(ctx context.Context, id string) (store.Abstraction, error) {
	return store.Abstraction{}, nil
}
func (m *mockStore) UpdateAbstraction(ctx context.Context, a store.Abstraction) error { return nil }
func (m *mockStore) ListAbstractions(ctx context.Context, level int, limit int) ([]store.Abstraction, error) {
	return nil, nil
}
func (m *mockStore) SearchAbstractionsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.Abstraction, error) {
	return nil, nil
}
func (m *mockStore) ArchiveAllAbstractions(ctx context.Context) (int, error) {
	return 0, nil
}

// --- Scoped queries ---
func (m *mockStore) SearchByProject(ctx context.Context, project string, query string, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) ListMemoriesByTimeRange(ctx context.Context, from, to time.Time, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) GetProjectSummary(ctx context.Context, project string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockStore) ListProjects(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockStore) RawMemoryExistsByPath(ctx context.Context, source string, project string, filePath string) (bool, error) {
	return false, nil
}
func (m *mockStore) CountRawUnprocessedByPathPatterns(ctx context.Context, patterns []string) (int, error) {
	return 0, nil
}
func (m *mockStore) BulkMarkRawProcessedByPathPatterns(ctx context.Context, patterns []string) (int, error) {
	return 0, nil
}
func (m *mockStore) ArchiveMemoriesByRawPathPatterns(ctx context.Context, patterns []string) (int, error) {
	return 0, nil
}
func (m *mockStore) BatchWriteRaw(ctx context.Context, raws []store.RawMemory) error { return nil }
func (m *mockStore) DeleteOldMetaObservations(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

// --- Lifecycle ---
func (m *mockStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// Mock LLM Provider
// ---------------------------------------------------------------------------

type mockLLMProvider struct {
	completeFn  func(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
	embedFn     func(ctx context.Context, text string) ([]float32, error)
	completions []llm.CompletionRequest // track calls
}

func newMockLLMProvider() *mockLLMProvider {
	return &mockLLMProvider{}
}

func (m *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	m.completions = append(m.completions, req)
	if m.completeFn != nil {
		return m.completeFn(ctx, req)
	}
	return llm.CompletionResponse{Content: `{"summary":"merged gist","content":"combined content"}`}, nil
}

func (m *mockLLMProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *mockLLMProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		results[i] = []float32{0.1, 0.2, 0.3}
	}
	return results, nil
}

func (m *mockLLMProvider) Health(ctx context.Context) error {
	return nil
}

func (m *mockLLMProvider) ModelInfo(ctx context.Context) (llm.ModelMetadata, error) {
	return llm.ModelMetadata{Name: "mock-model"}, nil
}

// ---------------------------------------------------------------------------
// Mock Event Bus
// ---------------------------------------------------------------------------

type mockBus struct {
	published []events.Event
}

func (m *mockBus) Publish(ctx context.Context, event events.Event) error {
	m.published = append(m.published, event)
	return nil
}
func (m *mockBus) Subscribe(eventType string, handler events.Handler) string { return "sub-1" }
func (m *mockBus) Unsubscribe(subscriptionID string)                         {}
func (m *mockBus) Close() error                                              { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testConfig() ConsolidationConfig {
	cfg := DefaultConfig()
	cfg.Interval = 1 * time.Second // fast for tests
	cfg.MaxMemoriesPerCycle = 100
	cfg.MinClusterSize = 3
	return cfg
}

// almostEqual compares two float32 values within a tolerance.
func almostEqual(a, b, tolerance float32) bool {
	return float32(math.Abs(float64(a-b))) <= tolerance
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewConsolidationAgent(t *testing.T) {
	ms := newMockStore()
	mlp := newMockLLMProvider()
	cfg := testConfig()
	log := testLogger()

	agent := NewConsolidationAgent(ms, mlp, cfg, log)

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.store != ms {
		t.Error("store was not set correctly")
	}
	if agent.llmProvider != mlp {
		t.Error("llmProvider was not set correctly")
	}
	if agent.config.DecayRate != cfg.DecayRate {
		t.Errorf("expected DecayRate %f, got %f", cfg.DecayRate, agent.config.DecayRate)
	}
	if agent.config.FadeThreshold != cfg.FadeThreshold {
		t.Errorf("expected FadeThreshold %f, got %f", cfg.FadeThreshold, agent.config.FadeThreshold)
	}
	if agent.config.ArchiveThreshold != cfg.ArchiveThreshold {
		t.Errorf("expected ArchiveThreshold %f, got %f", cfg.ArchiveThreshold, agent.config.ArchiveThreshold)
	}
	if agent.ctx != nil {
		t.Error("expected nil context before Start()")
	}
	if agent.cancel != nil {
		t.Error("expected nil cancel func before Start()")
	}
}

func TestConsolidationAgentName(t *testing.T) {
	ms := newMockStore()
	mlp := newMockLLMProvider()
	agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

	name := agent.Name()
	if name != "consolidation-agent" {
		t.Errorf("expected Name() = %q, got %q", "consolidation-agent", name)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float32
		tol      float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			tol:      1e-6,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			tol:      1e-6,
		},
		{
			name:     "similar non-unit vectors",
			a:        []float32{3, 4, 0},
			b:        []float32{4, 3, 0},
			expected: 24.0 / 25.0, // dot=24, |a|=5, |b|=5
			tol:      1e-5,
		},
		{
			name:     "different lengths returns 0",
			a:        []float32{1, 2},
			b:        []float32{1, 2, 3},
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "empty vectors returns 0",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "nil vectors returns 0",
			a:        nil,
			b:        nil,
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "zero vector a returns 0",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 2, 3},
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "zero vector b returns 0",
			a:        []float32{1, 2, 3},
			b:        []float32{0, 0, 0},
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "both zero vectors returns 0",
			a:        []float32{0, 0, 0},
			b:        []float32{0, 0, 0},
			expected: 0.0,
			tol:      1e-6,
		},
		{
			name:     "high-dimensional identical",
			a:        []float32{0.1, 0.2, 0.3, 0.4, 0.5},
			b:        []float32{0.1, 0.2, 0.3, 0.4, 0.5},
			expected: 1.0,
			tol:      1e-5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := cosineSimilarity(tc.a, tc.b)
			if !almostEqual(result, tc.expected, tc.tol) {
				t.Errorf("cosineSimilarity(%v, %v) = %f, want %f (tol %f)", tc.a, tc.b, result, tc.expected, tc.tol)
			}
		})
	}
}

func TestFindClusters(t *testing.T) {
	ms := newMockStore()
	mlp := newMockLLMProvider()
	cfg := testConfig()
	cfg.MinClusterSize = 2 // lower threshold for test
	agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

	// Create memories with embeddings. Memories 1-3 have nearly identical embeddings
	// (high cosine similarity). Memory 4 is orthogonal.
	memories := []store.Memory{
		{ID: "m1", Embedding: []float32{1.0, 0.0, 0.0}, Summary: "mem1"},
		{ID: "m2", Embedding: []float32{0.99, 0.05, 0.0}, Summary: "mem2"},
		{ID: "m3", Embedding: []float32{0.98, 0.1, 0.0}, Summary: "mem3"},
		{ID: "m4", Embedding: []float32{0.0, 1.0, 0.0}, Summary: "mem4"},      // orthogonal
		{ID: "m5", Embedding: nil, Summary: "no embedding"},                   // no embedding
		{ID: "m6", Embedding: []float32{0.0, 0.0, 1.0}, Summary: "different"}, // different direction
	}

	t.Run("clusters similar memories together", func(t *testing.T) {
		clusters := agent.findClusters(memories)

		// m1, m2, m3 should form a cluster (cosine sim > 0.85)
		// m4, m5, m6 should not cluster together
		foundCluster := false
		for _, cluster := range clusters {
			ids := make(map[string]bool)
			for _, mem := range cluster {
				ids[mem.ID] = true
			}
			if ids["m1"] && ids["m2"] && ids["m3"] {
				foundCluster = true
				break
			}
		}
		if !foundCluster {
			t.Errorf("expected m1, m2, m3 to form a cluster, got %d clusters", len(clusters))
			for i, cluster := range clusters {
				ids := make([]string, len(cluster))
				for j, mem := range cluster {
					ids[j] = mem.ID
				}
				t.Logf("  cluster %d: %v", i, ids)
			}
		}
	})

	t.Run("skips memories without embeddings", func(t *testing.T) {
		clusters := agent.findClusters(memories)
		for _, cluster := range clusters {
			for _, mem := range cluster {
				if mem.ID == "m5" {
					t.Error("memory without embedding should not be in any cluster")
				}
			}
		}
	})

	t.Run("empty memories returns nil", func(t *testing.T) {
		clusters := agent.findClusters(nil)
		if clusters != nil {
			t.Errorf("expected nil clusters for empty input, got %v", clusters)
		}
	})

	t.Run("respects min cluster size", func(t *testing.T) {
		cfg2 := testConfig()
		cfg2.MinClusterSize = 10
		agent2 := NewConsolidationAgent(ms, mlp, cfg2, testLogger())
		clusters := agent2.findClusters(memories)
		if len(clusters) != 0 {
			t.Errorf("expected 0 clusters with minClusterSize=10, got %d", len(clusters))
		}
	})
}

func TestDecaySalience(t *testing.T) {
	now := time.Now()

	t.Run("applies decay to active and fading memories", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.95
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		// Memory accessed long ago (> 168h) gets full decay
		oldMemory := store.Memory{
			ID:           "old-mem",
			Salience:     0.8,
			AccessCount:  0,
			LastAccessed: now.Add(-200 * time.Hour),
			CreatedAt:    now.Add(-200 * time.Hour),
			State:        "active",
		}

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{oldMemory}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{}, fmt.Errorf("not found")
		}

		decayed, processed, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if processed != 1 {
			t.Errorf("expected 1 processed, got %d", processed)
		}
		if decayed != 1 {
			t.Errorf("expected 1 decayed, got %d", decayed)
		}

		// Verify batch update was called
		if len(ms.batchUpdateSalienceCalls) != 1 {
			t.Fatalf("expected 1 BatchUpdateSalience call, got %d", len(ms.batchUpdateSalienceCalls))
		}

		updates := ms.batchUpdateSalienceCalls[0]
		newSalience, ok := updates["old-mem"]
		if !ok {
			t.Fatal("expected old-mem in updates")
		}
		// Full decay: recencyFactor=1.0, accessBonus=1.0, effective = 0.95^1.0 = 0.95
		// newSalience = 0.8 * 0.95 = 0.76
		expectedSalience := float32(0.8 * 0.95)
		if !almostEqual(newSalience, expectedSalience, 0.01) {
			t.Errorf("expected salience ~%f, got %f", expectedSalience, newSalience)
		}
	})

	t.Run("recency protection for recently accessed memories", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.95
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		recentMemory := store.Memory{
			ID:           "recent-mem",
			Salience:     0.8,
			AccessCount:  0,
			LastAccessed: now.Add(-2 * time.Hour), // within 24h
			CreatedAt:    now.Add(-48 * time.Hour),
			State:        "active",
		}

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{recentMemory}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{}, fmt.Errorf("not found")
		}

		_, _, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ms.batchUpdateSalienceCalls) != 1 {
			t.Fatalf("expected 1 BatchUpdateSalience call, got %d", len(ms.batchUpdateSalienceCalls))
		}

		updates := ms.batchUpdateSalienceCalls[0]
		newSalience := updates["recent-mem"]
		// recencyFactor=0.8, accessBonus=1.0, effective = 0.95^0.8 ≈ 0.9592
		// newSalience = 0.8 * 0.9592 ≈ 0.7674
		effectiveDecay := math.Pow(0.95, 0.8)
		expectedSalience := float32(0.8 * effectiveDecay)
		if !almostEqual(newSalience, expectedSalience, 0.01) {
			t.Errorf("expected salience ~%f for recently accessed memory, got %f", expectedSalience, newSalience)
		}
		// Recently accessed memory should decay slower (higher salience remains)
		fullDecaySalience := float32(0.8 * 0.95)
		if newSalience <= fullDecaySalience {
			t.Errorf("recently accessed memory should decay slower: got %f, full decay would give %f", newSalience, fullDecaySalience)
		}
	})

	t.Run("access count bonus reduces decay", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.95
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		frequentMemory := store.Memory{
			ID:           "freq-mem",
			Salience:     0.8,
			AccessCount:  15, // 15 * 0.02 = 0.30 → capped at 0.30, so accessBonus = 0.7
			LastAccessed: now.Add(-200 * time.Hour),
			CreatedAt:    now.Add(-200 * time.Hour),
			State:        "active",
		}

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{frequentMemory}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{}, fmt.Errorf("not found")
		}

		_, _, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ms.batchUpdateSalienceCalls) != 1 {
			t.Fatalf("expected 1 BatchUpdateSalience call, got %d", len(ms.batchUpdateSalienceCalls))
		}

		updates := ms.batchUpdateSalienceCalls[0]
		newSalience := updates["freq-mem"]
		// recencyFactor=1.0 (old), accessBonus=0.7 (15*0.02=0.3, capped), effective = 0.95^(1.0*0.7)
		effectiveDecay := math.Pow(0.95, 1.0*0.7)
		expectedSalience := float32(0.8 * effectiveDecay)
		if !almostEqual(newSalience, expectedSalience, 0.01) {
			t.Errorf("expected salience ~%f for frequently accessed memory, got %f", expectedSalience, newSalience)
		}
	})

	t.Run("critical significance slows decay", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.95
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		criticalMemory := store.Memory{
			ID:           "crit-mem",
			Salience:     0.8,
			AccessCount:  0,
			LastAccessed: now.Add(-200 * time.Hour),
			CreatedAt:    now.Add(-200 * time.Hour),
			State:        "active",
		}

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{criticalMemory}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{Significance: "critical"}, nil
		}

		_, _, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ms.batchUpdateSalienceCalls) != 1 {
			t.Fatalf("expected 1 BatchUpdateSalience call, got %d", len(ms.batchUpdateSalienceCalls))
		}

		updates := ms.batchUpdateSalienceCalls[0]
		newSalience := updates["crit-mem"]
		// Critical: effective decay raised to 0.8 power → decays 20% slower
		effectiveDecay := math.Pow(0.95, 1.0*1.0)
		expectedCritical := float32(0.8 * math.Pow(effectiveDecay, 0.8))
		if !almostEqual(newSalience, expectedCritical, 0.01) {
			t.Errorf("expected salience ~%f for critical memory, got %f", expectedCritical, newSalience)
		}
	})

	t.Run("no memories returns zero counts", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			return nil, nil
		}

		decayed, processed, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decayed != 0 || processed != 0 {
			t.Errorf("expected 0 decayed and 0 processed, got %d and %d", decayed, processed)
		}
		if len(ms.batchUpdateSalienceCalls) != 0 {
			t.Error("expected no BatchUpdateSalience calls when there are no memories")
		}
	})

	t.Run("salience floor at 0.01", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.01 // very aggressive decay
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		tinyMemory := store.Memory{
			ID:           "tiny-mem",
			Salience:     0.02,
			AccessCount:  0,
			LastAccessed: now.Add(-200 * time.Hour),
			CreatedAt:    now.Add(-200 * time.Hour),
			State:        "active",
		}

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{tinyMemory}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{}, fmt.Errorf("not found")
		}

		_, _, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ms.batchUpdateSalienceCalls) != 1 {
			t.Fatalf("expected 1 BatchUpdateSalience call, got %d", len(ms.batchUpdateSalienceCalls))
		}

		updates := ms.batchUpdateSalienceCalls[0]
		newSalience := updates["tiny-mem"]
		if newSalience < 0.01 {
			t.Errorf("salience should not go below 0.01, got %f", newSalience)
		}
	})

	t.Run("zero LastAccessed falls back to CreatedAt", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.95
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		mem := store.Memory{
			ID:           "no-access-mem",
			Salience:     0.8,
			AccessCount:  0,
			LastAccessed: time.Time{}, // zero value
			CreatedAt:    now.Add(-200 * time.Hour),
			State:        "active",
		}

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{mem}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{}, fmt.Errorf("not found")
		}

		decayed, _, err := agent.decaySalience(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decayed != 1 {
			t.Errorf("expected 1 decayed, got %d", decayed)
		}
		// Should use full decay since CreatedAt is > 168h ago
		updates := ms.batchUpdateSalienceCalls[0]
		newSalience := updates["no-access-mem"]
		expectedSalience := float32(0.8 * 0.95)
		if !almostEqual(newSalience, expectedSalience, 0.01) {
			t.Errorf("expected salience ~%f when LastAccessed is zero, got %f", expectedSalience, newSalience)
		}
	})
}

func TestTransitionStates(t *testing.T) {
	t.Run("active memory below fade threshold transitions to fading", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.FadeThreshold = 0.3
		cfg.ArchiveThreshold = 0.1
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{
					{ID: "fading-mem", Salience: 0.2, State: "active"},
				}, nil
			}
			return nil, nil
		}

		toFading, toArchived, err := agent.transitionStates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toFading != 1 {
			t.Errorf("expected 1 fading transition, got %d", toFading)
		}
		if toArchived != 0 {
			t.Errorf("expected 0 archived transitions, got %d", toArchived)
		}

		// Verify the correct state update was called
		found := false
		for _, call := range ms.updateStateCalls {
			if call.ID == "fading-mem" && call.State == "fading" {
				found = true
			}
		}
		if !found {
			t.Error("expected UpdateState(fading-mem, fading) to be called")
		}
	})

	t.Run("active memory below archive threshold goes straight to archived", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.FadeThreshold = 0.3
		cfg.ArchiveThreshold = 0.1
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{
					{ID: "archive-mem", Salience: 0.05, State: "active"},
				}, nil
			}
			return nil, nil
		}

		toFading, toArchived, err := agent.transitionStates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toFading != 0 {
			t.Errorf("expected 0 fading transitions, got %d", toFading)
		}
		if toArchived != 1 {
			t.Errorf("expected 1 archived transition, got %d", toArchived)
		}

		found := false
		for _, call := range ms.updateStateCalls {
			if call.ID == "archive-mem" && call.State == "archived" {
				found = true
			}
		}
		if !found {
			t.Error("expected UpdateState(archive-mem, archived) to be called")
		}
	})

	t.Run("fading memory below archive threshold transitions to archived", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.FadeThreshold = 0.3
		cfg.ArchiveThreshold = 0.1
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "fading" {
				return []store.Memory{
					{ID: "fading-to-archive", Salience: 0.05, State: "fading"},
				}, nil
			}
			return nil, nil
		}

		toFading, toArchived, err := agent.transitionStates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toFading != 0 {
			t.Errorf("expected 0 fading transitions, got %d", toFading)
		}
		if toArchived != 1 {
			t.Errorf("expected 1 archived transition, got %d", toArchived)
		}
	})

	t.Run("memory above thresholds stays in current state", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.FadeThreshold = 0.3
		cfg.ArchiveThreshold = 0.1
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{
					{ID: "healthy-mem", Salience: 0.9, State: "active"},
				}, nil
			}
			return nil, nil
		}

		toFading, toArchived, err := agent.transitionStates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toFading != 0 {
			t.Errorf("expected 0 fading transitions, got %d", toFading)
		}
		if toArchived != 0 {
			t.Errorf("expected 0 archived transitions, got %d", toArchived)
		}
		if len(ms.updateStateCalls) != 0 {
			t.Error("expected no UpdateState calls for healthy memory")
		}
	})

	t.Run("mixed memories transition correctly", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.FadeThreshold = 0.3
		cfg.ArchiveThreshold = 0.1
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{
					{ID: "healthy", Salience: 0.9, State: "active"},
					{ID: "going-fading", Salience: 0.2, State: "active"},
					{ID: "going-archived", Salience: 0.05, State: "active"},
				}, nil
			}
			if state == "fading" {
				return []store.Memory{
					{ID: "fading-to-archive", Salience: 0.08, State: "fading"},
					{ID: "fading-stable", Salience: 0.15, State: "fading"},
				}, nil
			}
			return nil, nil
		}

		toFading, toArchived, err := agent.transitionStates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toFading != 1 {
			t.Errorf("expected 1 fading transition, got %d", toFading)
		}
		// going-archived (from active) + fading-to-archive (from fading)
		if toArchived != 2 {
			t.Errorf("expected 2 archived transitions, got %d", toArchived)
		}
	})
}

func TestPruneAssociations(t *testing.T) {
	t.Run("delegates to store with correct threshold", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.AssocPruneThreshold = 0.05
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.pruneWeakAssociationsFn = func(ctx context.Context, threshold float32) (int, error) {
			return 7, nil
		}

		pruned, err := agent.pruneAssociations(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pruned != 7 {
			t.Errorf("expected 7 pruned, got %d", pruned)
		}

		if len(ms.pruneWeakAssocCalls) != 1 {
			t.Fatalf("expected 1 PruneWeakAssociations call, got %d", len(ms.pruneWeakAssocCalls))
		}
		if ms.pruneWeakAssocCalls[0] != 0.05 {
			t.Errorf("expected threshold 0.05, got %f", ms.pruneWeakAssocCalls[0])
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

		ms.pruneWeakAssociationsFn = func(ctx context.Context, threshold float32) (int, error) {
			return 0, fmt.Errorf("db error")
		}

		_, err := agent.pruneAssociations(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestDeleteExpired(t *testing.T) {
	t.Run("delegates to store with correct cutoff", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.RetentionWindow = 90 * 24 * time.Hour
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		ms.deleteOldArchivedFn = func(ctx context.Context, olderThan time.Time) (int, error) {
			// Verify cutoff is roughly 90 days ago
			expectedCutoff := time.Now().Add(-90 * 24 * time.Hour)
			diff := olderThan.Sub(expectedCutoff)
			if diff < -time.Minute || diff > time.Minute {
				t.Errorf("expected cutoff ~90 days ago, got %v (diff %v)", olderThan, diff)
			}
			return 5, nil
		}

		deleted, err := agent.deleteExpired(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deleted != 5 {
			t.Errorf("expected 5 deleted, got %d", deleted)
		}

		if len(ms.deleteOldArchivedCalls) != 1 {
			t.Fatalf("expected 1 DeleteOldArchived call, got %d", len(ms.deleteOldArchivedCalls))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

		ms.deleteOldArchivedFn = func(ctx context.Context, olderThan time.Time) (int, error) {
			return 0, fmt.Errorf("db error")
		}

		_, err := agent.deleteExpired(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestRunCycle(t *testing.T) {
	t.Run("full cycle end-to-end", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		cfg := testConfig()
		cfg.DecayRate = 0.95
		cfg.FadeThreshold = 0.3
		cfg.ArchiveThreshold = 0.1
		cfg.AssocPruneThreshold = 0.05
		cfg.RetentionWindow = 90 * 24 * time.Hour
		cfg.MinClusterSize = 5 // high enough that no merges happen with test data
		agent := NewConsolidationAgent(ms, mlp, cfg, testLogger())

		bus := &mockBus{}
		agent.bus = bus

		now := time.Now()

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return []store.Memory{
					{ID: "a1", Salience: 0.8, AccessCount: 0, LastAccessed: now.Add(-200 * time.Hour), CreatedAt: now.Add(-200 * time.Hour), State: "active"},
					{ID: "a2", Salience: 0.2, AccessCount: 0, LastAccessed: now.Add(-200 * time.Hour), CreatedAt: now.Add(-200 * time.Hour), State: "active"},
					{ID: "a3", Salience: 0.05, AccessCount: 0, LastAccessed: now.Add(-200 * time.Hour), CreatedAt: now.Add(-200 * time.Hour), State: "active"},
				}, nil
			}
			if state == "fading" {
				return []store.Memory{
					{ID: "f1", Salience: 0.08, AccessCount: 0, LastAccessed: now.Add(-200 * time.Hour), CreatedAt: now.Add(-200 * time.Hour), State: "fading"},
				}, nil
			}
			return nil, nil
		}
		ms.getMemoryAttributesFn = func(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
			return store.MemoryAttributes{}, fmt.Errorf("not found")
		}
		ms.pruneWeakAssociationsFn = func(ctx context.Context, threshold float32) (int, error) {
			return 3, nil
		}
		ms.deleteOldArchivedFn = func(ctx context.Context, olderThan time.Time) (int, error) {
			return 2, nil
		}

		report, err := agent.runCycle(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify report fields
		if report == nil {
			t.Fatal("expected non-nil report")
		}
		if report.MemoriesProcessed != 4 { // 3 active + 1 fading
			t.Errorf("expected 4 processed, got %d", report.MemoriesProcessed)
		}
		if report.MemoriesDecayed < 1 {
			t.Errorf("expected at least 1 decayed, got %d", report.MemoriesDecayed)
		}
		if report.AssociationsPruned != 3 {
			t.Errorf("expected 3 associations pruned, got %d", report.AssociationsPruned)
		}
		if report.ExpiredDeleted != 2 {
			t.Errorf("expected 2 expired deleted, got %d", report.ExpiredDeleted)
		}
		if report.Duration <= 0 {
			t.Error("expected positive duration")
		}

		// Verify consolidation record was written
		if len(ms.writeConsolidationCalls) != 1 {
			t.Errorf("expected 1 WriteConsolidation call, got %d", len(ms.writeConsolidationCalls))
		}

		// Verify ConsolidationCompleted event was published (no ConsolidationStarted — that's a request event from other agents)
		if len(bus.published) < 1 {
			t.Errorf("expected at least 1 event (completed), got %d", len(bus.published))
		}
		if _, ok := bus.published[len(bus.published)-1].(events.ConsolidationCompleted); !ok {
			t.Errorf("expected last event to be ConsolidationCompleted, got %T", bus.published[len(bus.published)-1])
		}
	})

	t.Run("cycle with no bus does not panic", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())
		// bus is nil

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			return nil, nil
		}

		report, err := agent.runCycle(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report == nil {
			t.Fatal("expected non-nil report")
		}
	})

	t.Run("cycle handles decay error", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			if state == "active" {
				return nil, fmt.Errorf("db connection lost")
			}
			return nil, nil
		}

		report, err := agent.runCycle(context.Background())
		if err == nil {
			t.Fatal("expected error from failed decay")
		}
		if report != nil {
			t.Error("expected nil report on error")
		}
	})

	t.Run("RunOnce delegates to runCycle", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			return nil, nil
		}

		report, err := agent.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report == nil {
			t.Fatal("expected non-nil report from RunOnce")
		}
	})

	t.Run("RunConsolidation delegates to runCycle", func(t *testing.T) {
		ms := newMockStore()
		mlp := newMockLLMProvider()
		agent := NewConsolidationAgent(ms, mlp, testConfig(), testLogger())

		ms.listMemoriesFn = func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
			return nil, nil
		}

		err := agent.RunConsolidation(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExtractJSONFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON object",
			input:    `{"summary":"test","content":"hello"}`,
			expected: `{"summary":"test","content":"hello"}`,
		},
		{
			name:     "JSON with leading whitespace",
			input:    `   {"summary":"test"}`,
			expected: `{"summary":"test"}`,
		},
		{
			name:     "JSON in json code fence",
			input:    "Here is the result:\n```json\n{\"summary\":\"test\"}\n```",
			expected: `{"summary":"test"}`,
		},
		{
			name:     "JSON in plain code fence",
			input:    "Sure:\n```\n{\"summary\":\"test\"}\n```",
			expected: `{"summary":"test"}`,
		},
		{
			name:     "JSON with surrounding prose",
			input:    "Here is the merged summary: {\"summary\":\"merged\",\"content\":\"details\"} Hope that helps!",
			expected: `{"summary":"merged","content":"details"}`,
		},
		{
			name:     "no JSON at all",
			input:    "Just some text without JSON",
			expected: "Just some text without JSON",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "JSON with nested braces",
			input:    `prefix {"outer":{"inner":"val"}} suffix`,
			expected: `{"outer":{"inner":"val"}}`,
		},
		{
			name:     "multiple json code fences returns first",
			input:    "```json\n{\"first\":true}\n```\n\n```json\n{\"second\":true}\n```",
			expected: `{"first":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractJSONFromResponse(tc.input)
			if result != tc.expected {
				t.Errorf("extractJSONFromResponse(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	t.Run("valid JSON parses correctly", func(t *testing.T) {
		var result struct {
			Summary string `json:"summary"`
			Content string `json:"content"`
		}
		err := parseJSON(`{"summary":"hello","content":"world"}`, &result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Summary != "hello" {
			t.Errorf("expected summary %q, got %q", "hello", result.Summary)
		}
		if result.Content != "world" {
			t.Errorf("expected content %q, got %q", "world", result.Content)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		var result struct {
			Summary string `json:"summary"`
		}
		err := parseJSON(`not json`, &result)
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("empty JSON object parses to zero values", func(t *testing.T) {
		var result struct {
			Summary string `json:"summary"`
			Count   int    `json:"count"`
		}
		err := parseJSON(`{}`, &result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Summary != "" {
			t.Errorf("expected empty summary, got %q", result.Summary)
		}
		if result.Count != 0 {
			t.Errorf("expected count 0, got %d", result.Count)
		}
	})

	t.Run("extra fields are ignored", func(t *testing.T) {
		var result struct {
			Summary string `json:"summary"`
		}
		err := parseJSON(`{"summary":"test","extra":"field","num":42}`, &result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Summary != "test" {
			t.Errorf("expected summary %q, got %q", "test", result.Summary)
		}
	})

	t.Run("empty string returns error", func(t *testing.T) {
		var result struct{}
		err := parseJSON("", &result)
		if err == nil {
			t.Fatal("expected error for empty string, got nil")
		}
	})
}
