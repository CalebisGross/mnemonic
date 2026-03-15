# Changelog

All notable changes to Mnemonic will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/). Versioning follows [Semantic Versioning](https://semver.org/).

## [0.14.1](https://github.com/AppSprout-dev/mnemonic/compare/v0.14.0...v0.14.1) (2026-03-14)


### Bug Fixes

* improve update badge contrast and readability ([0ecad1c](https://github.com/AppSprout-dev/mnemonic/commit/0ecad1ca86cb457754607bea1f1290fa9f443a91))
* update badge visibility and daemon restart ([e8fe843](https://github.com/AppSprout-dev/mnemonic/commit/e8fe843ea3befc575d563fab3eec3cdc1fc40339))
* update badge visibility and daemon restart logic ([da0d41c](https://github.com/AppSprout-dev/mnemonic/commit/da0d41c29115eb3a19743e1e346bab9619d25997))

## [0.14.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.13.0...v0.14.0) (2026-03-14)


### Features

* add self-update mechanism (CLI + dashboard) ([fd1c814](https://github.com/AppSprout-dev/mnemonic/commit/fd1c814d3c158283a6821b53e74a2272b48de7dc))
* add self-update mechanism (CLI + dashboard) ([bb9497b](https://github.com/AppSprout-dev/mnemonic/commit/bb9497bae0260cb5f7e12d097feab458f0a51fdd))
* show version in dashboard header ([6c0a3e1](https://github.com/AppSprout-dev/mnemonic/commit/6c0a3e130fd8795bce6f23bee30a8821513061be))
* show version in dashboard header ([c7208ab](https://github.com/AppSprout-dev/mnemonic/commit/c7208ab2c0ddc89835d3b3e255fb061017e8424a))


### Bug Fixes

* add summary fallback in consolidation createGist ([24940b7](https://github.com/AppSprout-dev/mnemonic/commit/24940b7940b0e5410c271ccba323991a821b4d95))
* add summary fallback in consolidation createGist ([697c32c](https://github.com/AppSprout-dev/mnemonic/commit/697c32c7b2dcb315803b3f1f20b6b44da803e1c5)), closes [#133](https://github.com/AppSprout-dev/mnemonic/issues/133)
* add WIN_HOME fallback and restore env overrides ([6e6f4e4](https://github.com/AppSprout-dev/mnemonic/commit/6e6f4e4ce2597303ad4ddbe03c44f4a0df13bb37))
* resolve MSYS2 make HOME mismatch breaking Go build on Windows ([22c5958](https://github.com/AppSprout-dev/mnemonic/commit/22c59588cbfbd245c425cce7d9d37268d9316412))
* resolve MSYS2 make HOME mismatch breaking Go build paths on Windows ([189a38c](https://github.com/AppSprout-dev/mnemonic/commit/189a38cecb144f95de75e41bf1db2924540408f4))
* use go-version-file in CI and release workflows ([0cf75c1](https://github.com/AppSprout-dev/mnemonic/commit/0cf75c11d07ed9a0efc0e4a4e173d8706af835c5))
* use go-version-file in CI and release workflows ([85bb335](https://github.com/AppSprout-dev/mnemonic/commit/85bb335629fbeacd0531cb5d4be4a6decf0b27f9))

## [0.13.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.12.0...v0.13.0) (2026-03-14)


### Features

* unified project identity system ([c9702c0](https://github.com/AppSprout-dev/mnemonic/commit/c9702c0e39d2d34368bcfb48d4434296cdce71b8))
* unified project identity system with config-driven resolver ([7043984](https://github.com/AppSprout-dev/mnemonic/commit/70439844cbf807d39840137d62f587c4ab847376))


### Bug Fixes

* resolve all golangci-lint v2 issues and pin CI to v2 ([abba510](https://github.com/AppSprout-dev/mnemonic/commit/abba510760eb8ba043b7986422f33d5067012a17))

## [0.12.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.11.1...v0.12.0) (2026-03-14)


### Features

* add full Windows platform support ([ee6ad90](https://github.com/AppSprout-dev/mnemonic/commit/ee6ad90a66c585b326773ae542d0deb3ead399eb))
* add timeline tag click-to-filter and perception project inference ([8cb004e](https://github.com/AppSprout-dev/mnemonic/commit/8cb004e69d5f62480ee5c42a66fe9500583bdad4))
* timeline tag filtering + perception project inference ([071ab88](https://github.com/AppSprout-dev/mnemonic/commit/071ab88af2d27c0971d45065479cfa591dc2d802))


### Bug Fixes

* address PR review — restore SIGTERM on Unix, fix CI check names ([03020a7](https://github.com/AppSprout-dev/mnemonic/commit/03020a79e75c4590f6288dad415f7f77d227395c))
* resolve cmd.Wait() double-call race and platform-guard SIGTERM ([c8e0749](https://github.com/AppSprout-dev/mnemonic/commit/c8e0749c434a2c0f792d4957e480a07897a255a9))

## [0.11.1](https://github.com/AppSprout-dev/mnemonic/compare/v0.11.0...v0.11.1) (2026-03-14)


### Bug Fixes

* use GitHub App token for release-please ([659bde2](https://github.com/AppSprout-dev/mnemonic/commit/659bde2a102ef94b0f343265dca34e1a57015c62))
* use GitHub App token in release-please workflow ([049e6cb](https://github.com/AppSprout-dev/mnemonic/commit/049e6cb4a4178ae134fc64415ea1b0baa8cdc325))

## [0.11.0](https://github.com/CalebisGross/mnemonic/compare/v0.10.0...v0.11.0) (2026-03-13)


### Features

* migrate repo to appsprout-dev org ([b11c086](https://github.com/CalebisGross/mnemonic/commit/b11c08676c1bf95f35e5b1c6fa1d23dc389e3e64))
* migrate repo to appsprout-dev org ([c29dcf6](https://github.com/CalebisGross/mnemonic/commit/c29dcf604421d7bf53d79c0d53514361ddd5970d))

## [0.10.0](https://github.com/appsprout-dev/mnemonic/compare/v0.9.0...v0.10.0) (2026-03-13)


### Features

* add ISO 8601 timestamps to evolution files ([cf76e54](https://github.com/appsprout-dev/mnemonic/commit/cf76e5499a38f6dc5d20fec4fd611ce5e039aaca))
* add ISO 8601 timestamps to evolution files ([f1e42dc](https://github.com/appsprout-dev/mnemonic/commit/f1e42dcfcb97d6b3d763fcd406f30a5dab2a618f))

## [0.9.0](https://github.com/appsprout-dev/mnemonic/compare/v0.8.2...v0.9.0) (2026-03-13)


### Features

* config sweep + full pipeline benchmark ([de6fc47](https://github.com/appsprout-dev/mnemonic/commit/de6fc47564cb1448c2798ee7e0aad172a476345f))

## [0.8.2](https://github.com/appsprout-dev/mnemonic/compare/v0.8.1...v0.8.2) (2026-03-13)


### Bug Fixes

* render markdown in evolution timeline changelog ([f675737](https://github.com/appsprout-dev/mnemonic/commit/f675737965c7565ed663fb6916b20fa99705ee14))
* render markdown in evolution timeline changelog ([95486a2](https://github.com/appsprout-dev/mnemonic/commit/95486a215a9f9b445b8f9035d7252de8c41c39e7))

## [0.8.1](https://github.com/appsprout-dev/mnemonic/compare/v0.8.0...v0.8.1) (2026-03-13)


### Bug Fixes

* correct CHANGELOG date, agent counts, and release-please marker ([65b64a6](https://github.com/appsprout-dev/mnemonic/commit/65b64a619ca91f956c3d6672ad64672a58e3e941))

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
