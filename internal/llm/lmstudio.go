package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// LMStudioProvider implements the Provider interface for LM Studio's OpenAI-compatible API.
type LMStudioProvider struct {
	endpoint       string
	chatModel      string
	embeddingModel string
	apiKey         string // optional API key for authenticated providers (e.g., Gemini)
	httpClient     *http.Client
	timeout        time.Duration
	sem            chan struct{} // concurrency limiter for LLM requests
}

// NewLMStudioProvider creates a new provider for OpenAI-compatible APIs (LM Studio, Gemini, etc.).
// endpoint should be the base URL (e.g., "http://localhost:1234/v1"), without a trailing slash.
// chatModel is the model name for text completion (e.g., "gpt-3.5-turbo").
// embeddingModel is the model name for embeddings (e.g., "nomic-embed-text").
// apiKey is an optional API key; when non-empty, requests include an Authorization: Bearer header.
// timeout is the request timeout duration.
// maxConcurrent limits simultaneous LLM requests (0 defaults to 2).
func NewLMStudioProvider(endpoint, chatModel, embeddingModel, apiKey string, timeout time.Duration, maxConcurrent int) *LMStudioProvider {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	return &LMStudioProvider{
		endpoint:       endpoint,
		chatModel:      chatModel,
		embeddingModel: embeddingModel,
		apiKey:         apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
		sem:     make(chan struct{}, maxConcurrent),
	}
}

// acquire blocks until a concurrency slot is available or ctx is cancelled.
func (p *LMStudioProvider) acquire(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release frees a concurrency slot.
func (p *LMStudioProvider) release() {
	<-p.sem
}

// setAuthHeader sets the Authorization header if an API key is configured.
func (p *LMStudioProvider) setAuthHeader(req *http.Request) {
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

// doWithRetry executes an HTTP request with exponential backoff on transient errors.
// Retries up to 3 times on connection errors and 5xx responses.
func (p *LMStudioProvider) doWithRetry(req *http.Request) (*http.Response, error) {
	const maxRetries = 3
	delays := [3]time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Debug("retrying LLM request", "attempt", attempt, "url", req.URL.String(), "delay", delays[attempt-1])
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delays[attempt-1]):
			}
			// Reset body for retry
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("failed to reset request body for retry: %w", err)
				}
				req.Body = body
			}
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = err
			// Connection errors are retryable
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			continue
		}

		// 5xx errors are retryable (server-side transient failures)
		if resp.StatusCode >= 500 && attempt < maxRetries {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}

// openAIMessage wraps a Message for OpenAI API serialization.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"` // pointer: null for tool-call assistant messages
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openAIToolCall represents a tool invocation in the OpenAI format.
type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAITool defines a tool in the OpenAI format.
type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

// openAIToolFunction describes the function within a tool definition.
type openAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// openAIResponseFormat mirrors the OpenAI response_format parameter.
type openAIResponseFormat struct {
	Type       string            `json:"type"`                  // "text", "json_object", or "json_schema"
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"` // for "json_schema" type
}

// openAIJSONSchema wraps a JSON schema for structured output.
type openAIJSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

// openAICompletionRequest is the request format for the OpenAI-compatible chat completion API.
type openAICompletionRequest struct {
	Model           string                `json:"model"`
	Messages        []openAIMessage       `json:"messages"`
	MaxTokens       int                   `json:"max_tokens,omitempty"`
	Temperature     float32               `json:"temperature,omitempty"`
	TopP            float32               `json:"top_p,omitempty"`
	Stop            []string              `json:"stop,omitempty"`
	Tools           []openAITool          `json:"tools,omitempty"`
	ResponseFormat  *openAIResponseFormat `json:"response_format,omitempty"`
	ReasoningEffort string                `json:"reasoning_effort,omitempty"`
}

// openAIChoice represents a single choice in a completion response.
type openAIChoice struct {
	Message struct {
		Role      string           `json:"role"`
		Content   *string          `json:"content"`
		ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

// openAIUsage contains token usage information.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAICompletionResponse is the response format from the OpenAI-compatible chat completion API.
type openAICompletionResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

// openAIEmbeddingRequest is the request format for the OpenAI-compatible embeddings API.
type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// openAIEmbeddingData represents a single embedding in the response.
type openAIEmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// openAIEmbeddingResponse is the response format from the OpenAI-compatible embeddings API.
type openAIEmbeddingResponse struct {
	Data  []openAIEmbeddingData `json:"data"`
	Usage openAIUsage           `json:"usage"`
}

// stringPtr returns a pointer to a string value.
func stringPtr(s string) *string { return &s }

// derefString safely dereferences a string pointer, returning empty string for nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Complete sends a completion request to LM Studio and returns the response.
func (p *LMStudioProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if err := p.acquire(ctx); err != nil {
		return CompletionResponse{}, &ErrProviderUnavailable{
			Endpoint: p.endpoint + "/chat/completions",
			Cause:    fmt.Errorf("concurrency limit reached: %w", err),
		}
	}
	defer p.release()

	// Convert input messages to OpenAI format
	messages := make([]openAIMessage, len(req.Messages))
	for i, msg := range req.Messages {
		oaiMsg := openAIMessage{
			Role:       msg.Role,
			ToolCallID: msg.ToolCallID,
		}
		// Content is a pointer — set to nil for assistant messages that only have tool calls
		if msg.Content != "" {
			oaiMsg.Content = stringPtr(msg.Content)
		}
		// Convert tool calls if present (for replaying assistant tool-call messages)
		if len(msg.ToolCalls) > 0 {
			oaiMsg.ToolCalls = make([]openAIToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				oaiMsg.ToolCalls[j] = openAIToolCall{
					ID:   tc.ID,
					Type: tc.Type,
				}
				oaiMsg.ToolCalls[j].Function.Name = tc.Function.Name
				oaiMsg.ToolCalls[j].Function.Arguments = tc.Function.Arguments
			}
		}
		messages[i] = oaiMsg
	}

	// Determine which model to use
	model := req.Model
	if model == "" {
		model = p.chatModel
	}

	// Qwen3 thinking mode: append /no_think to the last user message
	// to disable chain-of-thought reasoning and get direct JSON output.
	isThinkingModel := false
	if strings.Contains(strings.ToLower(model), "qwen3") {
		isThinkingModel = true
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" && messages[i].Content != nil {
				noThink := *messages[i].Content + " /no_think"
				messages[i].Content = &noThink
				break
			}
		}
	}
	// Gemini thinking models (gemini-2.5+, gemini-3+): detect by model name.
	if strings.Contains(strings.ToLower(model), "gemini-2.5") || strings.Contains(strings.ToLower(model), "gemini-3") {
		isThinkingModel = true
	}

	// Build the request
	apiReq := openAICompletionRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}

	// Thinking models: disable reasoning for structured output requests.
	// Thinking tokens consume the max_tokens budget and can starve the actual
	// JSON output, causing parse failures.
	if isThinkingModel && req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" {
		apiReq.ReasoningEffort = "none"
	}

	// Convert tools if present
	if len(req.Tools) > 0 {
		apiReq.Tools = make([]openAITool, len(req.Tools))
		for i, tool := range req.Tools {
			apiReq.Tools[i] = openAITool{
				Type: tool.Type,
				Function: openAIToolFunction{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			}
		}
	}

	// Set response format if specified (structured output / JSON mode)
	if req.ResponseFormat != nil {
		rf := &openAIResponseFormat{
			Type: req.ResponseFormat.Type,
		}
		if req.ResponseFormat.JSONSchema != nil {
			rf.JSONSchema = &openAIJSONSchema{
				Name:   req.ResponseFormat.JSONSchema.Name,
				Strict: req.ResponseFormat.JSONSchema.Strict,
				Schema: req.ResponseFormat.JSONSchema.Schema,
			}
		}
		apiReq.ResponseFormat = rf
	}

	// Marshal the request body
	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	url := fmt.Sprintf("%s/chat/completions", p.endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(reqBody)), nil
	}

	httpReq.Header.Set("Content-Type", "application/json")
	p.setAuthHeader(httpReq)

	// Execute the request with retry
	httpResp, err := p.doWithRetry(httpReq)
	if err != nil {
		return CompletionResponse{}, &ErrProviderUnavailable{
			Endpoint: url,
			Cause:    err,
		}
	}
	defer func() { _ = httpResp.Body.Close() }()

	// Check HTTP status
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return CompletionResponse{}, &ErrProviderUnavailable{
			Endpoint: url,
			Cause:    fmt.Errorf("http %d: %s", httpResp.StatusCode, string(body)),
		}
	}

	// Decode response
	var apiResp openAICompletionResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&apiResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract completion from response
	if len(apiResp.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("no choices in response")
	}

	choice := apiResp.Choices[0]
	resp := CompletionResponse{
		Content:          derefString(choice.Message.Content),
		StopReason:       choice.FinishReason,
		TokensUsed:       apiResp.Usage.TotalTokens,
		PromptTokens:     apiResp.Usage.PromptTokens,
		CompletionTokens: apiResp.Usage.CompletionTokens,
	}

	// If the model made tool calls, convert them
	if len(choice.Message.ToolCalls) > 0 {
		resp.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			resp.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return resp, nil
}

// Embed generates a single embedding for the given text.
func (p *LMStudioProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.BatchEmbed(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings[0], nil
}

// BatchEmbed generates embeddings for multiple texts in a single request.
func (p *LMStudioProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	if err := p.acquire(ctx); err != nil {
		return nil, &ErrProviderUnavailable{
			Endpoint: p.endpoint + "/embeddings",
			Cause:    fmt.Errorf("concurrency limit reached: %w", err),
		}
	}
	defer p.release()

	// Build the request
	apiReq := openAIEmbeddingRequest{
		Model: p.embeddingModel,
		Input: texts,
	}

	// Marshal the request body
	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	url := fmt.Sprintf("%s/embeddings", p.endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(reqBody)), nil
	}

	httpReq.Header.Set("Content-Type", "application/json")
	p.setAuthHeader(httpReq)

	// Execute the request with retry
	httpResp, err := p.doWithRetry(httpReq)
	if err != nil {
		return nil, &ErrProviderUnavailable{
			Endpoint: url,
			Cause:    err,
		}
	}
	defer func() { _ = httpResp.Body.Close() }()

	// Check HTTP status
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, &ErrProviderUnavailable{
			Endpoint: url,
			Cause:    fmt.Errorf("http %d: %s", httpResp.StatusCode, string(body)),
		}
	}

	// Decode response
	var apiResp openAIEmbeddingResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Sort embeddings by index to ensure correct ordering
	embeddings := make([][]float32, len(texts))
	for _, embData := range apiResp.Data {
		if embData.Index < 0 || embData.Index >= len(embeddings) {
			return nil, fmt.Errorf("embedding index %d out of bounds", embData.Index)
		}
		embeddings[embData.Index] = embData.Embedding
	}

	return embeddings, nil
}

// Health checks if LM Studio is reachable by calling the models endpoint.
func (p *LMStudioProvider) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/models", p.endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	p.setAuthHeader(httpReq)

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return &ErrProviderUnavailable{
			Endpoint: url,
			Cause:    err,
		}
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return &ErrProviderUnavailable{
			Endpoint: url,
			Cause:    fmt.Errorf("http %d: %s", httpResp.StatusCode, string(body)),
		}
	}

	return nil
}

// ModelInfo returns metadata about the currently configured models.
// Since LM Studio doesn't expose detailed model info via the API,
// we return basic metadata based on what we know.
func (p *LMStudioProvider) ModelInfo(ctx context.Context) (ModelMetadata, error) {
	// Verify the provider is healthy first
	if err := p.Health(ctx); err != nil {
		return ModelMetadata{}, err
	}

	// Return metadata based on configured models
	// Note: These are estimates; actual values depend on the specific model loaded
	return ModelMetadata{
		Name:              p.chatModel,
		ContextWindow:     2048, // Conservative default; actual value depends on the loaded model
		SupportsEmbedding: p.embeddingModel != "",
		MaxTokens:         1024, // Conservative default for response tokens
	}, nil
}

// EmbeddingModelName returns the configured embedding model name.
func (p *LMStudioProvider) EmbeddingModelName() string {
	return p.embeddingModel
}
