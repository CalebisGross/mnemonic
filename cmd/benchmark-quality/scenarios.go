package main

import (
	"fmt"
	"math"
	"time"

	"github.com/appsprout/mnemonic/internal/store"
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
	return []scenario{
		debuggingScenario(),
		architectureScenario(),
		learningScenario(),
	}
}

func debuggingScenario() scenario {
	now := time.Now()
	const dims = 64

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
				Embedding: syntheticEmbedding(32+i, dims, 0.05), // Noise in different region.
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
		{Query: "What regressions have we seen", ExpectedIDs: []string{"dbg-4"}},
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
	const dims = 64

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
				Embedding: syntheticEmbedding(32+i, dims, 0.05),
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
		{Query: "What architecture decisions have we made", ExpectedIDs: []string{"arch-1", "arch-2", "arch-3", "arch-4", "arch-5"}},
		{Query: "What were the tradeoffs", ExpectedIDs: []string{"arch-2", "arch-4", "arch-6"}},
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
	const dims = 64

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
				Embedding: syntheticEmbedding(32+i, dims, 0.05),
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
