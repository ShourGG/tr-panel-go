package api

import (
	"compress/gzip"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type gzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipWriter) Write(data []byte) (int, error) {
	if g.Header().Get("Content-Type") == "" {
		g.Header().Set("Content-Type", http.DetectContentType(data))
	}
	return g.writer.Write(data)
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	return g.writer.Write([]byte(s))
}

func shouldCompress(path string) bool {
	if strings.HasPrefix(path, "/api/ws") || strings.HasPrefix(path, "/ws") {
		return false
	}
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/assets/") {
		return true
	}

	switch {
	case strings.HasSuffix(path, ".html"),
		strings.HasSuffix(path, ".js"),
		strings.HasSuffix(path, ".css"),
		strings.HasSuffix(path, ".json"),
		strings.HasSuffix(path, ".svg"),
		strings.HasSuffix(path, ".txt"),
		strings.HasSuffix(path, ".map"):
		return true
	default:
		return false
	}
}

func GzipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldCompress(c.Request.URL.Path) {
			c.Next()
			return
		}
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}
		if strings.Contains(strings.ToLower(c.GetHeader("Connection")), "upgrade") {
			c.Next()
			return
		}

		gz, err := gzip.NewWriterLevel(c.Writer, gzip.BestSpeed)
		if err != nil {
			c.Next()
			return
		}
		defer gz.Close()

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Header("Content-Length", "")

		c.Writer = &gzipWriter{
			ResponseWriter: c.Writer,
			writer:         gz,
		}

		c.Next()
	}
}
