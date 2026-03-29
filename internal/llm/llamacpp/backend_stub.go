//go:build !llamacpp

package llamacpp

import "github.com/appsprout-dev/mnemonic/internal/llm"

// NewBackend returns nil when llama.cpp support is not compiled in.
// The EmbeddedProvider handles nil backends gracefully by returning
// ErrProviderUnavailable on all inference calls.
func NewBackend() llm.Backend {
	return nil
}
