package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfigureTrustedProxiesDoesNotTrustSpoofedForwardedForByDefault(t *testing.T) {
	old := os.Getenv("TRUSTED_PROXIES")
	if err := os.Unsetenv("TRUSTED_PROXIES"); err != nil {
		t.Fatalf("unset TRUSTED_PROXIES: %v", err)
	}
	t.Cleanup(func() {
		if old == "" {
			_ = os.Unsetenv("TRUSTED_PROXIES")
		} else {
			_ = os.Setenv("TRUSTED_PROXIES", old)
		}
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := configureTrustedProxies(router); err != nil {
		t.Fatalf("configure default trusted proxies: %v", err)
	}
	router.GET("/client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.99")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Body.String(); got != "192.0.2.10" {
		t.Fatalf("spoofed X-Forwarded-For changed client IP: got %q", got)
	}
}

func TestConfigureTrustedProxiesUsesOnlyExplicitProxyAddresses(t *testing.T) {
	old := os.Getenv("TRUSTED_PROXIES")
	if err := os.Setenv("TRUSTED_PROXIES", "127.0.0.1"); err != nil {
		t.Fatalf("set TRUSTED_PROXIES: %v", err)
	}
	t.Cleanup(func() {
		if old == "" {
			_ = os.Unsetenv("TRUSTED_PROXIES")
		} else {
			_ = os.Setenv("TRUSTED_PROXIES", old)
		}
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := configureTrustedProxies(router); err != nil {
		t.Fatalf("configure explicit trusted proxy: %v", err)
	}
	router.GET("/client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
	request.RemoteAddr = "127.0.0.1:8080"
	request.Header.Set("X-Forwarded-For", "198.51.100.99")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Body.String(); got != "198.51.100.99" {
		t.Fatalf("explicit trusted proxy should provide forwarded client IP: got %q", got)
	}
}
