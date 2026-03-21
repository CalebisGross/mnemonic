package concepts

import "testing"

func TestFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "go agent file",
			path:     "internal/agent/retrieval/agent.go",
			expected: []string{"agent", "retrieval"},
		},
		{
			name:     "mcp server",
			path:     "internal/mcp/server.go",
			expected: []string{"mcp", "server"},
		},
		{
			name:     "absolute path filters OS segments",
			path:     "/home/user/Projects/mnemonic/internal/store/sqlite/sqlite.go",
			expected: []string{"mnemonic", "store", "sqlite"},
		},
		{
			name:     "absolute path with home dir stripped",
			path:     homeDir + "/Projects/mnemonic/internal/agent/encoding/agent.go",
			expected: []string{"mnemonic", "agent", "encoding"},
		},
		{
			name:     "test file with underscores",
			path:     "internal/agent/perception/heuristic_filter_test.go",
			expected: []string{"agent", "perception", "heuristic", "filter"},
		},
		{
			name:     "config yaml",
			path:     "config.yaml",
			expected: []string{"config"},
		},
		{
			name:     "short segments filtered",
			path:     "a/b/cd/mcp.go",
			expected: []string{"mcp"},
		},
		{
			name:     "no duplicates",
			path:     "agent/agent/agent.go",
			expected: []string{"agent"},
		},
		{
			name:     "empty path",
			path:     "",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromPath(tc.path)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
			for i := range tc.expected {
				if got[i] != tc.expected[i] {
					t.Fatalf("expected[%d] = %q, got %q (full: %v)", i, tc.expected[i], got[i], got)
				}
			}
		})
	}
}

func TestFromEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expected  string
	}{
		{"file created", "file_created", "created"},
		{"file modified", "file_modified", "modified"},
		{"file deleted", "file_deleted", "deleted"},
		{"dir activity skipped", "dir_activity", ""},
		{"empty string", "", ""},
		{"command executed skipped", "command_executed", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromEventType(tc.eventType)
			if got != tc.expected {
				t.Fatalf("FromEventType(%q) = %q, want %q", tc.eventType, got, tc.expected)
			}
		})
	}
}

func TestFromCommand(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "git commit with flags",
			content:  "git commit -m 'fix bug'",
			expected: []string{"git", "commit"},
		},
		{
			name:     "git push",
			content:  "git push origin main",
			expected: []string{"git", "push"},
		},
		{
			name:     "make build",
			content:  "make build",
			expected: []string{"make", "build"},
		},
		{
			name:     "go test with flags",
			content:  "go test -v ./...",
			expected: []string{"go", "test"},
		},
		{
			name:     "docker run with flags",
			content:  "docker -D run --rm ubuntu",
			expected: []string{"docker", "run"},
		},
		{
			name:     "simple command",
			content:  "ls -la",
			expected: []string{"ls"},
		},
		{
			name:     "simple command no flags",
			content:  "pwd",
			expected: []string{"pwd"},
		},
		{
			name:     "npm install",
			content:  "npm install express",
			expected: []string{"npm", "install"},
		},
		{
			name:     "empty string",
			content:  "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			content:  "   ",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromCommand(tc.content)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
			for i := range tc.expected {
				if got[i] != tc.expected[i] {
					t.Fatalf("expected[%d] = %q, got %q (full: %v)", i, tc.expected[i], got[i], got)
				}
			}
		})
	}
}
