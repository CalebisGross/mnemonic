package config

import (
	"testing"
	"time"
)

// TestDefaultConfigValid tests that Default() returns a valid config with all fields populated.
func TestDefaultConfigValid(t *testing.T) {
	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	// Test LLM config
	if cfg.LLM.Endpoint == "" {
		t.Fatal("LLM.Endpoint is empty")
	}
	if cfg.LLM.ChatModel == "" {
		t.Fatal("LLM.ChatModel is empty")
	}
	if cfg.LLM.EmbeddingModel == "" {
		t.Fatal("LLM.EmbeddingModel is empty")
	}
	if cfg.LLM.MaxTokens <= 0 {
		t.Fatal("LLM.MaxTokens is not positive")
	}
	if cfg.LLM.Temperature < 0 || cfg.LLM.Temperature > 2 {
		t.Fatal("LLM.Temperature is out of range")
	}
	if cfg.LLM.TimeoutSec <= 0 {
		t.Fatal("LLM.TimeoutSec is not positive")
	}

	// Test Store config
	if cfg.Store.DBPath == "" {
		t.Fatal("Store.DBPath is empty")
	}
	if cfg.Store.JournalMode == "" {
		t.Fatal("Store.JournalMode is empty")
	}

	// Test Memory config
	if cfg.Memory.MaxWorkingMemory <= 0 {
		t.Fatal("Memory.MaxWorkingMemory is not positive")
	}

	// Test Perception config
	if !cfg.Perception.Enabled {
		t.Fatal("Perception should be enabled by default")
	}
	if !cfg.Perception.Filesystem.Enabled {
		t.Fatal("Filesystem perception should be enabled by default")
	}
	if !cfg.Perception.Terminal.Enabled {
		t.Fatal("Terminal perception should be enabled by default")
	}

	// Test Encoding config
	if !cfg.Encoding.Enabled {
		t.Fatal("Encoding should be enabled by default")
	}
	if cfg.Encoding.MaxConcepts <= 0 {
		t.Fatal("Encoding.MaxConcepts is not positive")
	}

	// Test Consolidation config
	if !cfg.Consolidation.Enabled {
		t.Fatal("Consolidation should be enabled by default")
	}
	if cfg.Consolidation.Interval <= 0 {
		t.Fatal("Consolidation.Interval is not positive")
	}
	if cfg.Consolidation.RetentionWindow <= 0 {
		t.Fatal("Consolidation.RetentionWindow is not positive")
	}
	if cfg.Consolidation.DecayRate <= 0 || cfg.Consolidation.DecayRate > 1 {
		t.Fatal("Consolidation.DecayRate is out of range")
	}

	// Test Retrieval config
	if cfg.Retrieval.MaxHops <= 0 {
		t.Fatal("Retrieval.MaxHops is not positive")
	}
	if cfg.Retrieval.MaxResults <= 0 {
		t.Fatal("Retrieval.MaxResults is not positive")
	}

	// Test Metacognition config
	if !cfg.Metacognition.Enabled {
		t.Fatal("Metacognition should be enabled by default")
	}
	if cfg.Metacognition.Interval <= 0 {
		t.Fatal("Metacognition.Interval is not positive")
	}

	// Test Dreaming config
	if !cfg.Dreaming.Enabled {
		t.Fatal("Dreaming should be enabled by default")
	}
	if cfg.Dreaming.Interval <= 0 {
		t.Fatal("Dreaming.Interval is not positive")
	}
	if cfg.Dreaming.BatchSize <= 0 {
		t.Fatal("Dreaming.BatchSize is not positive")
	}
	if cfg.Dreaming.SalienceThreshold < 0 || cfg.Dreaming.SalienceThreshold > 1 {
		t.Fatal("Dreaming.SalienceThreshold is out of range")
	}

	// Test MCP config
	if !cfg.MCP.Enabled {
		t.Fatal("MCP should be enabled by default")
	}

	// Test API config
	if cfg.API.Port <= 0 || cfg.API.Port > 65535 {
		t.Fatal("API.Port is out of range")
	}

	// Test Web config
	if !cfg.Web.Enabled {
		t.Fatal("Web should be enabled by default")
	}

	// Test Logging config
	if cfg.Logging.Level == "" {
		t.Fatal("Logging.Level is empty")
	}
	if cfg.Logging.Format == "" {
		t.Fatal("Logging.Format is empty")
	}
}

// TestParseDurationString tests parsing duration strings like "6h", "24h", "90d".
func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Standard Go duration formats
		{"30s", 30 * time.Second, false},
		{"5m", 5 * time.Minute, false},
		{"1h", 1 * time.Hour, false},

		// Custom format: hours
		{"6h", 6 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"0.5h", 30 * time.Minute, false},

		// Custom format: days
		{"1d", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"90d", 90 * 24 * time.Hour, false},

		// Custom format: weeks
		{"1w", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},

		// Error cases
		{"", 0, true},
		{"invalid", 0, true},
		{"10x", 0, true},
		{"x10h", 0, true},
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

// TestDreamingConfigDefaults tests that DreamingConfig defaults are sensible.
func TestDreamingConfigDefaults(t *testing.T) {
	cfg := Default()

	dreaming := cfg.Dreaming

	if dreaming.Enabled != true {
		t.Fatal("Dreaming should be enabled by default")
	}

	if dreaming.Interval != 3*time.Hour {
		t.Fatalf("expected Interval 3h, got %v", dreaming.Interval)
	}

	if dreaming.BatchSize < 1 {
		t.Fatalf("expected positive BatchSize, got %d", dreaming.BatchSize)
	}

	if dreaming.SalienceThreshold < 0 || dreaming.SalienceThreshold > 1 {
		t.Fatalf("SalienceThreshold out of range: %f", dreaming.SalienceThreshold)
	}

	if dreaming.AssociationBoostFactor <= 0 {
		t.Fatalf("AssociationBoostFactor should be positive, got %f", dreaming.AssociationBoostFactor)
	}

	if dreaming.NoisePruneThreshold < 0 || dreaming.NoisePruneThreshold > 1 {
		t.Fatalf("NoisePruneThreshold out of range: %f", dreaming.NoisePruneThreshold)
	}
}

// TestMCPConfigDefaults tests MCPConfig defaults.
func TestMCPConfigDefaults(t *testing.T) {
	cfg := Default()

	mcp := cfg.MCP

	if !mcp.Enabled {
		t.Fatal("MCP should be enabled by default")
	}
}

// TestConsolidationConfigDefaults tests that consolidation config has sensible defaults.
func TestConsolidationConfigDefaults(t *testing.T) {
	cfg := Default()

	consolidation := cfg.Consolidation

	if consolidation.Enabled != true {
		t.Fatal("Consolidation should be enabled by default")
	}

	if consolidation.Interval == 0 {
		t.Fatal("Consolidation interval should be set")
	}

	if consolidation.RetentionWindow == 0 {
		t.Fatal("Consolidation retention window should be set")
	}

	if consolidation.DecayRate <= 0 || consolidation.DecayRate > 1 {
		t.Fatalf("DecayRate out of range: %f", consolidation.DecayRate)
	}

	if consolidation.FadeThreshold < 0 || consolidation.FadeThreshold > 1 {
		t.Fatalf("FadeThreshold out of range: %f", consolidation.FadeThreshold)
	}

	if consolidation.ArchiveThreshold < 0 || consolidation.ArchiveThreshold > 1 {
		t.Fatalf("ArchiveThreshold out of range: %f", consolidation.ArchiveThreshold)
	}

	if consolidation.MaxMemoriesPerCycle <= 0 {
		t.Fatalf("MaxMemoriesPerCycle should be positive, got %d", consolidation.MaxMemoriesPerCycle)
	}

	if consolidation.MaxMergesPerCycle <= 0 {
		t.Fatalf("MaxMergesPerCycle should be positive, got %d", consolidation.MaxMergesPerCycle)
	}

	if consolidation.MinClusterSize <= 0 {
		t.Fatalf("MinClusterSize should be positive, got %d", consolidation.MinClusterSize)
	}
}

// TestRetrievalConfigDefaults tests retrieval config defaults.
func TestRetrievalConfigDefaults(t *testing.T) {
	cfg := Default()

	retrieval := cfg.Retrieval

	if retrieval.MaxHops <= 0 {
		t.Fatalf("MaxHops should be positive, got %d", retrieval.MaxHops)
	}

	if retrieval.ActivationThreshold < 0 || retrieval.ActivationThreshold > 1 {
		t.Fatalf("ActivationThreshold out of range: %f", retrieval.ActivationThreshold)
	}

	if retrieval.DecayFactor <= 0 || retrieval.DecayFactor > 1 {
		t.Fatalf("DecayFactor out of range: %f", retrieval.DecayFactor)
	}

	if retrieval.MaxResults <= 0 {
		t.Fatalf("MaxResults should be positive, got %d", retrieval.MaxResults)
	}
}

// TestProcessDurationParsing tests that process() correctly parses all duration strings.
func TestProcessDurationParsing(t *testing.T) {
	cfg := Default()

	// Call process() to parse raw duration strings
	if err := cfg.process(t.TempDir()); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	// Verify all durations were parsed correctly
	if cfg.Consolidation.Interval == 0 {
		t.Fatal("Consolidation.Interval not parsed")
	}
	if cfg.Consolidation.RetentionWindow == 0 {
		t.Fatal("Consolidation.RetentionWindow not parsed")
	}
	if cfg.Metacognition.Interval == 0 {
		t.Fatal("Metacognition.Interval not parsed")
	}
	if cfg.Dreaming.Interval == 0 {
		t.Fatal("Dreaming.Interval not parsed")
	}
}

// TestValidateErrorsOnMissingLLMEndpoint tests validation catches missing LLM endpoint.
func TestValidateErrorsOnMissingLLMEndpoint(t *testing.T) {
	cfg := Default()
	cfg.LLM.Endpoint = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on missing LLM.Endpoint")
	}
}

// TestValidateErrorsOnInvalidTemperature tests validation catches invalid temperature.
func TestValidateErrorsOnInvalidTemperature(t *testing.T) {
	cfg := Default()
	cfg.LLM.Temperature = 2.5

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on temperature > 2")
	}

	cfg.LLM.Temperature = -0.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on negative temperature")
	}
}

// TestValidateErrorsOnInvalidPort tests validation catches invalid port.
func TestValidateErrorsOnInvalidPort(t *testing.T) {
	cfg := Default()
	cfg.API.Port = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on port <= 0")
	}

	cfg.API.Port = 65536
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on port > 65535")
	}
}

// TestValidateErrorsOnInvalidDecayRate tests validation catches invalid decay rate.
func TestValidateErrorsOnInvalidDecayRate(t *testing.T) {
	cfg := Default()
	cfg.Consolidation.DecayRate = 1.5

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on decay rate > 1")
	}

	cfg.Consolidation.DecayRate = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on decay rate <= 0")
	}
}

// TestValidateErrorsOnInvalidThresholds tests validation catches invalid thresholds.
func TestValidateErrorsOnInvalidThresholds(t *testing.T) {
	cfg := Default()

	// Test FadeThreshold
	cfg.Consolidation.FadeThreshold = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on fade threshold > 1")
	}

	// Test ArchiveThreshold
	cfg.Consolidation.ArchiveThreshold = -0.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on archive threshold < 0")
	}

	// Test ActivationThreshold
	cfg.Retrieval.ActivationThreshold = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error on activation threshold > 1")
	}
}

// TestValidateSucceedsWithDefault tests that default config passes validation.
func TestValidateSucceedsWithDefault(t *testing.T) {
	cfg := Default()

	// Process to populate parsed fields
	if err := cfg.process(t.TempDir()); err != nil {
		t.Fatalf("process() failed: %v", err)
	}

	// Validate should pass
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() failed on default config: %v", err)
	}
}

// TestMetacognitionConfigDefaults tests metacognition config defaults.
func TestMetacognitionConfigDefaults(t *testing.T) {
	cfg := Default()

	metacognition := cfg.Metacognition

	if !metacognition.Enabled {
		t.Fatal("Metacognition should be enabled by default")
	}

	if metacognition.Interval == 0 {
		t.Fatal("Metacognition interval should be set")
	}
}

// TestEncodingConfigDefaults tests encoding config defaults.
func TestEncodingConfigDefaults(t *testing.T) {
	cfg := Default()

	encoding := cfg.Encoding

	if !encoding.Enabled {
		t.Fatal("Encoding should be enabled by default")
	}

	if encoding.MaxConcepts <= 0 {
		t.Fatalf("MaxConcepts should be positive, got %d", encoding.MaxConcepts)
	}

	if encoding.FindSimilarLimit <= 0 {
		t.Fatalf("FindSimilarLimit should be positive, got %d", encoding.FindSimilarLimit)
	}
}

// TestPerceptionConfigDefaults tests perception config defaults.
func TestPerceptionConfigDefaults(t *testing.T) {
	cfg := Default()

	perception := cfg.Perception

	if !perception.Enabled {
		t.Fatal("Perception should be enabled by default")
	}

	// Filesystem
	if !perception.Filesystem.Enabled {
		t.Fatal("Filesystem perception should be enabled by default")
	}
	if len(perception.Filesystem.WatchDirs) == 0 {
		t.Fatal("Filesystem WatchDirs should be set")
	}
	if perception.Filesystem.MaxContentBytes <= 0 {
		t.Fatalf("MaxContentBytes should be positive, got %d", perception.Filesystem.MaxContentBytes)
	}

	// Terminal
	if !perception.Terminal.Enabled {
		t.Fatal("Terminal perception should be enabled by default")
	}
	if perception.Terminal.PollIntervalSec <= 0 {
		t.Fatalf("Terminal PollIntervalSec should be positive, got %d", perception.Terminal.PollIntervalSec)
	}

	// Heuristics
	if perception.Heuristics.MinContentLength < 0 {
		t.Fatalf("MinContentLength should be non-negative, got %d", perception.Heuristics.MinContentLength)
	}
	if perception.Heuristics.MaxContentLength <= 0 {
		t.Fatalf("MaxContentLength should be positive, got %d", perception.Heuristics.MaxContentLength)
	}
}
