package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setActiveGameTasksForTest(t *testing.T, tasks map[string]ActiveGameTask) {
	t.Helper()

	activeGameTasks.Lock()
	previous := make(map[string]ActiveGameTask, len(activeGameTasks.tasks))
	for gameType, task := range activeGameTasks.tasks {
		previous[gameType] = task
	}
	activeGameTasks.tasks = make(map[string]ActiveGameTask, len(tasks))
	for gameType, task := range tasks {
		activeGameTasks.tasks[gameType] = task
	}
	activeGameTasks.Unlock()

	t.Cleanup(func() {
		activeGameTasks.Lock()
		activeGameTasks.tasks = previous
		activeGameTasks.Unlock()
	})
}

func TestGetInstallProgressNormalizesLegacyTShockAndIncludesActiveTasks(t *testing.T) {
	setActiveGameTasksForTest(t, map[string]ActiveGameTask{
		"vanilla": {
			GameType:  "vanilla",
			Action:    "install",
			Progress:  42,
			Message:   "正在下载原版服务端",
			StartedAt: "2026-08-24T00:00:00Z",
			UpdatedAt: "2026-08-24T00:01:00Z",
		},
		"tshock5": {
			GameType:  "tshock5",
			Action:    "repair",
			Progress:  68,
			Message:   "正在修复 .NET 6.0",
			StartedAt: "2026-08-24T00:00:00Z",
			UpdatedAt: "2026-08-24T00:01:00Z",
		},
	})

	router := gin.New()
	router.GET("/progress", GetInstallProgress)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/progress?gameType=tshock", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GetInstallProgress status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Active      bool                      `json:"active"`
			GameType    string                    `json:"gameType"`
			Action      string                    `json:"action"`
			Status      string                    `json:"status"`
			Progress    int                       `json:"progress"`
			Message     string                    `json:"message"`
			ActiveCount int                       `json:"activeCount"`
			ActiveTasks map[string]ActiveGameTask `json:"activeTasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode GetInstallProgress response: %v", err)
	}

	if !response.Success || !response.Data.Active {
		t.Fatalf("expected an active success response, got %#v", response)
	}
	if response.Data.GameType != "tshock5" {
		t.Fatalf("gameType = %q, want normalized tshock5", response.Data.GameType)
	}
	if response.Data.Action != "repair" || response.Data.Status != "repairing" {
		t.Fatalf("task state = action=%q status=%q, want repair/repairing", response.Data.Action, response.Data.Status)
	}
	if response.Data.Progress != 68 || response.Data.Message != "正在修复 .NET 6.0" {
		t.Fatalf("unexpected selected task payload: %#v", response.Data)
	}
	if response.Data.ActiveCount != 2 || len(response.Data.ActiveTasks) != 2 {
		t.Fatalf("active task list = count %d, len %d; want 2", response.Data.ActiveCount, len(response.Data.ActiveTasks))
	}
	if response.Data.ActiveTasks["vanilla"].Progress != 42 {
		t.Fatalf("vanilla task was missing from activeTasks: %#v", response.Data.ActiveTasks)
	}
}

func TestGetInstallProgressReportsIdleStateWhenNoTaskExists(t *testing.T) {
	setActiveGameTasksForTest(t, map[string]ActiveGameTask{})

	router := gin.New()
	router.GET("/progress", GetInstallProgress)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/progress?gameType=vanilla", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GetInstallProgress status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Active      bool                      `json:"active"`
			GameType    string                    `json:"gameType"`
			Status      string                    `json:"status"`
			ActiveCount int                       `json:"activeCount"`
			ActiveTasks map[string]ActiveGameTask `json:"activeTasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode idle GetInstallProgress response: %v", err)
	}

	if !response.Success || response.Data.Active || response.Data.Status != "idle" {
		t.Fatalf("expected idle success response, got %#v", response)
	}
	if response.Data.GameType != "vanilla" || response.Data.ActiveCount != 0 || len(response.Data.ActiveTasks) != 0 {
		t.Fatalf("unexpected idle response payload: %#v", response.Data)
	}
}
