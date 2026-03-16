package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	store "github.com/appsprout-dev/mnemonic/internal/store"
)

// abstractionColumns is the standard column list for abstraction queries.
const abstractionColumns = `id, level, title, description, parent_id, source_pattern_ids, source_memory_ids, confidence, concepts, embedding, access_count, state, created_at, updated_at`

// WriteAbstraction inserts a new abstraction.
func (s *SQLiteStore) WriteAbstraction(ctx context.Context, a store.Abstraction) error {
	sourcePatternIDs, _ := encodeStringSlice(a.SourcePatternIDs)
	sourceMemoryIDs, _ := encodeStringSlice(a.SourceMemoryIDs)
	concepts, _ := encodeStringSlice(a.Concepts)
	var embeddingBlob []byte
	if len(a.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(a.Embedding)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO abstractions (`+abstractionColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID,
		a.Level,
		a.Title,
		a.Description,
		nullableString(a.ParentID),
		sourcePatternIDs,
		sourceMemoryIDs,
		a.Confidence,
		concepts,
		embeddingBlob,
		a.AccessCount,
		a.State,
		a.CreatedAt.Format(time.RFC3339),
		a.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to write abstraction: %w", err)
	}
	return nil
}

// GetAbstraction retrieves an abstraction by ID.
func (s *SQLiteStore) GetAbstraction(ctx context.Context, id string) (store.Abstraction, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+abstractionColumns+` FROM abstractions WHERE id = ?`, id)
	return scanAbstraction(row)
}

// UpdateAbstraction updates an existing abstraction.
func (s *SQLiteStore) UpdateAbstraction(ctx context.Context, a store.Abstraction) error {
	sourcePatternIDs, _ := encodeStringSlice(a.SourcePatternIDs)
	sourceMemoryIDs, _ := encodeStringSlice(a.SourceMemoryIDs)
	concepts, _ := encodeStringSlice(a.Concepts)
	var embeddingBlob []byte
	if len(a.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(a.Embedding)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE abstractions
		SET level = ?, title = ?, description = ?, parent_id = ?,
		    source_pattern_ids = ?, source_memory_ids = ?, confidence = ?,
		    concepts = ?, embedding = ?, access_count = ?, state = ?, updated_at = ?
		WHERE id = ?`,
		a.Level,
		a.Title,
		a.Description,
		nullableString(a.ParentID),
		sourcePatternIDs,
		sourceMemoryIDs,
		a.Confidence,
		concepts,
		embeddingBlob,
		a.AccessCount,
		a.State,
		a.UpdatedAt.Format(time.RFC3339),
		a.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update abstraction: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("abstraction with id %s: %w", a.ID, store.ErrNotFound)
	}
	return nil
}

// ListAbstractions lists abstractions, optionally filtered by level.
// Pass level=0 to list all levels.
func (s *SQLiteStore) ListAbstractions(ctx context.Context, level int, limit int) ([]store.Abstraction, error) {
	var query string
	var args []interface{}

	if level == 0 {
		query = `SELECT ` + abstractionColumns + ` FROM abstractions WHERE state = 'active' ORDER BY confidence DESC LIMIT ?`
		args = []interface{}{limit}
	} else {
		query = `SELECT ` + abstractionColumns + ` FROM abstractions WHERE state = 'active' AND level = ? ORDER BY confidence DESC LIMIT ?`
		args = []interface{}{level, limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list abstractions: %w", err)
	}
	return scanAbstractionRows(rows)
}

// ListAbstractionsByState lists abstractions filtered by state (e.g. "active", "fading", "archived").
// Pass level=0 equivalent: returns all levels for the given state.
func (s *SQLiteStore) ListAbstractionsByState(ctx context.Context, state string, limit int) ([]store.Abstraction, error) {
	query := `SELECT ` + abstractionColumns + ` FROM abstractions WHERE state = ? ORDER BY confidence DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, state, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list abstractions by state: %w", err)
	}
	return scanAbstractionRows(rows)
}

// SearchAbstractionsByEmbedding finds active abstractions most similar to the given embedding.
func (s *SQLiteStore) SearchAbstractionsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.Abstraction, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("empty embedding")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, embedding FROM abstractions WHERE state = 'active' AND embedding IS NOT NULL AND length(embedding) > 0`)
	if err != nil {
		return nil, fmt.Errorf("failed to query abstraction embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var abstractions []store.Abstraction
	for _, c := range candidates {
		a, err := s.GetAbstraction(ctx, c.id)
		if err != nil {
			continue
		}
		abstractions = append(abstractions, a)
	}

	return abstractions, nil
}

// scanAbstractionFrom scans a single Abstraction from any scanner.
func scanAbstractionFrom(s scanner) (store.Abstraction, error) {
	var a store.Abstraction
	var parentID, sourcePatternIDsStr, sourceMemoryIDsStr, conceptsStr sql.NullString
	var embeddingBlob []byte

	err := s.Scan(
		&a.ID,
		&a.Level,
		&a.Title,
		&a.Description,
		&parentID,
		&sourcePatternIDsStr,
		&sourceMemoryIDsStr,
		&a.Confidence,
		&conceptsStr,
		&embeddingBlob,
		&a.AccessCount,
		&a.State,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return a, err
	}

	a.ParentID = parentID.String
	a.SourcePatternIDs, _ = decodeStringSlice(sourcePatternIDsStr.String)
	a.SourceMemoryIDs, _ = decodeStringSlice(sourceMemoryIDsStr.String)
	a.Concepts, _ = decodeStringSlice(conceptsStr.String)
	if len(embeddingBlob) > 0 {
		a.Embedding = decodeEmbedding(embeddingBlob)
	}

	return a, nil
}

// scanAbstraction scans a single abstraction row.
func scanAbstraction(row *sql.Row) (store.Abstraction, error) {
	a, err := scanAbstractionFrom(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return a, fmt.Errorf("abstraction: %w", store.ErrNotFound)
		}
		return a, fmt.Errorf("failed to scan abstraction: %w", err)
	}
	return a, nil
}

// scanAbstractionRows scans multiple abstraction rows.
func scanAbstractionRows(rows *sql.Rows) ([]store.Abstraction, error) {
	defer func() { _ = rows.Close() }()
	var abstractions []store.Abstraction

	for rows.Next() {
		a, err := scanAbstractionFrom(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan abstraction row: %w", err)
		}
		abstractions = append(abstractions, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading abstraction rows: %w", err)
	}

	return abstractions, nil
}

// ArchiveAllAbstractions transitions all active abstractions to archived state.
func (s *SQLiteStore) ArchiveAllAbstractions(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE abstractions SET state = 'archived', updated_at = datetime('now') WHERE state = 'active'`)
	if err != nil {
		return 0, fmt.Errorf("archiving abstractions: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
