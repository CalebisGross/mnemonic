#!/usr/bin/env python3
"""Evaluate embedding model on mnemonic IR benchmark scenarios.

Mirrors the Go benchmark-quality scenarios: embeds all memories and queries,
ranks by cosine similarity, computes nDCG@5, Precision@5, Recall@5, MRR.

Compares base model vs fine-tuned model side-by-side.

Usage:
    source ~/Projects/felixlm/.venv/bin/activate
    python training/scripts/eval_embedding_ir.py \
        --base-model nomic-ai/nomic-embed-text-v2-moe \
        --finetuned-model models/mnemonic-embed-v1/final
"""

import argparse
import math
import numpy as np
from dataclasses import dataclass, field

# Scenario data extracted from cmd/benchmark-quality/scenarios.go

@dataclass
class Memory:
    id: str
    summary: str
    content: str
    label: str  # "signal" or "noise"

@dataclass
class Query:
    text: str
    expected_ids: list

@dataclass
class Scenario:
    name: str
    memories: list
    queries: list


def build_scenarios():
    scenarios = []

    # 1. Debugging Session
    scenarios.append(Scenario(
        name="Debugging Session",
        memories=[
            Memory("dbg-1", "Nil pointer dereference in auth middleware when token is empty",
                   "Stack trace showed nil pointer in auth.go line 42. The JWT token was nil because the Authorization header was missing entirely. Added a nil check before accessing token.Claims.", "signal"),
            Memory("dbg-2", "Root cause: missing nil check on JWT claims before type assertion",
                   "The real root cause was a type assertion on nil claims. The fix was to check if token != nil && token.Claims != nil before the type assertion. This pattern appears in 3 other middleware handlers too.", "signal"),
            Memory("dbg-3", "Auth crash fix: guard clause for nil token before claims access",
                   "Added guard clause: if token == nil { return unauthorized }. Applied same pattern to refreshToken and validateSession handlers. All three had the same vulnerability.", "signal"),
            Memory("dbg-4", "Regression: auth middleware now rejects valid tokens with empty subject field",
                   "After the nil pointer fix, discovered that some service-to-service tokens have empty Subject fields. The new validation was too strict. Relaxed the check to only require non-nil token and valid claims type.", "signal"),
            Memory("dbg-5", "Added test coverage for auth middleware nil token edge cases",
                   "Wrote table-driven tests covering: nil token, nil claims, empty subject, expired token, and valid token. All pass. The regression is fixed and won't recur.", "signal"),
            Memory("dbg-6", "Panic recovery middleware catches nil pointer panics but loses stack trace",
                   "The panic recovery middleware was catching panics but only logging the recovered value, not the stack trace. Added debug.Stack() to capture full trace on recovery.", "signal"),
            Memory("dbg-7", "Database connection pool exhaustion during load test",
                   "Under 500 concurrent requests, the connection pool (max 25) was exhausted. Root cause: a query in the reporting handler wasn't closing rows. Added defer rows.Close() and increased pool to 50.", "signal"),
            Memory("dbg-8", "Race condition in session cache: concurrent map write panic",
                   "Production panic: concurrent map writes in session cache. Switched from map to sync.Map for the hot path. The session cache is read-heavy so sync.Map is appropriate here.", "signal"),
            Memory("noise-dbg-1", "Chrome opened new tab: reddit.com/r/golang", "Browser activity: navigated to reddit.com/r/golang", "noise"),
            Memory("noise-dbg-2", "File manager: browsed ~/Downloads directory", "Nautilus file browser accessed ~/Downloads", "noise"),
            Memory("noise-dbg-3", "Clipboard: copied URL https://pkg.go.dev/net/http", "Clipboard paste event", "noise"),
            Memory("noise-dbg-4", "node_modules/package-lock.json changed", "File watcher: lockfile updated after npm install", "noise"),
            Memory("noise-dbg-5", ".DS_Store modified in project root", "Filesystem metadata file changed", "noise"),
            Memory("noise-dbg-6", "Chrome LevelDB compaction in Default/Local Storage", "Chrome internal database maintenance", "noise"),
            Memory("noise-dbg-7", "GNOME dconf: changed desktop wallpaper setting", "Desktop settings write to dconf database", "noise"),
            Memory("noise-dbg-8", "Terminal: ran 'ls -la' in home directory", "Shell command execution observed", "noise"),
            Memory("noise-dbg-9", "Terminal: ran 'clear' command", "Terminal screen cleared", "noise"),
            Memory("noise-dbg-10", "PipeWire: audio output switched to headphones", "Audio subsystem configuration change", "noise"),
            Memory("noise-dbg-11", "Trash: moved old-notes.txt to trash", "File deletion via desktop trash", "noise"),
            Memory("noise-dbg-12", "File watcher: .git/index modified", "Git index updated after staging", "noise"),
        ],
        queries=[
            Query("nil pointer bug in auth", ["dbg-1", "dbg-2", "dbg-3"]),
            Query("How did we fix the auth crash", ["dbg-2", "dbg-3", "dbg-5"]),
            Query("What regression issues have we seen", ["dbg-4"]),
        ],
    ))

    # 2. Architecture Decision
    scenarios.append(Scenario(
        name="Architecture Decision",
        memories=[
            Memory("arch-1", "Chose SQLite over Postgres because no server dependency needed",
                   "Decision: SQLite for the data store. Postgres would give us better concurrency but requires a server process. Since mnemonic is local-first and single-user, SQLite with WAL mode is sufficient and eliminates the deployment complexity.", "signal"),
            Memory("arch-2", "Decided on event bus architecture over direct agent calls",
                   "Agents communicate via event bus, not direct function calls. This decouples them so we can add/remove agents without modifying others. The tradeoff is slightly more complex debugging since events are asynchronous.", "signal"),
            Memory("arch-3", "8 cognitive agents for separation of concerns",
                   "Settled on 8 specialized agents plus an orchestrator. Each maps to a cognitive function. Considered fewer (3-4 monolithic agents) but the fine-grained split makes testing easier and maps better to the memory science literature.", "signal"),
            Memory("arch-4", "Considered event sourcing but chose CRUD with temporal metadata",
                   "Event sourcing would give us full history replay but adds significant complexity. Instead, we store memories with timestamps, access counts, and state transitions.", "signal"),
            Memory("arch-5", "FTS5 for full-text search instead of external Elasticsearch",
                   "SQLite FTS5 gives us BM25-ranked full-text search without an external service. Combined with our in-memory embedding index for semantic search. Two retrieval paths that get merged with configurable weights.", "signal"),
            Memory("arch-6", "Local LLM via LM Studio for air-gapped semantic processing",
                   "All LLM operations go through a local model running in LM Studio. No cloud API calls. This means encoding quality depends on the local model, but we get full privacy and offline operation.", "signal"),
            Memory("arch-7", "Spread activation for associative retrieval with 3-hop max",
                   "Retrieval uses spread activation across the association graph. Activation decays exponentially per hop with a 0.7 factor. After testing, 3 hops is the sweet spot.", "signal"),
            Memory("arch-8", "Config-driven tunables in config.yaml rather than hardcoded values",
                   "All tunable parameters live in config.yaml: decay rates, thresholds, intervals, model settings. This lets users adjust behavior without recompiling.", "signal"),
            Memory("noise-arch-1", "GNOME dconf: workspace count changed to 4", "Desktop settings write", "noise"),
            Memory("noise-arch-2", "LM Studio: downloaded qwen2.5-coder-7b model", "Model download activity", "noise"),
            Memory("noise-arch-3", "Trash: removed old-backup.tar.gz", "File deletion", "noise"),
            Memory("noise-arch-4", ".DS_Store created in internal/agent/", "Filesystem metadata", "noise"),
            Memory("noise-arch-5", "Chrome: visited stackoverflow.com/questions/tagged/go", "Browser navigation", "noise"),
            Memory("noise-arch-6", "Terminal: ran 'git log --oneline' in ~/Projects/mem", "Shell command", "noise"),
            Memory("noise-arch-7", "File watcher: go.sum modified after go mod tidy", "Dependency file change", "noise"),
            Memory("noise-arch-8", "Chrome LevelDB: Default/IndexedDB compaction", "Browser database maintenance", "noise"),
            Memory("noise-arch-9", "PipeWire: microphone input level adjusted", "Audio settings change", "noise"),
            Memory("noise-arch-10", "Nautilus: browsed /usr/share/fonts directory", "File manager activity", "noise"),
            Memory("noise-arch-11", "Terminal: ran 'top' for 3 seconds", "System monitoring command", "noise"),
            Memory("noise-arch-12", "GNOME: screen locked due to idle timeout", "Desktop idle event", "noise"),
        ],
        queries=[
            Query("Why did we choose SQLite", ["arch-1", "arch-5"]),
            Query("What architecture decision have we made", ["arch-1", "arch-2", "arch-3", "arch-4", "arch-5"]),
            Query("What were the tradeoff considerations", ["arch-2", "arch-4", "arch-6"]),
        ],
    ))

    # 3. Learning & Insights
    scenarios.append(Scenario(
        name="Learning & Insights",
        memories=[
            Memory("learn-1", "Go's sql.NullString needed for nullable columns in SQLite",
                   "When scanning nullable TEXT columns from SQLite, you need sql.NullString (or *string). A plain string will panic on NULL values.", "signal"),
            Memory("learn-2", "FTS5 rank function returns negative BM25 scores (lower is better)",
                   "SQLite FTS5's rank column returns negative BM25 scores. More negative = better match. This is counterintuitive. We negate and normalize to 0-1 range.", "signal"),
            Memory("learn-3", "Spread activation works best with 3 hops max for our graph density",
                   "Tested spread activation with 1-5 hops. At 4+ hops, too many unrelated memories get activated. At 1-2 hops, we miss important transitive connections. 3 hops with 0.7 decay is the sweet spot.", "signal"),
            Memory("learn-4", "Go embed directive requires files to be in or below the package directory",
                   "go:embed can only access files in the same directory or subdirectories. Tried to embed ../config.yaml and got a compile error.", "signal"),
            Memory("learn-5", "slog.Handler must implement Enabled, Handle, WithAttrs, WithGroup",
                   "Implementing a custom slog.Handler requires all four methods. Forgot WithGroup initially and got a compile error.", "signal"),
            Memory("learn-6", "Cosine similarity of zero vectors returns NaN — must guard against it",
                   "If either embedding is all zeros (failed encoding), cosine similarity returns NaN which propagates through the scoring pipeline. Added a guard.", "signal"),
            Memory("learn-7", "Pure-Go SQLite driver modernc.org/sqlite eliminates CGO requirement",
                   "Migrated from mattn/go-sqlite3 (CGO) to modernc.org/sqlite (pure Go). No more CGO_ENABLED=1 needed. FTS5 is included by default.", "signal"),
            Memory("learn-8", "D3 force simulation needs alpha decay tuning for stable layouts",
                   "D3's force simulation alpha decays to 0. For our graph, alphaDecay of 0.02 (slower) produces more stable layouts.", "signal"),
            Memory("noise-learn-1", "Clipboard: copied https://go.dev/doc/effective_go", "URL clipboard event", "noise"),
            Memory("noise-learn-2", "Terminal: ran 'cd ~/Projects/mem' command", "Directory change", "noise"),
            Memory("noise-learn-3", "Terminal: ran 'clear' command", "Terminal clear", "noise"),
            Memory("noise-learn-4", "Terminal: ran 'ls -la internal/' command", "Directory listing", "noise"),
            Memory("noise-learn-5", "Chrome: opened 5 new tabs from search results", "Browser activity", "noise"),
            Memory("noise-learn-6", "PipeWire: bluetooth headphones connected", "Audio device event", "noise"),
            Memory("noise-learn-7", "File watcher: .git/COMMIT_EDITMSG modified", "Git commit editing", "noise"),
            Memory("noise-learn-8", "GNOME: notification from Slack: new message in #general", "Desktop notification", "noise"),
            Memory("noise-learn-9", "Terminal: ran 'make test' — 42 tests passed", "Test execution", "noise"),
            Memory("noise-learn-10", "Clipboard: copied error message from terminal output", "Clipboard event", "noise"),
            Memory("noise-learn-11", "File watcher: /tmp/go-build cache updated", "Build cache change", "noise"),
            Memory("noise-learn-12", "Chrome: downloaded go1.22.0.linux-amd64.tar.gz", "File download", "noise"),
        ],
        queries=[
            Query("What did we learn about FTS5", ["learn-2"]),
            Query("Go gotchas and quirks", ["learn-1", "learn-4", "learn-5", "learn-7"]),
            Query("What patterns work well for retrieval", ["learn-3", "learn-6"]),
        ],
    ))

    # 4. Deep Graph Investigation
    scenarios.append(Scenario(
        name="Deep Graph Investigation",
        memories=[
            Memory("inv-1", "Alert: API latency spike detected in monitoring dashboard",
                   "PagerDuty alert fired for p99 latency exceeding 5s on /api/v1/search endpoint.", "signal"),
            Memory("inv-2", "Traced latency to database query in search handler",
                   "Profiling shows 90% of latency in SearchHandler.Query(). The FTS5 query is doing a full table scan.", "signal"),
            Memory("inv-3", "FTS5 index corruption caused by direct DELETE without rebuild",
                   "Found the root cause: a cleanup job was running DELETE FROM memories_fts WHERE rowid IN (...) directly. FTS5 requires using the content table for deletes.", "signal"),
            Memory("inv-4", "Fix: rebuilt FTS5 index using INSERT INTO memories_fts(memories_fts) VALUES('rebuild')",
                   "Rebuilt the FTS5 index with the rebuild command. Latency immediately dropped back to normal.", "signal"),
            Memory("inv-5", "Prevention: rewrote cleanup job to use content table DELETE pattern",
                   "Rewrote the cleanup job to delete from the content table instead of directly from the FTS5 table.", "signal"),
            Memory("inv-6", "Post-mortem: added FTS5 index health check to monitoring suite",
                   "Added automated FTS5 integrity check to the health monitoring suite. Runs every hour.", "signal"),
            Memory("inv-7", "Similar issue last month: embedding index got stale after bulk import",
                   "Recalled a similar incident where the embedding similarity index became stale after a bulk import.", "signal"),
            Memory("inv-8", "General principle: always use the ORM/content table path for writes",
                   "This is the second time bypassing the content table path caused index corruption. Establishing a rule.", "signal"),
            Memory("inv-9", "The cleanup job was deployed 3 days ago in release v0.7.2",
                   "Git blame shows the problematic cleanup job was added in commit abc123 as part of v0.7.2.", "signal"),
            Memory("inv-10", "Lesson: need pre-deploy check that validates FTS5 writes go through content table",
                   "Adding a CI lint rule that scans for direct INSERT/DELETE on *_fts tables.", "signal"),
            Memory("noise-inv-1", "Terminal: ran 'htop' for system monitoring", "Process monitoring activity", "noise"),
            Memory("noise-inv-2", "Chrome: browsed SQLite documentation page", "Browser navigation", "noise"),
            Memory("noise-inv-3", "File watcher: go.sum changed after dependency update", "Dependency file change", "noise"),
            Memory("noise-inv-4", "Clipboard: copied SQL query from chat", "Clipboard event", "noise"),
            Memory("noise-inv-5", "Terminal: ran 'git status' in project directory", "Git status check", "noise"),
            Memory("noise-inv-6", "GNOME: screen brightness adjusted", "Display settings change", "noise"),
            Memory("noise-inv-7", "Terminal: ran 'df -h' to check disk space", "Disk space check", "noise"),
            Memory("noise-inv-8", "Chrome: visited GitHub issues page", "Browser navigation", "noise"),
            Memory("noise-inv-9", "PipeWire: audio output volume changed", "Audio settings adjustment", "noise"),
            Memory("noise-inv-10", "Terminal: ran 'make test' in project root", "Test execution", "noise"),
            Memory("noise-inv-11", "File watcher: /tmp/benchmark-* directory created", "Temp file creation", "noise"),
            Memory("noise-inv-12", "Terminal: ran 'tail -f /var/log/syslog'", "Log monitoring", "noise"),
        ],
        queries=[
            Query("How did we fix the latency issue", ["inv-3", "inv-4", "inv-5"]),
            Query("What caused the FTS5 index corruption", ["inv-2", "inv-3", "inv-9"]),
            Query("What principles did we establish from this incident", ["inv-8", "inv-10"]),
            Query("What similar incidents have we seen before", ["inv-7"]),
        ],
    ))

    # 5. Needle in Haystack
    scenarios.append(Scenario(
        name="Needle in Haystack",
        memories=[
            Memory("needle-1", "Decision: use WAL mode for SQLite to improve concurrent read performance",
                   "Switched SQLite to WAL journal mode. This allows concurrent readers while a write is in progress.", "signal"),
            Memory("needle-2", "Insight: slog structured logging is significantly better than fmt.Printf for debugging agents",
                   "After switching all agent logging to slog, debugging became much easier. Structured fields make it possible to filter and trace.", "signal"),
            Memory("needle-3", "Error: nil pointer in consolidation agent when processing memories with no embedding",
                   "Consolidation agent panicked on memories that had empty embeddings (encoding failed). Added nil check before cosine similarity.", "signal"),
            Memory("needle-4", "Learning: Go's context.WithTimeout doesn't cancel if you forget to defer cancel()",
                   "Found a goroutine leak in the retrieval agent. context.WithTimeout returns a cancel func that MUST be deferred.", "signal"),
            Memory("noise-needle-1", "Chrome: opened 3 tabs from Google search results", "Browser activity", "noise"),
            Memory("noise-needle-2", "File watcher: node_modules/.cache updated", "Build cache write", "noise"),
            Memory("noise-needle-3", "Terminal: ran 'ls -la internal/agent/' to list files", "Directory listing", "noise"),
            Memory("noise-needle-4", "Clipboard: copied import path github.com/appsprout-dev/mnemonic", "Go import path copied", "noise"),
            Memory("noise-needle-5", "Chrome: read Go blog post about generics", "Browser: reading technical content", "noise"),
            Memory("noise-needle-6", "File watcher: .git/FETCH_HEAD modified after git fetch", "Git metadata update", "noise"),
            Memory("noise-needle-7", "Terminal: ran 'go doc context.WithTimeout'", "Go documentation lookup", "noise"),
            Memory("noise-needle-8", "GNOME: notification from Signal: 2 new messages", "Desktop notification", "noise"),
            Memory("noise-needle-9", "File watcher: internal/store/sqlite/store.go saved", "File save event", "noise"),
            Memory("noise-needle-10", "Terminal: ran 'curl localhost:9999/api/v1/health'", "Health check API call", "noise"),
            Memory("noise-needle-11", "Chrome: visited pkg.go.dev/database/sql", "Go package documentation", "noise"),
            Memory("noise-needle-12", "PipeWire: switched output to speakers", "Audio output change", "noise"),
            Memory("noise-needle-13", "File watcher: bin/mnemonic rebuilt", "Binary rebuild event", "noise"),
            Memory("noise-needle-14", "Terminal: ran 'systemctl --user status mnemonic'", "Service status check", "noise"),
            Memory("noise-needle-15", "Clipboard: copied error message 'database is locked'", "Error message clipboard event", "noise"),
            Memory("noise-needle-16", "Chrome: browsed SQLite WAL documentation", "Browser: SQLite docs", "noise"),
            Memory("noise-needle-17", "File watcher: config.yaml modified", "Config file change", "noise"),
            Memory("noise-needle-18", "Terminal: ran 'git diff HEAD~1' to review changes", "Git diff command", "noise"),
            Memory("noise-needle-19", "GNOME: workspace switched from 1 to 2", "Desktop workspace change", "noise"),
            Memory("noise-needle-20", "File watcher: internal/agent/consolidation/agent_test.go saved", "Test file save", "noise"),
            Memory("noise-needle-21", "Terminal: ran 'make build' successfully", "Build command execution", "noise"),
            Memory("noise-needle-22", "Chrome: opened mnemonic GitHub issues page", "Browser: project management", "noise"),
            Memory("noise-needle-23", "Clipboard: copied function signature from agent.go", "Code snippet clipboard event", "noise"),
            Memory("noise-needle-24", "Terminal: ran 'go test -v ./internal/store/...' — 18 tests passed", "Test execution", "noise"),
            Memory("noise-needle-25", "File watcher: memory.db-wal grew to 2MB", "WAL file size change", "noise"),
        ],
        queries=[
            Query("What decision did we make about the database", ["needle-1"]),
            Query("What error and bug have we found", ["needle-3", "needle-4"]),
            Query("What did we learn about Go and logging", ["needle-2", "needle-4"]),
        ],
    ))

    # 6. Associative Recall
    scenarios.append(Scenario(
        name="Associative Recall",
        memories=[
            Memory("ar-1", "Authentication service returning 503 errors during peak traffic hours",
                   "The auth service started returning 503 errors at 2pm during peak load. Users unable to log in. 40% error rate on /auth/token endpoint.", "signal"),
            Memory("ar-2", "Redis connection pool fully exhausted — all 25 slots occupied by stale handles",
                   "Investigated the backing store and found the Redis connection pool completely drained. netstat shows 25 ESTABLISHED connections to port 6379.", "signal"),
            Memory("ar-3", "Unclosed connections in token validation — conn.Close() missing from error return path",
                   "Root cause found in validateToken(). When HMAC verification fails, the function returns early but never calls conn.Close().", "signal"),
            Memory("ar-4", "Applied defer conn.Close() to all Redis call sites and bumped pool ceiling to 50",
                   "Patched all 7 Redis call sites to use defer conn.Close() immediately after acquiring. Raised MaxIdle from 25 to 50.", "signal"),
            Memory("ar-5", "Dashboard page load times tripled after Tuesday's release to production",
                   "After deploying v2.8.0 on Tuesday, the main dashboard went from 800ms to 2.4s load time.", "signal"),
            Memory("ar-6", "New analytics aggregation query running without an index — sequential scan on 4M row table",
                   "EXPLAIN ANALYZE revealed the new monthly_summary view does a sequential scan on the events table (4.2M rows).", "signal"),
            Memory("ar-7", "Created migration 042 with composite index on (org_id, created_at DESC)",
                   "Wrote migration file 042_add_events_org_created_idx.sql. Uses CREATE INDEX CONCURRENTLY. Dashboard load times back to 750ms.", "signal"),
            Memory("ar-8", "E-commerce team reported 12% cart abandonment spike correlated with auth outage",
                   "Product analytics showed a 12% increase in cart abandonment between 2pm and 4pm. Estimated revenue impact: $47K.", "signal"),
            Memory("noise-ar-1", "Chrome: visited pkg.go.dev/context documentation", "Browser navigation to Go docs", "noise"),
            Memory("noise-ar-2", "Terminal: executed 'df -h' to check available disk space", "Disk usage monitoring", "noise"),
            Memory("noise-ar-3", "File watcher: .git/objects directory modified", "Git internal object creation", "noise"),
            Memory("noise-ar-4", "Clipboard: copied JSON payload from Postman response", "API testing clipboard activity", "noise"),
            Memory("noise-ar-5", "GNOME: display brightness adjusted via slider", "Desktop brightness control", "noise"),
            Memory("noise-ar-6", "Terminal: ran 'docker logs api-gateway --tail 50'", "Container log inspection", "noise"),
            Memory("noise-ar-7", "File watcher: /var/log/syslog rotated by logrotate", "System log rotation", "noise"),
            Memory("noise-ar-8", "Chrome: opened three Stack Overflow tabs about goroutine patterns", "Browser research", "noise"),
            Memory("noise-ar-9", "Terminal: executed 'kubectl get pods -n staging'", "Kubernetes inspection", "noise"),
            Memory("noise-ar-10", "PipeWire: USB microphone connected and configured", "Audio hardware plug", "noise"),
            Memory("noise-ar-11", "File watcher: node_modules/.package-lock.json updated", "NPM lockfile modification", "noise"),
            Memory("noise-ar-12", "Terminal: ran 'htop' for 5 seconds then quit", "System resource monitoring", "noise"),
            Memory("noise-ar-13", "Clipboard: copied function signature from source file", "Code snippet clipboard", "noise"),
            Memory("noise-ar-14", "GNOME: workspace switched from workspace 2 to workspace 3", "Virtual desktop navigation", "noise"),
            Memory("noise-ar-15", "File watcher: ~/.local/share/Trash/files updated", "Trash directory modification", "noise"),
        ],
        queries=[
            Query("What caused the authentication errors", ["ar-1", "ar-2", "ar-3"]),
            Query("Why did the deployment make things slow", ["ar-5", "ar-6", "ar-7"]),
            Query("What was the business impact of the auth incident", ["ar-1", "ar-8"]),
            Query("How did we fix the Redis connection pool exhaustion", ["ar-2", "ar-3", "ar-4"]),
        ],
    ))

    return scenarios


# --- IR Metrics ---

def cosine_sim(a, b):
    dot = np.dot(a, b)
    na, nb = np.linalg.norm(a), np.linalg.norm(b)
    if na == 0 or nb == 0:
        return 0.0
    return float(dot / (na * nb))


def dcg_at_k(relevances, k):
    """Discounted Cumulative Gain at k."""
    dcg = 0.0
    for i in range(min(k, len(relevances))):
        dcg += relevances[i] / math.log2(i + 2)
    return dcg


def ndcg_at_k(relevances, k):
    """Normalized DCG at k."""
    dcg = dcg_at_k(relevances, k)
    ideal = dcg_at_k(sorted(relevances, reverse=True), k)
    if ideal == 0:
        return 0.0
    return dcg / ideal


def evaluate_scenario(model, scenario, k=5, prefix=""):
    """Embed all memories and queries, compute IR metrics."""
    # Build memory texts
    mem_texts = []
    mem_ids = []
    for m in scenario.memories:
        text = f"{m.summary} {m.content}"
        if prefix:
            text = f"{prefix}: {text}"
        mem_texts.append(text)
        mem_ids.append(m.id)

    # Embed memories
    mem_embeddings = model.encode(mem_texts, show_progress_bar=False)

    query_results = []
    for q in scenario.queries:
        # Embed query
        q_text = f"{prefix}: {q.text}" if prefix else q.text
        q_emb = model.encode([q_text], show_progress_bar=False)[0]

        # Rank by cosine similarity
        sims = [(mem_ids[i], cosine_sim(q_emb, mem_embeddings[i])) for i in range(len(mem_ids))]
        sims.sort(key=lambda x: x[1], reverse=True)

        # Top-k results
        top_k = sims[:k]
        top_k_ids = [s[0] for s in top_k]

        expected_set = set(q.expected_ids)

        # Precision@k
        hits = sum(1 for mid in top_k_ids if mid in expected_set)
        precision = hits / k

        # Recall@k
        recall = hits / len(expected_set) if expected_set else 0.0

        # MRR
        mrr = 0.0
        for rank, mid in enumerate(top_k_ids, 1):
            if mid in expected_set:
                mrr = 1.0 / rank
                break

        # nDCG@k
        relevances = [1.0 if mid in expected_set else 0.0 for mid in top_k_ids]
        ndcg = ndcg_at_k(relevances, k)

        query_results.append({
            "query": q.text,
            "precision": precision,
            "recall": recall,
            "mrr": mrr,
            "ndcg": ndcg,
            "top_k": [(mid, f"{sim:.4f}") for mid, sim in top_k],
        })

    return query_results


def print_results(name, scenario_results, scenarios):
    """Print formatted results table."""
    print(f"\n{'='*80}")
    print(f"  {name}")
    print(f"{'='*80}")

    total_p, total_r, total_mrr, total_ndcg, total_q = 0, 0, 0, 0, 0

    for sc, results in zip(scenarios, scenario_results):
        print(f"\n  {sc.name}")
        print(f"  {'─'*76}")
        print(f"  {'Query':<45} {'P@5':>6} {'R@5':>6} {'MRR':>6} {'nDCG':>6}")
        print(f"  {'─'*76}")

        for r in results:
            q_short = r["query"][:43] + ".." if len(r["query"]) > 45 else r["query"]
            print(f"  {q_short:<45} {r['precision']:>6.3f} {r['recall']:>6.3f} {r['mrr']:>6.3f} {r['ndcg']:>6.3f}")
            total_p += r["precision"]
            total_r += r["recall"]
            total_mrr += r["mrr"]
            total_ndcg += r["ndcg"]
            total_q += 1

    print(f"\n  {'─'*76}")
    print(f"  {'AVERAGE':<45} {total_p/total_q:>6.3f} {total_r/total_q:>6.3f} {total_mrr/total_q:>6.3f} {total_ndcg/total_q:>6.3f}")
    print()

    return {
        "precision": total_p / total_q,
        "recall": total_r / total_q,
        "mrr": total_mrr / total_q,
        "ndcg": total_ndcg / total_q,
    }


def main():
    parser = argparse.ArgumentParser(description="Evaluate embedding models on mnemonic IR benchmark")
    parser.add_argument("--base-model", type=str, default="nomic-ai/nomic-embed-text-v2-moe")
    parser.add_argument("--finetuned-model", type=str, default="models/mnemonic-embed-v1/final")
    parser.add_argument("--k", type=int, default=5, help="Top-k for metrics")
    parser.add_argument("--finetuned-only", action="store_true", help="Skip base model eval")
    args = parser.parse_args()

    from sentence_transformers import SentenceTransformer

    scenarios = build_scenarios()

    # Determine prefix
    MODEL_PREFIXES = {
        "nomic-ai/nomic-embed-text-v2-moe": "search_document",
        "nomic-ai/nomic-embed-text-v1.5": "search_document",
    }
    prefix = MODEL_PREFIXES.get(args.base_model, "")

    if not args.finetuned_only:
        # Evaluate base model
        print(f"Loading base model: {args.base_model}...")
        base_model = SentenceTransformer(args.base_model, trust_remote_code=True)
        print(f"  Dim: {base_model.get_sentence_embedding_dimension()}")

        base_results = []
        for sc in scenarios:
            base_results.append(evaluate_scenario(base_model, sc, args.k, prefix))

        base_agg = print_results(f"BASE: {args.base_model}", base_results, scenarios)

        # Free memory
        del base_model
        import gc; gc.collect()
        import torch; torch.cuda.empty_cache() if torch.cuda.is_available() else None

    # Evaluate fine-tuned model
    print(f"Loading fine-tuned model: {args.finetuned_model}...")
    ft_model = SentenceTransformer(args.finetuned_model, trust_remote_code=True)
    print(f"  Dim: {ft_model.get_sentence_embedding_dimension()}")

    ft_results = []
    for sc in scenarios:
        ft_results.append(evaluate_scenario(ft_model, sc, args.k, prefix))

    ft_agg = print_results(f"FINE-TUNED: {args.finetuned_model}", ft_results, scenarios)

    # Comparison
    if not args.finetuned_only:
        print(f"\n{'='*80}")
        print(f"  COMPARISON (fine-tuned vs base)")
        print(f"{'='*80}")
        for metric in ["precision", "recall", "mrr", "ndcg"]:
            base_val = base_agg[metric]
            ft_val = ft_agg[metric]
            delta = ft_val - base_val
            pct = (delta / base_val * 100) if base_val > 0 else float('inf')
            marker = "+" if delta > 0 else ""
            print(f"  {metric.upper():<12} base={base_val:.4f}  ft={ft_val:.4f}  delta={marker}{delta:.4f} ({marker}{pct:.1f}%)")
        print()


if __name__ == "__main__":
    main()
