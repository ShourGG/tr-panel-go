package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                      string
	Env                       string
	UpdateChannel             string
	SteamAPIKey               string
	UseGitHubMirror           bool
	GitHubMirrorURL           string
	DownloadRetries           int
	DownloadTimeout           int
	EnableMultiThread         bool
	DownloadTicketTTL         int
	DownloadAccelEnabled      bool
	DownloadAccelType         string
	DownloadAccelBackupPrefix string
	DownloadAccelDataPrefix   string
	BackupRemoteEnabled       bool
	BackupRemoteProvider      string
	BackupRemotePrefix        string
	BackupRemoteTimeout       int
	R2AccountID               string
	R2AccessKeyID             string
	R2SecretAccessKey         string
	R2Bucket                  string
	R2PublicBaseURL           string
	R2Endpoint                string
}

func Load() *Config {
	_ = godotenv.Load()
	useGitHubMirror := getEnv("USE_GITHUB_MIRROR", "true") == "true"
	enableMultiThread := getEnv("ENABLE_MULTI_THREAD", "false") == "true"
	downloadAccelEnabled := getEnv("DOWNLOAD_ACCEL_ENABLED", "false") == "true"
	backupRemoteEnabled := getEnv("BACKUP_REMOTE_ENABLED", "false") == "true"
	retries := 3
	if retriesStr := getEnv("DOWNLOAD_RETRIES", "5"); retriesStr != "" {
		if val, err := strconv.Atoi(retriesStr); err == nil {
			retries = val
		}
	}
	timeout := 300
	if timeoutStr := getEnv("DOWNLOAD_TIMEOUT", "600"); timeoutStr != "" {
		if val, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = val
		}
	}
	return &Config{
		Port:                      getEnv("PORT", "8800"),
		Env:                       getEnv("ENV", "development"),
		UpdateChannel:             getEnv("UPDATE_CHANNEL", "stable"),
		SteamAPIKey:               getEnv("STEAM_API_KEY", ""),
		UseGitHubMirror:           useGitHubMirror,
		GitHubMirrorURL:           getEnv("GITHUB_MIRROR_URL", "https://ghfast.top/"),
		DownloadRetries:           retries,
		DownloadTimeout:           timeout,
		EnableMultiThread:         enableMultiThread,
		DownloadTicketTTL:         mustParseEnvInt("DOWNLOAD_TICKET_TTL", 90),
		DownloadAccelEnabled:      downloadAccelEnabled,
		DownloadAccelType:         getEnv("DOWNLOAD_ACCEL_TYPE", "nginx"),
		DownloadAccelBackupPrefix: getEnv("DOWNLOAD_ACCEL_BACKUP_PREFIX", "/__downloads/backups"),
		DownloadAccelDataPrefix:   getEnv("DOWNLOAD_ACCEL_DATA_PREFIX", "/__downloads/data"),
		BackupRemoteEnabled:       backupRemoteEnabled,
		BackupRemoteProvider:      getEnv("BACKUP_REMOTE_PROVIDER", "r2"),
		BackupRemotePrefix:        getEnv("BACKUP_REMOTE_PREFIX", "terraria-panel"),
		BackupRemoteTimeout:       mustParseEnvInt("BACKUP_REMOTE_TIMEOUT", 300),
		R2AccountID:               getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:             getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:         getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2Bucket:                  getEnv("R2_BUCKET", ""),
		R2PublicBaseURL:           getEnv("R2_PUBLIC_BASE_URL", ""),
		R2Endpoint:                getEnv("R2_ENDPOINT", ""),
	}
}

func mustParseEnvInt(key string, defaultValue int) int {
	value := getEnv(key, "")
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
