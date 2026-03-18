package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TrainingExample captures a full LLM request/response pair for training data generation.
type TrainingExample struct {
	Timestamp      time.Time        `json:"timestamp"`
	TaskType       string           `json:"task_type"` // "encoding", "synthesis", "abstraction", "perception", "classification"
	Caller         string           `json:"caller"`
	Request        TrainingRequest  `json:"request"`
	Response       TrainingResponse `json:"response"`
	ParseSuccess   bool             `json:"parse_success"`
	LatencyMs      int64            `json:"latency_ms"`
	PromptTokens   int              `json:"prompt_tokens"`
	CompletionToks int              `json:"completion_tokens"`
	Error          string           `json:"error,omitempty"`
}

// TrainingRequest captures the input side of an LLM call.
type TrainingRequest struct {
	Messages       []Message       `json:"messages"`
	Model          string          `json:"model,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float32         `json:"temperature,omitempty"`
	TopP           float32         `json:"top_p,omitempty"`
	Stop           []string        `json:"stop,omitempty"`
}

// TrainingResponse captures the output side of an LLM call.
type TrainingResponse struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason,omitempty"`
}

// TrainingCaptureProvider wraps a Provider to capture full request/response pairs
// as JSONL training data for fine-tuning a bespoke local model.
type TrainingCaptureProvider struct {
	inner    Provider
	caller   string
	dir      string
	mu       sync.Mutex
	file     *os.File
	fileDate string // tracks which date the current file belongs to
	enabled  bool
}

// NewTrainingCaptureProvider creates a capture-enabled provider wrapper.
// dir is the directory to write JSONL files into (e.g., ~/.mnemonic/training-data).
// If dir is empty or creation fails, capture is silently disabled.
func NewTrainingCaptureProvider(inner Provider, caller, dir string) *TrainingCaptureProvider {
	p := &TrainingCaptureProvider{
		inner:  inner,
		caller: caller,
		dir:    dir,
	}

	if dir == "" {
		return p
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		slog.Warn("training capture disabled: cannot create directory", "dir", dir, "error", err)
		return p
	}

	if err := p.openFileForDate(time.Now()); err != nil {
		slog.Warn("training capture disabled: cannot open file", "error", err)
		return p
	}

	p.enabled = true
	slog.Info("training data capture enabled", "caller", caller, "file", p.filePath(p.fileDate))
	return p
}

// filePath returns the JSONL file path for the given date string.
func (p *TrainingCaptureProvider) filePath(date string) string {
	return filepath.Join(p.dir, fmt.Sprintf("capture_%s.jsonl", date))
}

// openFileForDate opens (or creates) the capture file for the given date.
// Caller must hold p.mu or be called during initialization.
func (p *TrainingCaptureProvider) openFileForDate(t time.Time) error {
	date := t.Format("2006-01-02")

	if p.file != nil {
		_ = p.file.Close()
	}

	fpath := p.filePath(date)
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("opening %s: %w", fpath, err)
	}

	p.file = f
	p.fileDate = date
	return nil
}

// ensureCurrentFile checks if the date has rolled over and opens a new file if needed.
// Must be called with p.mu held.
func (p *TrainingCaptureProvider) ensureCurrentFile() {
	today := time.Now().Format("2006-01-02")
	if today == p.fileDate {
		return
	}

	slog.Info("training capture: rolling to new daily file", "caller", p.caller, "date", today)
	if err := p.openFileForDate(time.Now()); err != nil {
		slog.Warn("training capture: failed to roll file, continuing with old file", "error", err)
	}
}

// classifyTask infers the task type from the request content.
// Examines both system and user messages to handle agents that don't use system prompts
// (e.g., retrieval synthesis uses user-role messages only).
func classifyTask(req CompletionRequest) string {
	// First pass: check system messages (most agents use these)
	for _, msg := range req.Messages {
		if msg.Role != "system" {
			continue
		}
		content := msg.Content

		if containsAny(content, "memory encoder", "encode this observation", "gist", "structured_concepts") {
			return "encoding"
		}
		// Episoding check before synthesis (episoding prompts contain "synthesize")
		if containsAny(content, "episode synthesizer", "episode summary") {
			return "episoding"
		}
		// Abstraction check before synthesis (abstraction prompts contain "synthesizer")
		if containsAny(content, "principle synthesizer", "principle", "patterns have emerged", "axiom") {
			return "abstraction"
		}
		// Consolidation
		if containsAny(content, "memory consolidator", "consolidat", "merge related memories") {
			return "consolidation"
		}
		if containsAny(content, "synthesize", "memories tell you", "recall") {
			return "synthesis"
		}
		if containsAny(content, "worth remembering", "perception", "memory perception") {
			return "perception"
		}
		if containsAny(content, "classify the relationship", "relation_type") {
			return "classification"
		}
	}

	// Second pass: check user messages for agents that skip system prompts.
	// The retrieval agent builds synthesis prompts as user messages.
	for _, msg := range req.Messages {
		if msg.Role != "user" {
			continue
		}
		content := msg.Content

		if containsAny(content, "synthesize", "memories tell you", "Summarize what the memories") {
			return "synthesis"
		}
	}

	// Third pass: infer from request structure.
	// If tools are present, it's almost certainly synthesis (only retrieval uses tools).
	if len(req.Tools) > 0 {
		return "synthesis"
	}

	return "unknown"
}

// containsAny returns true if s contains any of the substrings (case-insensitive).
func containsAny(s string, subs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func (p *TrainingCaptureProvider) capture(req CompletionRequest, resp CompletionResponse, latencyMs int64, callErr error) {
	if !p.enabled {
		return
	}

	// Don't capture failed calls — they have no useful response to learn from.
	if callErr != nil {
		return
	}

	example := TrainingExample{
		Timestamp: time.Now().UTC(),
		TaskType:  classifyTask(req),
		Caller:    p.caller,
		Request: TrainingRequest{
			Messages:       req.Messages,
			Model:          req.Model,
			Tools:          req.Tools,
			ResponseFormat: req.ResponseFormat,
			MaxTokens:      req.MaxTokens,
			Temperature:    req.Temperature,
			TopP:           req.TopP,
			Stop:           req.Stop,
		},
		Response: TrainingResponse{
			Content:    resp.Content,
			ToolCalls:  resp.ToolCalls,
			StopReason: resp.StopReason,
		},
		LatencyMs:      latencyMs,
		PromptTokens:   resp.PromptTokens,
		CompletionToks: resp.CompletionTokens,
	}

	// Quick JSON parse check for structured output responses.
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" {
		var js json.RawMessage
		example.ParseSuccess = json.Unmarshal([]byte(resp.Content), &js) == nil
	} else {
		example.ParseSuccess = true // non-JSON responses are always "valid"
	}

	data, err := json.Marshal(example)
	if err != nil {
		slog.Warn("training capture: marshal failed", "error", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureCurrentFile()

	if _, err := p.file.Write(append(data, '\n')); err != nil {
		slog.Warn("training capture: write failed", "error", err)
	}
}

// Complete delegates to the inner provider and captures the exchange.
func (p *TrainingCaptureProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	start := time.Now()
	resp, err := p.inner.Complete(ctx, req)
	latency := time.Since(start).Milliseconds()
	p.capture(req, resp, latency, err)
	return resp, err
}

// Embed delegates to the inner provider (embeddings are not captured for training).
func (p *TrainingCaptureProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return p.inner.Embed(ctx, text)
}

// BatchEmbed delegates to the inner provider (embeddings are not captured for training).
func (p *TrainingCaptureProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.inner.BatchEmbed(ctx, texts)
}

// Health delegates to the inner provider.
func (p *TrainingCaptureProvider) Health(ctx context.Context) error {
	return p.inner.Health(ctx)
}

// ModelInfo delegates to the inner provider.
func (p *TrainingCaptureProvider) ModelInfo(ctx context.Context) (ModelMetadata, error) {
	return p.inner.ModelInfo(ctx)
}

// Close flushes and closes the capture file.
func (p *TrainingCaptureProvider) Close() error {
	if p.file != nil {
		return p.file.Close()
	}
	return nil
}
