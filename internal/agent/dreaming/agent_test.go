package dreaming

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/store"
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

	if agent.ctx == nil {
		t.Fatal("context not created")
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

// mockStore is a minimal mock implementation of the Store interface for testing.
type mockStore struct{}

func (m *mockStore) WriteRaw(ctx context.Context, raw store.RawMemory) error {
	return nil
}

func (m *mockStore) GetRaw(ctx context.Context, id string) (store.RawMemory, error) {
	return store.RawMemory{}, nil
}

func (m *mockStore) ListRawUnprocessed(ctx context.Context, limit int) ([]store.RawMemory, error) {
	return nil, nil
}

func (m *mockStore) ListRawMemoriesAfter(ctx context.Context, after time.Time, limit int) ([]store.RawMemory, error) {
	return nil, nil
}

func (m *mockStore) MarkRawProcessed(ctx context.Context, id string) error {
	return nil
}

func (m *mockStore) WriteMemory(ctx context.Context, mem store.Memory) error {
	return nil
}

func (m *mockStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) GetMemoryByRawID(ctx context.Context, rawID string) (store.Memory, error) {
	return store.Memory{}, nil
}

func (m *mockStore) UpdateMemory(ctx context.Context, mem store.Memory) error {
	return nil
}

func (m *mockStore) UpdateSalience(ctx context.Context, id string, salience float32) error {
	return nil
}

func (m *mockStore) UpdateState(ctx context.Context, id string, state string) error {
	return nil
}

func (m *mockStore) IncrementAccess(ctx context.Context, id string) error {
	return nil
}

func (m *mockStore) ListMemories(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
	return nil, nil
}

func (m *mockStore) CountMemories(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockStore) SearchByFullText(ctx context.Context, query string, limit int) ([]store.Memory, error) {
	return nil, nil
}

func (m *mockStore) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
	return nil, nil
}

func (m *mockStore) SearchByConcepts(ctx context.Context, concepts []string, limit int) ([]store.Memory, error) {
	return nil, nil
}

func (m *mockStore) CreateAssociation(ctx context.Context, assoc store.Association) error {
	return nil
}

func (m *mockStore) GetAssociations(ctx context.Context, memoryID string) ([]store.Association, error) {
	return nil, nil
}

func (m *mockStore) UpdateAssociationStrength(ctx context.Context, sourceID, targetID string, strength float32) error {
	return nil
}

func (m *mockStore) UpdateAssociationType(ctx context.Context, sourceID, targetID string, relationType string) error {
	return nil
}
func (m *mockStore) WriteRetrievalFeedback(ctx context.Context, fb store.RetrievalFeedback) error {
	return nil
}
func (m *mockStore) GetRetrievalFeedback(ctx context.Context, queryID string) (store.RetrievalFeedback, error) {
	return store.RetrievalFeedback{}, nil
}

func (m *mockStore) ActivateAssociation(ctx context.Context, sourceID, targetID string) error {
	return nil
}

func (m *mockStore) PruneWeakAssociations(ctx context.Context, strengthThreshold float32) (int, error) {
	return 0, nil
}

func (m *mockStore) BatchUpdateSalience(ctx context.Context, updates map[string]float32) error {
	return nil
}

func (m *mockStore) BatchMergeMemories(ctx context.Context, sourceIDs []string, gist store.Memory) error {
	return nil
}

func (m *mockStore) DeleteOldArchived(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}

func (m *mockStore) WriteConsolidation(ctx context.Context, record store.ConsolidationRecord) error {
	return nil
}

func (m *mockStore) GetLastConsolidation(ctx context.Context) (store.ConsolidationRecord, error) {
	return store.ConsolidationRecord{}, nil
}

func (m *mockStore) ListAllAssociations(ctx context.Context) ([]store.Association, error) {
	return nil, nil
}

func (m *mockStore) ListAllRawMemories(ctx context.Context) ([]store.RawMemory, error) {
	return nil, nil
}

func (m *mockStore) WriteMetaObservation(ctx context.Context, obs store.MetaObservation) error {
	return nil
}

func (m *mockStore) ListMetaObservations(ctx context.Context, observationType string, limit int) ([]store.MetaObservation, error) {
	return nil, nil
}

func (m *mockStore) GetDeadMemories(ctx context.Context, cutoffDate time.Time) ([]store.Memory, error) {
	return nil, nil
}

func (m *mockStore) GetSourceDistribution(ctx context.Context) (map[string]int, error) {
	return nil, nil
}

func (m *mockStore) GetStatistics(ctx context.Context) (store.StoreStatistics, error) {
	return store.StoreStatistics{}, nil
}

// --- Episode operations ---
func (m *mockStore) CreateEpisode(ctx context.Context, ep store.Episode) error { return nil }
func (m *mockStore) GetEpisode(ctx context.Context, id string) (store.Episode, error) {
	return store.Episode{}, nil
}
func (m *mockStore) UpdateEpisode(ctx context.Context, ep store.Episode) error { return nil }
func (m *mockStore) ListEpisodes(ctx context.Context, state string, limit, offset int) ([]store.Episode, error) {
	return nil, nil
}
func (m *mockStore) GetOpenEpisode(ctx context.Context) (store.Episode, error) {
	return store.Episode{}, fmt.Errorf("no open episode")
}
func (m *mockStore) CloseEpisode(ctx context.Context, id string) error { return nil }

// --- Multi-resolution operations ---
func (m *mockStore) WriteMemoryResolution(ctx context.Context, res store.MemoryResolution) error {
	return nil
}
func (m *mockStore) GetMemoryResolution(ctx context.Context, memoryID string) (store.MemoryResolution, error) {
	return store.MemoryResolution{}, nil
}

// --- Structured concept operations ---
func (m *mockStore) WriteConceptSet(ctx context.Context, cs store.ConceptSet) error { return nil }
func (m *mockStore) GetConceptSet(ctx context.Context, memoryID string) (store.ConceptSet, error) {
	return store.ConceptSet{}, nil
}
func (m *mockStore) SearchByEntity(ctx context.Context, name string, entityType string, limit int) ([]store.Memory, error) {
	return nil, nil
}

// --- Memory attribute operations ---
func (m *mockStore) WriteMemoryAttributes(ctx context.Context, attrs store.MemoryAttributes) error {
	return nil
}
func (m *mockStore) GetMemoryAttributes(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
	return store.MemoryAttributes{}, nil
}

// --- Pattern operations ---
func (m *mockStore) WritePattern(ctx context.Context, p store.Pattern) error { return nil }
func (m *mockStore) GetPattern(ctx context.Context, id string) (store.Pattern, error) {
	return store.Pattern{}, nil
}
func (m *mockStore) UpdatePattern(ctx context.Context, p store.Pattern) error { return nil }
func (m *mockStore) ListPatterns(ctx context.Context, project string, limit int) ([]store.Pattern, error) {
	return nil, nil
}
func (m *mockStore) SearchPatternsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.Pattern, error) {
	return nil, nil
}

// --- Abstraction operations ---
func (m *mockStore) WriteAbstraction(ctx context.Context, a store.Abstraction) error { return nil }
func (m *mockStore) GetAbstraction(ctx context.Context, id string) (store.Abstraction, error) {
	return store.Abstraction{}, nil
}
func (m *mockStore) UpdateAbstraction(ctx context.Context, a store.Abstraction) error { return nil }
func (m *mockStore) ListAbstractions(ctx context.Context, level int, limit int) ([]store.Abstraction, error) {
	return nil, nil
}
func (m *mockStore) SearchAbstractionsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.Abstraction, error) {
	return nil, nil
}

// --- Scoped queries ---
func (m *mockStore) SearchByProject(ctx context.Context, project string, query string, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) ListMemoriesByTimeRange(ctx context.Context, from, to time.Time, limit int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) GetProjectSummary(ctx context.Context, project string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockStore) ListProjects(ctx context.Context) ([]string, error) { return nil, nil }

func (m *mockStore) RawMemoryExistsByPath(ctx context.Context, source string, project string, filePath string) (bool, error) {
	return false, nil
}

func (m *mockStore) BatchWriteRaw(ctx context.Context, raws []store.RawMemory) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}
