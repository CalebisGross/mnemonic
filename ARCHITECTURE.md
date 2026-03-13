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
| LLM runtime | LM Studio / Gemini API / any OpenAI-compatible provider | Local or cloud, model-agnostic |
| Embeddings | Provider-supplied (e.g. embeddinggemma, Gemini embedding) | Separate model slot, local or cloud semantic search |
| Platform | macOS ARM (primary), Linux x86_64 (next), Windows (planned) | Cross-platform via build tags |

---

## Core Abstractions (Interfaces)

These are the load-bearing walls. Get them right and everything downstream is swappable.

### 1. `llm.Provider`
```go
type Provider interface {
    Complete(ctx, req CompletionRequest) (CompletionResponse, error)
    Embed(ctx, text string) ([]float32, error)
    BatchEmbed(ctx, texts []string) ([][]float32, error)
    Health(ctx) error
    ModelInfo(ctx) (ModelMetadata, error)
}
```
- Implementations: LM Studio (local, OpenAI-compatible), Gemini API (cloud), any OpenAI-compatible endpoint
- Instrumented wrapper tracks per-call token usage, latency, and caller identity

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

- All 8 cognitive agents + orchestrator implement this (reactor is a separate engine)
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

### Layer 5 — Episoding (Temporal Clustering)

Clusters raw memories into time-window episodes (default 10-minute windows). When a window closes:

1. Collect raw memories in the time range
2. LLM synthesizes a title, narrative, and emotional tone
3. Extract concepts, files modified, and event timeline
4. Create episode record linking to constituent memories
5. Emit `EpisodeClosed` event

Claude-aware prompt for AI-assisted development sessions — recognizes coding patterns, debugging flows, and decision-making.

### Layer 6 — Meta-Cognition (Self-Reflection)

Runs periodically (default every 4 hours):

- Memory growth patterns (topic concentration)
- Retrieval success/failure rate (via user feedback)
- Association graph health (isolated clusters, density)
- Consolidation effectiveness
- Re-embed orphaned memories, trigger consolidation when needed
- Log observations to `meta_observations` table

### Layer 7 — Dreaming (Memory Replay)

Runs periodically (default every 1 hour). Replays and cross-pollinates memories:

1. Select batch of recent memories (default 60)
2. Strengthen associations between related memories across projects
3. Link memories to discovered patterns
4. Generate higher-order insights from memory clusters
5. Prune noise associations (below threshold)

### Layer 8 — Abstraction (Hierarchical Knowledge)

Runs periodically (default every 2 hours). Builds hierarchical knowledge:

- **Level 1 — Patterns**: Recurring themes extracted from memory clusters
- **Level 2 — Principles**: Generalizations across patterns
- **Level 3 — Axioms**: Fundamental truths with high confidence

Abstractions are grounded in evidence. Those that lose supporting evidence are demoted or archived.

### Layer 9 — Reactor (Event-Driven Rules)

Event-driven rule engine that fires condition-action chains in response to system events:

- Trigger consolidation when DB grows too large
- Kick off dreaming when an episode closes
- Escalate health alerts when agents fail repeatedly

---

## API Surface

### HTTP REST (`http://localhost:9999/api/v1/`)

```
GET    /health                 System health check
GET    /stats                  Memory statistics

POST   /memories               Create raw memory (explicit user input)
GET    /memories                List memories (with filters)
GET    /memories/:id            Get specific memory
GET    /memories/:id/context    Get memory with surrounding context
GET    /raw/:id                 Get raw (unprocessed) memory

GET    /episodes                List episodes
GET    /episodes/:id            Get specific episode

POST   /query                   Query memories (spread activation + LLM synthesis)
POST   /feedback                Submit retrieval feedback

POST   /consolidation/run       Force consolidation cycle
POST   /ingest                  Bulk-ingest a directory

GET    /insights                Meta-cognition observations
GET    /patterns                Discovered patterns
GET    /abstractions            Hierarchical abstractions
GET    /projects                Project summaries

GET    /llm/usage               LLM token usage by agent

GET    /graph                   Association graph for D3.js visualization

GET    /agent/evolution          Agent SDK evolution state (conditional)
GET    /agent/changelog          Agent evolution changelog (conditional)
GET    /agent/sessions           Agent session history (conditional)
GET    /agent/config             Agent SDK configuration (conditional)
```

Optional bearer token authentication via `Authorization: Bearer <token>` header (configure with `mnemonic generate-token`).

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
- LLM usage monitoring (per-agent token consumption and cost)
- Memory source tags (hoverable, showing origin: filesystem, terminal, clipboard, MCP, consolidation)
- 5 themes: Midnight, Ember, Nord, Slate, Parchment (persists in localStorage)
- Agent SDK dashboard: evolution state, principles, strategies, session timeline, chat

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

-- Episodes (temporal clusters of raw events)
CREATE TABLE episodes (
    id TEXT PRIMARY KEY,
    title TEXT,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    summary TEXT,
    narrative TEXT,
    salience REAL DEFAULT 0.5,
    emotional_tone TEXT,
    state TEXT DEFAULT 'open',       -- open | closed
    concepts JSON DEFAULT '[]',
    files_modified JSON DEFAULT '[]',
    event_timeline JSON DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Multi-resolution memory (gist / narrative / detail)
CREATE TABLE memory_resolutions (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    gist TEXT,
    narrative TEXT,
    detail_raw_ids JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Structured concept extraction
CREATE TABLE concept_sets (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    topics JSON,
    entities JSON,
    actions JSON,
    causality JSON,
    significance TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Patterns discovered through consolidation/dreaming
CREATE TABLE patterns (
    id TEXT PRIMARY KEY,
    pattern_type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    evidence_ids JSON DEFAULT '[]',
    strength REAL DEFAULT 0.5,
    project TEXT,
    concepts JSON DEFAULT '[]',
    embedding BLOB,
    state TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Abstractions: hierarchical knowledge (level 1=pattern, 2=principle, 3=axiom)
CREATE TABLE abstractions (
    id TEXT PRIMARY KEY,
    level INTEGER DEFAULT 1,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    parent_id TEXT,
    source_pattern_ids JSON DEFAULT '[]',
    confidence REAL DEFAULT 0.5,
    concepts JSON DEFAULT '[]',
    embedding BLOB,
    state TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- System metadata key-value store
CREATE TABLE system_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- LLM usage tracking (per-call instrumentation)
CREATE TABLE llm_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    operation TEXT NOT NULL,
    caller TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    success INTEGER NOT NULL DEFAULT 1
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
│   ├── benchmark/main.go                  # End-to-end benchmark
│   └── benchmark-quality/                 # Memory quality IR benchmark
├── internal/
│   ├── llm/
│   │   ├── provider.go                    # LLM interface
│   │   ├── lmstudio.go                    # LM Studio / OpenAI-compatible implementation
│   │   ├── instrumented.go                # Usage-tracking wrapper (tokens, latency, caller)
│   │   └── pricing.go                     # Token cost estimation
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
│   ├── ingest/                            # Project ingestion engine
│   ├── mcp/server.go                      # MCP server (13 tools for Claude Code)
│   ├── backup/                            # Export/import logic
│   ├── daemon/                            # Service management (macOS LaunchAgent + Linux systemd)
│   ├── config/config.go                   # Configuration loading
│   └── logger/logger.go                   # Structured logging
├── sdk/                                   # Python agent SDK (self-evolving assistant)
│   ├── agent/                             # Agent implementation
│   ├── tests/                             # SDK tests
│   └── pyproject.toml
├── migrations/                            # SQLite schema migrations
├── scripts/                               # Utility scripts
├── config.yaml
├── Makefile
└── go.mod
```

---

## Build History

All original build phases are **complete**. Current focus is SDK features, dashboard polish, and recall quality.

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
- ~~**MCP server integration**~~ — **Done.** 13 MCP tools implemented (`internal/mcp/server.go`)

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
