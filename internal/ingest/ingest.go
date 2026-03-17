package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/ingest/extract"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/watcher/filesystem"
)

const batchSize = 50

// ProjectResolver resolves paths and names to canonical project names.
type ProjectResolver interface {
	Resolve(input string) string
}

// Config holds parameters for an ingestion run.
type Config struct {
	Dir               string
	Project           string
	DryRun            bool
	ExcludePatterns   []string
	SensitivePatterns []string
	MaxContentBytes   int
	OnProgress        func(current, total int, path string) // optional progress callback
	ProjectResolver   ProjectResolver
}

// Result holds the outcome of an ingestion run.
type Result struct {
	FilesFound        int
	FilesWritten      int
	FilesSkipped      int
	FilesFailed       int
	FilesExtracted    int
	ChunksCreated     int
	DuplicatesSkipped int
	Project           string
	Elapsed           time.Duration
}

// minChunkWords is the minimum word count for a document chunk to be ingested.
const minChunkWords = 50

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

	// Resolve project name
	project := cfg.Project
	if cfg.ProjectResolver != nil {
		if project != "" {
			project = cfg.ProjectResolver.Resolve(project)
		} else {
			project = cfg.ProjectResolver.Resolve(absDir)
		}
	}
	if project == "" {
		project = filepath.Base(absDir)
	}

	// Build extractor registry
	registry := buildExtractorRegistry(log)

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
		// Allow extractable binary files through; skip the rest
		ext := strings.ToLower(filepath.Ext(path))
		if filesystem.IsBinaryFile(path) && !registry.HasExtractor(ext) {
			excluded++
			return nil
		}
		if len(cfg.SensitivePatterns) > 0 && filesystem.IsSensitiveFile(path, cfg.SensitivePatterns) {
			log.Warn("skipping sensitive file", "path", path)
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

		fileInfo, _ := os.Stat(path)
		var fileSize int64
		if fileInfo != nil {
			fileSize = fileInfo.Size()
		}

		baseMeta := map[string]any{
			"path":     path,
			"rel_path": relPath,
			"ext":      filepath.Ext(path),
			"size":     fileSize,
		}

		// Check if we have an extractor for this file type
		ext := strings.ToLower(filepath.Ext(path))
		if extractor := registry.Get(ext); extractor != nil {
			newRaws := extractDocument(extractor, path, relPath, cfg.MaxContentBytes, baseMeta, project, log)
			if len(newRaws) == 0 {
				result.FilesSkipped++
			} else {
				result.FilesExtracted++
				result.ChunksCreated += len(newRaws)
				batch = append(batch, newRaws...)
			}
		} else {
			// Plain text path (unchanged)
			content := filesystem.ReadFileContent(path, cfg.MaxContentBytes, log)
			if content == "" {
				result.FilesSkipped++
				if cfg.OnProgress != nil {
					cfg.OnProgress(i+1, len(files), relPath)
				}
				continue
			}
			if filesystem.IsBinaryContent(content) {
				result.FilesSkipped++
				if cfg.OnProgress != nil {
					cfg.OnProgress(i+1, len(files), relPath)
				}
				continue
			}

			raw := store.RawMemory{
				ID:              uuid.New().String(),
				Timestamp:       time.Now(),
				Source:          "ingest",
				Type:            "file",
				Content:         fmt.Sprintf("File %s:\n%s", relPath, content),
				Metadata:        baseMeta,
				HeuristicScore:  0.5,
				InitialSalience: 0.5,
				Processed:       false,
				CreatedAt:       time.Now(),
				Project:         project,
			}
			batch = append(batch, raw)
		}

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

// buildExtractorRegistry creates the extractor registry, registering
// available extractors and logging their status.
func buildExtractorRegistry(log *slog.Logger) *extract.Registry {
	registry := extract.NewRegistry()

	pdfExt := &extract.PDFExtractor{}
	if pdfExt.Available() {
		registry.Register(".pdf", pdfExt)
		log.Info("PDF extraction enabled (pdftotext found)")
	} else {
		log.Warn("PDF extraction disabled: pdftotext not found in PATH")
	}

	docxExt := &extract.DOCXExtractor{}
	registry.Register(".docx", docxExt)

	pptxExt := &extract.PPTXExtractor{}
	registry.Register(".pptx", pptxExt)

	rtfExt := &extract.RTFExtractor{}
	registry.Register(".rtf", rtfExt)

	odtExt := &extract.ODTExtractor{}
	registry.Register(".odt", odtExt)

	log.Info("document extraction enabled",
		"formats", "pdf,docx,pptx,rtf,odt")

	return registry
}

// extractDocument runs the extractor on a file and produces RawMemory entries:
// one summary memory for the full document, plus per-chunk memories for
// individual pages or sections. Chunks below minChunkWords are skipped.
func extractDocument(
	extractor extract.Extractor,
	path, relPath string,
	maxBytes int,
	baseMeta map[string]any,
	project string,
	log *slog.Logger,
) []store.RawMemory {
	result, err := extractor.Extract(path, maxBytes, log)
	if err != nil {
		log.Warn("extraction failed, skipping", "path", path, "error", err)
		return nil
	}
	if result.FullText == "" {
		return nil
	}

	documentID := uuid.New().String()
	totalChunks := len(result.Chunks) + 1 // +1 for summary
	var raws []store.RawMemory

	// Summary memory (chunk 0) — full document
	if extract.WordCount(result.FullText) >= minChunkWords {
		meta := mergeMeta(baseMeta, result.Metadata, map[string]any{
			"document_id":  documentID,
			"chunk_index":  0,
			"total_chunks": totalChunks,
		})
		raws = append(raws, store.RawMemory{
			ID:              uuid.New().String(),
			Timestamp:       time.Now(),
			Source:          "ingest",
			Type:            "document",
			Content:         fmt.Sprintf("Document %s:\n%s", relPath, result.FullText),
			Metadata:        meta,
			HeuristicScore:  0.5,
			InitialSalience: 0.5,
			Processed:       false,
			CreatedAt:       time.Now(),
			Project:         project,
		})
	}

	// Page/section memories (chunks 1..N)
	for i, chunk := range result.Chunks {
		if extract.WordCount(chunk.Text) < minChunkWords {
			continue
		}
		meta := mergeMeta(baseMeta, result.Metadata, map[string]any{
			"document_id":  documentID,
			"chunk_index":  i + 1,
			"total_chunks": totalChunks,
			"source_page":  chunk.PageNumber,
		})
		label := fmt.Sprintf("Document %s (section %d)", relPath, chunk.PageNumber)
		raws = append(raws, store.RawMemory{
			ID:              uuid.New().String(),
			Timestamp:       time.Now(),
			Source:          "ingest",
			Type:            "document",
			Content:         fmt.Sprintf("%s:\n%s", label, chunk.Text),
			Metadata:        meta,
			HeuristicScore:  0.5,
			InitialSalience: 0.5,
			Processed:       false,
			CreatedAt:       time.Now(),
			Project:         project,
		})
	}

	return raws
}

// mergeMeta combines multiple metadata maps into one. Later maps override earlier ones.
func mergeMeta(maps ...map[string]any) map[string]any {
	merged := make(map[string]any)
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
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
