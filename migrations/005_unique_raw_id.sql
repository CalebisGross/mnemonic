-- Migration 005: Add unique constraint on memories.raw_id
-- Prevents duplicate encoding of the same raw memory across processes.

-- Step 1: Remove duplicate encoded memories, keeping only the oldest per raw_id.
-- For each raw_id with multiple encoded memories, keep the one with the earliest
-- created_at and archive the rest.
UPDATE memories
SET state = 'archived'
WHERE raw_id IS NOT NULL
  AND id NOT IN (
    SELECT id FROM (
      SELECT id, ROW_NUMBER() OVER (PARTITION BY raw_id ORDER BY created_at ASC) as rn
      FROM memories
      WHERE raw_id IS NOT NULL
    ) ranked
    WHERE rn = 1
  )
  AND state != 'archived';

-- Step 2: Delete archived duplicates (they have no unique value).
DELETE FROM memories
WHERE raw_id IS NOT NULL
  AND id NOT IN (
    SELECT id FROM (
      SELECT id, ROW_NUMBER() OVER (PARTITION BY raw_id ORDER BY created_at ASC) as rn
      FROM memories
      WHERE raw_id IS NOT NULL
    ) ranked
    WHERE rn = 1
  );

-- Step 3: Create unique index on raw_id (partial — allows NULLs).
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_raw_id_unique ON memories(raw_id) WHERE raw_id IS NOT NULL;
