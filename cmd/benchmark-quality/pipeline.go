package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/appsprout-dev/mnemonic/internal/agent/abstraction"
	"github.com/appsprout-dev/mnemonic/internal/agent/consolidation"
	"github.com/appsprout-dev/mnemonic/internal/agent/dreaming"
	"github.com/appsprout-dev/mnemonic/internal/agent/encoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/episoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store/sqlite"
)

// runPipelineScenario executes a full pipeline scenario:
// raw events -> encoding -> episoding -> dreaming -> consolidation -> retrieval.
func runPipelineScenario(
	ctx context.Context,
	sc pipelineScenario,
	cfg benchConfig,
	cycles int,
	verbose bool,
	log *slog.Logger,
) (pipelineResult, error) {
	result := pipelineResult{Name: sc.Name}

	// Create isolated temp DB.
	tmpDir, err := os.MkdirTemp("", "mnemonic-pipeline-*")
	if err != nil {
		return result, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "pipeline.db")
	s, err := sqlite.NewSQLiteStore(dbPath, 5000)
	if err != nil {
		return result, fmt.Errorf("creating store: %w", err)
	}
	defer func() { _ = s.Close() }()

	var p llm.Provider = &semanticStubProvider{}
	if cfg.Provider != nil {
		p = cfg.Provider
	}

	// Create all agents with configs from benchConfig.
	encAgent := encoding.NewEncodingAgentWithConfig(s, p, log, cfg.Encoding)
	epiAgent := episoding.NewEpisodingAgent(s, p, log, cfg.Episoding)
	dreamAgent := dreaming.NewDreamingAgent(s, p, cfg.Dreaming, log)
	consolAgent := consolidation.NewConsolidationAgent(s, p, cfg.Consolidation, log)
	absAgent := abstraction.NewAbstractionAgent(s, p, cfg.Abstraction, log)
	retAgent := retrieval.NewRetrievalAgent(s, p, cfg.Retrieval, log)

	// Phase 1: Inject raw events.
	for _, raw := range sc.RawEvents {
		if err := s.WriteRaw(ctx, raw); err != nil {
			return result, fmt.Errorf("writing raw event %s: %w", raw.ID, err)
		}
	}

	if verbose {
		fmt.Printf("    Injected %d raw events\n", len(sc.RawEvents))
	}

	// Phase 2: Encoding — process all raw events into encoded memories.
	encoded, err := encAgent.EncodeAllPending(ctx)
	if err != nil {
		return result, fmt.Errorf("encoding: %w", err)
	}
	result.EncodedCount = encoded

	if verbose {
		fmt.Printf("    Encoded %d memories\n", encoded)
	}

	// Phase 3: Episoding — group into episodes.
	if err := epiAgent.ProcessAllPending(ctx); err != nil {
		return result, fmt.Errorf("episoding: %w", err)
	}

	episodes, err := s.ListEpisodes(ctx, "", 200, 0)
	if err != nil {
		return result, fmt.Errorf("listing episodes: %w", err)
	}
	result.EpisodeCount = len(episodes)

	if verbose {
		fmt.Printf("    Created %d episodes\n", len(episodes))
	}

	// Phase 4: Dreaming — strengthen associations, cross-pollinate, generate insights.
	if _, err := dreamAgent.RunOnce(ctx); err != nil {
		log.Warn("dreaming cycle error", "error", err)
	}

	// Phase 5: Consolidation cycles with salience decay.
	for i := 0; i < cycles; i++ {
		allMems, err := s.ListMemories(ctx, "", 500, 0)
		if err != nil {
			return result, fmt.Errorf("listing memories for decay: %w", err)
		}
		updates := make(map[string]float32, len(allMems))
		for _, m := range allMems {
			updates[m.ID] = m.Salience * cfg.BenchDecay
		}
		if err := s.BatchUpdateSalience(ctx, updates); err != nil {
			return result, fmt.Errorf("batch update salience: %w", err)
		}

		rpt, err := consolAgent.RunOnce(ctx)
		if err != nil {
			log.Warn("consolidation cycle error", "cycle", i+1, "error", err)
		} else if verbose && rpt != nil {
			fmt.Printf("    Cycle %d: decayed=%d, fading=%d, archived=%d\n",
				i+1, rpt.MemoriesDecayed, rpt.TransitionedFading, rpt.TransitionedArchived)
		}
	}

	// Phase 5b: Abstraction — synthesize patterns into principles.
	if _, err := absAgent.RunOnce(ctx); err != nil {
		log.Warn("abstraction cycle error", "error", err)
	}

	// Phase 6: Score signal survival and noise suppression.
	result.SignalSurvival, result.NoiseSuppression = scorePipelineSurvival(ctx, s, sc)

	// Phase 6b: Score associations per signal memory.
	result.AvgAssociations = scorePipelineAssociations(ctx, s, sc)

	// Phase 7: Retrieval queries.
	for _, q := range sc.Queries {
		resp, err := retAgent.Query(ctx, retrieval.QueryRequest{
			Query:      q.Query,
			MaxResults: 5,
		})
		if err != nil {
			return result, fmt.Errorf("query %q: %w", q.Query, err)
		}
		qr := scorePipelineQuery(q, resp)
		result.QueryResults = append(result.QueryResults, qr)
	}

	return result, nil
}

// scorePipelineSurvival computes signal survival and noise suppression rates.
// Signal: raw events in SignalIDs that produced memories still active or fading.
// Noise: raw events NOT in SignalIDs that produced memories that are fading/archived/merged.
func scorePipelineSurvival(ctx context.Context, s *sqlite.SQLiteStore, sc pipelineScenario) (signalSurvival, noiseSuppression float64) {
	var signalTotal, signalSurvived int
	var noiseTotal, noiseSuppressed int

	for _, raw := range sc.RawEvents {
		isSignal := sc.SignalIDs[raw.ID]
		mem, err := s.GetMemoryByRawID(ctx, raw.ID)
		if err != nil {
			// No encoded memory found — noise was never encoded (good suppression)
			// or signal was lost (bad).
			if isSignal {
				signalTotal++
			} else {
				noiseTotal++
				noiseSuppressed++ // never encoded = suppressed
			}
			continue
		}

		if isSignal {
			signalTotal++
			if mem.State == "active" || mem.State == "fading" {
				signalSurvived++
			}
		} else {
			noiseTotal++
			if mem.State == "fading" || mem.State == "archived" || mem.State == "merged" {
				noiseSuppressed++
			}
		}
	}

	if signalTotal > 0 {
		signalSurvival = float64(signalSurvived) / float64(signalTotal)
	}
	if noiseTotal > 0 {
		noiseSuppression = float64(noiseSuppressed) / float64(noiseTotal)
	}
	return
}

// scorePipelineAssociations computes the average number of associations per signal memory.
func scorePipelineAssociations(ctx context.Context, s *sqlite.SQLiteStore, sc pipelineScenario) float64 {
	var totalAssocs int
	var signalMems int

	for rawID := range sc.SignalIDs {
		mem, err := s.GetMemoryByRawID(ctx, rawID)
		if err != nil {
			continue
		}
		assocs, err := s.GetAssociations(ctx, mem.ID)
		if err != nil {
			continue
		}
		totalAssocs += len(assocs)
		signalMems++
	}

	if signalMems == 0 {
		return 0
	}
	return float64(totalAssocs) / float64(signalMems)
}

// scorePipelineQuery scores a pipeline query by checking if expected concepts
// appear in the returned memories' concepts.
func scorePipelineQuery(q pipelineQuery, resp retrieval.QueryResponse) queryResult {
	qr := queryResult{Query: q.Query}
	k := 5
	if len(resp.Memories) < k {
		k = len(resp.Memories)
	}
	if k == 0 {
		return qr
	}

	// Count how many of the expected concepts appear in the top-k results.
	expectedSet := make(map[string]bool, len(q.ExpectedConcepts))
	for _, c := range q.ExpectedConcepts {
		expectedSet[strings.ToLower(c)] = true
	}

	conceptHits := 0
	firstHitRank := 0

	for i := 0; i < k; i++ {
		mem := resp.Memories[i].Memory
		hit := false
		for _, c := range mem.Concepts {
			if expectedSet[strings.ToLower(c)] {
				hit = true
				break
			}
		}
		if hit {
			conceptHits++
			if firstHitRank == 0 {
				firstHitRank = i + 1
			}
		}
	}

	totalExpected := len(q.ExpectedConcepts)
	if totalExpected == 0 {
		totalExpected = 1
	}

	qr.PrecisionAtK = float64(conceptHits) / float64(k)
	qr.RecallAtK = float64(conceptHits) / float64(totalExpected)
	if firstHitRank > 0 {
		qr.MRR = 1.0 / float64(firstHitRank)
	}
	// Simplified nDCG: treat concept hits as binary relevance.
	dcg := 0.0
	for i := 0; i < k; i++ {
		mem := resp.Memories[i].Memory
		for _, c := range mem.Concepts {
			if expectedSet[strings.ToLower(c)] {
				dcg += 1.0 / math.Log2(float64(i+2))
				break
			}
		}
	}
	idealDCG := 0.0
	idealK := conceptHits
	if idealK > k {
		idealK = k
	}
	for i := 0; i < idealK; i++ {
		idealDCG += 1.0 / math.Log2(float64(i+2))
	}
	if idealDCG > 0 {
		qr.NDCG = dcg / idealDCG
	}

	return qr
}
