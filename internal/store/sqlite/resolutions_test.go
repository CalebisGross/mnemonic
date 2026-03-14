//go:build sqlite_fts5

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

func TestWriteAndGetMemoryResolution(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	// Create prerequisite raw memory and memory
	writeRawForMemory(t, s, "raw-res-1")
	mem := store.Memory{
		ID:        "mem-res-1",
		RawID:     "raw-res-1",
		Timestamp: time.Now(),
		Content:   "test content",
		Summary:   "test summary",
		Concepts:  []string{"test"},
		Salience:  0.5,
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("WriteMemory failed: %v", err)
	}

	res := store.MemoryResolution{
		MemoryID:     "mem-res-1",
		Gist:         "Quick test gist",
		Narrative:    "This is a detailed narrative about the test memory.",
		DetailRawIDs: []string{"raw-res-1"},
		CreatedAt:    time.Now(),
	}

	err := s.WriteMemoryResolution(ctx, res)
	if err != nil {
		t.Fatalf("WriteMemoryResolution failed: %v", err)
	}

	got, err := s.GetMemoryResolution(ctx, "mem-res-1")
	if err != nil {
		t.Fatalf("GetMemoryResolution failed: %v", err)
	}

	if got.Gist != "Quick test gist" {
		t.Errorf("expected gist 'Quick test gist', got %q", got.Gist)
	}
	if got.Narrative != "This is a detailed narrative about the test memory." {
		t.Errorf("narrative mismatch")
	}
}

func TestWriteAndGetConceptSet(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	writeRawForMemory(t, s, "raw-cs-1")
	mem := store.Memory{
		ID:        "mem-cs-1",
		RawID:     "raw-cs-1",
		Timestamp: time.Now(),
		Content:   "test",
		Summary:   "test",
		Concepts:  []string{},
		Salience:  0.5,
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("WriteMemory failed: %v", err)
	}

	cs := store.ConceptSet{
		MemoryID:     "mem-cs-1",
		Topics:       []store.Topic{{Label: "Go", Path: "programming/go"}},
		Entities:     []store.Entity{{Name: "main.go", Type: "file", Context: "modified"}},
		Actions:      []store.Action{{Verb: "debugged", Object: "auth system", Details: "fixed token cache"}},
		Causality:    []store.CausalLink{{Relation: "caused_by", Description: "token TTL was wrong"}},
		Significance: "important",
		CreatedAt:    time.Now(),
	}

	err := s.WriteConceptSet(ctx, cs)
	if err != nil {
		t.Fatalf("WriteConceptSet failed: %v", err)
	}

	got, err := s.GetConceptSet(ctx, "mem-cs-1")
	if err != nil {
		t.Fatalf("GetConceptSet failed: %v", err)
	}

	if len(got.Topics) != 1 || got.Topics[0].Path != "programming/go" {
		t.Errorf("topics mismatch: %+v", got.Topics)
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "main.go" {
		t.Errorf("entities mismatch: %+v", got.Entities)
	}
	if got.Significance != "important" {
		t.Errorf("expected significance 'important', got %q", got.Significance)
	}
}

func TestWriteAndGetMemoryAttributes(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	writeRawForMemory(t, s, "raw-attr-1")
	mem := store.Memory{
		ID:        "mem-attr-1",
		RawID:     "raw-attr-1",
		Timestamp: time.Now(),
		Content:   "test",
		Summary:   "test",
		Concepts:  []string{},
		Salience:  0.5,
		State:     "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.WriteMemory(ctx, mem); err != nil {
		t.Fatalf("WriteMemory failed: %v", err)
	}

	attrs := store.MemoryAttributes{
		MemoryID:       "mem-attr-1",
		Significance:   "critical",
		EmotionalTone:  "frustrating",
		Outcome:        "failure",
		CausalityNotes: "deployment broke production",
		CreatedAt:      time.Now(),
	}

	err := s.WriteMemoryAttributes(ctx, attrs)
	if err != nil {
		t.Fatalf("WriteMemoryAttributes failed: %v", err)
	}

	got, err := s.GetMemoryAttributes(ctx, "mem-attr-1")
	if err != nil {
		t.Fatalf("GetMemoryAttributes failed: %v", err)
	}

	if got.Significance != "critical" {
		t.Errorf("expected 'critical', got %q", got.Significance)
	}
	if got.EmotionalTone != "frustrating" {
		t.Errorf("expected 'frustrating', got %q", got.EmotionalTone)
	}
	if got.CausalityNotes != "deployment broke production" {
		t.Errorf("causality notes mismatch")
	}
}
