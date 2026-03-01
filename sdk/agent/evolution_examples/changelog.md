# Evolution Changelog

All self-modifications are logged here with date, what changed, and rationale.

## 2026-02-27 (critical review)

### Post-review evolution updates
- **p12 confidence 0.7→0.8**: Validated again — Explore subagent missed duplicate `ALL_MNEMONIC_TOOLS` definitions across `options.py` and `subagents.py`; targeted Grep found it instantly.
- **code_review strategy**: Added 2 tips — grep for duplicate symbol definitions across modules; distinguish "what has tests" from "what matters and lacks tests" (49 passing tests masked zero orchestration coverage).
- **Stored 3 memories**: duplicate ALL_MNEMONIC_TOOLS bug (insight), PRE_TASK_PROMPT format string vulnerability (error), subagent-vs-grep review finding (learning).

## 2026-02-27

### Merged evolution directories and added p11-p13, prompt_audit, code_review, pp6
- Discovered two evolution dirs: `evolution/` (stale, root) and `sdk/agent/evolution/` (canonical, used by daemon + SDK)
- Agent chat wrote changes to wrong dir (`evolution/`), overwrote prompt_patches.yaml destructively
- Merged unique data from root into canonical SDK dir:
  - p11 (DB-first system audit), p12 (verify subagent claims), p13 (symbol-targeted grep)
  - `prompt_audit` and `code_review` strategies
  - pp6 (skip recall for non-technical questions) — renumbered from agent's "pp2"
- Fixed pp4/pp5 ordering in prompt_patches.yaml
- Removed stale `evolution/` dir to prevent future confusion
- Updated CLAUDE.md project layout to reference `sdk/agent/evolution/`

### Implemented 6 SDK improvements (49/49 tests pass)
- **Fix model switch no-op** (`web.py`): Per-connection `session_model` + `dataclasses.replace()` for isolated config. Client lifecycle loop recreates `ClaudeSDKClient` on model change.
- **ConversationStore tests** (`tests/test_conversation_store.py`): 27 new tests covering CRUD, auto-titling, cost accumulation, continuation summaries, preferences, rotation, error recovery.
- **Pre-task recall skip logic** (`prompts.py`): `PRE_TASK_PROMPT` now lets Claude decide whether recall is useful; skips for non-technical inputs.
- **Deduplicate stream handlers** (`session.py` + `web.py`): Extracted `stream_events()` async generator consumed by both CLI and WebSocket.
- **Move orchestration prompts** (`session.py` → `prompts.py`): `PRE_TASK_PROMPT`, `POST_TASK_PROMPT`, `EVOLVE_PROMPT` now live alongside `BASE_PROMPT`.
- **Configurable subagent models**: Added `Config.subagent_model`, `make_subagents()` factory, `--subagent-model` CLI arg.

### Added code_review strategy
- Source: Full SDK code review found 13 issues across 8 Python files
- Key steps: read ALL files first, run tests for baseline, identify cross-cutting concerns, prioritize by severity × effort

### Added tip to encoding_audit strategy: post-coaching verification via timestamps
**Rationale:** When verifying coaching effectiveness, raw counts without `state='active'` include archived memories and can appear to show regression. The definitive check is: filter by `state='active'` AND compare memory timestamps against coaching application date. Confirmed in this session — 15 total template echo memories looked like regression from 9, but all 6 active ones were pre-coaching (2026-02-25/26). Zero post-coaching. Count discrepancy = state filter mismatch, not a real regression.

### Wrote coaching.yaml for local LLM encoding quality
**File:** `~/.mnemonic/coaching.yaml` (created)
**Rationale:** Audit found 3 systematic encoding failure modes across 41+ memories:
1. **Placeholder leakage** — 41 memories contain `["concept1","concept2"]` verbatim copied from the prompt template instead of real extracted concepts.
2. **Template echo** — 9+ active memories have summaries that ARE the prompt instructions ("compressed version preserving key details", "1-2 sentences — what happened and why").
3. **Meta-commentary** — LLM writes about the encoding task itself ("A person's memory encoder should remember this event") instead of encoding the content.

Coaching YAML includes 5 explicit RULES with examples of bad vs. good output:
- RULE 1: Never output template placeholders (concept1/concept2, "compressed version", etc.)
- RULE 2: Never describe the encoding task — write what happened, not what to do with it
- RULE 3: Be specific — name actual functions, files, error types (not "A person encountered an error")
- RULE 4: Concepts must be real noun phrases, 3-6 terms, extracted from actual content
- RULE 5: For file_modified events, describe the specific change, not the file's general purpose

Daemon rebuilt and restarted to pick up new coaching file. MCP server process will pick it up on next session restart.

### Added strategy: encoding_audit
**Rationale:** This is a repeatable task type with a clear playbook that didn't exist. Key lessons: `audit_encodings` shows only recent pairs — direct DB queries (`concepts LIKE '%concept1%'`) are the reliable way to find systemic failures. `limit=5` is the safe ceiling (limit=20 hit the 99k token cap). Coaching YAML must be cumulative (read existing before writing). MCP server needs session restart to pick up new coaching.

---

## 2026-02-26

### Added principle p10: Use recall_timeline for temporal/reflective questions
**Rationale:** Pre-task recall used the user's verbatim temporal question ("we've done a lot of work this morning. what have you learned?") as a semantic query — returned generic/partial results. `recall_timeline` reconstructed the morning's work accurately in one call. The memory_analysis strategy already had this tip, but it wasn't wired into pre-task recall behavior for temporal tasks. Confidence 0.8 — direct failure evidence.

### Added strategy: system_review
**Rationale:** "Do you see areas for improvement?" is a recurring question type with no dedicated strategy. The right tools are status + get_insights + recall_project + evolution files — not semantic recall (which returns filesystem noise for these queries). Codified the approach after observing semantic recall fail for the second time on meta/assessment questions. Distinct from self_improvement (which is about updating evolution files) and codebase_audit (which is about code analysis).

### Added prompt patch pp5: diagnostic tools for meta/assessment questions
**Rationale:** Closes the remaining gap after pp4 (temporal) — assessment questions like "what should we improve?" are a third query type where semantic recall underperforms. pp5 redirects to status+get_insights+recall_project for synthesis-style questions.

---

### Added prompt patch pp4: recall_timeline as primary tool for temporal tasks
**Rationale:** The pre-task recall instructions say "call recall with a query summarizing the upcoming task" — but when the task IS a temporal/reflective question, that produces poor semantic queries. pp4 explicitly redirects to recall_timeline for those cases. Closes the gap between the memory_analysis strategy tip and actual pre-task behavior.

---

## 2026-02-25

### Updated memory_analysis strategy: added abstraction audit tip
**Rationale:** Diagnosed abstraction duplication bug — 27 active L2s instead of expected ~8-10. Root cause: ListAbstractions limit=20 with 29 total L2s means 9 fall outside the dedup window; synthesizeAxioms has no dedup check at all. Added SQL snippet to memory_analysis tips so future sessions can quickly spot this. Stored specific bug details in mnemonic memory (type=error, project=sdk).

### Increased p8 confidence: 0.7 → 0.8
**Rationale:** Validated again — project scope bug diagnosis confirmed the stale binary hypothesis. The MCP server (old binary) had its own encoding agent that hadn't been rebuilt with the fix, while the daemon (new binary) was irrelevant to the MCP encoding path. Same pattern, second confirmation.

### Added principle p9: Trace data flow path before inspecting code
**Rationale:** Spent multiple turns verifying correct code (GetRaw, WriteMemory SQL, encoding agent) before discovering the MCP server runs its own encoder entirely separate from the daemon. The fix was in the daemon but MCP memories are encoded by the MCP server's embedded encoder. Mapping which process handles which stage upfront would have found the root cause in 2 turns instead of 8+.

### Updated debugging strategy: added pipeline path tip
**Rationale:** Same session — the most costly wrong turn was assuming the daemon handled all encoding. Added explicit tip to map the data flow path early in multi-process debugging scenarios.

---

## 2026-02-24

### Added principle p1: Check Makefile for build requirements
**Rationale:** While debugging "no such module: fts5" SQLite error, discovered the mnemonic binary needed rebuild with `-tags "sqlite_fts5"`. The Makefile contained this critical information but I checked it late in the debugging process. Checking build files early saves time.

### Added strategy: daemon_startup
**Rationale:** Successfully started mnemonic daemon by following a systematic approach: check if running → examine config → check Makefile → rebuild with correct flags → start → verify. This pattern applies to most daemon/service startup tasks and should be reused.

### Added principle p2: Check both memory scopes for full context
**Rationale:** When analyzing memory systems, discovered that recall_project (project-scoped) showed 0 memories while status (system-wide) showed 385 total. This scope distinction is important for understanding whether you're working with project-specific context or global knowledge.

### Added strategy: memory_analysis
**Rationale:** Developed systematic approach for analyzing memory systems: parallel recalls → get insights/status → compare scopes → identify repeated patterns → review warnings → synthesize. Key discovery: repeated abstractions with same confidence indicate strongly reinforced patterns worth prioritizing.

### Added strategy: system_verification
**Rationale:** Verified mnemonic daemon was fully operational and autonomous by checking 6 layers: process status, database state, logs, config, queues, and growth metrics. Discovered that single-layer checks (just "is it running?") give false confidence. Multi-layer verification proved 311 items processing, 10 new memories in 10 min, and real-time activity. Key insight: always check PRAGMA table_info before querying unfamiliar schemas.

### Added principle p3: Read evolution files before analysis tasks
**Rationale:** During a codebase audit task, evolution files were read *after* the analysis, missing the opportunity to build on prior knowledge. Reading them first would avoid rediscovering already-known patterns and allow the analysis to start from a higher baseline.

### Added strategy: codebase_audit
**Rationale:** Conducted a full audit of the mnemonic SDK. Discovered that the most effective approach is: read evolution files first → check mnemonic health warnings → delegate deep code exploration to Explore subagent → cross-reference health signals with code findings → produce prioritized output. Health warnings (source_balance, quality_audit) turned out to be the highest-signal findings, revealing daemon-level bugs (dedup failure, source flooding).

### Added prompt patch pp1: Always call mcp__mnemonic__feedback after recalls
**Rationale:** The mnemonic system flagged a `recall_effectiveness` warning repeatedly. Investigation showed that feedback was never being called after recall results were evaluated. This deprives the retrieval system of quality signals. Adding a prompt patch ensures feedback becomes a consistent habit.

## 2026-02-25

### Added strategy: code_improvement
**Rationale:** Implemented logging, sessions.json rotation, and 12 new tests across 4 files — all 22 tests passed on the first run with no debugging. The key pattern: read all target files before editing any of them, implement smallest changes first, run tests once at the end. Also learned that test helper functions at module level significantly reduce boilerplate. Distinct from `codebase_audit` (which is about analysis) — this strategy covers actual implementation.

## 2026-02-25 (self-improvement session)

### Created MEMORY.md
**Rationale:** The auto-memory directory existed but MEMORY.md was never created. This is the primary mechanism for persisting project context between Claude Code sessions — it is auto-loaded into every conversation. Without it, every session starts with zero project context and must re-derive everything via recall calls that often return 0 project-scoped results (the mnemonic daemon stores system-wide memories, not project-tagged ones). MEMORY.md now contains: project overview, key file paths, DO NOT MODIFY list, build/run commands, conventions, hard constraints, and current evolution state.

### Added principle p4: Batch independent tool calls
**Rationale:** The single most reliable way to reduce latency and turn count. When multiple tool reads/queries are independent, batching them in one response cuts time in half. This is stated in the system instructions but not yet explicit in principles — making it explicit ensures it stays top of mind.

### Added principle p5: Don't retry the same failing approach
**Rationale:** System instructions explicitly warn against brute-forcing blocked paths. Confirmed through experience that the second failure is a clear signal to change strategy. Added as a principle because the temptation to "try one more time" is strong — having an explicit rule breaks that pattern.

### Added principle p6: Verify no existing file before creating a new one
**Rationale:** File proliferation is a real cost — both cognitive (harder to navigate) and practical (duplicated logic, maintenance burden). Glob/Grep are fast; checking takes seconds. Added because "prefer editing over creating" is in the system instructions but not enforced as an internalized habit yet.

### Added strategy: debugging
**Rationale:** No systematic debugging strategy existed despite this being one of the most common task types. Built from the FTS5 experience (check Makefile first) and general debugging principles. Key insight: form a specific hypothesis before investigating further — wide exploration without a hypothesis wastes turns.

### Added strategy: new_feature
**Rationale:** Distinct from `code_improvement` (which refines existing code) and `codebase_audit` (analysis only). New feature implementation has its own shape: understand existing patterns first, implement in layers (types → logic → tests), confirm nothing equivalent already exists. Built from consistent patterns observed across multiple feature sessions.

### Added prompt patch pp2: Maintain MEMORY.md
**Rationale:** Without an explicit instruction to update MEMORY.md, it will drift out of date or never be created in the first place (as was the case here). The patch creates a habit: at the end of significant sessions, update the file. The 150-line guideline prevents it from growing beyond the 200-line truncation limit.

## 2026-02-25 (post-task reflection: project-scoped memories bug fix)

### Added principle p8: stale binary = fix not taking effect
**Rationale:** After fixing the encoding agent, encoded memories still had empty project fields. The fix was correct in source, but the MCP server process was still running the old binary. This pattern — correct fix, no effect — is reliably diagnosed by checking whether all relevant processes were restarted. Multi-mode binaries (daemon vs mcp subcommand) are fully independent processes.

### Added 2 tips to debugging strategy
**Rationale:** (1) Verify binary reload across all processes after a fix. (2) To test async pipeline fixes, bypass the event path via direct DB inserts — avoids waiting for long queues and isolates which process handles the data.

## 2026-02-25 (bug fix: 0 project-scoped memories)

### Fixed: encoding agent dropped project/session_id fields
**File:** `internal/agent/encoding/agent.go:455`
**Rationale:** `store.Memory{}` was created without `Project: raw.Project` or `SessionID: raw.SessionID`. All encoded memories got empty project fields, making `recall_project` always return 0. Fixed by adding both fields to the struct literal. All tests pass. Takes effect in the next session when the MCP server process restarts with the new binary.

**Architecture insight:** The `mcp` subcommand runs its own encoding agent (not the daemon's). The daemon's encoding agent is the fallback for anything the MCP server's agent doesn't pick up via events.

## 2026-02-25 (post-task reflection: "has anything changed" query)

### Updated memory_analysis strategy: added recall_timeline tip
**Rationale:** User asked "has anything changed since our last chat?" — semantic recall returned irrelevant filesystem file-modification records (matched on "memory" concepts, not recency). `recall_timeline` is purpose-built for temporal/change-detection queries: it retrieves memories chronologically within a configurable time window. Added as a tip to the memory_analysis strategy. Semantic recall should never be used for "what changed recently" — it's for knowledge retrieval, not temporal inspection.

## 2026-02-25 (post-task reflection: subjective recall query + subagent delegation gap)

### Added prompt patch pp3: Never delegate pre-task recalls to subagents
**Rationale:** Pre-task recall (recall/get_patterns/get_insights) was delegated to a memory-archivist subagent. The subagent executed the calls and held the query_id internally — the main session never received it, making mcp__mnemonic__feedback impossible to call. This silently violated pp1 on every session that used subagent delegation for recalls. pp3 makes the constraint explicit: pre-task recalls must always run in the main session.

### Stored insight in mnemonic (type=insight, project=sdk)
**Content:** Delegating recall calls to subagents severs the query_id chain needed for pp1 feedback.

---

## 2026-02-25 (post-task reflection: memory status inquiry)

### Added principle p7: Use diagnostic tools for status questions, not semantic recall
**Rationale:** User asked "can you see all the memories now?" — semantic recall returned unrelated memory-architecture docs. mcp__mnemonic__status gave the precise answer (532 total, 0 project-scoped) in one call. Recall is for knowledge retrieval; status/logs/ps are for state inspection. These are different question types requiring different tools.

### Post-task: Fixed stale MEMORY.md counts and added self_improvement strategy
**Rationale:** After completing the self-improvement work, MEMORY.md still said "3 principles, 5 strategies" — stale immediately. Fixed counts to reflect actual state (6 principles, 7 strategies, 2 prompt patches). Also codified the self_improvement task type as a strategy, since this is a repeatable pattern with a clear playbook: read evolution files → recall in parallel → identify gaps → prioritize by leverage → execute → update changelog last.
