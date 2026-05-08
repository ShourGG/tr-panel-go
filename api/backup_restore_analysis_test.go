package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/middleware"
	"terraria-panel/models"
	"terraria-panel/storage"
	"terraria-panel/utils"

	"github.com/gin-gonic/gin"
)

func TestAnalyzeBackupRestoreRejectsPreparingRoom(t *testing.T) {
	backupPath := createBackupRestoreTestArchive(t, "world.wld")

	analysis, err := analyzeBackupRestore(backupPath, &models.Room{
		ID:         2,
		Name:       "Target Room",
		ServerType: "vanilla",
		WorldFile:  "world.wld",
		Status:     "preparing",
	})
	if err != nil {
		t.Fatalf("expected analysis to succeed, got error: %v", err)
	}

	if analysis.CanRestore {
		t.Fatalf("expected preparing room to be rejected for restore")
	}
	if len(analysis.FatalIssues) == 0 {
		t.Fatalf("expected preparing room to produce fatal issues")
	}

	var roomStatusCheck *backupAnalysisCheck
	for i := range analysis.Checks {
		if analysis.Checks[i].Key == "roomStatus" {
			roomStatusCheck = &analysis.Checks[i]
			break
		}
	}
	if roomStatusCheck == nil {
		t.Fatalf("expected roomStatus check to exist")
	}
	if roomStatusCheck.Status != "fatal" {
		t.Fatalf("expected roomStatus check to be fatal, got %s", roomStatusCheck.Status)
	}
}

func TestAnalyzeBackupHandlerReportsRestoreEligibilityByRoomStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backupDir, cleanup := setupBackupResolvePathTest(t)
	defer cleanup()

	backupPath := filepath.Join(backupDir, "room-1_source-room_20260506_120000.zip")
	createBackupRestoreTestArchiveAtPath(t, backupPath, "world.wld")
	backupID := strings.TrimSuffix(filepath.Base(backupPath), filepath.Ext(backupPath))

	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	user := &models.User{ID: 1, Username: "tester", Role: "admin"}
	token, err := middleware.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	testCases := []struct {
		name           string
		status         string
		wantCanRestore bool
		wantCheck      string
	}{
		{name: "stopped room can restore", status: "stopped", wantCanRestore: true, wantCheck: "success"},
		{name: "preparing room is rejected", status: "preparing", wantCanRestore: false, wantCheck: "fatal"},
		{name: "running room is rejected", status: "running", wantCanRestore: false, wantCheck: "fatal"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			room := &models.Room{
				Name:       fmt.Sprintf("Room-%s", tc.status),
				ServerType: "vanilla",
				WorldFile:  "world.wld",
				Port:       7777,
				MaxPlayers: 8,
				Status:     tc.status,
			}
			if err := roomStorage.Create(room); err != nil {
				t.Fatalf("failed to create room: %v", err)
			}

			router := gin.New()
			router.Use(middleware.AuthMiddleware())
			router.POST("/api/backups/:id/analyze", AnalyzeBackup)

			body := strings.NewReader(fmt.Sprintf(`{"targetRoomId":%d}`, room.ID))
			request := httptest.NewRequest(http.MethodPost, "/api/backups/"+backupID+"/analyze", body)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("unexpected status: got %d body=%s", response.Code, response.Body.String())
			}

			var payload struct {
				Success bool                  `json:"success"`
				Data    backupRestoreAnalysis `json:"data"`
				Error   string                `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if !payload.Success {
				t.Fatalf("expected success response, got error=%s body=%s", payload.Error, response.Body.String())
			}
			if payload.Data.CanRestore != tc.wantCanRestore {
				t.Fatalf("unexpected canRestore: got %v want %v", payload.Data.CanRestore, tc.wantCanRestore)
			}
			if tc.wantCanRestore {
				if payload.Data.FatalIssues == nil {
					t.Fatalf("expected fatalIssues to encode as an empty array, got nil")
				}
				if payload.Data.Warnings == nil {
					t.Fatalf("expected warnings to encode as an empty array, got nil")
				}
			}

			var roomStatusCheck *backupAnalysisCheck
			for i := range payload.Data.Checks {
				if payload.Data.Checks[i].Key == "roomStatus" {
					roomStatusCheck = &payload.Data.Checks[i]
					break
				}
			}
			if roomStatusCheck == nil {
				t.Fatalf("expected roomStatus check to exist")
			}
			if roomStatusCheck.Status != tc.wantCheck {
				t.Fatalf("unexpected roomStatus check: got %s want %s", roomStatusCheck.Status, tc.wantCheck)
			}
		})
	}
}

func TestRestoreBackupHandlerRequiresForceForWorldMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backupDir, cleanup := setupBackupRestorePathTest(t)
	defer cleanup()

	backupPath := filepath.Join(backupDir, "room-1_source-room_20260508_120000.zip")
	createBackupRestoreTestArchiveAtPath(t, backupPath, "restored.wld")
	backupID := strings.TrimSuffix(filepath.Base(backupPath), filepath.Ext(backupPath))

	room := createBackupRestoreTestRoom(t, "stopped", "current.wld")
	roomDir := createRestoreTargetDir(t, room.ID, "current.wld", "current-world")

	router, token := newBackupRestoreTestRouter(t)
	response := performBackupRestoreRequest(t, router, token, backupID, room.ID, false)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected force confirmation conflict, got %d body=%s", response.Code, response.Body.String())
	}

	assertFileContent(t, filepath.Join(roomDir, "current.wld"), "current-world")
	assertFileMissing(t, filepath.Join(roomDir, "restored.wld"))
	refreshedRoom := getBackupRestoreTestRoom(t, room.ID)
	if refreshedRoom.WorldFile != "current.wld" {
		t.Fatalf("world file changed before force confirmation: got %s", refreshedRoom.WorldFile)
	}
}

func TestRestoreBackupHandlerRestoresStoppedRoomAndUpdatesWorldFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backupDir, cleanup := setupBackupRestorePathTest(t)
	defer cleanup()

	backupPath := filepath.Join(backupDir, "room-1_source-room_20260508_130000.zip")
	createBackupRestoreTestArchiveAtPath(t, backupPath, "restored.wld")
	backupID := strings.TrimSuffix(filepath.Base(backupPath), filepath.Ext(backupPath))

	room := createBackupRestoreTestRoom(t, "stopped", "current.wld")
	roomDir := createRestoreTargetDir(t, room.ID, "current.wld", "current-world")
	if err := os.WriteFile(filepath.Join(roomDir, "stale.txt"), []byte("stale"), 0644); err != nil {
		t.Fatalf("failed to create stale file: %v", err)
	}

	router, token := newBackupRestoreTestRouter(t)
	response := performBackupRestoreRequest(t, router, token, backupID, room.ID, true)

	if response.Code != http.StatusOK {
		t.Fatalf("expected restore success, got %d body=%s", response.Code, response.Body.String())
	}

	assertFileMissing(t, filepath.Join(roomDir, "current.wld"))
	assertFileMissing(t, filepath.Join(roomDir, "stale.txt"))
	assertFileContent(t, filepath.Join(roomDir, "restored.wld"), "world-data")

	refreshedRoom := getBackupRestoreTestRoom(t, room.ID)
	if refreshedRoom.WorldFile != "restored.wld" {
		t.Fatalf("expected restored world file to be persisted, got %s", refreshedRoom.WorldFile)
	}
}

func TestRestoreBackupHandlerRejectsRunningRoomWithoutChangingFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backupDir, cleanup := setupBackupRestorePathTest(t)
	defer cleanup()

	backupPath := filepath.Join(backupDir, "room-1_source-room_20260508_140000.zip")
	createBackupRestoreTestArchiveAtPath(t, backupPath, "restored.wld")
	backupID := strings.TrimSuffix(filepath.Base(backupPath), filepath.Ext(backupPath))

	room := createBackupRestoreTestRoom(t, "running", "current.wld")
	roomDir := createRestoreTargetDir(t, room.ID, "current.wld", "current-world")

	router, token := newBackupRestoreTestRouter(t)
	response := performBackupRestoreRequest(t, router, token, backupID, room.ID, true)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected running room to be rejected, got %d body=%s", response.Code, response.Body.String())
	}

	assertFileContent(t, filepath.Join(roomDir, "current.wld"), "current-world")
	assertFileMissing(t, filepath.Join(roomDir, "restored.wld"))
}

func createBackupRestoreTestArchive(t *testing.T, worldFile string) string {
	t.Helper()

	rootDir := t.TempDir()
	sourceDir := filepath.Join(rootDir, "room")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, worldFile), []byte("world-data"), 0644); err != nil {
		t.Fatalf("failed to create world file: %v", err)
	}

	backupPath := filepath.Join(rootDir, "backup.zip")
	manifest := utils.NewBackupManifest(1, "Source Room", "vanilla", worldFile, "full", "", time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	if err := utils.CreateBackupArchive(backupPath, sourceDir, manifest); err != nil {
		t.Fatalf("failed to create backup archive: %v", err)
	}

	return backupPath
}

func createBackupRestoreTestArchiveAtPath(t *testing.T, backupPath string, worldFile string) {
	t.Helper()

	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, worldFile), []byte("world-data"), 0644); err != nil {
		t.Fatalf("failed to create world file: %v", err)
	}

	manifest := utils.NewBackupManifest(1, "Source Room", "vanilla", worldFile, "full", "", time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	if err := utils.CreateBackupArchive(backupPath, sourceDir, manifest); err != nil {
		t.Fatalf("failed to create backup archive: %v", err)
	}
}

func setupBackupRestorePathTest(t *testing.T) (string, func()) {
	t.Helper()

	oldDataDir := config.DataDir
	backupDir, cleanupBackup := setupBackupResolvePathTest(t)
	config.DataDir = filepath.Dir(backupDir)

	return backupDir, func() {
		cleanupBackup()
		config.DataDir = oldDataDir
	}
}

func newBackupRestoreTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()

	user := &models.User{ID: 1, Username: "tester", Role: "admin"}
	token, err := middleware.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.POST("/api/backups/:id/restore", RestoreBackup)
	return router, token
}

func performBackupRestoreRequest(t *testing.T, router *gin.Engine, token string, backupID string, roomID int, force bool) *httptest.ResponseRecorder {
	t.Helper()

	body := strings.NewReader(fmt.Sprintf(`{"targetRoomId":%d,"force":%t}`, roomID, force))
	request := httptest.NewRequest(http.MethodPost, "/api/backups/"+backupID+"/restore", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func createBackupRestoreTestRoom(t *testing.T, status string, worldFile string) *models.Room {
	t.Helper()

	room := &models.Room{
		Name:       fmt.Sprintf("Room-%s-%s", status, worldFile),
		ServerType: "vanilla",
		WorldFile:  worldFile,
		Port:       7777,
		MaxPlayers: 8,
		Status:     status,
	}
	if err := storage.NewSQLiteRoomStorage(db.DB).Create(room); err != nil {
		t.Fatalf("failed to create room: %v", err)
	}
	return room
}

func getBackupRestoreTestRoom(t *testing.T, roomID int) *models.Room {
	t.Helper()

	room, err := storage.NewSQLiteRoomStorage(db.DB).GetByID(roomID)
	if err != nil {
		t.Fatalf("failed to get room: %v", err)
	}
	if room == nil {
		t.Fatalf("expected room #%d to exist", roomID)
	}
	return room
}

func createRestoreTargetDir(t *testing.T, roomID int, worldFile string, content string) string {
	t.Helper()

	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", roomID))
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		t.Fatalf("failed to create room dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, worldFile), []byte(content), 0644); err != nil {
		t.Fatalf("failed to create current world file: %v", err)
	}
	return roomDir
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()

	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
	if string(actual) != expected {
		t.Fatalf("unexpected file content for %s: got %q want %q", path, string(actual), expected)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file %s to be missing", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
}
