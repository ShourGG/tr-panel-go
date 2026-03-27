package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraria-panel/models"

	"github.com/gin-gonic/gin"
)

func TestPlayerActionsRejectInvalidIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name    string
		method  string
		route   string
		path    string
		handler gin.HandlerFunc
	}{
		{name: "kick non numeric id", method: http.MethodPost, route: "/players/:id/kick", path: "/players/abc/kick", handler: KickPlayer},
		{name: "kick zero id", method: http.MethodPost, route: "/players/:id/kick", path: "/players/0/kick", handler: KickPlayer},
		{name: "ban non numeric id", method: http.MethodPost, route: "/players/:id/ban", path: "/players/abc/ban", handler: BanPlayer},
		{name: "ban zero id", method: http.MethodPost, route: "/players/:id/ban", path: "/players/0/ban", handler: BanPlayer},
		{name: "unban non numeric id", method: http.MethodPost, route: "/players/:id/unban", path: "/players/abc/unban", handler: UnbanPlayer},
		{name: "unban zero id", method: http.MethodPost, route: "/players/:id/unban", path: "/players/0/unban", handler: UnbanPlayer},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Handle(tc.method, tc.route, tc.handler)

			request := httptest.NewRequest(tc.method, tc.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			assertPlayerErrorResponse(t, response, http.StatusBadRequest, "无效的玩家ID")
		})
	}
}

func TestGetPlayersRejectsInvalidRoomID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []string{
		"/players?roomId=abc",
		"/players?roomId=-1",
	}

	for _, target := range testCases {
		t.Run(target, func(t *testing.T) {
			router := gin.New()
			router.GET("/players", GetPlayers)

			request := httptest.NewRequest(http.MethodGet, target, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			assertPlayerErrorResponse(t, response, http.StatusBadRequest, "房间ID")
		})
	}
}

func TestGetPlayersRejectsInvalidStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/players", GetPlayers)

	request := httptest.NewRequest(http.MethodGet, "/players?status=unknown", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertPlayerErrorResponse(t, response, http.StatusBadRequest, "玩家状态")
}

func assertPlayerErrorResponse(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantErrorFragment string) {
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
