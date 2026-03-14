package perception

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestExtractPrefix(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	// sep builds expected patterns with the platform's native separator.
	sep := string(filepath.Separator)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "config app dir",
			path: filepath.Join(home, ".config", "Code", "User", "settings.json"),
			want: "." + sep + filepath.Join(".config", "Code") + sep,
		},
		{
			name: "local share app dir",
			path: filepath.Join(home, ".local", "share", "gnome-shell", "extensions", "foo"),
			want: "." + sep + filepath.Join(".local", "share", "gnome-shell") + sep,
		},
		{
			name: "not under home",
			path: filepath.Join(string(filepath.Separator), "tmp", "foo", "bar"),
			want: "",
		},
		{
			name: "no recognizable base",
			path: filepath.Join(home, "Documents", "projects", "foo.go"),
			want: "",
		},
		{
			name: "config dir with no app subdir",
			path: filepath.Join(home, ".config", "somefile"),
			want: "",
		},
		{
			name: "project venv",
			path: filepath.Join(home, "Projects", "felixlm", ".venv", "lib", "python3.12", "site-packages", "pip", "config.py"),
			want: "." + sep + filepath.Join("Projects", "felixlm", ".venv") + sep,
		},
		{
			name: "project node_modules",
			path: filepath.Join(home, "Projects", "webapp", "node_modules", "express", "lib", "router.js"),
			want: "." + sep + filepath.Join("Projects", "webapp", "node_modules") + sep,
		},
		{
			name: "project pycache",
			path: filepath.Join(home, "Projects", "myapp", "__pycache__", "module.cpython-312.pyc"),
			want: "." + sep + filepath.Join("Projects", "myapp", "__pycache__") + sep,
		},
		{
			name: "normal project file (no noise dir)",
			path: filepath.Join(home, "Projects", "myapp", "internal", "server", "handler.go"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPrefix(tt.path)
			if got != tt.want {
				t.Errorf("extractPrefix(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRejectionTracker_PromotesAfterThreshold(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	sep := string(filepath.Separator)

	var promoted []string
	onPromote := func(pattern string) {
		promoted = append(promoted, pattern)
	}

	rt := newRejectionTracker(
		rejectionTrackerConfig{
			Threshold:   5,
			Window:      1 * time.Hour,
			MaxPromoted: 10,
		},
		testLogger(),
		onPromote,
	)

	path := filepath.Join(home, ".config", "Code", "User", "settings.json")

	// Record 4 rejections — should not promote yet
	for i := 0; i < 4; i++ {
		rt.recordRejection(path)
	}
	if len(promoted) != 0 {
		t.Errorf("expected no promotions after 4 rejections, got %d", len(promoted))
	}

	// 5th rejection should trigger promotion
	rt.recordRejection(path)
	if len(promoted) != 1 {
		t.Fatalf("expected 1 promotion after 5 rejections, got %d", len(promoted))
	}
	wantPattern := "." + sep + filepath.Join(".config", "Code") + sep
	if promoted[0] != wantPattern {
		t.Errorf("promoted pattern = %q, want %q", promoted[0], wantPattern)
	}

	// Further rejections for the same prefix should be no-ops
	rt.recordRejection(path)
	if len(promoted) != 1 {
		t.Errorf("expected no additional promotions, got %d", len(promoted))
	}
}

func TestRejectionTracker_MaxPromotedCap(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	var promoted []string
	onPromote := func(pattern string) {
		promoted = append(promoted, pattern)
	}

	rt := newRejectionTracker(
		rejectionTrackerConfig{
			Threshold:   1,
			Window:      1 * time.Hour,
			MaxPromoted: 2,
		},
		testLogger(),
		onPromote,
	)

	// Promote 2 distinct prefixes
	rt.recordRejection(filepath.Join(home, ".config", "AppA", "file"))
	rt.recordRejection(filepath.Join(home, ".config", "AppB", "file"))

	if len(promoted) != 2 {
		t.Fatalf("expected 2 promotions, got %d", len(promoted))
	}

	// 3rd distinct prefix should be capped
	rt.recordRejection(filepath.Join(home, ".config", "AppC", "file"))
	if len(promoted) != 2 {
		t.Errorf("expected cap at 2 promotions, got %d", len(promoted))
	}
}

func TestRejectionTracker_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "learned.txt")

	sep := string(filepath.Separator)

	var promoted []string
	onPromote := func(pattern string) {
		promoted = append(promoted, pattern)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	// First tracker: promote a pattern
	rt1 := newRejectionTracker(
		rejectionTrackerConfig{
			Threshold:   1,
			Window:      1 * time.Hour,
			MaxPromoted: 10,
			PersistPath: persistPath,
		},
		testLogger(),
		onPromote,
	)
	rt1.recordRejection(filepath.Join(home, ".config", "Code", "User", "settings.json"))

	if len(promoted) != 1 {
		t.Fatalf("expected 1 promotion, got %d", len(promoted))
	}

	// Verify file was written
	data, err := os.ReadFile(persistPath)
	if err != nil {
		t.Fatalf("failed to read persist file: %v", err)
	}
	if string(data) == "" {
		t.Fatal("persist file is empty")
	}

	// Second tracker: should load the persisted exclusion
	rt2 := newRejectionTracker(
		rejectionTrackerConfig{
			Threshold:   1,
			Window:      1 * time.Hour,
			MaxPromoted: 10,
			PersistPath: persistPath,
		},
		testLogger(),
		nil,
	)

	exclusions := rt2.learnedExclusions()
	if len(exclusions) != 1 {
		t.Fatalf("expected 1 loaded exclusion, got %d", len(exclusions))
	}
	wantExclusion := "." + sep + filepath.Join(".config", "Code") + sep
	if exclusions[0] != wantExclusion {
		t.Errorf("loaded exclusion = %q, want %q", exclusions[0], wantExclusion)
	}
}
