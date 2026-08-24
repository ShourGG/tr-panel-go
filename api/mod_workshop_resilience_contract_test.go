package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"terraria-panel/config"

	"github.com/gin-gonic/gin"
)

type workshopRoundTripper func(*http.Request) (*http.Response, error)

func (f workshopRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func useWorkshopHTTPTestClient(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	oldClient := steamWorkshopHTTPClient
	oldTTL := steamWorkshopRequestTTL
	oldRetryDelay := steamWorkshopRetryDelay
	steamWorkshopHTTPClient = &http.Client{Transport: transport}
	steamWorkshopRequestTTL = 100 * time.Millisecond
	steamWorkshopRetryDelay = func(int) time.Duration { return 0 }
	t.Cleanup(func() {
		steamWorkshopHTTPClient = oldClient
		steamWorkshopRequestTTL = oldTTL
		steamWorkshopRetryDelay = oldRetryDelay
	})
}

func workshopHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestFetchSteamWorkshopQueryRetriesRateLimitsAndServerErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("STEAM_API_KEY", "test-key")
			var attempts atomic.Int32
			useWorkshopHTTPTestClient(t, workshopRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("User-Agent") != "Terraria-Panel/1.0" {
					t.Fatalf("missing Workshop User-Agent: %q", request.Header.Get("User-Agent"))
				}
				if attempts.Add(1) < 3 {
					return workshopHTTPResponse(status, "temporary failure"), nil
				}
				return workshopHTTPResponse(http.StatusOK, `{"response":{"total":1,"publishedfiledetails":[{"publishedfileid":"2619954303","title":"Recipe Browser"}]}}`), nil
			}))

			result, err := fetchSteamWorkshopQueryPage("3", 1, 10, "")
			if err != nil {
				t.Fatalf("fetch after temporary HTTP %d failures: %v", status, err)
			}
			if got := attempts.Load(); got != 3 {
				t.Fatalf("attempt count = %d, want 3", got)
			}
			if result.Response.Total != 1 || len(result.Response.PublishedFiles) != 1 {
				t.Fatalf("unexpected successful Workshop payload: %+v", result.Response)
			}
		})
	}
}

func TestFetchSteamWorkshopQueryBoundsTimeoutAndClassifiesIt(t *testing.T) {
	t.Setenv("STEAM_API_KEY", "test-key")
	var attempts atomic.Int32
	useWorkshopHTTPTestClient(t, workshopRoundTripper(func(request *http.Request) (*http.Response, error) {
		attempts.Add(1)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}))
	steamWorkshopRequestTTL = 10 * time.Millisecond

	startedAt := time.Now()
	_, err := fetchSteamWorkshopQueryPage("3", 1, 10, "")
	if err == nil || !strings.Contains(err.Error(), "请求超时") {
		t.Fatalf("expected classified timeout, got %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("timeout attempt count = %d, want 3", got)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("timeout retries exceeded bounded test window: %s", elapsed)
	}
}

func TestFetchSteamWorkshopQueryRejectsInvalidJSONWithoutRetry(t *testing.T) {
	t.Setenv("STEAM_API_KEY", "test-key")
	var attempts atomic.Int32
	useWorkshopHTTPTestClient(t, workshopRoundTripper(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return workshopHTTPResponse(http.StatusOK, `{not-json`), nil
	}))

	_, err := fetchSteamWorkshopQueryPage("3", 1, 10, "")
	if err == nil || !strings.Contains(err.Error(), "数据格式无效") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("invalid JSON must not retry, attempts = %d", got)
	}
}

func TestSearchWorkshopModsReturnsMarkedStaleCacheAfterNetworkFailure(t *testing.T) {
	t.Setenv("STEAM_API_KEY", "test-key")
	oldDataDir := config.DataDir
	config.DataDir = t.TempDir()
	t.Cleanup(func() { config.DataDir = oldDataDir })

	workshopCacheMu.Lock()
	oldCache := workshopCache
	workshopCache = make(map[string]*workshopCacheEntry)
	workshopCacheMu.Unlock()
	t.Cleanup(func() {
		workshopCacheMu.Lock()
		workshopCache = oldCache
		workshopCacheMu.Unlock()
	})

	cacheKey := "3||1|20"
	cachedPayload, err := json.Marshal(map[string]any{
		"success": true,
		"data": map[string]any{
			"total": 1,
			"items": []map[string]any{{"publishedfileid": "2619954303", "title": "Recipe Browser"}},
		},
	})
	if err != nil {
		t.Fatalf("marshal stale response fixture: %v", err)
	}
	workshopCacheMu.Lock()
	workshopCache[cacheKey] = &workshopCacheEntry{
		data:     cachedPayload,
		cachedAt: time.Now().Add(-workshopCacheTTL - time.Minute),
	}
	workshopCacheMu.Unlock()

	useWorkshopHTTPTestClient(t, workshopRoundTripper(func(*http.Request) (*http.Response, error) {
		return workshopHTTPResponse(http.StatusServiceUnavailable, "offline"), nil
	}))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mods/workshop", SearchWorkshopMods)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/mods/workshop?page=1&pageSize=20", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("stale fallback status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Workshop-Cache") != "stale" {
		t.Fatalf("expected stale cache header, got %q", response.Header().Get("X-Workshop-Cache"))
	}

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Stale    bool   `json:"stale"`
			Warning  string `json:"warning"`
			CachedAt string `json:"cachedAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode stale response: %v", err)
	}
	if !payload.Success || !payload.Data.Stale || payload.Data.Warning == "" || payload.Data.CachedAt == "" {
		t.Fatalf("stale fallback omitted user-visible metadata: %+v", payload)
	}
}
