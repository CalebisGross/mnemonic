# Changelog

All notable changes to Mnemonic will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/). Versioning follows [Semantic Versioning](https://semver.org/).

## [0.18.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.17.0...v0.18.0) (2026-03-18)


### Features

* implement --llm flag for benchmark-quality ([082ce53](https://github.com/AppSprout-dev/mnemonic/commit/082ce53475e08312bd69385ef2cfe6b9ebeb8fff))
* implement --llm flag for benchmark-quality with real Gemini provider ([35e71ea](https://github.com/AppSprout-dev/mnemonic/commit/35e71ea056c7430cff5aedd181732d66d8867af2)), closes [#173](https://github.com/AppSprout-dev/mnemonic/issues/173)
* scaffold EmbeddedProvider for in-process llama.cpp inference ([df32fc9](https://github.com/AppSprout-dev/mnemonic/commit/df32fc9ed2ad1205117acefa1da3f695916a94e8)), closes [#174](https://github.com/AppSprout-dev/mnemonic/issues/174)
* scaffold EmbeddedProvider for llama.cpp integration ([74d7084](https://github.com/AppSprout-dev/mnemonic/commit/74d708472c0df28e7386164405321c95e0135a0c))


### Bug Fixes

* make API key file fallback and tests Windows-compatible ([25a6135](https://github.com/AppSprout-dev/mnemonic/commit/25a6135d60c860f2c1f5e5f4bebce4eb3c2d17a2))
* stop capturing failed LLM calls and add API key file fallback ([619b4f7](https://github.com/AppSprout-dev/mnemonic/commit/619b4f72c91e9b736d92440fb8dc9c00e3e82815))
* stop capturing failed LLM calls and add API key file fallback ([3ce5822](https://github.com/AppSprout-dev/mnemonic/commit/3ce5822690cae486ed18d7b8795ab5f1b3e6b153))

## [0.17.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.16.0...v0.17.0) (2026-03-17)


### Features

* clickable version label + changelog link in dashboard ([b32c49c](https://github.com/AppSprout-dev/mnemonic/commit/b32c49caea6ba0b72286b49f6011462ee3cebebb))
* make version label a clickable changelog link in dashboard ([e615d95](https://github.com/AppSprout-dev/mnemonic/commit/e615d95d6983b1580c0c8a8c79f1cc5696bf55b3))


### Bug Fixes

* add missing type column to SearchByFullText FTS query ([48c1d95](https://github.com/AppSprout-dev/mnemonic/commit/48c1d958f49cfecbb2f35f682c69cee64cf2a16b))
* add missing type column to SearchByFullText FTS query ([fd82fb7](https://github.com/AppSprout-dev/mnemonic/commit/fd82fb7e802375d1c0b70bb48981a912fe6105dc))
* propagate memory type from raw_memories to memories table and API ([c84fdbf](https://github.com/AppSprout-dev/mnemonic/commit/c84fdbff50d355deac382f058347546c97aec97d))
* propagate memory type to API and web UI ([49d89c3](https://github.com/AppSprout-dev/mnemonic/commit/49d89c3f9cfd077355259812a7fbdd3e4ee8e720))
* strip all non-alphanumeric chars in FTS query sanitizer ([52bb990](https://github.com/AppSprout-dev/mnemonic/commit/52bb990fe7897502bb12ec5663ac7fad0adf0908))
* strip FTS5 metacharacters from query sanitizer ([9907c4b](https://github.com/AppSprout-dev/mnemonic/commit/9907c4bb319f10f615da73bbc3a0281ef9705184))

## [0.16.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.15.1...v0.16.0) (2026-03-17)


### Features

* add audit_mix.py for pretraining data validation ([61c99de](https://github.com/AppSprout-dev/mnemonic/commit/61c99deb8c0566fbd483d1b0b163248277b797a3))
* add Felix-LM v3 training bridge with streaming shard reader ([93f26ee](https://github.com/AppSprout-dev/mnemonic/commit/93f26ee34cd24064abf29de446ea63a5fed8641d))
* add MixedPretrainDataset for multi-source token shard reading ([0908908](https://github.com/AppSprout-dev/mnemonic/commit/090890891c38ff86d79aa4ce5ebcc02255755b03)), closes [#156](https://github.com/AppSprout-dev/mnemonic/issues/156)
* pretraining data pipeline and training bridge for mnemonic-LM ([5d3635a](https://github.com/AppSprout-dev/mnemonic/commit/5d3635ad916a69529f8e0db1e9e028b7a15c26ff))


### Bug Fixes

* drop darwin/amd64 release build ([3230dc0](https://github.com/AppSprout-dev/mnemonic/commit/3230dc07f0a0c8824674d1a78bcd09caab0222ee))
* drop darwin/amd64 release build ([d979854](https://github.com/AppSprout-dev/mnemonic/commit/d979854d70bf3ed12047b4a8857c5748ecc66e24))
* resolve tokenizer path and remove GPT-2 fallback ([2614f96](https://github.com/AppSprout-dev/mnemonic/commit/2614f9609d9179fb5d854d61dee09499032d5308))

## [0.15.1](https://github.com/AppSprout-dev/mnemonic/compare/v0.15.0...v0.15.1) (2026-03-17)


### Bug Fixes

* use macos-13 runner for darwin/amd64 release builds ([d7c70d4](https://github.com/AppSprout-dev/mnemonic/commit/d7c70d44ce2516458b953a44b093ee22a94d0b21))
* use macos-13 runner for darwin/amd64 release builds ([5936e95](https://github.com/AppSprout-dev/mnemonic/commit/5936e957bd1844025fbb69a4473a39f945e4f9e0))

## [0.15.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.14.2...v0.15.0) (2026-03-17)


### Features

* add PDF and DOCX extraction to ingest pipeline ([83b9ce0](https://github.com/AppSprout-dev/mnemonic/commit/83b9ce0395a6c3103b0245a5a5b4bb9d884f2f2d))
* add PDF and DOCX extraction to ingest pipeline ([b8c1c2c](https://github.com/AppSprout-dev/mnemonic/commit/b8c1c2c5b3e3792b29b7080ab8d5006be6a46e75)), closes [#158](https://github.com/AppSprout-dev/mnemonic/issues/158)
* add PPTX, RTF, and ODT extractors to ingest pipeline ([50b37f9](https://github.com/AppSprout-dev/mnemonic/commit/50b37f9b282d8bd7bd8c761fbafc01c4c8f0ed3a))
* add PPTX, RTF, and ODT extractors to ingest pipeline ([9761469](https://github.com/AppSprout-dev/mnemonic/commit/9761469dbba6e1b5f06d1446a5df8f72f3fa2c5c)), closes [#160](https://github.com/AppSprout-dev/mnemonic/issues/160)
* add retrieval comparison benchmark and fix spread activation bug ([84a73f8](https://github.com/AppSprout-dev/mnemonic/commit/84a73f8171920acbb0a3ab12d863ebf3705e8b24))
* add training data capture pipeline for bespoke local LLM ([69baf19](https://github.com/AppSprout-dev/mnemonic/commit/69baf1976a213f99439a91ecf8b60248e1f01df7))
* migrate SQLite driver from mattn/go-sqlite3 to modernc.org/sqlite ([3ad7d70](https://github.com/AppSprout-dev/mnemonic/commit/3ad7d7091a8ce65e70df53e6c66d21c8c8aa5e7e))
* migrate to pure-Go SQLite driver (drop CGO requirement) ([4a01daf](https://github.com/AppSprout-dev/mnemonic/commit/4a01daf9a95800953172ee7aefe9af513d0b3f3b))
* retrieval comparison benchmark + spread activation fix ([8ecf3ab](https://github.com/AppSprout-dev/mnemonic/commit/8ecf3abf4b803a4ceede34246e74b4224b4aa822))
* training data capture pipeline for bespoke local LLM ([09b5911](https://github.com/AppSprout-dev/mnemonic/commit/09b5911ae5bf991444e330356a958ccade9224cd))


### Bug Fixes

* aggregate LLM chart data server-side for accurate time bucketing ([fe06fb7](https://github.com/AppSprout-dev/mnemonic/commit/fe06fb7cb786725b77c523ab8432441e9da0e33c))
* aggregate LLM chart data server-side for accurate time bucketing ([5add9b0](https://github.com/AppSprout-dev/mnemonic/commit/5add9b0a69e208fc203557a06634b98a84dd448d))
* deduplicate filesystem events from atomic saves ([3a4e132](https://github.com/AppSprout-dev/mnemonic/commit/3a4e13222fa51341728b20ccf5e9c45dac878891))
* use reciprocal rank scoring in FTS merge to preserve BM25 ordering ([136f49d](https://github.com/AppSprout-dev/mnemonic/commit/136f49dbafb7b509df635f6e230ee768bda8e081))

## [0.14.2](https://github.com/AppSprout-dev/mnemonic/compare/v0.14.1...v0.14.2) (2026-03-16)


### Bug Fixes

* add missing source column to memory scan queries ([8442ab7](https://github.com/AppSprout-dev/mnemonic/commit/8442ab75c7416a5262bccfc13d27db8644b797ce))
* dashboard update button restarts daemon via PID fallback ([569e9e7](https://github.com/AppSprout-dev/mnemonic/commit/569e9e78dbe9314fcc5887b95d599e23c257cf22))
* dashboard update button restarts daemon via PID fallback ([625fa09](https://github.com/AppSprout-dev/mnemonic/commit/625fa0923c6691aaa929a7fd5931b6735b2c91c0))
* refresh activity panel timestamps every 30 seconds ([e050813](https://github.com/AppSprout-dev/mnemonic/commit/e0508130247b1b5de7e56f53a821d729b839b164))
* refresh activity panel timestamps every 30s ([d781408](https://github.com/AppSprout-dev/mnemonic/commit/d78140804a9c3aa5fccc3eb7fcfc663ac775e4a7))
* resolve 5 daemon bugs from system audit ([5d75f46](https://github.com/AppSprout-dev/mnemonic/commit/5d75f46dc8185b27ed24eb7880d6cd9b1a0ab43c))
* resolve memory scan error + 5 daemon bugs from system audit ([ef915c9](https://github.com/AppSprout-dev/mnemonic/commit/ef915c9c6bc05d3bcf29b66a1da6bd11f1aea0c7))

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
