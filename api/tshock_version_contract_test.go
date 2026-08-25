package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDetectInstalledTShockVersionUsesOfficialMarkersContract(t *testing.T) {
	tshockDir := t.TempDir()
	markerPath := filepath.Join(tshockDir, ".tshock_version")
	if err := os.WriteFile(markerPath, []byte("v6.1.0\n"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	detection := detectInstalledTShockVersion(tshockDir)
	if detection.Version != "6" {
		t.Fatalf("unexpected major version: got %q want %q", detection.Version, "6")
	}
	if detection.RawVersion != "v6.1.0" {
		t.Fatalf("unexpected raw version: got %q want %q", detection.RawVersion, "v6.1.0")
	}
	if !detection.Detected {
		t.Fatalf("expected detected=true, got false")
	}
	if !strings.Contains(detection.Message, "版本标记文件") {
		t.Fatalf("unexpected message: %q", detection.Message)
	}
}

func TestDetectInstalledTShockVersionIgnoresConfigKeyHeuristicsContract(t *testing.T) {
	tshockDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tshockDir, "TShock.Server.dll"), []byte("placeholder"), 0644); err != nil {
		t.Fatalf("write binary marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tshockDir, "config.json"), []byte(`{"Settings":{"服务器端口":7777,"ServerPort":7777}}`), 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	detection := detectInstalledTShockVersion(tshockDir)
	if detection.Version != "unknown" {
		t.Fatalf("config key heuristics should not determine version, got %q", detection.Version)
	}
	if detection.Detected {
		t.Fatalf("config key heuristics should not mark version as detected")
	}
	if !strings.Contains(detection.Message, "缺少官方版本标记") {
		t.Fatalf("unexpected message: %q", detection.Message)
	}
}

func TestIncompleteTShockFilesAreNotReportedAsInstalledContract(t *testing.T) {
	oldServersDir := config.ServersDir
	oldRuntimeCheck := tshockRuntimeInstalled
	serversDir := filepath.Join(t.TempDir(), "servers")
	tshockDir := filepath.Join(serversDir, "tshock")
	if err := os.MkdirAll(tshockDir, 0755); err != nil {
		t.Fatalf("create tshock directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tshockDir, "TShock.Server"), []byte("partial"), 0755); err != nil {
		t.Fatalf("write core file: %v", err)
	}
	config.ServersDir = serversDir
	tshockRuntimeInstalled = func(string) bool { return true }
	t.Cleanup(func() {
		config.ServersDir = oldServersDir
		tshockRuntimeInstalled = oldRuntimeCheck
	})

	detection := inspectTShockInstallation(tshockDir)
	if detection.State != "incomplete" || detection.Installed || detection.Complete || detection.Version != "unknown" {
		t.Fatalf("unexpected incomplete installation detection: %#v", detection)
	}
	if !strings.Contains(detection.Message, "ServerPlugins/TShockAPI.dll") || !strings.Contains(detection.Message, "ServerPlugins/TShockAPI.deps.json") {
		t.Fatalf("incomplete installation message must list missing core files: %q", detection.Message)
	}
	if checkTShockInstalled() {
		t.Fatal("a core-file residue must not be reported as an installed TShock server")
	}
	if isGameInstalledForType("tshock5") || isGameInstalledForType("tshock6") {
		t.Fatal("an unverified TShock residue must not block either TShock install card")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/tshock-version", DetectTShockVersion)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tshock-version", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected detection response: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			InstallationState string `json:"installationState"`
			Installed         bool   `json:"installed"`
			Message           string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode detection response: %v", err)
	}
	if !payload.Success || payload.Data.InstallationState != "incomplete" || payload.Data.Installed || !strings.Contains(payload.Data.Message, "ServerPlugins/TShockAPI.dll") {
		t.Fatalf("unexpected detection payload: %#v", payload)
	}
}

func TestTShockCoreValidationReportsEachMissingOfficialPluginFile(t *testing.T) {
	tshockDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tshockDir, "ServerPlugins"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("write invalid plugin path: %v", err)
	}
	missing := missingTShockCoreFiles(tshockDir)
	if len(missing) != len(tshockRequiredCoreFiles) {
		t.Fatalf("invalid plugin directory should report both files, got %v", missing)
	}

	if err := os.Remove(filepath.Join(tshockDir, "ServerPlugins")); err != nil {
		t.Fatalf("remove invalid plugin path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tshockDir, "ServerPlugins"), 0755); err != nil {
		t.Fatalf("create plugin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tshockDir, "ServerPlugins", "TShockAPI.dll"), []byte("api"), 0644); err != nil {
		t.Fatalf("write API plugin: %v", err)
	}
	missing = missingTShockCoreFiles(tshockDir)
	if len(missing) != 1 || missing[0] != "ServerPlugins/TShockAPI.deps.json" {
		t.Fatalf("missing deps file was not reported precisely: %v", missing)
	}
	if err := validateTShockCoreFiles(tshockDir); err == nil || !strings.Contains(err.Error(), "ServerPlugins/TShockAPI.deps.json") {
		t.Fatalf("expected clear missing deps validation error, got %v", err)
	}

	if err := os.WriteFile(filepath.Join(tshockDir, "ServerPlugins", "TShockAPI.deps.json"), []byte(`{"runtimeTarget":{}}`), 0644); err != nil {
		t.Fatalf("write deps file: %v", err)
	}
	if err := validateTShockCoreFiles(tshockDir); err != nil {
		t.Fatalf("complete TShock core files should validate: %v", err)
	}
}

func TestTShockCompletionMarkerRequiresCoreVersionAndRuntimeContract(t *testing.T) {
	tshockDir := t.TempDir()
	oldRuntimeCheck := tshockRuntimeInstalled
	tshockRuntimeInstalled = func(required string) bool { return required == "9.0" }
	t.Cleanup(func() { tshockRuntimeInstalled = oldRuntimeCheck })

	writeTShockCoreFiles(t, tshockDir)
	if err := os.WriteFile(filepath.Join(tshockDir, ".tshock_version"), []byte("6.1.0\n"), 0644); err != nil {
		t.Fatalf("write version marker: %v", err)
	}
	if err := writeTShockInstallCompleteMarker(tshockDir, "6.1.0"); err != nil {
		t.Fatalf("write completion marker: %v", err)
	}

	detection := inspectTShockInstallation(tshockDir)
	if detection.State != "installed" || !detection.Installed || !detection.Complete || !detection.RuntimeReady || detection.Version != "6" {
		t.Fatalf("unexpected completed installation detection: %#v", detection)
	}

	tshockRuntimeInstalled = func(string) bool { return false }
	detection = inspectTShockInstallation(tshockDir)
	if detection.State != "runtime-missing" || detection.Installed || detection.RuntimeReady {
		t.Fatalf("missing runtime must not report installed: %#v", detection)
	}
}

func TestTShockInstallationForTargetMarksOtherMajorAsConflict(t *testing.T) {
	detection := tshockInstallationDetection{
		State:           "installed",
		Version:         "6",
		RawVersion:      "6.1.0",
		Installed:       true,
		Complete:        true,
		RuntimeReady:    true,
		VersionDetected: true,
		Message:         "检测到 TShock 6",
	}

	for _, target := range []string{"5", "6"} {
		projected := tshockInstallationForTarget(detection, target)
		if target == "5" {
			if projected.State != "conflict" || projected.Installed || projected.Complete {
				t.Fatalf("other major must be a non-installed conflict: %#v", projected)
			}
			if !strings.Contains(projected.Message, "只能安装一个") {
				t.Fatalf("conflict message is not actionable: %q", projected.Message)
			}
			continue
		}
		if projected.State != "installed" || !projected.Installed || !projected.Complete {
			t.Fatalf("matching major must retain installed state: %#v", projected)
		}
	}
}

func TestInitializePluginServerConfigRequiresOfficialFirstRunContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldConfigService := configService
	oldServersDir := config.ServersDir

	serversDir := filepath.Join(t.TempDir(), "servers")
	tshockDir := filepath.Join(serversDir, "tshock")
	if err := os.MkdirAll(tshockDir, 0755); err != nil {
		t.Fatalf("mkdir tshock dir: %v", err)
	}
	config.ServersDir = serversDir
	InitConfigService(tshockDir)

	t.Cleanup(func() {
		configService = oldConfigService
		config.ServersDir = oldServersDir
	})

	router := gin.New()
	router.POST("/plugin-server/tshock-config/initialize", InitializePluginServerConfig)

	request := httptest.NewRequest(http.MethodPost, "/plugin-server/tshock-config/initialize", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected status: got %d want %d, body=%s", response.Code, http.StatusConflict, response.Body.String())
	}

	var payload struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Success {
		t.Fatalf("expected success=false, body=%s", response.Body.String())
	}
	if payload.Code != "OFFICIAL_FIRST_RUN_REQUIRED" {
		t.Fatalf("unexpected code: got %q want %q", payload.Code, "OFFICIAL_FIRST_RUN_REQUIRED")
	}
	if !strings.Contains(payload.Error, "官方首次启动流程") {
		t.Fatalf("unexpected error: %q", payload.Error)
	}
	if _, err := os.Stat(filepath.Join(tshockDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("initialize endpoint should not create local config.json, stat err=%v", err)
	}
}
