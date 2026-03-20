//go:build sqlite_fts5

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	store "github.com/appsprout-dev/mnemonic/internal/store"
)

// Helper function to create a test store with a temporary database
func createTestStore(t *testing.T) *SQLiteStore {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath, 5000)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	return s
}

// writeRawForMemory creates a raw memory prerequisite for a memory with the given raw ID.
func writeRawForMemory(t *testing.T, s *SQLiteStore, rawID string) {
	t.Helper()
	if rawID == "" {
		rawID = "raw-" + t.Name()
	}
	raw := store.RawMemory{
		ID:              rawID,
		Timestamp:       time.Now(),
		Source:          "test",
		Type:            "test",
		Content:         "test content",
		HeuristicScore:  0.5,
		InitialSalience: 0.5,
		Processed:       false,
		CreatedAt:       time.Now(),
	}
	if err := s.WriteRaw(context.Background(), raw); err != nil {
		t.Fatalf("failed to write prerequisite raw memory: %v", err)
	}
}

// TestWriteRawReadRaw tests round-trip write and read of raw memories.
func TestWriteRawReadRaw(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	raw := store.RawMemory{
		ID:              "raw-1",
		Timestamp:       time.Now(),
		Source:          "terminal",
		Type:            "command_executed",
		Content:         "cd /tmp && ls -la",
		Metadata:        map[string]interface{}{"shell": "bash"},
		HeuristicScore:  0.75,
		InitialSalience: 0.6,
		Processed:       false,
		CreatedAt:       time.Now(),
	}

	// Write the raw memory
	if err := s.WriteRaw(ctx, raw); err != nil {
		t.Fatalf("write raw failed: %v", err)
	}

	// Read it back
	retrieved, err := s.GetRaw(ctx, raw.ID)
	if err != nil {
		t.Fatalf("get raw failed: %v", err)
	}

	if retrieved.ID != raw.ID {
		t.Fatalf("ID mismatch: expected %s, got %s", raw.ID, retrieved.ID)
	}
	if retrieved.Source != raw.Source {
		t.Fatalf("source mismatch: expected %s, got %s", raw.Source, retrieved.Source)
	}
	if retrieved.Content != raw.Content {
		t.Fatalf("content mismatch: expected %s, got %s", raw.Content, retrieved.Content)
	}
	if retrieved.HeuristicScore != raw.HeuristicScore {
		t.Fatalf("heuristic score mismatch: expected %f, got %f", raw.HeuristicScore, retrieved.HeuristicScore)
	}
}

// TestWriteMemoryReadMemory tests round-trip write and read of encoded memories.
func TestWriteMemoryReadMemory(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	mem := store.Memory{
		ID:           "mem-1",
		RawID:        "raw-1",
		Timestamp:    time.Now(),
		Content:      "compressed memory content",
		Summary:      "A test memory",
		Concepts:     []string{"test", "memory", "example"},
		Embedding:    []float32{0.1, 0.2, 0.3, 0.4},
		Salience:     0.8,
		AccessCount:  0,
		LastAccessed: time.Time{},
		State:        "active",
		GistOf:       []string{},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Create the prerequisite raw memory
	writeRawForMemory(t, s, mem.RawID)

	// Write the memory
	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("write memory failed: %v", err)
	}

	// Read it back
	retrieved, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory failed: %v", err)
	}

	if retrieved.ID != mem.ID {
		t.Fatalf("ID mismatch: expected %s, got %s", mem.ID, retrieved.ID)
	}
	if retrieved.Summary != mem.Summary {
		t.Fatalf("summary mismatch: expected %s, got %s", mem.Summary, retrieved.Summary)
	}
	if retrieved.Salience != mem.Salience {
		t.Fatalf("salience mismatch: expected %f, got %f", mem.Salience, retrieved.Salience)
	}
	if retrieved.State != mem.State {
		t.Fatalf("state mismatch: expected %s, got %s", mem.State, retrieved.State)
	}
	if len(retrieved.Concepts) != len(mem.Concepts) {
		t.Fatalf("concepts length mismatch: expected %d, got %d", len(mem.Concepts), len(retrieved.Concepts))
	}
	if len(retrieved.Embedding) != len(mem.Embedding) {
		t.Fatalf("embedding length mismatch: expected %d, got %d", len(mem.Embedding), len(retrieved.Embedding))
	}
}

// TestListMemoriesWithStateFilter tests filtering memories by state.
func TestListMemoriesWithStateFilter(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create multiple memories with different states
	memories := []store.Memory{
		{ID: "m1", RawID: "raw-m1", State: "active", Summary: "Active 1", Salience: 0.8, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m2", RawID: "raw-m2", State: "active", Summary: "Active 2", Salience: 0.7, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m3", RawID: "raw-m3", State: "fading", Summary: "Fading 1", Salience: 0.5, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m4", RawID: "raw-m4", State: "archived", Summary: "Archived 1", Salience: 0.2, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, mem := range memories {
		writeRawForMemory(t, s, mem.RawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Test listing all memories
	all, err := s.ListMemories(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("list all memories failed: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 memories, got %d", len(all))
	}

	// Test listing active memories
	active, err := s.ListMemories(ctx, "active", 10, 0)
	if err != nil {
		t.Fatalf("list active memories failed: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active memories, got %d", len(active))
	}

	// Test listing fading memories
	fading, err := s.ListMemories(ctx, "fading", 10, 0)
	if err != nil {
		t.Fatalf("list fading memories failed: %v", err)
	}
	if len(fading) != 1 {
		t.Fatalf("expected 1 fading memory, got %d", len(fading))
	}
}

// TestUpdateSalience tests updating a memory's salience.
func TestUpdateSalience(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	mem := store.Memory{
		ID:        "m1",
		RawID:     "raw-m1",
		Summary:   "Test",
		Salience:  0.5,
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	writeRawForMemory(t, s, mem.RawID)

	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("write memory failed: %v", err)
	}

	// Update salience
	newSalience := float32(0.9)
	if err := s.UpdateSalience(ctx, mem.ID, newSalience); err != nil {
		t.Fatalf("update salience failed: %v", err)
	}

	// Verify the update
	retrieved, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory failed: %v", err)
	}

	if retrieved.Salience != newSalience {
		t.Fatalf("salience mismatch: expected %f, got %f", newSalience, retrieved.Salience)
	}
}

// TestIncrementAccess tests incrementing access count and updating last_accessed.
func TestIncrementAccess(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	mem := store.Memory{
		ID:           "m1",
		RawID:        "raw-m1",
		Summary:      "Test",
		AccessCount:  0,
		LastAccessed: time.Time{},
		State:        "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	writeRawForMemory(t, s, mem.RawID)

	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("write memory failed: %v", err)
	}

	// Increment access multiple times
	for i := 0; i < 3; i++ {
		if err := s.IncrementAccess(ctx, mem.ID); err != nil {
			t.Fatalf("increment access failed: %v", err)
		}
	}

	// Verify the increment
	retrieved, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory failed: %v", err)
	}

	if retrieved.AccessCount != 3 {
		t.Fatalf("access count mismatch: expected 3, got %d", retrieved.AccessCount)
	}

	if retrieved.LastAccessed.IsZero() {
		t.Fatal("expected last_accessed to be set")
	}
}

// TestUpdateStateTransitions tests state transitions (active → fading → archived).
func TestUpdateStateTransitions(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	mem := store.Memory{
		ID:        "m1",
		RawID:     "raw-m1",
		Summary:   "Test",
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	writeRawForMemory(t, s, mem.RawID)

	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("write memory failed: %v", err)
	}

	// Transition: active → fading
	if err := s.UpdateState(ctx, mem.ID, "fading"); err != nil {
		t.Fatalf("update state to fading failed: %v", err)
	}

	retrieved, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory failed: %v", err)
	}
	if retrieved.State != "fading" {
		t.Fatalf("state mismatch: expected fading, got %s", retrieved.State)
	}

	// Transition: fading → archived
	if err := s.UpdateState(ctx, mem.ID, "archived"); err != nil {
		t.Fatalf("update state to archived failed: %v", err)
	}

	retrieved, err = s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory failed: %v", err)
	}
	if retrieved.State != "archived" {
		t.Fatalf("state mismatch: expected archived, got %s", retrieved.State)
	}
}

// TestCreateAssociation tests creating an association between two memories.
func TestCreateAssociation(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create two memories
	for i := 1; i <= 2; i++ {
		rawID := "raw-" + string(rune('0'+i))
		mem := store.Memory{
			ID:        string(rune('0' + i)),
			RawID:     rawID,
			Summary:   "Test",
			State:     "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		writeRawForMemory(t, s, rawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Create association
	assoc := store.Association{
		SourceID:        "1",
		TargetID:        "2",
		Strength:        0.7,
		RelationType:    "similar",
		CreatedAt:       time.Now(),
		LastActivated:   time.Now(),
		ActivationCount: 0,
	}

	if err := s.CreateAssociation(ctx, assoc); err != nil {
		t.Fatalf("create association failed: %v", err)
	}
}

// TestGetAssociations tests retrieving associations for a memory.
func TestGetAssociations(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create three memories
	for i := 1; i <= 3; i++ {
		rawID := "raw-" + string(rune('0'+i))
		mem := store.Memory{
			ID:        string(rune('0' + i)),
			RawID:     rawID,
			Summary:   "Test",
			State:     "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		writeRawForMemory(t, s, rawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Create associations: 1->2 and 1->3
	for targetID := 2; targetID <= 3; targetID++ {
		assoc := store.Association{
			SourceID:      "1",
			TargetID:      string(rune('0' + targetID)),
			Strength:      0.5 + float32(targetID)*0.1,
			RelationType:  "similar",
			CreatedAt:     time.Now(),
			LastActivated: time.Now(),
		}
		if err := s.CreateAssociation(ctx, assoc); err != nil {
			t.Fatalf("create association failed: %v", err)
		}
	}

	// Get associations for memory "1"
	assocs, err := s.GetAssociations(ctx, "1")
	if err != nil {
		t.Fatalf("get associations failed: %v", err)
	}

	if len(assocs) != 2 {
		t.Fatalf("expected 2 associations, got %d", len(assocs))
	}
}

// TestListAllAssociations tests retrieving all associations in the system.
func TestListAllAssociations(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create memories
	for i := 1; i <= 3; i++ {
		rawID := "raw-" + string(rune('0'+i))
		mem := store.Memory{
			ID:        string(rune('0' + i)),
			RawID:     rawID,
			Summary:   "Test",
			State:     "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		writeRawForMemory(t, s, rawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Create multiple associations
	for i := 1; i <= 2; i++ {
		for j := i + 1; j <= 3; j++ {
			assoc := store.Association{
				SourceID:      string(rune('0' + i)),
				TargetID:      string(rune('0' + j)),
				Strength:      0.5,
				RelationType:  "similar",
				CreatedAt:     time.Now(),
				LastActivated: time.Now(),
			}
			if err := s.CreateAssociation(ctx, assoc); err != nil {
				t.Fatalf("create association failed: %v", err)
			}
		}
	}

	// List all associations
	allAssocs, err := s.ListAllAssociations(ctx)
	if err != nil {
		t.Fatalf("list all associations failed: %v", err)
	}

	if len(allAssocs) != 3 {
		t.Fatalf("expected 3 associations, got %d", len(allAssocs))
	}
}

// TestGetStatistics tests that GetStatistics returns correct counts.
func TestGetStatistics(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create memories with different states
	memories := []store.Memory{
		{ID: "m1", RawID: "raw-m1", State: "active", Summary: "A1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m2", RawID: "raw-m2", State: "active", Summary: "A2", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m3", RawID: "raw-m3", State: "fading", Summary: "F1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m4", RawID: "raw-m4", State: "archived", Summary: "Ar1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, mem := range memories {
		writeRawForMemory(t, s, mem.RawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Create associations
	for i := 0; i < 2; i++ {
		assoc := store.Association{
			SourceID:      "m1",
			TargetID:      memories[i+1].ID,
			Strength:      0.5,
			RelationType:  "similar",
			CreatedAt:     time.Now(),
			LastActivated: time.Now(),
		}
		if err := s.CreateAssociation(ctx, assoc); err != nil {
			t.Fatalf("create association failed: %v", err)
		}
	}

	// Get statistics
	stats, err := s.GetStatistics(ctx)
	if err != nil {
		t.Fatalf("get statistics failed: %v", err)
	}

	if stats.TotalMemories != 4 {
		t.Fatalf("total memories mismatch: expected 4, got %d", stats.TotalMemories)
	}
	if stats.ActiveMemories != 2 {
		t.Fatalf("active memories mismatch: expected 2, got %d", stats.ActiveMemories)
	}
	if stats.FadingMemories != 1 {
		t.Fatalf("fading memories mismatch: expected 1, got %d", stats.FadingMemories)
	}
	if stats.ArchivedMemories != 1 {
		t.Fatalf("archived memories mismatch: expected 1, got %d", stats.ArchivedMemories)
	}
	if stats.TotalAssociations != 2 {
		t.Fatalf("total associations mismatch: expected 2, got %d", stats.TotalAssociations)
	}
}

// TestWriteMetaObservation tests storing a meta-observation.
func TestWriteMetaObservation(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	obs := store.MetaObservation{
		ID:              "obs-1",
		ObservationType: "quality_audit",
		Severity:        "warning",
		Details: map[string]interface{}{
			"score": 0.72,
			"issue": "Low quality memories detected",
		},
		CreatedAt: time.Now(),
	}

	if err := s.WriteMetaObservation(ctx, obs); err != nil {
		t.Fatalf("write meta observation failed: %v", err)
	}
}

// TestListMetaObservations tests retrieving meta-observations.
func TestListMetaObservations(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Write multiple observations of different types
	observations := []store.MetaObservation{
		{
			ID:              "obs-1",
			ObservationType: "quality_audit",
			Severity:        "warning",
			Details:         map[string]interface{}{"issue": "test1"},
			CreatedAt:       time.Now(),
		},
		{
			ID:              "obs-2",
			ObservationType: "quality_audit",
			Severity:        "info",
			Details:         map[string]interface{}{"issue": "test2"},
			CreatedAt:       time.Now(),
		},
		{
			ID:              "obs-3",
			ObservationType: "source_balance",
			Severity:        "critical",
			Details:         map[string]interface{}{"issue": "test3"},
			CreatedAt:       time.Now(),
		},
	}

	for _, obs := range observations {
		if err := s.WriteMetaObservation(ctx, obs); err != nil {
			t.Fatalf("write meta observation failed: %v", err)
		}
	}

	// List all observations
	allObs, err := s.ListMetaObservations(ctx, "", 10)
	if err != nil {
		t.Fatalf("list all observations failed: %v", err)
	}
	if len(allObs) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(allObs))
	}

	// List observations by type
	qualityObs, err := s.ListMetaObservations(ctx, "quality_audit", 10)
	if err != nil {
		t.Fatalf("list quality_audit observations failed: %v", err)
	}
	if len(qualityObs) != 2 {
		t.Fatalf("expected 2 quality_audit observations, got %d", len(qualityObs))
	}
}

// TestCountMemories tests the CountMemories function.
func TestCountMemories(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Count empty store
	count, err := s.CountMemories(ctx)
	if err != nil {
		t.Fatalf("count memories failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 memories, got %d", count)
	}

	// Add memories
	for i := 1; i <= 5; i++ {
		rawID := "raw-" + string(rune('0'+i))
		mem := store.Memory{
			ID:        string(rune('0' + i)),
			RawID:     rawID,
			Summary:   "Test",
			State:     "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		writeRawForMemory(t, s, rawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Count again
	count, err = s.CountMemories(ctx)
	if err != nil {
		t.Fatalf("count memories failed: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 memories, got %d", count)
	}
}

// TestSearchByConcepts tests basic concept-based search functionality.
func TestSearchByConcepts(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create memories with different concepts
	memories := []store.Memory{
		{ID: "m1", RawID: "raw-m1", Summary: "Test 1", Concepts: []string{"golang", "testing"}, State: "active", Salience: 0.8, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m2", RawID: "raw-m2", Summary: "Test 2", Concepts: []string{"golang", "web"}, State: "active", Salience: 0.7, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "m3", RawID: "raw-m3", Summary: "Test 3", Concepts: []string{"python", "testing"}, State: "active", Salience: 0.6, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, mem := range memories {
		writeRawForMemory(t, s, mem.RawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Search for memories with "golang" concept
	results, err := s.SearchByConcepts(ctx, []string{"golang"}, 10)
	if err != nil {
		t.Fatalf("search by concepts failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results with golang concept, got %d", len(results))
	}

	// Search for memories with "testing" concept
	results, err = s.SearchByConcepts(ctx, []string{"testing"}, 10)
	if err != nil {
		t.Fatalf("search by concepts failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results with testing concept, got %d", len(results))
	}
}

// TestGetDeadMemories tests retrieving memories that haven't been accessed since a cutoff date.
func TestGetDeadMemories(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-60 * 24 * time.Hour) // 60 days ago

	// Create memories with different last_accessed times
	memories := []store.Memory{
		{ID: "m1", RawID: "raw-m1", Summary: "Recent", State: "active", LastAccessed: now.Add(-1 * time.Hour), CreatedAt: oldTime, UpdatedAt: oldTime},
		{ID: "m2", RawID: "raw-m2", Summary: "Dead 1", State: "active", LastAccessed: oldTime, CreatedAt: oldTime, UpdatedAt: oldTime},
		{ID: "m3", RawID: "raw-m3", Summary: "Dead 2", State: "active", LastAccessed: time.Time{}, CreatedAt: oldTime, UpdatedAt: oldTime},
	}

	for _, mem := range memories {
		writeRawForMemory(t, s, mem.RawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Get dead memories from 30 days ago
	deadMemories, err := s.GetDeadMemories(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("get dead memories failed: %v", err)
	}

	if len(deadMemories) != 2 {
		t.Fatalf("expected 2 dead memories, got %d", len(deadMemories))
	}
}

// TestUpdateMemory tests updating an existing memory.
func TestUpdateMemory(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	mem := store.Memory{
		ID:        "m1",
		RawID:     "raw-m1",
		Summary:   "Original",
		Content:   "Original content",
		State:     "active",
		Salience:  0.5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	writeRawForMemory(t, s, mem.RawID)

	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("write memory failed: %v", err)
	}

	// Update the memory
	mem.Summary = "Updated"
	mem.Content = "Updated content"
	mem.Salience = 0.9
	mem.UpdatedAt = time.Now()

	if err := s.UpdateMemory(ctx, mem); err != nil {
		t.Fatalf("update memory failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory failed: %v", err)
	}

	if retrieved.Summary != "Updated" {
		t.Fatalf("summary mismatch: expected Updated, got %s", retrieved.Summary)
	}
	if retrieved.Content != "Updated content" {
		t.Fatalf("content mismatch: expected Updated content, got %s", retrieved.Content)
	}
	if retrieved.Salience != 0.9 {
		t.Fatalf("salience mismatch: expected 0.9, got %f", retrieved.Salience)
	}
}

// TestPruneWeakAssociations tests pruning associations below a strength threshold.
func TestPruneWeakAssociations(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Create memories
	for i := 1; i <= 3; i++ {
		rawID := "raw-" + string(rune('0'+i))
		mem := store.Memory{
			ID:        string(rune('0' + i)),
			RawID:     rawID,
			Summary:   "Test",
			State:     "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		writeRawForMemory(t, s, rawID)
		if err := s.WriteMemory(ctx, mem); err != nil {
			t.Fatalf("write memory failed: %v", err)
		}
	}

	// Create associations with different strengths
	assocs := []store.Association{
		{SourceID: "1", TargetID: "2", Strength: 0.9, RelationType: "similar", CreatedAt: time.Now(), LastActivated: time.Now()},
		{SourceID: "1", TargetID: "3", Strength: 0.2, RelationType: "similar", CreatedAt: time.Now(), LastActivated: time.Now()},
		{SourceID: "2", TargetID: "3", Strength: 0.3, RelationType: "similar", CreatedAt: time.Now(), LastActivated: time.Now()},
	}

	for _, assoc := range assocs {
		if err := s.CreateAssociation(ctx, assoc); err != nil {
			t.Fatalf("create association failed: %v", err)
		}
	}

	// Prune associations with strength < 0.5
	pruned, err := s.PruneWeakAssociations(ctx, 0.5)
	if err != nil {
		t.Fatalf("prune weak associations failed: %v", err)
	}

	if pruned != 2 {
		t.Fatalf("expected 2 associations pruned, got %d", pruned)
	}

	// Verify remaining associations
	remaining, err := s.ListAllAssociations(ctx)
	if err != nil {
		t.Fatalf("list all associations failed: %v", err)
	}

	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining association, got %d", len(remaining))
	}
}

// TestListRawUnprocessed tests retrieving unprocessed raw memories.
func TestListRawUnprocessed(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Write raw memories with different processed states
	raws := []store.RawMemory{
		{
			ID:              "r1",
			Timestamp:       time.Now(),
			Source:          "test",
			Content:         "test 1",
			Processed:       false,
			CreatedAt:       time.Now(),
			HeuristicScore:  0.5,
			InitialSalience: 0.6,
		},
		{
			ID:              "r2",
			Timestamp:       time.Now(),
			Source:          "test",
			Content:         "test 2",
			Processed:       true,
			CreatedAt:       time.Now(),
			HeuristicScore:  0.5,
			InitialSalience: 0.6,
		},
		{
			ID:              "r3",
			Timestamp:       time.Now(),
			Source:          "test",
			Content:         "test 3",
			Processed:       false,
			CreatedAt:       time.Now(),
			HeuristicScore:  0.5,
			InitialSalience: 0.6,
		},
	}

	for _, raw := range raws {
		if err := s.WriteRaw(ctx, raw); err != nil {
			t.Fatalf("write raw failed: %v", err)
		}
	}

	// List unprocessed
	unprocessed, err := s.ListRawUnprocessed(ctx, 10)
	if err != nil {
		t.Fatalf("list unprocessed failed: %v", err)
	}

	if len(unprocessed) != 2 {
		t.Fatalf("expected 2 unprocessed memories, got %d", len(unprocessed))
	}
}

// TestMarkRawProcessed tests marking a raw memory as processed.
func TestMarkRawProcessed(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	raw := store.RawMemory{
		ID:              "r1",
		Timestamp:       time.Now(),
		Source:          "test",
		Content:         "test",
		Processed:       false,
		CreatedAt:       time.Now(),
		HeuristicScore:  0.5,
		InitialSalience: 0.6,
	}

	if err := s.WriteRaw(ctx, raw); err != nil {
		t.Fatalf("write raw failed: %v", err)
	}

	// Mark as processed
	if err := s.MarkRawProcessed(ctx, raw.ID); err != nil {
		t.Fatalf("mark raw processed failed: %v", err)
	}

	// Verify
	retrieved, err := s.GetRaw(ctx, raw.ID)
	if err != nil {
		t.Fatalf("get raw failed: %v", err)
	}

	if !retrieved.Processed {
		t.Fatal("expected raw memory to be marked processed")
	}
}

// TestClaimRawForEncoding tests the atomic claim-or-skip behavior.
func TestClaimRawForEncoding(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	raw := store.RawMemory{
		ID:              "claim-test-1",
		Timestamp:       time.Now(),
		Source:          "test",
		Content:         "test content",
		Processed:       false,
		CreatedAt:       time.Now(),
		HeuristicScore:  0.5,
		InitialSalience: 0.6,
	}

	if err := s.WriteRaw(ctx, raw); err != nil {
		t.Fatalf("write raw failed: %v", err)
	}

	// First claim should succeed
	if err := s.ClaimRawForEncoding(ctx, raw.ID); err != nil {
		t.Fatalf("first claim should succeed: %v", err)
	}

	// Second claim should return ErrAlreadyClaimed
	err := s.ClaimRawForEncoding(ctx, raw.ID)
	if err == nil {
		t.Fatal("second claim should fail")
	}
	if !errors.Is(err, store.ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed, got: %v", err)
	}

	// Verify raw is marked processed
	retrieved, err := s.GetRaw(ctx, raw.ID)
	if err != nil {
		t.Fatalf("get raw failed: %v", err)
	}
	if !retrieved.Processed {
		t.Fatal("expected raw memory to be marked processed after claim")
	}
}

// TestWriteMemoryDuplicateRawID tests the UNIQUE constraint on raw_id.
func TestWriteMemoryDuplicateRawID(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	mem1 := store.Memory{
		ID:        "m1",
		RawID:     "raw-1",
		Timestamp: time.Now(),
		Content:   "first encoding",
		Summary:   "first",
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	mem2 := store.Memory{
		ID:        "m2",
		RawID:     "raw-1", // same raw_id
		Timestamp: time.Now(),
		Content:   "duplicate encoding",
		Summary:   "duplicate",
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.WriteMemory(ctx, mem1); err != nil {
		t.Fatalf("first write should succeed: %v", err)
	}

	err := s.WriteMemory(ctx, mem2)
	if err == nil {
		t.Fatal("second write with same raw_id should fail")
	}
	if !errors.Is(err, store.ErrDuplicateRawID) {
		t.Fatalf("expected ErrDuplicateRawID, got: %v", err)
	}
}

func TestSanitizeFTSQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"single word", "mnemonic", "mnemonic*"},
		{"multiple words", "memory search", "memory* OR search*"},
		{"stop words filtered", "the memory is good", "memory* OR good*"},
		{"short words filtered", "a x memory", "memory*"},
		{"colon in middle stripped", "lm:studio", "lmstudio*"},
		{"colon at edge stripped", "lm:", "lm*"},
		{"hyphen stripped", "felix-lm", "felixlm*"},
		{"parentheses stripped", "(memory)", "memory*"},
		{"mixed punctuation", "hello! world? foo:bar", "hello* OR world* OR foobar*"},
		{"all stop words", "the a an and or", ""},
		{"all short words", "a x i", ""},
		{"preserves digits", "v3 gpt4", "v3* OR gpt4*"},
		{"FTS operators neutralized", "NOT NEAR memory", "not* OR near* OR memory*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFTSQuery(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRetrievalFeedbackAccessSnapshot(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	t.Run("round-trip with access snapshot", func(t *testing.T) {
		fb := store.RetrievalFeedback{
			QueryID:      "q-snap-1",
			QueryText:    "test query",
			RetrievedIDs: []string{"mem-1", "mem-2", "mem-3"},
			TraversedAssocs: []store.TraversedAssoc{
				{SourceID: "mem-1", TargetID: "mem-2"},
			},
			AccessSnapshot: []store.AccessSnapshotEntry{
				{MemoryID: "mem-1", Rank: 1, Score: 0.95},
				{MemoryID: "mem-2", Rank: 2, Score: 0.72},
				{MemoryID: "mem-3", Rank: 3, Score: 0.41},
			},
			CreatedAt: time.Now(),
		}

		if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
			t.Fatalf("WriteRetrievalFeedback: %v", err)
		}

		got, err := s.GetRetrievalFeedback(ctx, "q-snap-1")
		if err != nil {
			t.Fatalf("GetRetrievalFeedback: %v", err)
		}

		if len(got.AccessSnapshot) != 3 {
			t.Fatalf("expected 3 snapshot entries, got %d", len(got.AccessSnapshot))
		}
		if got.AccessSnapshot[0].MemoryID != "mem-1" || got.AccessSnapshot[0].Rank != 1 {
			t.Errorf("snapshot[0] = %+v, want mem-1 rank 1", got.AccessSnapshot[0])
		}
		if got.AccessSnapshot[1].Score != 0.72 {
			t.Errorf("snapshot[1].Score = %f, want 0.72", got.AccessSnapshot[1].Score)
		}
		if got.AccessSnapshot[2].Rank != 3 {
			t.Errorf("snapshot[2].Rank = %d, want 3", got.AccessSnapshot[2].Rank)
		}
	})

	t.Run("backward compat with nil snapshot", func(t *testing.T) {
		fb := store.RetrievalFeedback{
			QueryID:      "q-old",
			QueryText:    "old query",
			RetrievedIDs: []string{"mem-3"},
			CreatedAt:    time.Now(),
		}

		if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
			t.Fatalf("WriteRetrievalFeedback: %v", err)
		}

		got, err := s.GetRetrievalFeedback(ctx, "q-old")
		if err != nil {
			t.Fatalf("GetRetrievalFeedback: %v", err)
		}

		if got.AccessSnapshot != nil && len(got.AccessSnapshot) != 0 {
			t.Errorf("expected nil or empty snapshot for old record, got %v", got.AccessSnapshot)
		}
	})

	t.Run("update preserves snapshot", func(t *testing.T) {
		// Write initial record with snapshot
		fb := store.RetrievalFeedback{
			QueryID:      "q-update",
			QueryText:    "update test",
			RetrievedIDs: []string{"mem-4"},
			AccessSnapshot: []store.AccessSnapshotEntry{
				{MemoryID: "mem-4", Rank: 1, Score: 0.88},
			},
			CreatedAt: time.Now(),
		}
		if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
			t.Fatalf("WriteRetrievalFeedback: %v", err)
		}

		// Simulate feedback update (re-write with quality set)
		fb.Feedback = "helpful"
		if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
			t.Fatalf("WriteRetrievalFeedback (update): %v", err)
		}

		got, err := s.GetRetrievalFeedback(ctx, "q-update")
		if err != nil {
			t.Fatalf("GetRetrievalFeedback: %v", err)
		}
		if got.Feedback != "helpful" {
			t.Errorf("expected feedback 'helpful', got %q", got.Feedback)
		}
		if len(got.AccessSnapshot) != 1 || got.AccessSnapshot[0].Score != 0.88 {
			t.Errorf("snapshot not preserved after update: %+v", got.AccessSnapshot)
		}
	})
}
