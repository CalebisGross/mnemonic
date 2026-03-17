package extract

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPPTXExtractorAvailable(t *testing.T) {
	ext := &PPTXExtractor{}
	if !ext.Available() {
		t.Error("PPTXExtractor.Available() should always return true")
	}
}

func TestPPTXExtract(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.pptx")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &PPTXExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 524288, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if result.FullText == "" {
		t.Fatal("expected non-empty FullText")
	}

	if len(result.Chunks) != 2 {
		t.Fatalf("expected 2 chunks (slides), got %d", len(result.Chunks))
	}

	// Verify slide 1
	if result.Chunks[0].PageNumber != 1 {
		t.Errorf("chunk[0].PageNumber = %d, want 1", result.Chunks[0].PageNumber)
	}
	if !containsWords(result.Chunks[0].Text, "Architecture Overview") {
		t.Errorf("chunk[0] missing expected text, got: %q", result.Chunks[0].Text)
	}

	// Verify slide 2
	if result.Chunks[1].PageNumber != 2 {
		t.Errorf("chunk[1].PageNumber = %d, want 2", result.Chunks[1].PageNumber)
	}
	if !containsWords(result.Chunks[1].Text, "Spread Activation") {
		t.Errorf("chunk[1] missing expected text, got: %q", result.Chunks[1].Text)
	}

	// Verify metadata
	if result.Metadata["extracted"] != true {
		t.Error("expected metadata extracted=true")
	}
	if result.Metadata["extractor"] != "pptx" {
		t.Error("expected metadata extractor=pptx")
	}
	if result.Metadata["slide_count"] != 2 {
		t.Errorf("expected metadata slide_count=2, got %v", result.Metadata["slide_count"])
	}
}

func TestPPTXExtractTruncation(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.pptx")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &PPTXExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 50, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.FullText) > 70 {
		t.Errorf("FullText not truncated: length %d", len(result.FullText))
	}
}

func TestPPTXExtractNonexistentFile(t *testing.T) {
	ext := &PPTXExtractor{}
	log := slog.Default()

	_, err := ext.Extract("testdata/nonexistent.pptx", 524288, log)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestPPTXExtractInvalidFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "invalid-*.pptx")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString("this is not a pptx file"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	ext := &PPTXExtractor{}
	log := slog.Default()

	_, err = ext.Extract(tmp.Name(), 524288, log)
	if err == nil {
		t.Error("expected error for invalid PPTX file")
	}
}
