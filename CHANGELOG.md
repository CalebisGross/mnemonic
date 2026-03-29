# Changelog

All notable changes to Mnemonic will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/). Versioning follows [Semantic Versioning](https://semver.org/).

## [0.34.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.33.0...v0.34.0) (2026-03-29)


### Features

* add CGo llama.cpp backend and wire into EmbeddedProvider ([ed71564](https://github.com/AppSprout-dev/mnemonic/commit/ed7156437b21130d3cdc7137a1417aca6e52a288))
* add deploy pipeline, embedding fine-tune script, and pre-register EXP-9 ([bf534bc](https://github.com/AppSprout-dev/mnemonic/commit/bf534bc0d89b4e89063133346641449844af8594))
* add embedding extraction via mean-pooled hidden states ([6da2d0d](https://github.com/AppSprout-dev/mnemonic/commit/6da2d0dd9a5bff85848b72ab3fc5d0c94e788b44))
* add llama.cpp submodule and fix GGUF export for Felix architecture ([93f1766](https://github.com/AppSprout-dev/mnemonic/commit/93f1766a76175d5b65b953f25b79d184a2fec4f6))
* add llama.cpp submodule, CGo bridge, and Qwen spoke training docs ([6c9e1d2](https://github.com/AppSprout-dev/mnemonic/commit/6c9e1d20b51a3835ef05acc983ed066f80d546ae))
* add LoRA support, gradient checkpointing fix, and EXP runner scripts ([9ba9738](https://github.com/AppSprout-dev/mnemonic/commit/9ba973847297c8fa601c41966d244dd54528f12f))
* add per-token logit validation for embedded LLM quality gating ([96775a2](https://github.com/AppSprout-dev/mnemonic/commit/96775a2aefe244b2db3150086fe2d0af3ef9d75c))
* add Phase 3 data pipeline scripts and pre-register EXP-6/7/8 ([640123b](https://github.com/AppSprout-dev/mnemonic/commit/640123beb0fbe19cb5b85695ea7a716b25b17fcd))
* add Phase 3 fine-tuning pipeline ([9c7df1c](https://github.com/AppSprout-dev/mnemonic/commit/9c7df1c579b044ee9243217984764ce822af0c7e))
* add Phase 3 fine-tuning pipeline ([498dd28](https://github.com/AppSprout-dev/mnemonic/commit/498dd287769a7e2a17f72dd4ff291f1495275e88))
* add Q8_0 quantization support and prefer quantized models ([b7a2488](https://github.com/AppSprout-dev/mnemonic/commit/b7a2488bd0713e157b8e5d91e2e60b9bda1ac81d))
* add Qwen spoke adapter, re-tokenization pipeline, and pre-register EXP-11-14 ([371464a](https://github.com/AppSprout-dev/mnemonic/commit/371464a33a8703c062a8dee216bd7fb1ead2c740))
* add ROCm HIP link flags and encoding comparison script ([8ae61c0](https://github.com/AppSprout-dev/mnemonic/commit/8ae61c0185afb4ea13ecce507320e362afa573f0))
* add standalone command center dashboard for GitHub tracking ([e750f60](https://github.com/AppSprout-dev/mnemonic/commit/e750f60c24e2f4559cd678615a3ac550dff4916c))
* add training script and evaluation hooks for Qwen spoke experiments ([36d8bdb](https://github.com/AppSprout-dev/mnemonic/commit/36d8bdb76fb71d1d40df9431001ab44904dc1539))
* complete EXP-9 mixed fine-tune, fix embedding script, add v2 model ([5bb94dc](https://github.com/AppSprout-dev/mnemonic/commit/5bb94dc88f96150f0ddd604cbd42d6c50a69ef7c))
* fix instrumented model label for embedded provider, add nomic BERT test ([f5e0c24](https://github.com/AppSprout-dev/mnemonic/commit/f5e0c24942775d7ce16b8b1e0db781aaa3223b7e))
* improve recall quality for LLM agents, fix Windows self-update ([7a8bfa6](https://github.com/AppSprout-dev/mnemonic/commit/7a8bfa6758dc6ffbb7fcf591c22fe0b41d486816))
* improve recall quality for LLM agents, fix Windows self-update ([aa07982](https://github.com/AppSprout-dev/mnemonic/commit/aa07982c11217f7950385d1c9e68796346efe0f2))
* Phase 3-4 autoresearch — fine-tuning pipeline, CGo backend, experiments ([49ad590](https://github.com/AppSprout-dev/mnemonic/commit/49ad5907f3cefb0c8d4295666cba509a4ff990eb))
* Qwen 3.5 2B + Felix spoke training infrastructure ([21facd3](https://github.com/AppSprout-dev/mnemonic/commit/21facd38369026add023e994dca20a2797c2b43e))
* run spoke gate analysis (EXP-8) and fix synthesis data generation ([c43587c](https://github.com/AppSprout-dev/mnemonic/commit/c43587cd9e486cb3123ec2eae78583120521aa64))
* standalone command center dashboard ([ea0536e](https://github.com/AppSprout-dev/mnemonic/commit/ea0536e992425a1e76deeb085d08a01bbaf80e33))
* strip coaching prompts from training data, tune LR to 3e-4 ([68c2725](https://github.com/AppSprout-dev/mnemonic/commit/68c2725538925b888c288124e1cd5141163b895c))
* update llama.cpp submodule with Qwen 3.5 spoke support ([75fa3f5](https://github.com/AppSprout-dev/mnemonic/commit/75fa3f5e55fa56a15022876b90be9a7c8534f9d1))
* verify nomic-bert embedding GGUF works via llama.cpp backend ([f0b71dd](https://github.com/AppSprout-dev/mnemonic/commit/f0b71dd09d1c083bb4969827ce51127a9bdddce2))


### Bug Fixes

* add missing MCP tools to web UI agent allowed_tools list ([ddb419f](https://github.com/AppSprout-dev/mnemonic/commit/ddb419ffea8c6841ccf668295f25381996900f20))
* ingest salience floor, JSON recall associations, graph edge priority, concept vocabulary ([088dca0](https://github.com/AppSprout-dev/mnemonic/commit/088dca07d08c3bb977b59b334b112dc8db7c146c))
* prevent crashes from embedding failures and concurrent llama.cpp access ([0c7f69e](https://github.com/AppSprout-dev/mnemonic/commit/0c7f69e8be9700a39d28be524413fb0280ba4294))
* resolve merge conflicts with main ([679b22d](https://github.com/AppSprout-dev/mnemonic/commit/679b22dcfed5fc6f321bacb9c939a84d6e96fc99))
* resolve NaN loss, dtype deprecation, and VRAM issues in training ([0bc050d](https://github.com/AppSprout-dev/mnemonic/commit/0bc050d3157b5d6778e0e6af66c6c24848b776cb))
* revert ingest salience floor exemption, keep heuristic boost ([ad0c3ab](https://github.com/AppSprout-dev/mnemonic/commit/ad0c3ab14bcdabafd7c9a07a04e09662510c388a))
* skip pre-migration backup when schema is current, add retention ([bed03d4](https://github.com/AppSprout-dev/mnemonic/commit/bed03d442bfd3f41f646b9511c2764e937b2e402))
* skip pre-migration backup when schema is current, add retention ([75a74c3](https://github.com/AppSprout-dev/mnemonic/commit/75a74c379be2bcc22b4d6e25c3ff0ec5436ebcf9))
* split ROCm link flags, add context window safety, and wire GBNF grammar ([9294aa0](https://github.com/AppSprout-dev/mnemonic/commit/9294aa02fc0a9c06b9f313a2c02f9787b5e72890))
* stress test issues — ingest salience, JSON associations, graph edges, concept vocabulary ([5ef4bfc](https://github.com/AppSprout-dev/mnemonic/commit/5ef4bfc198bdf9ff1c21ab88f78a36999e92793b))
* update EmbeddedProvider prompt format to match Felix-LM fine-tuning ([be284b3](https://github.com/AppSprout-dev/mnemonic/commit/be284b34ca9c1d776e900747f86de55170edf064))

## [0.33.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.32.0...v0.33.0) (2026-03-21)


### Features

* add Mind page to dashboard — force-directed memory graph visualization ([a8ae051](https://github.com/AppSprout-dev/mnemonic/commit/a8ae0517ae16483acdf3b1a8ebad7edd1d2d928d))
* live cognitive metrics, system analysis, and embedding backfill ([442e999](https://github.com/AppSprout-dev/mnemonic/commit/442e999c77e39ba5bde74a1b8012bfbd162f845d))
* Mind graph page, live cognitive metrics, and system analysis ([aca26f7](https://github.com/AppSprout-dev/mnemonic/commit/aca26f7d926a124e2de133fbcf7f2474bfb43ecf))
* redesign tools page research analytics and session activity ([1fb5018](https://github.com/AppSprout-dev/mnemonic/commit/1fb5018a8d319e39899bcf82b2b4a2f273d11372))


### Bug Fixes

* NULL raw_id crash, feedback bloat, and runtime metrics ([470e207](https://github.com/AppSprout-dev/mnemonic/commit/470e207b29126bb65373bb0f81bc35d654a739f8))
* NULL raw_id crash, feedback bloat, and runtime metrics ([#332](https://github.com/AppSprout-dev/mnemonic/issues/332), [#333](https://github.com/AppSprout-dev/mnemonic/issues/333), [#334](https://github.com/AppSprout-dev/mnemonic/issues/334)) ([ecc6f94](https://github.com/AppSprout-dev/mnemonic/commit/ecc6f945e57f02dfeb1c69377d811ac898bdcf5d))

## [0.32.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.31.0...v0.32.0) (2026-03-21)


### Features

* dynamic tool count, associate_with validation, session timeline polish ([c318dd2](https://github.com/AppSprout-dev/mnemonic/commit/c318dd211760cee1aad614fb9b3828d63ee4acff))
* dynamic tool count, associate_with validation, session timeline polish ([95e59f7](https://github.com/AppSprout-dev/mnemonic/commit/95e59f77223733524d02a2220d4725c9ca8ec274))
* make agent constants configurable via config.yaml ([5d709ff](https://github.com/AppSprout-dev/mnemonic/commit/5d709ff8d816437226a3a22aae25e0935bbeb45d))
* make agent constants configurable via config.yaml ([87e87e1](https://github.com/AppSprout-dev/mnemonic/commit/87e87e1d6e5462219b5a81ea14fabf9f02dbd294))
* make MCP/API salience and feedback weights configurable ([d51c88a](https://github.com/AppSprout-dev/mnemonic/commit/d51c88a32e252ae987fff40786dde063c42427b4))
* make MCP/API salience and feedback weights configurable ([d66b349](https://github.com/AppSprout-dev/mnemonic/commit/d66b349e30c881b0510910a89a0c5b1d57ab1ab9))
* make perception scoring weights configurable ([2b7e4d8](https://github.com/AppSprout-dev/mnemonic/commit/2b7e4d8b8a23b007ca8b714b28ad3ec789ead4a8))
* make perception scoring weights configurable via config.yaml ([8d1fc7e](https://github.com/AppSprout-dev/mnemonic/commit/8d1fc7ef9bedbcdb91685f8369e4b720bbc38845))
* make reactor cooldowns and startup delays configurable ([de7f166](https://github.com/AppSprout-dev/mnemonic/commit/de7f1668e27de733c1424ece63d11c3ed0a6a9de))
* make reactor cooldowns and startup delays configurable ([0504c06](https://github.com/AppSprout-dev/mnemonic/commit/0504c06f7df318807127f80cfe59835ab4a30a17))


### Bug Fixes

* prevent consolidation from reactivating dismissed patterns, filter exclude_concepts on patterns/principles ([7a583a7](https://github.com/AppSprout-dev/mnemonic/commit/7a583a77d1dadb0fa601a8521098f7f5c98895d9))
* prevent dismissed pattern reactivation + filter exclude_concepts on patterns ([7d1255b](https://github.com/AppSprout-dev/mnemonic/commit/7d1255b9c32e662872147bc3bd31082df3b2fe20))

## [0.31.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.30.0...v0.31.0) (2026-03-21)


### Features

* dashboard session timeline, encoding pipeline, salience histogram ([1cee5d2](https://github.com/AppSprout-dev/mnemonic/commit/1cee5d2f6cfc4afa36c691c108e76740c8cb2c01))
* dashboard session timeline, encoding pipeline, salience histogram ([6e7e5ef](https://github.com/AppSprout-dev/mnemonic/commit/6e7e5ef12aa0820eb98a00205e9c4d23180df444)), closes [#309](https://github.com/AppSprout-dev/mnemonic/issues/309)


### Bug Fixes

* add --resume-step and restore _orig_mod prefix stripping ([cb1be55](https://github.com/AppSprout-dev/mnemonic/commit/cb1be555958824b8bb23005ae72b2429f0ee2692))

## [0.30.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.29.1...v0.30.0) (2026-03-21)


### Features

* add --resume support to training script ([71c72b9](https://github.com/AppSprout-dev/mnemonic/commit/71c72b99c0e7183ebc8e05fe933b86e8675dfff9))
* add --resume support to training script ([fbdf2f1](https://github.com/AppSprout-dev/mnemonic/commit/fbdf2f1647424e933839dd14f1a8f4fc0526de2a))
* bulk forget and exclude_concepts for recall ([86c6768](https://github.com/AppSprout-dev/mnemonic/commit/86c6768171db7aca28f2dec91b9cbc8bc27669ee))
* bulk forget and exclude_concepts for recall ([912306d](https://github.com/AppSprout-dev/mnemonic/commit/912306d9f42a94df759476e152a91320e958b702)), closes [#307](https://github.com/AppSprout-dev/mnemonic/issues/307)
* dashboard activity tracker and pattern management ([57d17b9](https://github.com/AppSprout-dev/mnemonic/commit/57d17b98fc42d1694b5d6fb74d75763d41b1b5f2))
* dashboard activity tracker and pattern management ([7b63565](https://github.com/AppSprout-dev/mnemonic/commit/7b635657e8b12e48e2cfc71ae40c8fb1d758ec08))
* explicit associations on remember and create_handoff tool ([4b8f929](https://github.com/AppSprout-dev/mnemonic/commit/4b8f929d3a90474bed63443328654bc961b195f9))
* explicit associations on remember and create_handoff tool ([ff7ff79](https://github.com/AppSprout-dev/mnemonic/commit/ff7ff794f6dee1b91ccdfdda76cc6b0dd6d7e3fd)), closes [#308](https://github.com/AppSprout-dev/mnemonic/issues/308)


### Bug Fixes

* correct docs images and remove unused 192x192 icon ([3c58802](https://github.com/AppSprout-dev/mnemonic/commit/3c58802806499a9d666604e0c466d0e04de22be6))
* correct docs images and remove unused 192x192 icon ([5f53534](https://github.com/AppSprout-dev/mnemonic/commit/5f5353464fade15b6b4439aabb18138a66bf0fbf))
* correct mnemonic.png and remove unused 512x512 icon ([11d3c44](https://github.com/AppSprout-dev/mnemonic/commit/11d3c44d7d56a53b8019e3233601a2abd8de6cc0))
* filter binary asset paths and numeric segments from concept extraction ([63fa4ef](https://github.com/AppSprout-dev/mnemonic/commit/63fa4ef010ab9387c1acc0d403d92bdcb113517b))
* filter binary asset paths and numeric segments from concept extraction ([7d82ec4](https://github.com/AppSprout-dev/mnemonic/commit/7d82ec493a62401d27a5bb6ee98636d33a829d8c)), closes [#305](https://github.com/AppSprout-dev/mnemonic/issues/305)
* pattern project scoping, decay, and dismiss_pattern tool ([f050a36](https://github.com/AppSprout-dev/mnemonic/commit/f050a36087a66c0dc6596ef3636df82ee1e537f2))
* pattern project scoping, decay, and dismiss_pattern tool ([43a0f96](https://github.com/AppSprout-dev/mnemonic/commit/43a0f966151ffc6be267f5fdec4844c4cbc85190)), closes [#306](https://github.com/AppSprout-dev/mnemonic/issues/306)
* use correct mnemonic.png and remove unused 512x512 icon ([daaae33](https://github.com/AppSprout-dev/mnemonic/commit/daaae339ac1cdef1b6193f71d1f16a754c653416))

## [0.29.1](https://github.com/AppSprout-dev/mnemonic/compare/v0.29.0...v0.29.1) (2026-03-21)


### Bug Fixes

* publish WatcherEvent to bus so retrieval agent receives activity ([#296](https://github.com/AppSprout-dev/mnemonic/issues/296)) ([c7fddc2](https://github.com/AppSprout-dev/mnemonic/commit/c7fddc21d78b34cc3b9ca09461d8f24b9927aefd))
* sync daemon activity to MCP and filter path noise from themes ([#298](https://github.com/AppSprout-dev/mnemonic/issues/298)) ([739d39b](https://github.com/AppSprout-dev/mnemonic/commit/739d39bfdbde12fc96ef6502ab2812db8d232d4d))

## [0.29.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.28.0...v0.29.0) (2026-03-20)


### Features

* add pipeline observability metrics to get_context ([c566a60](https://github.com/AppSprout-dev/mnemonic/commit/c566a60a530f712c8434f353faccaa45fda9cf1a))
* add pipeline observability metrics to get_context MCP tool ([aebdaec](https://github.com/AppSprout-dev/mnemonic/commit/aebdaec6f0099dbdb2f1e0901555b616a08a832b))
* boost recall scores from recent watcher activity ([108af2e](https://github.com/AppSprout-dev/mnemonic/commit/108af2e31e31ea6e5770aa739489c51cca8ecea2))
* boost recall scores from recent watcher activity ([#277](https://github.com/AppSprout-dev/mnemonic/issues/277)) ([110762a](https://github.com/AppSprout-dev/mnemonic/commit/110762a4f609f898eadc463fea1effcfd50f0d35))
* enrich get_context themes with event types and terminal commands ([86c6a52](https://github.com/AppSprout-dev/mnemonic/commit/86c6a5262e1be01cd05d1e1fd72170d97323d3b3))
* enrich get_context themes with event types and terminal commands ([b0257e9](https://github.com/AppSprout-dev/mnemonic/commit/b0257e9ae3b2bc1e19bf058f449e9ad5bd3d3c52))

## [0.28.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.27.0...v0.28.0) (2026-03-20)


### Features

* 2-tier early dedup to prevent wasted LLM tokens ([4bcfef3](https://github.com/AppSprout-dev/mnemonic/commit/4bcfef36422cf210080563baff3c0ea93aa56bba))
* research analytics dashboard and API endpoint ([0d4d9d2](https://github.com/AppSprout-dev/mnemonic/commit/0d4d9d2a97d20ace00cb128b744055b877fb0444))


### Bug Fixes

* extract get_context themes from file paths instead of source code ([15cf4af](https://github.com/AppSprout-dev/mnemonic/commit/15cf4afc545b2c222424b8c72caa7643f58c034f))
* extract get_context themes from file paths instead of source code ([654bc54](https://github.com/AppSprout-dev/mnemonic/commit/654bc5437a3fc88c83db152611c24886a7fdd7be))

## [0.27.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.26.1...v0.27.0) (2026-03-20)


### Features

* add batch_recall MCP tool for parallel multi-query recall ([1d3cc08](https://github.com/AppSprout-dev/mnemonic/commit/1d3cc08dc3883c706533f5397ab19fd796e00f56)), closes [#275](https://github.com/AppSprout-dev/mnemonic/issues/275)
* add DB checkpointing to lifecycle test ([020ee54](https://github.com/AppSprout-dev/mnemonic/commit/020ee549012486cd8341844a8b5840b0c502ac94)), closes [#272](https://github.com/AppSprout-dev/mnemonic/issues/272)
* add proactive context push via get_context MCP tool ([7bedc64](https://github.com/AppSprout-dev/mnemonic/commit/7bedc641e59987d90ae690fa70f68dde5aea487d))
* structured JSON output for all recall MCP tools ([3df5c75](https://github.com/AppSprout-dev/mnemonic/commit/3df5c75b032586028849e266dafaf1a709a15993)), closes [#276](https://github.com/AppSprout-dev/mnemonic/issues/276)

## [0.26.1](https://github.com/AppSprout-dev/mnemonic/compare/v0.26.0...v0.26.1) (2026-03-20)


### Bug Fixes

* pattern decay rates and watcher noise filtering ([8983b13](https://github.com/AppSprout-dev/mnemonic/commit/8983b13afddc5536449b7e59a93f4f399f239854))

## [0.26.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.25.0...v0.26.0) (2026-03-20)


### Features

* agent UX improvements for recall filtering and noise reduction ([89328ec](https://github.com/AppSprout-dev/mnemonic/commit/89328ec983b3d5cb0622fc5f194bbd0729b00982))


### Bug Fixes

* comprehensive dedup quality with type/project/source awareness ([4342c39](https://github.com/AppSprout-dev/mnemonic/commit/4342c39beca53dc66756364bba17a4804ecc657c)), closes [#266](https://github.com/AppSprout-dev/mnemonic/issues/266)

## [0.25.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.24.0...v0.25.0) (2026-03-20)


### Features

* add lifecycle simulation test suite ([63b3490](https://github.com/AppSprout-dev/mnemonic/commit/63b3490576c7295393a124c82c59597200262922))
* expand synthetic data catalogs for lifecycle test ([9103a2c](https://github.com/AppSprout-dev/mnemonic/commit/9103a2c8cf1d52a1a66fee03626cfa1ac1540e03)), closes [#257](https://github.com/AppSprout-dev/mnemonic/issues/257)


### Bug Fixes

* add missing columns to FTS5 search query ([ca8c967](https://github.com/AppSprout-dev/mnemonic/commit/ca8c967959b7a9722db8ce99773fbdb224c89a91))

## [0.24.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.23.0...v0.24.0) (2026-03-20)


### Features

* add Windows binary to release pipeline and self-updater ([27fbb1a](https://github.com/AppSprout-dev/mnemonic/commit/27fbb1a5aad592321b42859508ef794244c6d9b5))
* add Windows binary to release pipeline and self-updater ([6ec4ecc](https://github.com/AppSprout-dev/mnemonic/commit/6ec4ecc131a9417c775a45e56f9935b7b54d7b4e))


### Bug Fixes

* address PR feedback — zip test coverage and nullglob ([112f376](https://github.com/AppSprout-dev/mnemonic/commit/112f3760ee74987bdc010e93207e224c6b871e9f))
* use logo in dashboard header and tab title ([0e33f28](https://github.com/AppSprout-dev/mnemonic/commit/0e33f283eedeb63eaf6a32ef69de36a096a78e93))
* use logo in dashboard header and tab title ([f1e51c6](https://github.com/AppSprout-dev/mnemonic/commit/f1e51c6c7b205aceeb0416097132df6bfa7d58b1))

## [0.23.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.22.0...v0.23.0) (2026-03-20)


### Features

* add exclude_path and list_exclusions MCP tools ([0ffc3d0](https://github.com/AppSprout-dev/mnemonic/commit/0ffc3d04fdf64c6a98c986b887199d3129d39846)), closes [#239](https://github.com/AppSprout-dev/mnemonic/issues/239)
* add pipeline health and source distribution to status ([8ccae78](https://github.com/AppSprout-dev/mnemonic/commit/8ccae7845efdd2a8e8263b3e46ab75371728766d)), closes [#237](https://github.com/AppSprout-dev/mnemonic/issues/237)
* add structured JSON output option to recall ([48138f1](https://github.com/AppSprout-dev/mnemonic/commit/48138f19aad1a23df0fc99e6fb4abb6a6374ec0b)), closes [#240](https://github.com/AppSprout-dev/mnemonic/issues/240)
* archive never-recalled watcher memories after 30 days ([2fb6ddc](https://github.com/AppSprout-dev/mnemonic/commit/2fb6ddc7265d3e4d24595d2e16c4e10b1ccc8a0c)), closes [#233](https://github.com/AppSprout-dev/mnemonic/issues/233)
* boost retrieval ranking for pattern and abstraction evidence ([aacbba2](https://github.com/AppSprout-dev/mnemonic/commit/aacbba272294a0672b0a7766114e614d377f7312)), closes [#238](https://github.com/AppSprout-dev/mnemonic/issues/238)
* dynamic watcher exclusions and structured JSON recall output ([808c9e8](https://github.com/AppSprout-dev/mnemonic/commit/808c9e821d16f9fc8278b951d3ae653d73079a72))
* improve concept extraction consistency ([2dd1bb6](https://github.com/AppSprout-dev/mnemonic/commit/2dd1bb6ca8469197791354148a4313a0bd345e59)), closes [#236](https://github.com/AppSprout-dev/mnemonic/issues/236)
* make recall synthesis opt-in instead of default ([6ab4956](https://github.com/AppSprout-dev/mnemonic/commit/6ab495611fabe2255e13fa424c75d8d86f4c4d8d)), closes [#234](https://github.com/AppSprout-dev/mnemonic/issues/234)
* prioritize MCP memories in encoding queue ([2466420](https://github.com/AppSprout-dev/mnemonic/commit/2466420ac288490bb0940a165486b2c9d1a26a17)), closes [#235](https://github.com/AppSprout-dev/mnemonic/issues/235)
* signal quality and pipeline efficiency improvements ([a76d701](https://github.com/AppSprout-dev/mnemonic/commit/a76d7010ca842ae43ce9a6459c5aeecdb86577e9))

## [0.22.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.21.0...v0.22.0) (2026-03-20)


### Features

* add explain parameter to recall for score breakdown ([314c0ce](https://github.com/AppSprout-dev/mnemonic/commit/314c0cefabac52947a152c1c7e6c715060e5e555)), closes [#229](https://github.com/AppSprout-dev/mnemonic/issues/229)
* add feedback-informed ranking and source-weighted scoring to retrieval ([8f19f56](https://github.com/AppSprout-dev/mnemonic/commit/8f19f563f45dccd48888e0f78a31395425eee95e))
* add LR bisection search with short probes ([8468f87](https://github.com/AppSprout-dev/mnemonic/commit/8468f872efe697bc9dc6bff1ea01f54519652bf8))
* add memory amendment tool for in-place content correction ([f1f7706](https://github.com/AppSprout-dev/mnemonic/commit/f1f77062ea3bcbef40fb519ef8fe6bf0ba4e5359)), closes [#222](https://github.com/AppSprout-dev/mnemonic/issues/222)
* add negative feedback auto-suppression ([f22239e](https://github.com/AppSprout-dev/mnemonic/commit/f22239e4d38c2c6d071f86e5068af0c58b1a41b4)), closes [#228](https://github.com/AppSprout-dev/mnemonic/issues/228)
* add session-scoped recall with list_sessions and recall_session ([276e346](https://github.com/AppSprout-dev/mnemonic/commit/276e346391b467a2674e4099b8f11369d14f4d35)), closes [#225](https://github.com/AppSprout-dev/mnemonic/issues/225)
* add Tools dashboard tab for MCP usage analytics ([70e7453](https://github.com/AppSprout-dev/mnemonic/commit/70e7453f3b953a333a0bbe5a4166c631abad433b))
* add Tools dashboard tab for MCP usage analytics ([a92386f](https://github.com/AppSprout-dev/mnemonic/commit/a92386fe604d8e52e38a58ba822fd24350d1a961))
* Claude-First Memory — 7 issues for collaborative knowledge system ([34858b4](https://github.com/AppSprout-dev/mnemonic/commit/34858b423867b83dcaa3780b2c45cc851ab24793))
* enrich remember response and add check_memory MCP tool ([b184e2c](https://github.com/AppSprout-dev/mnemonic/commit/b184e2c4f099923f5abf90e145ad953c04239026)), closes [#227](https://github.com/AppSprout-dev/mnemonic/issues/227)
* feedback-informed ranking and source-weighted scoring ([6d2aab0](https://github.com/AppSprout-dev/mnemonic/commit/6d2aab03aa23be8be373e34e91cc7bf2c30e0181))
* LR bisection search for optimal pretraining LR ([4dc6735](https://github.com/AppSprout-dev/mnemonic/commit/4dc673580031b62f5545fe029d7a150c65cf1ffe))
* soften abstraction demotion and add grace period ([17e03c7](https://github.com/AppSprout-dev/mnemonic/commit/17e03c7c4da1389102b3e169d7a52d14e9095811)), closes [#226](https://github.com/AppSprout-dev/mnemonic/issues/226)
* surface associations in recall results ([8fc6ca0](https://github.com/AppSprout-dev/mnemonic/commit/8fc6ca0418d71f039d6eba4186d0afb8156ec527)), closes [#224](https://github.com/AppSprout-dev/mnemonic/issues/224)


### Bug Fixes

* prevent duplicate encoding across multiple mnemonic processes ([b2bcff5](https://github.com/AppSprout-dev/mnemonic/commit/b2bcff58a72de9f7d65f90160a281eb7a61b0fb1))
* prevent duplicate encoding across multiple mnemonic processes ([0339564](https://github.com/AppSprout-dev/mnemonic/commit/03395646db4e4dd3bd8ed16c75642e9876fd45e3))

## [0.21.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.20.0...v0.21.0) (2026-03-19)


### Features

* add encoding salience floor, lockfile rejection, and dedup tuning ([e8acc7b](https://github.com/AppSprout-dev/mnemonic/commit/e8acc7bbde538735fe400781bb741d174a3b1719))
* add logarithmic pattern scaling and reset-patterns CLI ([28632f9](https://github.com/AppSprout-dev/mnemonic/commit/28632f9573d73693ff2391c4998e6910cb306dfb))
* add MCP tool usage analytics ([862ff50](https://github.com/AppSprout-dev/mnemonic/commit/862ff503d120d60119839e8d9b3f26f2def87b04))
* add MCP tool usage analytics ([4a3b387](https://github.com/AppSprout-dev/mnemonic/commit/4a3b387f5de908b3e95158f9463e82b4bfd89d97))
* detect version changes at startup and create memory ([b9565f5](https://github.com/AppSprout-dev/mnemonic/commit/b9565f58557412ec8036b57359f9854286f299b7))
* ground abstraction prompts and wire retrieval feedback into metacognition ([947c700](https://github.com/AppSprout-dev/mnemonic/commit/947c70061ce2d39e6988fc038a0c159c3af18b07))
* memory quality improvements from v0.20.0 audit ([6eab1e2](https://github.com/AppSprout-dev/mnemonic/commit/6eab1e2fb859a19f011f5003fea803cc0d73de7c))


### Bug Fixes

* add dedup check and processed guard to event-driven encoding path ([7f100fb](https://github.com/AppSprout-dev/mnemonic/commit/7f100fbadf24dcc30967b312b7633b3793eb89df))
* add dedup check to event-driven encoding path ([71738f0](https://github.com/AppSprout-dev/mnemonic/commit/71738f078b4c43253c59ba2fa44c2d0d3a08031e))
* handle rows.Close error to satisfy errcheck lint ([035650e](https://github.com/AppSprout-dev/mnemonic/commit/035650e2f98f544154a1c1c7d6a98677664c7b0b))

## [0.20.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.19.0...v0.20.0) (2026-03-19)


### Features

* add dedup CLI command for duplicate cleanup ([dfd7970](https://github.com/AppSprout-dev/mnemonic/commit/dfd7970370e12733740c629edaa8ea59c0598a2c))
* add dedup CLI command for one-time duplicate cleanup ([#206](https://github.com/AppSprout-dev/mnemonic/issues/206)) ([340e38e](https://github.com/AppSprout-dev/mnemonic/commit/340e38e8ba8913a86ce472a4dd8c7ba40be41265))
* add sweep automation and fix EXP-2 registry ([bfc891d](https://github.com/AppSprout-dev/mnemonic/commit/bfc891da7e5ffd35039237a82ddc2deba180e961))
* sweep automation and EXP-2 registry fix ([f9da750](https://github.com/AppSprout-dev/mnemonic/commit/f9da750b9a3f5e5f3cc6184bd131d21d0ccab1b1))


### Bug Fixes

* auto-refresh dashboard after update ([0c12bf2](https://github.com/AppSprout-dev/mnemonic/commit/0c12bf25212fb53b2b55041e7155cf70f79597e4))
* auto-refresh dashboard after update ([3a1baca](https://github.com/AppSprout-dev/mnemonic/commit/3a1baca74c25c2d859ee13da3142253733fa6094))
* orphaned associations and bidirectional lookup bug ([7c1df31](https://github.com/AppSprout-dev/mnemonic/commit/7c1df31c30248cf875fb3e6fa3a74c2fa2b48f44))

## [0.19.0](https://github.com/AppSprout-dev/mnemonic/compare/v0.18.0...v0.19.0) (2026-03-19)


### Features

* add batch embedding and self-sustaining pattern decay ([1f73770](https://github.com/AppSprout-dev/mnemonic/commit/1f737709ef9c0592e206a4d8c23497cbbe6be7f8))
* add batch embedding and self-sustaining pattern decay ([8c2cac4](https://github.com/AppSprout-dev/mnemonic/commit/8c2cac446bd8e155aa45784f03726db8ec603cee)), closes [#189](https://github.com/AppSprout-dev/mnemonic/issues/189)
* add dreaming quality gate and event bus test determinism ([206ac5c](https://github.com/AppSprout-dev/mnemonic/commit/206ac5c1b3e6deedcf8e67fc53527944493b6755))
* add dreaming quality gate and event bus test determinism ([3165465](https://github.com/AppSprout-dev/mnemonic/commit/31654653358e8b2af69441341f1882e5b0f9091b)), closes [#190](https://github.com/AppSprout-dev/mnemonic/issues/190)
* add MMR diversity filter and encoding dedup ([#192](https://github.com/AppSprout-dev/mnemonic/issues/192), [#194](https://github.com/AppSprout-dev/mnemonic/issues/194)) ([59a9dbb](https://github.com/AppSprout-dev/mnemonic/commit/59a9dbb7de994368290091c43db45fe9184d803a))
* externalize consolidation agent tunables to config ([9ca5052](https://github.com/AppSprout-dev/mnemonic/commit/9ca50525ceab3efa01f8a0bc49c894111ea3b4ee))
* externalize consolidation agent tunables to config ([f05bdfc](https://github.com/AppSprout-dev/mnemonic/commit/f05bdfc79fa368957219cee6b6211a73fb417954)), closes [#187](https://github.com/AppSprout-dev/mnemonic/issues/187)
* externalize perception and encoding tunables to config ([761a590](https://github.com/AppSprout-dev/mnemonic/commit/761a5904107733a26cca050e49a851746a2d15ee))
* externalize perception and encoding tunables to config ([318e3de](https://github.com/AppSprout-dev/mnemonic/commit/318e3de377eab490af4d1241f93f8fd4f3f00222)), closes [#188](https://github.com/AppSprout-dev/mnemonic/issues/188)
* externalize retrieval agent tunables to config ([f8abd3a](https://github.com/AppSprout-dev/mnemonic/commit/f8abd3a136a9bf0ac09bed42b714d55985df9773))
* externalize retrieval agent tunables to config ([1065ff5](https://github.com/AppSprout-dev/mnemonic/commit/1065ff569b719e59475d82c40256b66b4a277fed)), closes [#186](https://github.com/AppSprout-dev/mnemonic/issues/186)
* MMR diversity filter and encoding dedup ([d1df706](https://github.com/AppSprout-dev/mnemonic/commit/d1df7064569dcffac908727230b4f7b14c724475))
* recall filters, project spread activation, feedback loop, session end ([fb2e519](https://github.com/AppSprout-dev/mnemonic/commit/fb2e5192bf75647c18ad42328200edcdc8f2a9b1))
* recall filters, project spread activation, feedback loop, session end ([#193](https://github.com/AppSprout-dev/mnemonic/issues/193), [#195](https://github.com/AppSprout-dev/mnemonic/issues/195), [#196](https://github.com/AppSprout-dev/mnemonic/issues/196), [#197](https://github.com/AppSprout-dev/mnemonic/issues/197)) ([09dad2c](https://github.com/AppSprout-dev/mnemonic/commit/09dad2c099c96950712f47e9203d8c08d4b1c1e4))
* store access snapshot in retrieval feedback ([3edbb65](https://github.com/AppSprout-dev/mnemonic/commit/3edbb65c28a7b480d40256ca31eff92fd0410b6c))
* store access snapshot in retrieval feedback ([0b7473f](https://github.com/AppSprout-dev/mnemonic/commit/0b7473fc6035925eb4c112e580ff1be865fc3939)), closes [#184](https://github.com/AppSprout-dev/mnemonic/issues/184)


### Bug Fixes

* suppress filesystem events during git operations ([cd43497](https://github.com/AppSprout-dev/mnemonic/commit/cd43497f483fff7aa52586c335af493533d6783f))
* suppress filesystem events during git operations ([86a760c](https://github.com/AppSprout-dev/mnemonic/commit/86a760c2e86b3dc811d7b0f538e090e9153658d8))

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
