package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
)

const schema = `
-- Raw observations before encoding
CREATE TABLE IF NOT EXISTS raw_memories (
    id TEXT PRIMARY KEY,
    timestamp DATETIME NOT NULL,
    source TEXT NOT NULL,
    type TEXT,
    content TEXT NOT NULL,
    metadata JSON,
    heuristic_score REAL DEFAULT 0.5,
    initial_salience REAL DEFAULT 0.5,
    processed BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_raw_timestamp ON raw_memories(timestamp);
CREATE INDEX IF NOT EXISTS idx_raw_processed ON raw_memories(processed);

-- Encoded memories
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    raw_id TEXT REFERENCES raw_memories(id),
    timestamp DATETIME NOT NULL,
    content TEXT NOT NULL,
    summary TEXT NOT NULL,
    concepts JSON,
    embedding BLOB,
    salience REAL DEFAULT 0.5,
    access_count INTEGER DEFAULT 0,
    last_accessed DATETIME,
    state TEXT DEFAULT 'active',
    gist_of JSON,
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_memory_state ON memories(state);
CREATE INDEX IF NOT EXISTS idx_memory_salience ON memories(salience);
CREATE INDEX IF NOT EXISTS idx_memory_timestamp ON memories(timestamp);

-- FTS5 for full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    summary,
    content,
    concepts,
    content='memories',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, summary, content, concepts)
    VALUES (new.rowid, new.summary, new.content, new.concepts);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, summary, content, concepts)
    VALUES('delete', old.rowid, old.summary, old.content, old.concepts);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, summary, content, concepts)
    VALUES('delete', old.rowid, old.summary, old.content, old.concepts);
    INSERT INTO memories_fts(rowid, summary, content, concepts)
    VALUES (new.rowid, new.summary, new.content, new.concepts);
END;

-- Association graph
CREATE TABLE IF NOT EXISTS associations (
    source_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    strength REAL DEFAULT 0.5,
    relation_type TEXT DEFAULT 'similar',
    created_at DATETIME DEFAULT (datetime('now')),
    last_activated DATETIME,
    activation_count INTEGER DEFAULT 0,
    PRIMARY KEY (source_id, target_id)
);
CREATE INDEX IF NOT EXISTS idx_assoc_source ON associations(source_id);
CREATE INDEX IF NOT EXISTS idx_assoc_target ON associations(target_id);
CREATE INDEX IF NOT EXISTS idx_assoc_strength ON associations(strength);

-- Meta-cognition observations
CREATE TABLE IF NOT EXISTS meta_observations (
    id TEXT PRIMARY KEY,
    observation_type TEXT NOT NULL,
    severity TEXT DEFAULT 'info',
    details JSON,
    created_at DATETIME DEFAULT (datetime('now'))
);

-- Retrieval feedback
CREATE TABLE IF NOT EXISTS retrieval_feedback (
    query_id TEXT PRIMARY KEY,
    query_text TEXT NOT NULL,
    retrieved_memory_ids JSON,
    traversed_assocs JSON,
    access_snapshot JSON,
    feedback TEXT,
    notes TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);

-- Consolidation history
CREATE TABLE IF NOT EXISTS consolidation_history (
    id TEXT PRIMARY KEY,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    duration_ms INTEGER,
    memories_processed INTEGER,
    memories_decayed INTEGER,
    merged_clusters INTEGER,
    associations_pruned INTEGER,
    created_at DATETIME DEFAULT (datetime('now'))
);
`

const migration002 = `
-- Migration 002: Episodic Memory Architecture

-- Episodic containers
CREATE TABLE IF NOT EXISTS episodes (
    id TEXT PRIMARY KEY,
    title TEXT,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    duration_sec INTEGER,
    raw_memory_ids JSON,
    memory_ids JSON,
    summary TEXT,
    narrative TEXT,
    salience REAL DEFAULT 0.5,
    emotional_tone TEXT,
    outcome TEXT,
    state TEXT DEFAULT 'open',
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_episode_start ON episodes(start_time);
CREATE INDEX IF NOT EXISTS idx_episode_end ON episodes(end_time);
CREATE INDEX IF NOT EXISTS idx_episode_state ON episodes(state);
CREATE INDEX IF NOT EXISTS idx_episode_salience ON episodes(salience);

-- Multi-resolution memory
CREATE TABLE IF NOT EXISTS memory_resolutions (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    gist TEXT,
    narrative TEXT,
    detail_raw_ids JSON,
    created_at DATETIME DEFAULT (datetime('now'))
);

-- Structured concepts
CREATE TABLE IF NOT EXISTS concept_sets (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    topics JSON,
    entities JSON,
    actions JSON,
    causality JSON,
    significance TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);

-- Emotional/motivational valence
CREATE TABLE IF NOT EXISTS memory_attributes (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    significance TEXT,
    emotional_tone TEXT,
    outcome TEXT,
    causality_notes TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);
`

const migration004 = `
-- Migration 004: Patterns, Abstractions, and Project/Session context

-- Patterns discovered through consolidation
CREATE TABLE IF NOT EXISTS patterns (
    id TEXT PRIMARY KEY,
    pattern_type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    evidence_ids JSON DEFAULT '[]',
    strength REAL DEFAULT 0.5,
    project TEXT,
    concepts JSON DEFAULT '[]',
    embedding BLOB,
    access_count INTEGER DEFAULT 0,
    last_accessed DATETIME,
    state TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_pattern_project ON patterns(project);
CREATE INDEX IF NOT EXISTS idx_pattern_state ON patterns(state);
CREATE INDEX IF NOT EXISTS idx_pattern_strength ON patterns(strength);
CREATE INDEX IF NOT EXISTS idx_pattern_type ON patterns(pattern_type);

-- Abstractions: hierarchical knowledge (memories -> patterns -> principles -> axioms)
CREATE TABLE IF NOT EXISTS abstractions (
    id TEXT PRIMARY KEY,
    level INTEGER DEFAULT 1,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    parent_id TEXT,
    source_pattern_ids JSON DEFAULT '[]',
    source_memory_ids JSON DEFAULT '[]',
    confidence REAL DEFAULT 0.5,
    concepts JSON DEFAULT '[]',
    embedding BLOB,
    access_count INTEGER DEFAULT 0,
    state TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_abstraction_level ON abstractions(level);
CREATE INDEX IF NOT EXISTS idx_abstraction_state ON abstractions(state);
CREATE INDEX IF NOT EXISTS idx_abstraction_confidence ON abstractions(confidence);
`

const migration005 = `
-- Migration 005: System metadata key-value store
CREATE TABLE IF NOT EXISTS system_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT (datetime('now'))
);
`

const migration006 = `
-- Migration 006: LLM usage tracking
CREATE TABLE IF NOT EXISTS llm_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    operation TEXT NOT NULL,
    caller TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    success INTEGER NOT NULL DEFAULT 1,
    error_message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_llm_usage_timestamp ON llm_usage(timestamp);
CREATE INDEX IF NOT EXISTS idx_llm_usage_caller ON llm_usage(caller);
`

const migration008 = `
-- Migration 008: Add type column to memories (propagated from raw_memories)
-- This enables the web UI to filter memories by type (decision, error, insight, learning, general).
`

// InitSchema initializes the SQLite database schema by creating all tables,
// indexes, and triggers if they don't already exist. It also configures
// important PRAGMA settings for performance and safety.
func InitSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Execute PRAGMA statements
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %q: %w", pragma, err)
		}
	}

	// Execute schema creation statements
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Apply migration 002: Episodic memory tables
	if _, err := db.Exec(migration002); err != nil {
		return fmt.Errorf("failed to apply migration 002: %w", err)
	}

	// Add episode_id column to memories if it doesn't exist (idempotent)
	_, err := db.Exec(`ALTER TABLE memories ADD COLUMN episode_id TEXT REFERENCES episodes(id)`)
	if err != nil {
		// Column already exists — this is expected on subsequent runs
		if !isAlterTableDuplicateColumn(err) {
			return fmt.Errorf("failed to add episode_id column: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_episode ON memories(episode_id)`); err != nil {
		return fmt.Errorf("failed to create idx_memory_episode index: %w", err)
	}

	// Migration 003: Add concepts, files_modified, and event_timeline to episodes
	migration003Columns := []struct {
		column string
		def    string
	}{
		{"concepts", "JSON DEFAULT '[]'"},
		{"files_modified", "JSON DEFAULT '[]'"},
		{"event_timeline", "JSON DEFAULT '[]'"},
	}
	for _, col := range migration003Columns {
		_, err := db.Exec(fmt.Sprintf(`ALTER TABLE episodes ADD COLUMN %s %s`, col.column, col.def))
		if err != nil && !isAlterTableDuplicateColumn(err) {
			return fmt.Errorf("failed to add episodes.%s column: %w", col.column, err)
		}
	}

	// Apply migration 004: Patterns, Abstractions, Project/Session context
	if _, err := db.Exec(migration004); err != nil {
		return fmt.Errorf("failed to apply migration 004: %w", err)
	}

	// Migration 004: Add project and session_id columns to existing tables
	migration004Columns := []struct {
		table  string
		column string
		def    string
	}{
		{"raw_memories", "project", "TEXT"},
		{"raw_memories", "session_id", "TEXT"},
		{"memories", "project", "TEXT"},
		{"memories", "session_id", "TEXT"},
		{"episodes", "project", "TEXT"},
	}
	for _, col := range migration004Columns {
		_, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, col.table, col.column, col.def))
		if err != nil && !isAlterTableDuplicateColumn(err) {
			return fmt.Errorf("failed to add %s.%s column: %w", col.table, col.column, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_project ON memories(project)`); err != nil {
		return fmt.Errorf("failed to create idx_memory_project index: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_session ON memories(session_id)`); err != nil {
		return fmt.Errorf("failed to create idx_memory_session index: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_raw_project ON raw_memories(project)`); err != nil {
		return fmt.Errorf("failed to create idx_raw_project index: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_episode_project ON episodes(project)`); err != nil {
		return fmt.Errorf("failed to create idx_episode_project index: %w", err)
	}

	// Migration 007: Add source column to memories
	_, err = db.Exec(`ALTER TABLE memories ADD COLUMN source TEXT`)
	if err != nil && !isAlterTableDuplicateColumn(err) {
		return fmt.Errorf("failed to add memories.source column: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_source ON memories(source)`); err != nil {
		return fmt.Errorf("failed to create idx_memory_source index: %w", err)
	}
	// Backfill source from raw_memories where possible
	_, _ = db.Exec(`UPDATE memories SET source = (SELECT raw_memories.source FROM raw_memories WHERE raw_memories.id = memories.raw_id) WHERE source IS NULL AND raw_id IS NOT NULL AND raw_id != ''`)

	// Apply migration 005: System metadata
	if _, err := db.Exec(migration005); err != nil {
		return fmt.Errorf("failed to apply migration 005: %w", err)
	}

	// Apply migration 006: LLM usage tracking
	if _, err := db.Exec(migration006); err != nil {
		return fmt.Errorf("failed to apply migration 006: %w", err)
	}

	// Apply migration 008: Add type column to memories
	if _, err := db.Exec(migration008); err != nil {
		return fmt.Errorf("failed to apply migration 008: %w", err)
	}
	_, err = db.Exec(`ALTER TABLE memories ADD COLUMN type TEXT`)
	if err != nil && !isAlterTableDuplicateColumn(err) {
		return fmt.Errorf("failed to add memories.type column: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_type ON memories(type)`); err != nil {
		return fmt.Errorf("failed to create idx_memory_type index: %w", err)
	}
	// Backfill type from raw_memories where possible
	_, _ = db.Exec(`UPDATE memories SET type = (SELECT raw_memories.type FROM raw_memories WHERE raw_memories.id = memories.raw_id) WHERE type IS NULL AND raw_id IS NOT NULL AND raw_id != ''`)

	// Migration 010: MCP tool usage tracking
	_, _ = db.Exec(`
CREATE TABLE IF NOT EXISTS tool_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    tool_name TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    project TEXT NOT NULL DEFAULT '',
    latency_ms INTEGER NOT NULL DEFAULT 0,
    success INTEGER NOT NULL DEFAULT 1,
    error_message TEXT NOT NULL DEFAULT '',
    query_text TEXT NOT NULL DEFAULT '',
    result_count INTEGER NOT NULL DEFAULT 0,
    memory_type TEXT NOT NULL DEFAULT '',
    rating TEXT NOT NULL DEFAULT '',
    response_size INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tool_usage_timestamp ON tool_usage(timestamp);
CREATE INDEX IF NOT EXISTS idx_tool_usage_tool ON tool_usage(tool_name);
`)

	// Migration 009: Add access_snapshot column to retrieval_feedback
	_, err = db.Exec(`ALTER TABLE retrieval_feedback ADD COLUMN access_snapshot JSON`)
	if err != nil && !isAlterTableDuplicateColumn(err) {
		return fmt.Errorf("failed to add retrieval_feedback.access_snapshot column: %w", err)
	}

	// Migration 011: Unique constraint on memories.raw_id to prevent duplicate encoding.
	// Multiple mnemonic processes (daemon + MCP instances) share the same DB; without a
	// DB-level guard, each process can independently encode the same raw memory.
	// Step 1: Delete duplicate encoded memories, keeping the oldest per raw_id.
	_, _ = db.Exec(`
		DELETE FROM memories
		WHERE raw_id IS NOT NULL
		  AND id NOT IN (
		    SELECT id FROM (
		      SELECT id, ROW_NUMBER() OVER (PARTITION BY raw_id ORDER BY created_at ASC) as rn
		      FROM memories
		      WHERE raw_id IS NOT NULL
		    ) ranked
		    WHERE rn = 1
		  )
	`)
	// Step 2: Create unique partial index (allows NULLs for gist/consolidated memories).
	_, _ = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_raw_id_unique ON memories(raw_id) WHERE raw_id IS NOT NULL`)

	// Migration 012: Feedback score and recall suppression for negative feedback auto-suppression.
	_, err = db.Exec(`ALTER TABLE memories ADD COLUMN feedback_score INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !isAlterTableDuplicateColumn(err) {
		return fmt.Errorf("failed to add memories.feedback_score column: %w", err)
	}
	_, err = db.Exec(`ALTER TABLE memories ADD COLUMN recall_suppressed INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !isAlterTableDuplicateColumn(err) {
		return fmt.Errorf("failed to add memories.recall_suppressed column: %w", err)
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_suppressed ON memories(recall_suppressed) WHERE recall_suppressed = 1`)

	// Migration 012b: Runtime watcher exclusions.
	_, _ = db.Exec(`
CREATE TABLE IF NOT EXISTS runtime_exclusions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL DEFAULT 'mcp',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`)

	// Migration 013: Memory amendments audit trail.
	_, _ = db.Exec(`
CREATE TABLE IF NOT EXISTS memory_amendments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id TEXT NOT NULL,
    old_content TEXT NOT NULL,
    old_summary TEXT NOT NULL,
    new_content TEXT NOT NULL,
    new_summary TEXT NOT NULL,
    amended_at TEXT NOT NULL DEFAULT (datetime('now')),
    source TEXT NOT NULL DEFAULT 'mcp'
);
CREATE INDEX IF NOT EXISTS idx_amendments_memory ON memory_amendments(memory_id);
`)

	return nil
}

// isAlterTableDuplicateColumn checks if an ALTER TABLE error is due to column already existing.
func isAlterTableDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}
