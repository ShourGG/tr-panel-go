package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"terraria-panel/config"
	"terraria-panel/models"

	"github.com/gin-gonic/gin"
)

func setupInstalledTShockForPluginCompatibilityTest(t *testing.T, major string) string {
	t.Helper()
	oldServersDir := config.ServersDir
	oldRuntimeCheck := tshockRuntimeInstalled
	serversDir := filepath.Join(t.TempDir(), "servers")
	tshockDir := filepath.Join(serversDir, "tshock")
	if err := os.MkdirAll(tshockDir, 0755); err != nil {
		t.Fatalf("create TShock dir: %v", err)
	}
	version := major + ".1.0"
	if err := os.WriteFile(filepath.Join(tshockDir, "TShock.Server"), []byte("server"), 0755); err != nil {
		t.Fatalf("write core file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tshockDir, ".tshock_version"), []byte(version), 0644); err != nil {
		t.Fatalf("write version marker: %v", err)
	}
	if err := writeTShockInstallCompleteMarker(tshockDir, version); err != nil {
		t.Fatalf("write completion marker: %v", err)
	}
	config.ServersDir = serversDir
	tshockRuntimeInstalled = func(string) bool { return true }
	t.Cleanup(func() {
		config.ServersDir = oldServersDir
		tshockRuntimeInstalled = oldRuntimeCheck
	})
	return tshockDir
}

func TestPluginFrameworkCompatibilityRejectsNet9ForTShock5AndAcceptsNet6(t *testing.T) {
	setupInstalledTShockForPluginCompatibilityTest(t, "5")

	net9 := []byte("MZ fake metadata TargetFramework NETCoreApp,Version=v9.0 System.Runtime, Version=9.0")
	err := validatePluginBytesForCurrentTShock(net9, "Net9Plugin.dll")
	if err == nil || !strings.Contains(err.Error(), ".NET 9") || !strings.Contains(err.Error(), "TShock 5") {
		t.Fatalf("expected clear TShock 5/.NET 9 compatibility error, got %v", err)
	}

	net6 := []byte("MZ fake metadata TargetFramework NETCoreApp,Version=v6.0 System.Runtime, Version=6.0")
	if err := validatePluginBytesForCurrentTShock(net6, "Net6Plugin.dll"); err != nil {
		t.Fatalf("expected .NET 6 plugin to be accepted for TShock 5: %v", err)
	}
}

func TestPluginFrameworkCompatibilityRequiresKnownNet9ForTShock6(t *testing.T) {
	setupInstalledTShockForPluginCompatibilityTest(t, "6")

	if err := validatePluginBytesForCurrentTShock([]byte("MZ no framework metadata"), "Unknown.dll"); err == nil || !strings.Contains(err.Error(), "TShock 6") || !strings.Contains(err.Error(), "无法识别") {
		t.Fatalf("expected clear unknown framework error for TShock 6, got %v", err)
	}
	if err := validatePluginBytesForCurrentTShock([]byte("MZ TargetFramework net9.0"), "Net9Plugin.dll"); err != nil {
		t.Fatalf("expected .NET 9 plugin to be accepted for TShock 6: %v", err)
	}
}

func TestAddRoomPluginValidatesFrameworkBeforeWritingFile(t *testing.T) {
	setupLivePlayerStatsContractDB(t)
	setupInstalledTShockForPluginCompatibilityTest(t, "5")
	oldDataDir := config.DataDir
	config.DataDir = t.TempDir()
	t.Cleanup(func() { config.DataDir = oldDataDir })

	room := &models.Room{
		Name:       "plugins-room",
		ServerType: "tshock",
		WorldFile:  "plugins-room.wld",
		Port:       17800,
		MaxPlayers: 8,
		Status:     "stopped",
	}
	if err := roomStorage.Create(room); err != nil {
		t.Fatalf("create test room: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/rooms/:id/plugins", AddRoomPlugin)
	pluginDir := filepath.Join(config.DataDir, "rooms", "room-"+strconv.Itoa(room.ID), "tshock", "ServerPlugins")

	response := submitPluginUpload(router, "/rooms/"+strconv.Itoa(room.ID)+"/plugins", "Rejected.dll", []byte("MZ NETCoreApp,Version=v9.0"))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "不兼容") {
		t.Fatalf("expected incompatible upload rejection, got %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "Rejected.dll")); !os.IsNotExist(err) {
		t.Fatalf("incompatible plugin was written before rejection, stat err=%v", err)
	}

	response = submitPluginUpload(router, "/rooms/"+strconv.Itoa(room.ID)+"/plugins", "Accepted.dll", []byte("MZ NETCoreApp,Version=v6.0"))
	if response.Code != http.StatusOK {
		t.Fatalf("expected compatible upload success, got %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "Accepted.dll")); err != nil {
		t.Fatalf("compatible plugin was not written: %v", err)
	}

	if err := validateEnabledTShockPlugins(pluginDir); err != nil {
		t.Fatalf("enabled compatible plugin did not pass startup preflight: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "Injected.dll"), []byte("MZ NETCoreApp,Version=v9.0"), 0644); err != nil {
		t.Fatalf("write incompatible plugin for preflight: %v", err)
	}
	if err := validateEnabledTShockPlugins(pluginDir); err == nil || !strings.Contains(err.Error(), "Injected.dll") {
		t.Fatalf("startup preflight must reject incompatible enabled plugin, got %v", err)
	}
}

func submitPluginUpload(router http.Handler, target, filename string, content []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write(content)
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
