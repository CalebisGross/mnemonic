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

	"github.com/appsprout-dev/mnemonic/internal/agent/abstraction"
	"github.com/appsprout-dev/mnemonic/internal/agent/consolidation"
	"github.com/appsprout-dev/mnemonic/internal/agent/dreaming"
	"github.com/appsprout-dev/mnemonic/internal/agent/encoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/episoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/store/sqlite"
)

var Version = "dev"

// benchConfig holds all tunable parameters for a benchmark run.
type benchConfig struct {
	Retrieval     retrieval.RetrievalConfig
	Consolidation consolidation.ConsolidationConfig
	Encoding      encoding.EncodingConfig
	Dreaming      dreaming.DreamingConfig
	Episoding     episoding.EpisodingConfig
	Abstraction   abstraction.AbstractionConfig
	BenchDecay    float32 // per-cycle salience decay (default 0.92)
}

// defaultBenchConfig returns a benchConfig with sensible defaults.
func defaultBenchConfig() benchConfig {
	return benchConfig{
		Retrieval:     retrieval.DefaultConfig(),
		Consolidation: consolidation.DefaultConfig(),
		Encoding:      encoding.DefaultConfig(),
		Dreaming: dreaming.DreamingConfig{
			Interval:               time.Hour,
			BatchSize:              60,
			SalienceThreshold:      0.3,
			AssociationBoostFactor: 1.15,
			NoisePruneThreshold:    0.15,
		},
		Episoding: episoding.EpisodingConfig{
			EpisodeWindowSizeMin: 10,
			MinEventsPerEpisode:  2,
			PollingInterval:      10 * time.Second,
		},
		Abstraction: abstraction.AbstractionConfig{
			Interval:    time.Hour,
			MinStrength: 0.4,
			MaxLLMCalls: 5,
		},
		BenchDecay: 0.92,
	}
}

func main() {
	var (
		verbose   bool
		cycles    int
		report    string
		llmMode   bool
		sweepFile string
		compare   bool
		setFlags  setFlagList
	)

	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.IntVar(&cycles, "cycles", 5, "number of consolidation cycles")
	flag.StringVar(&report, "report", "", "output format: 'markdown' writes benchmark-results.md")
	flag.BoolVar(&llmMode, "llm", false, "use real LLM for embeddings (requires LM Studio)")
	flag.StringVar(&sweepFile, "sweep", "", "path to sweep YAML file for parameter tuning")
	flag.BoolVar(&compare, "compare", false, "run comparison benchmark: FTS vs Vector vs Hybrid vs Mnemonic")
	flag.Var(&setFlags, "set", "override a config parameter (repeatable, format: key=value)")
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

	ctx := context.Background()
	scenarios := allScenarios()

	// Parse -set overrides into a map.
	overrides := make(map[string]float64)
	for _, s := range setFlags {
		key, val, parseErr := parseSetFlag(s)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", parseErr)
			os.Exit(1)
		}
		overrides[key] = val
	}

	// Sweep mode: run parameter sweep and exit.
	if sweepFile != "" {
		def, loadErr := loadSweepDefinition(sweepFile)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", loadErr)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("  Mnemonic Config Sweep")
		fmt.Printf("  Version: %s  |  LLM: semantic-stub  |  Params: %d\n", Version, len(def.Sweeps))
		fmt.Println()

		sweepReport, sweepErr := runSweep(ctx, def, scenarios, cycles, verbose, log)
		if sweepErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", sweepErr)
			os.Exit(1)
		}

		printSweepReport(sweepReport)

		if report == "markdown" {
			sweepCycles := cycles
			if def.Cycles > 0 {
				sweepCycles = def.Cycles
			}
			if writeErr := writeSweepMarkdownReport(sweepReport, sweepCycles); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Error writing sweep report: %v\n", writeErr)
			} else {
				fmt.Println("  Sweep report written to sweep-results.md")
			}
		}
		fmt.Println()
		os.Exit(0)
	}

	// Comparison mode: run all approaches against all scenarios.
	if compare {
		cfg := defaultBenchConfig()
		if len(overrides) > 0 {
			if overrideErr := applyOverrides(&cfg, overrides); overrideErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", overrideErr)
				os.Exit(1)
			}
		}

		compReport, compErr := runComparison(ctx, scenarios, cfg, cycles, verbose, log)
		if compErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", compErr)
			os.Exit(1)
		}

		printComparisonReport(compReport)

		if report == "markdown" {
			if writeErr := writeComparisonMarkdown(compReport); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Error writing comparison report: %v\n", writeErr)
			} else {
				fmt.Println("  Comparison report written to comparison-results.md")
			}
		}

		fmt.Println()
		os.Exit(0)
	}

	// Standard mode: run all scenarios with optional -set overrides.
	cfg := defaultBenchConfig()
	if len(overrides) > 0 {
		if overrideErr := applyOverrides(&cfg, overrides); overrideErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", overrideErr)
			os.Exit(1)
		}
	}

	fmt.Println()
	fmt.Println("  Mnemonic Memory Quality Benchmark")
	fmt.Printf("  Version: %s  |  LLM: semantic-stub  |  Cycles: %d\n", Version, cycles)
	fmt.Println()

	var allResults []scenarioResult

	for _, sc := range scenarios {
		if verbose {
			fmt.Printf("  Running: %s\n", sc.Name)
		}

		result, runErr := runScenario(ctx, sc, cfg, cycles, verbose, log)
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "  Error in scenario %q: %v\n", sc.Name, runErr)
			os.Exit(1)
		}

		allResults = append(allResults, result)
		printScenarioResult(result)
	}

	// Aggregate and print direct scenario results.
	agg := aggregateResults(allResults)
	printAggregate(agg)

	// Run pipeline scenarios.
	pipelineScenarios := allPipelineScenarios()
	var pipelineResults []pipelineResult

	if len(pipelineScenarios) > 0 {
		fmt.Println()
		fmt.Println("  Pipeline Scenarios")
		fmt.Println()

		for _, ps := range pipelineScenarios {
			if verbose {
				fmt.Printf("  Running pipeline: %s\n", ps.Name)
			}

			pr, runErr := runPipelineScenario(ctx, ps, cfg, cycles, verbose, log)
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "  Error in pipeline scenario %q: %v\n", ps.Name, runErr)
				os.Exit(1)
			}

			pipelineResults = append(pipelineResults, pr)
			printPipelineResult(pr)
		}
	}

	if report == "markdown" {
		if writeErr := writeMarkdownReport(allResults, agg, cycles); writeErr != nil {
			fmt.Fprintf(os.Stderr, "Error writing report: %v\n", writeErr)
		} else {
			fmt.Println("  Report written to benchmark-results.md")
		}
	}

	_ = pipelineResults // used in future for combined reporting

	fmt.Println()

	if agg.Overall == "PASS" {
		os.Exit(0)
	}
	os.Exit(1)
}

// runScenario executes a single benchmark scenario with its own isolated DB.
// Each call creates a fresh temp DB, store, and agents to prevent cross-contamination.
func runScenario(
	ctx context.Context,
	sc scenario,
	cfg benchConfig,
	cycles int,
	verbose bool,
	log *slog.Logger,
) (scenarioResult, error) {
	result := scenarioResult{Name: sc.Name}

	// Create an isolated temp DB for this scenario.
	tmpDir, err := os.MkdirTemp("", "mnemonic-bench-*")
	if err != nil {
		return result, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "bench.db")
	s, err := sqlite.NewSQLiteStore(dbPath, 5000)
	if err != nil {
		return result, fmt.Errorf("creating store: %w", err)
	}
	defer func() { _ = s.Close() }()

	stub := &semanticStubProvider{}
	retAgent := retrieval.NewRetrievalAgent(s, stub, cfg.Retrieval, log)
	consolAgent := consolidation.NewConsolidationAgent(s, stub, cfg.Consolidation, log)

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
func scoreQuery(q benchmarkQuery, resp retrieval.QueryResponse, _ []labeledMemory) queryResult {
	qr := queryResult{Query: q.Query}
	const requestedK = 5
	actualK := len(resp.Memories)
	if actualK > requestedK {
		actualK = requestedK
	}

	expectedSet := make(map[string]bool, len(q.ExpectedIDs))
	for _, id := range q.ExpectedIDs {
		expectedSet[id] = true
	}
	totalRelevant := len(q.ExpectedIDs)

	relevantInK := 0
	dcg := 0.0
	firstRelevantRank := 0

	for i := 0; i < actualK; i++ {
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

	// Always divide by requestedK, not actualK.
	qr.PrecisionAtK = float64(relevantInK) / float64(requestedK)
	if totalRelevant > 0 {
		qr.RecallAtK = float64(relevantInK) / float64(totalRelevant)
	}
	if firstRelevantRank > 0 {
		qr.MRR = 1.0 / float64(firstRelevantRank)
	}

	idealDCG := 0.0
	idealK := totalRelevant
	if idealK > requestedK {
		idealK = requestedK
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
