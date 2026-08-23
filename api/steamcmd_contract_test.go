package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"terraria-panel/config"
)

func TestSteamCMDStateRequiresSuccessfulInitializationMarker(t *testing.T) {
	oldDataDir := config.DataDir
	t.Cleanup(func() { config.DataDir = oldDataDir })
	config.DataDir = t.TempDir()

	_, launcherPath, runtimePath := getSteamCMDPaths()
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0755); err != nil {
		t.Fatalf("create SteamCMD directory: %v", err)
	}
	if err := os.WriteFile(launcherPath, []byte("launcher"), 0755); err != nil {
		t.Fatalf("create SteamCMD launcher: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(filepath.Dir(runtimePath), 0755); err != nil {
			t.Fatalf("create SteamCMD runtime directory: %v", err)
		}
		if err := os.WriteFile(runtimePath, []byte("runtime"), 0755); err != nil {
			t.Fatalf("create SteamCMD runtime: %v", err)
		}
	}

	installed, ready, _, _, message := getSteamCMDState()
	if !installed {
		t.Fatal("SteamCMD should be reported as installed when the launcher exists")
	}
	if ready {
		t.Fatal("SteamCMD must not be ready before successful initialization marker exists")
	}
	if !strings.Contains(message, "初始化") {
		t.Fatalf("state message = %q, want initialization guidance", message)
	}

	if err := markSteamCMDReady(); err != nil {
		t.Fatalf("write SteamCMD ready marker: %v", err)
	}
	installed, ready, _, _, message = getSteamCMDState()
	if !installed || !ready {
		t.Fatalf("SteamCMD state after marker = installed:%v ready:%v message:%q", installed, ready, message)
	}
}
