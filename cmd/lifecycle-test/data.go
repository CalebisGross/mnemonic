package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

// memoryTemplate is a template for generating synthetic memories.
type memoryTemplate struct {
	Source  string // "mcp", "filesystem", "terminal", "clipboard"
	Type    string // "decision", "error", "insight", "learning", etc.
	Content string
	Project string
}

// seedMemories returns the 10 first-use memories for Phase 2.
func seedMemories(clock *SimClock) []store.RawMemory {
	templates := []memoryTemplate{
		// 3 decisions
		{Source: "mcp", Type: "decision", Content: "Chose SQLite with FTS5 for full-text search over Postgres because we need a local-first embedded database with no server dependency.", Project: "mnemonic"},
		{Source: "mcp", Type: "decision", Content: "Selected Go for the daemon implementation. The single binary deployment model and goroutine concurrency align with the agent architecture.", Project: "mnemonic"},
		{Source: "mcp", Type: "decision", Content: "Decided to use an event bus for inter-agent communication instead of direct function calls. This keeps agents decoupled and testable.", Project: "mnemonic"},
		// 3 errors
		{Source: "mcp", Type: "error", Content: "Nil pointer panic in retrieval agent when searching with empty embedding vector. Added guard clause to check embedding length before cosine similarity calculation.", Project: "mnemonic"},
		{Source: "mcp", Type: "error", Content: "FTS5 index corruption after concurrent writes from encoding agent. Fixed by serializing writes through a mutex in the store layer.", Project: "mnemonic"},
		{Source: "mcp", Type: "error", Content: "Memory consolidation loop was merging unrelated memories because cosine similarity threshold was too low. Raised threshold from 0.5 to 0.7.", Project: "mnemonic"},
		// 2 insights
		{Source: "mcp", Type: "insight", Content: "Spread activation with 3 hops produces the best retrieval quality. Beyond 3 hops, noise dominates signal in the association graph.", Project: "mnemonic"},
		{Source: "mcp", Type: "insight", Content: "MCP-sourced memories have 3x higher retrieval value than filesystem watcher memories. Should weight initial salience by source.", Project: "mnemonic"},
		// 2 learnings
		{Source: "mcp", Type: "learning", Content: "Go's sql.NullString is needed for nullable TEXT columns in SQLite. Using plain string causes silent data corruption on NULL values.", Project: "mnemonic"},
		{Source: "mcp", Type: "learning", Content: "The gorilla/websocket library requires explicit ping/pong handling for connection keepalive. Without it, connections drop after 60 seconds idle.", Project: "mnemonic"},
	}

	memories := make([]store.RawMemory, len(templates))
	for i, t := range templates {
		memories[i] = store.RawMemory{
			ID:              fmt.Sprintf("seed-%02d", i+1),
			Timestamp:       clock.Now().Add(time.Duration(i) * time.Minute),
			Source:          t.Source,
			Type:            t.Type,
			Content:         t.Content,
			HeuristicScore:  0.8,
			InitialSalience: 0.8,
			Project:         t.Project,
			CreatedAt:       clock.Now().Add(time.Duration(i) * time.Minute),
		}
	}
	return memories
}

// mcpSignalTemplates contains high-value MCP memories across projects.
var mcpSignalTemplates = []memoryTemplate{
	// Mnemonic decisions
	{Source: "mcp", Type: "decision", Content: "Switched from polling-based watcher to fsnotify for filesystem events. Reduces CPU usage from 15%% to 0.2%% idle.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Added bearer token authentication to the REST API. Previously any local process could read memories.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Chose bag-of-words embeddings for the stub LLM provider. Simple, deterministic, and vocabulary-aware for meaningful test coverage.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Implemented event bus architecture for agent communication instead of direct function calls between cognitive agents.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Used pure Go SQLite driver modernc.org/sqlite to avoid CGO dependency on Linux builds.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Implemented graceful shutdown with context cancellation propagating through all agents and the API server.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Chose FTS5 over FTS4 for full-text search because FTS5 supports BM25 ranking out of the box.", Project: "mnemonic"},
	{Source: "mcp", Type: "decision", Content: "Embedded the dashboard SPA directly in the Go binary using go:embed for zero-dependency deployment.", Project: "mnemonic"},
	// Mnemonic errors
	{Source: "mcp", Type: "error", Content: "Dreaming agent infinite loop when association graph has cycles. Fixed by tracking visited nodes during spread activation.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "Memory encoding timeout when LLM server is overloaded. Added 30s timeout with exponential backoff retry.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "Abstraction agent crashed when no patterns exist yet. Added early return guard for empty pattern list.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "Race condition in concurrent encoding when two goroutines claim the same raw memory. Added ClaimRawForEncoding with row-level locking.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "Dashboard WebSocket connection drops after 60 seconds idle. Added ping/pong keepalive handler to gorilla/websocket.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "FTS5 index corruption after concurrent writes from encoding agent. Fixed by serializing writes through a mutex.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "Consolidation merge produced duplicate patterns when two similar memory clusters overlapped. Added dedup check before pattern creation.", Project: "mnemonic"},
	{Source: "mcp", Type: "error", Content: "Self-update on Windows failed because the binary was locked by the running process. Implemented rename-and-replace strategy.", Project: "mnemonic"},
	// Mnemonic insights
	{Source: "mcp", Type: "insight", Content: "Episoding works best with 10-minute windows. Shorter windows fragment related memories, longer windows merge unrelated sessions.", Project: "mnemonic"},
	{Source: "mcp", Type: "insight", Content: "Association strength follows a power law distribution. Top 5%% of associations carry 60%% of retrieval value.", Project: "mnemonic"},
	{Source: "mcp", Type: "insight", Content: "Consolidation decay rate of 0.92 per cycle provides good balance between retaining signal and pruning noise over 30-day windows.", Project: "mnemonic"},
	{Source: "mcp", Type: "insight", Content: "MCP-sourced memories have 3x higher retrieval value than filesystem watcher memories based on feedback analysis.", Project: "mnemonic"},
	{Source: "mcp", Type: "insight", Content: "Spread activation with 3 hops produces the best retrieval quality. Beyond 3 hops noise dominates signal in the graph.", Project: "mnemonic"},
	{Source: "mcp", Type: "insight", Content: "Hybrid retrieval combining FTS5 and vector search with reciprocal rank fusion outperforms either method alone by 15%%.", Project: "mnemonic"},
	{Source: "mcp", Type: "insight", Content: "The reactor engine chain pattern works well for coordinating post-consolidation tasks like metacognition triggers.", Project: "mnemonic"},
	// Mnemonic learnings
	{Source: "mcp", Type: "learning", Content: "SQLite WAL mode is essential for concurrent read/write access. Without it, encoding agent blocks retrieval queries.", Project: "mnemonic"},
	{Source: "mcp", Type: "learning", Content: "The slog structured logger performs better than logrus for high-throughput event logging in the perception agent.", Project: "mnemonic"},
	{Source: "mcp", Type: "learning", Content: "Go build tags for platform-specific code must appear before the package declaration. Misplaced tags silently compile wrong code.", Project: "mnemonic"},
	{Source: "mcp", Type: "learning", Content: "Go sql.NullString is needed for nullable TEXT columns in SQLite. Using plain string causes silent data corruption on NULL values.", Project: "mnemonic"},
	{Source: "mcp", Type: "learning", Content: "The gorilla/websocket library requires explicit ping/pong handling for connection keepalive on the dashboard.", Project: "mnemonic"},
	{Source: "mcp", Type: "learning", Content: "SQLite PRAGMA journal_mode=WAL must be set per connection, not just once at database creation time.", Project: "mnemonic"},
	{Source: "mcp", Type: "learning", Content: "Go embed directive paths are relative to the source file, not the module root. Learned this debugging missing dashboard assets.", Project: "mnemonic"},

	// Felix-LM project (cross-project)
	{Source: "mcp", Type: "decision", Content: "Adopted conventional commits for the felix-lm project to match mnemonic's release-please workflow.", Project: "felix-lm"},
	{Source: "mcp", Type: "decision", Content: "Chose AdamW optimizer with cosine annealing schedule for the 100M parameter pretraining run.", Project: "felix-lm"},
	{Source: "mcp", Type: "decision", Content: "Implemented gradient accumulation of 4 micro-steps to simulate larger batch sizes on the RX 7800 XT.", Project: "felix-lm"},
	{Source: "mcp", Type: "error", Content: "PyTorch ROCm build fails on Ubuntu 24.04 with Python 3.14. Pinned to Python 3.12 for compatibility.", Project: "felix-lm"},
	{Source: "mcp", Type: "error", Content: "Training loss spiked at step 12000 due to corrupt data shard. Added checksum validation to the data loader.", Project: "felix-lm"},
	{Source: "mcp", Type: "error", Content: "VRAM out-of-memory crash during HP sweep with batch size 32. Added automatic VRAM cap detection.", Project: "felix-lm"},
	{Source: "mcp", Type: "insight", Content: "Learning rate warmup of 500 steps consistently outperforms no-warmup across all model sizes tested.", Project: "felix-lm"},
	{Source: "mcp", Type: "insight", Content: "Weight decay of 0.01 provides better generalization than 0.1 for the 100M model architecture.", Project: "felix-lm"},
	{Source: "mcp", Type: "insight", Content: "Mixed precision training with bfloat16 gives identical loss curves to float32 but uses 40%% less VRAM.", Project: "felix-lm"},
	{Source: "mcp", Type: "learning", Content: "Unsloth 4-bit quantization reduces VRAM from 14GB to 6GB with only 2%% perplexity increase on the validation set.", Project: "felix-lm"},
	{Source: "mcp", Type: "learning", Content: "ROCm hipcc compiler requires explicit device targeting with PYTORCH_ROCM_ARCH=gfx1100 for the 7800 XT.", Project: "felix-lm"},
	{Source: "mcp", Type: "learning", Content: "The tokenizer's padding side must match the model architecture. GPT-style models need left padding for batch inference.", Project: "felix-lm"},

	// Sample-project (third project for cross-project testing)
	{Source: "mcp", Type: "decision", Content: "Chose chi router over gorilla/mux for the sample REST API because chi has better middleware composability.", Project: "sample-project"},
	{Source: "mcp", Type: "error", Content: "Connection pool exhaustion under load testing. Increased max idle connections from 5 to 25 in database/sql config.", Project: "sample-project"},
	{Source: "mcp", Type: "insight", Content: "Request logging middleware adds 50 microseconds per request. Acceptable overhead for the observability benefit.", Project: "sample-project"},
	{Source: "mcp", Type: "learning", Content: "Go http.Server ReadTimeout should be set to prevent slowloris attacks. Default zero timeout leaves connections open forever.", Project: "sample-project"},
}

// noiseTemplates contains filesystem, terminal, and clipboard noise.
var noiseTemplates = []memoryTemplate{
	// Filesystem noise
	{Source: "filesystem", Type: "file_modified", Content: "Modified ~/.config/Code/User/settings.json: changed editor.fontSize from 14 to 15", Project: ""},
	{Source: "filesystem", Type: "file_created", Content: "Created /tmp/go-build-cache/ab/abc123.o: Go build artifact", Project: ""},
	{Source: "filesystem", Type: "file_modified", Content: "Modified ~/.local/share/gnome-shell/extensions/prefs.js: GNOME extension preferences update", Project: ""},
	{Source: "filesystem", Type: "file_created", Content: "Created ~/Downloads/screenshot-2026-01-05.png: desktop screenshot", Project: ""},
	{Source: "filesystem", Type: "file_modified", Content: "Modified ~/.bashrc: added export PATH=$PATH:~/go/bin", Project: ""},
	{Source: "filesystem", Type: "file_created", Content: "Created /tmp/mnemonic-bench-xyz/pipeline.db: benchmark temp database", Project: ""},
	{Source: "filesystem", Type: "file_modified", Content: "Modified ~/.config/gtk-3.0/settings.ini: GTK theme changed to Adwaita-dark", Project: ""},
	{Source: "filesystem", Type: "file_created", Content: "Created ~/.cache/thumbnails/large/abcdef.png: thumbnail cache entry", Project: ""},
	{Source: "filesystem", Type: "file_modified", Content: "Modified /etc/hosts: added local development domain mapping", Project: ""},
	{Source: "filesystem", Type: "file_created", Content: "Created ~/Documents/notes-2026-01.md: personal notes file", Project: ""},
	{Source: "filesystem", Type: "file_modified", Content: "Modified ~/.ssh/config: added new host entry for staging server", Project: ""},
	{Source: "filesystem", Type: "file_created", Content: "Created ~/Downloads/go1.22.0.linux-amd64.tar.gz: Go binary download", Project: ""},
	// Terminal noise
	{Source: "terminal", Type: "command_executed", Content: "git status: On branch main, nothing to commit, working tree clean", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "make build: go build -o bin/mnemonic ./cmd/mnemonic", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "ls -la ~/Projects/: listed directory contents", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "top: system monitor showing 4.2GB RAM used, load average 1.2", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "docker ps: no containers running", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "git log --oneline -5: viewed recent commit history", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "curl http://127.0.0.1:9999/api/v1/health: checked daemon health endpoint", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "systemctl --user status mnemonic: daemon is active and running", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "go test ./internal/store/... : ran store package tests, all passed", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "df -h: checked disk usage, 45GB free on root partition", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "htop: 8 cores, 2.1GB used of 32GB RAM, no swap activity", Project: ""},
	{Source: "terminal", Type: "command_executed", Content: "rocm-smi: GPU temperature 42C, VRAM 0MB/16GB used, idle", Project: ""},
	// Clipboard noise
	{Source: "clipboard", Type: "clipboard_copy", Content: "https://pkg.go.dev/modernc.org/sqlite", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "func (s *SQLiteStore) WriteRaw(ctx context.Context, raw RawMemory) error {", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "SELECT id, summary, salience FROM memories WHERE state = 'active'", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "127.0.0.1:9999", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "https://github.com/appsprout-dev/mnemonic/pull/248", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "export PYTORCH_ROCM_ARCH=gfx1100", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "CREATE INDEX idx_memories_salience ON memories(salience DESC);", Project: ""},
	{Source: "clipboard", Type: "clipboard_copy", Content: "func TestRetrievalQuery(t *testing.T) {", Project: ""},
}

// generateDailyMemories creates a batch of memories for one simulated day.
// Each memory gets a unique content suffix (day+index) to avoid encoding dedup.
// Distribution: ~30% MCP signal, ~50% filesystem/terminal noise, ~20% clipboard.
func generateDailyMemories(rng *rand.Rand, clock *SimClock, day int, count int) []store.RawMemory {
	memories := make([]store.RawMemory, 0, count)

	for i := 0; i < count; i++ {
		// Pick from signal or noise based on distribution.
		var t memoryTemplate
		roll := rng.Float32()
		switch {
		case roll < 0.30:
			t = mcpSignalTemplates[rng.Intn(len(mcpSignalTemplates))]
		case roll < 0.80:
			t = noiseTemplates[rng.Intn(len(noiseTemplates))]
		default:
			// Clipboard subset from noise templates.
			clipTemplates := make([]memoryTemplate, 0)
			for _, nt := range noiseTemplates {
				if nt.Source == "clipboard" {
					clipTemplates = append(clipTemplates, nt)
				}
			}
			t = clipTemplates[rng.Intn(len(clipTemplates))]
		}

		// Append day+index to make each memory's content unique for dedup.
		content := fmt.Sprintf("%s [day %d, event %d]", t.Content, day, i+1)

		heuristic := float32(0.3)
		salience := float32(0.3)
		switch t.Source {
		case "mcp":
			heuristic = 0.7 + rng.Float32()*0.2
			salience = 0.7 + rng.Float32()*0.2
		case "filesystem", "terminal":
			heuristic = 0.1 + rng.Float32()*0.3
			salience = 0.1 + rng.Float32()*0.3
		case "clipboard":
			heuristic = 0.2 + rng.Float32()*0.3
			salience = 0.2 + rng.Float32()*0.3
		}

		ts := clock.Now().Add(time.Duration(i) * 2 * time.Minute)
		memories = append(memories, store.RawMemory{
			ID:              fmt.Sprintf("day%02d-%03d", day, i+1),
			Timestamp:       ts,
			Source:          t.Source,
			Type:            t.Type,
			Content:         content,
			HeuristicScore:  heuristic,
			InitialSalience: salience,
			Project:         t.Project,
			CreatedAt:       ts,
		})
	}

	return memories
}

// syntheticProjectFiles returns file contents for a small synthetic Go project.
func syntheticProjectFiles() map[string]string {
	return map[string]string{
		"main.go": `package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/health", healthHandler)
	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}
`,
		"handler.go": `package main

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Status  string ` + "`json:\"status\"`" + `
	Message string ` + "`json:\"message\"`" + `
}

func jsonResponse(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
`,
		"store.go": `package main

import (
	"database/sql"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
`,
		"config.go": `package main

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port     int    ` + "`yaml:\"port\"`" + `
	Database string ` + "`yaml:\"database\"`" + `
	LogLevel string ` + "`yaml:\"log_level\"`" + `
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
`,
		"middleware.go": `package main

import (
	"log"
	"net/http"
	"time"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
`,
		"README.md": `# Sample Project

This is a sample Go web server used for testing mnemonic's project ingestion pipeline.
It demonstrates a typical small Go project with HTTP handlers, middleware, configuration
loading, and a SQLite-backed store. The project structure follows standard Go conventions
with separate files for handlers, middleware, storage, and configuration.

## Architecture

The server exposes a REST API with health check and JSON response endpoints. Authentication
is handled via bearer token middleware. Configuration is loaded from a YAML file. The store
uses SQLite for persistence with proper connection lifecycle management.
`,
		"docs/design.md": `# Design Document

## Overview

This service provides a lightweight REST API for managing resources. The architecture
prioritizes simplicity and local-first operation, using SQLite for storage and Go's
standard library for HTTP serving.

## Key Decisions

1. SQLite over Postgres: No external dependencies, single file database, good enough
   for our expected scale of hundreds of concurrent users.
2. Standard library HTTP: No framework overhead, direct control over middleware chain,
   well-understood error handling patterns.
3. YAML configuration: Human-readable, supports comments, widely used in Go ecosystem.

## Performance Considerations

The SQLite WAL mode enables concurrent reads during writes. Connection pooling is managed
by database/sql with sensible defaults. The middleware chain adds approximately 50 microseconds
per request for logging and authentication.
`,
		"config.yaml": `port: 8080
database: "./data.db"
log_level: "info"
`,
	}
}
