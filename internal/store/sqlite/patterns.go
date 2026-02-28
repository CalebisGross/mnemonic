package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	store "github.com/appsprout/mnemonic/internal/store"
)

// patternColumns is the standard column list for pattern queries.
const patternColumns = `id, pattern_type, title, description, evidence_ids, strength, project, concepts, embedding, access_count, last_accessed, state, created_at, updated_at`

// WritePattern inserts a new pattern.
func (s *SQLiteStore) WritePattern(ctx context.Context, p store.Pattern) error {
	evidenceIDs, _ := encodeStringSlice(p.EvidenceIDs)
	concepts, _ := encodeStringSlice(p.Concepts)
	var embeddingBlob []byte
	if len(p.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(p.Embedding)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO patterns (`+patternColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID,
		p.PatternType,
		p.Title,
		p.Description,
		evidenceIDs,
		p.Strength,
		nullableString(p.Project),
		concepts,
		embeddingBlob,
		p.AccessCount,
		nullableTime(p.LastAccessed),
		p.State,
		p.CreatedAt.Format(time.RFC3339),
		p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to write pattern: %w", err)
	}
	return nil
}

// GetPattern retrieves a pattern by ID.
func (s *SQLiteStore) GetPattern(ctx context.Context, id string) (store.Pattern, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+patternColumns+` FROM patterns WHERE id = ?`, id)
	return scanPattern(row)
}

// UpdatePattern updates an existing pattern.
func (s *SQLiteStore) UpdatePattern(ctx context.Context, p store.Pattern) error {
	evidenceIDs, _ := encodeStringSlice(p.EvidenceIDs)
	concepts, _ := encodeStringSlice(p.Concepts)
	var embeddingBlob []byte
	if len(p.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(p.Embedding)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE patterns
		SET pattern_type = ?, title = ?, description = ?, evidence_ids = ?, strength = ?,
		    project = ?, concepts = ?, embedding = ?, access_count = ?, last_accessed = ?,
		    state = ?, updated_at = ?
		WHERE id = ?`,
		p.PatternType,
		p.Title,
		p.Description,
		evidenceIDs,
		p.Strength,
		nullableString(p.Project),
		concepts,
		embeddingBlob,
		p.AccessCount,
		nullableTime(p.LastAccessed),
		p.State,
		p.UpdatedAt.Format(time.RFC3339),
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update pattern: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("pattern with id %s not found", p.ID)
	}
	return nil
}

// ListPatterns lists patterns, optionally filtered by project.
func (s *SQLiteStore) ListPatterns(ctx context.Context, project string, limit int) ([]store.Pattern, error) {
	var query string
	var args []interface{}

	if project == "" {
		query = `SELECT ` + patternColumns + ` FROM patterns WHERE state = 'active' ORDER BY strength DESC LIMIT ?`
		args = []interface{}{limit}
	} else {
		query = `SELECT ` + patternColumns + ` FROM patterns WHERE state = 'active' AND project = ? ORDER BY strength DESC LIMIT ?`
		args = []interface{}{project, limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list patterns: %w", err)
	}
	return scanPatternRows(rows)
}

// SearchPatternsByEmbedding searches patterns using embedding similarity.
func (s *SQLiteStore) SearchPatternsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.Pattern, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding cannot be empty")
	}

	// Load all active pattern embeddings and do in-memory similarity search
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, embedding FROM patterns WHERE state = 'active' AND embedding IS NOT NULL AND length(embedding) > 0`)
	if err != nil {
		return nil, fmt.Errorf("failed to query pattern embeddings: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id    string
		score float32
	}
	var candidates []candidate

	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			continue
		}
		emb := decodeEmbedding(blob)
		if len(emb) == 0 {
			continue
		}
		score := cosineSimilarity(embedding, emb)
		candidates = append(candidates, candidate{id: id, score: score})
	}

	// Sort by score descending and take top-limit
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	// Fetch full patterns
	var patterns []store.Pattern
	for _, c := range candidates {
		p, err := s.GetPattern(ctx, c.id)
		if err != nil {
			continue
		}
		patterns = append(patterns, p)
	}

	return patterns, nil
}

// nullableTime converts a zero time to nil for SQL NULL.
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339)
}

// scanPattern scans a single pattern row.
func scanPattern(row *sql.Row) (store.Pattern, error) {
	var p store.Pattern
	var evidenceIDsStr, conceptsStr sql.NullString
	var embeddingBlob []byte
	var project, lastAccessedStr sql.NullString

	err := row.Scan(
		&p.ID,
		&p.PatternType,
		&p.Title,
		&p.Description,
		&evidenceIDsStr,
		&p.Strength,
		&project,
		&conceptsStr,
		&embeddingBlob,
		&p.AccessCount,
		&lastAccessedStr,
		&p.State,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return p, fmt.Errorf("pattern not found")
		}
		return p, fmt.Errorf("failed to scan pattern: %w", err)
	}

	p.Project = project.String
	p.EvidenceIDs, _ = decodeStringSlice(evidenceIDsStr.String)
	p.Concepts, _ = decodeStringSlice(conceptsStr.String)
	if len(embeddingBlob) > 0 {
		p.Embedding = decodeEmbedding(embeddingBlob)
	}
	if lastAccessedStr.Valid && lastAccessedStr.String != "" {
		p.LastAccessed, _ = time.Parse(time.RFC3339, lastAccessedStr.String)
	}

	return p, nil
}

// scanPatternRows scans multiple pattern rows.
func scanPatternRows(rows *sql.Rows) ([]store.Pattern, error) {
	defer rows.Close()
	var patterns []store.Pattern

	for rows.Next() {
		var p store.Pattern
		var evidenceIDsStr, conceptsStr sql.NullString
		var embeddingBlob []byte
		var project, lastAccessedStr sql.NullString

		err := rows.Scan(
			&p.ID,
			&p.PatternType,
			&p.Title,
			&p.Description,
			&evidenceIDsStr,
			&p.Strength,
			&project,
			&conceptsStr,
			&embeddingBlob,
			&p.AccessCount,
			&lastAccessedStr,
			&p.State,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pattern row: %w", err)
		}

		p.Project = project.String
		p.EvidenceIDs, _ = decodeStringSlice(evidenceIDsStr.String)
		p.Concepts, _ = decodeStringSlice(conceptsStr.String)
		if len(embeddingBlob) > 0 {
			p.Embedding = decodeEmbedding(embeddingBlob)
		}
		if lastAccessedStr.Valid && lastAccessedStr.String != "" {
			p.LastAccessed, _ = time.Parse(time.RFC3339, lastAccessedStr.String)
		}

		patterns = append(patterns, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading pattern rows: %w", err)
	}

	return patterns, nil
}

// cosineSimilarity and sqrt32 are defined in embindex.go
