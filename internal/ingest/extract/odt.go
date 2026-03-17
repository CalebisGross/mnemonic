package extract

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// ODTExtractor extracts text from .odt files using stdlib ZIP+XML parsing.
type ODTExtractor struct{}

// Available always returns true (pure Go, no external dependency).
func (o *ODTExtractor) Available() bool { return true }

// Extract opens the ODT file, reads content.xml, and extracts text from
// <text:p> elements. Groups paragraphs into chunks using GroupParagraphs.
func (o *ODTExtractor) Extract(path string, maxBytes int, log *slog.Logger) (Result, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return Result{}, fmt.Errorf("opening odt %s: %w", path, err)
	}
	defer func() { _ = r.Close() }()

	// Find content.xml
	var contentFile *zip.File
	for _, f := range r.File {
		if f.Name == "content.xml" {
			contentFile = f
			break
		}
	}
	if contentFile == nil {
		return Result{}, fmt.Errorf("content.xml not found in odt %s", path)
	}

	rc, err := contentFile.Open()
	if err != nil {
		return Result{}, fmt.Errorf("opening content.xml in odt %s: %w", path, err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return Result{}, fmt.Errorf("reading content.xml in odt %s: %w", path, err)
	}

	paragraphs := extractODTParagraphs(data)
	if len(paragraphs) == 0 {
		return Result{}, nil
	}

	fullText := strings.Join(paragraphs, "\n")
	chunks := GroupParagraphs(paragraphs)

	metadata := map[string]any{
		"extracted": true,
		"extractor": "odt",
	}

	return Result{
		FullText: Truncate(fullText, maxBytes),
		Chunks:   chunks,
		Metadata: metadata,
	}, nil
}

// extractODTParagraphs parses ODF content.xml and returns text from <text:p> elements.
func extractODTParagraphs(data []byte) []string {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	var paragraphs []string
	var currentPara strings.Builder
	inParagraph := false
	depth := 0 // nesting depth within a paragraph

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if isODTTextElement(t.Name, "p") || isODTTextElement(t.Name, "h") {
				inParagraph = true
				depth = 1
				currentPara.Reset()
			} else if inParagraph {
				depth++
				// <text:tab/> becomes a tab
				if isODTTextElement(t.Name, "tab") {
					currentPara.WriteByte('\t')
				}
				// <text:s/> becomes a space
				if isODTTextElement(t.Name, "s") {
					currentPara.WriteByte(' ')
				}
				// <text:line-break/> becomes a newline
				if isODTTextElement(t.Name, "line-break") {
					currentPara.WriteByte('\n')
				}
			}

		case xml.EndElement:
			if inParagraph {
				depth--
				if (isODTTextElement(t.Name, "p") || isODTTextElement(t.Name, "h")) && depth <= 0 {
					text := strings.TrimSpace(currentPara.String())
					if text != "" {
						paragraphs = append(paragraphs, text)
					}
					inParagraph = false
				}
			}

		case xml.CharData:
			if inParagraph {
				currentPara.Write(t)
			}
		}
	}

	return paragraphs
}

// isODTTextElement checks if an XML name matches a text-namespace element.
func isODTTextElement(name xml.Name, local string) bool {
	return name.Local == local &&
		(strings.Contains(name.Space, "opendocument") ||
			strings.HasSuffix(name.Space, "/text"))
}
