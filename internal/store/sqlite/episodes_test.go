//go:build sqlite_fts5

package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

func TestCreateAndGetEpisode(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	ep := store.Episode{
		ID:            "ep-001",
		Title:         "Test Episode",
		StartTime:     time.Now().Add(-30 * time.Minute),
		EndTime:       time.Now(),
		DurationSec:   1800,
		RawMemoryIDs:  []string{"raw-1", "raw-2"},
		MemoryIDs:     []string{},
		Summary:       "A test episode",
		Narrative:     "This is a detailed narrative about the test episode.",
		Salience:      0.7,
		EmotionalTone: "satisfying",
		Outcome:       "success",
		State:         "open",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := s.CreateEpisode(ctx, ep)
	if err != nil {
		t.Fatalf("CreateEpisode failed: %v", err)
	}

	got, err := s.GetEpisode(ctx, "ep-001")
	if err != nil {
		t.Fatalf("GetEpisode failed: %v", err)
	}

	if got.Title != "Test Episode" {
		t.Errorf("expected title 'Test Episode', got %q", got.Title)
	}
	if got.EmotionalTone != "satisfying" {
		t.Errorf("expected tone 'satisfying', got %q", got.EmotionalTone)
	}
	if len(got.RawMemoryIDs) != 2 {
		t.Errorf("expected 2 raw memory IDs, got %d", len(got.RawMemoryIDs))
	}
}

func TestUpdateEpisode(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	ep := store.Episode{
		ID:           "ep-002",
		Title:        "Original",
		StartTime:    time.Now(),
		EndTime:      time.Now(),
		RawMemoryIDs: []string{},
		MemoryIDs:    []string{},
		State:        "open",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.CreateEpisode(ctx, ep); err != nil {
		t.Fatalf("CreateEpisode failed: %v", err)
	}

	ep.Title = "Updated Title"
	ep.Summary = "Updated summary"
	ep.State = "closed"
	err := s.UpdateEpisode(ctx, ep)
	if err != nil {
		t.Fatalf("UpdateEpisode failed: %v", err)
	}

	got, _ := s.GetEpisode(ctx, "ep-002")
	if got.Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %q", got.Title)
	}
	if got.State != "closed" {
		t.Errorf("expected state 'closed', got %q", got.State)
	}
}

func TestListEpisodes(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ep := store.Episode{
			ID:           fmt.Sprintf("ep-%d", i),
			StartTime:    time.Now().Add(time.Duration(-i) * time.Hour),
			EndTime:      time.Now(),
			RawMemoryIDs: []string{},
			MemoryIDs:    []string{},
			State:        "closed",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if i == 0 {
			ep.State = "open"
		}
		if err := s.CreateEpisode(ctx, ep); err != nil {
			t.Fatalf("CreateEpisode failed: %v", err)
		}
	}

	// List all
	all, err := s.ListEpisodes(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 episodes, got %d", len(all))
	}

	// List by state
	closed, err := s.ListEpisodes(ctx, "closed", 10, 0)
	if err != nil {
		t.Fatalf("ListEpisodes(closed) failed: %v", err)
	}
	if len(closed) != 2 {
		t.Errorf("expected 2 closed episodes, got %d", len(closed))
	}
}

func TestGetOpenEpisode(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	ep := store.Episode{
		ID:           "ep-open",
		StartTime:    time.Now(),
		EndTime:      time.Now(),
		RawMemoryIDs: []string{},
		MemoryIDs:    []string{},
		State:        "open",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.CreateEpisode(ctx, ep); err != nil {
		t.Fatalf("CreateEpisode failed: %v", err)
	}

	got, err := s.GetOpenEpisode(ctx)
	if err != nil {
		t.Fatalf("GetOpenEpisode failed: %v", err)
	}
	if got.ID != "ep-open" {
		t.Errorf("expected ep-open, got %q", got.ID)
	}
}

func TestCloseEpisode(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	ep := store.Episode{
		ID:           "ep-close",
		StartTime:    time.Now(),
		EndTime:      time.Now(),
		RawMemoryIDs: []string{},
		MemoryIDs:    []string{},
		State:        "open",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.CreateEpisode(ctx, ep); err != nil {
		t.Fatalf("CreateEpisode failed: %v", err)
	}

	err := s.CloseEpisode(ctx, "ep-close")
	if err != nil {
		t.Fatalf("CloseEpisode failed: %v", err)
	}

	got, _ := s.GetEpisode(ctx, "ep-close")
	if got.State != "closed" {
		t.Errorf("expected state 'closed', got %q", got.State)
	}
}
