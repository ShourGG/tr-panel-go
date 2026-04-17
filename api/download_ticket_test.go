package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"terraria-panel/config"
	"terraria-panel/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestIssueAndParseDownloadTicket(t *testing.T) {
	ticket, expiresAt, err := issueDownloadTicket(downloadKindFile, "rooms/room-1/world.wld", false, "world.wld", 7)
	if err != nil {
		t.Fatalf("expected ticket to be issued, got error: %v", err)
	}
	if ticket == "" {
		t.Fatal("expected non-empty ticket")
	}

	claims, err := parseDownloadTicket(ticket, downloadKindFile)
	if err != nil {
		t.Fatalf("expected ticket to parse, got error: %v", err)
	}

	if claims.Target != "rooms/room-1/world.wld" {
		t.Fatalf("expected target to round-trip, got %s", claims.Target)
	}
	if claims.UserID != 7 {
		t.Fatalf("expected user id 7, got %d", claims.UserID)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected expiresAt to be present")
	}
	if diff := claims.ExpiresAt.Time.Sub(expiresAt); diff < -time.Second || diff > time.Second {
		t.Fatalf("expected expiresAt close to %v, got %+v", expiresAt, claims.ExpiresAt)
	}
}

func TestParseDownloadTicketRejectsExpiredTicket(t *testing.T) {
	now := time.Now()
	expiredTicket, err := middleware.SignJWT(downloadTicketClaims{
		Kind:     downloadKindBackup,
		Target:   "backup-1",
		FileName: "backup.zip",
		UserID:   1,
		Nonce:    "expired-ticket",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    downloadTicketIssuer,
			Subject:   "backup:1",
			IssuedAt:  jwt.NewNumericDate(now.Add(-5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-2 * time.Minute)),
		},
	})
	if err != nil {
		t.Fatalf("failed to sign expired ticket: %v", err)
	}

	if _, err := parseDownloadTicket(expiredTicket, downloadKindBackup); err == nil {
		t.Fatal("expected expired ticket to be rejected")
	}
}

func TestBuildAccelRedirectPath(t *testing.T) {
	backupDir, filePath, cleanup := setupDownloadTicketRoots(t)
	defer cleanup()

	cfg := &config.Config{
		DownloadAccelBackupPrefix: "/__downloads/backups",
		DownloadAccelDataPrefix:   "/__downloads/data",
	}

	redirectPath, err := buildAccelRedirectPath(cfg, downloadTargetBackup, filePath)
	if err != nil {
		t.Fatalf("expected redirect path to build, got error: %v", err)
	}

	expected := "/__downloads/backups/room-1_test.zip"
	if redirectPath != expected {
		t.Fatalf("expected %s, got %s", expected, redirectPath)
	}

	if _, err := os.Stat(filepath.Join(backupDir, "room-1_test.zip")); err != nil {
		t.Fatalf("expected backup file to exist, got error: %v", err)
	}
}

func TestServeStaticDownloadUsesAccelWhenEnabled(t *testing.T) {
	_, filePath, cleanup := setupDownloadTicketRoots(t)
	defer cleanup()

	t.Setenv("DOWNLOAD_ACCEL_ENABLED", "true")
	t.Setenv("DOWNLOAD_ACCEL_TYPE", "nginx")
	t.Setenv("DOWNLOAD_ACCEL_BACKUP_PREFIX", "/__downloads/backups")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	if err := serveStaticDownload(ctx, filePath, "room-1_test.zip", downloadTargetBackup); err != nil {
		t.Fatalf("expected accelerated download to succeed, got error: %v", err)
	}

	if got := recorder.Header().Get("X-Accel-Redirect"); got != "/__downloads/backups/room-1_test.zip" {
		t.Fatalf("expected X-Accel-Redirect header to be set, got %s", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got == "" {
		t.Fatal("expected Content-Disposition header to be set")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestGetManagedDownloadNameAvoidsDoubleZipSuffix(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "mods.zip")
	if err := os.WriteFile(filePath, []byte("zip"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if got := getManagedDownloadName(info, true); got != "mods.zip" {
		t.Fatalf("expected mods.zip, got %s", got)
	}
}

func setupDownloadTicketRoots(t *testing.T) (string, string, func()) {
	t.Helper()

	oldDataDir := config.DataDir
	oldBackupDir := config.BackupDir

	rootDir := t.TempDir()
	dataDir := filepath.Join(rootDir, "data")
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	filePath := filepath.Join(backupDir, "room-1_test.zip")
	if err := os.WriteFile(filePath, []byte("backup"), 0644); err != nil {
		t.Fatalf("failed to create backup file: %v", err)
	}

	config.DataDir = dataDir
	config.BackupDir = backupDir

	return backupDir, filePath, func() {
		config.DataDir = oldDataDir
		config.BackupDir = oldBackupDir
	}
}
