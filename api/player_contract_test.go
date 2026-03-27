package api

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraria-panel/models"

	"github.com/gin-gonic/gin"
)

type fakeTaskStorage struct {
	getByIDTask *models.ScheduledTask
	getByIDErr  error
	createErr   error
	updateErr   error

	getByIDCalls int
	createCalls  int
	updateCalls  int
}

func (f *fakeTaskStorage) GetAll() ([]models.ScheduledTask, error) {
	return nil, nil
}

func (f *fakeTaskStorage) GetByID(id int) (*models.ScheduledTask, error) {
	f.getByIDCalls++
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	if f.getByIDTask == nil {
		return nil, nil
	}
	copy := *f.getByIDTask
	return &copy, nil
}

func (f *fakeTaskStorage) GetEnabled() ([]models.ScheduledTask, error) {
	return nil, nil
}

func (f *fakeTaskStorage) Create(task *models.ScheduledTask) error {
	f.createCalls++
	return f.createErr
}

func (f *fakeTaskStorage) Update(task *models.ScheduledTask) error {
	f.updateCalls++
	return f.updateErr
}

func (f *fakeTaskStorage) Delete(id int) error {
	return nil
}

func (f *fakeTaskStorage) CreateLog(log *models.TaskExecutionLog) error {
	return nil
}

func (f *fakeTaskStorage) GetLogs(taskID int, limit int) ([]models.TaskExecutionLog, error) {
	return nil, nil
}

func (f *fakeTaskStorage) DeleteLogs(taskID int) error {
	return nil
}

func TestAllowedTaskTypesContract(t *testing.T) {
	expected := map[string]struct{}{
		"backup":         {},
		"restart":        {},
		"cleanup_backup": {},
		"cleanup_log":    {},
		"broadcast":      {},
		"custom_command": {},
	}

	if !maps.Equal(allowedTaskTypes, expected) {
		t.Fatalf("allowed task types changed: got %#v want %#v", allowedTaskTypes, expected)
	}
}

func TestCreateTaskRejectsInvalidType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storage := &fakeTaskStorage{}
	restoreTaskGlobals(t, storage)

	router := gin.New()
	router.POST("/tasks", CreateTask)

	response := performTaskRequest(t, router, http.MethodPost, "/tasks", map[string]any{
		"name":           "nightly",
		"type":           "invalid_type",
		"cronExpression": "0 0 * * *",
	})

	assertTaskErrorResponse(t, response, http.StatusBadRequest, "任务类型")

	if storage.createCalls != 0 {
		t.Fatalf("CreateTask should stop before touching storage, createCalls=%d", storage.createCalls)
	}
}

func TestUpdateTaskRejectsInvalidTypeBeforeUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storage := &fakeTaskStorage{
		getByIDTask: &models.ScheduledTask{
			ID:   42,
			Name: "nightly",
			Type: "backup",
		},
	}
	restoreTaskGlobals(t, storage)

	router := gin.New()
	router.PUT("/tasks/:id", UpdateTask)

	response := performTaskRequest(t, router, http.MethodPut, "/tasks/42", map[string]any{
		"type": "invalid_type",
	})

	assertTaskErrorResponse(t, response, http.StatusBadRequest, "任务类型")

	if storage.getByIDCalls != 1 {
		t.Fatalf("UpdateTask should fetch the task once before validating type, getByIDCalls=%d", storage.getByIDCalls)
	}
	if storage.updateCalls != 0 {
		t.Fatalf("UpdateTask should not persist invalid task types, updateCalls=%d", storage.updateCalls)
	}
}

func restoreTaskGlobals(t *testing.T, storage *fakeTaskStorage) {
	t.Helper()

	oldTaskStorage := taskStorage
	oldTaskScheduler := taskScheduler
	taskStorage = storage
	taskScheduler = nil
	t.Cleanup(func() {
		taskStorage = oldTaskStorage
		taskScheduler = oldTaskScheduler
	})
}

func performTaskRequest(t *testing.T, router http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func assertTaskErrorResponse(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantErrorFragment string) {
	t.Helper()

	if response.Code != wantStatus {
		t.Fatalf("unexpected status: got %d want %d, body=%s", response.Code, wantStatus, response.Body.String())
	}

	var payload models.Response
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Success {
		t.Fatalf("expected success=false, body=%s", response.Body.String())
	}
	if !strings.Contains(payload.Error, wantErrorFragment) {
		t.Fatalf("unexpected error message: got %q want fragment %q", payload.Error, wantErrorFragment)
	}
}
