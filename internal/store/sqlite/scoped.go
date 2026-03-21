package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	store "github.com/appsprout-dev/mnemonic/internal/store"
)

// SearchByProject searches memories within a specific project using FTS.
func (s *SQLiteStore) SearchByProject(ctx context.Context, project string, query string, limit int) ([]store.Memory, error) {
	if query == "" {
		// No query — return recent memories for this project
		sqlQuery := `SELECT ` + memoryColumns + ` FROM memories WHERE project = ? AND state IN ('active', 'fading') ORDER BY timestamp DESC LIMIT ?`
		rows, err := s.db.QueryContext(ctx, sqlQuery, project, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to search by project: %w", err)
		}
		return scanMemoryRows(rows)
	}

	// FTS search scoped to project
	safeQuery := sanitizeFTSQuery(query)
	if safeQuery == "" {
		return nil, nil
	}

	ftsQuery := `
	SELECT m.id, m.raw_id, m.timestamp, m.content, m.summary, m.concepts, m.embedding,
	       m.salience, m.access_count, m.last_accessed, m.state, m.gist_of, m.episode_id,
	       m.source, m.project, m.session_id, m.created_at, m.updated_at
	FROM memories m
	WHERE m.project = ?
	AND m.rowid IN (SELECT rowid FROM memories_fts WHERE memories_fts MATCH ?)
	LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, ftsQuery, project, safeQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search by project with FTS: %w", err)
	}
	return scanMemoryRows(rows)
}

// ListMemoriesByTimeRange lists memories within a time range.
func (s *SQLiteStore) ListMemoriesByTimeRange(ctx context.Context, from, to time.Time, limit int) ([]store.Memory, error) {
	query := `SELECT ` + memoryColumns + ` FROM memories WHERE timestamp BETWEEN ? AND ? AND state IN ('active', 'fading') ORDER BY timestamp DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query,
		from.Format(time.RFC3339),
		to.Format(time.RFC3339),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories by time range: %w", err)
	}
	return scanMemoryRows(rows)
}

// ListMemoriesBySession returns all memories created during a given session.
func (s *SQLiteStore) ListMemoriesBySession(ctx context.Context, sessionID string) ([]store.Memory, error) {
	query := `SELECT ` + memoryColumns + ` FROM memories WHERE session_id = ? ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories by session: %w", err)
	}
	return scanMemoryRows(rows)
}

// ListSessions returns recent sessions with metadata.
func (s *SQLiteStore) ListSessions(ctx context.Context, since time.Time, limit int) ([]store.SessionSummary, error) {
	query := `
	SELECT session_id, MIN(created_at), MAX(created_at), COUNT(*),
		COALESCE((
			SELECT GROUP_CONCAT(topic, ',')
			FROM (
				SELECT json_extract(je.value, '$.label') AS topic, COUNT(*) AS cnt
				FROM memories m2
				JOIN concept_sets cs ON cs.memory_id = m2.id
				, json_each(cs.topics) je
				WHERE m2.session_id = memories.session_id
				  AND json_extract(je.value, '$.label') IS NOT NULL
				GROUP BY topic
				ORDER BY cnt DESC
				LIMIT 5
			)
		), '') AS top_concepts
	FROM memories
	WHERE session_id IS NOT NULL AND session_id != '' AND created_at >= ?
	GROUP BY session_id
	ORDER BY MAX(created_at) DESC
	LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []store.SessionSummary
	for rows.Next() {
		var ss store.SessionSummary
		var startStr, endStr, conceptsStr string
		if err := rows.Scan(&ss.SessionID, &startStr, &endStr, &ss.MemoryCount, &conceptsStr); err != nil {
			continue
		}
		ss.StartTime, _ = time.Parse(time.RFC3339, startStr)
		ss.EndTime, _ = time.Parse(time.RFC3339, endStr)
		if conceptsStr != "" {
			ss.TopConcepts = strings.Split(conceptsStr, ",")
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// GetSessionMemories returns memories for a specific session, ordered by creation time.
func (s *SQLiteStore) GetSessionMemories(ctx context.Context, sessionID string, limit int) ([]store.Memory, error) {
	query := `SELECT ` + memoryColumns + ` FROM memories WHERE session_id = ? ORDER BY created_at ASC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("getting session memories: %w", err)
	}
	return scanMemoryRows(rows)
}

// --- Runtime exclusions ---

// AddRuntimeExclusion adds a watcher exclusion pattern to the DB.
func (s *SQLiteStore) AddRuntimeExclusion(ctx context.Context, pattern string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO runtime_exclusions (pattern, source) VALUES (?, 'mcp')`, pattern)
	if err != nil {
		return fmt.Errorf("adding runtime exclusion: %w", err)
	}
	return nil
}

// RemoveRuntimeExclusion removes a watcher exclusion pattern from the DB.
func (s *SQLiteStore) RemoveRuntimeExclusion(ctx context.Context, pattern string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM runtime_exclusions WHERE pattern = ?`, pattern)
	if err != nil {
		return fmt.Errorf("removing runtime exclusion: %w", err)
	}
	return nil
}

// ListRuntimeExclusions returns all runtime exclusion patterns.
func (s *SQLiteStore) ListRuntimeExclusions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pattern FROM runtime_exclusions ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("listing runtime exclusions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var patterns []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

// GetProjectSummary returns aggregate stats for a specific project.
func (s *SQLiteStore) GetProjectSummary(ctx context.Context, project string) (map[string]interface{}, error) {
	summary := make(map[string]interface{})

	// Count memories by state
	var active, fading, archived, total int
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN state = 'active' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'fading' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'archived' THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM memories WHERE project = ?`, project,
	).Scan(&active, &fading, &archived, &total)
	if err != nil {
		return nil, fmt.Errorf("failed to get project memory counts: %w", err)
	}

	summary["project"] = project
	summary["total_memories"] = total
	summary["active_memories"] = active
	summary["fading_memories"] = fading
	summary["archived_memories"] = archived

	// Get most recent activity
	var lastActivity string
	err = s.db.QueryRowContext(ctx,
		`SELECT MAX(timestamp) FROM memories WHERE project = ?`, project,
	).Scan(&lastActivity)
	if err == nil && lastActivity != "" {
		summary["last_activity"] = lastActivity
	}

	// Count patterns for this project
	var patternCount int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM patterns WHERE project = ? AND state = 'active'`, project,
	).Scan(&patternCount)
	if err == nil {
		summary["active_patterns"] = patternCount
	}

	// Get top concepts
	rows, err := s.db.QueryContext(ctx,
		`SELECT concepts FROM memories WHERE project = ? AND state = 'active' ORDER BY salience DESC LIMIT 20`, project)
	if err == nil {
		defer func() { _ = rows.Close() }()
		conceptCounts := make(map[string]int)
		for rows.Next() {
			var conceptsStr string
			if err := rows.Scan(&conceptsStr); err != nil {
				continue
			}
			concepts, _ := decodeStringSlice(conceptsStr)
			for _, c := range concepts {
				conceptCounts[c]++
			}
		}
		// Get top 10 concepts
		var topConcepts []string
		for concept := range conceptCounts {
			topConcepts = append(topConcepts, concept)
		}
		if len(topConcepts) > 10 {
			topConcepts = topConcepts[:10]
		}
		summary["top_concepts"] = topConcepts
	}

	return summary, nil
}

// ListProjects returns all distinct project names.
func (s *SQLiteStore) ListProjects(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT project FROM memories WHERE project IS NOT NULL AND project != '' ORDER BY project`)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projects []string
	for rows.Next() {
		var project string
		if err := rows.Scan(&project); err != nil {
			continue
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading project rows: %w", err)
	}

	return projects, nil
}
