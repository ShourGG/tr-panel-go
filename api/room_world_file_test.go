package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/storage"
	"terraria-panel/utils"

	"github.com/gin-gonic/gin"
)

func TestCreateRoomDefaultsWorldFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupRoomWorldFileTest(t)
	defer cleanup()

	router := gin.New()
	router.POST("/api/rooms", CreateRoom)

	body := strings.NewReader(`{"name":"Room A","serverType":"vanilla","port":7777,"maxPlayers":8}`)
	request := httptest.NewRequest(http.MethodPost, "/api/rooms", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected room creation to succeed, got %d body=%s", response.Code, response.Body.String())
	}

	rooms, err := storage.NewSQLiteRoomStorage(db.DB).GetAll()
	if err != nil {
		t.Fatalf("failed to get rooms: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected one room, got %d", len(rooms))
	}
	if rooms[0].WorldFile != ".wld" {
		t.Fatalf("expected default world file .wld, got %q", rooms[0].WorldFile)
	}
}

func TestUpdateRoomPreservesExistingWorldFileWhenPayloadOmitsIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupRoomWorldFileTest(t)
	defer cleanup()

	room := &models.Room{
		Name:       "Room A",
		ServerType: "vanilla",
		WorldFile:  "current.wld",
		Port:       7777,
		MaxPlayers: 8,
		Status:     "stopped",
	}
	roomStore := storage.NewSQLiteRoomStorage(db.DB)
	if err := roomStore.Create(room); err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	router := gin.New()
	router.PUT("/api/rooms/:id", UpdateRoom)

	body := strings.NewReader(`{"name":"Room A Updated","serverType":"vanilla","port":7778,"maxPlayers":12}`)
	request := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/rooms/%d", room.ID), body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected room update to succeed, got %d body=%s", response.Code, response.Body.String())
	}

	updatedRoom, err := roomStore.GetByID(room.ID)
	if err != nil {
		t.Fatalf("failed to get updated room: %v", err)
	}
	if updatedRoom.WorldFile != "current.wld" {
		t.Fatalf("expected world file to be preserved, got %q", updatedRoom.WorldFile)
	}
}

func TestCreateBackupUsesDetectedWorldFileWhenRoomConfigIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupRoomWorldFileTest(t)
	defer cleanup()

	room := &models.Room{
		Name:       "Room A",
		ServerType: "vanilla",
		WorldFile:  "",
		Port:       7777,
		MaxPlayers: 8,
		Status:     "stopped",
	}
	roomStore := storage.NewSQLiteRoomStorage(db.DB)
	if err := roomStore.Create(room); err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		t.Fatalf("failed to create room dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "detected.wld"), []byte("world-data"), 0644); err != nil {
		t.Fatalf("failed to create world file: %v", err)
	}

	router := gin.New()
	router.POST("/api/backups", CreateBackup)

	body := strings.NewReader(fmt.Sprintf(`{"roomId":%d,"type":"full"}`, room.ID))
	request := httptest.NewRequest(http.MethodPost, "/api/backups", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected backup creation to succeed, got %d body=%s", response.Code, response.Body.String())
	}

	refreshedRoom, err := roomStore.GetByID(room.ID)
	if err != nil {
		t.Fatalf("failed to get refreshed room: %v", err)
	}
	if refreshedRoom.WorldFile != "detected.wld" {
		t.Fatalf("expected detected world file to be persisted, got %q", refreshedRoom.WorldFile)
	}

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Backup utils.BackupSummary `json:"backup"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("expected success response, got error=%s", payload.Error)
	}
	if payload.Data.Backup.WorldFile != "detected.wld" {
		t.Fatalf("expected backup manifest world file detected.wld, got %q", payload.Data.Backup.WorldFile)
	}
}

func TestAnalyzeBackupRestoreNormalizesEmptyTargetWorldFile(t *testing.T) {
	backupPath := createBackupRestoreTestArchive(t, ".wld")

	analysis, err := analyzeBackupRestore(backupPath, &models.Room{
		ID:         2,
		Name:       "Target Room",
		ServerType: "vanilla",
		WorldFile:  "",
		Status:     "stopped",
	})
	if err != nil {
		t.Fatalf("expected analysis to succeed, got error: %v", err)
	}

	if analysis.TargetRoom.WorldFile != ".wld" {
		t.Fatalf("expected target room world file to be normalized, got %q", analysis.TargetRoom.WorldFile)
	}

	var worldFileCheck *backupAnalysisCheck
	for i := range analysis.Checks {
		if analysis.Checks[i].Key == "worldFile" {
			worldFileCheck = &analysis.Checks[i]
			break
		}
	}
	if worldFileCheck == nil {
		t.Fatalf("expected worldFile check to exist")
	}
	if worldFileCheck.Status != "success" {
		t.Fatalf("expected worldFile check success, got %s: %s", worldFileCheck.Status, worldFileCheck.Detail)
	}
}

func setupRoomWorldFileTest(t *testing.T) func() {
	t.Helper()

	oldRoomStorage := roomStorage
	oldDataDir := config.DataDir
	backupDir, cleanupBackup := setupBackupResolvePathTest(t)
	config.DataDir = filepath.Dir(backupDir)
	roomStorage = storage.NewSQLiteRoomStorage(db.DB)

	return func() {
		roomStorage = oldRoomStorage
		config.DataDir = oldDataDir
		cleanupBackup()
	}
}
