package api

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	allowedOrigins     map[string]struct{}
	allowAllOrigins    bool
	loadOriginsOnce    sync.Once
	defaultOriginRules = []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:8800",
		"http://127.0.0.1:8800",
	}
)

func loadAllowedOrigins() {
	allowedOrigins = make(map[string]struct{})
	origins := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))

	if origins == "" {
		for _, origin := range defaultOriginRules {
			allowedOrigins[strings.ToLower(origin)] = struct{}{}
		}
		return
	}

	if origins == "*" {
		allowAllOrigins = true
		return
	}

	for _, origin := range strings.Split(origins, ",") {
		cleanOrigin := strings.ToLower(strings.TrimSpace(origin))
		if cleanOrigin == "" {
			continue
		}
		allowedOrigins[cleanOrigin] = struct{}{}
	}
}

func isOriginAllowed(origin string) bool {
	if origin == "" {
		return true
	}

	loadOriginsOnce.Do(loadAllowedOrigins)
	if allowAllOrigins {
		return true
	}

	_, ok := allowedOrigins[strings.ToLower(strings.TrimSpace(origin))]
	return ok
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		origin := c.GetHeader("Origin")
		if origin != "" {
			if !isOriginAllowed(origin) {
				if c.Request.Method == "OPTIONS" {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Set("Vary", "Origin")
			}
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
