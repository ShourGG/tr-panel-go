package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeConfigRequiresOfficialFirstRunContract(t *testing.T) {
	tshockDir := filepath.Join(t.TempDir(), "tshock")
	service := NewConfigService(tshockDir)

	err := service.InitializeConfig()
	if err == nil {
		t.Fatal("expected InitializeConfig to refuse local template initialization")
	}
	if !strings.Contains(err.Error(), "官方首次启动流程") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(tshockDir); statErr != nil {
		t.Fatalf("expected tshock directory to be created, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(tshockDir, "config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("InitializeConfig should not create config.json, stat err=%v", statErr)
	}
}
