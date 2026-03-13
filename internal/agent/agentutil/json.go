package agentutil

import "strings"

// ExtractJSON extracts a JSON object from an LLM response that may contain
// markdown fences, prose, or other surrounding text.
//
// It tries, in order:
//  1. Raw string starting with '{'
//  2. Content between ```json ... ``` fences
//  3. Content between ``` ... ``` fences (if it starts with '{')
//  4. First '{' to its matching '}' using brace-depth tracking
//     (handles nested braces and quoted strings correctly)
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 0 && s[0] == '{' {
		return s
	}

	// Look for JSON between ```json ... ``` fences
	if idx := strings.Index(s, "```json"); idx != -1 {
		start := idx + 7
		end := strings.Index(s[start:], "```")
		if end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}

	// Look for JSON between ``` ... ``` fences
	if idx := strings.Index(s, "```"); idx != -1 {
		start := idx + 3
		if start < len(s) && s[start] == '\n' {
			start++
		}
		end := strings.Index(s[start:], "```")
		if end != -1 {
			candidate := strings.TrimSpace(s[start : start+end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate
			}
		}
	}

	// Find the first '{' and its matching '}' using brace counting.
	// This correctly handles nested objects and quoted strings.
	first := strings.Index(s, "{")
	if first != -1 {
		depth := 0
		inString := false
		escaped := false
		for i := first; i < len(s); i++ {
			if escaped {
				escaped = false
				continue
			}
			ch := s[i]
			if ch == '\\' && inString {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					return s[first : i+1]
				}
			}
		}
	}

	return s
}
