package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"terraria-panel/config"

	"github.com/gin-gonic/gin"
)

func TestDeleteModAcceptsExactDisplayedName(t *testing.T) {
	modDir, cleanup := setupDeleteModTestData(t)
	defer cleanup()

	fullName := "live-upload-check-20260417-225702"
	if err := os.WriteFile(filepath.Join(modDir, fullName+".tmod"), []byte("mod"), 0644); err != nil {
		t.Fatalf("failed to create mod file: %v", err)
	}

	enabledFile := filepath.Join(modDir, "enabled.json")
	writeDeleteModJSONFile(t, enabledFile, []string{fullName})

	mappingFile := filepath.Join(modDir, "workshop_mapping.json")
	writeDeleteModJSONFile(t, mappingFile, map[string]ModMappingData{
		"12345": {
			ModName: fullName,
		},
	})

	recorder := executeDeleteModRequest(t, fullName)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	if _, err := os.Stat(filepath.Join(modDir, fullName+".tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected mod file to be deleted, got err=%v", err)
	}

	var enabledMods []string
	readDeleteModJSONFile(t, enabledFile, &enabledMods)
	if len(enabledMods) != 0 {
		t.Fatalf("expected enabled.json to be cleaned, got %v", enabledMods)
	}

	mapping := make(map[string]ModMappingData)
	readDeleteModJSONFile(t, mappingFile, &mapping)
	if len(mapping) != 0 {
		t.Fatalf("expected workshop mapping to be cleaned, got %v", mapping)
	}
}

func TestDeleteModKeepsLegacyExtractedNameCompatibility(t *testing.T) {
	modDir, cleanup := setupDeleteModTestData(t)
	defer cleanup()

	fullName := "ExampleMod-20260417-225702"
	if err := os.WriteFile(filepath.Join(modDir, fullName+".tmod"), []byte("mod"), 0644); err != nil {
		t.Fatalf("failed to create mod file: %v", err)
	}

	recorder := executeDeleteModRequest(t, "ExampleMod-20260417")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	if _, err := os.Stat(filepath.Join(modDir, fullName+".tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy deletion request to remove file, got err=%v", err)
	}
}

func setupDeleteModTestData(t *testing.T) (string, func()) {
	t.Helper()

	oldDataDir := config.DataDir
	dataDir := t.TempDir()
	modDir := filepath.Join(dataDir, "tModLoader", "Mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("failed to create mod dir: %v", err)
	}

	config.DataDir = dataDir

	return modDir, func() {
		config.DataDir = oldDataDir
	}
}

func executeDeleteModRequest(t *testing.T, modName string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/mods/"+modName, nil)
	ctx.Params = gin.Params{{Key: "name", Value: modName}}

	DeleteMod(ctx)
	return recorder
}

func writeDeleteModJSONFile(t *testing.T, filePath string, payload interface{}) {
	t.Helper()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal %s: %v", filePath, err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("failed to write %s: %v", filePath, err)
	}
}

func readDeleteModJSONFile(t *testing.T, filePath string, target interface{}) {
	t.Helper()

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", filePath, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("failed to unmarshal %s: %v", filePath, err)
	}
}
