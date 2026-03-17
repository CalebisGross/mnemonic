package extract

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// skipDestinations are RTF group destinations whose content should be ignored.
var skipDestinations = map[string]bool{
	"fonttbl":    true,
	"colortbl":   true,
	"stylesheet": true,
	"info":       true,
	"pict":       true,
	"object":     true,
	"fldinst":    true,
	"header":     true,
	"footer":     true,
	"headerl":    true,
	"headerr":    true,
	"footerl":    true,
	"footerr":    true,
}

// RTFExtractor extracts text from RTF files using a pure-Go parser.
type RTFExtractor struct{}

// Available always returns true (pure Go, no external dependency).
func (r *RTFExtractor) Available() bool { return true }

// Extract reads the RTF file, strips control words, and returns plain text
// grouped into paragraph chunks.
func (r *RTFExtractor) Extract(path string, maxBytes int, log *slog.Logger) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("reading rtf %s: %w", path, err)
	}

	text := stripRTF(string(data))
	if strings.TrimSpace(text) == "" {
		return Result{}, nil
	}

	// Split into paragraphs and group into chunks
	var paragraphs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paragraphs = append(paragraphs, line)
		}
	}

	if len(paragraphs) == 0 {
		return Result{}, nil
	}

	fullText := strings.Join(paragraphs, "\n")
	chunks := GroupParagraphs(paragraphs)

	metadata := map[string]any{
		"extracted": true,
		"extractor": "rtf",
	}

	return Result{
		FullText: Truncate(fullText, maxBytes),
		Chunks:   chunks,
		Metadata: metadata,
	}, nil
}

// stripRTF parses RTF content and returns plain text.
// Handles group nesting, control words, hex escapes, and skip destinations.
func stripRTF(input string) string {
	var result strings.Builder
	depth := 0
	skipDepth := -1 // depth at which we entered a skip destination
	i := 0
	n := len(input)

	for i < n {
		ch := input[i]

		switch ch {
		case '{':
			depth++
			i++

			// Check if this group starts with a skip destination
			if skipDepth < 0 && i < n {
				word := peekControlWord(input, i)
				if word != "" && skipDestinations[word] {
					skipDepth = depth
				}
				// \* marks an ignorable destination — check the next word
				if word == "*" {
					j := i + len(word) + 1 // skip past \*
					for j < n && input[j] == ' ' {
						j++
					}
					next := peekControlWord(input, j)
					if next != "" && skipDestinations[next] {
						skipDepth = depth
					}
				}
			}

		case '}':
			if depth == skipDepth {
				skipDepth = -1
			}
			depth--
			i++

		case '\\':
			if skipDepth >= 0 {
				i++
				skipPastControlWord(input, &i, n)
				continue
			}
			i++
			if i >= n {
				continue
			}

			next := input[i]
			switch next {
			case '\\', '{', '}':
				// Escaped literal character
				result.WriteByte(next)
				i++
			case '\'':
				// Hex character escape: \'xx
				if i+2 < n {
					hexStr := input[i+1 : i+3]
					if val, err := strconv.ParseUint(hexStr, 16, 8); err == nil {
						result.WriteByte(byte(val))
					}
					i += 3
				} else {
					i++
				}
			case '\n', '\r':
				// Line break in RTF source — paragraph break
				result.WriteByte('\n')
				i++
			default:
				// Control word
				word, param, end := readControlWord(input, i, n)
				i = end

				if skipDepth < 0 {
					switch word {
					case "par", "line":
						result.WriteByte('\n')
					case "tab":
						result.WriteByte('\t')
					case "u":
						// Unicode escape: \uN followed by replacement char
						if codepoint, err := strconv.Atoi(param); err == nil {
							if codepoint < 0 {
								codepoint += 65536
							}
							result.WriteRune(rune(codepoint))
						}
						// Skip the replacement character
						if i < n && input[i] != '\\' && input[i] != '{' && input[i] != '}' {
							i++
						}
					}
				}
			}

		default:
			if skipDepth < 0 && ch != '\r' && ch != '\n' {
				result.WriteByte(ch)
			}
			i++
		}
	}

	return result.String()
}

// peekControlWord returns the control word starting at position i (after the backslash).
func peekControlWord(input string, i int) string {
	n := len(input)
	if i >= n || input[i] != '\\' {
		return ""
	}
	i++ // skip backslash
	if i >= n {
		return ""
	}
	// Special single-char control symbols
	if input[i] == '*' {
		return "*"
	}

	start := i
	for i < n && input[i] >= 'a' && input[i] <= 'z' {
		i++
	}
	if i == start {
		return ""
	}
	return input[start:i]
}

// readControlWord reads a control word and its optional numeric parameter.
// Returns the word, parameter string, and new position.
func readControlWord(input string, i, n int) (string, string, int) {
	start := i
	for i < n && input[i] >= 'a' && input[i] <= 'z' {
		i++
	}
	word := input[start:i]

	// Read optional numeric parameter (including negative)
	paramStart := i
	if i < n && (input[i] == '-' || (input[i] >= '0' && input[i] <= '9')) {
		i++
		for i < n && input[i] >= '0' && input[i] <= '9' {
			i++
		}
	}
	param := input[paramStart:i]

	// Consume trailing space delimiter
	if i < n && input[i] == ' ' {
		i++
	}

	return word, param, i
}

// skipPastControlWord advances past a control word without processing it.
func skipPastControlWord(input string, i *int, n int) {
	pos := *i
	if pos >= n {
		return
	}
	ch := input[pos]

	// Escaped chars
	if ch == '\\' || ch == '{' || ch == '}' || ch == '\'' {
		if ch == '\'' && pos+2 < n {
			*i = pos + 3
		} else {
			*i = pos + 1
		}
		return
	}

	// Control word
	for pos < n && input[pos] >= 'a' && input[pos] <= 'z' {
		pos++
	}
	// Skip numeric parameter
	if pos < n && (input[pos] == '-' || (input[pos] >= '0' && input[pos] <= '9')) {
		pos++
		for pos < n && input[pos] >= '0' && input[pos] <= '9' {
			pos++
		}
	}
	// Consume trailing space
	if pos < n && input[pos] == ' ' {
		pos++
	}
	*i = pos
}
