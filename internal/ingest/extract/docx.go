package extract

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fumiama/go-docx"
)

// DOCXExtractor extracts text from .docx files using a pure-Go library.
type DOCXExtractor struct{}

// Available always returns true (pure Go, no external dependency).
func (d *DOCXExtractor) Available() bool { return true }

// Extract opens the DOCX file, iterates paragraphs and tables, and groups
// them into chunks of approximately docxChunkTarget characters.
func (d *DOCXExtractor) Extract(path string, maxBytes int, log *slog.Logger) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("opening docx %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("stat docx %s: %w", path, err)
	}

	doc, err := docx.Parse(f, info.Size())
	if err != nil {
		return Result{}, fmt.Errorf("parsing docx %s: %w", path, err)
	}

	// Extract text from paragraphs and tables
	var paragraphs []string
	for _, item := range doc.Document.Body.Items {
		switch item.(type) {
		case *docx.Paragraph, *docx.Table:
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				paragraphs = append(paragraphs, text)
			}
		}
	}

	if len(paragraphs) == 0 {
		return Result{}, nil
	}

	fullText := strings.Join(paragraphs, "\n")

	// Group paragraphs into chunks of ~docxChunkTarget characters
	chunks := GroupParagraphs(paragraphs)

	metadata := map[string]any{
		"extracted": true,
		"extractor": "docx",
	}

	return Result{
		FullText: Truncate(fullText, maxBytes),
		Chunks:   chunks,
		Metadata: metadata,
	}, nil
}
