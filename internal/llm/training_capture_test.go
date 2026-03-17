package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockProvider is a minimal Provider for testing.
type mockProvider struct {
	completeResp CompletionResponse
	completeErr  error
}

func (m *mockProvider) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return m.completeResp, m.completeErr
}

func (m *mockProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func (m *mockProvider) BatchEmbed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

func (m *mockProvider) Health(_ context.Context) error { return nil }

func (m *mockProvider) ModelInfo(_ context.Context) (ModelMetadata, error) {
	return ModelMetadata{Name: "test"}, nil
}

func TestTrainingCaptureProvider_CapturesCompletion(t *testing.T) {
	dir := t.TempDir()

	inner := &mockProvider{
		completeResp: CompletionResponse{
			Content:          `{"gist":"test gist","summary":"test summary"}`,
			StopReason:       "stop",
			PromptTokens:     100,
			CompletionTokens: 50,
		},
	}

	p := NewTrainingCaptureProvider(inner, "encoding", dir)
	defer func() { _ = p.Close() }()

	if !p.enabled {
		t.Fatal("capture should be enabled with valid dir")
	}

	req := CompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a memory encoder. Produce structured_concepts and a gist."},
			{Role: "user", Content: "User saved a file"},
		},
		Model: "gemini-3-flash",
		ResponseFormat: &ResponseFormat{
			Type: "json_schema",
			JSONSchema: &JSONSchema{
				Name:   "encoding_response",
				Strict: true,
				Schema: json.RawMessage(`{"type":"object"}`),
			},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
		TopP:        0.9,
		Stop:        []string{"\n\n"},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != inner.completeResp.Content {
		t.Fatal("response should pass through from inner provider")
	}

	// Close to flush
	_ = p.Close()
	p.enabled = false // prevent double-close issues

	// Read the captured file
	files, err := filepath.Glob(filepath.Join(dir, "capture_*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatal("expected capture file to exist")
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("failed to read capture file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 captured example, got %d", len(lines))
	}

	var example TrainingExample
	if err := json.Unmarshal([]byte(lines[0]), &example); err != nil {
		t.Fatalf("failed to parse captured example: %v", err)
	}

	// Task type and caller
	if example.TaskType != "encoding" {
		t.Errorf("expected task_type 'encoding', got %q", example.TaskType)
	}
	if example.Caller != "encoding" {
		t.Errorf("expected caller 'encoding', got %q", example.Caller)
	}

	// Parse success
	if !example.ParseSuccess {
		t.Error("expected parse_success=true for valid JSON response")
	}

	// Token counts
	if example.PromptTokens != 100 {
		t.Errorf("expected prompt_tokens=100, got %d", example.PromptTokens)
	}
	if example.CompletionToks != 50 {
		t.Errorf("expected completion_tokens=50, got %d", example.CompletionToks)
	}

	// Request fields (including previously missing ones)
	if len(example.Request.Messages) != 2 {
		t.Errorf("expected 2 messages captured, got %d", len(example.Request.Messages))
	}
	if example.Request.Model != "gemini-3-flash" {
		t.Errorf("expected model 'gemini-3-flash', got %q", example.Request.Model)
	}
	if example.Request.TopP != 0.9 {
		t.Errorf("expected top_p=0.9, got %v", example.Request.TopP)
	}
	if len(example.Request.Stop) != 1 || example.Request.Stop[0] != "\n\n" {
		t.Errorf("expected stop=[\\n\\n], got %v", example.Request.Stop)
	}
	if example.Request.Temperature != 0.3 {
		t.Errorf("expected temperature=0.3, got %v", example.Request.Temperature)
	}
	if example.Request.MaxTokens != 1024 {
		t.Errorf("expected max_tokens=1024, got %d", example.Request.MaxTokens)
	}

	// Response fields (including previously missing StopReason)
	if example.Response.StopReason != "stop" {
		t.Errorf("expected stop_reason='stop', got %q", example.Response.StopReason)
	}
}

func TestTrainingCaptureProvider_DisabledWithEmptyDir(t *testing.T) {
	inner := &mockProvider{
		completeResp: CompletionResponse{Content: "hello"},
	}

	p := NewTrainingCaptureProvider(inner, "test", "")
	if p.enabled {
		t.Error("capture should be disabled with empty dir")
	}

	// Should still pass through to inner
	resp, err := p.Complete(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello" {
		t.Error("response should pass through even when capture disabled")
	}
}

func TestClassifyTask(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		tools    []Tool
		expected string
	}{
		{
			name: "encoding prompt (system)",
			messages: []Message{
				{Role: "system", Content: "You are a memory encoder. Produce a gist and structured_concepts."},
			},
			expected: "encoding",
		},
		{
			name: "synthesis prompt (system with tools)",
			messages: []Message{
				{Role: "system", Content: "Summarize what the memories tell you."},
			},
			tools:    []Tool{{Type: "function"}},
			expected: "synthesis",
		},
		{
			name: "synthesis prompt (user-only, no system)",
			messages: []Message{
				{Role: "user", Content: "Summarize what the memories tell you about this topic."},
			},
			expected: "synthesis",
		},
		{
			name: "synthesis inferred from tools (no keywords)",
			messages: []Message{
				{Role: "user", Content: "Find relevant information about authentication."},
			},
			tools:    []Tool{{Type: "function", Function: ToolFunction{Name: "search_memories"}}},
			expected: "synthesis",
		},
		{
			name: "abstraction prompt (patterns)",
			messages: []Message{
				{Role: "system", Content: "These patterns have emerged from work. Is there a principle?"},
			},
			expected: "abstraction",
		},
		{
			name: "abstraction prompt (principle synthesizer)",
			messages: []Message{
				{Role: "system", Content: "You are a principle synthesizer. Extract general principles from patterns. Output JSON only."},
			},
			expected: "abstraction",
		},
		{
			name: "consolidation prompt",
			messages: []Message{
				{Role: "system", Content: "You are a memory consolidator. Merge related memories into a single summary. Output JSON only."},
			},
			expected: "consolidation",
		},
		{
			name: "perception prompt",
			messages: []Message{
				{Role: "system", Content: "Evaluate if this is worth remembering."},
			},
			expected: "perception",
		},
		{
			name: "classification prompt",
			messages: []Message{
				{Role: "system", Content: "classify the relationship between these memories. Return relation_type."},
			},
			expected: "classification",
		},
		{
			name: "episoding prompt",
			messages: []Message{
				{Role: "system", Content: "You are an episode synthesizer. Create an episode summary."},
			},
			expected: "episoding",
		},
		{
			name: "unknown prompt",
			messages: []Message{
				{Role: "system", Content: "You are a helpful assistant."},
			},
			expected: "unknown",
		},
		{
			name: "case insensitive matching",
			messages: []Message{
				{Role: "system", Content: "YOU ARE A MEMORY ENCODER. PRODUCE STRUCTURED_CONCEPTS."},
			},
			expected: "encoding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := CompletionRequest{
				Messages: tt.messages,
				Tools:    tt.tools,
			}
			got := classifyTask(req)
			if got != tt.expected {
				t.Errorf("classifyTask() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTrainingCaptureProvider_RecordsParseFailure(t *testing.T) {
	dir := t.TempDir()

	inner := &mockProvider{
		completeResp: CompletionResponse{
			Content: "not valid json at all",
		},
	}

	p := NewTrainingCaptureProvider(inner, "encoding", dir)
	defer func() { _ = p.Close() }()

	req := CompletionRequest{
		Messages: []Message{{Role: "system", Content: "memory encoder with gist and structured_concepts"}},
		ResponseFormat: &ResponseFormat{
			Type:       "json_schema",
			JSONSchema: &JSONSchema{Name: "test", Strict: true},
		},
	}

	_, _ = p.Complete(context.Background(), req)
	_ = p.Close()
	p.enabled = false

	files, _ := filepath.Glob(filepath.Join(dir, "capture_*.jsonl"))
	data, _ := os.ReadFile(files[0])

	var example TrainingExample
	_ = json.Unmarshal([]byte(strings.TrimSpace(string(data))), &example)

	if example.ParseSuccess {
		t.Error("expected parse_success=false for invalid JSON response")
	}
}

func TestTrainingCaptureProvider_FileRollover(t *testing.T) {
	dir := t.TempDir()

	inner := &mockProvider{
		completeResp: CompletionResponse{Content: "hello"},
	}

	p := NewTrainingCaptureProvider(inner, "test", dir)
	defer func() { _ = p.Close() }()

	if !p.enabled {
		t.Fatal("capture should be enabled")
	}

	// Verify fileDate is set
	if p.fileDate == "" {
		t.Fatal("expected fileDate to be set")
	}

	// Simulate a date rollover by manually changing the tracked date
	p.mu.Lock()
	p.fileDate = "2020-01-01" // force a stale date
	p.mu.Unlock()

	// Next capture should trigger rollover
	_, _ = p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "test"}},
	})

	// The file should now have today's date
	p.mu.Lock()
	currentDate := p.fileDate
	p.mu.Unlock()

	if currentDate == "2020-01-01" {
		t.Error("expected file to roll over to today's date")
	}

	// Verify today's file exists
	todayFile := filepath.Join(dir, "capture_"+currentDate+".jsonl")
	if _, err := os.Stat(todayFile); os.IsNotExist(err) {
		t.Errorf("expected today's capture file to exist: %s", todayFile)
	}
}
