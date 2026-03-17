package extract

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestRTFExtractorAvailable(t *testing.T) {
	ext := &RTFExtractor{}
	if !ext.Available() {
		t.Error("RTFExtractor.Available() should always return true")
	}
}

func TestRTFExtract(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.rtf")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &RTFExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 524288, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if result.FullText == "" {
		t.Fatal("expected non-empty FullText")
	}

	// Should have at least one chunk
	if len(result.Chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Verify content contains expected text
	if !containsWords(result.FullText, "first paragraph") {
		t.Errorf("FullText missing 'first paragraph', got: %q", result.FullText)
	}
	if !containsWords(result.FullText, "encoding agent") {
		t.Errorf("FullText missing 'encoding agent', got: %q", result.FullText)
	}
	if !containsWords(result.FullText, "spread activation") {
		t.Errorf("FullText missing 'spread activation', got: %q", result.FullText)
	}

	// Should NOT contain RTF control words
	if containsWords(result.FullText, "\\rtf") {
		t.Error("FullText should not contain RTF control words")
	}
	if containsWords(result.FullText, "\\fonttbl") {
		t.Error("FullText should not contain font table")
	}

	// Verify metadata
	if result.Metadata["extracted"] != true {
		t.Error("expected metadata extracted=true")
	}
	if result.Metadata["extractor"] != "rtf" {
		t.Error("expected metadata extractor=rtf")
	}
}

func TestRTFExtractTruncation(t *testing.T) {
	fixture := filepath.Join("testdata", "sample.rtf")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("test fixture missing: %v", err)
	}

	ext := &RTFExtractor{}
	log := slog.Default()

	result, err := ext.Extract(fixture, 50, log)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.FullText) > 70 {
		t.Errorf("FullText not truncated: length %d", len(result.FullText))
	}
}

func TestRTFExtractNonexistentFile(t *testing.T) {
	ext := &RTFExtractor{}
	log := slog.Default()

	_, err := ext.Extract("testdata/nonexistent.rtf", 524288, log)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestStripRTF(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "simple text",
			input:    `{\rtf1 Hello World}`,
			contains: "Hello World",
		},
		{
			name:     "escaped braces",
			input:    `{\rtf1 test \{ and \}}`,
			contains: "test { and }",
		},
		{
			name:     "paragraph breaks",
			input:    `{\rtf1 First\par Second}`,
			contains: "First\nSecond",
		},
		{
			name:     "hex escape",
			input:    `{\rtf1 caf\'e9}`,
			contains: "caf\xe9",
		},
		{
			name:     "skip font table",
			input:    `{\rtf1{\fonttbl{\f0 Arial;}}Hello}`,
			contains: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripRTF(tt.input)
			if !containsWords(got, tt.contains) {
				t.Errorf("stripRTF() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

func TestStripRTFSkipsDestinations(t *testing.T) {
	input := `{\rtf1{\fonttbl{\f0 Times;}}{\colortbl;\red0;}Visible text}`
	got := stripRTF(input)
	if containsWords(got, "Times") {
		t.Errorf("font table text should be stripped, got: %q", got)
	}
	if !containsWords(got, "Visible text") {
		t.Errorf("visible text should be present, got: %q", got)
	}
}
