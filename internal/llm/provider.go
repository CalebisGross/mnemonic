package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Message represents a single turn in a conversation.
type Message struct {
	Role       string     `json:"role"` // "system", "user", "assistant", "tool"
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // populated when assistant requests tool use
	ToolCallID string     `json:"tool_call_id,omitempty"` // set when Role="tool" to match the request
}

// ResponseFormat specifies the expected output format for the LLM response.
// Supports "json_object" (any valid JSON) or "json_schema" (schema-constrained).
type ResponseFormat struct {
	Type       string      `json:"type"`                  // "text", "json_object", or "json_schema"
	JSONSchema *JSONSchema `json:"json_schema,omitempty"` // required when Type is "json_schema"
}

// JSONSchema defines a JSON schema constraint for structured LLM output.
type JSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

// CompletionRequest is the input to a completion call.
type CompletionRequest struct {
	Messages       []Message       `json:"messages"`
	Model          string          `json:"model,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float32         `json:"temperature,omitempty"`
	TopP           float32         `json:"top_p,omitempty"`
	Stop           []string        `json:"stop,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// CompletionResponse is the output of a completion call.
type CompletionResponse struct {
	Content          string     `json:"content"`
	StopReason       string     `json:"stop_reason"`
	TokensUsed       int        `json:"tokens_used"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	MeanProb         float32    `json:"mean_prob,omitempty"` // mean token probability (embedded provider only)
	MinProb          float32    `json:"min_prob,omitempty"`  // min token probability (embedded provider only)
}

// Tool defines a function the LLM can call during completion.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ToolCall represents the LLM requesting a tool invocation.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the name and arguments of a requested tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// StructuredOutput wraps a completion with the raw and parsed output.
type StructuredOutput[T any] struct {
	Raw    string
	Parsed T
}

// ModelMetadata describes the capabilities of a model.
type ModelMetadata struct {
	Name              string `json:"name"`
	ContextWindow     int    `json:"context_window"`
	SupportsEmbedding bool   `json:"supports_embedding"`
	MaxTokens         int    `json:"max_tokens"`
}

// Provider is the abstraction for any LLM backend.
type Provider interface {
	// Complete sends a prompt and gets a text response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// Embed generates a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// BatchEmbed generates embeddings for multiple texts efficiently.
	BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)

	// Health checks if the LLM backend is reachable.
	Health(ctx context.Context) error

	// ModelInfo returns metadata about the current model.
	ModelInfo(ctx context.Context) (ModelMetadata, error)
}

// ErrProviderUnavailable is returned when the LLM backend is not reachable.
type ErrProviderUnavailable struct {
	Endpoint string
	Cause    error
}

func (e *ErrProviderUnavailable) Error() string {
	return fmt.Sprintf("llm provider unavailable at %s: %v", e.Endpoint, e.Cause)
}

func (e *ErrProviderUnavailable) Unwrap() error {
	return e.Cause
}
