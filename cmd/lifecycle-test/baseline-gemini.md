# Mnemonic Lifecycle Simulation Results

## Summary

| Phase | Assertions | Duration | Status |
|-------|-----------|----------|--------|
| install | 5/5 | 0ms | PASS |
| first-use | 7/7 | 35533ms | PASS |
| ingest | 2/2 | 27190ms | PASS |
| daily | 3/3 | 1447568ms | PASS |
| consolidation | 1/1 | 11490ms | PASS |
| dreaming | 0/0 | 2036ms | PASS |
| growth | 3/3 | 2702663ms | PASS |
| longterm | 2/2 | 4916ms | PASS |

**Total: 23 passed, 0 failed**

## Phase Details

### install

- [x] table count (expected: >= 15, actual: 23)
- [x] FTS5 table present (expected: 1, actual: 1)
- [x] zero memories (expected: 0, actual: 0)
- [x] zero episodes (expected: 0, actual: 0)
- [x] zero associations (expected: 0, actual: 0)

**Metrics:**

- tables: 23.00

### first-use

- [x] encoded count (expected: 10, actual: 10)
- [x] episodes created (expected: >= 1, actual: 1)
- [x] total memories (expected: 10, actual: 10)
- [x] all have concepts (expected: true, actual: true)
- [x] all have embeddings (expected: true, actual: true)
- [x] all active state (expected: true, actual: true)
- [x] retrieval returns results (expected: > 0, actual: 5)

**Metrics:**

- encoded: 10.00
- episodes: 1.00
- retrieval_results: 5.00

### ingest

- [x] files written (expected: >= 3, actual: 8)
- [x] dedup: zero new writes (expected: 0, actual: 0)

**Metrics:**

- duplicates_skipped: 0.00
- encoded: 8.00
- files_found: 8.00
- files_skipped: 0.00
- files_written: 8.00

### daily

- [x] total memories (expected: >= 40, actual: 97)
- [x] episodes created (expected: >= 5, actual: 317)
- [x] associations created (expected: >= 1, actual: 470)

**Metrics:**

- feedback_count: 0.00
- total_associations: 470.00
- total_episodes: 317.00
- total_memories: 97.00
- total_written: 266.00

### consolidation

- [x] some memories transitioned (expected: > 0, actual: 53)

**Metrics:**

- mcp_active: 39.00
- patterns: 4.00
- post_active: 44.00
- post_archived: 4.00
- post_fading: 49.00
- pre_active: 97.00

### dreaming


**Metrics:**

- assocs_strengthened: 488.00
- avg_assocs_per_memory: 5.56
- axioms_created: 0.00
- cross_project_links: 0.00
- insights_generated: 1.00
- memories_replayed: 44.00
- new_assocs: 69.00
- observations: 0.00
- principles_created: 0.00
- total_abstractions: 1.00
- total_associations: 539.00
- total_observations: 0.00

### growth

- [x] total memories >= 60 (expected: >= 60, actual: 115)
- [x] not all active (expected: < 115, actual: 40)
- [x] retrieval returns results (expected: > 0, actual: 24)

**Metrics:**

- abstractions: 4.00
- active_memories: 40.00
- archived_memories: 9.00
- avg_retrieval_latency_ms: 758.00
- avg_retrieval_results: 4.80
- fading_memories: 66.00
- total_added: 596.00
- total_associations: 704.00
- total_memories: 115.00

### longterm

- [x] some archived (expected: > 0, actual: 109)
- [x] active < total (expected: < 115, actual: 0)

**Metrics:**

- active_memories: 0.00
- archived_memories: 109.00
- audit_observations: 0.00
- db_size_mb: 5.33
- fading_memories: 6.00
- regression_results: 15.00
- storage_bytes: 5591040.00
- total_memories: 115.00

