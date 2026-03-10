package main

import (
	"github.com/appsprout/mnemonic/internal/store"
)

// labeledMemory wraps a memory with its ground-truth label.
type labeledMemory struct {
	Memory store.Memory
	Label  string // "signal", "noise", "duplicate"
}

// benchmarkQuery defines a test query with expected results.
type benchmarkQuery struct {
	Query       string
	ExpectedIDs []string // IDs of signal memories that should appear
}

// scenario defines a complete test scenario.
type scenario struct {
	Name         string
	Memories     []labeledMemory
	Associations []store.Association
	Queries      []benchmarkQuery
}

// queryResult holds IR metrics for a single query.
type queryResult struct {
	Query        string
	PrecisionAtK float64
	RecallAtK    float64
	MRR          float64
	NDCG         float64
}

// systemMetrics holds system-level quality metrics.
type systemMetrics struct {
	NoiseSuppression float64
	SignalRetention  float64
}

// scenarioResult holds all results for one scenario.
type scenarioResult struct {
	Name            string
	BaselineQueries []queryResult
	PostQueries     []queryResult
	SystemMetrics   systemMetrics
}

// aggregateResult holds the final aggregated scores.
type aggregateResult struct {
	AvgPrecision        float64
	AvgMRR              float64
	AvgNDCG             float64
	AvgNoiseSuppression float64
	AvgSignalRetention  float64
	Overall             string // "PASS", "WARN", "FAIL"
}
