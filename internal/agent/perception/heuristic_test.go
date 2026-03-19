package perception

import (
	"log/slog"
	"os"
	"testing"
)

func newTestFilter() *HeuristicFilter {
	return NewHeuristicFilter(HeuristicConfig{
		MinContentLength:   10,
		MaxContentLength:   100000,
		FrequencyThreshold: 5,
		FrequencyWindowMin: 10,
	}, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

func TestEvaluate_VenvPathHardReject(t *testing.T) {
	hf := newTestFilter()

	// Content loaded with high-signal keywords that would normally boost score above 0.2
	keywordRichContent := `def test_config_error():
    """Fix the deployment bug by updating the release config."""
    install_package("important-lib")
    raise ImportError("critical error in merge")
`

	tests := []struct {
		name string
		path string
	}{
		{"venv path", "/home/user/Projects/foo/venv/lib/python3.12/site-packages/pip/config.py"},
		{".venv path", "/home/user/Projects/foo/.venv/lib/python3.12/site-packages/pip/network/auth.py"},
		{"site-packages path", "/usr/lib/python3/dist-packages/site-packages/keyring/core.py"},
		{"node_modules path", "/home/user/Projects/app/node_modules/express/lib/router.js"},
		{"__pycache__ path", "/home/user/Projects/foo/__pycache__/module.cpython-312.pyc"},
		{"mypy_cache path", "/home/user/Projects/foo/.mypy_cache/3.12/module.meta.json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := Event{
				Source:  "filesystem",
				Type:    "file_modified",
				Path:    tc.path,
				Content: keywordRichContent,
			}

			result := hf.Evaluate(event)
			if result.Pass {
				t.Errorf("expected hard reject for %s, got Pass=true (score=%.2f, rationale=%q)",
					tc.path, result.Score, result.Rationale)
			}
			if result.Score != 0.0 {
				t.Errorf("expected score 0.0 for %s, got %.2f", tc.path, result.Score)
			}
		})
	}
}

func TestEvaluate_AppInternalPathHardReject(t *testing.T) {
	hf := newTestFilter()

	content := `{"error": "config merge failed", "fix": "update release"}`

	event := Event{
		Source:  "filesystem",
		Type:    "file_modified",
		Path:    "/home/user/.config/google-chrome/Default/Local Storage/leveldb/000123.log",
		Content: content,
	}

	result := hf.Evaluate(event)
	if result.Pass {
		t.Errorf("expected hard reject for chrome internal path, got Pass=true (score=%.2f)", result.Score)
	}
}

func TestEvaluate_TrivialCommandHardReject(t *testing.T) {
	hf := newTestFilter()

	// Bare "pwd" should be hard-rejected even if content somehow has keywords
	event := Event{
		Source:  "terminal",
		Type:    "command_executed",
		Content: "pwd",
	}

	result := hf.Evaluate(event)
	if result.Pass {
		t.Errorf("expected hard reject for trivial command, got Pass=true (score=%.2f)", result.Score)
	}
}

func TestEvaluate_NormalSourceCodePasses(t *testing.T) {
	hf := newTestFilter()

	event := Event{
		Source:  "filesystem",
		Type:    "file_modified",
		Path:    "/home/user/Projects/myapp/internal/server/handler.go",
		Content: "func handleRequest(w http.ResponseWriter, r *http.Request) { /* error handling */ }",
	}

	result := hf.Evaluate(event)
	if !result.Pass {
		t.Errorf("expected normal source code to pass, got Pass=false (score=%.2f, rationale=%q)",
			result.Score, result.Rationale)
	}
}

func TestEvaluate_DesktopNoiseHardReject(t *testing.T) {
	hf := newTestFilter()

	content := `{"error": "config merge failed", "fix": "update release"}`

	tests := []struct {
		name string
		path string
	}{
		{"wireplumber restore-stream", "/home/user/.local/state/wireplumber/restore-stream"},
		{"wireplumber default-routes", "/home/user/.local/state/wireplumber/default-routes"},
		{"copilot ide lock", "/home/user/.copilot/ide/346d20a4-955c-47fd-adec-84a8195fb292.lock"},
		{"snap revision", "/home/user/snap/code/current/.last_revision"},
		{"gtk bookmarks", "/home/user/.config/gtk-3.0/bookmarks"},
		{"egg-info", "/home/user/Projects/foo/foo.egg-info/PKG-INFO"},
		{"mnemonic internal", "/home/user/.mnemonic/memory.db-journal"},
		{"claude internal", "/home/user/.claude/settings.json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := Event{
				Source:  "filesystem",
				Type:    "file_modified",
				Path:    tc.path,
				Content: content,
			}

			result := hf.Evaluate(event)
			if result.Pass {
				t.Errorf("expected hard reject for %s, got Pass=true (score=%.2f, rationale=%q)",
					tc.path, result.Score, result.Rationale)
			}
			if result.Score != 0.0 {
				t.Errorf("expected score 0.0 for %s, got %.2f", tc.path, result.Score)
			}
		})
	}
}

func TestEvaluate_LockfileHardReject(t *testing.T) {
	hf := newTestFilter()

	content := `github.com/google/uuid v1.6.0 h1:abc123=\ngithub.com/google/uuid v1.6.0/go.mod h1:def456=`

	tests := []struct {
		name string
		path string
	}{
		{"go.sum", "/home/user/Projects/myapp/go.sum"},
		{"package-lock.json", "/home/user/Projects/app/package-lock.json"},
		{"yarn.lock", "/home/user/Projects/app/yarn.lock"},
		{"Cargo.lock", "/home/user/Projects/rs/Cargo.lock"},
		{"poetry.lock", "/home/user/Projects/py/poetry.lock"},
		{"pnpm-lock.yaml", "/home/user/Projects/app/pnpm-lock.yaml"},
		{"Gemfile.lock", "/home/user/Projects/rb/Gemfile.lock"},
		{"composer.lock", "/home/user/Projects/php/composer.lock"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := Event{
				Source:  "filesystem",
				Type:    "file_modified",
				Path:    tc.path,
				Content: content,
			}

			result := hf.Evaluate(event)
			if result.Pass {
				t.Errorf("expected hard reject for %s, got Pass=true (score=%.2f, rationale=%q)",
					tc.path, result.Score, result.Rationale)
			}
			if result.Score != 0.0 {
				t.Errorf("expected score 0.0 for %s, got %.2f", tc.path, result.Score)
			}
		})
	}
}

func TestEvaluate_ClipboardURLHardReject(t *testing.T) {
	hf := newTestFilter()

	event := Event{
		Source:  "clipboard",
		Type:    "clipboard_change",
		Content: "https://github.com/some/repo/issues/123",
	}

	result := hf.Evaluate(event)
	if result.Pass {
		t.Errorf("expected hard reject for URL-only clipboard, got Pass=true (score=%.2f)", result.Score)
	}
}
