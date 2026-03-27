package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

type ExportFormat string

const (
	FormatJSON   ExportFormat = "json"
	FormatSQLite ExportFormat = "sqlite"
)

type ExportMetadata struct {
	Version     string    `json:"version"`
	ExportTime  time.Time `json:"export_time"`
	MemoryCount int       `json:"memory_count"`
	AssocCount  int       `json:"association_count"`
	RawCount    int       `json:"raw_memory_count"`
}

type ExportData struct {
	Metadata     ExportMetadata      `json:"metadata"`
	Memories     []store.Memory      `json:"memories"`
	Associations []store.Association `json:"associations"`
	RawMemories  []store.RawMemory   `json:"raw_memories"`
}

func ExportJSON(ctx context.Context, s store.Store, outputPath string) error {
	memories, err := s.ListMemories(ctx, "", 10000, 0)
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	associations, err := s.ListAllAssociations(ctx)
	if err != nil {
		return fmt.Errorf("failed to list associations: %w", err)
	}

	rawMemories, err := s.ListAllRawMemories(ctx)
	if err != nil {
		return fmt.Errorf("failed to list raw memories: %w", err)
	}

	exportData := ExportData{
		Metadata: ExportMetadata{
			Version:     "0.3.0",
			ExportTime:  time.Now(),
			MemoryCount: len(memories),
			AssocCount:  len(associations),
			RawCount:    len(rawMemories),
		},
		Memories:     memories,
		Associations: associations,
		RawMemories:  rawMemories,
	}

	jsonData, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal export data: %w", err)
	}

	tmpPath := outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func ExportSQLite(ctx context.Context, dbPath string, outputPath string) error {
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer func() { _ = src.Close() }()

	tmpPath := outputPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to copy database: %w", err)
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close destination file: %w", err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func BackupWithRetention(ctx context.Context, s store.Store, backupDir string, maxBackups int) (string, error) {
	timestamp := time.Now().Format("2006-01-02_15-30-45")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("backup_%s.json", timestamp))

	if err := ExportJSON(ctx, s, backupPath); err != nil {
		return "", fmt.Errorf("failed to export JSON: %w", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return "", fmt.Errorf("failed to read backup directory: %w", err)
	}

	var jsonFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			jsonFiles = append(jsonFiles, entry.Name())
		}
	}

	// Sort by name (which includes timestamp in ascending order)
	if len(jsonFiles) > maxBackups {
		filesToDelete := len(jsonFiles) - maxBackups
		for i := 0; i < filesToDelete; i++ {
			oldBackupPath := filepath.Join(backupDir, jsonFiles[i])
			if err := os.Remove(oldBackupPath); err != nil {
				return "", fmt.Errorf("failed to delete old backup: %w", err)
			}
		}
	}

	return backupPath, nil
}

// BackupSQLiteFile creates a timestamped copy of the database file in the backup directory.
// This is intended for pre-migration safety backups before the store is opened.
// Returns the backup path, or empty string if the source does not exist.
func BackupSQLiteFile(dbPath string, backupDir string) (string, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", nil // no DB yet, nothing to back up
	}

	timestamp := time.Now().Format("2006-01-02_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("pre_migrate_%s.db", timestamp))

	if err := ExportSQLite(context.Background(), dbPath, backupPath); err != nil {
		return "", fmt.Errorf("backing up database: %w", err)
	}

	return backupPath, nil
}

// ReadSchemaVersion opens the database read-only and returns PRAGMA user_version.
// Returns 0 for databases that have never had the version set (pre-existing DBs).
func ReadSchemaVersion(dbPath string) (int, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("opening database for version check: %w", err)
	}
	defer func() { _ = db.Close() }()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("reading user_version: %w", err)
	}
	return version, nil
}

// PruneOldBackups keeps the most recent `keep` pre-migration backup files in dir
// and removes older ones. Only targets files matching the "pre_migrate_*.db" pattern.
func PruneOldBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading backup directory: %w", err)
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "pre_migrate_") && strings.HasSuffix(e.Name(), ".db") {
			backups = append(backups, e.Name())
		}
	}

	// Filenames contain timestamps, so lexicographic sort = chronological order.
	sort.Strings(backups)

	if len(backups) <= keep {
		return nil
	}

	for _, name := range backups[:len(backups)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("removing old backup %s: %w", name, err)
		}
	}
	return nil
}

func EnsureBackupDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	backupDir := filepath.Join(homeDir, ".mnemonic", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	return backupDir, nil
}
