package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/models"
)

func TestResolveBackupPathFallsBackToBackupDir(t *testing.T) {
	backupDir, cleanup := setupBackupResolvePathTest(t)
	defer cleanup()

	backupPath := filepath.Join(backupDir, "room-1_test_20260417_180000.zip")
	if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
		t.Fatalf("failed to create backup archive: %v", err)
	}

	resolvedPath, err := resolveBackupPath("room-1_test_20260417_180000")
	if err != nil {
		t.Fatalf("expected backup to resolve, got error: %v", err)
	}
	if resolvedPath != backupPath {
		t.Fatalf("expected %s, got %s", backupPath, resolvedPath)
	}
}

func TestResolveBackupPathUsesRecordedLocalPath(t *testing.T) {
	backupDir, cleanup := setupBackupResolvePathTest(t)
	defer cleanup()

	backupPath := filepath.Join(backupDir, "room-1_test_20260417_180500.zip")
	if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
		t.Fatalf("failed to create backup archive: %v", err)
	}

	recordStorage := getBackupRecordStorage()
	if err := recordStorage.Upsert(&models.BackupRecord{
		ID:             "backup-uuid-001",
		FileName:       "room-1_test_20260417_180500.zip",
		LocalPath:      backupPath,
		StorageType:    "local",
		UploadStatus:   "local_only",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ChecksumSHA256: "test",
	}); err != nil {
		t.Fatalf("failed to persist backup record: %v", err)
	}

	resolvedPath, err := resolveBackupPath("backup-uuid-001")
	if err != nil {
		t.Fatalf("expected record-based backup to resolve, got error: %v", err)
	}
	if resolvedPath != backupPath {
		t.Fatalf("expected %s, got %s", backupPath, resolvedPath)
	}
}

func setupBackupResolvePathTest(t *testing.T) (string, func()) {
	t.Helper()

	oldBackupDir := config.BackupDir
	oldDB := db.DB

	rootDir := t.TempDir()
	backupDir := filepath.Join(rootDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}
	config.BackupDir = backupDir

	if err := db.Init(filepath.Join(rootDir, "panel.db")); err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}

	return backupDir, func() {
		if db.DB != nil {
			_ = db.DB.Close()
		}
		db.DB = oldDB
		config.BackupDir = oldBackupDir
	}
}
