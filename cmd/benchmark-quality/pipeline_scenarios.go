package main

import (
	"fmt"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

func allPipelineScenarios() []pipelineScenario {
	return []pipelineScenario{
		fullDayScenario(),
		crossPollinationScenario(),
		noiseStormScenario(),
	}
}

// fullDayScenario simulates a realistic dev work session.
// 30 raw events: coding, debugging, browsing, system noise.
// 8 should survive as signal after the full pipeline.
func fullDayScenario() pipelineScenario {
	base := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	signalIDs := map[string]bool{
		"fd-1":  true, // decided on SQLite for persistence
		"fd-3":  true, // fixed nil pointer in auth middleware
		"fd-5":  true, // implemented retry logic for API calls
		"fd-8":  true, // discovered FTS5 requires compile flag
		"fd-12": true, // chose slog over zerolog for structured logging
		"fd-15": true, // designed memory consolidation algorithm
		"fd-20": true, // fixed race condition in event bus
		"fd-25": true, // wrote integration test for full pipeline
	}

	events := []store.RawMemory{
		// Signal: architectural decision
		{ID: "fd-1", Timestamp: base, Source: "mcp", Type: "decision",
			Content:        "Decided to use SQLite with FTS5 for the persistence layer. PostgreSQL was considered but rejected for air-gapped deployment simplicity. SQLite gives us full-text search, vector similarity, and zero configuration.",
			HeuristicScore: 0.9, InitialSalience: 0.8, Project: "mnemonic", CreatedAt: base},
		// Noise: file browsing
		{ID: "fd-2", Timestamp: base.Add(2 * time.Minute), Source: "filesystem", Type: "file_read",
			Content:        "Read file: /home/user/.config/Code/settings.json",
			HeuristicScore: 0.1, InitialSalience: 0.2, Project: "mnemonic", CreatedAt: base.Add(2 * time.Minute)},
		// Signal: bug fix
		{ID: "fd-3", Timestamp: base.Add(5 * time.Minute), Source: "mcp", Type: "error",
			Content:        "Fixed nil pointer dereference in auth middleware. The session token was being accessed before checking if the user was authenticated. Added guard clause: if session == nil { return ErrUnauthorized }.",
			HeuristicScore: 0.85, InitialSalience: 0.75, Project: "mnemonic", CreatedAt: base.Add(5 * time.Minute)},
		// Noise: terminal noise
		{ID: "fd-4", Timestamp: base.Add(7 * time.Minute), Source: "terminal", Type: "command",
			Content:        "ls -la /tmp/build-artifacts/",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(7 * time.Minute)},
		// Signal: implementation
		{ID: "fd-5", Timestamp: base.Add(10 * time.Minute), Source: "mcp", Type: "implementation",
			Content:        "Implemented exponential backoff retry logic for LLM API calls. Uses jitter to avoid thundering herd. Base delay 1s, max 30s, max retries 3. Falls back to heuristic encoding on permanent failure.",
			HeuristicScore: 0.8, InitialSalience: 0.7, Project: "mnemonic", CreatedAt: base.Add(10 * time.Minute)},
		// Noise: clipboard
		{ID: "fd-6", Timestamp: base.Add(12 * time.Minute), Source: "clipboard", Type: "copy",
			Content:        "https://pkg.go.dev/database/sql",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "mnemonic", CreatedAt: base.Add(12 * time.Minute)},
		// Noise: file system event
		{ID: "fd-7", Timestamp: base.Add(14 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/mem/go.sum",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(14 * time.Minute)},
		// Signal: learning
		{ID: "fd-8", Timestamp: base.Add(18 * time.Minute), Source: "mcp", Type: "learning",
			Content:        "Discovered that SQLite FTS5 requires the sqlite_fts5 build tag in Go. Without it, CREATE VIRTUAL TABLE fails silently. All builds must use: CGO_ENABLED=1 go build -tags sqlite_fts5.",
			HeuristicScore: 0.85, InitialSalience: 0.75, Project: "mnemonic", CreatedAt: base.Add(18 * time.Minute)},
		// Noise: git operations
		{ID: "fd-9", Timestamp: base.Add(20 * time.Minute), Source: "terminal", Type: "command",
			Content:        "git status",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(20 * time.Minute)},
		{ID: "fd-10", Timestamp: base.Add(21 * time.Minute), Source: "terminal", Type: "command",
			Content:        "git diff --stat",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(21 * time.Minute)},
		// Noise: filesystem
		{ID: "fd-11", Timestamp: base.Add(23 * time.Minute), Source: "filesystem", Type: "file_created",
			Content:        "Created: /tmp/mnemonic-test-12345/bench.db",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(23 * time.Minute)},
		// Signal: decision
		{ID: "fd-12", Timestamp: base.Add(25 * time.Minute), Source: "mcp", Type: "decision",
			Content:        "Chose slog over zerolog for structured logging. slog is in the standard library since Go 1.21, reducing external dependencies. It supports JSON and text handlers, and integrates cleanly with context.",
			HeuristicScore: 0.8, InitialSalience: 0.7, Project: "mnemonic", CreatedAt: base.Add(25 * time.Minute)},
		// Noise: build output
		{ID: "fd-13", Timestamp: base.Add(27 * time.Minute), Source: "terminal", Type: "command",
			Content:        "go build -tags sqlite_fts5 ./cmd/mnemonic/",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "mnemonic", CreatedAt: base.Add(27 * time.Minute)},
		// Noise: filesystem churn
		{ID: "fd-14", Timestamp: base.Add(28 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/mem/bin/mnemonic",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(28 * time.Minute)},
		// Signal: design
		{ID: "fd-15", Timestamp: base.Add(30 * time.Minute), Source: "mcp", Type: "insight",
			Content:        "Designed the memory consolidation algorithm. Each cycle: (1) decay all salience by factor, (2) transition low-salience to fading, (3) archive memories below threshold, (4) merge similar fading memories, (5) extract patterns from clusters. Inspired by human sleep consolidation.",
			HeuristicScore: 0.9, InitialSalience: 0.85, Project: "mnemonic", CreatedAt: base.Add(30 * time.Minute)},
		// Noise: various terminal
		{ID: "fd-16", Timestamp: base.Add(32 * time.Minute), Source: "terminal", Type: "command",
			Content:        "cat /proc/cpuinfo | head -20",
			HeuristicScore: 0.05, InitialSalience: 0.1, CreatedAt: base.Add(32 * time.Minute)},
		{ID: "fd-17", Timestamp: base.Add(33 * time.Minute), Source: "terminal", Type: "command",
			Content:        "docker ps",
			HeuristicScore: 0.05, InitialSalience: 0.1, CreatedAt: base.Add(33 * time.Minute)},
		{ID: "fd-18", Timestamp: base.Add(34 * time.Minute), Source: "filesystem", Type: "file_read",
			Content:        "Read file: /home/user/.bashrc",
			HeuristicScore: 0.05, InitialSalience: 0.1, CreatedAt: base.Add(34 * time.Minute)},
		// Noise: clipboard
		{ID: "fd-19", Timestamp: base.Add(35 * time.Minute), Source: "clipboard", Type: "copy",
			Content:        "func (s *SQLiteStore) Close() error",
			HeuristicScore: 0.15, InitialSalience: 0.2, Project: "mnemonic", CreatedAt: base.Add(35 * time.Minute)},
		// Signal: bug fix
		{ID: "fd-20", Timestamp: base.Add(38 * time.Minute), Source: "mcp", Type: "error",
			Content:        "Fixed race condition in event bus. The Publish method was not holding the mutex while iterating subscribers, allowing concurrent Subscribe/Unsubscribe to modify the map. Solution: copy subscriber list under lock before dispatching.",
			HeuristicScore: 0.85, InitialSalience: 0.8, Project: "mnemonic", CreatedAt: base.Add(38 * time.Minute)},
		// Noise
		{ID: "fd-21", Timestamp: base.Add(40 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/mem/internal/events/bus.go",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "mnemonic", CreatedAt: base.Add(40 * time.Minute)},
		{ID: "fd-22", Timestamp: base.Add(41 * time.Minute), Source: "terminal", Type: "command",
			Content:        "go test -tags sqlite_fts5 ./internal/events/ -v -race",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "mnemonic", CreatedAt: base.Add(41 * time.Minute)},
		// Noise: system
		{ID: "fd-23", Timestamp: base.Add(43 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/.local/share/recently-used.xbel",
			HeuristicScore: 0.02, InitialSalience: 0.05, CreatedAt: base.Add(43 * time.Minute)},
		{ID: "fd-24", Timestamp: base.Add(44 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/.cache/thumbnails/large/abc123.png",
			HeuristicScore: 0.02, InitialSalience: 0.05, CreatedAt: base.Add(44 * time.Minute)},
		// Signal: testing
		{ID: "fd-25", Timestamp: base.Add(48 * time.Minute), Source: "mcp", Type: "implementation",
			Content:        "Wrote integration test for the full encoding pipeline. Test creates a raw memory, runs the encoding agent, verifies: memory is created with correct concepts, embedding is generated, similar memories are linked via associations, and raw memory is marked as processed.",
			HeuristicScore: 0.8, InitialSalience: 0.7, Project: "mnemonic", CreatedAt: base.Add(48 * time.Minute)},
		// Noise: trailing terminal
		{ID: "fd-26", Timestamp: base.Add(50 * time.Minute), Source: "terminal", Type: "command",
			Content:        "make test",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "mnemonic", CreatedAt: base.Add(50 * time.Minute)},
		{ID: "fd-27", Timestamp: base.Add(51 * time.Minute), Source: "filesystem", Type: "file_read",
			Content:        "Read file: /home/user/Projects/mem/Makefile",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "mnemonic", CreatedAt: base.Add(51 * time.Minute)},
		{ID: "fd-28", Timestamp: base.Add(52 * time.Minute), Source: "terminal", Type: "command",
			Content:        "systemctl --user status mnemonic",
			HeuristicScore: 0.1, InitialSalience: 0.15, CreatedAt: base.Add(52 * time.Minute)},
		{ID: "fd-29", Timestamp: base.Add(53 * time.Minute), Source: "clipboard", Type: "copy",
			Content:        "127.0.0.1:9999",
			HeuristicScore: 0.05, InitialSalience: 0.1, CreatedAt: base.Add(53 * time.Minute)},
		{ID: "fd-30", Timestamp: base.Add(55 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/mem/internal/agent/encoding/agent_test.go",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "mnemonic", CreatedAt: base.Add(55 * time.Minute)},
	}

	return pipelineScenario{
		Name:      "Full Day",
		RawEvents: events,
		SignalIDs: signalIDs,
		Queries: []pipelineQuery{
			{Query: "What architectural decisions were made about persistence?", ExpectedConcepts: []string{"sqlite", "persistence", "database", "fts5"}},
			{Query: "What bugs were fixed today?", ExpectedConcepts: []string{"nil", "pointer", "race", "condition", "bug", "fix"}},
			{Query: "What did we learn about Go build requirements?", ExpectedConcepts: []string{"build", "sqlite", "fts5", "cgo", "tag"}},
		},
	}
}

// crossPollinationScenario tests cross-project association discovery.
// 20 raw events across 3 projects sharing overlapping concepts.
func crossPollinationScenario() pipelineScenario {
	base := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)

	signalIDs := map[string]bool{
		"cp-1":  true, // Project A: caching layer design
		"cp-3":  true, // Project A: Redis integration decision
		"cp-5":  true, // Project B: database migration strategy
		"cp-7":  true, // Project B: query optimization
		"cp-10": true, // Project C: API rate limiting
		"cp-12": true, // Project C: circuit breaker pattern
		"cp-15": true, // Project A: cache invalidation strategy
	}

	events := []store.RawMemory{
		// Project A: Caching layer
		{ID: "cp-1", Timestamp: base, Source: "mcp", Type: "decision",
			Content:        "Designing a caching layer for the API. Need to cache database query results to reduce latency. Considering Redis for distributed cache with TTL-based expiry.",
			HeuristicScore: 0.85, InitialSalience: 0.8, Project: "api-gateway", CreatedAt: base},
		{ID: "cp-2", Timestamp: base.Add(2 * time.Minute), Source: "terminal", Type: "command",
			Content:        "redis-cli ping",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "api-gateway", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "cp-3", Timestamp: base.Add(5 * time.Minute), Source: "mcp", Type: "decision",
			Content:        "Chose Redis over Memcached for the caching layer. Redis supports data structures (sorted sets for leaderboards, hashes for user sessions), persistence, and pub/sub for cache invalidation broadcasts.",
			HeuristicScore: 0.8, InitialSalience: 0.75, Project: "api-gateway", CreatedAt: base.Add(5 * time.Minute)},
		{ID: "cp-4", Timestamp: base.Add(7 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/api-gateway/internal/cache/redis.go",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "api-gateway", CreatedAt: base.Add(7 * time.Minute)},
		// Project B: Database work
		{ID: "cp-5", Timestamp: base.Add(10 * time.Minute), Source: "mcp", Type: "decision",
			Content:        "Designed database migration strategy for the analytics service. Using golang-migrate for versioned migrations. Each migration is idempotent. Schema changes must be backwards-compatible for zero-downtime deploys.",
			HeuristicScore: 0.85, InitialSalience: 0.8, Project: "analytics", CreatedAt: base.Add(10 * time.Minute)},
		{ID: "cp-6", Timestamp: base.Add(12 * time.Minute), Source: "terminal", Type: "command",
			Content:        "psql -h localhost analytics_dev -c '\\dt'",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "analytics", CreatedAt: base.Add(12 * time.Minute)},
		{ID: "cp-7", Timestamp: base.Add(15 * time.Minute), Source: "mcp", Type: "insight",
			Content:        "Optimized the query performance for the analytics dashboard. Added composite index on (user_id, timestamp) which reduced query time from 2.3s to 45ms. Also added query result caching with 5-minute TTL to further reduce database load.",
			HeuristicScore: 0.9, InitialSalience: 0.85, Project: "analytics", CreatedAt: base.Add(15 * time.Minute)},
		{ID: "cp-8", Timestamp: base.Add(17 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/analytics/migrations/005_add_composite_index.sql",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "analytics", CreatedAt: base.Add(17 * time.Minute)},
		{ID: "cp-9", Timestamp: base.Add(19 * time.Minute), Source: "terminal", Type: "command",
			Content:        "go test ./internal/db/ -v -count=1",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "analytics", CreatedAt: base.Add(19 * time.Minute)},
		// Project C: API resilience
		{ID: "cp-10", Timestamp: base.Add(22 * time.Minute), Source: "mcp", Type: "implementation",
			Content:        "Implemented API rate limiting using a token bucket algorithm. Each API key gets 100 requests per minute. When the bucket is empty, requests return 429 Too Many Requests. The bucket refills at a steady rate.",
			HeuristicScore: 0.85, InitialSalience: 0.8, Project: "payment-svc", CreatedAt: base.Add(22 * time.Minute)},
		{ID: "cp-11", Timestamp: base.Add(24 * time.Minute), Source: "terminal", Type: "command",
			Content:        "curl -w '%{http_code}' http://localhost:8080/api/health",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "payment-svc", CreatedAt: base.Add(24 * time.Minute)},
		{ID: "cp-12", Timestamp: base.Add(27 * time.Minute), Source: "mcp", Type: "implementation",
			Content:        "Implemented circuit breaker pattern for external payment API calls. Uses three states: closed (normal), open (failing, reject immediately), half-open (test with single request). Thresholds: 5 failures to open, 30s timeout to half-open, 2 successes to close.",
			HeuristicScore: 0.85, InitialSalience: 0.8, Project: "payment-svc", CreatedAt: base.Add(27 * time.Minute)},
		// Noise across projects
		{ID: "cp-13", Timestamp: base.Add(29 * time.Minute), Source: "filesystem", Type: "file_read",
			Content:        "Read file: /home/user/.ssh/config",
			HeuristicScore: 0.05, InitialSalience: 0.1, CreatedAt: base.Add(29 * time.Minute)},
		{ID: "cp-14", Timestamp: base.Add(30 * time.Minute), Source: "clipboard", Type: "copy",
			Content:        "SELECT * FROM events WHERE user_id = $1 ORDER BY timestamp DESC LIMIT 100",
			HeuristicScore: 0.15, InitialSalience: 0.2, Project: "analytics", CreatedAt: base.Add(30 * time.Minute)},
		// Signal: back to Project A with cross-project insight
		{ID: "cp-15", Timestamp: base.Add(33 * time.Minute), Source: "mcp", Type: "insight",
			Content:        "Cache invalidation strategy: using Redis pub/sub to broadcast invalidation events. When the database is updated, publish the changed key to a channel. All API instances subscribe and clear their local cache. Similar to the circuit breaker pattern in the payment service — both need distributed state coordination.",
			HeuristicScore: 0.9, InitialSalience: 0.85, Project: "api-gateway", CreatedAt: base.Add(33 * time.Minute)},
		// More noise
		{ID: "cp-16", Timestamp: base.Add(35 * time.Minute), Source: "terminal", Type: "command",
			Content:        "docker-compose logs -f redis",
			HeuristicScore: 0.1, InitialSalience: 0.15, CreatedAt: base.Add(35 * time.Minute)},
		{ID: "cp-17", Timestamp: base.Add(36 * time.Minute), Source: "filesystem", Type: "file_modified",
			Content:        "Modified: /home/user/Projects/api-gateway/docker-compose.yml",
			HeuristicScore: 0.1, InitialSalience: 0.15, Project: "api-gateway", CreatedAt: base.Add(36 * time.Minute)},
		{ID: "cp-18", Timestamp: base.Add(37 * time.Minute), Source: "terminal", Type: "command",
			Content:        "git log --oneline -5",
			HeuristicScore: 0.05, InitialSalience: 0.1, CreatedAt: base.Add(37 * time.Minute)},
		{ID: "cp-19", Timestamp: base.Add(38 * time.Minute), Source: "filesystem", Type: "file_read",
			Content:        "Read file: /home/user/.config/Code/User/keybindings.json",
			HeuristicScore: 0.02, InitialSalience: 0.05, CreatedAt: base.Add(38 * time.Minute)},
		{ID: "cp-20", Timestamp: base.Add(40 * time.Minute), Source: "clipboard", Type: "copy",
			Content:        "package cache",
			HeuristicScore: 0.05, InitialSalience: 0.1, Project: "api-gateway", CreatedAt: base.Add(40 * time.Minute)},
	}

	return pipelineScenario{
		Name:      "Cross-Pollination",
		RawEvents: events,
		SignalIDs: signalIDs,
		Queries: []pipelineQuery{
			{Query: "What caching strategies are we using across projects?", ExpectedConcepts: []string{"cache", "redis", "ttl", "invalidation"}},
			{Query: "How are we handling database performance?", ExpectedConcepts: []string{"database", "query", "index", "optimization", "migration"}},
			{Query: "What resilience patterns have we implemented?", ExpectedConcepts: []string{"circuit", "breaker", "rate", "limiting", "retry"}},
		},
	}
}

// noiseStormScenario is a stress test with 6 genuine insights buried in 34 noise events.
// The noise events contain realistic project paths to make filtering harder.
func noiseStormScenario() pipelineScenario {
	base := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)

	signalIDs := map[string]bool{
		"ns-5":  true, // critical security fix
		"ns-12": true, // architecture decision
		"ns-20": true, // performance insight
		"ns-28": true, // deployment strategy
		"ns-33": true, // testing philosophy
		"ns-38": true, // error handling pattern
	}

	var events []store.RawMemory

	// Generate 40 events, 6 signal + 34 noise.
	eventIdx := 1
	addNoise := func(source, typ, content string) {
		id := fmt.Sprintf("ns-%d", eventIdx)
		events = append(events, store.RawMemory{
			ID: id, Timestamp: base.Add(time.Duration(eventIdx) * time.Minute),
			Source: source, Type: typ, Content: content,
			HeuristicScore: 0.05 + float32(eventIdx%5)*0.03, InitialSalience: 0.1 + float32(eventIdx%3)*0.05,
			Project: "mnemonic", CreatedAt: base.Add(time.Duration(eventIdx) * time.Minute),
		})
		eventIdx++
	}
	addSignal := func(id, typ, content string) {
		events = append(events, store.RawMemory{
			ID: id, Timestamp: base.Add(time.Duration(eventIdx) * time.Minute),
			Source: "mcp", Type: typ, Content: content,
			HeuristicScore: 0.85, InitialSalience: 0.8,
			Project: "mnemonic", CreatedAt: base.Add(time.Duration(eventIdx) * time.Minute),
		})
		eventIdx++
	}

	// Block 1: noise leading up to first signal
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/internal/store/sqlite/sqlite.go")
	addNoise("terminal", "command", "go vet ./...")
	addNoise("filesystem", "file_read", "Read file: /home/user/Projects/mem/go.mod")
	addNoise("filesystem", "file_modified", "Modified: /home/user/.local/share/recently-used.xbel")

	// Signal 1: security fix
	addSignal("ns-5", "error", "Critical security fix: SQL injection vulnerability in the search endpoint. User input was being interpolated directly into the FTS5 MATCH query. Fixed by using parameterized queries: db.Query(\"SELECT * FROM memories WHERE content MATCH ?\", userInput).")

	// Block 2: more noise
	addNoise("filesystem", "file_created", "Created: /tmp/go-build-12345/main.o")
	addNoise("terminal", "command", "ps aux | grep mnemonic")
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/internal/api/routes/search.go")
	addNoise("clipboard", "copy", "SELECT id, content FROM memories_fts WHERE content MATCH ?")
	addNoise("terminal", "command", "curl http://127.0.0.1:9999/api/v1/health")
	addNoise("filesystem", "file_read", "Read file: /home/user/Projects/mem/config.yaml")

	// Signal 2: architecture decision
	addSignal("ns-12", "decision", "Architecture decision: all agent communication must go through the event bus, never direct function calls. This enables: (1) loose coupling between agents, (2) easy testing with mock subscribers, (3) future distributed deployment, (4) audit trail of all inter-agent communication.")

	// Block 3: noise
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/internal/events/bus.go")
	addNoise("terminal", "command", "git add -A && git commit -m 'fix: parameterize FTS5 queries'")
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/.git/COMMIT_EDITMSG")
	addNoise("terminal", "command", "make build")
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/bin/mnemonic")
	addNoise("filesystem", "file_read", "Read file: /home/user/Projects/mem/Makefile")
	addNoise("terminal", "command", "systemctl --user restart mnemonic")

	// Signal 3: performance insight
	addSignal("ns-20", "insight", "Performance insight: the spread activation algorithm was visiting the same node multiple times in deep graphs. Added a visited set to skip already-activated memories. This reduced query time from 150ms to 12ms for queries hitting the 5-hop maximum.")

	// Block 4: noise
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/internal/agent/retrieval/agent.go")
	addNoise("terminal", "command", "go test -tags sqlite_fts5 -bench=BenchmarkQuery ./internal/agent/retrieval/")
	addNoise("clipboard", "copy", "BenchmarkQuery-8  1000  12345 ns/op")
	addNoise("filesystem", "file_read", "Read file: /home/user/Projects/mem/internal/agent/retrieval/agent_test.go")
	addNoise("terminal", "command", "git diff HEAD~1")
	addNoise("filesystem", "file_modified", "Modified: /home/user/.cache/go-build/12/abc.a")
	addNoise("filesystem", "file_read", "Read file: /home/user/.config/Code/User/settings.json")

	// Signal 4: deployment strategy
	addSignal("ns-28", "decision", "Deployment strategy: mnemonic runs as a systemd user service on Linux and a LaunchAgent on macOS. The binary is self-contained with embedded web assets. Installation: copy binary, run 'mnemonic install', which creates the service file and enables it.")

	// Block 5: noise
	addNoise("terminal", "command", "journalctl --user -u mnemonic -f")
	addNoise("filesystem", "file_modified", "Modified: /home/user/.config/systemd/user/mnemonic.service")
	addNoise("filesystem", "file_read", "Read file: /home/user/Projects/mem/internal/daemon/service_linux.go")
	addNoise("terminal", "command", "which mnemonic")

	// Signal 5: testing philosophy
	addSignal("ns-33", "insight", "Testing insight: integration tests that hit a real SQLite database are more valuable than unit tests with mocked stores. We caught 3 bugs in the last month that mocked tests would have missed: FTS5 query syntax differences, foreign key constraint violations, and transaction isolation issues.")

	// Block 6: trailing noise
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/internal/store/sqlite/sqlite_test.go")
	addNoise("terminal", "command", "go test -tags sqlite_fts5 ./internal/store/sqlite/ -v -count=1")
	addNoise("clipboard", "copy", "t.Run(\"test FTS5 match\", func(t *testing.T) {")
	addNoise("filesystem", "file_read", "Read file: /home/user/.local/share/Code/logs/main.log")

	// Signal 6: error handling
	addSignal("ns-38", "learning", "Error handling pattern: always wrap errors with context using fmt.Errorf(\"doing X for Y: %%w\", id, err). This creates a chain that reads like a stack trace: 'consolidation cycle: listing memories: scanning row: sql: connection reset'. Makes debugging production issues much faster.")

	// Final noise
	addNoise("terminal", "command", "git push origin feat/benchmark-sweep")
	addNoise("filesystem", "file_modified", "Modified: /home/user/Projects/mem/.git/refs/remotes/origin/feat/benchmark-sweep")

	return pipelineScenario{
		Name:      "Noise Storm",
		RawEvents: events,
		SignalIDs: signalIDs,
		Queries: []pipelineQuery{
			{Query: "What security issues have we found and fixed?", ExpectedConcepts: []string{"security", "sql", "injection", "vulnerability", "fix"}},
			{Query: "What are the key architecture principles?", ExpectedConcepts: []string{"event", "bus", "agent", "architecture", "coupling"}},
			{Query: "What performance optimizations were made?", ExpectedConcepts: []string{"performance", "spread", "activation", "query", "optimization"}},
		},
	}
}
