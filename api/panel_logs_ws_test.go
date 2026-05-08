package api

import (
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"terraria-panel/config"
	"terraria-panel/middleware"
	"terraria-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestPanelLogsWebSocketStreamsPanelLogHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldLogsDir := config.LogsDir
	config.LogsDir = t.TempDir()
	t.Cleanup(func() {
		config.LogsDir = oldLogsDir
	})

	if err := os.WriteFile(config.PanelLogFile(), []byte("[2026-05-06 12:00:00] [INFO] panel boot\n"), 0644); err != nil {
		t.Fatalf("failed to write panel log fixture: %v", err)
	}

	token, err := middleware.GenerateToken(&models.User{ID: 1, Username: "tester", Role: "admin"})
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	router := gin.New()
	router.GET("/api/ws/panel/logs", HandlePanelLogsWS)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws/panel/logs?token=" + url.QueryEscape(token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect panel logs websocket: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 3; i++ {
		var msg struct {
			Type    string `json:"type"`
			Level   string `json:"level"`
			Message string `json:"message"`
			Time    string `json:"time"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("failed to read panel log websocket message: %v", err)
		}
		if strings.Contains(msg.Message, "panel boot") {
			if msg.Type != "log" {
				t.Fatalf("expected panel history message type log, got %q", msg.Type)
			}
			if msg.Level != "info" {
				t.Fatalf("expected panel history level info, got %q", msg.Level)
			}
			return
		}
	}

	t.Fatalf("expected panel log history to include fixture line")
}
