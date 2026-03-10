package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/store"
)

// mockStore is a minimal mock of the Store interface for testing.
type mockStore struct{}

func (m *mockStore) WriteRaw(ctx context.Context, raw store.RawMemory) error { return nil }
func (m *mockStore) GetRaw(ctx context.Context, id string) (store.RawMemory, error) {
	return store.RawMemory{}, nil
}
func (m *mockStore) ListRawUnprocessed(ctx context.Context, limit int) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) ListRawMemoriesAfter(ctx context.Context, after time.Time, limit int) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) MarkRawProcessed(ctx context.Context, id string) error   { return nil }
func (m *mockStore) WriteMemory(ctx context.Context, mem store.Memory) error { return nil }
func (m *mockStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) GetMemoryByRawID(ctx context.Context, rawID string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) UpdateMemory(ctx context.Context, mem store.Memory) error { return nil }
func (m *mockStore) UpdateSalience(ctx context.Context, id string, salience float32) error {
	return nil
}
func (m *mockStore) UpdateState(ctx context.Context, id string, state string) error {
	return nil
}
func (m *mockStore) IncrementAccess(ctx context.Context, id string) error { return nil }
func (m *mockStore) ListMemories(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) CountMemories(ctx context.Context) (int, error) { return 0, nil }
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
func (m *mockStore) PruneWeakAssociations(ctx context.Context, strengthThreshold float32) (int, error) {
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
	return store.StoreStatistics{}, nil
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
func (m *mockStore) DeleteOldMetaObservations(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

func (m *mockStore) Close() error { return nil }

// mockBus is a minimal mock of the Bus interface for testing.
type mockBus struct{}

func (m *mockBus) Publish(ctx context.Context, event events.Event) error { return nil }
func (m *mockBus) Subscribe(eventType string, handler events.Handler) string {
	return "test-sub-id"
}
func (m *mockBus) Unsubscribe(subscriptionID string) {}
func (m *mockBus) Close() error                      { return nil }

// TestHandleInitialize tests handleInitialize returns correct protocol version and server info.
func TestHandleInitialize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0)

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := srv.handleInitialize(req)

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.JSONRPC != "2.0" {
		t.Fatalf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 1 {
		t.Fatalf("expected ID 1, got %v", resp.ID)
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	// Round-trip through JSON to get standard Go types
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if protocolVersion, ok := result["protocolVersion"]; !ok {
		t.Fatal("protocolVersion not in result")
	} else if protocolVersion != "2024-11-05" {
		t.Fatalf("expected protocol version 2024-11-05, got %v", protocolVersion)
	}

	// Check serverInfo
	if serverInfo, ok := result["serverInfo"]; !ok {
		t.Fatal("serverInfo not in result")
	} else {
		serverInfoMap := serverInfo.(map[string]interface{})
		if serverInfoMap["name"] != "mnemonic" {
			t.Fatalf("expected server name 'mnemonic', got %v", serverInfoMap["name"])
		}
		if serverInfoMap["version"] != "test" {
			t.Fatalf("expected server version 'test', got %v", serverInfoMap["version"])
		}
	}
}

// TestHandleToolsList tests handleToolsList returns all 10 tools.
func TestHandleToolsList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0)

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := srv.handleToolsList(req)

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	// Round-trip through JSON to get standard Go types (like a real MCP client)
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	toolsInterface, ok := result["tools"]
	if !ok {
		t.Fatal("tools not in result")
	}

	toolsArray, ok := toolsInterface.([]interface{})
	if !ok {
		t.Fatalf("tools is not an array, got %T", toolsInterface)
	}

	if len(toolsArray) != 13 {
		t.Fatalf("expected 13 tools, got %d", len(toolsArray))
	}

	// Verify tool names
	expectedTools := map[string]bool{
		"remember":        false,
		"recall":          false,
		"forget":          false,
		"status":          false,
		"recall_project":  false,
		"recall_timeline": false,
		"session_summary": false,
		"get_patterns":    false,
		"get_insights":    false,
		"feedback":        false,
		"audit_encodings": false,
		"coach_local_llm": false,
		"ingest_project":  false,
	}

	for _, toolInterface := range toolsArray {
		toolMap := toolInterface.(map[string]interface{})
		toolName := toolMap["name"].(string)
		if _, ok := expectedTools[toolName]; ok {
			expectedTools[toolName] = true
		} else {
			t.Fatalf("unexpected tool: %s", toolName)
		}
	}

	// Verify all expected tools were found
	for toolName, found := range expectedTools {
		if !found {
			t.Fatalf("tool %s not found", toolName)
		}
	}
}

// TestJSONRPCErrorResponse tests JSON-RPC error response format.
func TestJSONRPCErrorResponse(t *testing.T) {
	resp := errorResponse(42, -32601, "Method not found")

	if resp.JSONRPC != "2.0" {
		t.Fatalf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 42 {
		t.Fatalf("expected ID 42, got %v", resp.ID)
	}

	if resp.Error == nil {
		t.Fatal("expected error object")
	}

	if resp.Error.Code != -32601 {
		t.Fatalf("expected code -32601, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Method not found" {
		t.Fatalf("expected message 'Method not found', got %s", resp.Error.Message)
	}

	if resp.Result != nil {
		t.Fatalf("expected nil result, got %v", resp.Result)
	}
}

// TestToolResult tests toolResult helper function.
func TestToolResult(t *testing.T) {
	text := "Test result text"
	result := toolResult(text)

	contentInterface, ok := result["content"]
	if !ok {
		t.Fatal("content not in result")
	}

	contentArray, ok := contentInterface.([]map[string]interface{})
	if !ok {
		t.Fatal("content is not an array of maps")
	}

	if len(contentArray) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contentArray))
	}

	item := contentArray[0]
	if item["type"] != "text" {
		t.Fatalf("expected type 'text', got %v", item["type"])
	}

	if item["text"] != text {
		t.Fatalf("expected text %q, got %q", text, item["text"])
	}
}

// TestToolError tests toolError helper function.
func TestToolError(t *testing.T) {
	errorText := "Test error"
	result := toolError(errorText)

	if result["isError"] != true {
		t.Fatalf("expected isError=true, got %v", result["isError"])
	}

	contentInterface, ok := result["content"]
	if !ok {
		t.Fatal("content not in result")
	}

	contentArray, ok := contentInterface.([]map[string]interface{})
	if !ok {
		t.Fatal("content is not an array of maps")
	}

	if len(contentArray) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contentArray))
	}

	item := contentArray[0]
	if item["type"] != "text" {
		t.Fatalf("expected type 'text', got %v", item["type"])
	}

	expectedText := "Error: " + errorText
	if item["text"] != expectedText {
		t.Fatalf("expected text %q, got %q", expectedText, item["text"])
	}
}

// TestSuccessResponse tests successResponse helper function.
func TestSuccessResponse(t *testing.T) {
	testResult := map[string]interface{}{"key": "value"}
	resp := successResponse(99, testResult)

	if resp.JSONRPC != "2.0" {
		t.Fatalf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 99 {
		t.Fatalf("expected ID 99, got %v", resp.ID)
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %v", resp.Error)
	}

	// Verify result
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	if resultMap["key"] != "value" {
		t.Fatalf("expected key=value in result, got %v", resultMap)
	}
}

// TestHandleRequestDispatch tests that handleRequest correctly dispatches to handlers.
func TestHandleRequestDispatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0)

	tests := []struct {
		method  string
		wantErr bool
	}{
		{"initialize", false},
		{"tools/list", false},
		{"notifications/initialized", false}, // Returns nil response
		{"invalid_method", true},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			req := &jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  tc.method,
			}

			resp := srv.handleRequest(context.Background(), req)

			if tc.method == "notifications/initialized" {
				if resp != nil {
					t.Fatal("expected nil response for notifications/initialized")
				}
			} else {
				if resp == nil {
					t.Fatal("expected non-nil response")
				}

				if tc.wantErr {
					if resp.Error == nil {
						t.Fatal("expected error in response")
					}
				} else {
					if resp.Error != nil {
						t.Fatalf("expected no error, got %v", resp.Error)
					}
				}
			}
		})
	}
}

// TestJSONRPCMarshal tests that responses can be marshalled to JSON correctly.
func TestJSONRPCMarshal(t *testing.T) {
	resp := errorResponse(1, -32700, "Parse error")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify it's valid JSON
	var unmarshalled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if unmarshalled["jsonrpc"] != "2.0" {
		t.Fatalf("jsonrpc field mismatch")
	}

	if unmarshalled["id"] != float64(1) {
		t.Fatalf("id field mismatch")
	}

	errorObj, ok := unmarshalled["error"].(map[string]interface{})
	if !ok {
		t.Fatal("error field is not a map")
	}

	if errorObj["code"] != float64(-32700) {
		t.Fatalf("error code mismatch")
	}

	if errorObj["message"] != "Parse error" {
		t.Fatalf("error message mismatch")
	}
}
