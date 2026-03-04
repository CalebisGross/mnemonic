package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/appsprout/mnemonic/internal/config"
	"github.com/appsprout/mnemonic/internal/ingest"
	"github.com/appsprout/mnemonic/internal/store/sqlite"
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

	// Resolve to absolute path for display
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Open store
	db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Ingesting %s...\n", absDir)

	ctx := context.Background()
	icfg := ingest.Config{
		Dir:             dir,
		Project:         *projectName,
		DryRun:          *dryRun,
		ExcludePatterns: cfg.Perception.Filesystem.ExcludePatterns,
		MaxContentBytes: cfg.Perception.Filesystem.MaxContentBytes,
		OnProgress: func(current, total int, path string) {
			fmt.Printf("  [%d/%d] %s\n", current, total, path)
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	result, err := ingest.Run(ctx, icfg, db, nil, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Project: %s\n", result.Project)

	if *dryRun {
		fmt.Printf("  Found %d files (dry run — nothing written)\n", result.FilesFound)
		return
	}

	fmt.Printf("\n  Done: %d files ingested in %s\n", result.FilesWritten, result.Elapsed.Round(1))
	if result.DuplicatesSkipped > 0 {
		fmt.Printf("  Duplicates skipped: %d\n", result.DuplicatesSkipped)
	}
	if result.FilesSkipped > 0 {
		fmt.Printf("  Skipped (binary/empty): %d\n", result.FilesSkipped)
	}
	if result.FilesFailed > 0 {
		fmt.Printf("  Failed: %d\n", result.FilesFailed)
	}
	if result.FilesWritten > 0 {
		encodeEstimate := result.FilesWritten * 8
		fmt.Printf("  The daemon will encode these over the next ~%d minutes.\n", encodeEstimate/60)
	}
}
