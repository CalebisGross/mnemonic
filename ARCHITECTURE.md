# Mnemonic v1 — Refined Architecture Specification

> Local-first, air-gapped, agentic memory that learns through use.
> The architecture is the intelligence. The LLM is the judgment.

---

## Core Design Philosophy

The cognitive model (spread activation, salience decay, associative linking) is the **durable foundation** — it's based on decades of cognitive science and won't be obsoleted by next month's model release. Everything else — LLM providers, storage backends, embedding models — is **swappable plumbing** behind clean interfaces.

---

## Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Fast, single binary, great concurrency, clean daemon |
| Store | SQLite (WAL mode) | Sub-ms lookups, FTS5, ACID, single file, embedded |
| LLM runtime | LM Studio | Local, OpenAI-compatible API, model-agnostic |
| Embeddings | LM Studio (e.g. nomic-embed-text) | Same runtime, separate model, local semantic search |
| Platform | macOS ARM (primary), Linux x86_64 (next), Windows (planned) | Cross-platform via build tags |

---

## Core Abstractions (Interfaces)

These are the load-bearing walls. Get them right and everything downstream is swappable.

### 1. `llm.Provider`
```go
type Provider interface {
    Complete(ctx, req CompletionRequest) (CompletionResponse, error)
    Embed(ctx, text string) ([]float32, error)
    StructuredComplete(ctx, req CompletionRequest, schema interface{}) (StructuredOutput, error)
    Health(ctx) error
    ModelInfo(ctx) (ModelMetadata, error)
}
```
- v1 implementation: LM Studio (OpenAI-compatible)
- Future: Ollama, vLLM, cloud APIs — zero agent code changes

### 2. `store.Store`
```go
type Store interface {
    // Raw memory CRUD
    WriteRaw(ctx, raw RawMemory) error
    ListRawUnprocessed(ctx, limit int) ([]RawMemory, error)
    // Encoded memory CRUD
    WriteMemory(ctx, mem Memory) error
    GetMemory(ctx, id string) (Memory, error)
    UpdateSalience(ctx, id string, salience float32) error
    // Search (FTS + embedding + concepts)
    SearchByFullText(ctx, query string, limit int) ([]Memory, error)
    SearchByEmbedding(ctx, embedding []float32, limit int) ([]RetrievalResult, error)
    // Association graph
    CreateAssociation(ctx, assoc Association) error
    GetAssociations(ctx, memoryID string) ([]Association, error)
    // Spread activation (first-class operation)
    SpreadActivation(ctx, entryPoints []Memory, maxHops int, config ActivationConfig) ([]RetrievalResult, error)
    // Batch ops for consolidation
    BatchUpdateSalience(ctx, updates map[string]float32) error
    BatchMergeMemories(ctx, sourceIDs []string, gistMemory Memory) error
    // Transactions
    BeginTx(ctx) (Tx, error)
    // Housekeeping
    GetStatistics(ctx) (StoreStatistics, error)
}
```
- v1 implementation: SQLite with FTS5 + binary embeddings
- Future: PostgreSQL, vector DBs — zero agent code changes

### 3. `watcher.Watcher`
```go
type Watcher interface {
    Start(ctx) error
    Stop(ctx) error
    Events() <-chan Event
    Health(ctx) error
}
```
- v1 implementations: Filesystem, Terminal History, Clipboard
- Each watcher is independent and pluggable

### 4. `agent.Agent`
```go
type Agent interface {
    Name() string
    Run(ctx, bus events.EventBus) error
    Health(ctx) error
}
```
- All 8 cognitive agents + orchestrator + reactor implement this
- Loosely coupled through the event bus

### 5. `events.EventBus`
```go
type EventBus interface {
    Publish(ctx, event Event) error
    Subscribe(eventType string, handler Handler) error
    Close() error
}
```
- v1 implementation: In-memory pub/sub
- Agents communicate through events, never direct calls

---

## Cognitive Layers

### Layer 1 — Perception (Full Ingestion)

**Three watchers active from v1:**
- **Filesystem**: Watch configured dirs, track create/modify/delete
- **Terminal**: Poll shell history, detect new entries
- **Clipboard**: Poll for text changes

**Heuristic pre-filter pipeline** (before any LLM call):
```
Raw Event → Size Filter → Pattern Blacklist → Frequency Dedup → Content Heuristic → [LLM Gate] → RawMemory
```
- Size: Skip >100KB or <10 chars
- Patterns: Skip .git, node_modules, temp files, .DS_Store
- Frequency: Skip if same event >5 times in 10 min
- Content: Simple keyword scoring (error, todo, important → higher score)
- Only events passing heuristics go to LLM for salience judgment

### Layer 2 — Encoding (Compression & Linking)

Triggered by `RawMemoryCreated` event:
1. LLM compresses raw content → summary + concepts (structured output)
2. Generate embedding via embedding provider
3. Find similar memories via embedding + FTS
4. Create association links with strength weights
5. Emit `MemoryEncoded` event

### Layer 3 — Consolidation (Sleep Cycle)

Runs every 6 hours (or on-demand). **Budget-constrained**: max 100 memories per cycle.

Operations in order:
1. **Decay**: `new_salience = salience * decay_rate^(hours_since_access)`
2. **State transitions**: active → fading (< 0.3) → archived (< 0.1) → deleted (> 90 days)
3. **Strengthen**: Recently accessed memories get salience boost
4. **Prune associations**: Weaken/remove low-strength, never-activated links
5. **Merge** (max 5 per cycle): Cluster highly-related memories → LLM creates gist memory

### Layer 4 — Retrieval (Associative Recall)

```
Query → [Parse + Embed] → Entry Points (FTS top 3 + Embedding top 3) → Spread Activation (3 hops) → Rank → Top 7 → [Optional: LLM Synthesis]
```

Spread activation follows strongest association links, with activation decaying per hop. Every accessed memory gets strengthened (access_count++, last_accessed updated).

### Layer 5 — Meta-Cognition (Basic Monitoring)

Runs daily. v1 scope is **observe and log**, not act:
- Memory growth patterns (topic concentration)
- Retrieval success/failure rate (via user feedback)
- Association graph health (isolated clusters, density)
- Consolidation effectiveness
- Log observations to `meta_observations` table

---

## API Surface

### HTTP REST (`http://localhost:9999/api/v1/`)

```
POST   /memories              Create raw memory (explicit user input)
GET    /memories               List memories (with filters)
GET    /memories/:id           Get specific memory
POST   /query                  Query memories (spread activation)
POST   /consolidation/run      Force consolidation cycle
GET    /consolidation/status   Last consolidation info
GET    /health                 System health check
GET    /stats                  Memory statistics
POST   /feedback               Submit retrieval feedback
```

### WebSocket (`ws://localhost:9999/ws`)

Real-time event stream. Clients can filter by event type:
- `raw_memory_created` — new perception event
- `memory_encoded` — memory compressed and stored
- `consolidation_completed` — sleep cycle finished
- `query_executed` — retrieval performed

### Web Dashboard (embedded in Go binary)

Served at `http://localhost:9999/`. Features:
- Memory count by state (active/fading/archived)
- Live event feed (real-time via WebSocket)
- Association graph visualization (D3.js)
- Query tester with score explanations
- System health (LLM status, store health, watcher status)

---

## SQLite Schema

```sql
-- Raw observations before encoding
CREATE TABLE raw_memories (
    id TEXT PRIMARY KEY,
    timestamp DATETIME NOT NULL,
    source TEXT NOT NULL,        -- 'terminal', 'filesystem', 'clipboard', 'user'
    type TEXT,
    content TEXT NOT NULL,
    metadata JSON,
    heuristic_score REAL,
    initial_salience REAL DEFAULT 0.5,
    processed BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Encoded memories (the real memory store)
CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    raw_id TEXT REFERENCES raw_memories(id),
    timestamp DATETIME NOT NULL,
    content TEXT NOT NULL,       -- compressed/encoded form
    summary TEXT NOT NULL,       -- one-liner
    concepts JSON,               -- extracted concepts
    embedding BLOB,              -- float32 vector
    salience REAL DEFAULT 0.5,
    access_count INTEGER DEFAULT 0,
    last_accessed DATETIME,
    state TEXT DEFAULT 'active', -- active | fading | archived | merged
    gist_of JSON,               -- if merged: source memory IDs
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- FTS5 for full-text search (auto-synced via triggers)
CREATE VIRTUAL TABLE memories_fts USING fts5(
    id UNINDEXED, summary, content, concepts,
    content='memories', content_rowid='rowid'
);

-- Association graph
CREATE TABLE associations (
    source_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    strength REAL DEFAULT 0.5,
    relation_type TEXT DEFAULT 'similar',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_activated DATETIME,
    activation_count INTEGER DEFAULT 0,
    PRIMARY KEY (source_id, target_id)
);

-- Meta-cognition observations
CREATE TABLE meta_observations (
    id TEXT PRIMARY KEY,
    observation_type TEXT NOT NULL,
    severity TEXT DEFAULT 'info',
    details JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Retrieval feedback for learning
CREATE TABLE retrieval_feedback (
    query_id TEXT PRIMARY KEY,
    query_text TEXT NOT NULL,
    retrieved_memory_ids JSON,
    feedback TEXT,              -- 'helpful', 'partial', 'irrelevant'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Consolidation history
CREATE TABLE consolidation_history (
    id TEXT PRIMARY KEY,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    duration_ms INTEGER,
    memories_processed INTEGER,
    memories_decayed INTEGER,
    merged_clusters INTEGER,
    associations_pruned INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## Project Structure

```
mnemonic/
├── cmd/
│   ├── mnemonic/
│   │   ├── main.go                        # Daemon entry point + CLI
│   │   └── ingest.go                      # Bulk ingest subcommand
│   └── benchmark/main.go                  # End-to-end benchmark
├── internal/
│   ├── llm/
│   │   ├── provider.go                    # LLM interface
│   │   └── lmstudio.go                    # LM Studio implementation
│   ├── store/
│   │   ├── store.go                       # Store interface + domain types
│   │   └── sqlite/                        # SQLite implementation (FTS5, embeddings, episodes, patterns)
│   ├── events/
│   │   ├── bus.go                         # EventBus interface
│   │   ├── inmemory.go                    # In-memory implementation
│   │   └── types.go                       # Event type definitions
│   ├── watcher/
│   │   ├── watcher.go                     # Watcher interface
│   │   ├── filesystem/                    # FSEvents (macOS) + fsnotify (Linux)
│   │   ├── terminal/watcher.go            # Shell history polling
│   │   └── clipboard/watcher.go           # Cross-platform clipboard
│   ├── agent/
│   │   ├── agent.go                       # Agent interface
│   │   ├── perception/                    # Layer 1: Watch + heuristic filter
│   │   ├── encoding/                      # Layer 2: LLM compression + linking
│   │   ├── episoding/                     # Layer 3: Temporal episode clustering
│   │   ├── consolidation/                 # Layer 4: Decay, merge, prune
│   │   ├── retrieval/                     # Layer 5: Spread activation + synthesis
│   │   ├── metacognition/                 # Layer 6: Self-reflection + audit
│   │   ├── dreaming/                      # Layer 7: Memory replay + cross-pollination
│   │   ├── abstraction/                   # Layer 8: Patterns → principles → axioms
│   │   ├── orchestrator/                  # Autonomous scheduler + health monitoring
│   │   └── reactor/                       # Event-driven rule engine
│   ├── api/
│   │   ├── server.go                      # HTTP + WebSocket server
│   │   └── routes/                        # REST endpoints (memories, query, graph, etc.)
│   ├── web/
│   │   ├── server.go                      # Static file serving (go:embed)
│   │   └── static/index.html              # Dashboard (D3.js graph, live feed, query tester)
│   ├── mcp/server.go                      # MCP server (10 tools for Claude Code)
│   ├── backup/                            # Export/import logic
│   ├── daemon/daemon.go                   # Service management (macOS LaunchAgent)
│   ├── config/config.go                   # Configuration loading
│   └── logger/logger.go                   # Structured logging
├── sdk/                                   # Python agent SDK (self-evolving assistant)
│   ├── agent/                             # Agent implementation
│   ├── tests/                             # SDK tests
│   └── pyproject.toml
├── migrations/                            # SQLite schema migrations
├── evolution/                             # Agent evolution data (principles, strategies)
├── scripts/                               # Utility scripts (pitch deck generator)
├── tests/                                 # User acceptance tests
├── config.yaml
├── Makefile
└── go.mod
```

---

## Build History

All original build phases are **complete**. Current focus is cross-platform support and graph visualization improvements.

### Completed

- Phase 1: Foundations (config, logging, LLM client, SQLite store)
- Phase 2: Core memory loop (event bus, encoding, retrieval)
- Phase 3: Perception (filesystem, terminal, clipboard watchers + heuristics)
- Phase 4: API + Dashboard (REST API, WebSocket, D3.js graph, query tester)
- Phase 5: Consolidation + Meta (decay, merge, prune, metacognition)
- Phase 6: Polish (CLI, daemon, signal handling)
- Bonus: Episoding, dreaming, abstraction agents; orchestrator; reactor; MCP server; Python agent SDK

---

## Explicitly Deferred to v2+

- **Multi-modal memory** (images, audio) — text-only for v1
- **Cross-device sync** — single machine for v1
- **User preference learning** — needs feedback data from v1
- **Advanced consolidation** (hierarchical memory, schema learning)
- **Database encryption** — air-gapped assumption covers v1
- **Local model fine-tuning** — LM Studio handles v1
- **Native macOS menu bar widget** — web dashboard covers v1, native UI later
- ~~**MCP server integration**~~ — **Done.** 10 MCP tools implemented (`internal/mcp/server.go`)

---

## Key Design Decisions

1. **Embeddings in v1**: Yes. Without them, retrieval is keyword-only and feels dumb. LM Studio serves embedding models with minimal overhead on M4.

2. **Heuristic pre-filter before LLM**: Multi-stage gate saves token budget. 80% of filesystem/terminal events are obvious noise.

3. **Spread activation over pure vector search**: Graph traversal captures meaning relationships (caused_by, contradicts). Embeddings are entry points, not the whole answer.

4. **Budget-constrained consolidation**: Max 100 memories and 5 merges per cycle. Prevents runaway LLM calls. Scales predictably.

5. **Event bus architecture**: Agents never call each other directly. Adding new observers or agents doesn't touch existing code.

6. **API-first**: The daemon is a service. CLI, dashboard, Claude Code, future tools — all talk to the same API. The memory system is infrastructure, not an app.

---

## Verification Plan

After each phase, verify:
- **Phase 1**: LLM client can chat + embed. Store can CRUD memories. FTS5 returns results.
- **Phase 2**: `POST /memories` → encoding pipeline → `POST /query` returns relevant results with spread activation.
- **Phase 3**: Start daemon, edit a file in a watched dir, see raw_memory appear. Run a terminal command, see it captured.
- **Phase 4**: Open `http://localhost:9999`, see live dashboard. WebSocket streams events in real-time. Query tester works.
- **Phase 5**: Run consolidation, verify salience decay. Let system run 24h, check meta-cognition observations.
