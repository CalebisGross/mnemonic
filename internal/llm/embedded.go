package llm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Backend is the interface for in-process LLM inference engines.
// The llama.cpp CGo implementation will satisfy this interface.
type Backend interface {
	// LoadModel loads a GGUF model file into memory.
	LoadModel(path string, opts BackendOptions) error

	// Complete runs text generation on the loaded model.
	Complete(ctx context.Context, req BackendCompletionRequest) (BackendCompletionResponse, error)

	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// BatchEmbed generates embedding vectors for multiple texts.
	BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)

	// Close releases all resources held by the backend.
	Close() error
}

// BackendOptions configures model loading for a Backend.
type BackendOptions struct {
	ContextSize int // context window size in tokens
	GPULayers   int // layers to offload to GPU (-1 = all, 0 = CPU only)
	Threads     int // CPU threads for inference (0 = auto-detect)
	BatchSize   int // prompt processing batch size
}

// BackendCompletionRequest is the input for a backend completion call.
type BackendCompletionRequest struct {
	Prompt      string   // formatted prompt text
	MaxTokens   int      // maximum tokens to generate
	Temperature float32  // sampling temperature
	TopP        float32  // nucleus sampling threshold
	Stop        []string // stop sequences
	Grammar     string   // GBNF grammar string for constrained decoding (empty = unconstrained)
}

// BackendCompletionResponse is the output of a backend completion call.
type BackendCompletionResponse struct {
	Text             string // generated text
	PromptTokens     int    // tokens in the prompt
	CompletionTokens int    // tokens generated
}

// EmbeddedProvider implements the Provider interface using in-process inference
// via a Backend (llama.cpp CGo bindings). This allows mnemonic to run its own
// GGUF models without an external API server.
type EmbeddedProvider struct {
	modelsDir      string
	chatModelFile  string
	embedModelFile string
	opts           BackendOptions
	maxTokens      int
	temperature    float32

	mu           sync.RWMutex
	chatBackend  Backend
	embedBackend Backend
	sem          chan struct{}
}

// EmbeddedProviderConfig holds the configuration for creating an EmbeddedProvider.
type EmbeddedProviderConfig struct {
	ModelsDir      string
	ChatModelFile  string
	EmbedModelFile string
	ContextSize    int
	GPULayers      int
	Threads        int
	BatchSize      int
	MaxTokens      int
	Temperature    float32
	MaxConcurrent  int
}

// NewEmbeddedProvider creates a new in-process inference provider.
// The provider is created in an unloaded state — call LoadModels to load GGUF files.
func NewEmbeddedProvider(cfg EmbeddedProviderConfig) *EmbeddedProvider {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 1
	}
	if cfg.ContextSize <= 0 {
		cfg.ContextSize = 2048
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 512
	}
	return &EmbeddedProvider{
		modelsDir:      cfg.ModelsDir,
		chatModelFile:  cfg.ChatModelFile,
		embedModelFile: cfg.EmbedModelFile,
		opts: BackendOptions{
			ContextSize: cfg.ContextSize,
			GPULayers:   cfg.GPULayers,
			Threads:     cfg.Threads,
			BatchSize:   cfg.BatchSize,
		},
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		sem:         make(chan struct{}, cfg.MaxConcurrent),
	}
}

// LoadModels loads the configured GGUF model files using the given backend factory.
// backendFactory creates a new Backend instance for each model.
func (p *EmbeddedProvider) LoadModels(backendFactory func() Backend) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Load chat model
	chatPath := filepath.Join(p.modelsDir, p.chatModelFile)
	if _, err := os.Stat(chatPath); err != nil {
		return fmt.Errorf("chat model not found at %s: %w", chatPath, err)
	}

	chatBackend := backendFactory()
	if err := chatBackend.LoadModel(chatPath, p.opts); err != nil {
		return fmt.Errorf("loading chat model %s: %w", chatPath, err)
	}
	p.chatBackend = chatBackend
	slog.Info("loaded embedded chat model", "path", chatPath)

	// Load embedding model if configured
	if p.embedModelFile != "" {
		embedPath := filepath.Join(p.modelsDir, p.embedModelFile)
		if _, err := os.Stat(embedPath); err != nil {
			return fmt.Errorf("embedding model not found at %s: %w", embedPath, err)
		}

		embedBackend := backendFactory()
		if err := embedBackend.LoadModel(embedPath, p.opts); err != nil {
			return fmt.Errorf("loading embedding model %s: %w", embedPath, err)
		}
		p.embedBackend = embedBackend
		slog.Info("loaded embedded embedding model", "path", embedPath)
	}

	return nil
}

// acquire blocks until a concurrency slot is available or ctx is cancelled.
func (p *EmbeddedProvider) acquire(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release frees a concurrency slot.
func (p *EmbeddedProvider) release() {
	<-p.sem
}

// formatPrompt converts a slice of Messages into a single prompt string.
// Uses the Felix-LM fine-tuning format: <|system|>\n...\n<|user|>\n...\n<|assistant|>\n
func formatPrompt(messages []Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString("<|")
		b.WriteString(msg.Role)
		b.WriteString("|>\n")
		b.WriteString(msg.Content)
		b.WriteByte('\n')
	}
	b.WriteString("<|assistant|>\n")
	return b.String()
}

// Complete sends a completion request to the in-process backend.
func (p *EmbeddedProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if err := p.acquire(ctx); err != nil {
		return CompletionResponse{}, fmt.Errorf("embedded provider busy: %w", err)
	}
	defer p.release()

	p.mu.RLock()
	backend := p.chatBackend
	p.mu.RUnlock()

	if backend == nil {
		return CompletionResponse{}, &ErrProviderUnavailable{
			Endpoint: "embedded",
			Cause:    fmt.Errorf("chat model not loaded — call LoadModels first"),
		}
	}

	// Determine generation parameters
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = p.temperature
	}

	// Convert response format to GBNF grammar if applicable
	grammar := ""
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		grammar = GBNFJSONObject
	}
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" && req.ResponseFormat.JSONSchema != nil {
		// For json_schema, we'd generate a GBNF grammar from the JSON schema.
		// For now, fall back to generic JSON constraint.
		grammar = GBNFJSONObject
	}

	backendReq := BackendCompletionRequest{
		Prompt:      formatPrompt(req.Messages),
		MaxTokens:   maxTokens,
		Temperature: temp,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Grammar:     grammar,
	}

	backendResp, err := backend.Complete(ctx, backendReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("embedded completion: %w", err)
	}

	return CompletionResponse{
		Content:          backendResp.Text,
		StopReason:       "stop",
		TokensUsed:       backendResp.PromptTokens + backendResp.CompletionTokens,
		PromptTokens:     backendResp.PromptTokens,
		CompletionTokens: backendResp.CompletionTokens,
	}, nil
}

// Embed generates a single embedding for the given text.
func (p *EmbeddedProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.BatchEmbed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

// BatchEmbed generates embeddings for multiple texts.
func (p *EmbeddedProvider) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	if err := p.acquire(ctx); err != nil {
		return nil, fmt.Errorf("embedded provider busy: %w", err)
	}
	defer p.release()

	p.mu.RLock()
	backend := p.embedBackend
	// Fall back to chat backend for embeddings if no dedicated embedding model.
	// The llama.cpp bridge creates a separate embedding context with mean pooling.
	if backend == nil {
		backend = p.chatBackend
	}
	p.mu.RUnlock()

	if backend == nil {
		return nil, &ErrProviderUnavailable{
			Endpoint: "embedded",
			Cause:    fmt.Errorf("no model loaded for embeddings — call LoadModels first"),
		}
	}

	return backend.BatchEmbed(ctx, texts)
}

// Health checks if the embedded models are loaded and ready.
func (p *EmbeddedProvider) Health(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.chatBackend == nil {
		return &ErrProviderUnavailable{
			Endpoint: "embedded",
			Cause:    fmt.Errorf("chat model not loaded"),
		}
	}
	return nil
}

// ModelInfo returns metadata about the loaded embedded model.
func (p *EmbeddedProvider) ModelInfo(ctx context.Context) (ModelMetadata, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.chatBackend == nil {
		return ModelMetadata{}, &ErrProviderUnavailable{
			Endpoint: "embedded",
			Cause:    fmt.Errorf("chat model not loaded"),
		}
	}

	return ModelMetadata{
		Name:              p.chatModelFile,
		ContextWindow:     p.opts.ContextSize,
		SupportsEmbedding: p.embedBackend != nil || p.embedModelFile == "",
		MaxTokens:         p.maxTokens,
	}, nil
}

// Close releases all backend resources.
func (p *EmbeddedProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	if p.chatBackend != nil {
		if err := p.chatBackend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing chat backend: %w", err))
		}
		p.chatBackend = nil
	}
	if p.embedBackend != nil {
		if err := p.embedBackend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing embed backend: %w", err))
		}
		p.embedBackend = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("closing embedded provider: %v", errs)
	}
	return nil
}
