package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const baseURL = "http://127.0.0.1:9999"

// --- API types (mirroring server structs) ---

type createMemoryReq struct {
	Content string `json:"content"`
	Source  string `json:"source"`
}

type createMemoryResp struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
}

type statsResp struct {
	Store struct {
		TotalMemories     int     `json:"total_memories"`
		ActiveMemories    int     `json:"active_memories"`
		TotalAssociations int     `json:"total_associations"`
		AvgAssocPerMem    float64 `json:"avg_associations_per_memory"`
	} `json:"store"`
}

type queryReq struct {
	Query      string `json:"query"`
	Limit      int    `json:"limit"`
	Synthesize bool   `json:"synthesize"`
}

type memoryJSON struct {
	ID       string   `json:"id"`
	Summary  string   `json:"summary"`
	Content  string   `json:"content"`
	Concepts []string `json:"concepts"`
	Salience float32  `json:"salience"`
	State    string   `json:"state"`
}

type retrievalResult struct {
	Memory memoryJSON `json:"memory"`
	Score  float32    `json:"score"`
}

type queryResp struct {
	QueryID   string            `json:"query_id"`
	Memories  []retrievalResult `json:"memories"`
	Synthesis string            `json:"synthesis"`
	TookMs    int64             `json:"took_ms"`
}

// --- Benchmark data ---

type seedMemory struct {
	content  string
	keywords []string // expected keywords for retrieval scoring
}

var decisions = []seedMemory{
	{
		content:  "Decision: Chose SQLite over Postgres for local-first simplicity. Postgres requires a running server and adds deployment complexity. SQLite with WAL mode gives us concurrent reads without the overhead.",
		keywords: []string{"sqlite", "postgres"},
	},
	{
		content:  "Decision: Selected WAL journal mode for the SQLite database. This allows concurrent readers while a single writer holds the lock. Much better than the default rollback journal for our read-heavy workload.",
		keywords: []string{"wal", "journal"},
	},
	{
		content:  "Decision: Used spread activation with 3 max hops for memory retrieval. More hops increase recall but slow down queries exponentially. Three hops is the sweet spot for our graph density.",
		keywords: []string{"spread activation", "hops", "retrieval"},
	},
	{
		content:  "Decision: Implemented a multi-resolution memory architecture with gist, summary, and full content levels. This lets the LLM work at the right level of detail — gist for browsing, full content for deep recall.",
		keywords: []string{"multi-resolution", "gist", "summary"},
	},
	{
		content:  "Decision: Kept the LLM read-only during retrieval synthesis. The model can browse memories using tools (search, follow associations, get details) but all writes go through the firmware pipeline. This prevents hallucinated writes.",
		keywords: []string{"read-only", "tool", "synthesis"},
	},
}

var errors_ = []seedMemory{
	{
		content:  "Error: Nil pointer panic in consolidation agent when the event bus was nil. The runCycle method published events without nil-checking the bus field. Fixed by adding a nil guard before bus.Publish calls.",
		keywords: []string{"nil pointer", "consolidation", "bus"},
	},
	{
		content:  "Error: FTS5 index corruption after directly deleting rows from the memories table. The FTS content table is linked to memories — you must delete from the FTS table first or use the rebuild command. Fixed by rebuilding the index.",
		keywords: []string{"fts", "corruption", "index"},
	},
	{
		content:  "Error: Binary plist data from the macOS Photos Library polluted the memory store with 323 garbage memories. The filesystem watcher's binary file check only looked at extensions, not content. Fixed by adding runtime binary content detection (>10% non-printable bytes).",
		keywords: []string{"binary", "photos library", "garbage"},
	},
	{
		content:  "Error: Consolidation agent entered infinite loop due to self-triggering event. It published ConsolidationStarted at the top of runCycle, but also subscribed to that event to trigger new cycles. Fixed by removing the self-publish.",
		keywords: []string{"infinite loop", "consolidation", "event"},
	},
	{
		content:  "Error: LLM tool-use synthesis timed out at 30 seconds. The Qwen3 8B model needs more time for multi-turn tool-use conversations. Fixed by increasing the timeout to 120 seconds in config.",
		keywords: []string{"timeout", "tool-use", "120"},
	},
}

var insights = []seedMemory{
	{
		content:  "Insight: 8B parameter models work well with 5 or fewer tools. Beyond 5 tools, the model starts confusing tool names and arguments. The sweet spot for Qwen3 VL 8B is 3-5 clearly distinct tools.",
		keywords: []string{"8b", "tools", "qwen"},
	},
	{
		content:  "Insight: Decay rate 0.95 per consolidation cycle is fairly aggressive. With 6-hour cycles, a memory's salience halves in about 14 cycles (3.5 days). Memories that aren't accessed or boosted will fade quickly.",
		keywords: []string{"decay", "0.95", "salience"},
	},
	{
		content:  "Insight: Event bus handler signatures in Go must match exactly. The Bus.Subscribe handler type is func(ctx context.Context, event Event) error — forgetting the context parameter or error return causes a compile error that's hard to debug.",
		keywords: []string{"event bus", "handler", "signature"},
	},
	{
		content:  "Insight: Spread activation with a decay factor of 0.7 means the third hop only contributes 0.7^2 = 0.49 of the original activation. This naturally limits the influence of distant associations without needing a hard cutoff.",
		keywords: []string{"spread activation", "decay", "0.7"},
	},
	{
		content:  "Insight: The encoding agent's contextual encoding feature (looking back at recent episodes and similar memories) dramatically improves association quality. Without it, memories are encoded in isolation and miss obvious connections.",
		keywords: []string{"encoding", "contextual", "association"},
	},
}

// --- Benchmark queries ---

type benchQuery struct {
	query         string
	expectedHits  []string // substrings that should appear in returned memory summaries/content
	synthesisHits []string // substrings that should appear in the synthesis
	description   string
}

var queries = []benchQuery{
	{
		query:         "Why did we choose SQLite?",
		expectedHits:  []string{"sqlite", "postgres"},
		synthesisHits: []string{"sqlite"},
		description:   "SQLite decision",
	},
	{
		query:         "What bugs and errors have we encountered?",
		expectedHits:  []string{"nil pointer", "fts", "binary", "infinite loop", "timeout"},
		synthesisHits: []string{"error", "fix"},
		description:   "Error recall",
	},
	{
		query:         "How does memory retrieval work?",
		expectedHits:  []string{"spread activation"},
		synthesisHits: []string{"retrieval"},
		description:   "Retrieval mechanism",
	},
	{
		query:         "What happened with the Photos Library?",
		expectedHits:  []string{"binary", "photos"},
		synthesisHits: []string{"binary"},
		description:   "Photos Library error",
	},
	{
		query:         "What are the architecture decisions?",
		expectedHits:  []string{"sqlite", "wal", "spread", "multi-resolution", "read-only"},
		synthesisHits: []string{"decision"},
		description:   "All decisions",
	},
}

// --- HTTP helpers ---

var client = &http.Client{Timeout: 180 * time.Second}

func postJSON(path string, body interface{}) ([]byte, int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Post(baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	respData, err := io.ReadAll(resp.Body)
	return respData, resp.StatusCode, err
}

func getJSON(path string) ([]byte, int, error) {
	resp, err := client.Get(baseURL + path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// --- Benchmark phases ---

func phase1Ingest() (int, time.Duration, error) {
	fmt.Println("\n=== Phase 1: Ingestion ===")

	all := make([]seedMemory, 0, len(decisions)+len(errors_)+len(insights))
	all = append(all, decisions...)
	all = append(all, errors_...)
	all = append(all, insights...)

	start := time.Now()
	ingested := 0

	for i, m := range all {
		t := time.Now()
		body, status, err := postJSON("/api/v1/memories", createMemoryReq{
			Content: m.content,
			Source:  "benchmark",
		})
		elapsed := time.Since(t)

		if err != nil {
			return ingested, time.Since(start), fmt.Errorf("memory %d: %w", i, err)
		}
		if status != 201 {
			return ingested, time.Since(start), fmt.Errorf("memory %d: status %d: %s", i, status, body)
		}

		var resp createMemoryResp
		_ = json.Unmarshal(body, &resp)
		fmt.Printf("  [%2d] %s  (%.0fms)\n", i+1, resp.ID[:8], float64(elapsed.Milliseconds()))
		ingested++
	}

	total := time.Since(start)
	fmt.Printf("  Ingested %d memories in %s (avg %dms)\n", ingested, total.Round(time.Millisecond), total.Milliseconds()/int64(ingested))
	return ingested, total, nil
}

func phase2WaitEncoding(expected int) (time.Duration, int, error) {
	fmt.Println("\n=== Phase 2: Wait for Encoding ===")

	start := time.Now()
	deadline := time.After(5 * time.Minute)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	lastCount := 0
	for {
		select {
		case <-deadline:
			return time.Since(start), lastCount, fmt.Errorf("encoding timed out after 5 minutes (got %d/%d)", lastCount, expected)
		case <-ticker.C:
			body, status, err := getJSON("/api/v1/stats")
			if err != nil || status != 200 {
				continue
			}
			var stats statsResp
			_ = json.Unmarshal(body, &stats)
			count := stats.Store.TotalMemories
			if count != lastCount {
				fmt.Printf("  Encoded: %d / %d\n", count, expected)
				lastCount = count
			}
			if count >= expected {
				elapsed := time.Since(start)
				fmt.Printf("  All %d memories encoded in %s\n", count, elapsed.Round(time.Millisecond))
				return elapsed, count, nil
			}
		}
	}
}

type queryResult struct {
	query        benchQuery
	resp         queryResp
	latency      time.Duration
	precision    float64 // fraction of expected hits found in top results
	synthesisOK  bool    // synthesis is non-empty
	synthesisHit bool    // synthesis contains expected keywords
	err          error
}

func phase3Retrieval() ([]queryResult, error) {
	fmt.Println("\n=== Phase 3: Retrieval + Synthesis ===")

	results := make([]queryResult, len(queries))

	for i, q := range queries {
		fmt.Printf("  Query %d: %q\n", i+1, q.query)

		t := time.Now()
		body, status, err := postJSON("/api/v1/query", queryReq{
			Query:      q.query,
			Limit:      7,
			Synthesize: true,
		})
		latency := time.Since(t)

		results[i] = queryResult{query: q, latency: latency}

		if err != nil {
			results[i].err = err
			fmt.Printf("    ERROR: %v\n", err)
			continue
		}
		if status != 200 {
			results[i].err = fmt.Errorf("status %d: %s", status, body)
			fmt.Printf("    ERROR: status %d\n", status)
			continue
		}

		var resp queryResp
		_ = json.Unmarshal(body, &resp)
		results[i].resp = resp

		// Score precision: how many expected keywords appear in returned memories
		hits := 0
		for _, expected := range q.expectedHits {
			for _, mem := range resp.Memories {
				combined := strings.ToLower(mem.Memory.Summary + " " + mem.Memory.Content)
				if strings.Contains(combined, strings.ToLower(expected)) {
					hits++
					break
				}
			}
		}
		if len(q.expectedHits) > 0 {
			results[i].precision = float64(hits) / float64(len(q.expectedHits))
		}

		// Score synthesis
		results[i].synthesisOK = resp.Synthesis != ""
		if resp.Synthesis != "" {
			synthLower := strings.ToLower(resp.Synthesis)
			synthHits := 0
			for _, kw := range q.synthesisHits {
				if strings.Contains(synthLower, strings.ToLower(kw)) {
					synthHits++
				}
			}
			results[i].synthesisHit = synthHits > 0
		}

		fmt.Printf("    Results: %d memories, precision: %.0f%%, synthesis: %d chars, took: %dms\n",
			len(resp.Memories),
			results[i].precision*100,
			len(resp.Synthesis),
			resp.TookMs)
	}

	return results, nil
}

func phase4Associations() (int, int, float64, error) {
	fmt.Println("\n=== Phase 4: Association Graph ===")

	body, status, err := getJSON("/api/v1/stats")
	if err != nil {
		return 0, 0, 0, err
	}
	if status != 200 {
		return 0, 0, 0, fmt.Errorf("status %d", status)
	}

	var stats statsResp
	_ = json.Unmarshal(body, &stats)

	memories := stats.Store.TotalMemories
	assocs := stats.Store.TotalAssociations
	ratio := stats.Store.AvgAssocPerMem

	fmt.Printf("  Memories: %d\n", memories)
	fmt.Printf("  Associations: %d\n", assocs)
	fmt.Printf("  Avg per memory: %.1f\n", ratio)

	return memories, assocs, ratio, nil
}

// --- Report ---

func printReport(
	ingested int, ingestTime time.Duration,
	encodeTime time.Duration, encoded int,
	memories int, assocs int, assocRatio float64,
	results []queryResult,
) {
	sep := strings.Repeat("─", 50)

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Mnemonic Benchmark Report")
	fmt.Printf("%s\n\n", sep)

	// Ingestion
	fmt.Printf("  Ingestion:    %d memories in %s", ingested, ingestTime.Round(time.Millisecond))
	if ingested > 0 {
		fmt.Printf(" (avg %dms each)", ingestTime.Milliseconds()/int64(ingested))
	}
	fmt.Println()

	// Encoding
	fmt.Printf("  Encoding:     %d memories in %s", encoded, encodeTime.Round(time.Second))
	if encoded > 0 {
		fmt.Printf(" (avg %s each)", (encodeTime / time.Duration(encoded)).Round(time.Millisecond))
	}
	fmt.Println()

	// Associations
	fmt.Printf("  Associations: %d total (%.1f per memory)\n", assocs, assocRatio)

	fmt.Println()

	// Retrieval summary
	totalPrecision := 0.0
	totalLatency := time.Duration(0)
	queriesOK := 0
	synthOK := 0
	synthHit := 0

	for _, r := range results {
		if r.err == nil {
			queriesOK++
			totalPrecision += r.precision
			totalLatency += r.latency
		}
		if r.synthesisOK {
			synthOK++
		}
		if r.synthesisHit {
			synthHit++
		}
	}

	fmt.Printf("  Retrieval:    %d/%d queries OK\n", queriesOK, len(results))
	if queriesOK > 0 {
		avgPrec := totalPrecision / float64(queriesOK) * 100
		avgLat := totalLatency / time.Duration(queriesOK)
		fmt.Printf("                avg precision: %.0f%%\n", avgPrec)
		fmt.Printf("                avg latency:   %s\n", avgLat.Round(time.Millisecond))
	}
	fmt.Printf("  Synthesis:    %d/%d non-empty, %d/%d on-topic\n",
		synthOK, len(results), synthHit, len(results))

	fmt.Printf("\n%s\n", sep)

	// Per-query results
	for i, r := range results {
		status := "PASS"
		if r.err != nil {
			status = "FAIL"
		} else if r.precision < 0.5 {
			status = "WEAK"
		}
		fmt.Printf("  Q%d  %-4s  %-28s %4.0f%%\n", i+1, status, r.query.description, r.precision*100)
	}

	fmt.Printf("%s\n", sep)
}

func main() {
	fmt.Println("Mnemonic Benchmark")
	fmt.Println("==================")

	// Check server is running
	_, status, err := getJSON("/api/v1/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot reach server at %s: %v\n", baseURL, err)
		fmt.Fprintln(os.Stderr, "Start the daemon first: mnemonic start")
		os.Exit(1)
	}
	if status != 200 {
		fmt.Fprintf(os.Stderr, "Server unhealthy (status %d)\n", status)
		os.Exit(1)
	}
	fmt.Println("Server is healthy.")

	// Phase 1: Ingest
	ingested, ingestTime, err := phase1Ingest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ingestion failed: %v\n", err)
		os.Exit(1)
	}

	// Phase 2: Wait for encoding
	encodeTime, encoded, err := phase2WaitEncoding(ingested)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Encoding warning: %v\n", err)
		// Continue anyway — partial results are still useful
	}

	// Phase 3: Retrieval + Synthesis
	results, err := phase3Retrieval()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Retrieval failed: %v\n", err)
		os.Exit(1)
	}

	// Phase 4: Association graph
	memories, assocs, ratio, err := phase4Associations()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Association check failed: %v\n", err)
	}

	// Report
	printReport(ingested, ingestTime, encodeTime, encoded, memories, assocs, ratio, results)
}
