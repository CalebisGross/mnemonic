package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/store/storetest"
)

// mockStore embeds the shared base mock and has no overrides.
type mockStore struct {
	storetest.MockStore
}

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
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0, nil, "")

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
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0, nil, "")

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

	if len(toolsArray) != 22 {
		t.Fatalf("expected 22 tools, got %d", len(toolsArray))
	}

	// Verify tool names
	expectedTools := map[string]bool{
		"remember":        false,
		"recall":          false,
		"batch_recall":    false,
		"get_context":     false,
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
		"list_sessions":   false,
		"recall_session":  false,
		"amend":           false,
		"check_memory":    false,
		"exclude_path":    false,
		"list_exclusions": false,
		"create_handoff":  false,
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
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0, nil, "")

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

// TestFormatDuration tests human-readable duration formatting.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 3*time.Minute + 30*time.Second, "3m"},
		{"hours and minutes", 2*time.Hour + 15*time.Minute, "2h15m"},
		{"zero", 0, "0s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatDuration(tc.d)
			if got != tc.expected {
				t.Fatalf("formatDuration(%v) = %q, want %q", tc.d, got, tc.expected)
			}
		})
	}
}

// TestCheckAcceptance tests that suggested IDs are detected in recall results.
func TestCheckAcceptance(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewMCPServer(&mockStore{}, nil, &mockBus{}, logger, "test", "", []string{}, 0, nil, "")

	// Simulate get_context suggesting two memory IDs.
	srv.contextSuggestedIDs["abc-123"] = time.Now()
	srv.contextSuggestedIDs["def-456"] = time.Now()
	srv.contextTotalOffered = 2

	// Simulate a recall result containing one of the suggested IDs.
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": "Found memory abc-123: some relevant context"},
		},
	}
	srv.checkAcceptance(result)

	if srv.contextAccepted != 1 {
		t.Fatalf("expected 1 acceptance, got %d", srv.contextAccepted)
	}
	if _, exists := srv.contextSuggestedIDs["abc-123"]; exists {
		t.Fatal("expected abc-123 to be removed from suggested IDs after acceptance")
	}
	if _, exists := srv.contextSuggestedIDs["def-456"]; !exists {
		t.Fatal("expected def-456 to still be in suggested IDs")
	}
}

// TestContextMetricsJSON tests that contextMetrics marshals all expected fields.
func TestContextMetricsJSON(t *testing.T) {
	m := contextMetrics{
		EncodedCount:     6,
		FallbackCount:    2,
		CoveragePct:      75.0,
		CandidatesBefore: 12,
		CandidatesAfter:  5,
		NoveltyPct:       41.67,
		ThemeHits:        map[string]int{"retrieval": 4, "mcp": 3},
		AvgEncodeLatMs:   4500.0,
		OldestUnencoded:  "2m",
		QueueDepth:       3,
		AcceptancePct:    60.0,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal contextMetrics: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	checks := []string{
		"encoded_count", "fallback_count", "encoding_coverage_pct",
		"candidates_before_dedup", "candidates_after_dedup", "novelty_pct",
		"theme_match_counts", "avg_encode_latency_ms", "oldest_unencoded_age",
		"encoding_queue_depth", "acceptance_rate_pct",
	}
	for _, key := range checks {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in JSON output", key)
		}
	}
}
