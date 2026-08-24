package api

import (
	"os"
	"path/filepath"
	"terraria-panel/config"
	"testing"
)

func TestTModLoaderInstallDetectionIgnoresDownloadResidue(t *testing.T) {
	oldServersDir := config.ServersDir
	serversDir := filepath.Join(t.TempDir(), "servers")
	tmodDir := filepath.Join(serversDir, "tModLoader")
	if err := os.MkdirAll(tmodDir, 0755); err != nil {
		t.Fatalf("create tModLoader directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmodDir, "download.tmp"), []byte("partial"), 0644); err != nil {
		t.Fatalf("write partial download: %v", err)
	}

	config.ServersDir = serversDir
	t.Cleanup(func() { config.ServersDir = oldServersDir })

	if checkTModLoaderInstalled() {
		t.Fatal("a partial download must not be reported as an installed tModLoader server")
	}

	if err := os.WriteFile(filepath.Join(tmodDir, "tModLoader.dll"), []byte("core"), 0644); err != nil {
		t.Fatalf("write tModLoader core: %v", err)
	}
	if !checkTModLoaderInstalled() {
		t.Fatal("a tModLoader core file should be reported as installed")
	}
}
