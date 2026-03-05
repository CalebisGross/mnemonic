package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

// ---------------------------------------------------------------------------
// Mock store
// ---------------------------------------------------------------------------

// mockStore implements store.Store with configurable behavior per-test.
type mockStore struct {
	writeRawFn      func(ctx context.Context, raw store.RawMemory) error
	getMemoryFn     func(ctx context.Context, id string) (store.Memory, error)
	listMemoriesFn  func(ctx context.Context, state string, limit, offset int) ([]store.Memory, error)
	countMemoriesFn func(ctx context.Context) (int, error)
	getStatisticsFn func(ctx context.Context) (store.StoreStatistics, error)
}

func (m *mockStore) WriteRaw(ctx context.Context, raw store.RawMemory) error {
	if m.writeRawFn != nil {
		return m.writeRawFn(ctx, raw)
	}
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
func (m *mockStore) MarkRawProcessed(ctx context.Context, id string) error { return nil }
func (m *mockStore) WriteMemory(ctx context.Context, mem store.Memory) error {
	return nil
}
func (m *mockStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	if m.getMemoryFn != nil {
		return m.getMemoryFn(ctx, id)
	}
	return store.Memory{}, nil
}
func (m *mockStore) GetMemoryByRawID(ctx context.Context, rawID string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) UpdateMemory(ctx context.Context, mem store.Memory) error { return nil }
func (m *mockStore) UpdateSalience(ctx context.Context, id string, salience float32) error {
	return nil
}
func (m *mockStore) UpdateState(ctx context.Context, id string, state string) error { return nil }
func (m *mockStore) IncrementAccess(ctx context.Context, id string) error           { return nil }
func (m *mockStore) ListMemories(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
	if m.listMemoriesFn != nil {
		return m.listMemoriesFn(ctx, state, limit, offset)
	}
	return nil, nil
}
func (m *mockStore) CountMemories(ctx context.Context) (int, error) {
	if m.countMemoriesFn != nil {
		return m.countMemoriesFn(ctx)
	}
	return 0, nil
}
func (m *mockStore) SearchByFullText(ctx context.Context, query string, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
	return nil, nil
}
func (m *mockStore) SearchByConcepts(ctx context.Context, concepts []string, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) CreateAssociation(ctx context.Context, assoc store.Association) error { return nil }
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
func (m *mockStore) PruneWeakAssociations(ctx context.Context, threshold float32) (int, error) {
	return 0, nil
}
func (m *mockStore) BatchUpdateSalience(ctx context.Context, updates map[string]float32) error {
	return nil
}
func (m *mockStore) BatchMergeMemories(ctx context.Context, sourceIDs []string, gist store.Memory) error {
	return nil
}
func (m *mockStore) DeleteOldArchived(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}
func (m *mockStore) WriteConsolidation(ctx context.Context, record store.ConsolidationRecord) error {
	return nil
}
func (m *mockStore) GetLastConsolidation(ctx context.Context) (store.ConsolidationRecord, error) {
	return store.ConsolidationRecord{}, nil
}
func (m *mockStore) ListAllAssociations(ctx context.Context) ([]store.Association, error) {
	return nil, nil
}
func (m *mockStore) ListAllRawMemories(ctx context.Context) ([]store.RawMemory, error) {
	return nil, nil
}
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
func (m *mockStore) GetStatistics(ctx context.Context) (store.StoreStatistics, error) {
	if m.getStatisticsFn != nil {
		return m.getStatisticsFn(ctx)
	}
	return store.StoreStatistics{}, nil
}
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
func (m *mockStore) WriteMemoryResolution(ctx context.Context, res store.MemoryResolution) error {
	return nil
}
func (m *mockStore) GetMemoryResolution(ctx context.Context, memoryID string) (store.MemoryResolution, error) {
	return store.MemoryResolution{}, nil
}
func (m *mockStore) WriteConceptSet(ctx context.Context, cs store.ConceptSet) error { return nil }
func (m *mockStore) GetConceptSet(ctx context.Context, memoryID string) (store.ConceptSet, error) {
	return store.ConceptSet{}, nil
}
func (m *mockStore) SearchByEntity(ctx context.Context, name string, entityType string, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) WriteMemoryAttributes(ctx context.Context, attrs store.MemoryAttributes) error {
	return nil
}
func (m *mockStore) GetMemoryAttributes(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
	return store.MemoryAttributes{}, nil
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

func (m *mockStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// Mock event bus
// ---------------------------------------------------------------------------

type mockBus struct {
	published []events.Event
}

func (b *mockBus) Publish(_ context.Context, event events.Event) error {
	b.published = append(b.published, event)
	return nil
}
func (b *mockBus) Subscribe(eventType string, handler events.Handler) string { return "sub-1" }
func (b *mockBus) Unsubscribe(subscriptionID string)                         {}
func (b *mockBus) Close() error                                              { return nil }

// ---------------------------------------------------------------------------
// Mock LLM provider
// ---------------------------------------------------------------------------

type mockLLMProvider struct{}

func (p *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: "mock response"}, nil
}
func (p *mockLLMProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}
func (p *mockLLMProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, nil
}
func (p *mockLLMProvider) Health(ctx context.Context) error {
	return nil
}
func (p *mockLLMProvider) ModelInfo(ctx context.Context) (llm.ModelMetadata, error) {
	return llm.ModelMetadata{Name: "mock"}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------------------------------
// Tests for HandleCreateMemory
// ---------------------------------------------------------------------------

func TestHandleCreateMemory(t *testing.T) {
	t.Run("valid request returns 201 with correct JSON", func(t *testing.T) {
		var captured store.RawMemory
		ms := &mockStore{
			writeRawFn: func(_ context.Context, raw store.RawMemory) error {
				captured = raw
				return nil
			},
		}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"content": "remember this fact", "source": "cli"}`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", rr.Code)
		}

		var resp CreateMemoryResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ID == "" {
			t.Error("expected non-empty ID in response")
		}
		if resp.Source != "cli" {
			t.Errorf("expected source 'cli', got %q", resp.Source)
		}
		if resp.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}

		// Verify the raw memory was written with expected fields.
		if captured.Content != "remember this fact" {
			t.Errorf("expected content 'remember this fact', got %q", captured.Content)
		}
		if captured.Source != "cli" {
			t.Errorf("expected source 'cli' on raw memory, got %q", captured.Source)
		}

		// Verify an event was published.
		if len(bus.published) == 0 {
			t.Fatal("expected at least one event to be published")
		}
		if bus.published[0].EventType() != events.TypeRawMemoryCreated {
			t.Errorf("expected event type %q, got %q", events.TypeRawMemoryCreated, bus.published[0].EventType())
		}
	})

	t.Run("default source is user when omitted", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"content": "some content"}`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", rr.Code)
		}
		var resp CreateMemoryResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Source != "web_ui" {
			t.Errorf("expected default source 'web_ui', got %q", resp.Source)
		}
	})
}

func TestHandleCreateMemoryEmptyContent(t *testing.T) {
	t.Run("missing content returns 400", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"content": "", "source": "user"}`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rr.Code)
		}

		var errResp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if errResp["code"] != "MISSING_FIELD" {
			t.Errorf("expected code 'MISSING_FIELD', got %v", errResp["code"])
		}
	})

	t.Run("content field absent returns 400", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"source": "user"}`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rr.Code)
		}
	})
}

func TestHandleCreateMemoryInvalidJSON(t *testing.T) {
	t.Run("garbage body returns 400", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `this is not json at all!!!`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rr.Code)
		}

		var errResp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if errResp["code"] != "INVALID_REQUEST" {
			t.Errorf("expected code 'INVALID_REQUEST', got %v", errResp["code"])
		}
	})

	t.Run("truncated JSON returns 400", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"content": "hello`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rr.Code)
		}
	})
}

func TestHandleCreateMemoryOversizedBody(t *testing.T) {
	t.Run("body exceeding 1MB is rejected", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		// Build a JSON body just over 1MB. The MaxBytesReader limit is 1<<20 = 1048576 bytes.
		bigContent := strings.Repeat("x", 1<<20+100)
		body := fmt.Sprintf(`{"content": "%s"}`, bigContent)
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400 for oversized body, got %d", rr.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for HandleListMemories
// ---------------------------------------------------------------------------

func TestHandleListMemories(t *testing.T) {
	t.Run("returns list with defaults", func(t *testing.T) {
		now := time.Now()
		testMemories := []store.Memory{
			{ID: "mem-1", Content: "first memory", Summary: "first memory", State: "active", CreatedAt: now},
			{ID: "mem-2", Content: "second memory", Summary: "second memory", State: "active", CreatedAt: now},
		}

		ms := &mockStore{
			listMemoriesFn: func(_ context.Context, state string, limit, offset int) ([]store.Memory, error) {
				if state != "active" {
					t.Errorf("expected default state 'active', got %q", state)
				}
				if limit != 50 {
					t.Errorf("expected default limit 50, got %d", limit)
				}
				if offset != 0 {
					t.Errorf("expected default offset 0, got %d", offset)
				}
				return testMemories, nil
			},
		}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp ListMemoriesResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Count != 2 {
			t.Errorf("expected count 2, got %d", resp.Count)
		}
		if resp.Limit != 50 {
			t.Errorf("expected limit 50 in response, got %d", resp.Limit)
		}
		if resp.Offset != 0 {
			t.Errorf("expected offset 0 in response, got %d", resp.Offset)
		}
		if len(resp.Memories) != 2 {
			t.Fatalf("expected 2 memories, got %d", len(resp.Memories))
		}
		if resp.Memories[0].ID != "mem-1" {
			t.Errorf("expected first memory ID 'mem-1', got %q", resp.Memories[0].ID)
		}
	})

	t.Run("returns empty array when no memories", func(t *testing.T) {
		ms := &mockStore{
			listMemoriesFn: func(_ context.Context, _ string, _, _ int) ([]store.Memory, error) {
				return nil, nil
			},
		}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp ListMemoriesResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Memories == nil {
			t.Error("expected non-nil memories array (empty, not null)")
		}
		if resp.Count != 0 {
			t.Errorf("expected count 0, got %d", resp.Count)
		}
	})
}

func TestHandleListMemoriesWithParams(t *testing.T) {
	t.Run("state, limit, offset query params are forwarded", func(t *testing.T) {
		ms := &mockStore{
			listMemoriesFn: func(_ context.Context, state string, limit, offset int) ([]store.Memory, error) {
				if state != "fading" {
					t.Errorf("expected state 'fading', got %q", state)
				}
				if limit != 10 {
					t.Errorf("expected limit 10, got %d", limit)
				}
				if offset != 5 {
					t.Errorf("expected offset 5, got %d", offset)
				}
				return []store.Memory{
					{ID: "mem-fading-1", Summary: "fading memory", State: "fading"},
				}, nil
			},
		}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories?state=fading&limit=10&offset=5", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp ListMemoriesResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Limit != 10 {
			t.Errorf("expected limit 10 in response, got %d", resp.Limit)
		}
		if resp.Offset != 5 {
			t.Errorf("expected offset 5 in response, got %d", resp.Offset)
		}
		if resp.Count != 1 {
			t.Errorf("expected count 1, got %d", resp.Count)
		}
	})

	t.Run("out-of-range limit is clamped to default", func(t *testing.T) {
		ms := &mockStore{
			listMemoriesFn: func(_ context.Context, _ string, limit, _ int) ([]store.Memory, error) {
				if limit != 50 {
					t.Errorf("expected clamped limit 50, got %d", limit)
				}
				return nil, nil
			},
		}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories?limit=5000", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("negative offset is clamped to zero", func(t *testing.T) {
		ms := &mockStore{
			listMemoriesFn: func(_ context.Context, _ string, _, offset int) ([]store.Memory, error) {
				if offset != 0 {
					t.Errorf("expected clamped offset 0, got %d", offset)
				}
				return nil, nil
			},
		}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories?offset=-10", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for HandleGetMemory
// ---------------------------------------------------------------------------

func TestHandleGetMemory(t *testing.T) {
	t.Run("valid ID returns memory", func(t *testing.T) {
		now := time.Now()
		expected := store.Memory{
			ID:        "mem-abc-123",
			Content:   "test memory content",
			Summary:   "test summary",
			State:     "active",
			Salience:  0.8,
			Concepts:  []string{"go", "testing"},
			CreatedAt: now,
			UpdatedAt: now,
		}
		ms := &mockStore{
			getMemoryFn: func(_ context.Context, id string) (store.Memory, error) {
				if id != "mem-abc-123" {
					return store.Memory{}, store.ErrNotFound
				}
				return expected, nil
			},
		}
		handler := HandleGetMemory(ms, testLogger())

		// Use Go 1.22+ SetPathValue to provide the {id} path parameter.
		req := httptest.NewRequest(http.MethodGet, "/memories/mem-abc-123", nil)
		req.SetPathValue("id", "mem-abc-123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
		}

		var resp GetMemoryResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Memory.ID != "mem-abc-123" {
			t.Errorf("expected memory ID 'mem-abc-123', got %q", resp.Memory.ID)
		}
		if resp.Memory.Content != "test memory content" {
			t.Errorf("expected content 'test memory content', got %q", resp.Memory.Content)
		}
		if resp.Memory.State != "active" {
			t.Errorf("expected state 'active', got %q", resp.Memory.State)
		}
	})
}

func TestHandleGetMemoryNotFound(t *testing.T) {
	t.Run("missing ID returns 404", func(t *testing.T) {
		ms := &mockStore{
			getMemoryFn: func(_ context.Context, id string) (store.Memory, error) {
				return store.Memory{}, store.ErrNotFound
			},
		}
		handler := HandleGetMemory(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories/nonexistent", nil)
		req.SetPathValue("id", "nonexistent")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d; body: %s", rr.Code, rr.Body.String())
		}

		var errResp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if errResp["code"] != "NOT_FOUND" {
			t.Errorf("expected code 'NOT_FOUND', got %v", errResp["code"])
		}
	})

	t.Run("empty path value returns 400", func(t *testing.T) {
		ms := &mockStore{}
		handler := HandleGetMemory(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories/", nil)
		// Do not set a path value -- simulates missing {id}.
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for HandleHealth
// ---------------------------------------------------------------------------

func TestHandleHealthCheck(t *testing.T) {
	t.Run("healthy system returns 200 with status ok", func(t *testing.T) {
		ms := &mockStore{
			countMemoriesFn: func(_ context.Context) (int, error) {
				return 42, nil
			},
		}
		llmProv := &mockLLMProvider{}
		handler := HandleHealth(ms, llmProv, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp HealthResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "ok" {
			t.Errorf("expected status 'ok', got %q", resp.Status)
		}
		if !resp.LLMAvailable {
			t.Error("expected LLMAvailable to be true")
		}
		if !resp.StoreHealthy {
			t.Error("expected StoreHealthy to be true")
		}
		if resp.MemoryCount != 42 {
			t.Errorf("expected MemoryCount 42, got %d", resp.MemoryCount)
		}
		if resp.Timestamp == "" {
			t.Error("expected non-empty timestamp")
		}
	})

	t.Run("degraded when LLM is unavailable", func(t *testing.T) {
		ms := &mockStore{
			countMemoriesFn: func(_ context.Context) (int, error) {
				return 10, nil
			},
		}
		llmProv := &failingLLMProvider{}
		handler := HandleHealth(ms, llmProv, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp HealthResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "degraded" {
			t.Errorf("expected status 'degraded', got %q", resp.Status)
		}
		if resp.LLMAvailable {
			t.Error("expected LLMAvailable to be false")
		}
		if !resp.StoreHealthy {
			t.Error("expected StoreHealthy to be true even when LLM is down")
		}
	})

	t.Run("degraded when store is unhealthy", func(t *testing.T) {
		ms := &mockStore{
			countMemoriesFn: func(_ context.Context) (int, error) {
				return 0, fmt.Errorf("db connection refused")
			},
		}
		llmProv := &mockLLMProvider{}
		handler := HandleHealth(ms, llmProv, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp HealthResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "degraded" {
			t.Errorf("expected status 'degraded', got %q", resp.Status)
		}
		if !resp.LLMAvailable {
			t.Error("expected LLMAvailable to be true")
		}
		if resp.StoreHealthy {
			t.Error("expected StoreHealthy to be false")
		}
		if resp.MemoryCount != 0 {
			t.Errorf("expected MemoryCount 0 on store failure, got %d", resp.MemoryCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for HandleCreateMemory store error
// ---------------------------------------------------------------------------

func TestHandleCreateMemoryStoreError(t *testing.T) {
	t.Run("store write failure returns 500", func(t *testing.T) {
		ms := &mockStore{
			writeRawFn: func(_ context.Context, _ store.RawMemory) error {
				return fmt.Errorf("disk full")
			},
		}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"content": "test content"}`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", rr.Code)
		}

		var errResp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if errResp["code"] != "STORE_ERROR" {
			t.Errorf("expected code 'STORE_ERROR', got %v", errResp["code"])
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for HandleListMemories store error
// ---------------------------------------------------------------------------

func TestHandleListMemoriesStoreError(t *testing.T) {
	t.Run("store list failure returns 500", func(t *testing.T) {
		ms := &mockStore{
			listMemoriesFn: func(_ context.Context, _ string, _, _ int) ([]store.Memory, error) {
				return nil, fmt.Errorf("connection timeout")
			},
		}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", rr.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for response Content-Type header
// ---------------------------------------------------------------------------

func TestResponseContentType(t *testing.T) {
	t.Run("create memory returns application/json", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		body := `{"content": "test"}`
		req := httptest.NewRequest(http.MethodPost, "/memories", strings.NewReader(body))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		ct := rr.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", ct)
		}
	})

	t.Run("list memories returns application/json", func(t *testing.T) {
		ms := &mockStore{}
		handler := HandleListMemories(ms, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/memories", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		ct := rr.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", ct)
		}
	})

	t.Run("error response returns application/json", func(t *testing.T) {
		ms := &mockStore{}
		bus := &mockBus{}
		handler := HandleCreateMemory(ms, bus, testLogger())

		req := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader([]byte("bad")))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		ct := rr.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", ct)
		}
	})
}

// ---------------------------------------------------------------------------
// failingLLMProvider - an LLM provider whose Health always fails
// ---------------------------------------------------------------------------

type failingLLMProvider struct{}

func (p *failingLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, fmt.Errorf("unavailable")
}
func (p *failingLLMProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("unavailable")
}
func (p *failingLLMProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("unavailable")
}
func (p *failingLLMProvider) Health(ctx context.Context) error {
	return fmt.Errorf("connection refused")
}
func (p *failingLLMProvider) ModelInfo(ctx context.Context) (llm.ModelMetadata, error) {
	return llm.ModelMetadata{}, fmt.Errorf("unavailable")
}
