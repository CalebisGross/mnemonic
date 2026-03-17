# Mnemonic — Development Guide

Mnemonic is a local-first, air-gapped semantic memory system built in Go. It uses 8 cognitive agents + orchestrator + reactor, SQLite with FTS5 + vector search, and LLMs (LM Studio locally or cloud APIs like Gemini) for semantic understanding.

## Build & Test

```bash
make build                    # go build ...
make test                     # go test ./... -v
make check                    # go fmt + go vet
make run                      # Build and run in foreground (serve mode)
golangci-lint run             # Lint (uses .golangci.yml config)
```

**Version** is injected via ldflags from `Makefile` (managed by release-please). The binary var is in `cmd/mnemonic/main.go`.

## Project Layout

```
cmd/mnemonic/          CLI + daemon entry point
cmd/benchmark/         End-to-end benchmark
cmd/benchmark-quality/ Memory quality IR benchmark
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
  mcp/                 MCP server (13 tools for Claude Code)
  store/               Store interface + SQLite implementation
  llm/                 LLM provider interface + implementations (LM Studio, Gemini/cloud API)
  ingest/              Project ingestion engine
  watcher/             Filesystem (FSEvents/fsnotify), terminal, clipboard
  daemon/              Service management (macOS launchd, Linux systemd, Windows Services)
  updater/             Self-update via GitHub Releases
  events/              Event bus (in-memory pub/sub)
  config/              Config loading (config.yaml)
  logger/              Structured logging (slog)
  backup/              Export/import
sdk/                   Python agent SDK (self-evolving assistant)
  agent/evolution/     Agent evolution data (created at runtime, gitignored)
  agent/evolution/examples/  Example evolution data for reference
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
- **New API route:** Add handler in `internal/api/routes/`, register in `internal/api/server.go`.
- **New MCP tool:** Add to `internal/mcp/server.go` tool registration.

## Platform Support

| Platform | Status |
|----------|--------|
| macOS ARM | Full support (primary dev platform) |
| Linux x86_64 | Supported — `serve`, `install`, `start`, `stop`, `uninstall` all work via systemd |
| Windows x86_64 | Supported — `serve`, `install`, `start`, `stop`, `uninstall` work via Windows Services |

## Known Issues

See [GitHub Issues](https://github.com/appsprout-dev/mnemonic/issues) for tracked bugs.

---

## MCP Tools Available

You have 13 tools via the `mnemonic` MCP server:

| Tool | When to Use |
|------|-------------|
| `remember` | Store decisions, errors, insights, learnings |
| `recall` | Search for relevant memories before starting work |
| `forget` | Archive irrelevant memories |
| `status` | Check memory system health and stats |
| `recall_project` | Get project-specific context and patterns |
| `recall_timeline` | See what happened in a time range |
| `session_summary` | Summarize current/recent session |
| `get_patterns` | View discovered recurring patterns |
| `get_insights` | View metacognition observations and abstractions |
| `feedback` | Report recall quality (helps system learn) |
| `audit_encodings` | Review recent encoding quality and suggest improvements |
| `coach_local_llm` | Write coaching guidance to improve local LLM prompts |
| `ingest_project` | Bulk-ingest a project directory into memory |

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
