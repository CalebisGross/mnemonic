//go:build llamacpp

package llamacpp

/*
#cgo CFLAGS: -I${SRCDIR}/csrc
#cgo LDFLAGS: ${SRCDIR}/csrc/bridge.o -L${SRCDIR}/../../../third_party/llama.cpp/build/src -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src -lllama -lggml -lggml-base -lggml-cpu -lm -lstdc++ -lpthread -fopenmp
#include "csrc/bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	"github.com/appsprout-dev/mnemonic/internal/llm"
)

// Backend implements llm.Backend using llama.cpp via CGo.
// All inference calls are serialized via a mutex because llama.cpp
// contexts are not thread-safe.
type Backend struct {
	mu    sync.Mutex
	model *C.mnm_model
}

// NewBackend creates a new llama.cpp backend instance.
func NewBackend() llm.Backend {
	return &Backend{}
}

func (b *Backend) LoadModel(path string, opts llm.BackendOptions) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	params := C.mnm_model_params{
		context_size: C.int(opts.ContextSize),
		gpu_layers:   C.int(opts.GPULayers),
		threads:      C.int(opts.Threads),
		batch_size:   C.int(opts.BatchSize),
	}

	b.model = C.mnm_load_model(cpath, params)
	if b.model == nil {
		return fmt.Errorf("failed to load model: %s", path)
	}
	return nil
}

func (b *Backend) Complete(_ context.Context, req llm.BackendCompletionRequest) (llm.BackendCompletionResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.model == nil {
		return llm.BackendCompletionResponse{}, fmt.Errorf("model not loaded")
	}

	cprompt := C.CString(req.Prompt)
	defer C.free(unsafe.Pointer(cprompt))

	var cgrammar *C.char
	if req.Grammar != "" {
		cgrammar = C.CString(req.Grammar)
		defer C.free(unsafe.Pointer(cgrammar))
	}

	// Build stop sequences
	var cstop **C.char
	nStop := len(req.Stop)
	if nStop > 0 {
		cstopSlice := make([]*C.char, nStop)
		for i, s := range req.Stop {
			cstopSlice[i] = C.CString(s)
			defer C.free(unsafe.Pointer(cstopSlice[i]))
		}
		cstop = (**C.char)(unsafe.Pointer(&cstopSlice[0]))
	}

	result := C.mnm_complete(
		b.model,
		cprompt,
		C.int(req.MaxTokens),
		C.float(req.Temperature),
		C.float(req.TopP),
		cgrammar,
		cstop,
		C.int(nStop),
	)

	var text string
	if result.text != nil {
		text = C.GoString(result.text)
		C.mnm_free_string(result.text)
	}

	return llm.BackendCompletionResponse{
		Text:             text,
		PromptTokens:     int(result.prompt_tokens),
		CompletionTokens: int(result.completion_tokens),
	}, nil
}

func (b *Backend) Embed(_ context.Context, text string) ([]float32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.model == nil {
		return nil, fmt.Errorf("model not loaded")
	}

	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))

	result := C.mnm_embed(b.model, ctext)
	if result.data == nil {
		return nil, fmt.Errorf("embedding extraction failed (model may not support embeddings)")
	}
	defer C.mnm_free_floats(result.data)

	dims := int(result.dims)
	embedding := make([]float32, dims)
	cSlice := unsafe.Slice((*float32)(unsafe.Pointer(result.data)), dims)
	copy(embedding, cSlice)

	return embedding, nil
}

func (b *Backend) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := b.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embedding text %d: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}

func (b *Backend) Close() error {
	if b.model != nil {
		C.mnm_free_model(b.model)
		b.model = nil
	}
	return nil
}
