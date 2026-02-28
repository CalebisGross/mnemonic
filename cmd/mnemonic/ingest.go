package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/appsprout/mnemonic/internal/config"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/appsprout/mnemonic/internal/store/sqlite"
	"github.com/appsprout/mnemonic/internal/watcher/filesystem"
)

func ingestCommand(configPath string, args []string) {
	// Separate flags from positional args (Go's flag package requires flags first,
	// but users naturally write: mnemonic ingest <dir> --dry-run)
	var flagArgs []string
	var posArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--dry-run" {
			flagArgs = append(flagArgs, args[i])
		} else if args[i] == "--project" && i+1 < len(args) {
			flagArgs = append(flagArgs, args[i], args[i+1])
			i++ // skip the value
		} else {
			posArgs = append(posArgs, args[i])
		}
	}

	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "scan and report without writing")
	projectName := fs.String("project", "", "project name (default: directory basename)")
	if err := fs.Parse(append(flagArgs, posArgs...)); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: mnemonic ingest <directory> [--dry-run] [--project NAME]\n")
		os.Exit(1)
	}

	dir := fs.Arg(0)

	// Resolve to absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Verify directory exists
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", absDir)
		os.Exit(1)
	}

	// Detect project name
	project := *projectName
	if project == "" {
		project = filepath.Base(absDir)
	}

	// Load config for exclude patterns and MaxContentBytes
	cfg, err := loadIngestConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Ingesting %s...\n", absDir)
	fmt.Printf("  Project: %s\n", project)

	// Phase 1: Discover files
	var files []string
	excluded := 0
	maxBytes := cfg.maxContentBytes

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Skip excluded directories (prune entire subtree)
		if info.IsDir() {
			if filesystem.MatchesExcludePattern(path, cfg.excludePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-regular files (symlinks, devices, etc.)
		if !info.Mode().IsRegular() {
			excluded++
			return nil
		}

		// Skip binary files by extension
		if filesystem.IsBinaryFile(path) {
			excluded++
			return nil
		}

		// Skip excluded paths
		if filesystem.MatchesExcludePattern(path, cfg.excludePatterns) {
			excluded++
			return nil
		}

		// Skip empty files
		if info.Size() == 0 {
			excluded++
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Scanning... %d files found (%d excluded)\n", len(files), excluded)

	if *dryRun {
		fmt.Println("\n  [dry-run] Files that would be ingested:")
		for _, f := range files {
			rel, _ := filepath.Rel(absDir, f)
			fmt.Printf("    %s\n", rel)
		}
		fmt.Printf("\n  Total: %d files (dry run — nothing written)\n", len(files))
		return
	}

	if len(files) == 0 {
		fmt.Println("  No files to ingest.")
		return
	}

	// Phase 2: Open store and write raw memories
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	db, err := sqlite.NewSQLiteStore(cfg.dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("  Writing to store...")

	ctx := context.Background()
	written := 0
	failed := 0
	skipped := 0
	start := time.Now()

	for i, path := range files {
		relPath, _ := filepath.Rel(absDir, path)

		// Read content with binary content check
		content := filesystem.ReadFileContent(path, maxBytes, log)
		if content == "" {
			skipped++
			continue
		}
		if filesystem.IsBinaryContent(content) {
			skipped++
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

		if err := db.WriteRaw(ctx, raw); err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR writing %s: %v\n", relPath, err)
			failed++
			continue
		}

		written++
		fmt.Printf("  [%d/%d] %s\n", i+1, len(files), relPath)
	}

	elapsed := time.Since(start)
	encodeEstimate := time.Duration(written) * 8 * time.Second

	fmt.Printf("\n  Done: %d files ingested in %s\n", written, elapsed.Round(time.Millisecond))
	if skipped > 0 {
		fmt.Printf("  Skipped: %d (binary content or empty)\n", skipped)
	}
	if failed > 0 {
		fmt.Printf("  Failed: %d\n", failed)
	}
	fmt.Printf("  The daemon will encode these over the next ~%s.\n", encodeEstimate.Round(time.Minute))
}

// ingestConfig holds the subset of config needed for ingestion.
type ingestConfig struct {
	excludePatterns []string
	maxContentBytes int
	dbPath          string
}

func loadIngestConfig(configPath string) (*ingestConfig, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	return &ingestConfig{
		excludePatterns: cfg.Perception.Filesystem.ExcludePatterns,
		maxContentBytes: cfg.Perception.Filesystem.MaxContentBytes,
		dbPath:          cfg.Store.DBPath,
	}, nil
}
