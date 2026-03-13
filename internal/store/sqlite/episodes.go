package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	store "github.com/appsprout/mnemonic/internal/store"
)

// episodeColumns is the standard column list for episode queries.
const episodeColumns = `id, title, start_time, end_time, duration_sec, raw_memory_ids, memory_ids, summary, narrative, salience, emotional_tone, outcome, state, created_at, updated_at, concepts, files_modified, event_timeline`

// CreateEpisode inserts a new episode.
func (s *SQLiteStore) CreateEpisode(ctx context.Context, ep store.Episode) error {
	rawIDs, _ := encodeStringSlice(ep.RawMemoryIDs)
	memIDs, _ := encodeStringSlice(ep.MemoryIDs)
	concepts, _ := encodeStringSlice(ep.Concepts)
	files, _ := encodeStringSlice(ep.FilesModified)
	timeline, _ := json.Marshal(ep.EventTimeline)
	if ep.EventTimeline == nil {
		timeline = []byte("[]")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO episodes (`+episodeColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.ID,
		ep.Title,
		ep.StartTime.Format(time.RFC3339),
		ep.EndTime.Format(time.RFC3339),
		ep.DurationSec,
		rawIDs,
		memIDs,
		ep.Summary,
		ep.Narrative,
		ep.Salience,
		ep.EmotionalTone,
		ep.Outcome,
		ep.State,
		ep.CreatedAt.Format(time.RFC3339),
		ep.UpdatedAt.Format(time.RFC3339),
		concepts,
		files,
		string(timeline),
	)
	if err != nil {
		return fmt.Errorf("failed to create episode: %w", err)
	}
	return nil
}

// GetEpisode retrieves an episode by ID.
func (s *SQLiteStore) GetEpisode(ctx context.Context, id string) (store.Episode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+episodeColumns+` FROM episodes WHERE id = ?`, id)
	return scanEpisode(row)
}

// UpdateEpisode updates an existing episode.
func (s *SQLiteStore) UpdateEpisode(ctx context.Context, ep store.Episode) error {
	rawIDs, _ := encodeStringSlice(ep.RawMemoryIDs)
	memIDs, _ := encodeStringSlice(ep.MemoryIDs)
	concepts, _ := encodeStringSlice(ep.Concepts)
	files, _ := encodeStringSlice(ep.FilesModified)
	timeline, _ := json.Marshal(ep.EventTimeline)
	if ep.EventTimeline == nil {
		timeline = []byte("[]")
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE episodes SET title=?, start_time=?, end_time=?, duration_sec=?, raw_memory_ids=?, memory_ids=?, summary=?, narrative=?, salience=?, emotional_tone=?, outcome=?, state=?, updated_at=?, concepts=?, files_modified=?, event_timeline=?
		WHERE id = ?`,
		ep.Title,
		ep.StartTime.Format(time.RFC3339),
		ep.EndTime.Format(time.RFC3339),
		ep.DurationSec,
		rawIDs,
		memIDs,
		ep.Summary,
		ep.Narrative,
		ep.Salience,
		ep.EmotionalTone,
		ep.Outcome,
		ep.State,
		time.Now().Format(time.RFC3339),
		concepts,
		files,
		string(timeline),
		ep.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update episode: %w", err)
	}
	return nil
}

// ListEpisodes returns episodes filtered by state.
func (s *SQLiteStore) ListEpisodes(ctx context.Context, state string, limit, offset int) ([]store.Episode, error) {
	var rows *sql.Rows
	var err error

	if state == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+episodeColumns+` FROM episodes ORDER BY start_time DESC LIMIT ? OFFSET ?`, limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+episodeColumns+` FROM episodes WHERE state = ? ORDER BY start_time DESC LIMIT ? OFFSET ?`, state, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list episodes: %w", err)
	}
	defer rows.Close()

	var episodes []store.Episode
	for rows.Next() {
		ep, err := scanEpisodeRow(rows)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, ep)
	}
	return episodes, rows.Err()
}

// GetOpenEpisode returns the latest episode with state "open".
func (s *SQLiteStore) GetOpenEpisode(ctx context.Context) (store.Episode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+episodeColumns+` FROM episodes WHERE state = 'open' ORDER BY start_time DESC LIMIT 1`)
	return scanEpisode(row)
}

// CloseEpisode sets an episode's state to "closed".
func (s *SQLiteStore) CloseEpisode(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE episodes SET state = 'closed', updated_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to close episode: %w", err)
	}
	return nil
}

// scanEpisodeFrom scans a single Episode from any scanner.
func scanEpisodeFrom(s scanner) (store.Episode, error) {
	var ep store.Episode
	var startStr, endStr, createdStr, updatedStr string
	var rawIDsStr, memIDsStr string
	var title, summary, narrative, emotionalTone, outcome sql.NullString
	var conceptsStr, filesStr, timelineStr sql.NullString

	err := s.Scan(
		&ep.ID, &title, &startStr, &endStr, &ep.DurationSec,
		&rawIDsStr, &memIDsStr, &summary, &narrative,
		&ep.Salience, &emotionalTone, &outcome, &ep.State,
		&createdStr, &updatedStr,
		&conceptsStr, &filesStr, &timelineStr,
	)
	if err != nil {
		return ep, err
	}

	ep.Title = title.String
	ep.Summary = summary.String
	ep.Narrative = narrative.String
	ep.EmotionalTone = emotionalTone.String
	ep.Outcome = outcome.String
	ep.StartTime, _ = time.Parse(time.RFC3339, startStr)
	ep.EndTime, _ = time.Parse(time.RFC3339, endStr)
	ep.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	ep.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	ep.RawMemoryIDs, _ = decodeStringSlice(rawIDsStr)
	ep.MemoryIDs, _ = decodeStringSlice(memIDsStr)
	ep.Concepts, _ = decodeStringSlice(conceptsStr.String)
	ep.FilesModified, _ = decodeStringSlice(filesStr.String)
	if timelineStr.Valid && timelineStr.String != "" {
		_ = json.Unmarshal([]byte(timelineStr.String), &ep.EventTimeline)
	}

	return ep, nil
}

// scanEpisode scans a single row into an Episode.
func scanEpisode(row *sql.Row) (store.Episode, error) {
	ep, err := scanEpisodeFrom(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ep, fmt.Errorf("episode: %w", store.ErrNotFound)
		}
		return ep, fmt.Errorf("failed to scan episode: %w", err)
	}
	return ep, nil
}

// scanEpisodeRow scans from sql.Rows (for list queries).
func scanEpisodeRow(rows *sql.Rows) (store.Episode, error) {
	ep, err := scanEpisodeFrom(rows)
	if err != nil {
		return ep, fmt.Errorf("failed to scan episode row: %w", err)
	}
	return ep, nil
}
