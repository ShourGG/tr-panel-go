package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUninstallGameRejectsMismatchedTShockMajorWithoutDeletingDirectory(t *testing.T) {
	oldServersDir := config.ServersDir
	oldRuntimeCheck := tshockRuntimeInstalled
	serversDir := filepath.Join(t.TempDir(), "servers")
	tshockDir := filepath.Join(serversDir, "tshock")
	if err := os.MkdirAll(tshockDir, 0755); err != nil {
		t.Fatalf("create tshock directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tshockDir, "TShock.Server"), []byte("server"), 0755); err != nil {
		t.Fatalf("write tshock core: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tshockDir, ".tshock_version"), []byte("6.1.0\n"), 0644); err != nil {
		t.Fatalf("write tshock version: %v", err)
	}
	if err := writeTShockInstallCompleteMarker(tshockDir, "6.1.0"); err != nil {
		t.Fatalf("write tshock completion marker: %v", err)
	}

	config.ServersDir = serversDir
	tshockRuntimeInstalled = func(string) bool { return true }
	t.Cleanup(func() {
		config.ServersDir = oldServersDir
		tshockRuntimeInstalled = oldRuntimeCheck
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/game/uninstall", UninstallGame)
	request := httptest.NewRequest(http.MethodPost, "/game/uninstall", strings.NewReader(`{"gameType":"tshock5","mode":"full"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected status: got %d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(tshockDir, "TShock.Server")); err != nil {
		t.Fatalf("mismatched uninstall deleted the shared directory: %v", err)
	}
	if !strings.Contains(response.Body.String(), "TShock 6") {
		t.Fatalf("response should identify the installed major: %s", response.Body.String())
	}
}
