package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/appsprout/mnemonic/internal/watcher/filesystem"
)

const batchSize = 50

// Config holds parameters for an ingestion run.
type Config struct {
	Dir             string
	Project         string
	DryRun          bool
	ExcludePatterns []string
	MaxContentBytes int
	OnProgress      func(current, total int, path string) // optional progress callback
}

// Result holds the outcome of an ingestion run.
type Result struct {
	FilesFound        int
	FilesWritten      int
	FilesSkipped      int
	FilesFailed       int
	DuplicatesSkipped int
	Project           string
	Elapsed           time.Duration
}

// Run executes directory ingestion. It walks the directory, applies filters,
// deduplicates against existing raw memories, and writes new raw memories
// in batched transactions.
func Run(ctx context.Context, cfg Config, s store.Store, bus events.Bus, log *slog.Logger) (*Result, error) {
	// Resolve directory
	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", cfg.Dir, err)
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", absDir)
	}

	// Default project name
	project := cfg.Project
	if project == "" {
		project = filepath.Base(absDir)
	}

	// Phase 1: Discover files
	var files []string
	excluded := 0

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if filesystem.MatchesExcludePattern(path, cfg.ExcludePatterns) {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			excluded++
			return nil
		}
		if filesystem.IsBinaryFile(path) {
			excluded++
			return nil
		}
		if filesystem.MatchesExcludePattern(path, cfg.ExcludePatterns) {
			excluded++
			return nil
		}
		if info.Size() == 0 {
			excluded++
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning directory: %w", err)
	}

	result := &Result{
		FilesFound: len(files),
		Project:    project,
	}

	if cfg.DryRun {
		return result, nil
	}

	if len(files) == 0 {
		return result, nil
	}

	// Phase 2: Read, dedup, and batch-write
	start := time.Now()
	var batch []store.RawMemory

	for i, path := range files {
		relPath, _ := filepath.Rel(absDir, path)

		// Dedup: skip if already ingested
		exists, err := s.RawMemoryExistsByPath(ctx, "ingest", project, path)
		if err != nil {
			log.Warn("dedup check failed", "path", path, "error", err)
		}
		if exists {
			result.DuplicatesSkipped++
			continue
		}

		// Read content
		content := filesystem.ReadFileContent(path, cfg.MaxContentBytes, log)
		if content == "" {
			result.FilesSkipped++
			continue
		}
		if filesystem.IsBinaryContent(content) {
			result.FilesSkipped++
			continue
		}

		fileInfo, _ := os.Stat(path)
		var fileSize int64
		if fileInfo != nil {
			fileSize = fileInfo.Size()
		}

		raw := store.RawMemory{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Source:    "ingest",
			Type:      "file",
			Content:   fmt.Sprintf("File %s:\n%s", relPath, content),
			Metadata: map[string]interface{}{
				"path":     path,
				"rel_path": relPath,
				"ext":      filepath.Ext(path),
				"size":     fileSize,
			},
			HeuristicScore:  0.5,
			InitialSalience: 0.5,
			Processed:       false,
			CreatedAt:       time.Now(),
			Project:         project,
		}

		batch = append(batch, raw)

		if cfg.OnProgress != nil {
			cfg.OnProgress(i+1, len(files), relPath)
		}

		// Flush batch
		if len(batch) >= batchSize {
			if err := flushBatch(ctx, s, bus, batch); err != nil {
				result.FilesFailed += len(batch)
				log.Error("batch write failed", "error", err)
			} else {
				result.FilesWritten += len(batch)
			}
			batch = batch[:0]
		}
	}

	// Flush remaining
	if len(batch) > 0 {
		if err := flushBatch(ctx, s, bus, batch); err != nil {
			result.FilesFailed += len(batch)
			log.Error("batch write failed", "error", err)
		} else {
			result.FilesWritten += len(batch)
		}
	}

	result.Elapsed = time.Since(start)
	return result, nil
}

// flushBatch writes a batch of raw memories and optionally publishes events.
func flushBatch(ctx context.Context, s store.Store, bus events.Bus, batch []store.RawMemory) error {
	if err := s.BatchWriteRaw(ctx, batch); err != nil {
		return err
	}
	if bus != nil {
		for _, raw := range batch {
			_ = bus.Publish(ctx, events.RawMemoryCreated{
				ID:     raw.ID,
				Source: raw.Source,
				Ts:     time.Now(),
			})
		}
	}
	return nil
}
