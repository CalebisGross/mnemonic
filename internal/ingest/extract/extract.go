package extract

import (
	"log/slog"
	"strings"
)

// Result holds extracted text and per-chunk splits from a document.
type Result struct {
	FullText string         // complete extracted text (for summary memory)
	Chunks   []Chunk        // page/section splits
	Metadata map[string]any // document-level: extractor, page_count, etc.
}

// Chunk represents a single page or section of extracted text.
type Chunk struct {
	Text       string
	PageNumber int // 1-based page number; 0 if not applicable (e.g., DOCX)
}

// Extractor extracts text content from a document file.
type Extractor interface {
	// Extract reads the file at path and returns extracted text split into chunks.
	// Returns error only for unexpected failures (not "tool missing").
	Extract(path string, maxBytes int, log *slog.Logger) (Result, error)
}

// Registry maps file extensions to their extractors.
type Registry struct {
	extractors map[string]Extractor
}

// NewRegistry creates an empty extractor registry.
func NewRegistry() *Registry {
	return &Registry{
		extractors: make(map[string]Extractor),
	}
}

// Register adds an extractor for the given extension (e.g., ".pdf").
func (r *Registry) Register(ext string, e Extractor) {
	r.extractors[strings.ToLower(ext)] = e
}

// Get returns the extractor for the given extension, or nil if none is registered.
func (r *Registry) Get(ext string) Extractor {
	return r.extractors[strings.ToLower(ext)]
}

// HasExtractor returns true if an extractor is registered for the given extension.
func (r *Registry) HasExtractor(ext string) bool {
	_, ok := r.extractors[strings.ToLower(ext)]
	return ok
}

// ChunkTarget is the target chunk size in characters for paragraph grouping.
const ChunkTarget = 2000

// GroupParagraphs groups consecutive paragraphs into chunks of approximately
// ChunkTarget characters. Each chunk gets a sequential number (1-based).
func GroupParagraphs(paragraphs []string) []Chunk {
	var chunks []Chunk
	var current strings.Builder
	chunkNum := 1

	for _, p := range paragraphs {
		if current.Len() > 0 && current.Len()+len(p)+1 > ChunkTarget {
			chunks = append(chunks, Chunk{
				Text:       current.String(),
				PageNumber: chunkNum,
			})
			chunkNum++
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(p)
	}

	if current.Len() > 0 {
		chunks = append(chunks, Chunk{
			Text:       current.String(),
			PageNumber: chunkNum,
		})
	}

	return chunks
}

// WordCount returns the number of whitespace-delimited tokens in text.
func WordCount(text string) int {
	return len(strings.Fields(text))
}

// Truncate returns text truncated to maxBytes, appending a marker if truncated.
func Truncate(text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	return text[:maxBytes] + "\n... [truncated]"
}
