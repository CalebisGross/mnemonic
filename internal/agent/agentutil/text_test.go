package agentutil

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		max     int
		want    string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"unicode fits", "Hello, 世界!", 10, "Hello, 世界!"},
		{"unicode truncated", "Hello, 世界!", 8, "Hello, 世..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.content, tt.max)
			if got != tt.want {
				t.Errorf("Truncate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeduplicateConcepts(t *testing.T) {
	got := DeduplicateConcepts([]string{"Go", "go", "Python", "GO", "python"})
	if len(got) != 2 {
		t.Fatalf("got %d concepts, want 2: %v", len(got), got)
	}
	if got[0] != "Go" || got[1] != "Python" {
		t.Errorf("got %v, want [Go Python]", got)
	}
}
