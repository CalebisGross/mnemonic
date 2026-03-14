package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	store "github.com/appsprout-dev/mnemonic/internal/store"
)

// --- Memory Resolution operations ---

func (s *SQLiteStore) WriteMemoryResolution(ctx context.Context, res store.MemoryResolution) error {
	detailIDs, _ := encodeStringSlice(res.DetailRawIDs)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO memory_resolutions (memory_id, gist, narrative, detail_raw_ids, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		res.MemoryID, res.Gist, res.Narrative, detailIDs,
		res.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to write memory resolution: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetMemoryResolution(ctx context.Context, memoryID string) (store.MemoryResolution, error) {
	var res store.MemoryResolution
	var detailIDsStr string
	var createdStr string
	var gist, narrative sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT memory_id, gist, narrative, detail_raw_ids, created_at
		FROM memory_resolutions WHERE memory_id = ?`, memoryID,
	).Scan(&res.MemoryID, &gist, &narrative, &detailIDsStr, &createdStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return res, fmt.Errorf("memory resolution for %s: %w", memoryID, store.ErrNotFound)
		}
		return res, fmt.Errorf("failed to get memory resolution: %w", err)
	}

	res.Gist = gist.String
	res.Narrative = narrative.String
	res.DetailRawIDs, _ = decodeStringSlice(detailIDsStr)
	res.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return res, nil
}

// --- Concept Set operations ---

func (s *SQLiteStore) WriteConceptSet(ctx context.Context, cs store.ConceptSet) error {
	topicsJSON, _ := json.Marshal(cs.Topics)
	entitiesJSON, _ := json.Marshal(cs.Entities)
	actionsJSON, _ := json.Marshal(cs.Actions)
	causalityJSON, _ := json.Marshal(cs.Causality)

	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO concept_sets (memory_id, topics, entities, actions, causality, significance, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cs.MemoryID,
		string(topicsJSON),
		string(entitiesJSON),
		string(actionsJSON),
		string(causalityJSON),
		cs.Significance,
		cs.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to write concept set: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetConceptSet(ctx context.Context, memoryID string) (store.ConceptSet, error) {
	var cs store.ConceptSet
	var topicsStr, entitiesStr, actionsStr, causalityStr string
	var significance sql.NullString
	var createdStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT memory_id, topics, entities, actions, causality, significance, created_at
		FROM concept_sets WHERE memory_id = ?`, memoryID,
	).Scan(&cs.MemoryID, &topicsStr, &entitiesStr, &actionsStr, &causalityStr, &significance, &createdStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return cs, fmt.Errorf("concept set for %s: %w", memoryID, store.ErrNotFound)
		}
		return cs, fmt.Errorf("failed to get concept set: %w", err)
	}

	_ = json.Unmarshal([]byte(topicsStr), &cs.Topics)
	_ = json.Unmarshal([]byte(entitiesStr), &cs.Entities)
	_ = json.Unmarshal([]byte(actionsStr), &cs.Actions)
	_ = json.Unmarshal([]byte(causalityStr), &cs.Causality)
	cs.Significance = significance.String
	cs.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

	return cs, nil
}

// SearchByEntity finds memories that reference a specific entity.
func (s *SQLiteStore) SearchByEntity(ctx context.Context, name string, entityType string, limit int) ([]store.Memory, error) {
	// Search in concept_sets.entities JSON for matching entity name and type
	// Use JSON path matching with LIKE for SQLite compatibility
	query := `SELECT m.id, m.raw_id, m.timestamp, m.content, m.summary, m.concepts,
		m.embedding, m.salience, m.access_count, m.last_accessed, m.state,
		m.gist_of, m.episode_id, m.project, m.session_id, m.created_at, m.updated_at
		FROM memories m
		JOIN concept_sets cs ON cs.memory_id = m.id
		WHERE cs.entities LIKE ?`
	args := []interface{}{"%" + name + "%"}

	if entityType != "" {
		query += ` AND cs.entities LIKE ?`
		args = append(args, `%"type":"`+entityType+`"%`)
	}

	query += ` ORDER BY m.salience DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search by entity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var memories []store.Memory
	for rows.Next() {
		mem, err := scanMemoryWithEpisode(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// scanMemoryWithEpisode scans a memory row that includes the episode_id column.
func scanMemoryWithEpisode(rows *sql.Rows) (store.Memory, error) {
	var mem store.Memory
	var timestampStr, createdStr, updatedStr string
	var lastAccessedStr sql.NullString
	var conceptsStr, gistOfStr string
	var embeddingBlob []byte
	var episodeID sql.NullString

	err := rows.Scan(
		&mem.ID, &mem.RawID, &timestampStr, &mem.Content, &mem.Summary,
		&conceptsStr, &embeddingBlob, &mem.Salience, &mem.AccessCount,
		&lastAccessedStr, &mem.State, &gistOfStr, &episodeID,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return mem, fmt.Errorf("failed to scan memory: %w", err)
	}

	mem.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
	mem.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	mem.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if lastAccessedStr.Valid {
		mem.LastAccessed, _ = time.Parse(time.RFC3339, lastAccessedStr.String)
	}
	mem.Concepts, _ = decodeStringSlice(conceptsStr)
	mem.GistOf, _ = decodeStringSlice(gistOfStr)
	mem.Embedding = decodeEmbedding(embeddingBlob)
	mem.EpisodeID = episodeID.String

	return mem, nil
}

// --- Memory Attributes operations ---

func (s *SQLiteStore) WriteMemoryAttributes(ctx context.Context, attrs store.MemoryAttributes) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO memory_attributes (memory_id, significance, emotional_tone, outcome, causality_notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		attrs.MemoryID,
		attrs.Significance,
		attrs.EmotionalTone,
		attrs.Outcome,
		attrs.CausalityNotes,
		attrs.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to write memory attributes: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetMemoryAttributes(ctx context.Context, memoryID string) (store.MemoryAttributes, error) {
	var attrs store.MemoryAttributes
	var significance, emotionalTone, outcome, causalityNotes sql.NullString
	var createdStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT memory_id, significance, emotional_tone, outcome, causality_notes, created_at
		FROM memory_attributes WHERE memory_id = ?`, memoryID,
	).Scan(&attrs.MemoryID, &significance, &emotionalTone, &outcome, &causalityNotes, &createdStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return attrs, fmt.Errorf("memory attributes for %s: %w", memoryID, store.ErrNotFound)
		}
		return attrs, fmt.Errorf("failed to get memory attributes: %w", err)
	}

	attrs.Significance = significance.String
	attrs.EmotionalTone = emotionalTone.String
	attrs.Outcome = outcome.String
	attrs.CausalityNotes = causalityNotes.String
	attrs.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

	return attrs, nil
}
