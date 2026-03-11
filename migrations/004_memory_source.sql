-- Add source column to memories table to track origin (filesystem, terminal, clipboard, mcp, consolidation, etc.)
ALTER TABLE memories ADD COLUMN source TEXT;

-- Backfill source from raw_memories where possible
UPDATE memories SET source = (
    SELECT raw_memories.source FROM raw_memories WHERE raw_memories.id = memories.raw_id
) WHERE memories.raw_id IS NOT NULL AND memories.raw_id != '';
