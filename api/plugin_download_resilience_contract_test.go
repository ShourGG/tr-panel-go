package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"terraria-panel/models"
)

func TestDownloadPluginPackageRetriesTruncatedResponse(t *testing.T) {
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	entry, err := zipWriter.Create("Plugins/ListPlugins.dll")
	if err != nil {
		t.Fatalf("create plugin archive entry: %v", err)
	}
	if _, err := entry.Write([]byte("MZ NETCoreApp,Version=v6.0")); err != nil {
		t.Fatalf("write plugin archive entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close plugin archive: %v", err)
	}
	archiveBytes := archive.Bytes()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("request method = %s, want GET", request.Method)
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(archiveBytes)))
		writer.WriteHeader(http.StatusOK)
		if requests.Add(1) == 1 {
			_, _ = writer.Write(archiveBytes[:len(archiveBytes)/2])
			return
		}
		_, _ = writer.Write(archiveBytes)
	}))
	defer server.Close()

	oldRetryDelay := pluginDownloadRetryDelay
	pluginDownloadRetryDelay = func(int) time.Duration { return 0 }
	t.Cleanup(func() { pluginDownloadRetryDelay = oldRetryDelay })

	destPath := filepath.Join(t.TempDir(), "Plugins.zip")
	progress := &models.DownloadProgress{}
	if err := downloadPluginPackage([]string{server.URL}, destPath, progress); err != nil {
		t.Fatalf("downloadPluginPackage should recover from a truncated response: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}

	reader, err := zip.OpenReader(destPath)
	if err != nil {
		t.Fatalf("retry output is not a valid ZIP: %v", err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "Plugins/ListPlugins.dll" {
		t.Fatalf("retry output entries = %+v, want ListPlugins.dll", reader.File)
	}
	entryReader, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("open retried plugin entry: %v", err)
	}
	defer entryReader.Close()
	if _, err := io.ReadAll(entryReader); err != nil {
		t.Fatalf("read retried plugin entry: %v", err)
	}
}
