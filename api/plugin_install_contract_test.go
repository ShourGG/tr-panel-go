package api

import (
	"os"
	"path/filepath"
	"testing"

	"terraria-panel/config"
)

func TestFindPluginDLLRecursesThroughPackageDirectories(t *testing.T) {
	extractDir := t.TempDir()
	wanted := filepath.Join(extractDir, "Plugins", "TransferPatch.dll")
	if err := os.MkdirAll(filepath.Dir(wanted), 0755); err != nil {
		t.Fatalf("create plugin package directory: %v", err)
	}
	if err := os.WriteFile(wanted, []byte("plugin"), 0644); err != nil {
		t.Fatalf("create plugin DLL: %v", err)
	}

	got, err := findPluginDLL(extractDir, "transferpatch")
	if err != nil {
		t.Fatalf("find nested plugin DLL: %v", err)
	}
	if got != wanted {
		t.Fatalf("plugin DLL = %q, want %q", got, wanted)
	}
}

func TestRoomPluginDirectoryMatchesRoomStartupDirectory(t *testing.T) {
	oldDataDir := config.DataDir
	t.Cleanup(func() { config.DataDir = oldDataDir })
	config.DataDir = filepath.Join("data-root")

	want := filepath.Join(config.DataDir, "rooms", "room-17", "tshock", "ServerPlugins")
	if got := getPluginsDir(17); got != want {
		t.Fatalf("room plugin directory = %q, want %q", got, want)
	}
}

func TestFindPluginDLLReportsMissingPlugin(t *testing.T) {
	_, err := findPluginDLL(t.TempDir(), "MissingPlugin")
	if err == nil {
		t.Fatal("expected missing plugin error")
	}
}
