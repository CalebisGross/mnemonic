//go:build sqlite_fts5

package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/store/sqlite"
)

// setupTestStore creates a temporary SQLite store seeded with test data.
// It returns the store, the path to the DB file, and a cleanup function.
func setupTestStore(t *testing.T) (store.Store, string, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := sqlite.NewSQLiteStore(dbPath, 5000)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	ctx := context.Background()
	now := time.Now()

	// Seed raw memories
	if err := s.WriteRaw(ctx, store.RawMemory{
		ID: "raw-1", Timestamp: now, Source: "test", Type: "test",
		Content: "test content one", Processed: true, CreatedAt: now,
	}); err != nil {
		t.Fatalf("failed to write raw-1: %v", err)
	}
	if err := s.WriteRaw(ctx, store.RawMemory{
		ID: "raw-2", Timestamp: now, Source: "test", Type: "test",
		Content: "test content two", Processed: true, CreatedAt: now,
	}); err != nil {
		t.Fatalf("failed to write raw-2: %v", err)
	}

	// Seed encoded memories
	if err := s.WriteMemory(ctx, store.Memory{
		ID: "mem-1", RawID: "raw-1", Timestamp: now,
		Content: "encoded content one", Summary: "summary one",
		Concepts: []string{"go", "testing"}, Salience: 0.5, State: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to write mem-1: %v", err)
	}
	if err := s.WriteMemory(ctx, store.Memory{
		ID: "mem-2", RawID: "raw-2", Timestamp: now,
		Content: "encoded content two", Summary: "summary two",
		Concepts: []string{"backup", "export"}, Salience: 0.7, State: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to write mem-2: %v", err)
	}

	// Seed an association
	if err := s.CreateAssociation(ctx, store.Association{
		SourceID: "mem-1", TargetID: "mem-2",
		Strength: 0.8, RelationType: "similar",
		CreatedAt: now, LastActivated: now, ActivationCount: 1,
	}); err != nil {
		t.Fatalf("failed to create association: %v", err)
	}

	cleanup := func() {
		_ = s.Close()
	}
	return s, dbPath, cleanup
}

func TestExportJSON_Success(t *testing.T) {
	s, _, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	outputPath := filepath.Join(t.TempDir(), "export.json")

	if err := ExportJSON(ctx, s, outputPath); err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("exported file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("exported file is empty")
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("exported JSON content is empty")
	}

	// Verify the JSON contains our seeded memories
	if !jsonContains(content, "mem-1") {
		t.Error("exported JSON does not contain mem-1")
	}
	if !jsonContains(content, "mem-2") {
		t.Error("exported JSON does not contain mem-2")
	}
}

func TestExportJSON_ParsesCorrectly(t *testing.T) {
	s, _, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	outputPath := filepath.Join(t.TempDir(), "export.json")

	if err := ExportJSON(ctx, s, outputPath); err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	var exported ExportData
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("failed to unmarshal exported JSON: %v", err)
	}

	if exported.Metadata.MemoryCount != 2 {
		t.Errorf("expected 2 memories in metadata, got %d", exported.Metadata.MemoryCount)
	}
	if len(exported.Memories) != 2 {
		t.Errorf("expected 2 memories in data, got %d", len(exported.Memories))
	}
	if exported.Metadata.AssocCount != 1 {
		t.Errorf("expected 1 association in metadata, got %d", exported.Metadata.AssocCount)
	}
	if len(exported.Associations) != 1 {
		t.Errorf("expected 1 association in data, got %d", len(exported.Associations))
	}
	if exported.Metadata.RawCount != 2 {
		t.Errorf("expected 2 raw memories in metadata, got %d", exported.Metadata.RawCount)
	}
	if len(exported.RawMemories) != 2 {
		t.Errorf("expected 2 raw memories in data, got %d", len(exported.RawMemories))
	}
	if exported.Metadata.Version == "" {
		t.Error("expected non-empty version in metadata")
	}
}

func TestExportSQLite_Success(t *testing.T) {
	s, dbPath, _ := setupTestStore(t)

	// Close the store to flush the WAL before copying the raw file.
	if err := s.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "export.db")

	if err := ExportSQLite(context.Background(), dbPath, outputPath); err != nil {
		t.Fatalf("ExportSQLite failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("exported SQLite file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("exported SQLite file is empty")
	}

	// Verify the file is a valid SQLite database by opening it
	db, err := sql.Open("sqlite3", outputPath)
	if err != nil {
		t.Fatalf("failed to open exported SQLite file: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		t.Fatalf("exported SQLite file is not a valid database: %v", err)
	}

	// Verify the memories table exists and has data
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count); err != nil {
		t.Fatalf("failed to query memories table: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 memories in exported DB, got %d", count)
	}
}

func TestBackupWithRetention(t *testing.T) {
	s, _, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	backupDir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	// BackupWithRetention uses a timestamp format that produces the same
	// filename within the same hour (minutes/seconds are literal "30"/"45").
	// To test retention properly, create pre-existing backup files with
	// distinct names, then call BackupWithRetention once with maxBackups=2.
	oldFiles := []string{
		filepath.Join(backupDir, "backup_2020-01-01_10-30-45.json"),
		filepath.Join(backupDir, "backup_2020-01-02_10-30-45.json"),
	}
	for _, f := range oldFiles {
		if err := os.WriteFile(f, []byte(`{"metadata":{}}`), 0644); err != nil {
			t.Fatalf("failed to create old backup file: %v", err)
		}
	}

	// Now call BackupWithRetention with maxBackups=2.
	// This creates a third file (timestamp > old files), then prunes to 2.
	path, err := BackupWithRetention(ctx, s, backupDir, 2)
	if err != nil {
		t.Fatalf("BackupWithRetention failed: %v", err)
	}
	if path == "" {
		t.Fatal("BackupWithRetention returned empty path")
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("failed to read backup dir: %v", err)
	}

	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}

	if len(jsonFiles) != 2 {
		t.Errorf("expected 2 backup files after retention, got %d: %v", len(jsonFiles), jsonFiles)
	}

	// The oldest file should have been deleted
	for _, f := range jsonFiles {
		if f == "backup_2020-01-01_10-30-45.json" {
			t.Error("oldest backup file was not deleted by retention")
		}
	}
}

func TestBackupSQLiteFile_ExistingDB(t *testing.T) {
	_, dbPath, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	backupPath, err := BackupSQLiteFile(dbPath, backupDir)
	if err != nil {
		t.Fatalf("BackupSQLiteFile failed: %v", err)
	}
	if backupPath == "" {
		t.Fatal("BackupSQLiteFile returned empty path for existing DB")
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}
}

func TestBackupSQLiteFile_NoDB(t *testing.T) {
	backupDir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	nonexistentPath := filepath.Join(t.TempDir(), "does_not_exist.db")

	backupPath, err := BackupSQLiteFile(nonexistentPath, backupDir)
	if err != nil {
		t.Fatalf("BackupSQLiteFile returned unexpected error: %v", err)
	}
	if backupPath != "" {
		t.Errorf("expected empty path for nonexistent DB, got %q", backupPath)
	}
}

func TestEnsureBackupDir(t *testing.T) {
	dir, err := EnsureBackupDir()
	if err != nil {
		t.Fatalf("EnsureBackupDir failed: %v", err)
	}

	expected := filepath.Join(os.Getenv("HOME"), ".mnemonic", "backups")
	if dir != expected {
		t.Errorf("expected backup dir %q, got %q", expected, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("backup dir does not exist after EnsureBackupDir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("backup dir path is not a directory")
	}
}

func TestReadSchemaVersion_AfterInitSchema(t *testing.T) {
	_, dbPath, cleanup := setupTestStore(t)
	defer cleanup()

	ver, err := ReadSchemaVersion(dbPath)
	if err != nil {
		t.Fatalf("ReadSchemaVersion failed: %v", err)
	}
	if ver != sqlite.SchemaVersion {
		t.Errorf("expected schema version %d, got %d", sqlite.SchemaVersion, ver)
	}
}

func TestReadSchemaVersion_FreshDB(t *testing.T) {
	// A fresh SQLite DB with no PRAGMA user_version set returns 0.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fresh.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open fresh db: %v", err)
	}
	// Create at least one table so the file exists on disk.
	if _, err := db.Exec("CREATE TABLE t(id INTEGER)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	_ = db.Close()

	ver, err := ReadSchemaVersion(dbPath)
	if err != nil {
		t.Fatalf("ReadSchemaVersion failed: %v", err)
	}
	if ver != 0 {
		t.Errorf("expected version 0 for fresh DB, got %d", ver)
	}
}

func TestPruneOldBackups(t *testing.T) {
	dir := t.TempDir()

	// Create 5 fake pre-migration backups with distinct timestamps.
	names := []string{
		"pre_migrate_2026-01-01_100000.db",
		"pre_migrate_2026-01-02_100000.db",
		"pre_migrate_2026-01-03_100000.db",
		"pre_migrate_2026-01-04_100000.db",
		"pre_migrate_2026-01-05_100000.db",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("fake"), 0644); err != nil {
			t.Fatalf("failed to create %s: %v", n, err)
		}
	}

	// Also create a non-matching file that should NOT be pruned.
	if err := os.WriteFile(filepath.Join(dir, "backup_2026-01-01.json"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := PruneOldBackups(dir, 3); err != nil {
		t.Fatalf("PruneOldBackups failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}

	// Should have 3 pre_migrate files + 1 json file = 4 total
	if len(remaining) != 4 {
		t.Errorf("expected 4 files remaining, got %d: %v", len(remaining), remaining)
	}

	// The oldest two should be gone.
	for _, name := range remaining {
		if name == names[0] || name == names[1] {
			t.Errorf("old backup %s should have been pruned", name)
		}
	}
}

func TestPruneOldBackups_FewFiles(t *testing.T) {
	dir := t.TempDir()

	// Only 2 backups, keep=3 — nothing should be deleted.
	for _, n := range []string{"pre_migrate_2026-01-01_100000.db", "pre_migrate_2026-01-02_100000.db"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := PruneOldBackups(dir, 3); err != nil {
		t.Fatalf("PruneOldBackups failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}
}

// jsonContains checks if a JSON string contains a given substring.
func jsonContains(jsonStr, substr string) bool {
	return len(jsonStr) > 0 && contains(jsonStr, substr)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
