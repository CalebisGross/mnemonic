# Changelog

All notable changes to Mnemonic will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/). Versioning follows [Semantic Versioning](https://semver.org/).

## [0.8.2](https://github.com/CalebisGross/mnemonic/compare/v0.8.1...v0.8.2) (2026-03-13)


### Bug Fixes

* render markdown in evolution timeline changelog ([f675737](https://github.com/CalebisGross/mnemonic/commit/f675737965c7565ed663fb6916b20fa99705ee14))
* render markdown in evolution timeline changelog ([95486a2](https://github.com/CalebisGross/mnemonic/commit/95486a215a9f9b445b8f9035d7252de8c41c39e7))

## [0.8.1](https://github.com/CalebisGross/mnemonic/compare/v0.8.0...v0.8.1) (2026-03-13)


### Bug Fixes

* correct CHANGELOG date, agent counts, and release-please marker ([65b64a6](https://github.com/CalebisGross/mnemonic/commit/65b64a619ca91f956c3d6672ad64672a58e3e941))

## [0.8.0] - 2026-03-13

### Added

- Multi-theme dashboard selector: Midnight, Ember, Nord, Slate, Parchment (persists in localStorage)
- Live-updating dashboard with real-time data refresh via WebSocket
- Memory source tracking with hoverable tags in timeline
- LLM usage monitoring with per-agent token tracking and dashboard display
- Gemini API support with API key authentication (any OpenAI-compatible provider)
- Optional bearer token API authentication (`mnemonic generate-token`)
- Project ingestion system (CLI `ingest`, API endpoint, MCP tool)
- `mnemonic diagnose` command for config/DB/LLM/disk diagnostics
- Embedding index scalability monitoring and drift detection
- Database integrity checks and disaster recovery
- Config validation, safe defaults, and configurable busy timeout
- Memory quality benchmark with scenario-based IR metrics
- Memory deduplication, decay, and TTL cleanup
- Hard-reject filters for desktop noise
- Sensitive file filtering in ingest, watcher, and perception
- User-facing documentation: troubleshooting, LM Studio setup, backup/restore
- Test coverage for llm, backup, and api/routes packages
- Release pipeline with multi-platform builds and Homebrew formula
- Conventional commits and release-please for automated versioning

### Fixed

- Graph visualization: reworked D3 force layout, adaptive forces, fit-to-screen, responsive SVG
- Dashboard XSS and silent error handling
- Dashboard badge colors converted from hardcoded `rgba()` to `color-mix()` for theme compatibility
- Pattern deduplication with embedding-level checks
- Noisy memory ingestion filtering
- Windows compilation errors
- N+1 queries, connection pooling, sentinel errors
- Release workflow: replaced deprecated macOS runner

### Changed

- Standardized exit codes and user-facing error messages
- Improved recall quality: pattern cleanup, concise synthesis
- Tuned config defaults for Gemini cloud API (higher concurrency, larger context windows)
- Updated all documentation to reflect current architecture and features

## [0.7.0] - 2025-03-11

### Added
- Gemini API support with API key authentication
- LLM usage monitoring with dashboard, API, and per-agent tracking
- Live-updating dashboard with real-time data refresh
- Multi-theme selector: Midnight, Ember, Nord, Slate, Parchment (persists in localStorage)
- Memory source tracking — `source` field on encoded memories, backfilled from raw observations, rendered as hoverable tags in timeline
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
- Graph visualization: reworked D3 force layout, adaptive forces, fit-to-screen, responsive SVG, label visibility
- Dashboard XSS and silent error handling
- Dashboard badge colors converted from hardcoded `rgba()` to `color-mix()` for theme compatibility
- Pattern deduplication with embedding-level checks
- Noisy memory ingestion filtering
- Windows compilation errors
- N+1 queries, connection pooling, sentinel errors
- Release workflow: replaced deprecated macOS runner

### Changed
- Standardized exit codes and user-facing error messages
- Improved recall quality: pattern cleanup, concise synthesis
- Tuned config defaults for Gemini cloud API (higher concurrency, larger context windows)

## [0.6.0] - 2025-02-01

Initial tracked release. Core memory system with 9 cognitive agents, orchestrator, reactor, SQLite FTS5 + vector search, MCP server, and embedded dashboard.
