package dreaming

import (
	"log/slog"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/store/storetest"
)

// TestNewDreamingAgent tests that NewDreamingAgent creates an agent with correct config.
func TestNewDreamingAgent(t *testing.T) {
	mockStore := &mockStore{}
	config := DreamingConfig{
		Interval:               3 * time.Hour,
		BatchSize:              20,
		SalienceThreshold:      0.3,
		AssociationBoostFactor: 1.15,
		NoisePruneThreshold:    0.15,
	}
	logger := slog.New(slog.NewTextHandler(nil, nil))

	agent := NewDreamingAgent(mockStore, nil, config, logger)

	if agent == nil {
		t.Fatal("NewDreamingAgent returned nil")
	}

	if agent.config.Interval != config.Interval {
		t.Fatalf("interval mismatch: expected %v, got %v", config.Interval, agent.config.Interval)
	}

	if agent.config.BatchSize != config.BatchSize {
		t.Fatalf("batch size mismatch: expected %d, got %d", config.BatchSize, agent.config.BatchSize)
	}

	if agent.config.SalienceThreshold != config.SalienceThreshold {
		t.Fatalf("salience threshold mismatch: expected %f, got %f", config.SalienceThreshold, agent.config.SalienceThreshold)
	}

	if agent.config.AssociationBoostFactor != config.AssociationBoostFactor {
		t.Fatalf("association boost factor mismatch: expected %f, got %f", config.AssociationBoostFactor, agent.config.AssociationBoostFactor)
	}

	if agent.config.NoisePruneThreshold != config.NoisePruneThreshold {
		t.Fatalf("noise prune threshold mismatch: expected %f, got %f", config.NoisePruneThreshold, agent.config.NoisePruneThreshold)
	}

	if agent.log == nil {
		t.Fatal("logger not set")
	}

	if agent.ctx != nil {
		t.Fatal("expected nil context before Start()")
	}
}

// TestCountSharedConceptsEmpty tests countSharedConcepts with empty inputs.
func TestCountSharedConceptsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected int
	}{
		{"both empty", []string{}, []string{}, 0},
		{"a empty", []string{}, []string{"a", "b"}, 0},
		{"b empty", []string{"a", "b"}, []string{}, 0},
		{"both nil", nil, nil, 0},
		{"a nil", nil, []string{"a", "b"}, 0},
		{"b nil", []string{"a", "b"}, nil, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := countSharedConcepts(tc.a, tc.b)
			if got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

// TestCountSharedConceptsNoOverlap tests countSharedConcepts with no overlap.
func TestCountSharedConceptsNoOverlap(t *testing.T) {
	a := []string{"apple", "banana"}
	b := []string{"cherry", "date"}

	result := countSharedConcepts(a, b)
	if result != 0 {
		t.Fatalf("expected 0 shared concepts, got %d", result)
	}
}

// TestCountSharedConceptsPartialOverlap tests countSharedConcepts with partial overlap.
func TestCountSharedConceptsPartialOverlap(t *testing.T) {
	a := []string{"golang", "testing", "databases"}
	b := []string{"golang", "web", "testing"}

	result := countSharedConcepts(a, b)
	if result != 2 {
		t.Fatalf("expected 2 shared concepts, got %d", result)
	}
}

// TestCountSharedConceptsFullOverlap tests countSharedConcepts with complete overlap.
func TestCountSharedConceptsFullOverlap(t *testing.T) {
	a := []string{"memory", "learning", "consolidation"}
	b := []string{"memory", "learning", "consolidation"}

	result := countSharedConcepts(a, b)
	if result != 3 {
		t.Fatalf("expected 3 shared concepts, got %d", result)
	}
}

// TestCountSharedConceptsCaseInsensitivity tests that countSharedConcepts is case-insensitive.
func TestCountSharedConceptsCaseInsensitivity(t *testing.T) {
	a := []string{"Golang", "Testing", "Memory"}
	b := []string{"golang", "TESTING", "database"}

	result := countSharedConcepts(a, b)
	if result != 2 {
		t.Fatalf("expected 2 shared concepts (case-insensitive), got %d", result)
	}
}

// TestCountSharedConceptsDuplicates tests countSharedConcepts with duplicate concepts.
func TestCountSharedConceptsDuplicates(t *testing.T) {
	a := []string{"golang", "golang", "testing"}
	b := []string{"golang", "testing", "testing"}

	result := countSharedConcepts(a, b)
	// a is deduplicated via map, but b is iterated directly,
	// so "testing" in b matches twice: count = golang(1) + testing(1) + testing(1) = 3
	if result != 3 {
		t.Fatalf("expected 3 shared concepts, got %d", result)
	}
}

// TestCountSharedConceptsSingleElement tests with single element lists.
func TestCountSharedConceptsSingleElement(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected int
	}{
		{"same single", []string{"test"}, []string{"test"}, 1},
		{"different single", []string{"test"}, []string{"other"}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := countSharedConcepts(tc.a, tc.b)
			if result != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, result)
			}
		})
	}
}

// TestCountSharedConceptsLargeLists tests with larger concept lists.
func TestCountSharedConceptsLargeLists(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e", "f", "g"}
	b := []string{"c", "d", "e", "h", "i", "j", "k"}

	result := countSharedConcepts(a, b)
	if result != 3 { // c, d, e
		t.Fatalf("expected 3 shared concepts, got %d", result)
	}
}

// TestCountSharedConceptsSpecialCharacters tests with special characters in concepts.
func TestCountSharedConceptsSpecialCharacters(t *testing.T) {
	a := []string{"Go-Lang", "Test_Suite", "ML/AI"}
	b := []string{"go-lang", "test_suite", "database"}

	result := countSharedConcepts(a, b)
	if result != 2 {
		t.Fatalf("expected 2 shared concepts, got %d", result)
	}
}

// TestCountSharedConceptsWhitespace tests with leading/trailing whitespace.
func TestCountSharedConceptsWhitespace(t *testing.T) {
	a := []string{" golang ", "testing"}
	b := []string{"golang", " testing "}

	// Note: The function does case-insensitive comparison but doesn't trim whitespace
	// So " golang " != "golang"
	result := countSharedConcepts(a, b)
	if result != 0 {
		t.Fatalf("expected 0 shared concepts (whitespace doesn't match), got %d", result)
	}
}

// TestDreamingAgentName tests that the agent returns the correct name.
func TestDreamingAgentName(t *testing.T) {
	mockStore := &mockStore{}
	config := DreamingConfig{
		Interval: 3 * time.Hour,
	}
	logger := slog.New(slog.NewTextHandler(nil, nil))

	agent := NewDreamingAgent(mockStore, nil, config, logger)

	name := agent.Name()
	if name != "dreaming-agent" {
		t.Fatalf("expected name 'dreaming-agent', got %q", name)
	}
}

// mockStore embeds the shared base mock and has no overrides.
type mockStore struct {
	storetest.MockStore
}
