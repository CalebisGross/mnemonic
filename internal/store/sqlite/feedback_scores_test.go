//go:build sqlite_fts5

package sqlite

import (
	"context"
	"testing"
	"time"

	store "github.com/appsprout-dev/mnemonic/internal/store"
)

func TestGetMemoryFeedbackScores(t *testing.T) {
	s := createTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Insert some feedback records
	now := time.Now()
	feedbacks := []store.RetrievalFeedback{
		{
			QueryID:      "q1",
			QueryText:    "test query 1",
			RetrievedIDs: []string{"m1", "m2", "m3"},
			Feedback:     "helpful",
			CreatedAt:    now,
		},
		{
			QueryID:      "q2",
			QueryText:    "test query 2",
			RetrievedIDs: []string{"m1", "m3"},
			Feedback:     "irrelevant",
			CreatedAt:    now,
		},
		{
			QueryID:      "q3",
			QueryText:    "test query 3",
			RetrievedIDs: []string{"m2"},
			Feedback:     "partial",
			CreatedAt:    now,
		},
		{
			QueryID:      "q4",
			QueryText:    "test query 4 (no rating)",
			RetrievedIDs: []string{"m1", "m2"},
			Feedback:     "",
			CreatedAt:    now,
		},
	}
	for _, fb := range feedbacks {
		if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
			t.Fatalf("failed to write feedback: %v", err)
		}
	}

	t.Run("scores computed correctly", func(t *testing.T) {
		scores, err := s.GetMemoryFeedbackScores(ctx, []string{"m1", "m2", "m3", "m4"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// m1: helpful(+1) + irrelevant(-1) = 0/2 = 0.0
		if score, ok := scores["m1"]; !ok || abs32(score-0.0) > 0.001 {
			t.Errorf("m1 score: want ~0.0, got %v (present=%v)", score, ok)
		}

		// m2: helpful(+1) + partial(0) = 1/2 = 0.5
		if score, ok := scores["m2"]; !ok || abs32(score-0.5) > 0.001 {
			t.Errorf("m2 score: want ~0.5, got %v (present=%v)", score, ok)
		}

		// m3: helpful(+1) + irrelevant(-1) = 0/2 = 0.0
		if score, ok := scores["m3"]; !ok || abs32(score-0.0) > 0.001 {
			t.Errorf("m3 score: want ~0.0, got %v (present=%v)", score, ok)
		}

		// m4: no feedback records
		if _, ok := scores["m4"]; ok {
			t.Errorf("m4 should not have a score")
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		scores, err := s.GetMemoryFeedbackScores(ctx, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if scores != nil {
			t.Errorf("expected nil for empty input, got %v", scores)
		}
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		scores, err := s.GetMemoryFeedbackScores(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if scores != nil {
			t.Errorf("expected nil for nil input, got %v", scores)
		}
	})

	t.Run("consistently helpful memory gets score 1.0", func(t *testing.T) {
		// Write additional feedback where m5 is always helpful
		for i := 0; i < 3; i++ {
			fb := store.RetrievalFeedback{
				QueryID:      "q_helpful_" + string(rune('a'+i)),
				QueryText:    "helpful query",
				RetrievedIDs: []string{"m5"},
				Feedback:     "helpful",
				CreatedAt:    now,
			}
			if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
				t.Fatalf("failed to write feedback: %v", err)
			}
		}
		scores, err := s.GetMemoryFeedbackScores(ctx, []string{"m5"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if score, ok := scores["m5"]; !ok || abs32(score-1.0) > 0.001 {
			t.Errorf("m5 score: want 1.0, got %v", score)
		}
	})
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
