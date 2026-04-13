package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"terraria-panel/config"
	"time"
)

type DownloadOptions struct {
	URL             string
	FilePath        string
	OnProgress      func(int)
	Retries         int
	Timeout         time.Duration
	UseGitHubMirror bool
	MirrorURL       string
}

func DownloadWithRetry(opts DownloadOptions) error {
	var lastErr error
	urls := getDownloadURLs(opts.URL, opts.UseGitHubMirror, opts.MirrorURL)
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		for i, url := range urls {
			if attempt > 0 || i > 0 {
				fmt.Printf("🔄 Retry attempt %d/%d, trying URL %d/%d\n", attempt+1, opts.Retries, i+1, len(urls))
				time.Sleep(time.Second * time.Duration(attempt+1))
			}
			fmt.Printf("📥 Downloading from: %s\n", url)
			err := downloadFile(url, opts.FilePath, opts.OnProgress, opts.Timeout)
			if err == nil {
				fmt.Printf("✅ Download successful!\n")
				return nil
			}
			lastErr = err
			fmt.Printf("❌ Download failed: %v\n", err)
		}
	}
	return fmt.Errorf("download failed after %d retries: %v", opts.Retries, lastErr)
}
func getDownloadURLs(originalURL string, useGitHubMirror bool, mirrorURL string) []string {
	urls := []string{}
	if useGitHubMirror && isGitHubURL(originalURL) {
		mirrors := []string{
			"https://ghfast.top/",
			"https://cors.isteed.cc/",
			"https://gh.noki.icu/",
		}
		if mirrorURL != "" && mirrorURL != "https://ghfast.top/" {
			mirrors = append([]string{mirrorURL}, mirrors...)
		}
		for _, mirror := range mirrors {
			mirrorURL := mirror + originalURL
			urls = append(urls, mirrorURL)
		}
	}
	urls = append(urls, originalURL)
	return urls
}
func isGitHubURL(url string) bool {
	return strings.Contains(url, "github.com") ||
		strings.Contains(url, "githubusercontent.com")
}
func downloadFile(url string, filepath string, onProgress func(int), timeout time.Duration) error {
	client := &http.Client{
		Timeout: timeout,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %v", err)
	}
	req.Header.Set("User-Agent", "Terraria-Panel/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	totalSize := resp.ContentLength
	if totalSize > 0 {
		fmt.Printf("📦 File size: %.2f MB\n", float64(totalSize)/1024/1024)
	}
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("create file failed: %v", err)
	}
	defer out.Close()
	var downloaded int64
	buf := make([]byte, 256*1024)
	lastPercent := -1
	lastReportTime := time.Now()
	startTime := time.Now()
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("write file failed: %v", writeErr)
			}
			downloaded += int64(n)
			if totalSize > 0 {
				percent := int(downloaded * 100 / totalSize)
				if percent != lastPercent || time.Since(lastReportTime) > time.Second {
					lastPercent = percent
					lastReportTime = time.Now()
					if onProgress != nil {
						onProgress(percent)
					}
					elapsed := time.Since(startTime).Seconds()
					speed := float64(downloaded) / elapsed / 1024 / 1024
					fmt.Printf("📊 Progress: %d%% (%.2f/%.2f MB) Speed: %.2f MB/s\n",
						percent,
						float64(downloaded)/1024/1024,
						float64(totalSize)/1024/1024,
						speed)
				}
			} else {
				virtualPercent := int(downloaded / (1024 * 1024))
				if downloaded > 0 && virtualPercent < 1 {
					virtualPercent = 1
				}
				if virtualPercent > 99 {
					virtualPercent = 99
				}

				if virtualPercent != lastPercent || time.Since(lastReportTime) > time.Second {
					lastPercent = virtualPercent
					lastReportTime = time.Now()
					if onProgress != nil {
						onProgress(virtualPercent)
					}
					elapsed := time.Since(startTime).Seconds()
					speed := float64(downloaded) / elapsed / 1024 / 1024
					fmt.Printf("📥 Downloaded: %.2f MB (virtual progress: %d%%) Speed: %.2f MB/s\n",
						float64(downloaded)/1024/1024,
						virtualPercent,
						speed)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read failed: %v", err)
		}
	}
	if onProgress != nil {
		onProgress(100)
	}
	elapsed := time.Since(startTime).Seconds()
	avgSpeed := float64(downloaded) / elapsed / 1024 / 1024
	fmt.Printf("✅ Download complete! Total: %.2f MB, Time: %.1fs, Avg Speed: %.2f MB/s\n",
		float64(downloaded)/1024/1024, elapsed, avgSpeed)
	if err := validateDownloadedFile(filepath, contentType); err != nil {
		_ = os.Remove(filepath)
		return fmt.Errorf("downloaded content validation failed: %v", err)
	}
	return nil
}

func validateDownloadedFile(filepath string, contentType string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("open file failed: %v", err)
	}
	defer file.Close()

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read file header failed: %v", err)
	}
	header = header[:n]

	if looksLikeHTML(header) {
		return fmt.Errorf("received HTML instead of archive/binary (content-type: %s, preview: %q)", contentType, sanitizePreview(header))
	}
	if looksLikeJSON(contentType, header) {
		return fmt.Errorf("received JSON instead of archive/binary (content-type: %s, preview: %q)", contentType, sanitizePreview(header))
	}
	return nil
}

func looksLikeHTML(data []byte) bool {
	trimmed := bytes.TrimSpace(bytes.ToLower(data))
	return bytes.HasPrefix(trimmed, []byte("<!doctype html")) ||
		bytes.HasPrefix(trimmed, []byte("<html")) ||
		bytes.HasPrefix(trimmed, []byte("<head")) ||
		bytes.HasPrefix(trimmed, []byte("<body"))
}

func looksLikeJSON(contentType string, data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if strings.Contains(contentType, "application/json") {
		return true
	}
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func sanitizePreview(data []byte) string {
	preview := string(bytes.TrimSpace(data))
	preview = strings.ReplaceAll(preview, "\n", " ")
	preview = strings.ReplaceAll(preview, "\r", " ")
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	return preview
}
func GetDownloadConfig(cfg *config.Config, url string, filepath string, onProgress func(int)) DownloadOptions {
	return DownloadOptions{
		URL:             url,
		FilePath:        filepath,
		OnProgress:      onProgress,
		Retries:         cfg.DownloadRetries,
		Timeout:         time.Duration(cfg.DownloadTimeout) * time.Second,
		UseGitHubMirror: cfg.UseGitHubMirror,
		MirrorURL:       cfg.GitHubMirrorURL,
	}
}
