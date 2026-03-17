package extract

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var slideFilePattern = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// PPTXExtractor extracts text from .pptx files using stdlib ZIP+XML parsing.
type PPTXExtractor struct{}

// Available always returns true (pure Go, no external dependency).
func (p *PPTXExtractor) Available() bool { return true }

// Extract opens the PPTX file, reads each slide's XML, and extracts text
// from <a:t> elements. Returns one Chunk per slide.
func (p *PPTXExtractor) Extract(path string, maxBytes int, log *slog.Logger) (Result, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return Result{}, fmt.Errorf("opening pptx %s: %w", path, err)
	}
	defer func() { _ = r.Close() }()

	// Collect slide files sorted by slide number
	type slideEntry struct {
		num  int
		file *zip.File
	}
	var slides []slideEntry

	for _, f := range r.File {
		matches := slideFilePattern.FindStringSubmatch(f.Name)
		if matches == nil {
			continue
		}
		num, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		slides = append(slides, slideEntry{num: num, file: f})
	}

	sort.Slice(slides, func(i, j int) bool {
		return slides[i].num < slides[j].num
	})

	if len(slides) == 0 {
		return Result{}, nil
	}

	// Extract text from each slide
	var chunks []Chunk
	var allText strings.Builder

	for _, s := range slides {
		text, err := extractSlideText(s.file)
		if err != nil {
			log.Warn("failed to parse slide", "file", s.file.Name, "error", err)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		if allText.Len() > 0 {
			allText.WriteByte('\n')
		}
		allText.WriteString(text)

		chunks = append(chunks, Chunk{
			Text:       text,
			PageNumber: s.num,
		})
	}

	if allText.Len() == 0 {
		return Result{}, nil
	}

	metadata := map[string]any{
		"extracted":   true,
		"extractor":   "pptx",
		"slide_count": len(chunks),
	}

	return Result{
		FullText: Truncate(allText.String(), maxBytes),
		Chunks:   chunks,
		Metadata: metadata,
	}, nil
}

// extractSlideText parses a slide XML file and returns all text content.
func extractSlideText(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	// Walk the XML looking for <a:t> elements
	var text strings.Builder
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	inText := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// <a:t> in the DrawingML namespace holds text
			if t.Name.Local == "t" && isDrawingMLNamespace(t.Name.Space) {
				inText = true
			}
		case xml.EndElement:
			if t.Name.Local == "t" && isDrawingMLNamespace(t.Name.Space) {
				inText = false
			}
			// Add line break after each paragraph end
			if t.Name.Local == "p" && isDrawingMLNamespace(t.Name.Space) {
				if text.Len() > 0 {
					text.WriteByte('\n')
				}
			}
		case xml.CharData:
			if inText {
				text.Write(t)
			}
		}
	}

	return text.String(), nil
}

// isDrawingMLNamespace checks if the namespace is a DrawingML namespace.
func isDrawingMLNamespace(ns string) bool {
	return strings.Contains(ns, "drawingml") ||
		strings.Contains(ns, "schemas.openxmlformats.org/drawingml")
}
