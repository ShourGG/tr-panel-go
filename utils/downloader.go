package utils
import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"terraria-panel/config"
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
			"https://ghproxy.com/",
			"https://gh-proxy.com/",
			"https://mirror.ghproxy.com/",
		}
		if mirrorURL != "" && mirrorURL != "https://ghproxy.com/" {
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
				if downloaded%(1024*1024) == 0 {
					if onProgress != nil {
						virtualPercent := int(downloaded / (1024 * 1024))
						if virtualPercent > 99 {
							virtualPercent = 99
						}
						onProgress(virtualPercent)
					}
					fmt.Printf("📥 Downloaded: %.2f MB\n", float64(downloaded)/1024/1024)
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
	if onProgress != nil && totalSize > 0 {
		onProgress(100)
	}
	elapsed := time.Since(startTime).Seconds()
	avgSpeed := float64(downloaded) / elapsed / 1024 / 1024
	fmt.Printf("✅ Download complete! Total: %.2f MB, Time: %.1fs, Avg Speed: %.2f MB/s\n",
		float64(downloaded)/1024/1024, elapsed, avgSpeed)
	return nil
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
