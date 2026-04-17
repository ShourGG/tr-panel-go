package middleware

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"terraria-panel/models"

	"github.com/gin-gonic/gin"
)

func TestExtractRequestTokenFallsBackToQueryToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/download?token=query-token", nil)

	token, err := ExtractRequestToken(req, true)
	if err != nil {
		t.Fatalf("expected query token to be accepted, got error: %v", err)
	}
	if token != "query-token" {
		t.Fatalf("expected query-token, got %s", token)
	}
}

func TestAuthMiddlewareRejectsQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &models.User{ID: 1, Username: "tester", Role: "admin"}
	token, err := GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/protected", func(c *gin.Context) {
		c.Status(204)
	})

	request := httptest.NewRequest("GET", "/protected?token="+url.QueryEscape(token), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != 401 {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestDownloadAuthMiddlewareAcceptsQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &models.User{ID: 1, Username: "tester", Role: "admin"}
	token, err := GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	router := gin.New()
	router.Use(DownloadAuthMiddleware())
	router.GET("/download", func(c *gin.Context) {
		c.Status(204)
	})

	request := httptest.NewRequest("GET", "/download?token="+url.QueryEscape(token), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != 204 {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}
