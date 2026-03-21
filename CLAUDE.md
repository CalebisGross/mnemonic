# Mnemonic — Development Guide

Mnemonic is a local-first, air-gapped semantic memory system built in Go. It uses 8 cognitive agents + orchestrator + reactor, SQLite with FTS5 + vector search, and LLMs (LM Studio locally or cloud APIs like Gemini) for semantic understanding.

## Build & Test

```bash
make build                    # go build ...
make test                     # go test ./... -v
make check                    # go fmt + go vet
make run                      # Build and run in foreground (serve mode)
make lifecycle-test           # Build + run full lifecycle simulation
golangci-lint run             # Lint (uses .golangci.yml config)
```

**Version** is injected via ldflags from `Makefile` (managed by release-please). The binary var is in `cmd/mnemonic/main.go`.

## Project Layout

```
cmd/mnemonic/          CLI + daemon entry point
cmd/benchmark/         End-to-end benchmark
cmd/benchmark-quality/ Memory quality IR benchmark
cmd/lifecycle-test/    Full lifecycle simulation (install → 3 months)
internal/
  agent/               8 cognitive agents + orchestrator + reactor
    perception/        Watch filesystem/terminal/clipboard, heuristic filter
    encoding/          LLM compression, concept extraction, association linking
    episoding/         Temporal episode clustering
    consolidation/     Decay, merge, prune (sleep cycle)
    retrieval/         Spread activation + LLM synthesis with tool-use
    metacognition/     Self-reflection, feedback processing, audit
    dreaming/          Memory replay, cross-pollination, insight generation
    abstraction/       Patterns → principles → axioms
    orchestrator/      Autonomous scheduler, health monitoring
    reactor/           Event-driven rule engine
  api/                 REST API server + routes
  web/                 Embedded dashboard (single-page app, D3.js charts)
  mcp/                 MCP server (23 tools for Claude Code)
  store/               Store interface + SQLite implementation
  llm/                 LLM provider interface + implementations (LM Studio, Gemini/cloud API)
  ingest/              Project ingestion engine
  watcher/             Filesystem (FSEvents/fsnotify), terminal, clipboard
  daemon/              Service management (macOS launchd, Linux systemd, Windows Services)
  updater/             Self-update via GitHub Releases
  events/              Event bus (in-memory pub/sub)
  config/              Config loading (config.yaml)
  logger/              Structured logging (slog)
  concepts/            Shared concept extraction (paths, commands, event types)
  backup/              Export/import
  testutil/            Shared test infrastructure (stub LLM provider)
sdk/                   Python agent SDK (self-evolving assistant)
  agent/evolution/     Agent evolution data (created at runtime, gitignored)
  agent/evolution/examples/  Example evolution data for reference
training/              Mnemonic-LM training infrastructure
  scripts/             Training, sweep, bisection, data download scripts
  configs/             Data mix config (pretrain_mix.yaml)
  docs/                Experiment registry, analysis docs
  data/                Tokenized pretraining shards (gitignored)
  sweep_results.tsv    HP sweep results log
  probe_results.tsv    Short probe results from LR bisection
migrations/            SQLite schema migrations
scripts/               Utility scripts
```

## Conventions

- **Event bus architecture:** Agents communicate via events, never direct calls. To add behavior, subscribe to events in the bus.
- **Store interface:** All data access goes through `store.Store` interface. The SQLite implementation is in `internal/store/sqlite/`.
- **Error handling:** Wrap errors with context: `fmt.Errorf("encoding memory %s: %w", id, err)`
- **Platform-specific code:** Use Go build tags (`//go:build darwin`, `//go:build !darwin`). See `internal/watcher/filesystem/` for examples.
- **Config:** All tunables live in `config.yaml`. Add new fields to `internal/config/config.go` struct.

## Adding Things

- **New agent:** Implement `agent.Agent` interface, register in `cmd/mnemonic/main.go` serve pipeline.
- **New CLI command:** Add case to the command switch in `cmd/mnemonic/main.go`.
- **New API route:** Add handler in `internal/api/routes/`, register in `internal/api/server.go`. Existing routes include `/api/v1/activity` (watcher concept tracker for MCP sync).
- **New MCP tool:** Add to `internal/mcp/server.go` tool registration.

## Platform Support

| Platform | Status |
|----------|--------|
| macOS ARM | Full support (primary dev platform) |
| Linux x86_64 | Supported — `serve`, `install`, `start`, `stop`, `uninstall` all work via systemd |
| Windows x86_64 | Supported — `serve`, `install`, `start`, `stop`, `uninstall` work via Windows Services |

## Training (Mnemonic-LM)

Training scripts live in `training/scripts/` and require the **Felix-LM venv**:

```bash
source ~/Projects/felixlm/.venv/bin/activate
```

Key scripts:

- `train_mnemonic_lm.py` — Main training script (imports Felix-LM v3 from `~/Projects/felixlm`)
- `run_sweep.sh` — Run HP sweep configs sequentially with auto-logging to TSV
- `bisect_lr.sh` — Binary search for optimal LR using short probes + full confirmation
- `validate.py` — Quality gate pipeline for fine-tuning data

All experiments must be pre-registered in `training/docs/experiment_registry.md` before running. See `.claude/rules/scientific-method.md` and `.claude/rules/experiment-logging.md`.

## Known Issues

See [GitHub Issues](https://github.com/appsprout-dev/mnemonic/issues) for tracked bugs.

---

## MCP Tools Available

You have 21 tools via the `mnemonic` MCP server:

| Tool | When to Use |
|------|-------------|
| `remember` | Store decisions, errors, insights, learnings (returns raw ID + salience) |
| `recall` | Semantic search with spread activation (`explain`, `include_associations`, `format`, `type`, `synthesize` params) |
| `batch_recall` | Run multiple recall queries in parallel — ideal for session start |
| `get_context` | Proactive suggestions based on recent daemon activity — call at natural breakpoints |
| `forget` | Archive irrelevant memories |
| `amend` | Update a memory's content in place (preserves associations, history, salience) |
| `check_memory` | Inspect a memory's encoding status, concepts, and associations |
| `status` | System health, encoding pipeline status, source distribution |
| `recall_project` | Get project-specific context and patterns |
| `recall_timeline` | See what happened in a time range |
| `recall_session` | Retrieve all memories from a specific MCP session |
| `list_sessions` | List recent sessions with time range and memory count |
| `session_summary` | Summarize current/recent session |
| `get_patterns` | View discovered recurring patterns |
| `get_insights` | View metacognition observations and abstractions |
| `feedback` | Report recall quality (drives ranking, can auto-suppress noisy memories) |
| `audit_encodings` | Review recent encoding quality and suggest improvements |
| `coach_local_llm` | Write coaching guidance to improve local LLM prompts |
| `ingest_project` | Bulk-ingest a project directory into memory |
| `exclude_path` | Add a watcher exclusion pattern at runtime |
| `list_exclusions` | List all runtime watcher exclusion patterns |
| `dismiss_pattern` | Archive a stale or irrelevant pattern to stop it surfacing in recall |
| `create_handoff` | Store structured session handoff notes (high salience, surfaced by recall_project) |

### At Session Start

- Use `recall_project` to load context for the current project
- Use `recall` with relevant keywords to find prior decisions

### During Work

- `remember` decisions with `type: "decision"` — e.g., "chose SQLite over Postgres for simplicity"
- `remember` errors with `type: "error"` — e.g., "nil pointer in auth middleware, fixed with guard clause"
- `remember` insights with `type: "insight"` — e.g., "spread activation works best with 3 hops max"
- `remember` learnings with `type: "learning"` — e.g., "Go's sql.NullString needed for nullable columns"

### After Recalls

- Use `feedback` to rate recall quality — this helps the system improve
- `helpful` = memories were relevant and useful
- `partial` = some relevant, some not
- `irrelevant` = memories didn't help

### Memory Types

When using `remember`, set the `type` field:

- `decision` — architectural choices, tradeoffs, "we chose X because Y"
- `error` — bugs found, error patterns, debugging insights
- `insight` — realizations about code, architecture, or process
- `learning` — new knowledge, API behaviors, framework quirks
- `general` — everything else (default)
