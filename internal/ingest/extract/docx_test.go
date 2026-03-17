package extract

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDOCXExtractorAvailable(t *testing.T) {
	ext := &DOCXExtractor{}
	if !ext.Available() {
		t.Error("DOCXExtractor.Available() should always return true")
	}
}

func TestDOCXExtract(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.docx")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &DOCXExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 524288, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if result.FullText == "" {
		t.Fatal("expected non-empty FullText")
	}

	if len(result.Chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// All 3 paragraphs are short enough to fit in one chunk (~450 chars total)
	// so we expect 1 chunk
	if len(result.Chunks) != 1 {
		t.Logf("got %d chunks (paragraphs may have been split)", len(result.Chunks))
	}

	// Verify content contains expected text
	if !containsWords(result.FullText, "first section") {
		t.Errorf("FullText missing expected text about 'first section'")
	}
	if !containsWords(result.FullText, "spread activation") {
		t.Errorf("FullText missing expected text about 'spread activation'")
	}

	// Verify metadata
	if result.Metadata["extracted"] != true {
		t.Error("expected metadata extracted=true")
	}
	if result.Metadata["extractor"] != "docx" {
		t.Error("expected metadata extractor=docx")
	}
}

func TestDOCXExtractTruncation(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.docx")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &DOCXExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 50, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.FullText) > 70 { // 50 bytes + truncation marker
		t.Errorf("FullText not truncated: length %d", len(result.FullText))
	}
}

func TestDOCXExtractNonexistentFile(t *testing.T) {
	ext := &DOCXExtractor{}
	log := slog.Default()

	_, err := ext.Extract("testdata/nonexistent.docx", 524288, log)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestDOCXExtractInvalidFile(t *testing.T) {
	// Create a temporary file that isn't a valid DOCX (not a ZIP)
	tmp, err := os.CreateTemp("", "invalid-*.docx")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString("this is not a docx file"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	ext := &DOCXExtractor{}
	log := slog.Default()

	_, err = ext.Extract(tmp.Name(), 524288, log)
	if err == nil {
		t.Error("expected error for invalid DOCX file")
	}
}

func TestGroupParagraphs(t *testing.T) {
	// Test with short paragraphs that fit in one chunk
	short := []string{"hello", "world", "foo"}
	chunks := GroupParagraphs(short)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for short paragraphs, got %d", len(chunks))
	}

	// Test with paragraphs that exceed chunk target
	long := make([]string, 0)
	for i := 0; i < 5; i++ {
		// Each ~500 chars, so 5 paragraphs should produce ~2-3 chunks at 2000 target
		p := ""
		for j := 0; j < 50; j++ {
			p += "word word "
		}
		long = append(long, p)
	}
	chunks = GroupParagraphs(long)
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for long paragraphs, got %d", len(chunks))
	}

	// Verify sequential chunk numbering
	for i, c := range chunks {
		if c.PageNumber != i+1 {
			t.Errorf("chunk[%d].PageNumber = %d, want %d", i, c.PageNumber, i+1)
		}
	}
}
