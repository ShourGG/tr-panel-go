package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"terraria-panel/config"
)

func TestCheckGameInstalledRecognizesLinuxTShockServer(t *testing.T) {
	oldServersDir := config.ServersDir
	oldRuntimeCheck := tshockRuntimeInstalled
	t.Cleanup(func() {
		config.ServersDir = oldServersDir
		tshockRuntimeInstalled = oldRuntimeCheck
	})

	config.ServersDir = t.TempDir()
	tshockDir := filepath.Join(config.ServersDir, "tshock")
	if err := os.MkdirAll(tshockDir, 0755); err != nil {
		t.Fatalf("create TShock directory: %v", err)
	}
	writeTShockCoreFiles(t, tshockDir)
	if err := os.WriteFile(filepath.Join(tshockDir, ".tshock_version"), []byte("5.2.4\n"), 0644); err != nil {
		t.Fatalf("create TShock version marker: %v", err)
	}
	if err := writeTShockInstallCompleteMarker(tshockDir, "5.2.4"); err != nil {
		t.Fatalf("create TShock completion marker: %v", err)
	}
	tshockRuntimeInstalled = func(string) bool { return true }

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/game/check", CheckGameInstalled)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/game/check", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("check game status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data struct {
			TShock bool `json:"tshock"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode check game response: %v", err)
	}
	if !payload.Data.TShock {
		t.Fatalf("TShock.Server was not recognized by /api/game/check: %s", response.Body.String())
	}
}
