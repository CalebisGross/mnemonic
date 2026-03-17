package main

import (
	"fmt"
	"math"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

// syntheticEmbedding creates a deterministic embedding vector.
// dimension selects the primary axis, jitter adds small variation.
func syntheticEmbedding(dimension int, dims int, jitter float64) []float32 {
	emb := make([]float32, dims)
	emb[dimension%dims] = 1.0
	// Add small values to adjacent dimensions for variation.
	if jitter > 0 {
		for i := range emb {
			if i != dimension%dims {
				emb[i] = float32(jitter * math.Sin(float64(i+dimension)))
			}
		}
	}
	// Normalize.
	var norm float64
	for _, v := range emb {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range emb {
			emb[i] = float32(float64(emb[i]) / norm)
		}
	}
	return emb
}

func allScenarios() []scenario {
	scenarios := []scenario{
		debuggingScenario(),
		architectureScenario(),
		learningScenario(),
		investigationScenario(),
		needleScenario(),
		associativeRecallScenario(),
	}
	// Recompute all memory embeddings using bowEmbedding so they live in the
	// same vector space as query embeddings. The original syntheticEmbedding
	// calls produced axis-aligned vectors in an arbitrary dimension, making
	// cosine similarity with bowEmbedding queries meaningless.
	for i := range scenarios {
		for j := range scenarios[i].Memories {
			m := &scenarios[i].Memories[j].Memory
			m.Embedding = bowEmbedding(m.Summary + " " + m.Content)
		}
	}
	return scenarios
}

func debuggingScenario() scenario {
	now := time.Now()
	const dims = 128

	// Signal memories — debugging-related.
	signal := []labeledMemory{
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-1", Summary: "Nil pointer dereference in auth middleware when token is empty",
			Content:   "Stack trace showed nil pointer in auth.go line 42. The JWT token was nil because the Authorization header was missing entirely. Added a nil check before accessing token.Claims.",
			Concepts:  []string{"nil pointer", "auth", "middleware", "JWT", "debugging"},
			Embedding: syntheticEmbedding(0, dims, 0.1), Salience: 0.7, State: "active",
			Timestamp: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-2", Summary: "Root cause: missing nil check on JWT claims before type assertion",
			Content:   "The real root cause was a type assertion on nil claims. The fix was to check if token != nil && token.Claims != nil before the type assertion. This pattern appears in 3 other middleware handlers too.",
			Concepts:  []string{"root cause", "nil pointer", "type assertion", "JWT", "fix"},
			Embedding: syntheticEmbedding(1, dims, 0.1), Salience: 0.75, State: "active",
			Timestamp: now.Add(-90 * time.Minute), CreatedAt: now.Add(-90 * time.Minute), UpdatedAt: now.Add(-90 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-3", Summary: "Auth crash fix: guard clause for nil token before claims access",
			Content:   "Added guard clause: if token == nil { return unauthorized }. Applied same pattern to refreshToken and validateSession handlers. All three had the same vulnerability.",
			Concepts:  []string{"auth", "fix", "guard clause", "nil pointer", "security"},
			Embedding: syntheticEmbedding(2, dims, 0.1), Salience: 0.8, State: "active",
			Timestamp: now.Add(-60 * time.Minute), CreatedAt: now.Add(-60 * time.Minute), UpdatedAt: now.Add(-60 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-4", Summary: "Regression: auth middleware now rejects valid tokens with empty subject field",
			Content:   "After the nil pointer fix, discovered that some service-to-service tokens have empty Subject fields. The new validation was too strict. Relaxed the check to only require non-nil token and valid claims type.",
			Concepts:  []string{"regression", "auth", "service tokens", "validation"},
			Embedding: syntheticEmbedding(3, dims, 0.1), Salience: 0.65, State: "active",
			Timestamp: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-5", Summary: "Added test coverage for auth middleware nil token edge cases",
			Content:   "Wrote table-driven tests covering: nil token, nil claims, empty subject, expired token, and valid token. All pass. The regression is fixed and won't recur.",
			Concepts:  []string{"testing", "auth", "middleware", "table-driven tests", "coverage"},
			Embedding: syntheticEmbedding(4, dims, 0.1), Salience: 0.6, State: "active",
			Timestamp: now.Add(-15 * time.Minute), CreatedAt: now.Add(-15 * time.Minute), UpdatedAt: now.Add(-15 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-6", Summary: "Panic recovery middleware catches nil pointer panics but loses stack trace",
			Content:   "The panic recovery middleware was catching panics but only logging the recovered value, not the stack trace. Added debug.Stack() to capture full trace on recovery.",
			Concepts:  []string{"panic", "recovery", "middleware", "stack trace", "debugging"},
			Embedding: syntheticEmbedding(5, dims, 0.12), Salience: 0.55, State: "active",
			Timestamp: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-7", Summary: "Database connection pool exhaustion during load test",
			Content:   "Under 500 concurrent requests, the connection pool (max 25) was exhausted. Root cause: a query in the reporting handler wasn't closing rows. Added defer rows.Close() and increased pool to 50.",
			Concepts:  []string{"database", "connection pool", "load test", "performance", "debugging"},
			Embedding: syntheticEmbedding(6, dims, 0.12), Salience: 0.6, State: "active",
			Timestamp: now.Add(-4 * time.Hour), CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "dbg-8", Summary: "Race condition in session cache: concurrent map write panic",
			Content:   "Production panic: concurrent map writes in session cache. Switched from map to sync.Map for the hot path. The session cache is read-heavy so sync.Map is appropriate here.",
			Concepts:  []string{"race condition", "concurrency", "session", "sync.Map", "debugging"},
			Embedding: syntheticEmbedding(7, dims, 0.12), Salience: 0.7, State: "active",
			Timestamp: now.Add(-5 * time.Hour), CreatedAt: now.Add(-5 * time.Hour), UpdatedAt: now.Add(-5 * time.Hour),
		}},
	}

	// Noise memories — desktop/system noise.
	noise := make([]labeledMemory, 12)
	noiseContents := []struct{ summary, content string }{
		{"Chrome opened new tab: reddit.com/r/golang", "Browser activity: navigated to reddit.com/r/golang"},
		{"File manager: browsed ~/Downloads directory", "Nautilus file browser accessed ~/Downloads"},
		{"Clipboard: copied URL https://pkg.go.dev/net/http", "Clipboard paste event"},
		{"node_modules/package-lock.json changed", "File watcher: lockfile updated after npm install"},
		{".DS_Store modified in project root", "Filesystem metadata file changed"},
		{"Chrome LevelDB compaction in Default/Local Storage", "Chrome internal database maintenance"},
		{"GNOME dconf: changed desktop wallpaper setting", "Desktop settings write to dconf database"},
		{"Terminal: ran 'ls -la' in home directory", "Shell command execution observed"},
		{"Terminal: ran 'clear' command", "Terminal screen cleared"},
		{"PipeWire: audio output switched to headphones", "Audio subsystem configuration change"},
		{"Trash: moved old-notes.txt to trash", "File deletion via desktop trash"},
		{"File watcher: .git/index modified", "Git index updated after staging"},
	}
	for i, nc := range noiseContents {
		noise[i] = labeledMemory{
			Label: "noise",
			Memory: store.Memory{
				ID:        fmt.Sprintf("noise-dbg-%d", i+1),
				Summary:   nc.summary,
				Content:   nc.content,
				Concepts:  []string{"system", "filesystem"},
				Embedding: syntheticEmbedding(32+i, dims, 0.05), // Debugging noise region.
				Salience:  0.3 + float32(i%3)*0.05,
				State:     "active",
				Timestamp: now.Add(-time.Duration(i*20) * time.Minute),
				CreatedAt: now.Add(-time.Duration(i*20) * time.Minute),
				UpdatedAt: now.Add(-time.Duration(i*20) * time.Minute),
			},
		}
	}

	allMems := append(signal, noise...)

	// Associations between signal memories.
	assocs := []store.Association{
		{SourceID: "dbg-1", TargetID: "dbg-2", Strength: 0.9, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "dbg-2", TargetID: "dbg-3", Strength: 0.85, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "dbg-3", TargetID: "dbg-4", Strength: 0.7, RelationType: "temporal", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "dbg-4", TargetID: "dbg-5", Strength: 0.75, RelationType: "reinforces", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "dbg-1", TargetID: "dbg-6", Strength: 0.5, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "dbg-7", TargetID: "dbg-8", Strength: 0.4, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
	}

	queries := []benchmarkQuery{
		{Query: "nil pointer bug in auth", ExpectedIDs: []string{"dbg-1", "dbg-2", "dbg-3"}},
		{Query: "How did we fix the auth crash", ExpectedIDs: []string{"dbg-2", "dbg-3", "dbg-5"}},
		{Query: "What regression issues have we seen", ExpectedIDs: []string{"dbg-4"}},
	}

	return scenario{
		Name:         "Debugging Session",
		Memories:     allMems,
		Associations: assocs,
		Queries:      queries,
	}
}

func architectureScenario() scenario {
	now := time.Now()
	const dims = 128

	signal := []labeledMemory{
		{Label: "signal", Memory: store.Memory{
			ID: "arch-1", Summary: "Chose SQLite over Postgres because no server dependency needed",
			Content:   "Decision: SQLite for the data store. Postgres would give us better concurrency but requires a server process. Since mnemonic is local-first and single-user, SQLite with WAL mode is sufficient and eliminates the deployment complexity.",
			Concepts:  []string{"SQLite", "Postgres", "architecture", "decision", "database"},
			Embedding: syntheticEmbedding(8, dims, 0.1), Salience: 0.8, State: "active",
			Timestamp: now.Add(-6 * time.Hour), CreatedAt: now.Add(-6 * time.Hour), UpdatedAt: now.Add(-6 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-2", Summary: "Decided on event bus architecture over direct agent calls",
			Content:   "Agents communicate via event bus, not direct function calls. This decouples them so we can add/remove agents without modifying others. The tradeoff is slightly more complex debugging since events are asynchronous.",
			Concepts:  []string{"event bus", "architecture", "agents", "decoupling", "decision"},
			Embedding: syntheticEmbedding(9, dims, 0.1), Salience: 0.75, State: "active",
			Timestamp: now.Add(-5 * time.Hour), CreatedAt: now.Add(-5 * time.Hour), UpdatedAt: now.Add(-5 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-3", Summary: "8 cognitive agents for separation of concerns: perception, encoding, retrieval, consolidation, episoding, metacognition, dreaming, abstraction",
			Content:   "Settled on 8 specialized agents plus an orchestrator. Each maps to a cognitive function. Considered fewer (3-4 monolithic agents) but the fine-grained split makes testing easier and maps better to the memory science literature.",
			Concepts:  []string{"agents", "architecture", "cognitive", "separation of concerns", "decision"},
			Embedding: syntheticEmbedding(10, dims, 0.1), Salience: 0.7, State: "active",
			Timestamp: now.Add(-4 * time.Hour), CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-4", Summary: "Considered event sourcing but chose CRUD with temporal metadata",
			Content:   "Event sourcing would give us full history replay but adds significant complexity. Instead, we store memories with timestamps, access counts, and state transitions. We can reconstruct timelines from this metadata without a full event log.",
			Concepts:  []string{"event sourcing", "CRUD", "architecture", "tradeoff", "decision"},
			Embedding: syntheticEmbedding(11, dims, 0.1), Salience: 0.65, State: "active",
			Timestamp: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-5", Summary: "FTS5 for full-text search instead of external Elasticsearch",
			Content:   "SQLite FTS5 gives us BM25-ranked full-text search without an external service. Combined with our in-memory embedding index for semantic search. Two retrieval paths that get merged with configurable weights.",
			Concepts:  []string{"FTS5", "search", "BM25", "architecture", "decision"},
			Embedding: syntheticEmbedding(12, dims, 0.1), Salience: 0.7, State: "active",
			Timestamp: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-6", Summary: "Local LLM via LM Studio for air-gapped semantic processing",
			Content:   "All LLM operations go through a local model running in LM Studio. No cloud API calls. This means encoding quality depends on the local model, but we get full privacy and offline operation.",
			Concepts:  []string{"LLM", "local", "air-gapped", "LM Studio", "architecture"},
			Embedding: syntheticEmbedding(13, dims, 0.1), Salience: 0.65, State: "active",
			Timestamp: now.Add(-90 * time.Minute), CreatedAt: now.Add(-90 * time.Minute), UpdatedAt: now.Add(-90 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-7", Summary: "Spread activation for associative retrieval with 3-hop max",
			Content:   "Retrieval uses spread activation across the association graph. Activation decays exponentially per hop with a 0.7 factor. After testing, 3 hops is the sweet spot — more hops retrieve too much noise.",
			Concepts:  []string{"spread activation", "retrieval", "association", "graph", "architecture"},
			Embedding: syntheticEmbedding(14, dims, 0.1), Salience: 0.7, State: "active",
			Timestamp: now.Add(-60 * time.Minute), CreatedAt: now.Add(-60 * time.Minute), UpdatedAt: now.Add(-60 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "arch-8", Summary: "Config-driven tunables in config.yaml rather than hardcoded values",
			Content:   "All tunable parameters live in config.yaml: decay rates, thresholds, intervals, model settings. This lets users adjust behavior without recompiling. Struct-based config with YAML tags.",
			Concepts:  []string{"config", "YAML", "tunables", "architecture"},
			Embedding: syntheticEmbedding(15, dims, 0.1), Salience: 0.55, State: "active",
			Timestamp: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		}},
	}

	noise := make([]labeledMemory, 12)
	noiseContents := []struct{ summary, content string }{
		{"GNOME dconf: workspace count changed to 4", "Desktop settings write"},
		{"LM Studio: downloaded qwen2.5-coder-7b model", "Model download activity"},
		{"Trash: removed old-backup.tar.gz", "File deletion"},
		{".DS_Store created in internal/agent/", "Filesystem metadata"},
		{"Chrome: visited stackoverflow.com/questions/tagged/go", "Browser navigation"},
		{"Terminal: ran 'git log --oneline' in ~/Projects/mem", "Shell command"},
		{"File watcher: go.sum modified after go mod tidy", "Dependency file change"},
		{"Chrome LevelDB: Default/IndexedDB compaction", "Browser database maintenance"},
		{"PipeWire: microphone input level adjusted", "Audio settings change"},
		{"Nautilus: browsed /usr/share/fonts directory", "File manager activity"},
		{"Terminal: ran 'top' for 3 seconds", "System monitoring command"},
		{"GNOME: screen locked due to idle timeout", "Desktop idle event"},
	}
	for i, nc := range noiseContents {
		noise[i] = labeledMemory{
			Label: "noise",
			Memory: store.Memory{
				ID:        fmt.Sprintf("noise-arch-%d", i+1),
				Summary:   nc.summary,
				Content:   nc.content,
				Concepts:  []string{"system", "desktop"},
				Embedding: syntheticEmbedding(48+i, dims, 0.05), // Architecture noise region.
				Salience:  0.3 + float32(i%3)*0.05,
				State:     "active",
				Timestamp: now.Add(-time.Duration(i*15) * time.Minute),
				CreatedAt: now.Add(-time.Duration(i*15) * time.Minute),
				UpdatedAt: now.Add(-time.Duration(i*15) * time.Minute),
			},
		}
	}

	allMems := append(signal, noise...)

	assocs := []store.Association{
		{SourceID: "arch-1", TargetID: "arch-5", Strength: 0.8, RelationType: "reinforces", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "arch-2", TargetID: "arch-3", Strength: 0.85, RelationType: "part_of", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "arch-3", TargetID: "arch-7", Strength: 0.7, RelationType: "part_of", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "arch-4", TargetID: "arch-1", Strength: 0.6, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "arch-6", TargetID: "arch-7", Strength: 0.5, RelationType: "reinforces", CreatedAt: now, LastActivated: now, ActivationCount: 1},
	}

	queries := []benchmarkQuery{
		{Query: "Why did we choose SQLite", ExpectedIDs: []string{"arch-1", "arch-5"}},
		{Query: "What architecture decision have we made", ExpectedIDs: []string{"arch-1", "arch-2", "arch-3", "arch-4", "arch-5"}},
		{Query: "What were the tradeoff considerations", ExpectedIDs: []string{"arch-2", "arch-4", "arch-6"}},
	}

	return scenario{
		Name:         "Architecture Decision",
		Memories:     allMems,
		Associations: assocs,
		Queries:      queries,
	}
}

func learningScenario() scenario {
	now := time.Now()
	const dims = 128

	signal := []labeledMemory{
		{Label: "signal", Memory: store.Memory{
			ID: "learn-1", Summary: "Go's sql.NullString needed for nullable columns in SQLite",
			Content:   "When scanning nullable TEXT columns from SQLite, you need sql.NullString (or *string). A plain string will panic on NULL values. Found this debugging a crash in the associations query.",
			Concepts:  []string{"Go", "sql.NullString", "SQLite", "nullable", "learning"},
			Embedding: syntheticEmbedding(16, dims, 0.1), Salience: 0.7, State: "active",
			Timestamp: now.Add(-4 * time.Hour), CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-2", Summary: "FTS5 rank function returns negative BM25 scores (lower is better)",
			Content:   "SQLite FTS5's rank column returns negative BM25 scores. More negative = better match. This is counterintuitive. We negate and normalize to 0-1 range for our scoring pipeline.",
			Concepts:  []string{"FTS5", "BM25", "ranking", "SQLite", "learning"},
			Embedding: syntheticEmbedding(17, dims, 0.1), Salience: 0.75, State: "active",
			Timestamp: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-3", Summary: "Spread activation works best with 3 hops max for our graph density",
			Content:   "Tested spread activation with 1-5 hops. At 4+ hops, too many unrelated memories get activated. At 1-2 hops, we miss important transitive connections. 3 hops with 0.7 decay is the sweet spot for graphs with ~100-500 nodes.",
			Concepts:  []string{"spread activation", "hops", "graph", "tuning", "insight"},
			Embedding: syntheticEmbedding(18, dims, 0.1), Salience: 0.8, State: "active",
			Timestamp: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-4", Summary: "Go embed directive requires files to be in or below the package directory",
			Content:   "go:embed can only access files in the same directory or subdirectories. Tried to embed ../config.yaml and got a compile error. Moved the template into internal/web/static/ instead.",
			Concepts:  []string{"Go", "embed", "directive", "filesystem", "learning"},
			Embedding: syntheticEmbedding(19, dims, 0.1), Salience: 0.6, State: "active",
			Timestamp: now.Add(-90 * time.Minute), CreatedAt: now.Add(-90 * time.Minute), UpdatedAt: now.Add(-90 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-5", Summary: "slog.Handler must implement Enabled, Handle, WithAttrs, WithGroup",
			Content:   "Implementing a custom slog.Handler requires all four methods. Forgot WithGroup initially and got a compile error. The interface is stricter than expected since slog is relatively new in Go 1.21+.",
			Concepts:  []string{"Go", "slog", "logging", "interface", "learning"},
			Embedding: syntheticEmbedding(20, dims, 0.1), Salience: 0.55, State: "active",
			Timestamp: now.Add(-60 * time.Minute), CreatedAt: now.Add(-60 * time.Minute), UpdatedAt: now.Add(-60 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-6", Summary: "Cosine similarity of zero vectors returns NaN — must guard against it",
			Content:   "If either embedding is all zeros (failed encoding), cosine similarity returns NaN which propagates through the scoring pipeline. Added a guard: if norm is 0, return similarity 0.",
			Concepts:  []string{"cosine similarity", "embedding", "NaN", "guard", "learning"},
			Embedding: syntheticEmbedding(21, dims, 0.1), Salience: 0.65, State: "active",
			Timestamp: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-7", Summary: "CGO_ENABLED=1 required for mattn/go-sqlite3 — won't link otherwise",
			Content:   "The mattn/go-sqlite3 driver uses cgo. Without CGO_ENABLED=1, you get a linker error. Also need the sqlite_fts5 build tag for full-text search support. Easy to forget when switching machines.",
			Concepts:  []string{"CGO", "Go", "SQLite", "build", "learning"},
			Embedding: syntheticEmbedding(22, dims, 0.1), Salience: 0.6, State: "active",
			Timestamp: now.Add(-15 * time.Minute), CreatedAt: now.Add(-15 * time.Minute), UpdatedAt: now.Add(-15 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "learn-8", Summary: "D3 force simulation needs alpha decay tuning for stable layouts",
			Content:   "D3's force simulation alpha decays to 0, stopping the simulation. For our graph, alphaDecay of 0.02 (slower) produces more stable layouts than the default 0.0228. Also alphaMin 0.001 prevents premature stop.",
			Concepts:  []string{"D3", "force simulation", "graph", "visualization", "learning"},
			Embedding: syntheticEmbedding(23, dims, 0.1), Salience: 0.55, State: "active",
			Timestamp: now.Add(-10 * time.Minute), CreatedAt: now.Add(-10 * time.Minute), UpdatedAt: now.Add(-10 * time.Minute),
		}},
	}

	noise := make([]labeledMemory, 12)
	noiseContents := []struct{ summary, content string }{
		{"Clipboard: copied https://go.dev/doc/effective_go", "URL clipboard event"},
		{"Terminal: ran 'cd ~/Projects/mem' command", "Directory change"},
		{"Terminal: ran 'clear' command", "Terminal clear"},
		{"Terminal: ran 'ls -la internal/' command", "Directory listing"},
		{"Chrome: opened 5 new tabs from search results", "Browser activity"},
		{"PipeWire: bluetooth headphones connected", "Audio device event"},
		{"File watcher: .git/COMMIT_EDITMSG modified", "Git commit editing"},
		{"GNOME: notification from Slack: new message in #general", "Desktop notification"},
		{"Terminal: ran 'make test' — 42 tests passed", "Test execution"},
		{"Clipboard: copied error message from terminal output", "Clipboard event"},
		{"File watcher: /tmp/go-build cache updated", "Build cache change"},
		{"Chrome: downloaded go1.22.0.linux-amd64.tar.gz", "File download"},
	}
	for i, nc := range noiseContents {
		noise[i] = labeledMemory{
			Label: "noise",
			Memory: store.Memory{
				ID:        fmt.Sprintf("noise-learn-%d", i+1),
				Summary:   nc.summary,
				Content:   nc.content,
				Concepts:  []string{"system", "terminal"},
				Embedding: syntheticEmbedding(64+i, dims, 0.05), // Learning noise region.
				Salience:  0.3 + float32(i%3)*0.05,
				State:     "active",
				Timestamp: now.Add(-time.Duration(i*12) * time.Minute),
				CreatedAt: now.Add(-time.Duration(i*12) * time.Minute),
				UpdatedAt: now.Add(-time.Duration(i*12) * time.Minute),
			},
		}
	}

	allMems := append(signal, noise...)

	assocs := []store.Association{
		{SourceID: "learn-1", TargetID: "learn-2", Strength: 0.7, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "learn-2", TargetID: "learn-3", Strength: 0.5, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "learn-1", TargetID: "learn-7", Strength: 0.6, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "learn-6", TargetID: "learn-3", Strength: 0.5, RelationType: "reinforces", CreatedAt: now, LastActivated: now, ActivationCount: 1},
	}

	queries := []benchmarkQuery{
		{Query: "What did we learn about FTS5", ExpectedIDs: []string{"learn-2"}},
		{Query: "Go gotchas and quirks", ExpectedIDs: []string{"learn-1", "learn-4", "learn-5", "learn-7"}},
		{Query: "What patterns work well for retrieval", ExpectedIDs: []string{"learn-3", "learn-6"}},
	}

	return scenario{
		Name:         "Learning & Insights",
		Memories:     allMems,
		Associations: assocs,
		Queries:      queries,
	}
}

// investigationScenario tests deep spread activation. Signal memories form
// a 6-node causal chain (A->B->C->D->E->F) with lateral branches.
// Queries require 3-4 hop traversal to find all relevant results.
// Differentiates: retrieval.max_hops, retrieval.decay_factor, retrieval.activation_threshold.
func investigationScenario() scenario {
	now := time.Now()
	const dims = 128

	// Signal memories forming a deep causal chain about a production incident.
	signal := []labeledMemory{
		{Label: "signal", Memory: store.Memory{
			ID: "inv-1", Summary: "Alert: API latency spike detected in monitoring dashboard",
			Content:   "PagerDuty alert fired for p99 latency exceeding 5s on /api/v1/search endpoint. Dashboard shows gradual degradation starting 2 hours ago.",
			Concepts:  []string{"alert", "latency", "monitoring", "api"},
			Embedding: syntheticEmbedding(0, dims, 0.15), Salience: 0.6, State: "active",
			Timestamp: now.Add(-6 * time.Hour), CreatedAt: now.Add(-6 * time.Hour), UpdatedAt: now.Add(-6 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-2", Summary: "Traced latency to database query in search handler",
			Content:   "Profiling shows 90% of latency in SearchHandler.Query(). The FTS5 query is doing a full table scan instead of using the index. EXPLAIN QUERY PLAN confirms index bypass.",
			Concepts:  []string{"database", "query", "fts5", "performance", "profiling"},
			Embedding: syntheticEmbedding(1, dims, 0.15), Salience: 0.65, State: "active",
			Timestamp: now.Add(-5 * time.Hour), CreatedAt: now.Add(-5 * time.Hour), UpdatedAt: now.Add(-5 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-3", Summary: "FTS5 index corruption caused by direct DELETE without rebuild",
			Content:   "Found the root cause: a cleanup job was running DELETE FROM memories_fts WHERE rowid IN (...) directly. FTS5 requires using the content table for deletes. The index became inconsistent, forcing fallback to full scan.",
			Concepts:  []string{"fts5", "index", "corruption", "root cause", "delete"},
			Embedding: syntheticEmbedding(2, dims, 0.15), Salience: 0.75, State: "active",
			Timestamp: now.Add(-4 * time.Hour), CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-4", Summary: "Fix: rebuilt FTS5 index using INSERT INTO memories_fts(memories_fts) VALUES('rebuild')",
			Content:   "Rebuilt the FTS5 index with the rebuild command. Latency immediately dropped back to normal (p99 < 200ms). Need to fix the cleanup job to prevent recurrence.",
			Concepts:  []string{"fts5", "fix", "rebuild", "index", "performance"},
			Embedding: syntheticEmbedding(3, dims, 0.15), Salience: 0.8, State: "active",
			Timestamp: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-5", Summary: "Prevention: rewrote cleanup job to use content table DELETE pattern",
			Content:   "Rewrote the cleanup job to delete from the content table instead of directly from the FTS5 table. Added a post-cleanup integrity check that verifies FTS5 index consistency.",
			Concepts:  []string{"cleanup", "prevention", "fts5", "integrity", "fix"},
			Embedding: syntheticEmbedding(4, dims, 0.15), Salience: 0.7, State: "active",
			Timestamp: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-6", Summary: "Post-mortem: added FTS5 index health check to monitoring suite",
			Content:   "Added automated FTS5 integrity check to the health monitoring suite. Runs every hour, alerts if index divergence detected. Also added a runbook for the rebuild procedure.",
			Concepts:  []string{"monitoring", "health check", "post-mortem", "fts5", "automation"},
			Embedding: syntheticEmbedding(5, dims, 0.15), Salience: 0.65, State: "active",
			Timestamp: now.Add(-1 * time.Hour), CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour),
		}},
		// Lateral branch: related but not in the main chain.
		{Label: "signal", Memory: store.Memory{
			ID: "inv-7", Summary: "Similar issue last month: embedding index got stale after bulk import",
			Content:   "Recalled a similar incident where the embedding similarity index became stale after a bulk import. The pattern is the same: bypassing the normal write path corrupts secondary indexes.",
			Concepts:  []string{"embedding", "index", "bulk import", "pattern", "incident"},
			Embedding: syntheticEmbedding(6, dims, 0.15), Salience: 0.55, State: "active",
			Timestamp: now.Add(-90 * time.Minute), CreatedAt: now.Add(-90 * time.Minute), UpdatedAt: now.Add(-90 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-8", Summary: "General principle: always use the ORM/content table path for writes, never raw SQL on derived tables",
			Content:   "This is the second time bypassing the content table path caused index corruption. Establishing a rule: all writes must go through the store interface, never direct SQL on FTS5 or embedding tables.",
			Concepts:  []string{"principle", "store interface", "architecture", "rule", "decision"},
			Embedding: syntheticEmbedding(7, dims, 0.15), Salience: 0.7, State: "active",
			Timestamp: now.Add(-45 * time.Minute), CreatedAt: now.Add(-45 * time.Minute), UpdatedAt: now.Add(-45 * time.Minute),
		}},
		// Another lateral: deployment timeline context.
		{Label: "signal", Memory: store.Memory{
			ID: "inv-9", Summary: "The cleanup job was deployed 3 days ago in release v0.7.2",
			Content:   "Git blame shows the problematic cleanup job was added in commit abc123 as part of v0.7.2. It was a well-intentioned optimization to reduce DB size but skipped the content table pattern.",
			Concepts:  []string{"deployment", "release", "git", "timeline", "investigation"},
			Embedding: syntheticEmbedding(8, dims, 0.15), Salience: 0.5, State: "active",
			Timestamp: now.Add(-150 * time.Minute), CreatedAt: now.Add(-150 * time.Minute), UpdatedAt: now.Add(-150 * time.Minute),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "inv-10", Summary: "Lesson: need pre-deploy check that validates FTS5 writes go through content table",
			Content:   "Adding a CI lint rule that scans for direct INSERT/DELETE on *_fts tables. Any FTS5 modification must go through the store layer. This would have caught the bug before deploy.",
			Concepts:  []string{"ci", "lint", "prevention", "fts5", "learning"},
			Embedding: syntheticEmbedding(9, dims, 0.15), Salience: 0.6, State: "active",
			Timestamp: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		}},
	}

	// Noise memories.
	noise := make([]labeledMemory, 12)
	noiseContents := []struct{ summary, content string }{
		{"Terminal: ran 'htop' for system monitoring", "Process monitoring activity"},
		{"Chrome: browsed SQLite documentation page", "Browser navigation"},
		{"File watcher: go.sum changed after dependency update", "Dependency file change"},
		{"Clipboard: copied SQL query from chat", "Clipboard event"},
		{"Terminal: ran 'git status' in project directory", "Git status check"},
		{"GNOME: screen brightness adjusted", "Display settings change"},
		{"Terminal: ran 'df -h' to check disk space", "Disk space check"},
		{"Chrome: visited GitHub issues page", "Browser navigation"},
		{"PipeWire: audio output volume changed", "Audio settings adjustment"},
		{"Terminal: ran 'make test' in project root", "Test execution"},
		{"File watcher: /tmp/benchmark-* directory created", "Temp file creation"},
		{"Terminal: ran 'tail -f /var/log/syslog'", "Log monitoring"},
	}
	for i, nc := range noiseContents {
		noise[i] = labeledMemory{
			Label: "noise",
			Memory: store.Memory{
				ID:        fmt.Sprintf("noise-inv-%d", i+1),
				Summary:   nc.summary,
				Content:   nc.content,
				Concepts:  []string{"system", "terminal"},
				Embedding: syntheticEmbedding(80+i, dims, 0.05), // Investigation noise region.
				Salience:  0.3 + float32(i%3)*0.05,
				State:     "active",
				Timestamp: now.Add(-time.Duration(i*18) * time.Minute),
				CreatedAt: now.Add(-time.Duration(i*18) * time.Minute),
				UpdatedAt: now.Add(-time.Duration(i*18) * time.Minute),
			},
		}
	}

	allMems := append(signal, noise...)

	// Deep causal chain: 1->2->3->4->5->6 (6 hops end-to-end).
	// Plus lateral branches: 3->7, 7->8, 1->9, 5->10.
	assocs := []store.Association{
		// Main chain.
		{SourceID: "inv-1", TargetID: "inv-2", Strength: 0.9, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-2", TargetID: "inv-3", Strength: 0.85, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-3", TargetID: "inv-4", Strength: 0.9, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-4", TargetID: "inv-5", Strength: 0.8, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-5", TargetID: "inv-6", Strength: 0.75, RelationType: "temporal", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		// Lateral branches.
		{SourceID: "inv-3", TargetID: "inv-7", Strength: 0.6, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-7", TargetID: "inv-8", Strength: 0.7, RelationType: "reinforces", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-1", TargetID: "inv-9", Strength: 0.5, RelationType: "temporal", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "inv-5", TargetID: "inv-10", Strength: 0.65, RelationType: "reinforces", CreatedAt: now, LastActivated: now, ActivationCount: 1},
	}

	// Queries designed to require multi-hop traversal.
	queries := []benchmarkQuery{
		// Starting from the alert, should follow chain to find the fix (3 hops away).
		{Query: "How did we fix the latency issue", ExpectedIDs: []string{"inv-3", "inv-4", "inv-5"}},
		// Starting from the post-mortem, should traverse back to root cause (4+ hops).
		{Query: "What caused the FTS5 index corruption", ExpectedIDs: []string{"inv-2", "inv-3", "inv-9"}},
		// Should find the principle (inv-8) which is only reachable via inv-3->inv-7->inv-8 (3 hops from many entry points).
		{Query: "What principles did we establish from this incident", ExpectedIDs: []string{"inv-8", "inv-10"}},
		// Cross-chain: connect the two lateral branches.
		{Query: "What similar incidents have we seen before", ExpectedIDs: []string{"inv-7"}},
	}

	return scenario{
		Name:         "Deep Graph Investigation",
		Memories:     allMems,
		Associations: assocs,
		Queries:      queries,
	}
}

// needleScenario tests noise suppression under high noise pressure.
// Only 4 signal memories buried in 25 noise with salience close to signal.
// Differentiates: consolidation.fade_threshold, consolidation.archive_threshold,
// consolidation.decay_rate, bench.decay_per_cycle.
func needleScenario() scenario {
	now := time.Now()
	const dims = 128

	// 4 signal memories — clear decisions/insights with moderate salience.
	signal := []labeledMemory{
		{Label: "signal", Memory: store.Memory{
			ID: "needle-1", Summary: "Decision: use WAL mode for SQLite to improve concurrent read performance",
			Content:   "Switched SQLite to WAL journal mode. This allows concurrent readers while a write is in progress. Critical for the daemon where the API serves reads while agents write.",
			Concepts:  []string{"sqlite", "wal", "performance", "decision", "concurrency"},
			Embedding: syntheticEmbedding(10, dims, 0.12), Salience: 0.65, State: "active",
			Timestamp: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "needle-2", Summary: "Insight: slog structured logging is significantly better than fmt.Printf for debugging agents",
			Content:   "After switching all agent logging to slog, debugging became much easier. Structured fields like agent=encoding, memory_id=abc123 make it possible to filter and trace individual memory lifecycles.",
			Concepts:  []string{"slog", "logging", "debugging", "insight", "agents"},
			Embedding: syntheticEmbedding(11, dims, 0.12), Salience: 0.6, State: "active",
			Timestamp: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "needle-3", Summary: "Error: nil pointer in consolidation agent when processing memories with no embedding",
			Content:   "Consolidation agent panicked on memories that had empty embeddings (encoding failed). Added nil check before cosine similarity calculation. Need to ensure all code paths handle missing embeddings gracefully.",
			Concepts:  []string{"nil pointer", "consolidation", "embedding", "error", "fix"},
			Embedding: syntheticEmbedding(12, dims, 0.12), Salience: 0.7, State: "active",
			Timestamp: now.Add(-1 * time.Hour), CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "needle-4", Summary: "Learning: Go's context.WithTimeout doesn't cancel if you forget to defer cancel()",
			Content:   "Found a goroutine leak in the retrieval agent. context.WithTimeout returns a cancel func that MUST be deferred. Without it, the internal timer goroutine leaks. Added go vet check for this.",
			Concepts:  []string{"go", "context", "goroutine", "leak", "learning"},
			Embedding: syntheticEmbedding(13, dims, 0.12), Salience: 0.6, State: "active",
			Timestamp: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		}},
	}

	// 25 noise memories with salience dangerously close to signal (0.35-0.55).
	noise := make([]labeledMemory, 25)
	noiseContents := []struct {
		summary, content string
		salience         float32
	}{
		{"Chrome: opened 3 tabs from Google search results", "Browser activity: multiple tabs opened", 0.35},
		{"File watcher: node_modules/.cache updated", "Build cache write", 0.38},
		{"Terminal: ran 'ls -la internal/agent/' to list files", "Directory listing in agent package", 0.42},
		{"Clipboard: copied import path github.com/appsprout-dev/mnemonic", "Go import path copied", 0.45},
		{"Chrome: read Go blog post about generics", "Browser: reading technical content", 0.48},
		{"File watcher: .git/FETCH_HEAD modified after git fetch", "Git metadata update", 0.35},
		{"Terminal: ran 'go doc context.WithTimeout'", "Go documentation lookup", 0.50},
		{"GNOME: notification from Signal: 2 new messages", "Desktop notification from messaging app", 0.36},
		{"File watcher: internal/store/sqlite/store.go saved", "File save event in editor", 0.44},
		{"Terminal: ran 'curl localhost:9999/api/v1/health'", "Health check API call", 0.47},
		{"Chrome: visited pkg.go.dev/database/sql", "Go package documentation", 0.49},
		{"PipeWire: switched output to speakers", "Audio output change", 0.34},
		{"File watcher: bin/mnemonic rebuilt", "Binary rebuild event", 0.43},
		{"Terminal: ran 'systemctl --user status mnemonic'", "Service status check", 0.46},
		{"Clipboard: copied error message 'database is locked'", "Error message clipboard event", 0.51},
		{"Chrome: browsed SQLite WAL documentation", "Browser: SQLite docs", 0.52},
		{"File watcher: config.yaml modified", "Config file change", 0.48},
		{"Terminal: ran 'git diff HEAD~1' to review changes", "Git diff command", 0.40},
		{"GNOME: workspace switched from 1 to 2", "Desktop workspace change", 0.33},
		{"File watcher: internal/agent/consolidation/agent_test.go saved", "Test file save", 0.45},
		{"Terminal: ran 'make build' successfully", "Build command execution", 0.47},
		{"Chrome: opened mnemonic GitHub issues page", "Browser: project management", 0.44},
		{"Clipboard: copied function signature from agent.go", "Code snippet clipboard event", 0.42},
		{"Terminal: ran 'go test -v ./internal/store/...' — 18 tests passed", "Test execution", 0.50},
		{"File watcher: memory.db-wal grew to 2MB", "WAL file size change", 0.39},
	}
	for i, nc := range noiseContents {
		noise[i] = labeledMemory{
			Label: "noise",
			Memory: store.Memory{
				ID:        fmt.Sprintf("noise-needle-%d", i+1),
				Summary:   nc.summary,
				Content:   nc.content,
				Concepts:  []string{"system", "activity"},
				Embedding: syntheticEmbedding(96+i, dims, 0.08), // Needle noise region.
				Salience:  nc.salience,
				State:     "active",
				Timestamp: now.Add(-time.Duration(i*7) * time.Minute),
				CreatedAt: now.Add(-time.Duration(i*7) * time.Minute),
				UpdatedAt: now.Add(-time.Duration(i*7) * time.Minute),
			},
		}
	}

	allMems := append(signal, noise...)

	// Minimal associations — signal memories are loosely related.
	assocs := []store.Association{
		{SourceID: "needle-1", TargetID: "needle-3", Strength: 0.5, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		{SourceID: "needle-3", TargetID: "needle-4", Strength: 0.4, RelationType: "similar", CreatedAt: now, LastActivated: now, ActivationCount: 1},
	}

	queries := []benchmarkQuery{
		{Query: "What decision did we make about the database", ExpectedIDs: []string{"needle-1"}},
		{Query: "What error and bug have we found", ExpectedIDs: []string{"needle-3", "needle-4"}},
		{Query: "What did we learn about Go and logging", ExpectedIDs: []string{"needle-2", "needle-4"}},
	}

	return scenario{
		Name:         "Needle in Haystack",
		Memories:     allMems,
		Associations: assocs,
		Queries:      queries,
	}
}

// associativeRecallScenario tests the core value proposition of spread activation.
// Signal memories form causal chains where intermediate/terminal nodes have ZERO
// keyword overlap with the query. Only graph traversal can discover them.
//
// This is the scenario that should clearly differentiate Mnemonic (full) from all
// baselines. If it doesn't, spread activation isn't working or isn't useful.
//
// Design principle: query keywords match ONLY the entry-point memory.
// All downstream memories in the chain use completely different vocabulary.
func associativeRecallScenario() scenario {
	now := time.Now()
	const dims = 128

	// --- Chain 1: Auth outage root cause analysis ---
	// Query matches ar-1 (auth, errors). The actual cause (ar-2: Redis pool)
	// and root cause (ar-3: unclosed connections) use entirely different vocab.
	signal := []labeledMemory{
		{Label: "signal", Memory: store.Memory{
			ID: "ar-1", Summary: "Authentication service returning 503 errors during peak traffic hours",
			Content:   "The auth service started returning 503 errors at 2pm during peak load. Users unable to log in. Dashboard shows 40% error rate on the /auth/token endpoint. Service health checks are passing but requests are timing out.",
			Concepts:  []string{"authentication", "error", "503", "traffic", "timeout"},
			Embedding: syntheticEmbedding(0, dims, 0.15), Salience: 0.75, State: "active",
			Timestamp: now.Add(-6 * time.Hour), CreatedAt: now.Add(-6 * time.Hour), UpdatedAt: now.Add(-6 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "ar-2", Summary: "Redis connection pool fully exhausted — all 25 slots occupied by stale handles",
			Content:   "Investigated the backing store and found the Redis connection pool completely drained. netstat shows 25 ESTABLISHED connections to port 6379, none being recycled. New connection attempts block indefinitely until the 30s dial timeout.",
			Concepts:  []string{"redis", "connection", "pool", "stale", "exhausted"},
			Embedding: syntheticEmbedding(59, dims, 0.15), Salience: 0.7, State: "active",
			Timestamp: now.Add(-5 * time.Hour), CreatedAt: now.Add(-5 * time.Hour), UpdatedAt: now.Add(-5 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "ar-3", Summary: "Unclosed connections in token validation — conn.Close() missing from error return path",
			Content:   "Root cause found in validateToken(). When HMAC verification fails, the function returns early but never calls conn.Close(). Under sustained invalid-token traffic, this leaks one connection per failed validation until the pool is starved.",
			Concepts:  []string{"leak", "close", "validation", "HMAC", "return"},
			Embedding: syntheticEmbedding(66, dims, 0.15), Salience: 0.8, State: "active",
			Timestamp: now.Add(-4 * time.Hour), CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "ar-4", Summary: "Applied defer conn.Close() to all Redis call sites and bumped pool ceiling to 50",
			Content:   "Patched all 7 Redis call sites to use defer conn.Close() immediately after acquiring. Raised MaxIdle from 25 to 50 as buffer. Added a connection lifetime limit of 5 minutes to prevent stale handle accumulation.",
			Concepts:  []string{"defer", "pool", "patch", "lifetime", "MaxIdle"},
			Embedding: syntheticEmbedding(92, dims, 0.15), Salience: 0.75, State: "active",
			Timestamp: now.Add(-3 * time.Hour), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
		}},

		// --- Chain 2: Deployment caused performance regression ---
		// Query matches ar-5 (deployment, slow). The actual cause (ar-6: missing index)
		// and fix (ar-7: migration) use different vocabulary.
		{Label: "signal", Memory: store.Memory{
			ID: "ar-5", Summary: "Dashboard page load times tripled after Tuesday's release to production",
			Content:   "After deploying v2.8.0 on Tuesday, the main dashboard went from 800ms to 2.4s load time. Users in the EU region are most affected. The deployment included 47 commits across 12 PRs.",
			Concepts:  []string{"deployment", "slow", "dashboard", "regression", "release"},
			Embedding: syntheticEmbedding(17, dims, 0.15), Salience: 0.7, State: "active",
			Timestamp: now.Add(-8 * time.Hour), CreatedAt: now.Add(-8 * time.Hour), UpdatedAt: now.Add(-8 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "ar-6", Summary: "New analytics aggregation query running without an index — sequential scan on 4M row table",
			Content:   "EXPLAIN ANALYZE revealed the new monthly_summary view does a sequential scan on the events table (4.2M rows). The WHERE clause filters on (org_id, created_at) but no composite index exists for those columns. Cost estimate: 847,000 vs 1,200 with index.",
			Concepts:  []string{"EXPLAIN", "sequential", "scan", "composite", "index", "cost"},
			Embedding: syntheticEmbedding(78, dims, 0.15), Salience: 0.8, State: "active",
			Timestamp: now.Add(-7 * time.Hour), CreatedAt: now.Add(-7 * time.Hour), UpdatedAt: now.Add(-7 * time.Hour),
		}},
		{Label: "signal", Memory: store.Memory{
			ID: "ar-7", Summary: "Created migration 042 with composite index on (org_id, created_at DESC) and CONCURRENTLY flag",
			Content:   "Wrote migration file 042_add_events_org_created_idx.sql. Uses CREATE INDEX CONCURRENTLY to avoid locking the table. Verified with EXPLAIN that the planner now uses an Index Scan with estimated cost 1,200. Dashboard load times back to 750ms.",
			Concepts:  []string{"migration", "CONCURRENTLY", "planner", "scan", "750ms"},
			Embedding: syntheticEmbedding(23, dims, 0.15), Salience: 0.75, State: "active",
			Timestamp: now.Add(-6 * time.Hour), CreatedAt: now.Add(-6 * time.Hour), UpdatedAt: now.Add(-6 * time.Hour),
		}},

		// --- Chain 3: Cross-domain business impact ---
		// Connected to chain 1 but uses business/product vocabulary.
		{Label: "signal", Memory: store.Memory{
			ID: "ar-8", Summary: "E-commerce team reported 12% cart abandonment spike correlated with the token validation window",
			Content:   "Product analytics showed a 12% increase in cart abandonment between 2pm and 4pm — exactly matching the auth outage window. Estimated revenue impact: $47K. The checkout flow requires re-authentication which was failing silently.",
			Concepts:  []string{"cart", "abandonment", "revenue", "checkout", "product"},
			Embedding: syntheticEmbedding(56, dims, 0.15), Salience: 0.65, State: "active",
			Timestamp: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}},
	}

	// Noise memories — carefully chosen to NOT share keywords with signal chains.
	noise := make([]labeledMemory, 15)
	noiseContents := []struct{ summary, content string }{
		{"Chrome: visited pkg.go.dev/context documentation", "Browser navigation to Go docs"},
		{"Terminal: executed 'df -h' to check available disk space", "Disk usage monitoring command"},
		{"File watcher: .git/objects directory modified", "Git internal object creation"},
		{"Clipboard: copied JSON payload from Postman response", "API testing clipboard activity"},
		{"GNOME: display brightness adjusted via slider", "Desktop brightness control event"},
		{"Terminal: ran 'docker logs api-gateway --tail 50'", "Container log inspection"},
		{"File watcher: /var/log/syslog rotated by logrotate", "System log rotation event"},
		{"Chrome: opened three Stack Overflow tabs about goroutine patterns", "Browser research activity"},
		{"Terminal: executed 'kubectl get pods -n staging'", "Kubernetes cluster inspection"},
		{"PipeWire: USB microphone connected and configured", "Audio hardware plug event"},
		{"File watcher: node_modules/.package-lock.json updated", "NPM lockfile modification"},
		{"Terminal: ran 'htop' for 5 seconds then quit", "System resource monitoring"},
		{"Clipboard: copied function signature from source file", "Code snippet clipboard event"},
		{"GNOME: workspace switched from workspace 2 to workspace 3", "Virtual desktop navigation"},
		{"File watcher: ~/.local/share/Trash/files updated", "Trash directory modification"},
	}
	for i, nc := range noiseContents {
		noise[i] = labeledMemory{
			Label: "noise",
			Memory: store.Memory{
				ID:        fmt.Sprintf("noise-ar-%d", i+1),
				Summary:   nc.summary,
				Content:   nc.content,
				Concepts:  []string{"system", "activity"},
				Embedding: syntheticEmbedding(100+i, dims, 0.05),
				Salience:  0.3 + float32(i%4)*0.05,
				State:     "active",
				Timestamp: now.Add(-time.Duration(i*15) * time.Minute),
				CreatedAt: now.Add(-time.Duration(i*15) * time.Minute),
				UpdatedAt: now.Add(-time.Duration(i*15) * time.Minute),
			},
		}
	}

	allMems := append(signal, noise...)

	// Associations: causal chains where traversal is required.
	assocs := []store.Association{
		// Chain 1: auth outage → Redis pool → unclosed conn → fix
		{SourceID: "ar-1", TargetID: "ar-2", Strength: 0.9, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 2},
		{SourceID: "ar-2", TargetID: "ar-3", Strength: 0.85, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 2},
		{SourceID: "ar-3", TargetID: "ar-4", Strength: 0.8, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		// Chain 2: deployment regression → missing index → migration fix
		{SourceID: "ar-5", TargetID: "ar-6", Strength: 0.85, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 2},
		{SourceID: "ar-6", TargetID: "ar-7", Strength: 0.9, RelationType: "caused_by", CreatedAt: now, LastActivated: now, ActivationCount: 1},
		// Cross-chain: auth outage → business impact
		{SourceID: "ar-1", TargetID: "ar-8", Strength: 0.7, RelationType: "temporal", CreatedAt: now, LastActivated: now, ActivationCount: 1},
	}

	// Queries designed so ONLY spread activation can find the expected answers.
	// Entry point keywords match one memory; the expected answers are only reachable via graph.
	queries := []benchmarkQuery{
		// Entry: ar-1 matches "authentication errors". Expected: ar-2 (Redis pool) and ar-3 (root cause).
		// ar-2/ar-3 have NO auth-related keywords — only reachable via ar-1→ar-2→ar-3.
		{
			Query:       "What caused the authentication errors",
			ExpectedIDs: []string{"ar-1", "ar-2", "ar-3"},
		},
		// Entry: ar-5 matches "deployment slow". Expected: ar-6 (missing index) and ar-7 (migration).
		// ar-6 talks about EXPLAIN/sequential scan, ar-7 about CREATE INDEX — no "deployment" keywords.
		{
			Query:       "Why did the deployment make things slow",
			ExpectedIDs: []string{"ar-5", "ar-6", "ar-7"},
		},
		// Entry: ar-1 matches "auth". Expected: ar-8 (business impact).
		// ar-8 talks about cart abandonment and revenue — completely different domain.
		{
			Query:       "What was the business impact of the auth incident",
			ExpectedIDs: []string{"ar-1", "ar-8"},
		},
		// This query tests full chain traversal. Entry: ar-2 matches "Redis pool".
		// Expected: ar-4 (the fix) which is 2 hops away via ar-2→ar-3→ar-4.
		{
			Query:       "How did we fix the Redis connection pool exhaustion",
			ExpectedIDs: []string{"ar-2", "ar-3", "ar-4"},
		},
	}

	return scenario{
		Name:         "Associative Recall",
		Memories:     allMems,
		Associations: assocs,
		Queries:      queries,
	}
}
