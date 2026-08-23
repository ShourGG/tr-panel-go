package api

import (
	"os"
	"path/filepath"
	"testing"

	"terraria-panel/config"
)

func TestRemoveGeneratedRoomConfigs(t *testing.T) {
	oldDataDir := config.DataDir
	t.Cleanup(func() {
		config.DataDir = oldDataDir
	})

	config.DataDir = t.TempDir()
	configDir := filepath.Join(config.DataDir, "configs")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}

	roomID := 42
	generatedPaths := []string{
		filepath.Join(configDir, "room-42-config.txt"),
		filepath.Join(configDir, "room-42-tshock.properties"),
	}
	for _, path := range generatedPaths {
		if err := os.WriteFile(path, []byte("generated"), 0644); err != nil {
			t.Fatalf("create generated config %s: %v", path, err)
		}
	}
	unrelatedPath := filepath.Join(configDir, "room-43-config.txt")
	if err := os.WriteFile(unrelatedPath, []byte("keep"), 0644); err != nil {
		t.Fatalf("create unrelated config: %v", err)
	}

	removeGeneratedRoomConfigs(roomID)
	removeGeneratedRoomConfigs(roomID)

	for _, path := range generatedPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("generated config still exists or stat failed for %s: %v", path, err)
		}
	}
	if content, err := os.ReadFile(unrelatedPath); err != nil {
		t.Fatalf("unrelated config was removed: %v", err)
	} else if string(content) != "keep" {
		t.Fatalf("unrelated config changed: %q", content)
	}
}
