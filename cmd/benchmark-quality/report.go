package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	threshPassPrecision = 0.70
	threshWarnPrecision = 0.50
	threshPassMRR       = 0.60
	threshWarnMRR       = 0.40
	threshPassNoise     = 0.60
	threshWarnNoise     = 0.40
	threshPassSignal    = 0.80
	threshWarnSignal    = 0.60
)

func grade(val, pass, warn float64) string {
	if val >= pass {
		return "PASS"
	}
	if val >= warn {
		return "WARN"
	}
	return "FAIL"
}

func bar(val float64, width int) string {
	filled := int(val * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("\u2588", filled) + strings.Repeat(" ", width-filled)
}

func aggregateResults(results []scenarioResult) aggregateResult {
	var totalPrec, totalMRR, totalNDCG float64
	var totalNoise, totalSignal float64
	queryCount := 0

	for _, r := range results {
		for _, q := range r.PostQueries {
			totalPrec += q.PrecisionAtK
			totalMRR += q.MRR
			totalNDCG += q.NDCG
			queryCount++
		}
		totalNoise += r.SystemMetrics.NoiseSuppression
		totalSignal += r.SystemMetrics.SignalRetention
	}

	n := float64(len(results))
	agg := aggregateResult{}
	if queryCount > 0 {
		agg.AvgPrecision = totalPrec / float64(queryCount)
		agg.AvgMRR = totalMRR / float64(queryCount)
		agg.AvgNDCG = totalNDCG / float64(queryCount)
	}
	if n > 0 {
		agg.AvgNoiseSuppression = totalNoise / n
		agg.AvgSignalRetention = totalSignal / n
	}

	// Overall: FAIL if any metric fails, WARN if any warns, else PASS.
	grades := []string{
		grade(agg.AvgPrecision, threshPassPrecision, threshWarnPrecision),
		grade(agg.AvgMRR, threshPassMRR, threshWarnMRR),
		grade(agg.AvgNoiseSuppression, threshPassNoise, threshWarnNoise),
		grade(agg.AvgSignalRetention, threshPassSignal, threshWarnSignal),
	}
	agg.Overall = "PASS"
	for _, g := range grades {
		if g == "FAIL" {
			agg.Overall = "FAIL"
			break
		}
		if g == "WARN" {
			agg.Overall = "WARN"
		}
	}

	return agg
}

func printScenarioResult(r scenarioResult) {
	fmt.Printf("  SCENARIO: %s\n", r.Name)

	// Post-consolidation query metrics.
	var avgPrec, avgMRR, avgNDCG float64
	for _, q := range r.PostQueries {
		avgPrec += q.PrecisionAtK
		avgMRR += q.MRR
		avgNDCG += q.NDCG
	}
	n := float64(len(r.PostQueries))
	if n > 0 {
		avgPrec /= n
		avgMRR /= n
		avgNDCG /= n
	}

	fmt.Printf("    Precision@5   %.2f  %s  %s\n", avgPrec, bar(avgPrec, 10), grade(avgPrec, threshPassPrecision, threshWarnPrecision))
	fmt.Printf("    MRR           %.2f  %s  %s\n", avgMRR, bar(avgMRR, 10), grade(avgMRR, threshPassMRR, threshWarnMRR))
	fmt.Printf("    nDCG          %.2f  %s  %s\n", avgNDCG, bar(avgNDCG, 10), grade(avgNDCG, 0.60, 0.40))
	fmt.Printf("    Noise Suppr.  %.2f  %s  %s\n", r.SystemMetrics.NoiseSuppression, bar(r.SystemMetrics.NoiseSuppression, 10), grade(r.SystemMetrics.NoiseSuppression, threshPassNoise, threshWarnNoise))
	fmt.Printf("    Signal Ret.   %.2f  %s  %s\n", r.SystemMetrics.SignalRetention, bar(r.SystemMetrics.SignalRetention, 10), grade(r.SystemMetrics.SignalRetention, threshPassSignal, threshWarnSignal))
	fmt.Println()
}

func printAggregate(agg aggregateResult) {
	fmt.Println("  AGGREGATE")
	fmt.Printf("    Precision@5 %.2f  |  MRR %.2f  |  nDCG %.2f\n", agg.AvgPrecision, agg.AvgMRR, agg.AvgNDCG)
	fmt.Printf("    Noise Suppression %.2f  |  Signal Retention %.2f\n", agg.AvgNoiseSuppression, agg.AvgSignalRetention)
	fmt.Printf("\n    Overall: %s\n", agg.Overall)
}

func writeMarkdownReport(results []scenarioResult, agg aggregateResult, cycles int) error {
	var sb strings.Builder

	sb.WriteString("# Mnemonic Memory Quality Benchmark\n\n")
	sb.WriteString(fmt.Sprintf("**Version:** %s | **LLM:** synthetic | **Cycles:** %d\n\n", Version, cycles))

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("## %s\n\n", r.Name))
		sb.WriteString("| Metric | Score | Grade |\n|---|---|---|\n")

		var avgPrec, avgMRR, avgNDCG float64
		for _, q := range r.PostQueries {
			avgPrec += q.PrecisionAtK
			avgMRR += q.MRR
			avgNDCG += q.NDCG
		}
		n := float64(len(r.PostQueries))
		if n > 0 {
			avgPrec /= n
			avgMRR /= n
			avgNDCG /= n
		}

		sb.WriteString(fmt.Sprintf("| Precision@5 | %.2f | %s |\n", avgPrec, grade(avgPrec, threshPassPrecision, threshWarnPrecision)))
		sb.WriteString(fmt.Sprintf("| MRR | %.2f | %s |\n", avgMRR, grade(avgMRR, threshPassMRR, threshWarnMRR)))
		sb.WriteString(fmt.Sprintf("| nDCG | %.2f | %s |\n", avgNDCG, grade(avgNDCG, 0.60, 0.40)))
		sb.WriteString(fmt.Sprintf("| Noise Suppression | %.2f | %s |\n", r.SystemMetrics.NoiseSuppression, grade(r.SystemMetrics.NoiseSuppression, threshPassNoise, threshWarnNoise)))
		sb.WriteString(fmt.Sprintf("| Signal Retention | %.2f | %s |\n", r.SystemMetrics.SignalRetention, grade(r.SystemMetrics.SignalRetention, threshPassSignal, threshWarnSignal)))
		sb.WriteString("\n")

		// Per-query breakdown.
		sb.WriteString("### Query Results (Post-Consolidation)\n\n")
		sb.WriteString("| Query | P@5 | MRR | nDCG |\n|---|---|---|---|\n")
		for _, q := range r.PostQueries {
			sb.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %.2f |\n", q.Query, q.PrecisionAtK, q.MRR, q.NDCG))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Aggregate\n\n")
	sb.WriteString("| Metric | Score | Grade |\n|---|---|---|\n")
	sb.WriteString(fmt.Sprintf("| Precision@5 | %.2f | %s |\n", agg.AvgPrecision, grade(agg.AvgPrecision, threshPassPrecision, threshWarnPrecision)))
	sb.WriteString(fmt.Sprintf("| MRR | %.2f | %s |\n", agg.AvgMRR, grade(agg.AvgMRR, threshPassMRR, threshWarnMRR)))
	sb.WriteString(fmt.Sprintf("| nDCG | %.2f | %s |\n", agg.AvgNDCG, grade(agg.AvgNDCG, 0.60, 0.40)))
	sb.WriteString(fmt.Sprintf("| Noise Suppression | %.2f | %s |\n", agg.AvgNoiseSuppression, grade(agg.AvgNoiseSuppression, threshPassNoise, threshWarnNoise)))
	sb.WriteString(fmt.Sprintf("| Signal Retention | %.2f | %s |\n", agg.AvgSignalRetention, grade(agg.AvgSignalRetention, threshPassSignal, threshWarnSignal)))
	sb.WriteString(fmt.Sprintf("\n**Overall: %s**\n", agg.Overall))

	return os.WriteFile("benchmark-results.md", []byte(sb.String()), 0644)
}
