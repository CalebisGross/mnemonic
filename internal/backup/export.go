package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/appsprout/mnemonic/internal/store"
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
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func ExportSQLite(ctx context.Context, dbPath string, outputPath string) error {
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer src.Close()

	tmpPath := outputPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to copy database: %w", err)
	}

	if err := dst.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close destination file: %w", err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		os.Remove(tmpPath)
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
