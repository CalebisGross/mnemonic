# Changelog

All notable changes to Mnemonic will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/). Versioning follows [Semantic Versioning](https://semver.org/).

## [0.7.0] - 2025-03-11

### Added
- Gemini API support with API key authentication
- LLM usage monitoring with dashboard, API, and per-agent tracking
- Optional bearer token API authentication
- Embedding index scalability monitoring
- Embedding drift detection (warns on LLM model changes)
- Database integrity checks and disaster recovery
- Config validation, safe defaults, and configurable busy timeout
- `mnemonic diagnose` command
- LM Studio startup warning and encoding queue status
- Project ingestion system (CLI, API, MCP tool)
- Memory quality benchmark with scenario-based IR metrics
- Memory deduplication, decay, and TTL cleanup
- Hard-reject filters for desktop noise
- User-facing documentation: troubleshooting, LM Studio setup, backup/restore
- Test coverage for llm, backup, and api/routes packages
- Release pipeline with multi-platform builds and Homebrew formula
- `make release` target for repeatable version bumps
- Sensitive file filtering in ingest, watcher, and perception

### Fixed
- Graph visualization: adaptive forces, fit-to-screen, responsive SVG, label visibility
- Dashboard XSS and silent error handling
- Pattern deduplication with embedding-level checks
- Noisy memory ingestion filtering
- Windows compilation errors
- N+1 queries, connection pooling, sentinel errors
- Release workflow: replaced deprecated macOS runner

### Changed
- Standardized exit codes and user-facing error messages
- Improved recall quality: pattern cleanup, concise synthesis

## [0.6.0] - 2025-02-01

Initial tracked release. Core memory system with 9 cognitive agents, orchestrator, reactor, SQLite FTS5 + vector search, MCP server, and embedded dashboard.
