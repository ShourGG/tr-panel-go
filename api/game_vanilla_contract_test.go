package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFinalizePreparedGameFilesUsesRequestedVanillaVersion(t *testing.T) {
	targetDir := t.TempDir()
	platform := "Linux"
	binaryName := "TerrariaServer.bin.x86_64"
	if runtime.GOOS == "windows" {
		platform = "Windows"
		binaryName = "TerrariaServer.exe"
	}

	sourceDir := filepath.Join(targetDir, "1457", platform)
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("create vanilla source directory: %v", err)
	}
	sourcePath := filepath.Join(sourceDir, binaryName)
	if err := os.WriteFile(sourcePath, []byte("server"), 0755); err != nil {
		t.Fatalf("create vanilla server binary: %v", err)
	}

	if err := finalizePreparedGameFiles("vanilla", targetDir, "1457", "1.4.5.7", func(string, int) {}); err != nil {
		t.Fatalf("finalize vanilla files: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, binaryName)); err != nil {
		t.Fatalf("vanilla server was not moved to target root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "1457")); !os.IsNotExist(err) {
		t.Fatalf("version directory still exists or stat failed: %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(targetDir, ".installed_version"))
	if err != nil {
		t.Fatalf("read vanilla version marker: %v", err)
	}
	if string(marker) != "1.4.5.7" {
		t.Fatalf("version marker = %q, want 1.4.5.7", marker)
	}
}

func TestFindVanillaServerBinaryFindsNestedVersionDirectory(t *testing.T) {
	targetDir := t.TempDir()
	platform := "Linux"
	binaryName := "TerrariaServer.bin.x86_64"
	if runtime.GOOS == "windows" {
		platform = "Windows"
		binaryName = "TerrariaServer.exe"
	}
	wanted := filepath.Join(targetDir, "1457", platform, binaryName)
	if err := os.MkdirAll(filepath.Dir(wanted), 0755); err != nil {
		t.Fatalf("create nested vanilla directory: %v", err)
	}
	if err := os.WriteFile(wanted, []byte("server"), 0755); err != nil {
		t.Fatalf("create nested vanilla binary: %v", err)
	}

	got, err := findVanillaServerBinary(targetDir)
	if err != nil {
		t.Fatalf("find nested vanilla binary: %v", err)
	}
	if !strings.EqualFold(got, wanted) {
		t.Fatalf("vanilla binary = %q, want %q", got, wanted)
	}
}

func TestFinalizePreparedGameFilesRejectsMissingVanillaBinary(t *testing.T) {
	targetDir := t.TempDir()
	err := finalizePreparedGameFiles("vanilla", targetDir, "1457", "1.4.5.7", func(string, int) {})
	if err == nil {
		t.Fatal("expected missing vanilla binary error")
	}
	if !strings.Contains(err.Error(), "原版") {
		t.Fatalf("missing vanilla error = %q, want a vanilla-specific message", err)
	}
}
