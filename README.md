# Mnemonic

A local-first, air-gapped semantic memory system that learns and organizes your knowledge through biologically-inspired cognitive processes.

## What is this?

Mnemonic is an autonomous memory daemon that runs entirely on your machine — no cloud APIs, no vendor lock-in. It watches your filesystem, terminal, and clipboard, automatically capturing and organizing information you interact with. It uses a cognitive architecture inspired by neuroscience with 8 specialized agents that perceive, encode, consolidate, retrieve, reflect, dream, discover patterns, and build hierarchical abstractions.

The system runs local LLMs via LM Studio for semantic understanding, stores everything in SQLite with full-text and vector search, and exposes a REST API with a live web dashboard. It's designed as persistent memory infrastructure — your CLI, your tools, and AI agents (like Claude Code) can all query your memory through a unified interface with 10 MCP tools.

The "analog LLM" vision: the association graph IS the model. Memories build into patterns, patterns into principles, principles into axioms. The system learns, self-corrects, and gets smarter autonomously.

## Quick Start

**Prerequisites:**
- Go 1.23+
- LM Studio running locally (or compatible OpenAI-compatible API)
- CGO enabled (for SQLite)

**Setup:**
```bash
git clone https://github.com/CalebisGross/mnemonic.git
cd mem
# Edit config.yaml: set llm.endpoint, llm.chat_model, llm.embedding_model
make build
./bin/mnemonic start
# Open http://127.0.0.1:9999
```

**First commands:**
```bash
./bin/mnemonic remember "I'm learning about memory systems"
./bin/mnemonic recall "memory"
./bin/mnemonic watch  # Live event stream
./bin/mnemonic status # System health
```

## Architecture

Mnemonic implements an eight-agent cognitive pipeline plus an autonomous orchestrator:

1. **Perception** — Watch filesystem, terminal, clipboard, MCP events. Pre-filter with heuristics (size, patterns, frequency, batch edit detection, recall-aware salience boosting).
2. **Encoding** — LLM-powered compression of raw events into memories. Extract structured concepts, generate embeddings, find related memories, create association links. Heuristic fallback when LLM unavailable.
3. **Episoding** — Cluster raw memories into temporal episodes with LLM synthesis. Claude-aware prompt for AI-assisted development sessions.
4. **Consolidation** — Sleep cycle (every 6h). Decay salience, merge related memories, prune weak associations, extract recurring patterns from memory clusters.
5. **Retrieval** — Spread activation: embed query → find entry points (FTS + embedding) → traverse association graph 3 hops → search patterns and abstractions → LLM synthesis with read-only tool-use (search, follow associations, get details, timeline, project context).
6. **Metacognition** — Periodic self-reflection. Audit memory quality, analyze retrieval feedback, re-embed orphaned memories, trigger consolidation when needed.
7. **Dreaming** — Replay memories, strengthen associations, cross-pollinate across projects, link memories to patterns, generate higher-order insights.
8. **Abstraction** — Build hierarchical knowledge: patterns → principles (level 2) → axioms (level 3). Verify grounding, demote abstractions that lose evidence.

**Orchestrator** — Autonomous scheduler: health monitoring, LLM health checks, DB size monitoring, periodic retrieval self-tests, health report generation (`~/.mnemonic/health.json`).

**Feedback loop** — Helpful recall results strengthen associations and boost salience. Irrelevant results weaken them. The system learns from usage.

All agents communicate via an event bus; none call each other directly.

For architectural deep dive, see [ARCHITECTURE.md](ARCHITECTURE.md).

## CLI Commands

| Category | Command | Purpose |
|----------|---------|---------|
| **Daemon** | `start`, `stop`, `restart`, `serve` | Lifecycle management |
| **Memory** | `remember TEXT` | Store explicit memory |
| **Memory** | `recall QUERY` | Retrieve matching memories |
| **Memory** | `consolidate` | Force consolidation cycle |
| **Memory** | `ingest DIR [--dry-run] [--project NAME]` | Bulk ingest directory into memory |
| **Data** | `export --format json\|sqlite` | Dump all memories |
| **Data** | `import FILE --mode merge\|replace` | Load JSON export |
| **Data** | `backup` | Timestamped backup (keeps 5) |
| **Insights** | `insights` | Memory health report |
| **Insights** | `meta-cycle` | Run metacognition analysis |
| **Insights** | `dream-cycle` | Run dream replay |
| **Insights** | `autopilot` | Show autonomous activity log |
| **MCP** | `mcp` | Run MCP server (stdio) |
| **Monitor** | `status` | System health snapshot |
| **Monitor** | `watch` | Live event stream |
| **Setup** | `install` | macOS LaunchAgent auto-start |
| **Setup** | `version` | Show version |

## MCP Integration

Expose Mnemonic as an MCP server for Claude Code or other AI agents:

**Claude Desktop config** (`~/.config/Claude/claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "mnemonic": {
      "command": "/path/to/bin/mnemonic",
      "args": ["--config", "/path/to/config.yaml", "mcp"]
    }
  }
}
```

**Available MCP tools (10):** `remember`, `recall`, `forget`, `status`, `recall_project`, `recall_timeline`, `session_summary`, `get_patterns`, `get_insights`, `feedback`

See [CLAUDE.md](CLAUDE.md) for Claude Code usage guidelines.

## Configuration

All settings live in `config.yaml`. Key sections:

- **llm** — LM Studio endpoint, chat model, embedding model, timeouts
- **store** — SQLite path, journal mode (use WAL for faster writes)
- **perception** — Watch directories, shell, clipboard; heuristic thresholds
- **encoding** — Concept extraction limits, similarity search
- **consolidation** — Decay rate, salience thresholds, budget (100 memories/cycle max), pattern extraction
- **retrieval** — Spread activation hops, activation decay, result limit, pattern/abstraction inclusion
- **metacognition** — Reflection interval, feedback processing
- **dreaming** — Replay interval, batch size, association boost, cross-project linking, insight generation
- **abstraction** — Interval, min pattern strength, max LLM calls per cycle
- **orchestrator** — Adaptive intervals, max DB size, self-test interval, auto-recovery
- **api** — Server host/port
- **logging** — Level, format, output file

See `config.yaml` for all defaults with inline documentation.

## Dashboard

Open `http://127.0.0.1:9999` for the embedded web UI:

- **Live feed** — Real-time events (raw perceptions, encoded memories, consolidations, dream cycles)
- **Memory browser** — Episodes, association graph, raw events
- **Patterns** — Discovered recurring patterns with strength and evidence counts
- **Abstractions** — Hierarchical knowledge (principles and axioms)
- **Association graph** — D3.js visualization of memory relationships
- **Query tester** — Try searches, see retrieval scores and synthesized responses
- **System health** — LLM status, store stats, project breakdown

## Project Structure

```
internal/
  llm/              LLM provider interface + LM Studio implementation
  embedding/        Embedding provider + caching
  store/            Store interface + SQLite implementation with FTS5
  events/           Event bus (pub/sub)
  watcher/          Filesystem, terminal, clipboard watchers
  agent/            Cognitive agents (perception, encoding, episoding, consolidation,
                      retrieval, metacognition, dreaming, abstraction, orchestrator)
  api/              HTTP + WebSocket server
  web/              Embedded dashboard
  config/           Configuration loading
  logger/           Structured logging
  daemon/           Daemon management (macOS launchd + Linux systemd)
  mcp/              MCP server implementation
  backup/           Backup/restore logic
cmd/mnemonic/       Main entry point
cmd/benchmark/      End-to-end benchmark suite
migrations/         SQLite schema
```

## Development

```bash
make build          # Compile binary
make test           # Run tests
make fmt            # Format code
make vet            # Lint
make clean          # Remove binaries
make check          # fmt + vet
make benchmark      # Build benchmark binary
```

All builds require `CGO_ENABLED=1` for SQLite and `-tags "sqlite_fts5"` for full-text search.

**First-time setup:**
```bash
make setup-hooks    # Configure git pre-commit hooks
```

## Platform Support

| Platform | Status | Notes |
|----------|--------|-------|
| macOS ARM (M-series) | **Full** | Primary development platform |
| macOS x86 | Untested | Should work via CGO cross-compile |
| Linux x86_64 | **Full** | All features including systemd daemon management |
| Windows | Not supported | Clipboard watcher has Windows code; daemon does not |

## License

AGPL-3.0. See [LICENSE](LICENSE) for details.
