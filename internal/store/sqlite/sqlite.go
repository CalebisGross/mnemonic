package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	store "github.com/appsprout/mnemonic/internal/store"
)

// SQLiteStore implements the Store interface using SQLite as the backend.
type SQLiteStore struct {
	db       *sql.DB
	dbPath   string
	embIndex *embeddingIndex // in-memory embedding cache for fast similarity search
}

// NewSQLiteStore opens a SQLite database and initializes the schema.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &SQLiteStore{db: db, dbPath: dbPath, embIndex: newEmbeddingIndex()}

	// Initialize the schema
	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Populate the in-memory embedding index from existing data
	if err := s.loadEmbeddingIndex(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load embedding index: %w", err)
	}

	return s, nil
}

// loadEmbeddingIndex reads all (id, embedding) pairs for active/fading memories
// and populates the in-memory index. Only loads the two columns needed, not full rows.
func (s *SQLiteStore) loadEmbeddingIndex() error {
	rows, err := s.db.Query(
		`SELECT id, embedding FROM memories WHERE state IN ('active', 'fading') AND embedding IS NOT NULL AND length(embedding) > 0`)
	if err != nil {
		return fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return fmt.Errorf("failed to scan embedding row: %w", err)
		}
		emb := decodeEmbedding(blob)
		if len(emb) > 0 {
			s.embIndex.Add(id, emb)
		}
	}
	return rows.Err()
}

// Helper functions for encoding/decoding

// encodeEmbedding converts a float32 slice to a binary blob using LittleEndian.
func encodeEmbedding(embedding []float32) []byte {
	if len(embedding) == 0 {
		return nil
	}
	buf := make([]byte, len(embedding)*4)
	for i, v := range embedding {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// decodeEmbedding converts a binary blob back to a float32 slice.
func decodeEmbedding(data []byte) []float32 {
	if len(data) == 0 {
		return nil
	}
	if len(data)%4 != 0 {
		return nil
	}
	embedding := make([]float32, len(data)/4)
	for i := 0; i < len(embedding); i++ {
		embedding[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return embedding
}

// Helper to encode string slices as JSON
func encodeStringSlice(slice []string) (string, error) {
	if len(slice) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(slice)
	if err != nil {
		return "", fmt.Errorf("failed to marshal string slice: %w", err)
	}
	return string(data), nil
}

// Helper to decode JSON string slices
func decodeStringSlice(jsonStr string) ([]string, error) {
	if jsonStr == "" || jsonStr == "[]" {
		return []string{}, nil
	}
	var slice []string
	err := json.Unmarshal([]byte(jsonStr), &slice)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal string slice: %w", err)
	}
	return slice, nil
}

// Helper to encode map as JSON
func encodeMap(m map[string]interface{}) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("failed to marshal map: %w", err)
	}
	return string(data), nil
}

// Helper to decode JSON map
func decodeMap(jsonStr string) (map[string]interface{}, error) {
	if jsonStr == "" || jsonStr == "{}" {
		return make(map[string]interface{}), nil
	}
	var m map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &m)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal map: %w", err)
	}
	return m, nil
}

// rawMemoryColumns is the standard column list for raw memory queries.
const rawMemoryColumns = `id, timestamp, source, type, content, metadata, heuristic_score, initial_salience, processed, project, session_id, created_at`

// scanRawMemory scans a raw memory row from the database.
func scanRawMemory(row *sql.Row) (store.RawMemory, error) {
	var raw store.RawMemory
	var metadataStr sql.NullString
	var processedVal interface{}
	var project, sessionID sql.NullString

	err := row.Scan(
		&raw.ID,
		&raw.Timestamp,
		&raw.Source,
		&raw.Type,
		&raw.Content,
		&metadataStr,
		&raw.HeuristicScore,
		&raw.InitialSalience,
		&processedVal,
		&project,
		&sessionID,
		&raw.CreatedAt,
	)
	if err != nil {
		return raw, err
	}

	// Handle boolean stored as int, string, or bool
	switch v := processedVal.(type) {
	case int64:
		raw.Processed = v != 0
	case bool:
		raw.Processed = v
	case string:
		raw.Processed = v == "1" || v == "true"
	default:
		raw.Processed = false
	}

	// Decode project and session_id
	raw.Project = project.String
	raw.SessionID = sessionID.String

	if metadataStr.Valid && metadataStr.String != "" {
		m, err := decodeMap(metadataStr.String)
		if err != nil {
			return raw, err
		}
		raw.Metadata = m
	} else {
		raw.Metadata = make(map[string]interface{})
	}

	// Parse timestamps
	if raw.Timestamp.IsZero() {
		raw.Timestamp, _ = time.Parse(time.RFC3339, raw.Timestamp.Format(time.RFC3339))
	}
	if raw.CreatedAt.IsZero() {
		raw.CreatedAt, _ = time.Parse(time.RFC3339, raw.CreatedAt.Format(time.RFC3339))
	}

	return raw, nil
}

// scanRawMemoryRows scans multiple raw memory rows from rows.
func scanRawMemoryRows(rows *sql.Rows) ([]store.RawMemory, error) {
	defer rows.Close()
	var rawMemories []store.RawMemory

	for rows.Next() {
		var raw store.RawMemory
		var metadataStr sql.NullString
		var processedVal interface{}
		var project, sessionID sql.NullString

		err := rows.Scan(
			&raw.ID,
			&raw.Timestamp,
			&raw.Source,
			&raw.Type,
			&raw.Content,
			&metadataStr,
			&raw.HeuristicScore,
			&raw.InitialSalience,
			&processedVal,
			&project,
			&sessionID,
			&raw.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan raw memory row: %w", err)
		}

		switch v := processedVal.(type) {
		case int64:
			raw.Processed = v != 0
		case bool:
			raw.Processed = v
		case string:
			raw.Processed = v == "1" || v == "true"
		default:
			raw.Processed = false
		}

		raw.Project = project.String
		raw.SessionID = sessionID.String

		if metadataStr.Valid && metadataStr.String != "" {
			m, err := decodeMap(metadataStr.String)
			if err != nil {
				return nil, err
			}
			raw.Metadata = m
		} else {
			raw.Metadata = make(map[string]interface{})
		}

		rawMemories = append(rawMemories, raw)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return rawMemories, nil
}

// memoryColumns is the standard column list for memory queries.
const memoryColumns = `id, raw_id, timestamp, content, summary, concepts, embedding, salience, access_count, last_accessed, state, gist_of, episode_id, project, session_id, created_at, updated_at`

// scanMemory scans a memory row from the database.
func scanMemory(row *sql.Row) (store.Memory, error) {
	var mem store.Memory
	var conceptsStr sql.NullString
	var embeddingBlob []byte
	var gistOfStr sql.NullString
	var lastAccessedStr sql.NullString
	var episodeID sql.NullString
	var project, sessionID sql.NullString

	err := row.Scan(
		&mem.ID,
		&mem.RawID,
		&mem.Timestamp,
		&mem.Content,
		&mem.Summary,
		&conceptsStr,
		&embeddingBlob,
		&mem.Salience,
		&mem.AccessCount,
		&lastAccessedStr,
		&mem.State,
		&gistOfStr,
		&episodeID,
		&project,
		&sessionID,
		&mem.CreatedAt,
		&mem.UpdatedAt,
	)
	if err != nil {
		return mem, err
	}

	// Decode concepts
	if conceptsStr.Valid && conceptsStr.String != "" {
		concepts, err := decodeStringSlice(conceptsStr.String)
		if err != nil {
			return mem, err
		}
		mem.Concepts = concepts
	} else {
		mem.Concepts = []string{}
	}

	// Decode embedding
	if len(embeddingBlob) > 0 {
		mem.Embedding = decodeEmbedding(embeddingBlob)
	} else {
		mem.Embedding = []float32{}
	}

	// Decode gist_of
	if gistOfStr.Valid && gistOfStr.String != "" {
		gistOf, err := decodeStringSlice(gistOfStr.String)
		if err != nil {
			return mem, err
		}
		mem.GistOf = gistOf
	} else {
		mem.GistOf = []string{}
	}

	// Decode episode_id
	mem.EpisodeID = episodeID.String

	// Decode project and session_id
	mem.Project = project.String
	mem.SessionID = sessionID.String

	// Parse last_accessed
	if lastAccessedStr.Valid && lastAccessedStr.String != "" {
		lastAccessed, err := time.Parse(time.RFC3339, lastAccessedStr.String)
		if err == nil {
			mem.LastAccessed = lastAccessed
		}
	}

	// Parse timestamps
	if !mem.Timestamp.IsZero() {
		mem.Timestamp, _ = time.Parse(time.RFC3339, mem.Timestamp.Format(time.RFC3339))
	}
	if !mem.CreatedAt.IsZero() {
		mem.CreatedAt, _ = time.Parse(time.RFC3339, mem.CreatedAt.Format(time.RFC3339))
	}
	if !mem.UpdatedAt.IsZero() {
		mem.UpdatedAt, _ = time.Parse(time.RFC3339, mem.UpdatedAt.Format(time.RFC3339))
	}

	return mem, nil
}

// scanMemoryRows scans multiple memory rows from rows.
func scanMemoryRows(rows *sql.Rows) ([]store.Memory, error) {
	defer rows.Close()
	var memories []store.Memory

	for rows.Next() {
		var mem store.Memory
		var conceptsStr sql.NullString
		var embeddingBlob []byte
		var gistOfStr sql.NullString
		var lastAccessedStr sql.NullString
		var episodeID sql.NullString
		var project, sessionID sql.NullString

		err := rows.Scan(
			&mem.ID,
			&mem.RawID,
			&mem.Timestamp,
			&mem.Content,
			&mem.Summary,
			&conceptsStr,
			&embeddingBlob,
			&mem.Salience,
			&mem.AccessCount,
			&lastAccessedStr,
			&mem.State,
			&gistOfStr,
			&episodeID,
			&project,
			&sessionID,
			&mem.CreatedAt,
			&mem.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory row: %w", err)
		}

		// Decode concepts
		if conceptsStr.Valid && conceptsStr.String != "" {
			concepts, err := decodeStringSlice(conceptsStr.String)
			if err != nil {
				return nil, err
			}
			mem.Concepts = concepts
		} else {
			mem.Concepts = []string{}
		}

		// Decode embedding
		if len(embeddingBlob) > 0 {
			mem.Embedding = decodeEmbedding(embeddingBlob)
		} else {
			mem.Embedding = []float32{}
		}

		// Decode gist_of
		if gistOfStr.Valid && gistOfStr.String != "" {
			gistOf, err := decodeStringSlice(gistOfStr.String)
			if err != nil {
				return nil, err
			}
			mem.GistOf = gistOf
		} else {
			mem.GistOf = []string{}
		}

		// Decode episode_id
		mem.EpisodeID = episodeID.String

		// Decode project and session_id
		mem.Project = project.String
		mem.SessionID = sessionID.String

		// Parse last_accessed
		if lastAccessedStr.Valid && lastAccessedStr.String != "" {
			lastAccessed, err := time.Parse(time.RFC3339, lastAccessedStr.String)
			if err == nil {
				mem.LastAccessed = lastAccessed
			}
		}

		memories = append(memories, mem)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return memories, nil
}

// Raw Memory Operations

// WriteRaw writes a raw memory to the database.
func (s *SQLiteStore) WriteRaw(ctx context.Context, raw store.RawMemory) error {
	metadataStr, err := encodeMap(raw.Metadata)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO raw_memories
	(id, timestamp, source, type, content, metadata, heuristic_score, initial_salience, processed, project, session_id, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		raw.ID,
		raw.Timestamp.Format(time.RFC3339),
		raw.Source,
		raw.Type,
		raw.Content,
		metadataStr,
		raw.HeuristicScore,
		raw.InitialSalience,
		boolToInt(raw.Processed),
		nullableString(raw.Project),
		nullableString(raw.SessionID),
		raw.CreatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to write raw memory: %w", err)
	}

	return nil
}

// GetRaw retrieves a raw memory by ID.
func (s *SQLiteStore) GetRaw(ctx context.Context, id string) (store.RawMemory, error) {
	query := `SELECT ` + rawMemoryColumns + ` FROM raw_memories WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanRawMemory(row)
}

// ListRawUnprocessed lists raw memories that haven't been processed yet.
func (s *SQLiteStore) ListRawUnprocessed(ctx context.Context, limit int) ([]store.RawMemory, error) {
	query := `SELECT ` + rawMemoryColumns + ` FROM raw_memories WHERE processed = 0 OR processed = 'false' ORDER BY created_at ASC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query raw memories: %w", err)
	}
	return scanRawMemoryRows(rows)
}

// ListRawMemoriesAfter lists all raw memories created after a given time, regardless of processed flag.
// This is used by the episoding agent to find raw memories that need episode assignment.
func (s *SQLiteStore) ListRawMemoriesAfter(ctx context.Context, after time.Time, limit int) ([]store.RawMemory, error) {
	query := `SELECT ` + rawMemoryColumns + ` FROM raw_memories WHERE timestamp > ? ORDER BY timestamp ASC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, after.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query raw memories after %v: %w", after, err)
	}
	return scanRawMemoryRows(rows)
}

// MarkRawProcessed marks a raw memory as processed.
func (s *SQLiteStore) MarkRawProcessed(ctx context.Context, id string) error {
	query := `UPDATE raw_memories SET processed = 1 WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark raw memory as processed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("raw memory with id %s not found", id)
	}

	return nil
}

// Memory Operations

// WriteMemory writes a memory to the database.
func (s *SQLiteStore) WriteMemory(ctx context.Context, mem store.Memory) error {
	conceptsStr, err := encodeStringSlice(mem.Concepts)
	if err != nil {
		return err
	}

	gistOfStr, err := encodeStringSlice(mem.GistOf)
	if err != nil {
		return err
	}

	var embeddingBlob []byte
	if len(mem.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(mem.Embedding)
	}

	query := `
	INSERT INTO memories
	(id, raw_id, timestamp, content, summary, concepts, embedding, salience, access_count, last_accessed, state, gist_of, episode_id, project, session_id, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Convert empty episode ID to nil so FK constraint allows NULL
	var episodeID interface{}
	if mem.EpisodeID != "" {
		episodeID = mem.EpisodeID
	}

	_, err = s.db.ExecContext(ctx, query,
		mem.ID,
		mem.RawID,
		mem.Timestamp.Format(time.RFC3339),
		mem.Content,
		mem.Summary,
		conceptsStr,
		embeddingBlob,
		mem.Salience,
		mem.AccessCount,
		mem.LastAccessed.Format(time.RFC3339),
		mem.State,
		gistOfStr,
		episodeID,
		nullableString(mem.Project),
		nullableString(mem.SessionID),
		mem.CreatedAt.Format(time.RFC3339),
		mem.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to write memory: %w", err)
	}

	// Update in-memory embedding index
	if (mem.State == "active" || mem.State == "fading") && len(mem.Embedding) > 0 {
		s.embIndex.Add(mem.ID, mem.Embedding)
	}

	// FTS is automatically synced via triggers defined in schema.go
	return nil
}

// GetMemory retrieves a memory by ID.
func (s *SQLiteStore) GetMemory(ctx context.Context, id string) (store.Memory, error) {
	query := `SELECT ` + memoryColumns + ` FROM memories WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanMemory(row)
}

// GetMemoryByRawID retrieves the encoded memory for a given raw memory ID.
func (s *SQLiteStore) GetMemoryByRawID(ctx context.Context, rawID string) (store.Memory, error) {
	query := `SELECT ` + memoryColumns + ` FROM memories WHERE raw_id = ? LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, rawID)
	return scanMemory(row)
}

// UpdateMemory updates an existing memory.
func (s *SQLiteStore) UpdateMemory(ctx context.Context, mem store.Memory) error {
	conceptsStr, err := encodeStringSlice(mem.Concepts)
	if err != nil {
		return err
	}

	gistOfStr, err := encodeStringSlice(mem.GistOf)
	if err != nil {
		return err
	}

	var embeddingBlob []byte
	if len(mem.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(mem.Embedding)
	}

	query := `
	UPDATE memories
	SET raw_id = ?, timestamp = ?, content = ?, summary = ?, concepts = ?, embedding = ?,
	    salience = ?, access_count = ?, last_accessed = ?, state = ?, gist_of = ?,
	    project = ?, session_id = ?, updated_at = ?
	WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		mem.RawID,
		mem.Timestamp.Format(time.RFC3339),
		mem.Content,
		mem.Summary,
		conceptsStr,
		embeddingBlob,
		mem.Salience,
		mem.AccessCount,
		mem.LastAccessed.Format(time.RFC3339),
		mem.State,
		gistOfStr,
		nullableString(mem.Project),
		nullableString(mem.SessionID),
		mem.UpdatedAt.Format(time.RFC3339),
		mem.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("memory with id %s not found", mem.ID)
	}

	// Update in-memory embedding index
	if (mem.State == "active" || mem.State == "fading") && len(mem.Embedding) > 0 {
		s.embIndex.Add(mem.ID, mem.Embedding)
	} else {
		// State changed away from searchable, or embedding removed
		s.embIndex.Remove(mem.ID)
	}

	// FTS is automatically synced via UPDATE trigger in schema.go
	return nil
}

// UpdateSalience updates the salience of a memory.
func (s *SQLiteStore) UpdateSalience(ctx context.Context, id string, salience float32) error {
	query := `UPDATE memories SET salience = ?, updated_at = ? WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, salience, time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to update salience: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("memory with id %s not found", id)
	}

	return nil
}

// UpdateState updates the state of a memory.
func (s *SQLiteStore) UpdateState(ctx context.Context, id string, state string) error {
	query := `UPDATE memories SET state = ?, updated_at = ? WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, state, time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("memory with id %s not found", id)
	}

	// Remove from embedding index if state moved away from searchable
	if state != "active" && state != "fading" {
		s.embIndex.Remove(id)
	}

	return nil
}

// IncrementAccess increments the access count and updates last_accessed.
func (s *SQLiteStore) IncrementAccess(ctx context.Context, id string) error {
	query := `UPDATE memories SET access_count = access_count + 1, last_accessed = ?, updated_at = ? WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to increment access: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("memory with id %s not found", id)
	}

	return nil
}

// ListMemories lists memories with pagination.
func (s *SQLiteStore) ListMemories(ctx context.Context, state string, limit, offset int) ([]store.Memory, error) {
	var query string
	var args []interface{}

	if state == "" {
		query = `
		SELECT ` + memoryColumns + `
		FROM memories ORDER BY created_at DESC LIMIT ? OFFSET ?
		`
		args = []interface{}{limit, offset}
	} else {
		query = `
		SELECT ` + memoryColumns + `
		FROM memories WHERE state = ? ORDER BY created_at DESC LIMIT ? OFFSET ?
		`
		args = []interface{}{state, limit, offset}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query memories: %w", err)
	}

	return scanMemoryRows(rows)
}

// CountMemories returns the total count of memories.
func (s *SQLiteStore) CountMemories(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM memories`

	var count int
	err := s.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count memories: %w", err)
	}

	return count, nil
}

// Search Operations

// ftsStopWords are common words filtered from FTS queries to reduce noise.
var ftsStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "can": true,
	"this": true, "that": true, "these": true, "those": true,
	"i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true,
	"what": true, "when": true, "where": true, "why": true, "how": true, "which": true, "who": true,
}

// sanitizeFTSQuery converts a raw query string into safe FTS5 syntax.
// Uses prefix matching (word*) for stemming-like behavior and filters stop words.
func sanitizeFTSQuery(query string) string {
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}
	terms := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, "\"'.,!?;:")
		w = strings.ToLower(w)
		if w == "" || len(w) < 2 || ftsStopWords[w] {
			continue
		}
		terms = append(terms, w+"*")
	}
	if len(terms) == 0 {
		return ""
	}
	return strings.Join(terms, " OR ")
}

// SearchByFullText searches for memories using full-text search.
func (s *SQLiteStore) SearchByFullText(ctx context.Context, query string, limit int) ([]store.Memory, error) {
	safeQuery := sanitizeFTSQuery(query)
	if safeQuery == "" {
		return nil, nil
	}

	ftsQuery := `
	SELECT m.id, m.raw_id, m.timestamp, m.content, m.summary, m.concepts, m.embedding,
	       m.salience, m.access_count, m.last_accessed, m.state, m.gist_of, m.episode_id,
	       m.project, m.session_id, m.created_at, m.updated_at
	FROM memories m
	JOIN memories_fts ON m.rowid = memories_fts.rowid
	WHERE memories_fts MATCH ?
	ORDER BY memories_fts.rank
	LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, ftsQuery, safeQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to perform FTS search: %w", err)
	}

	return scanMemoryRows(rows)
}

// SearchByEmbedding searches for memories using embedding similarity.
// Uses an in-memory embedding index for fast cosine similarity search,
// then fetches only the top-K full memory rows from the database.
func (s *SQLiteStore) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]store.RetrievalResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding cannot be empty")
	}

	// Search the in-memory index (no DB I/O, no row decoding)
	matches := s.embIndex.Search(embedding, limit)
	if len(matches) == 0 {
		return []store.RetrievalResult{}, nil
	}

	// Fetch only the matched memories from DB by ID
	results := make([]store.RetrievalResult, 0, len(matches))
	for _, m := range matches {
		mem, err := s.GetMemory(ctx, m.id)
		if err != nil {
			continue // Memory may have been deleted between index search and fetch
		}
		results = append(results, store.RetrievalResult{
			Memory:      mem,
			Score:       m.score,
			Explanation: "Embedding similarity",
		})
	}

	return results, nil
}

// SearchByConcepts searches for memories by concepts.
func (s *SQLiteStore) SearchByConcepts(ctx context.Context, concepts []string, limit int) ([]store.Memory, error) {
	if len(concepts) == 0 {
		return []store.Memory{}, nil
	}

	// Build LIKE conditions for concept matching
	query := `
	SELECT ` + memoryColumns + `
	FROM memories
	WHERE `

	args := make([]interface{}, 0)
	conditions := make([]string, 0)

	for _, concept := range concepts {
		conditions = append(conditions, "concepts LIKE ?")
		args = append(args, "%"+concept+"%")
	}

	// Join conditions with OR
	for i, cond := range conditions {
		query += cond
		if i < len(conditions)-1 {
			query += " OR "
		}
	}

	query += ` ORDER BY salience DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search by concepts: %w", err)
	}

	return scanMemoryRows(rows)
}

// Association Operations

// CreateAssociation creates a new association between two memories.
func (s *SQLiteStore) CreateAssociation(ctx context.Context, assoc store.Association) error {
	query := `
	INSERT INTO associations (source_id, target_id, strength, relation_type, created_at, last_activated, activation_count)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		assoc.SourceID,
		assoc.TargetID,
		assoc.Strength,
		assoc.RelationType,
		assoc.CreatedAt.Format(time.RFC3339),
		assoc.LastActivated.Format(time.RFC3339),
		assoc.ActivationCount,
	)

	if err != nil {
		return fmt.Errorf("failed to create association: %w", err)
	}

	return nil
}

// GetAssociations retrieves all associations for a memory.
func (s *SQLiteStore) GetAssociations(ctx context.Context, memoryID string) ([]store.Association, error) {
	query := `
	SELECT source_id, target_id, strength, relation_type, created_at, last_activated, activation_count
	FROM associations WHERE source_id = ? OR target_id = ?
	`

	rows, err := s.db.QueryContext(ctx, query, memoryID, memoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to query associations: %w", err)
	}
	defer rows.Close()

	var associations []store.Association
	for rows.Next() {
		var assoc store.Association
		var createdAtStr string
		var lastActivatedSql sql.NullString

		err := rows.Scan(
			&assoc.SourceID,
			&assoc.TargetID,
			&assoc.Strength,
			&assoc.RelationType,
			&createdAtStr,
			&lastActivatedSql,
			&assoc.ActivationCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan association row: %w", err)
		}

		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err == nil {
			assoc.CreatedAt = createdAt
		}

		if lastActivatedSql.Valid && lastActivatedSql.String != "" {
			lastActivated, err := time.Parse(time.RFC3339, lastActivatedSql.String)
			if err == nil {
				assoc.LastActivated = lastActivated
			}
		}

		associations = append(associations, assoc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return associations, nil
}

// UpdateAssociationStrength updates the strength of an association.
func (s *SQLiteStore) UpdateAssociationStrength(ctx context.Context, sourceID, targetID string, strength float32) error {
	query := `UPDATE associations SET strength = ? WHERE source_id = ? AND target_id = ?`

	result, err := s.db.ExecContext(ctx, query, strength, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("failed to update association strength: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("association from %s to %s not found", sourceID, targetID)
	}

	return nil
}

// UpdateAssociationType updates the relation type of an existing association.
func (s *SQLiteStore) UpdateAssociationType(ctx context.Context, sourceID, targetID string, relationType string) error {
	query := `UPDATE associations SET relation_type = ? WHERE source_id = ? AND target_id = ?`

	result, err := s.db.ExecContext(ctx, query, relationType, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("failed to update association type: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("association from %s to %s not found", sourceID, targetID)
	}

	return nil
}

// ActivateAssociation activates an association, updating last_activated and incrementing activation_count.
func (s *SQLiteStore) ActivateAssociation(ctx context.Context, sourceID, targetID string) error {
	query := `UPDATE associations SET last_activated = ?, activation_count = activation_count + 1 WHERE source_id = ? AND target_id = ?`

	result, err := s.db.ExecContext(ctx, query, time.Now().Format(time.RFC3339), sourceID, targetID)
	if err != nil {
		return fmt.Errorf("failed to activate association: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("association from %s to %s not found", sourceID, targetID)
	}

	return nil
}

// PruneWeakAssociations deletes associations with strength below threshold.
func (s *SQLiteStore) PruneWeakAssociations(ctx context.Context, strengthThreshold float32) (int, error) {
	query := `DELETE FROM associations WHERE strength < ?`

	result, err := s.db.ExecContext(ctx, query, strengthThreshold)
	if err != nil {
		return 0, fmt.Errorf("failed to prune weak associations: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// Batch Operations

// BatchUpdateSalience updates salience for multiple memories.
func (s *SQLiteStore) BatchUpdateSalience(ctx context.Context, updates map[string]float32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `UPDATE memories SET salience = ?, updated_at = ? WHERE id = ?`
	now := time.Now().Format(time.RFC3339)

	for id, salience := range updates {
		_, err := tx.ExecContext(ctx, query, salience, now, id)
		if err != nil {
			return fmt.Errorf("failed to update salience for id %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BatchMergeMemories merges multiple source memories into a gist memory.
func (s *SQLiteStore) BatchMergeMemories(ctx context.Context, sourceIDs []string, gist store.Memory) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Write the gist memory
	conceptsStr, err := encodeStringSlice(gist.Concepts)
	if err != nil {
		return err
	}

	gistOfStr, err := encodeStringSlice(sourceIDs)
	if err != nil {
		return err
	}

	var embeddingBlob []byte
	if len(gist.Embedding) > 0 {
		embeddingBlob = encodeEmbedding(gist.Embedding)
	}

	writeQuery := `
	INSERT INTO memories
	(id, raw_id, timestamp, content, summary, concepts, embedding, salience, access_count, last_accessed, state, gist_of, episode_id, project, session_id, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Convert empty episode ID to nil so FK constraint allows NULL
	var gistEpisodeID interface{}
	if gist.EpisodeID != "" {
		gistEpisodeID = gist.EpisodeID
	}

	_, err = tx.ExecContext(ctx, writeQuery,
		gist.ID,
		gist.RawID,
		gist.Timestamp.Format(time.RFC3339),
		gist.Content,
		gist.Summary,
		conceptsStr,
		embeddingBlob,
		gist.Salience,
		gist.AccessCount,
		gist.LastAccessed.Format(time.RFC3339),
		gist.State,
		gistOfStr,
		gistEpisodeID,
		nullableString(gist.Project),
		nullableString(gist.SessionID),
		gist.CreatedAt.Format(time.RFC3339),
		gist.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to write gist memory: %w", err)
	}

	// Update source memories to merged state
	updateStateQuery := `UPDATE memories SET state = ?, updated_at = ? WHERE id = ?`
	now := time.Now().Format(time.RFC3339)

	for _, sourceID := range sourceIDs {
		_, err := tx.ExecContext(ctx, updateStateQuery, "merged", now, sourceID)
		if err != nil {
			return fmt.Errorf("failed to update state for source id %s: %w", sourceID, err)
		}
	}

	// Redirect associations from source memories to gist
	// Get all associations from source memories
	getAssocQuery := `SELECT source_id, target_id, strength, relation_type, created_at, last_activated, activation_count
	FROM associations WHERE source_id = ? OR target_id = ?`

	redirectQuery := `INSERT OR IGNORE INTO associations (source_id, target_id, strength, relation_type, created_at, last_activated, activation_count)
	VALUES (?, ?, ?, ?, ?, ?, ?)`

	deleteQuery := `DELETE FROM associations WHERE (source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?)`

	for _, sourceID := range sourceIDs {
		rows, err := tx.QueryContext(ctx, getAssocQuery, sourceID, sourceID)
		if err != nil {
			return fmt.Errorf("failed to query associations for source id %s: %w", sourceID, err)
		}

		assocList := make([]store.Association, 0)
		for rows.Next() {
			var assoc store.Association
			var createdAtStr, lastActivatedStr string

			err := rows.Scan(
				&assoc.SourceID,
				&assoc.TargetID,
				&assoc.Strength,
				&assoc.RelationType,
				&createdAtStr,
				&lastActivatedStr,
				&assoc.ActivationCount,
			)
			if err != nil {
				rows.Close()
				return fmt.Errorf("failed to scan association: %w", err)
			}

			createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
			assoc.CreatedAt = createdAt
			if lastActivatedStr != "" {
				lastActivated, _ := time.Parse(time.RFC3339, lastActivatedStr)
				assoc.LastActivated = lastActivated
			}

			assocList = append(assocList, assoc)
		}
		rows.Close()

		// Redirect each association
		for _, assoc := range assocList {
			newSourceID := assoc.SourceID
			newTargetID := assoc.TargetID

			if assoc.SourceID == sourceID {
				newSourceID = gist.ID
			}
			if assoc.TargetID == sourceID {
				newTargetID = gist.ID
			}

			_, err := tx.ExecContext(ctx, redirectQuery,
				newSourceID,
				newTargetID,
				assoc.Strength,
				assoc.RelationType,
				assoc.CreatedAt.Format(time.RFC3339),
				assoc.LastActivated.Format(time.RFC3339),
				assoc.ActivationCount,
			)
			if err != nil {
				return fmt.Errorf("failed to redirect association: %w", err)
			}

			// Delete old association
			_, err = tx.ExecContext(ctx, deleteQuery, sourceID, assoc.TargetID, assoc.SourceID, sourceID)
			if err != nil {
				return fmt.Errorf("failed to delete old association: %w", err)
			}
		}
	}

	// FTS is automatically synced via INSERT trigger in schema.go

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update embedding index: remove merged sources, add gist
	for _, sourceID := range sourceIDs {
		s.embIndex.Remove(sourceID)
	}
	if (gist.State == "active" || gist.State == "fading") && len(gist.Embedding) > 0 {
		s.embIndex.Add(gist.ID, gist.Embedding)
	}

	return nil
}

// DeleteOldArchived deletes archived memories older than the specified time.
func (s *SQLiteStore) DeleteOldArchived(ctx context.Context, olderThan time.Time) (int, error) {
	query := `DELETE FROM memories WHERE state = 'archived' AND created_at < ?`

	result, err := s.db.ExecContext(ctx, query, olderThan.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to delete old archived memories: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// Consolidation Operations

// WriteConsolidation writes a consolidation record.
func (s *SQLiteStore) WriteConsolidation(ctx context.Context, record store.ConsolidationRecord) error {
	query := `
	INSERT INTO consolidation_history
	(id, start_time, end_time, duration_ms, memories_processed, memories_decayed, merged_clusters, associations_pruned, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		record.ID,
		record.StartTime.Format(time.RFC3339),
		record.EndTime.Format(time.RFC3339),
		record.DurationMs,
		record.MemoriesProcessed,
		record.MemoriesDecayed,
		record.MergedClusters,
		record.AssociationsPruned,
		record.CreatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to write consolidation record: %w", err)
	}

	return nil
}

// GetLastConsolidation retrieves the most recent consolidation record.
func (s *SQLiteStore) GetLastConsolidation(ctx context.Context) (store.ConsolidationRecord, error) {
	var record store.ConsolidationRecord
	var startTimeStr, endTimeStr, createdAtStr string

	query := `
	SELECT id, start_time, end_time, duration_ms, memories_processed, memories_decayed, merged_clusters, associations_pruned, created_at
	FROM consolidation_history ORDER BY created_at DESC LIMIT 1
	`

	err := s.db.QueryRowContext(ctx, query).Scan(
		&record.ID,
		&startTimeStr,
		&endTimeStr,
		&record.DurationMs,
		&record.MemoriesProcessed,
		&record.MemoriesDecayed,
		&record.MergedClusters,
		&record.AssociationsPruned,
		&createdAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return record, fmt.Errorf("no consolidation records found")
		}
		return record, fmt.Errorf("failed to query consolidation record: %w", err)
	}

	startTime, _ := time.Parse(time.RFC3339, startTimeStr)
	record.StartTime = startTime

	endTime, _ := time.Parse(time.RFC3339, endTimeStr)
	record.EndTime = endTime

	createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
	record.CreatedAt = createdAt

	return record, nil
}

// GetStatistics computes and returns store statistics.
func (s *SQLiteStore) GetStatistics(ctx context.Context) (store.StoreStatistics, error) {
	var stats store.StoreStatistics

	// Count memories by state
	countQuery := `
	SELECT
		COALESCE(SUM(CASE WHEN state = 'active' THEN 1 ELSE 0 END), 0) as active,
		COALESCE(SUM(CASE WHEN state = 'fading' THEN 1 ELSE 0 END), 0) as fading,
		COALESCE(SUM(CASE WHEN state = 'archived' THEN 1 ELSE 0 END), 0) as archived,
		COALESCE(SUM(CASE WHEN state = 'merged' THEN 1 ELSE 0 END), 0) as merged,
		COUNT(*) as total
	FROM memories
	`

	err := s.db.QueryRowContext(ctx, countQuery).Scan(
		&stats.ActiveMemories,
		&stats.FadingMemories,
		&stats.ArchivedMemories,
		&stats.MergedMemories,
		&stats.TotalMemories,
	)
	if err != nil {
		return stats, fmt.Errorf("failed to count memories: %w", err)
	}

	// Count episodes
	episodeQuery := `SELECT COUNT(*) FROM episodes`
	err = s.db.QueryRowContext(ctx, episodeQuery).Scan(&stats.TotalEpisodes)
	if err != nil {
		return stats, fmt.Errorf("failed to count episodes: %w", err)
	}

	// Count associations
	assocQuery := `SELECT COUNT(*) FROM associations`
	err = s.db.QueryRowContext(ctx, assocQuery).Scan(&stats.TotalAssociations)
	if err != nil {
		return stats, fmt.Errorf("failed to count associations: %w", err)
	}

	// Compute average associations per memory
	if stats.TotalMemories > 0 {
		stats.AvgAssociationsPerMem = float32(stats.TotalAssociations) / float32(stats.TotalMemories)
	}

	// Get the last consolidation time
	lastConsolidation, err := s.GetLastConsolidation(ctx)
	if err == nil {
		stats.LastConsolidation = lastConsolidation.CreatedAt
	}

	// Get storage size
	var pageCount, pageSize int64
	err = s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == nil {
		err = s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
		if err == nil {
			stats.StorageSizeBytes = pageCount * pageSize
		}
	}

	return stats, nil
}

// ============================================================================
// Export/Backup operations
// ============================================================================

// ListAllAssociations returns all associations in the system.
func (s *SQLiteStore) ListAllAssociations(ctx context.Context) ([]store.Association, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source_id, target_id, strength, relation_type, created_at, last_activated, activation_count
		 FROM associations ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list associations: %w", err)
	}
	defer rows.Close()

	var associations []store.Association
	for rows.Next() {
		var a store.Association
		var createdAt, lastActivated sql.NullString
		if err := rows.Scan(&a.SourceID, &a.TargetID, &a.Strength, &a.RelationType,
			&createdAt, &lastActivated, &a.ActivationCount); err != nil {
			return nil, fmt.Errorf("failed to scan association: %w", err)
		}
		if createdAt.Valid {
			a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt.String)
		}
		if lastActivated.Valid {
			a.LastActivated, _ = time.Parse("2006-01-02 15:04:05", lastActivated.String)
		}
		associations = append(associations, a)
	}
	return associations, rows.Err()
}

// ListAllRawMemories returns all raw memories in the system.
func (s *SQLiteStore) ListAllRawMemories(ctx context.Context) ([]store.RawMemory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rawMemoryColumns+` FROM raw_memories ORDER BY timestamp DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list raw memories: %w", err)
	}
	return scanRawMemoryRows(rows)
}

// ============================================================================
// Metacognition operations
// ============================================================================

// WriteMetaObservation stores a meta-observation.
func (s *SQLiteStore) WriteMetaObservation(ctx context.Context, obs store.MetaObservation) error {
	detailsJSON, err := json.Marshal(obs.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal details: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO meta_observations (id, observation_type, severity, details, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		obs.ID, obs.ObservationType, obs.Severity, string(detailsJSON), obs.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to write meta observation: %w", err)
	}
	return nil
}

// WriteRetrievalFeedback stores a retrieval traversal record for later feedback processing.
func (s *SQLiteStore) WriteRetrievalFeedback(ctx context.Context, fb store.RetrievalFeedback) error {
	retrievedJSON, err := json.Marshal(fb.RetrievedIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal retrieved IDs: %w", err)
	}
	traversedJSON, err := json.Marshal(fb.TraversedAssocs)
	if err != nil {
		return fmt.Errorf("failed to marshal traversed assocs: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO retrieval_feedback (query_id, query_text, retrieved_memory_ids, traversed_assocs, feedback, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		fb.QueryID, fb.QueryText, string(retrievedJSON), string(traversedJSON), fb.Feedback, fb.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to write retrieval feedback: %w", err)
	}
	return nil
}

// GetRetrievalFeedback retrieves a feedback record by query ID.
func (s *SQLiteStore) GetRetrievalFeedback(ctx context.Context, queryID string) (store.RetrievalFeedback, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT query_id, query_text, retrieved_memory_ids, COALESCE(traversed_assocs, '[]'), COALESCE(feedback, ''), created_at
		 FROM retrieval_feedback WHERE query_id = ?`, queryID)

	var fb store.RetrievalFeedback
	var retrievedJSON, traversedJSON, createdAtStr string
	err := row.Scan(&fb.QueryID, &fb.QueryText, &retrievedJSON, &traversedJSON, &fb.Feedback, &createdAtStr)
	if err != nil {
		return store.RetrievalFeedback{}, fmt.Errorf("failed to get retrieval feedback: %w", err)
	}

	if err := json.Unmarshal([]byte(retrievedJSON), &fb.RetrievedIDs); err != nil {
		fb.RetrievedIDs = nil
	}
	if err := json.Unmarshal([]byte(traversedJSON), &fb.TraversedAssocs); err != nil {
		fb.TraversedAssocs = nil
	}
	fb.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)

	return fb, nil
}

// ListMetaObservations retrieves observations, optionally filtered by type.
func (s *SQLiteStore) ListMetaObservations(ctx context.Context, observationType string, limit int) ([]store.MetaObservation, error) {
	var rows *sql.Rows
	var err error

	if observationType != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, observation_type, severity, details, created_at
			 FROM meta_observations WHERE observation_type = ?
			 ORDER BY created_at DESC LIMIT ?`,
			observationType, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, observation_type, severity, details, created_at
			 FROM meta_observations
			 ORDER BY created_at DESC LIMIT ?`,
			limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list meta observations: %w", err)
	}
	defer rows.Close()

	var observations []store.MetaObservation
	for rows.Next() {
		var obs store.MetaObservation
		var detailsJSON, createdStr string
		if err := rows.Scan(&obs.ID, &obs.ObservationType, &obs.Severity, &detailsJSON, &createdStr); err != nil {
			return nil, fmt.Errorf("failed to scan meta observation: %w", err)
		}
		obs.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		if obs.CreatedAt.IsZero() {
			obs.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
		}
		if detailsJSON != "" {
			_ = json.Unmarshal([]byte(detailsJSON), &obs.Details)
		}
		observations = append(observations, obs)
	}
	return observations, rows.Err()
}

// GetDeadMemories returns active memories that haven't been accessed since cutoffDate.
func (s *SQLiteStore) GetDeadMemories(ctx context.Context, cutoffDate time.Time) ([]store.Memory, error) {
	cutoffStr := cutoffDate.Format("2006-01-02 15:04:05")
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+memoryColumns+`
		 FROM memories
		 WHERE state = 'active'
		 AND (last_accessed IS NULL OR last_accessed < ?)
		 ORDER BY salience ASC`,
		cutoffStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get dead memories: %w", err)
	}

	return scanMemoryRows(rows)
}

// GetSourceDistribution returns a count of raw memories grouped by source.
func (s *SQLiteStore) GetSourceDistribution(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source, COUNT(*) FROM raw_memories GROUP BY source ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to get source distribution: %w", err)
	}
	defer rows.Close()

	dist := make(map[string]int)
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, fmt.Errorf("failed to scan source distribution: %w", err)
		}
		dist[source] = count
	}
	return dist, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Helper functions

// nullableString converts an empty string to nil for SQL NULL, or returns the string.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// boolToInt converts a boolean to an int (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cosineSimilarity computes the cosine similarity between two embedding vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}
