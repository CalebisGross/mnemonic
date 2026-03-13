package agentutil

import "strings"

// Truncate truncates content to maxChars runes, appending "..." if truncated.
// It uses a fast path when byte length already fits.
func Truncate(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}
	return string(runes[:maxChars]) + "..."
}

// DeduplicateConcepts returns unique concepts (case-insensitive),
// preserving the first occurrence's casing.
func DeduplicateConcepts(concepts []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, c := range concepts {
		lower := strings.ToLower(c)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, c)
		}
	}
	return result
}
