-- Migration 006: Add composite indexes for common query patterns
--
-- ListMemories: WHERE state = ? ORDER BY created_at DESC LIMIT ?
-- Currently uses idx_memory_state but still sorts 33K+ rows.
-- Composite index makes ORDER BY + LIMIT essentially free.

CREATE INDEX IF NOT EXISTS idx_memories_state_created ON memories(state, created_at DESC);

-- ListMemoriesByProject: WHERE project = ? AND state IN (...) ORDER BY timestamp DESC
CREATE INDEX IF NOT EXISTS idx_memories_project_state ON memories(project, state, timestamp DESC);

-- Episode memory lookup: WHERE episode_id = ?
CREATE INDEX IF NOT EXISTS idx_memories_episode ON memories(episode_id) WHERE episode_id IS NOT NULL;
