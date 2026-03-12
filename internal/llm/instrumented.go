package llm

import (
	"context"
	"log/slog"
	"time"
)

// LLMUsageRecord captures metrics from a single LLM API call.
type LLMUsageRecord struct {
	Timestamp        time.Time `json:"timestamp"`
	Operation        string    `json:"operation"` // "complete", "embed", "batch_embed"
	Caller           string    `json:"caller"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	LatencyMs        int64     `json:"latency_ms"`
	Success          bool      `json:"success"`
	ErrorMessage     string    `json:"error_message,omitempty"`
}

// UsageRecorder persists LLM usage records.
type UsageRecorder interface {
	RecordLLMUsage(ctx context.Context, record LLMUsageRecord) error
}

// InstrumentedProvider wraps a Provider to capture usage metrics on every call.
type InstrumentedProvider struct {
	inner    Provider
	recorder UsageRecorder
	caller   string
	model    string
}

// NewInstrumentedProvider wraps inner with usage tracking.
// caller identifies the agent (e.g., "encoding", "retrieval").
// model is the default model name for logging.
func NewInstrumentedProvider(inner Provider, recorder UsageRecorder, caller, model string) *InstrumentedProvider {
	return &InstrumentedProvider{
		inner:    inner,
		recorder: recorder,
		caller:   caller,
		model:    model,
	}
}

func (p *InstrumentedProvider) record(ctx context.Context, rec LLMUsageRecord) {
	if err := p.recorder.RecordLLMUsage(ctx, rec); err != nil {
		slog.Warn("failed to record LLM usage", "error", err, "caller", rec.Caller)
	}
}

// Complete delegates to the inner provider and records usage.
func (p *InstrumentedProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	start := time.Now()
	resp, err := p.inner.Complete(ctx, req)
	latency := time.Since(start).Milliseconds()

	model := req.Model
	if model == "" {
		model = p.model
	}

	rec := LLMUsageRecord{
		Timestamp:        start,
		Operation:        "complete",
		Caller:           p.caller,
		Model:            model,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
		TotalTokens:      resp.TokensUsed,
		LatencyMs:        latency,
		Success:          err == nil,
	}
	if err != nil {
		rec.ErrorMessage = err.Error()
	}
	p.record(ctx, rec)

	return resp, err
}

// Embed delegates to the inner provider and records usage.
func (p *InstrumentedProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	start := time.Now()
	result, err := p.inner.Embed(ctx, text)
	latency := time.Since(start).Milliseconds()

	// Estimate tokens for embeddings (~4 chars per token)
	estTokens := len(text) / 4
	if estTokens < 1 {
		estTokens = 1
	}

	rec := LLMUsageRecord{
		Timestamp:    start,
		Operation:    "embed",
		Caller:       p.caller,
		Model:        p.model,
		PromptTokens: estTokens,
		TotalTokens:  estTokens,
		LatencyMs:    latency,
		Success:      err == nil,
	}
	if err != nil {
		rec.ErrorMessage = err.Error()
	}
	p.record(ctx, rec)

	return result, err
}

// BatchEmbed delegates to the inner provider and records usage.
func (p *InstrumentedProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return p.inner.BatchEmbed(ctx, texts)
	}

	start := time.Now()
	result, err := p.inner.BatchEmbed(ctx, texts)
	latency := time.Since(start).Milliseconds()

	// Estimate tokens for all texts
	estTokens := 0
	for _, t := range texts {
		estTokens += len(t) / 4
	}
	if estTokens < 1 {
		estTokens = 1
	}

	rec := LLMUsageRecord{
		Timestamp:    start,
		Operation:    "batch_embed",
		Caller:       p.caller,
		Model:        p.model,
		PromptTokens: estTokens,
		TotalTokens:  estTokens,
		LatencyMs:    latency,
		Success:      err == nil,
	}
	if err != nil {
		rec.ErrorMessage = err.Error()
	}
	p.record(ctx, rec)

	return result, err
}

// Health delegates to the inner provider without recording.
func (p *InstrumentedProvider) Health(ctx context.Context) error {
	return p.inner.Health(ctx)
}

// ModelInfo delegates to the inner provider without recording.
func (p *InstrumentedProvider) ModelInfo(ctx context.Context) (ModelMetadata, error) {
	return p.inner.ModelInfo(ctx)
}
