package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateStagedTShockInstallationRequiresExpectedVersion(t *testing.T) {
	stageDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stageDir, "TShock.Server"), []byte("server"), 0755); err != nil {
		t.Fatalf("write core file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, ".tshock_version"), []byte("5.2.4"), 0644); err != nil {
		t.Fatalf("write version marker: %v", err)
	}

	if runtime.GOOS != "linux" {
		if err := validateStagedTShockInstallation(stageDir, "5"); err != nil {
			t.Fatalf("validate matching staged TShock: %v", err)
		}
	}
	if err := validateStagedTShockInstallation(stageDir, "6"); err == nil {
		t.Fatal("expected staged TShock validation to reject a mismatched major version")
	}
}

func TestReplaceTShockInstallationDirectoryReplacesOnlyAfterPreparedStage(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "tshock")
	stageDir := filepath.Join(root, "stage")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "old-core"), []byte("old"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "TShock.Server"), []byte("new"), 0755); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	if err := replaceTShockInstallationDirectory(stageDir, targetDir); err != nil {
		t.Fatalf("replace TShock directory: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "TShock.Server")); err != nil || string(got) != "new" {
		t.Fatalf("replacement core = %q, err=%v", string(got), err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "old-core")); !os.IsNotExist(err) {
		t.Fatalf("old program file survived replacement, stat err=%v", err)
	}
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Fatalf("stage directory should have been renamed away, stat err=%v", err)
	}
}

func TestCopyTShockUserDataPreservesPluginsAndDatabaseOnly(t *testing.T) {
	root := t.TempDir()
	existingDir := filepath.Join(root, "existing")
	stageDir := filepath.Join(root, "stage")
	for _, dir := range []string{
		filepath.Join(existingDir, "ServerPlugins"),
		filepath.Join(existingDir, "tshock"),
		stageDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(existingDir, "ServerPlugins", "Useful.dll"), []byte("plugin"), 0644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "tshock", "tshock.sqlite"), []byte("database"), 0644); err != nil {
		t.Fatalf("write database: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "TShock.Server"), []byte("old program"), 0755); err != nil {
		t.Fatalf("write old program: %v", err)
	}
	if err := copyTShockUserData(existingDir, stageDir); err != nil {
		t.Fatalf("copy user data: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDir, "ServerPlugins", "Useful.dll")); err != nil {
		t.Fatalf("plugin was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDir, "tshock", "tshock.sqlite")); err != nil {
		t.Fatalf("database was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDir, "TShock.Server")); !os.IsNotExist(err) {
		t.Fatalf("old program must not be copied into stage, stat err=%v", err)
	}
}
