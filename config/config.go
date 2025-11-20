package config
import (
	"os"
	"strconv"
	"github.com/joho/godotenv"
)
type Config struct {
	Port              string
	Env               string
	UseGitHubMirror   bool
	GitHubMirrorURL   string
	DownloadRetries   int
	DownloadTimeout   int
	EnableMultiThread bool
}
func Load() *Config {
	_ = godotenv.Load()
	useGitHubMirror := getEnv("USE_GITHUB_MIRROR", "true") == "true"
	enableMultiThread := getEnv("ENABLE_MULTI_THREAD", "false") == "true"
	retries := 3
	if retriesStr := getEnv("DOWNLOAD_RETRIES", "3"); retriesStr != "" {
		if val, err := strconv.Atoi(retriesStr); err == nil {
			retries = val
		}
	}
	timeout := 300
	if timeoutStr := getEnv("DOWNLOAD_TIMEOUT", "300"); timeoutStr != "" {
		if val, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = val
		}
	}
	return &Config{
		Port:              getEnv("PORT", "8800"),
		Env:               getEnv("ENV", "development"),
		UseGitHubMirror:   useGitHubMirror,
		GitHubMirrorURL:   getEnv("GITHUB_MIRROR_URL", "https://ghproxy.com/"),
		DownloadRetries:   retries,
		DownloadTimeout:   timeout,
		EnableMultiThread: enableMultiThread,
	}
}
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
