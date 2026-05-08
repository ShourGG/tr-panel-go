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
	"time"

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
