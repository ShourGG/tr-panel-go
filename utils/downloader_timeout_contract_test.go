package utils

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadFileUsesIdleTimeoutInsteadOfTotalDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flushing")
		}
		for _, chunk := range [][]byte{
			{0x50, 0x4b, 0x03, 0x04},
			[]byte("slow-download-a"),
			[]byte("slow-download-b"),
			[]byte("slow-download-c"),
		} {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "download.zip")
	if err := downloadFile(server.URL, output, nil, 100*time.Millisecond); err != nil {
		t.Fatalf("a slow but continuously active download should succeed: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "PK\x03\x04" {
		t.Fatalf("downloaded payload has unexpected header: % X", data[:4])
	}
}

func TestDownloadFileFailsWhenBodyIsIdlePastTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte{0x50, 0x4b, 0x03, 0x04})
		flusher.Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "download.zip")
	err := downloadFile(server.URL, output, nil, 30*time.Millisecond)
	if err == nil {
		t.Fatal("an idle body read should fail after the configured timeout")
	}
	if _, statErr := os.Stat(output); statErr != nil {
		t.Fatalf("partial output should remain available for retry diagnostics: %v", statErr)
	}
}
