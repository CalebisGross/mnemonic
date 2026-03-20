package main

import (
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/testutil/stubllm"
)

// semanticStubProvider wraps the shared stubllm.Provider for local use.
type semanticStubProvider = stubllm.Provider

// vocabulary re-exports the shared vocabulary for len() checks in reports.
var vocabulary = stubllm.Vocabulary

// bowEmbedding re-exports the shared embedding function for scenario use.
func bowEmbedding(text string) []float32 {
	return stubllm.BowEmbedding(text)
}

// Ensure the shared provider satisfies llm.Provider at compile time.
var _ llm.Provider = (*stubllm.Provider)(nil)
