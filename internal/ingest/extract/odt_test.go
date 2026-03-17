package extract

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestODTExtractorAvailable(t *testing.T) {
	ext := &ODTExtractor{}
	if !ext.Available() {
		t.Error("ODTExtractor.Available() should always return true")
	}
}

func TestODTExtract(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.odt")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &ODTExtractor{}
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

	// Verify content
	if !containsWords(result.FullText, "first paragraph") {
		t.Errorf("FullText missing 'first paragraph', got: %q", result.FullText)
	}
	if !containsWords(result.FullText, "spread activation") {
		t.Errorf("FullText missing 'spread activation', got: %q", result.FullText)
	}

	// Verify metadata
	if result.Metadata["extracted"] != true {
		t.Error("expected metadata extracted=true")
	}
	if result.Metadata["extractor"] != "odt" {
		t.Error("expected metadata extractor=odt")
	}
}

func TestODTExtractTruncation(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.odt")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &ODTExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 50, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.FullText) > 70 {
		t.Errorf("FullText not truncated: length %d", len(result.FullText))
	}
}

func TestODTExtractNonexistentFile(t *testing.T) {
	ext := &ODTExtractor{}
	log := slog.Default()

	_, err := ext.Extract("testdata/nonexistent.odt", 524288, log)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestODTExtractInvalidFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "invalid-*.odt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString("this is not an odt file"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	ext := &ODTExtractor{}
	log := slog.Default()

	_, err = ext.Extract(tmp.Name(), 524288, log)
	if err == nil {
		t.Error("expected error for invalid ODT file")
	}
}
