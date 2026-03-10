package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/appsprout/mnemonic/internal/agent/consolidation"
	"github.com/appsprout/mnemonic/internal/agent/retrieval"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/appsprout/mnemonic/internal/store/sqlite"
)

var Version = "dev"

func main() {
	var (
		verbose bool
		cycles  int
		report  string
		llmMode bool
	)

	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.IntVar(&cycles, "cycles", 5, "number of consolidation cycles")
	flag.StringVar(&report, "report", "", "output format: 'markdown' writes benchmark-results.md")
	flag.BoolVar(&llmMode, "llm", false, "use real LLM for embeddings (requires LM Studio)")
	flag.Parse()

	if llmMode {
		fmt.Fprintln(os.Stderr, "Error: --llm mode not yet implemented")
		os.Exit(1)
	}

	logLevel := slog.LevelError
	if verbose {
		logLevel = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	// Create temp directory for the benchmark DB.
	tmpDir, err := os.MkdirTemp("", "mnemonic-bench-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "bench.db")
	s, err := sqlite.NewSQLiteStore(dbPath, 5000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	// Create agents with default configs and stub LLM.
	stub := &stubProvider{}
	retAgent := retrieval.NewRetrievalAgent(s, stub, retrieval.DefaultConfig(), log)
	consolAgent := consolidation.NewConsolidationAgent(s, stub, consolidation.DefaultConfig(), log)

	ctx := context.Background()
	scenarios := allScenarios()

	fmt.Println()
	fmt.Println("  Mnemonic Memory Quality Benchmark")
	fmt.Printf("  Version: %s  |  LLM: synthetic  |  Cycles: %d\n", Version, cycles)
	fmt.Println()

	var allResults []scenarioResult

	for _, sc := range scenarios {
		if verbose {
			fmt.Printf("  Running: %s\n", sc.Name)
		}

		result, runErr := runScenario(ctx, s, retAgent, consolAgent, sc, cycles, verbose, log)
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "  Error in scenario %q: %v\n", sc.Name, runErr)
			os.Exit(1)
		}

		allResults = append(allResults, result)
		printScenarioResult(result)
	}

	// Aggregate and print final results.
	agg := aggregateResults(allResults)
	printAggregate(agg)

	if report == "markdown" {
		if writeErr := writeMarkdownReport(allResults, agg, cycles); writeErr != nil {
			fmt.Fprintf(os.Stderr, "Error writing report: %v\n", writeErr)
		} else {
			fmt.Println("  Report written to benchmark-results.md")
		}
	}

	fmt.Println()

	if agg.Overall == "PASS" {
		os.Exit(0)
	}
	os.Exit(1)
}

func runScenario(
	ctx context.Context,
	s *sqlite.SQLiteStore,
	retAgent *retrieval.RetrievalAgent,
	consolAgent *consolidation.ConsolidationAgent,
	sc scenario,
	cycles int,
	verbose bool,
	log *slog.Logger,
) (scenarioResult, error) {
	result := scenarioResult{Name: sc.Name}

	// Phase 1: Create a dummy raw memory so FK constraints are satisfied,
	// then ingest all benchmark memories referencing it.
	rawID := "bench-raw-" + sc.Name
	if err := s.WriteRaw(ctx, store.RawMemory{
		ID:        rawID,
		Timestamp: time.Now(),
		Source:    "benchmark",
		Type:      "synthetic",
		Content:   "Benchmark scenario: " + sc.Name,
		Processed: true,
		CreatedAt: time.Now(),
	}); err != nil {
		return result, fmt.Errorf("writing raw memory: %w", err)
	}

	for i := range sc.Memories {
		sc.Memories[i].Memory.RawID = rawID
	}

	for _, mem := range sc.Memories {
		if err := s.WriteMemory(ctx, mem.Memory); err != nil {
			return result, fmt.Errorf("writing memory %s: %w", mem.Memory.ID, err)
		}
	}

	// Create associations between signal memories.
	for _, assoc := range sc.Associations {
		if err := s.CreateAssociation(ctx, assoc); err != nil {
			return result, fmt.Errorf("creating association: %w", err)
		}
	}

	// Phase 2: Baseline query.
	for _, q := range sc.Queries {
		resp, err := retAgent.Query(ctx, retrieval.QueryRequest{
			Query:      q.Query,
			MaxResults: 5,
		})
		if err != nil {
			return result, fmt.Errorf("baseline query %q: %w", q.Query, err)
		}

		qr := scoreQuery(q, resp, sc.Memories)
		result.BaselineQueries = append(result.BaselineQueries, qr)
	}

	// Phase 3: Access simulation — query signal topics to bump access counts.
	for _, q := range sc.Queries {
		_, _ = retAgent.Query(ctx, retrieval.QueryRequest{
			Query:      q.Query,
			MaxResults: 3,
		})
	}

	// Phase 4: Consolidation cycles with salience decay to simulate time passing.
	for i := 0; i < cycles; i++ {
		allMems, err := s.ListMemories(ctx, "", 500, 0)
		if err != nil {
			return result, fmt.Errorf("listing memories for time shift: %w", err)
		}
		updates := make(map[string]float32, len(allMems))
		for _, m := range allMems {
			updates[m.ID] = m.Salience * 0.92
		}
		if err := s.BatchUpdateSalience(ctx, updates); err != nil {
			return result, fmt.Errorf("batch update salience: %w", err)
		}

		report, err := consolAgent.RunOnce(ctx)
		if err != nil {
			log.Warn("consolidation cycle error", "cycle", i+1, "error", err)
		} else if verbose && report != nil {
			fmt.Printf("    Cycle %d: decayed=%d, fading=%d, archived=%d\n",
				i+1, report.MemoriesDecayed, report.TransitionedFading, report.TransitionedArchived)
		}
	}

	// Phase 5: Post-consolidation query.
	for _, q := range sc.Queries {
		resp, err := retAgent.Query(ctx, retrieval.QueryRequest{
			Query:      q.Query,
			MaxResults: 5,
		})
		if err != nil {
			return result, fmt.Errorf("post-consolidation query %q: %w", q.Query, err)
		}

		qr := scoreQuery(q, resp, sc.Memories)
		result.PostQueries = append(result.PostQueries, qr)
	}

	// Phase 6: System quality metrics.
	allMems, err := s.ListMemories(ctx, "", 500, 0)
	if err != nil {
		return result, fmt.Errorf("listing memories for scoring: %w", err)
	}

	result.SystemMetrics = scoreSystem(sc.Memories, allMems)

	return result, nil
}

// scoreQuery computes IR metrics for a single query result.
func scoreQuery(q benchmarkQuery, resp retrieval.QueryResponse, labeled []labeledMemory) queryResult {
	labelMap := make(map[string]string, len(labeled))
	for _, lm := range labeled {
		labelMap[lm.Memory.ID] = lm.Label
	}

	qr := queryResult{Query: q.Query}
	k := 5
	if len(resp.Memories) < k {
		k = len(resp.Memories)
	}

	expectedSet := make(map[string]bool, len(q.ExpectedIDs))
	for _, id := range q.ExpectedIDs {
		expectedSet[id] = true
	}
	totalRelevant := len(q.ExpectedIDs)

	relevantInK := 0
	dcg := 0.0
	firstRelevantRank := 0

	for i := 0; i < k; i++ {
		memID := resp.Memories[i].Memory.ID
		isRelevant := expectedSet[memID]

		if isRelevant {
			relevantInK++
			if firstRelevantRank == 0 {
				firstRelevantRank = i + 1
			}
			dcg += 1.0 / math.Log2(float64(i+2))
		}
	}

	if k > 0 {
		qr.PrecisionAtK = float64(relevantInK) / float64(k)
	}
	if totalRelevant > 0 {
		qr.RecallAtK = float64(relevantInK) / float64(totalRelevant)
	}
	if firstRelevantRank > 0 {
		qr.MRR = 1.0 / float64(firstRelevantRank)
	}

	idealDCG := 0.0
	idealK := totalRelevant
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

// scoreSystem computes noise suppression and signal retention.
func scoreSystem(labeled []labeledMemory, currentMems []store.Memory) systemMetrics {
	stateMap := make(map[string]string, len(currentMems))
	for _, m := range currentMems {
		stateMap[m.ID] = m.State
	}

	var noiseTotal, noiseSuppressed int
	var signalTotal, signalRetained int

	for _, lm := range labeled {
		state := stateMap[lm.Memory.ID]
		switch lm.Label {
		case "noise":
			noiseTotal++
			if state == "fading" || state == "archived" || state == "merged" {
				noiseSuppressed++
			}
		case "signal":
			signalTotal++
			if state == "active" || state == "fading" {
				signalRetained++
			}
		}
	}

	sm := systemMetrics{}
	if noiseTotal > 0 {
		sm.NoiseSuppression = float64(noiseSuppressed) / float64(noiseTotal)
	}
	if signalTotal > 0 {
		sm.SignalRetention = float64(signalRetained) / float64(signalTotal)
	}

	return sm
}
