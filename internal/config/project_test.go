package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectResolver_AliasMatch(t *testing.T) {
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:    "mnemonic",
			Paths:   []string{"/home/user/Projects/mem"},
			Aliases: []string{"mem", "sdk", "mnemonic-sdk"},
		},
	})

	tests := []struct {
		input string
		want  string
	}{
		{"mem", "mnemonic"},
		{"sdk", "mnemonic"},
		{"mnemonic-sdk", "mnemonic"},
		{"mnemonic", "mnemonic"},
		{"MEM", "mnemonic"}, // case-insensitive
		{"Mnemonic", "mnemonic"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := pr.Resolve(tt.input)
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProjectResolver_PathPrefixMatch(t *testing.T) {
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:  "mnemonic",
			Paths: []string{"/home/user/Projects/mem"},
		},
		{
			Name:  "felixlm",
			Paths: []string{"/home/user/Projects/felixlm"},
		},
	})

	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/Projects/mem/internal/foo.go", "mnemonic"},
		{"/home/user/Projects/mem", "mnemonic"},
		{"/home/user/Projects/felixlm/src/main.go", "felixlm"},
		{"/home/user/Projects/other/file.go", "other"}, // fallback heuristic
		{"/tmp/random/file.go", ""},                    // no match
	}

	for _, tt := range tests {
		got := pr.Resolve(tt.input)
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProjectResolver_PathBoundary(t *testing.T) {
	// Ensure "/home/user/Projects/mem" doesn't match "/home/user/Projects/memory"
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:  "mnemonic",
			Paths: []string{"/home/user/Projects/mem"},
		},
	})

	got := pr.Resolve("/home/user/Projects/memory/file.go")
	if got == "mnemonic" {
		t.Error("Resolve should not match partial directory name 'memory' against prefix 'mem'")
	}
	// Should fall back to heuristic and return "memory"
	if got != "memory" {
		t.Errorf("Resolve(/home/user/Projects/memory/file.go) = %q, want %q", got, "memory")
	}
}

func TestProjectResolver_LongestPathWins(t *testing.T) {
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:  "parent",
			Paths: []string{"/home/user/Projects"},
		},
		{
			Name:  "child",
			Paths: []string{"/home/user/Projects/child"},
		},
	})

	got := pr.Resolve("/home/user/Projects/child/file.go")
	if got != "child" {
		t.Errorf("Resolve should prefer longer path match, got %q, want %q", got, "child")
	}
}

func TestProjectResolver_FallbackHeuristic(t *testing.T) {
	// Empty config — pure heuristic mode
	pr := NewProjectResolver(nil)

	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/Projects/myapp/file.go", "myapp"},
		{"/home/user/src/webapp/file.go", "webapp"},
		{"/home/user/repos/tool/main.go", "tool"},
		{"/home/user/workspace/project/x.go", "project"},
		{"/tmp/random/file.go", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := pr.Resolve(tt.input)
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProjectResolver_FallbackWithAliasResolution(t *testing.T) {
	// Heuristic infers "mem" from path, alias resolves it to "mnemonic"
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:    "mnemonic",
			Aliases: []string{"mem"},
			// No paths registered — relies on heuristic + alias
		},
	})

	got := pr.Resolve("/home/user/Projects/mem/internal/foo.go")
	if got != "mnemonic" {
		t.Errorf("Resolve should resolve heuristic result through aliases, got %q, want %q", got, "mnemonic")
	}
}

func TestProjectResolver_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	absPath := filepath.Join(home, "Projects", "mem")
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:  "mnemonic",
			Paths: []string{absPath},
		},
	})

	got := pr.Resolve("~/Projects/mem/internal/foo.go")
	if got != "mnemonic" {
		t.Errorf("Resolve with ~ should expand and match, got %q, want %q", got, "mnemonic")
	}
}

func TestProjectResolver_EmptyInput(t *testing.T) {
	pr := NewProjectResolver([]ProjectConfig{
		{Name: "mnemonic", Paths: []string{"/home/user/Projects/mem"}},
	})

	if got := pr.Resolve(""); got != "" {
		t.Errorf("Resolve(\"\") = %q, want empty string", got)
	}
}

func TestProjectResolver_MultipleProjects(t *testing.T) {
	pr := NewProjectResolver([]ProjectConfig{
		{
			Name:    "mnemonic",
			Paths:   []string{"/home/user/Projects/mem"},
			Aliases: []string{"mem"},
		},
		{
			Name:    "petra",
			Paths:   []string{"/home/user/Projects/Petra-PB"},
			Aliases: []string{"petra-pb"},
		},
	})

	tests := []struct {
		input string
		want  string
	}{
		{"mem", "mnemonic"},
		{"petra-pb", "petra"},
		{"/home/user/Projects/mem/foo.go", "mnemonic"},
		{"/home/user/Projects/Petra-PB/src/app.ts", "petra"},
	}

	for _, tt := range tests {
		got := pr.Resolve(tt.input)
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
