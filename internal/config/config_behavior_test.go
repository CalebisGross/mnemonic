package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Config Behavioral Tests — verify loading, parsing, and override behaviors
// ---------------------------------------------------------------------------

func TestDurationParsingEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"0s", 0, false},
		{"0m", 0, false},
		{"0h", 0, false},
		{"7w", 7 * 7 * 24 * time.Hour, false},
		{"365d", 365 * 24 * time.Hour, false},
		{"0.1d", time.Duration(float64(24*time.Hour) * 0.1), false},
		{"52w", 52 * 7 * 24 * time.Hour, false},
		{"1h30m", 90 * time.Minute, false}, // standard Go duration
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseDurationString(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseDurationString(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.expected {
				t.Fatalf("parseDurationString(%q) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestPathExpansionTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde_slash_foo", "~/foo", filepath.Join(home, "foo")},
		{"tilde_only", "~", home},
		{"absolute_unchanged", "/tmp/test.db", "/tmp/test.db"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandPath(tc.input)
			if err != nil {
				t.Fatalf("expandPath(%q) error: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("expandPath(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestResolvePathRelativeToConfigDir(t *testing.T) {
	// Use a temp directory as the config dir so paths are valid on all platforms.
	configDir := t.TempDir()

	relPath := filepath.Join("data", "memory.db")
	absPath := filepath.Join(configDir, "existing", "memory.db")

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"relative_path", relPath, filepath.Join(configDir, relPath)},
		{"absolute_path_unchanged", absPath, absPath},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePath(tc.path, configDir)
			if err != nil {
				t.Fatalf("resolvePath(%q, %q) error: %v", tc.path, configDir, err)
			}
			if got != tc.expected {
				t.Errorf("resolvePath(%q, %q) = %q, want %q", tc.path, configDir, got, tc.expected)
			}
		})
	}
}

func TestEnvVarOverrideLLMAPIKey(t *testing.T) {
	// Set the env var, process config, verify it's picked up
	testKey := "test-api-key-12345"
	t.Setenv("LLM_API_KEY", testKey)

	cfg := Default()
	if err := cfg.process(t.TempDir()); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	if cfg.LLM.APIKey != testKey {
		t.Errorf("expected APIKey=%q, got %q", testKey, cfg.LLM.APIKey)
	}
}

func TestEnvVarAbsentLLMAPIKeyEmpty(t *testing.T) {
	// Ensure no env var is set
	t.Setenv("LLM_API_KEY", "")

	cfg := Default()
	if err := cfg.process(t.TempDir()); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	if cfg.LLM.APIKey != "" {
		t.Errorf("expected empty APIKey when env var unset, got %q", cfg.LLM.APIKey)
	}
}

func TestDefaultProcessValidateRoundTrip(t *testing.T) {
	cfg := Default()

	if err := cfg.process(t.TempDir()); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() failed on processed default config: %v", err)
	}

	// Verify key durations were populated
	if cfg.Consolidation.Interval == 0 {
		t.Error("Consolidation.Interval should be non-zero after process()")
	}
	if cfg.Consolidation.RetentionWindow == 0 {
		t.Error("Consolidation.RetentionWindow should be non-zero after process()")
	}
	if cfg.Dreaming.Interval == 0 {
		t.Error("Dreaming.Interval should be non-zero after process()")
	}

	// Verify paths were expanded (no ~ remaining in DBPath)
	if cfg.Store.DBPath[0] == '~' {
		t.Errorf("Store.DBPath still contains ~: %q", cfg.Store.DBPath)
	}
}

func TestPartialYAMLOverridePreservesDefaults(t *testing.T) {
	// Write a minimal YAML that only overrides LLM endpoint
	yamlContent := `llm:
  endpoint: "http://custom:8080/v1"
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Overridden field should have new value
	if cfg.LLM.Endpoint != "http://custom:8080/v1" {
		t.Errorf("expected overridden endpoint, got %q", cfg.LLM.Endpoint)
	}

	// Non-overridden fields should retain defaults
	defaults := Default()

	if cfg.LLM.ChatModel != defaults.LLM.ChatModel {
		t.Errorf("ChatModel changed: expected %q, got %q", defaults.LLM.ChatModel, cfg.LLM.ChatModel)
	}
	if cfg.LLM.MaxTokens != defaults.LLM.MaxTokens {
		t.Errorf("MaxTokens changed: expected %d, got %d", defaults.LLM.MaxTokens, cfg.LLM.MaxTokens)
	}
	if cfg.Retrieval.MaxHops != defaults.Retrieval.MaxHops {
		t.Errorf("Retrieval.MaxHops changed: expected %d, got %d", defaults.Retrieval.MaxHops, cfg.Retrieval.MaxHops)
	}
	if cfg.Consolidation.DecayRate != defaults.Consolidation.DecayRate {
		t.Errorf("Consolidation.DecayRate changed: expected %f, got %f", defaults.Consolidation.DecayRate, cfg.Consolidation.DecayRate)
	}
	if cfg.Dreaming.BatchSize != defaults.Dreaming.BatchSize {
		t.Errorf("Dreaming.BatchSize changed: expected %d, got %d", defaults.Dreaming.BatchSize, cfg.Dreaming.BatchSize)
	}
}

func TestProcessExpandsAllPaths(t *testing.T) {
	cfg := Default()
	tmpDir := t.TempDir()

	if err := cfg.process(tmpDir); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	// Store.DBPath should be absolute after processing
	if !filepath.IsAbs(cfg.Store.DBPath) {
		t.Errorf("Store.DBPath should be absolute, got %q", cfg.Store.DBPath)
	}

	// All watch dirs should be absolute
	for i, dir := range cfg.Perception.Filesystem.WatchDirs {
		if !filepath.IsAbs(dir) {
			t.Errorf("WatchDirs[%d] should be absolute, got %q", i, dir)
		}
	}

	// LearnedExclusionsPath should be absolute
	if cfg.Perception.LearnedExclusionsPath != "" && !filepath.IsAbs(cfg.Perception.LearnedExclusionsPath) {
		t.Errorf("LearnedExclusionsPath should be absolute, got %q", cfg.Perception.LearnedExclusionsPath)
	}
}

func TestProcessParsesDurationFields(t *testing.T) {
	cfg := Default()
	tmpDir := t.TempDir()

	// Verify raw fields are set (they drive the parsing)
	if cfg.Consolidation.IntervalRaw == "" {
		t.Fatal("IntervalRaw should be set in defaults")
	}

	if err := cfg.process(tmpDir); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	// All duration fields that have raw counterparts should be parsed
	durations := map[string]time.Duration{
		"Consolidation.Interval":        cfg.Consolidation.Interval,
		"Consolidation.RetentionWindow": cfg.Consolidation.RetentionWindow,
		"Metacognition.Interval":        cfg.Metacognition.Interval,
		"Dreaming.Interval":             cfg.Dreaming.Interval,
	}

	for name, d := range durations {
		if d == 0 {
			t.Errorf("%s should be non-zero after process(), got 0", name)
		}
	}
}
