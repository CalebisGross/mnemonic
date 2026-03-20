package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// PhaseDaily simulates 12 days of mixed usage (days 3-14).
type PhaseDaily struct{}

func (p *PhaseDaily) Name() string { return "daily" }

func (p *PhaseDaily) Run(ctx context.Context, h *Harness, verbose bool) (*PhaseResult, error) {
	result := &PhaseResult{
		Name:    p.Name(),
		Metrics: make(map[string]float64),
	}

	rng := rand.New(rand.NewSource(42)) // deterministic
	totalWritten := 0

	for day := 3; day <= 14; day++ {
		h.Clock.Advance(16 * time.Hour) // advance to next day

		// Generate 20-25 memories per day.
		count := 20 + rng.Intn(6)
		memories := generateDailyMemories(rng, h.Clock, day, count)

		for _, raw := range memories {
			if err := h.Store.WriteRaw(ctx, raw); err != nil {
				return result, fmt.Errorf("writing memory day %d: %w", day, err)
			}
		}
		totalWritten += len(memories)

		// Encode and episode after each day.
		encoded, err := h.Encoder.EncodeAllPending(ctx)
		if err != nil {
			return result, fmt.Errorf("encoding day %d: %w", day, err)
		}

		if err := h.Episoder.ProcessAllPending(ctx); err != nil {
			return result, fmt.Errorf("episoding day %d: %w", day, err)
		}

		if verbose && day%4 == 0 {
			fmt.Printf("\n    Day %d: wrote %d, encoded %d\n", day, len(memories), encoded)
		}

		// Simulate retrieval + feedback every few days.
		if day%3 == 0 {
			qr, err := h.Retriever.Query(ctx, retrieval.QueryRequest{
				Query:      "SQLite database decisions and errors",
				MaxResults: 5,
			})
			if err == nil && len(qr.Memories) > 0 {
				// Record feedback for the query.
				memIDs := make([]string, 0, len(qr.Memories))
				for _, m := range qr.Memories {
					memIDs = append(memIDs, m.Memory.ID)
				}
				_ = h.Store.WriteRetrievalFeedback(ctx, store.RetrievalFeedback{
					QueryID:      qr.QueryID,
					QueryText:    "SQLite database decisions and errors",
					RetrievedIDs: memIDs,
					Feedback:     "helpful",
				})
			}
		}

		h.Clock.Advance(8 * time.Hour) // work day
	}

	result.Metrics["total_written"] = float64(totalWritten)

	if verbose {
		fmt.Printf("    Total: %d memories written over 12 days\n", totalWritten)
	}

	// Assertions on accumulated state.
	stats, err := h.Store.GetStatistics(ctx)
	if err != nil {
		return result, fmt.Errorf("getting statistics: %w", err)
	}

	// Encoding dedup merges semantically identical content, so unique count is lower than raw count.
	result.AssertGE("total memories", stats.TotalMemories, 40)
	result.AssertGE("episodes created", stats.TotalEpisodes, 5)
	result.AssertGE("associations created", stats.TotalAssociations, 1)
	result.Metrics["total_memories"] = float64(stats.TotalMemories)
	result.Metrics["total_episodes"] = float64(stats.TotalEpisodes)
	result.Metrics["total_associations"] = float64(stats.TotalAssociations)

	// Check that feedback was recorded. Use a very old "since" time.
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	feedback, err := h.Store.ListRecentRetrievalFeedback(ctx, epoch, 10)
	if err != nil {
		return result, fmt.Errorf("listing feedback: %w", err)
	}
	result.Metrics["feedback_count"] = float64(len(feedback))
	// Feedback may be empty if retrieval returned no results on some days.
	// This is an informational metric, not a hard assertion.

	return result, nil
}
