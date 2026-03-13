package agentutil

import "testing"

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "raw JSON",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "with leading whitespace",
			input: `   {"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "json code fence",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "plain code fence",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "surrounded by prose",
			input: "Here is the result:\n{\"key\": \"value\"}\nDone.",
			want:  `{"key": "value"}`,
		},
		{
			name:  "nested braces",
			input: `Some text {"outer": {"inner": "val"}} more text`,
			want:  `{"outer": {"inner": "val"}}`,
		},
		{
			name:  "braces in quoted strings",
			input: `prefix {"text": "has {braces} inside"} suffix`,
			want:  `{"text": "has {braces} inside"}`,
		},
		{
			name:  "no JSON",
			input: "just some text",
			want:  "just some text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractJSON(tt.input)
			if got != tt.want {
				t.Errorf("ExtractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}
