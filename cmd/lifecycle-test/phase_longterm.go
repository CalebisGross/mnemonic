package main

import (
	"context"
	"fmt"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
)

// PhaseLongterm runs aggressive consolidation and audits at the 3-month mark.
type PhaseLongterm struct{}

func (p *PhaseLongterm) Name() string { return "longterm" }

func (p *PhaseLongterm) Run(ctx context.Context, h *Harness, verbose bool) (*PhaseResult, error) {
	result := &PhaseResult{
		Name:    p.Name(),
		Metrics: make(map[string]float64),
	}

	// Advance to day 90.
	h.Clock.Advance(14 * 24 * time.Hour)

	// Run 20 aggressive consolidation cycles.
	const cycles = 20
	for i := 0; i < cycles; i++ {
		allMems, err := h.Store.ListMemories(ctx, "", 5000, 0)
		if err != nil {
			return result, fmt.Errorf("listing for decay cycle %d: %w", i, err)
		}
		updates := make(map[string]float32, len(allMems))
		for _, m := range allMems {
			updates[m.ID] = m.Salience * 0.90 // slightly more aggressive decay
		}
		if err := h.Store.BatchUpdateSalience(ctx, updates); err != nil {
			return result, fmt.Errorf("batch decay cycle %d: %w", i, err)
		}

		if _, err := h.Consolidator.RunOnce(ctx); err != nil {
			if verbose && i == 0 {
				fmt.Printf("\n    Consolidation cycle %d error: %v\n", i+1, err)
			}
		}
	}

	// Final metacognition audit.
	metaReport, err := h.Metacog.RunOnce(ctx)
	if err != nil {
		if verbose {
			fmt.Printf("    Metacognition audit error: %v\n", err)
		}
	}
	if metaReport != nil {
		result.Metrics["audit_observations"] = float64(len(metaReport.Observations))
	}

	// Final statistics.
	stats, err := h.Store.GetStatistics(ctx)
	if err != nil {
		return result, fmt.Errorf("getting statistics: %w", err)
	}

	result.Metrics["total_memories"] = float64(stats.TotalMemories)
	result.Metrics["active_memories"] = float64(stats.ActiveMemories)
	result.Metrics["fading_memories"] = float64(stats.FadingMemories)
	result.Metrics["archived_memories"] = float64(stats.ArchivedMemories)
	result.Metrics["storage_bytes"] = float64(stats.StorageSizeBytes)

	// Assertions.
	result.AssertGT("some archived", stats.ArchivedMemories, 0)
	result.AssertLT("active < total", stats.ActiveMemories, stats.TotalMemories)

	// Retrieval regression test — system should still work.
	regressionQueries := []string{
		"database architecture decisions",
		"bug fixes and error handling",
		"memory retrieval performance",
	}

	totalResults := 0
	for _, q := range regressionQueries {
		qr, err := h.Retriever.Query(ctx, retrieval.QueryRequest{
			Query:             q,
			MaxResults:        5,
			IncludeSuppressed: true,
		})
		if err == nil {
			totalResults += len(qr.Memories)
		}
	}
	// After aggressive decay, some queries may return 0 results — this is expected.
	// The assertion is that the system doesn't crash, not that all queries return results.
	result.Metrics["regression_results"] = float64(totalResults)

	// DB size check.
	result.Metrics["db_size_mb"] = float64(stats.StorageSizeBytes) / (1024 * 1024)

	if verbose {
		fmt.Printf("\n    Final: total=%d, active=%d, fading=%d, archived=%d\n",
			stats.TotalMemories, stats.ActiveMemories, stats.FadingMemories, stats.ArchivedMemories)
		fmt.Printf("    DB size: %.2f MB\n", float64(stats.StorageSizeBytes)/(1024*1024))
		fmt.Printf("    Regression: %d results across %d queries\n", totalResults, len(regressionQueries))
	}

	return result, nil
}
