package perception

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindGitDir(t *testing.T) {
	// Create a temp directory structure with .git
	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(tmp, "src", "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"file in repo root", filepath.Join(tmp, "README.md"), gitDir},
		{"file in nested dir", filepath.Join(subDir, "main.go"), gitDir},
		{"file outside any repo", "/tmp/random/file.txt", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findGitDir(tt.path)
			if got != tt.want {
				t.Errorf("findGitDir(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsRecentGitOp(t *testing.T) {
	// Create a temp repo with .git and sentinel files
	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	pa := &PerceptionAgent{
		gitOpCooldown: 10 * time.Second,
	}

	filePath := filepath.Join(tmp, "CHANGELOG.md")

	// No sentinel files exist yet — should not suppress
	if pa.isRecentGitOp(filePath) {
		t.Error("expected false when no sentinel files exist")
	}

	// Create FETCH_HEAD with current mtime — should suppress
	fetchHead := filepath.Join(gitDir, "FETCH_HEAD")
	if err := os.WriteFile(fetchHead, []byte("abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !pa.isRecentGitOp(filePath) {
		t.Error("expected true when FETCH_HEAD was just modified")
	}

	// Set FETCH_HEAD mtime to 30 seconds ago — should not suppress
	old := time.Now().Add(-30 * time.Second)
	if err := os.Chtimes(fetchHead, old, old); err != nil {
		t.Fatal(err)
	}
	if pa.isRecentGitOp(filePath) {
		t.Error("expected false when FETCH_HEAD is old")
	}

	// Create ORIG_HEAD with current mtime (simulates merge/rebase) — should suppress
	origHead := filepath.Join(gitDir, "ORIG_HEAD")
	if err := os.WriteFile(origHead, []byte("def456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !pa.isRecentGitOp(filePath) {
		t.Error("expected true when ORIG_HEAD was just modified")
	}
}

func TestIsRecentGitOp_NotInRepo(t *testing.T) {
	pa := &PerceptionAgent{
		gitOpCooldown: 10 * time.Second,
	}

	// A path that's definitely not in a git repo
	if pa.isRecentGitOp("/tmp/not-a-repo/somefile.txt") {
		t.Error("expected false for path outside any git repo")
	}
}
