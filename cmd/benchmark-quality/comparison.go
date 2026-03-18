package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/consolidation"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/store/sqlite"
)

// --- Comparison types ---

// comparisonQueryResult holds IR metrics for one query from one retriever.
type comparisonQueryResult struct {
	Query        string
	PrecisionAtK float64
	RecallAtK    float64
	MRR          float64
	NDCG         float64
}

// approachResult holds aggregate metrics for one retrieval approach across queries.
type approachResult struct {
	Name         string
	Queries      []comparisonQueryResult
	AvgPrecision float64
	AvgRecall    float64
	AvgMRR       float64
	AvgNDCG      float64
}

// comparisonScenarioResult holds results for all approaches on one scenario.
type comparisonScenarioResult struct {
	ScenarioName string
	Approaches   []approachResult
}

// comparisonReport holds the full comparison results.
type comparisonReport struct {
	Version       string
	Cycles        int
	QueryCount    int
	ScenarioCount int
	Scenarios     []comparisonScenarioResult
	Aggregate     []approachResult // aggregated across all scenarios
}

// --- Comparison runner ---

func runComparison(
	ctx context.Context,
	scenarios []scenario,
	cfg benchConfig,
	cycles int,
	verbose bool,
	log *slog.Logger,
) (comparisonReport, error) {
	report := comparisonReport{
		Version:       Version,
		Cycles:        cycles,
		ScenarioCount: len(scenarios),
	}

	// Accumulators for aggregate metrics per approach.
	type accumulator struct {
		totalPrec, totalRecall, totalMRR, totalNDCG float64
		count                                       int
	}
	aggMap := make(map[string]*accumulator)

	for _, sc := range scenarios {
		if verbose {
			fmt.Printf("  Scenario: %s\n", sc.Name)
		}

		scResult, err := runComparisonScenario(ctx, sc, cfg, cycles, verbose, log)
		if err != nil {
			return report, fmt.Errorf("scenario %q: %w", sc.Name, err)
		}

		report.Scenarios = append(report.Scenarios, scResult)

		for _, ar := range scResult.Approaches {
			acc, ok := aggMap[ar.Name]
			if !ok {
				acc = &accumulator{}
				aggMap[ar.Name] = acc
			}
			for _, q := range ar.Queries {
				acc.totalPrec += q.PrecisionAtK
				acc.totalRecall += q.RecallAtK
				acc.totalMRR += q.MRR
				acc.totalNDCG += q.NDCG
				acc.count++
			}
		}
	}

	// Compute aggregates in consistent order.
	approachOrder := []string{
		"FTS5 (BM25)",
		"Vector (Cosine)",
		"Hybrid (RRF)",
		"Mnemonic (no spread)",
		"Mnemonic (full)",
	}
	for _, name := range approachOrder {
		acc, ok := aggMap[name]
		if !ok || acc.count == 0 {
			continue
		}
		n := float64(acc.count)
		report.Aggregate = append(report.Aggregate, approachResult{
			Name:         name,
			AvgPrecision: acc.totalPrec / n,
			AvgRecall:    acc.totalRecall / n,
			AvgMRR:       acc.totalMRR / n,
			AvgNDCG:      acc.totalNDCG / n,
		})
		report.QueryCount = acc.count // same for all approaches
	}

	return report, nil
}

func runComparisonScenario(
	ctx context.Context,
	sc scenario,
	cfg benchConfig,
	cycles int,
	verbose bool,
	log *slog.Logger,
) (comparisonScenarioResult, error) {
	result := comparisonScenarioResult{ScenarioName: sc.Name}

	// Create isolated temp DB.
	tmpDir, err := os.MkdirTemp("", "mnemonic-compare-*")
	if err != nil {
		return result, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "compare.db")
	s, err := sqlite.NewSQLiteStore(dbPath, 5000)
	if err != nil {
		return result, fmt.Errorf("creating store: %w", err)
	}
	defer func() { _ = s.Close() }()

	var p llm.Provider = &semanticStubProvider{}
	if cfg.Provider != nil {
		p = cfg.Provider
	}

	// Create dummy raw memory for FK constraint.
	rawID := "compare-raw-" + sc.Name
	if err := s.WriteRaw(ctx, store.RawMemory{
		ID:        rawID,
		Timestamp: time.Now(),
		Source:    "benchmark",
		Type:      "synthetic",
		Content:   "Comparison scenario: " + sc.Name,
		Processed: true,
		CreatedAt: time.Now(),
	}); err != nil {
		return result, fmt.Errorf("writing raw memory: %w", err)
	}

	// Ingest all memories.
	for i := range sc.Memories {
		sc.Memories[i].Memory.RawID = rawID
	}
	for _, mem := range sc.Memories {
		if err := s.WriteMemory(ctx, mem.Memory); err != nil {
			return result, fmt.Errorf("writing memory %s: %w", mem.Memory.ID, err)
		}
	}

	// Create associations.
	for _, assoc := range sc.Associations {
		if err := s.CreateAssociation(ctx, assoc); err != nil {
			return result, fmt.Errorf("creating association: %w", err)
		}
	}

	// Run consolidation cycles with salience decay.
	consolAgent := consolidation.NewConsolidationAgent(s, p, cfg.Consolidation, log)
	for i := 0; i < cycles; i++ {
		allMems, listErr := s.ListMemories(ctx, "", 500, 0)
		if listErr != nil {
			return result, fmt.Errorf("listing memories for decay: %w", listErr)
		}
		updates := make(map[string]float32, len(allMems))
		for _, m := range allMems {
			updates[m.ID] = m.Salience * cfg.BenchDecay
		}
		if err := s.BatchUpdateSalience(ctx, updates); err != nil {
			return result, fmt.Errorf("batch update salience: %w", err)
		}
		rpt, consolErr := consolAgent.RunOnce(ctx)
		if consolErr != nil {
			log.Warn("consolidation cycle error", "cycle", i+1, "error", consolErr)
		} else if verbose && rpt != nil {
			fmt.Printf("    Cycle %d: decayed=%d fading=%d archived=%d\n",
				i+1, rpt.MemoriesDecayed, rpt.TransitionedFading, rpt.TransitionedArchived)
		}
	}

	// Create all retrievers.
	fullCfg := cfg.Retrieval
	noSpreadCfg := cfg.Retrieval
	noSpreadCfg.MaxHops = 0

	retrievers := []Retriever{
		newFTSRetriever(s),
		newVectorRetriever(s, p),
		newHybridRetriever(s, p),
		newMnemonicRetriever(s, p, noSpreadCfg, log, "Mnemonic (no spread)"),
		newMnemonicRetriever(s, p, fullCfg, log, "Mnemonic (full)"),
	}

	// Run each retriever against each query.
	for _, ret := range retrievers {
		ar := approachResult{Name: ret.Name()}

		for _, q := range sc.Queries {
			results, retErr := ret.Retrieve(ctx, q.Query, 5)
			if retErr != nil {
				return result, fmt.Errorf("retriever %q query %q: %w", ret.Name(), q.Query, retErr)
			}
			qr := scoreComparisonQuery(q, results)
			ar.Queries = append(ar.Queries, qr)
		}

		// Compute averages.
		if len(ar.Queries) > 0 {
			n := float64(len(ar.Queries))
			for _, q := range ar.Queries {
				ar.AvgPrecision += q.PrecisionAtK
				ar.AvgRecall += q.RecallAtK
				ar.AvgMRR += q.MRR
				ar.AvgNDCG += q.NDCG
			}
			ar.AvgPrecision /= n
			ar.AvgRecall /= n
			ar.AvgMRR /= n
			ar.AvgNDCG /= n
		}

		result.Approaches = append(result.Approaches, ar)

		if verbose {
			fmt.Printf("    %-22s P@5=%.2f  R@5=%.2f  MRR=%.2f  nDCG=%.2f\n",
				ret.Name(), ar.AvgPrecision, ar.AvgRecall, ar.AvgMRR, ar.AvgNDCG)
		}
	}

	return result, nil
}

// scoreComparisonQuery computes IR metrics from ranked results against expected IDs.
// P@k always divides by the requested k (5), not the number of results returned.
// A retriever returning fewer results is penalized for the empty positions.
func scoreComparisonQuery(q benchmarkQuery, results []rankedResult) comparisonQueryResult {
	qr := comparisonQueryResult{Query: q.Query}

	const requestedK = 5
	actualK := len(results)
	if actualK > requestedK {
		actualK = requestedK
	}
	if actualK == 0 {
		return qr
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
		isRelevant := expectedSet[results[i].MemoryID]
		if isRelevant {
			relevantInK++
			if firstRelevantRank == 0 {
				firstRelevantRank = i + 1
			}
			dcg += 1.0 / math.Log2(float64(i+2))
		}
	}

	// Always divide by requestedK, not actualK. If a retriever returns
	// fewer than k results, the missing positions are implicitly non-relevant.
	qr.PrecisionAtK = float64(relevantInK) / float64(requestedK)
	if totalRelevant > 0 {
		qr.RecallAtK = float64(relevantInK) / float64(totalRelevant)
	}
	if firstRelevantRank > 0 {
		qr.MRR = 1.0 / float64(firstRelevantRank)
	}

	// nDCG: ideal DCG assumes all relevant docs are at top positions.
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

// --- Comparison report printing ---

func printComparisonReport(report comparisonReport) {
	sep := strings.Repeat("─", 72)

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Mnemonic Retrieval Comparison Benchmark")
	fmt.Printf("  Version: %s  |  Scenarios: %d  |  Queries: %d  |  Consolidation cycles: %d\n",
		report.Version, report.ScenarioCount, report.QueryCount, report.Cycles)
	fmt.Printf("  Embeddings: deterministic bag-of-words (128-dim, %d-word vocabulary)\n", len(vocabulary))
	fmt.Printf("%s\n\n", sep)

	// Aggregate table.
	fmt.Println("  AGGREGATE RESULTS")
	fmt.Println()
	fmt.Printf("  %-22s  %6s  %6s  %6s  %6s\n", "Approach", "P@5", "R@5", "MRR", "nDCG")
	fmt.Printf("  %-22s  %6s  %6s  %6s  %6s\n", strings.Repeat("─", 22), "──────", "──────", "──────", "──────")

	// Find the hybrid baseline for delta calculation.
	var hybridNDCG float64
	for _, ar := range report.Aggregate {
		if ar.Name == "Hybrid (RRF)" {
			hybridNDCG = ar.AvgNDCG
			break
		}
	}

	for _, ar := range report.Aggregate {
		delta := ""
		if hybridNDCG > 0 && ar.Name != "Hybrid (RRF)" {
			pct := (ar.AvgNDCG - hybridNDCG) / hybridNDCG * 100
			if pct >= 0 {
				delta = fmt.Sprintf("  +%.0f%%", pct)
			} else {
				delta = fmt.Sprintf("  %.0f%%", pct)
			}
		}
		fmt.Printf("  %-22s  %6.3f  %6.3f  %6.3f  %6.3f%s\n",
			ar.Name, ar.AvgPrecision, ar.AvgRecall, ar.AvgMRR, ar.AvgNDCG, delta)
	}

	// Per-scenario breakdown.
	fmt.Printf("\n%s\n", sep)
	fmt.Println("  PER-SCENARIO BREAKDOWN")

	for _, sc := range report.Scenarios {
		fmt.Printf("\n  %s\n", sc.ScenarioName)
		fmt.Printf("  %-22s  %6s  %6s  %6s  %6s\n", "", "P@5", "R@5", "MRR", "nDCG")

		for _, ar := range sc.Approaches {
			fmt.Printf("  %-22s  %6.3f  %6.3f  %6.3f  %6.3f\n",
				ar.Name, ar.AvgPrecision, ar.AvgRecall, ar.AvgMRR, ar.AvgNDCG)
		}
	}

	// Per-query detail for each scenario.
	fmt.Printf("\n%s\n", sep)
	fmt.Println("  PER-QUERY DETAIL")

	for _, sc := range report.Scenarios {
		fmt.Printf("\n  %s\n", sc.ScenarioName)

		// Collect query names from the first approach.
		if len(sc.Approaches) == 0 {
			continue
		}
		for qi, q := range sc.Approaches[0].Queries {
			queryStr := q.Query
			if len(queryStr) > 50 {
				queryStr = queryStr[:47] + "..."
			}
			fmt.Printf("\n    Q: %s\n", queryStr)
			fmt.Printf("    %-22s  %6s  %6s\n", "", "P@5", "nDCG")
			for _, ar := range sc.Approaches {
				if qi < len(ar.Queries) {
					fmt.Printf("    %-22s  %6.3f  %6.3f\n",
						ar.Name, ar.Queries[qi].PrecisionAtK, ar.Queries[qi].NDCG)
				}
			}
		}
	}

	fmt.Printf("\n%s\n", sep)

	// Methodology note.
	fmt.Println("  METHODOLOGY")
	fmt.Println("  • Each scenario runs in an isolated SQLite database")
	fmt.Println("  • Signal memories have labeled ground-truth IDs for scoring")
	fmt.Println("  • Noise memories simulate realistic desktop/terminal activity")
	fmt.Println("  • Consolidation cycles apply salience decay before retrieval")
	fmt.Println("  • IR metrics: Precision@5, Recall@5, Mean Reciprocal Rank, nDCG@5")
	fmt.Println("  • RRF baseline uses k=60 (Cormack et al. 2009)")
	fmt.Println("  • Embeddings are deterministic bag-of-words (not LLM-generated)")
	fmt.Printf("%s\n\n", sep)
}

// writeComparisonMarkdown writes the comparison report as a markdown file.
func writeComparisonMarkdown(report comparisonReport) error {
	var sb strings.Builder

	sb.WriteString("# Mnemonic Retrieval Comparison Benchmark\n\n")
	fmt.Fprintf(&sb, "**Version:** %s | **Scenarios:** %d | **Queries:** %d | **Consolidation Cycles:** %d\n\n",
		report.Version, report.ScenarioCount, report.QueryCount, report.Cycles)
	sb.WriteString("**Embeddings:** Deterministic bag-of-words (128-dim, ")
	fmt.Fprintf(&sb, "%d-word vocabulary)\n\n", len(vocabulary))

	// Aggregate table.
	sb.WriteString("## Aggregate Results\n\n")
	sb.WriteString("| Approach | P@5 | R@5 | MRR | nDCG | vs Hybrid |\n")
	sb.WriteString("|---|---|---|---|---|---|\n")

	var hybridNDCG float64
	for _, ar := range report.Aggregate {
		if ar.Name == "Hybrid (RRF)" {
			hybridNDCG = ar.AvgNDCG
			break
		}
	}

	for _, ar := range report.Aggregate {
		delta := "—"
		if hybridNDCG > 0 && ar.Name != "Hybrid (RRF)" {
			pct := (ar.AvgNDCG - hybridNDCG) / hybridNDCG * 100
			if pct >= 0 {
				delta = fmt.Sprintf("+%.0f%%", pct)
			} else {
				delta = fmt.Sprintf("%.0f%%", pct)
			}
		}
		fmt.Fprintf(&sb, "| %s | %.3f | %.3f | %.3f | %.3f | %s |\n",
			ar.Name, ar.AvgPrecision, ar.AvgRecall, ar.AvgMRR, ar.AvgNDCG, delta)
	}

	// Per-scenario tables.
	sb.WriteString("\n## Per-Scenario Breakdown\n")

	for _, sc := range report.Scenarios {
		fmt.Fprintf(&sb, "\n### %s\n\n", sc.ScenarioName)
		sb.WriteString("| Approach | P@5 | R@5 | MRR | nDCG |\n")
		sb.WriteString("|---|---|---|---|---|\n")
		for _, ar := range sc.Approaches {
			fmt.Fprintf(&sb, "| %s | %.3f | %.3f | %.3f | %.3f |\n",
				ar.Name, ar.AvgPrecision, ar.AvgRecall, ar.AvgMRR, ar.AvgNDCG)
		}
	}

	// Methodology.
	sb.WriteString("\n## Methodology\n\n")
	sb.WriteString("- Each scenario runs in an isolated SQLite database (no cross-contamination)\n")
	sb.WriteString("- Signal memories have labeled ground-truth IDs for precision/recall scoring\n")
	sb.WriteString("- Noise memories simulate realistic desktop, terminal, and clipboard activity\n")
	sb.WriteString("- Consolidation cycles apply salience decay (0.92/cycle) before retrieval\n")
	sb.WriteString("- **IR Metrics:** Precision@5, Recall@5, Mean Reciprocal Rank (MRR), Normalized Discounted Cumulative Gain (nDCG@5)\n")
	sb.WriteString("- **RRF Baseline:** Reciprocal Rank Fusion with k=60 ([Cormack et al. 2009](https://doi.org/10.1145/1571941.1572114))\n")
	sb.WriteString("- **Embeddings:** Deterministic bag-of-words — not LLM-generated. Real-world performance with LLM embeddings may differ.\n\n")

	sb.WriteString("### Approaches\n\n")
	sb.WriteString("| Approach | Description |\n|---|---|\n")
	sb.WriteString("| FTS5 (BM25) | SQLite full-text search with BM25 ranking. Pure keyword matching. |\n")
	sb.WriteString("| Vector (Cosine) | Embedding cosine similarity only. Standard semantic search. |\n")
	sb.WriteString("| Hybrid (RRF) | FTS5 + Vector combined via Reciprocal Rank Fusion. Industry-standard RAG. |\n")
	sb.WriteString("| Mnemonic (no spread) | Full Mnemonic pipeline minus spread activation. Tests merge + scoring. |\n")
	sb.WriteString("| Mnemonic (full) | Full pipeline: FTS5 + Vector + merge + spread activation over association graph. |\n")

	return os.WriteFile("comparison-results.md", []byte(sb.String()), 0644)
}
