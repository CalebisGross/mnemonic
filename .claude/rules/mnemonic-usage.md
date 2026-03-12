# Mnemonic MCP Tool Usage — Mandatory

## Session Start (MUST — before any other work)

1. Call `recall_project` to load project context
2. Call `recall` with keywords relevant to the user's first request
3. If either call returns useful context, use it to inform your work

Do NOT skip these steps. Do NOT jump straight into coding.

## During Work (MUST)

- **Decisions**: When you make or recommend an architectural/design choice, call `remember` with `type: "decision"`
- **Errors**: When you encounter and resolve a bug or error, call `remember` with `type: "error"`
- **Insights**: When you discover something non-obvious about the codebase, call `remember` with `type: "insight"`
- **Learnings**: When you learn something about a library, API, or framework behavior, call `remember` with `type: "learning"`

You do not need to remember every trivial action. Use judgment — if it would be useful to know in a future session, remember it.

## After Recalls (MUST)

- After using `recall` and acting on the results, call `feedback` to rate quality (`helpful`, `partial`, or `irrelevant`)
- This trains the retrieval system — skipping it degrades future recall quality

## Before Committing (SHOULD)

- Review the session's work and `remember` any decisions or insights that haven't been stored yet
- Call `session_summary` if the session involved significant work

## General

- Prefer specific `recall` queries over broad ones — "SQLite FTS5 migration" not "database stuff"
- Set the `type` field on every `remember` call — never use the default "general" when a specific type fits
- When a recall returns irrelevant noise, say so via `feedback` — this is how the system improves
