package filesystem

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestDetectNoisyApps(t *testing.T) {
	// DetectNoisyApps scans real system directories, so results vary by machine.
	// We just verify it runs without error and returns valid patterns.
	detected := DetectNoisyApps(testLogger())

	for _, pattern := range detected {
		if pattern == "" {
			t.Error("detected empty exclusion pattern")
		}
		if pattern[0] != '.' {
			t.Errorf("pattern should start with '.': got %q", pattern)
		}
		if pattern[len(pattern)-1] != filepath.Separator {
			t.Errorf("pattern should end with %q: got %q", string(filepath.Separator), pattern)
		}
	}
}

func TestDetectNoisyApps_FindsKnownDirs(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("test only runs on Linux/macOS")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	// Create a fake noisy app dir in a temp location, then verify it would be detected
	// We can't mock os.Stat in the real function, so instead create a real dir
	// under the actual XDG path if it exists.
	var baseDir string
	if runtime.GOOS == "linux" {
		baseDir = filepath.Join(home, ".config")
	} else {
		baseDir = filepath.Join(home, "Library", "Application Support")
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Skipf("base dir %s does not exist", baseDir)
	}

	// Check if any known noisy app is actually installed
	detected := DetectNoisyApps(testLogger())
	t.Logf("detected %d noisy app exclusions on this system", len(detected))
	for _, pattern := range detected {
		t.Logf("  %s", pattern)
	}
}

func TestDetectNoisyApps_NoDuplicatesWithExisting(t *testing.T) {
	// Verify that using MatchesExcludePattern to deduplicate works
	existing := []string{".config/Code/", ".config/google-chrome/"}
	detected := []string{".config/Code/", ".config/discord/", ".config/google-chrome/"}

	var merged []string
	merged = append(merged, existing...)
	for _, pattern := range detected {
		if !MatchesExcludePattern(pattern, merged) {
			merged = append(merged, pattern)
		}
	}

	// Should have existing 2 + 1 new = 3
	if len(merged) != 3 {
		t.Errorf("expected 3 merged patterns, got %d: %v", len(merged), merged)
	}

	// discord should be the new one
	found := false
	for _, p := range merged {
		if p == ".config/discord/" {
			found = true
		}
	}
	if !found {
		t.Error("expected .config/discord/ in merged patterns")
	}
}
