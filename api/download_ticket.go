package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"terraria-panel/config"
	"terraria-panel/middleware"
	"terraria-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	downloadKindBackup       = "backup"
	downloadKindFile         = "file"
	downloadTicketIssuer     = "terraria-panel-download"
	downloadAccelTypeNginx   = "nginx"
	defaultDownloadTicketTTL = 90
	minDownloadTicketTTL     = 30
	maxDownloadTicketTTL     = 600
)

type downloadTargetRoot string

const (
	downloadTargetBackup downloadTargetRoot = "backup"
	downloadTargetData   downloadTargetRoot = "data"
)

type downloadTicketClaims struct {
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Archive  bool   `json:"archive,omitempty"`
	FileName string `json:"file_name,omitempty"`
	UserID   int    `json:"user_id"`
	Nonce    string `json:"nonce"`
	jwt.RegisteredClaims
}

func CreateBackupDownloadTicket(c *gin.Context) {
	backupID := strings.TrimSpace(c.Param("id"))
	backupPath, err := resolveBackupPath(backupID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的备份ID"))
		return
	}

	userID, ok := getDownloadTicketUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("缺少用户上下文"))
		return
	}

	backupName := filepath.Base(backupPath)
	ticket, expiresAt, err := issueDownloadTicket(downloadKindBackup, backupID, false, backupName, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("生成下载票据失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"ticket":      ticket,
		"downloadUrl": buildTicketDownloadURL(downloadKindBackup, ticket),
		"fileName":    backupName,
		"expiresAt":   expiresAt.Format(time.RFC3339),
		"mode":        getStaticDownloadMode(downloadTargetBackup, backupPath),
	}))
}

func CreateFileDownloadTicket(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Archive bool   `json:"archive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	fullPath, info, relativePath, err := resolveManagedFileDownloadTarget(req.Path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			c.JSON(http.StatusNotFound, models.ErrorResponse("文件不存在"))
		default:
			c.JSON(http.StatusBadRequest, models.ErrorResponse("非法文件路径"))
		}
		return
	}

	userID, ok := getDownloadTicketUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("缺少用户上下文"))
		return
	}

	downloadName := getManagedDownloadName(info, req.Archive)
	ticket, expiresAt, err := issueDownloadTicket(downloadKindFile, relativePath, req.Archive, downloadName, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("生成下载票据失败"))
		return
	}

	mode := "direct"
	if !(info.IsDir() || req.Archive) {
		mode = getStaticDownloadMode(downloadTargetData, fullPath)
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"ticket":      ticket,
		"downloadUrl": buildTicketDownloadURL(downloadKindFile, ticket),
		"fileName":    downloadName,
		"expiresAt":   expiresAt.Format(time.RFC3339),
		"mode":        mode,
	}))
}

func DownloadBackupByTicket(c *gin.Context) {
	claims, err := parseDownloadTicket(c.Param("ticket"), downloadKindBackup)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("下载票据无效或已过期"))
		return
	}

	if err := serveBackupDownload(c, claims.Target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("备份下载失败: "+err.Error()))
	}
}

func DownloadFileByTicket(c *gin.Context) {
	claims, err := parseDownloadTicket(c.Param("ticket"), downloadKindFile)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("下载票据无效或已过期"))
		return
	}

	if err := serveManagedFileDownload(c, claims.Target, claims.Archive); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			c.JSON(http.StatusNotFound, models.ErrorResponse("文件不存在"))
		default:
			c.JSON(http.StatusBadRequest, models.ErrorResponse("文件下载失败: "+err.Error()))
		}
	}
}

func serveBackupDownload(c *gin.Context, backupID string) error {
	backupPath, err := resolveBackupPath(backupID)
	if err != nil {
		return err
	}
	return serveStaticDownload(c, backupPath, filepath.Base(backupPath), downloadTargetBackup)
}

func serveManagedFileDownload(c *gin.Context, relativePath string, archive bool) error {
	fullPath, info, _, err := resolveManagedFileDownloadTarget(relativePath)
	if err != nil {
		return err
	}

	if info.IsDir() || archive {
		return streamZipArchive(c, fullPath, info.IsDir())
	}

	return serveStaticDownload(c, fullPath, info.Name(), downloadTargetData)
}

func resolveManagedFileDownloadTarget(relativePath string) (string, os.FileInfo, string, error) {
	fullPath, err := resolveDataPath(relativePath)
	if err != nil {
		return "", nil, "", err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", nil, "", err
	}

	normalizedRelativePath, err := filepath.Rel(config.DataDir, fullPath)
	if err != nil {
		return "", nil, "", err
	}

	return fullPath, info, filepath.ToSlash(normalizedRelativePath), nil
}

func serveStaticDownload(c *gin.Context, fullPath string, downloadName string, target downloadTargetRoot) error {
	if tryServeAcceleratedDownload(c, fullPath, downloadName, target) {
		return nil
	}

	c.FileAttachment(fullPath, downloadName)
	return nil
}

func tryServeAcceleratedDownload(c *gin.Context, fullPath string, downloadName string, target downloadTargetRoot) bool {
	cfg := config.Load()
	if !cfg.DownloadAccelEnabled || !strings.EqualFold(cfg.DownloadAccelType, downloadAccelTypeNginx) {
		return false
	}

	internalPath, err := buildAccelRedirectPath(cfg, target, fullPath)
	if err != nil {
		log.Printf("[Download] Skip accelerated download for %s: %v", fullPath, err)
		return false
	}

	contentDisposition := buildAttachmentDisposition(downloadName)
	contentType := detectDownloadContentType(downloadName)
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Header("Content-Disposition", contentDisposition)
	c.Header("X-Accel-Redirect", internalPath)
	c.Status(http.StatusOK)
	return true
}

func buildAccelRedirectPath(cfg *config.Config, target downloadTargetRoot, fullPath string) (string, error) {
	var (
		rootDir        string
		internalPrefix string
	)

	switch target {
	case downloadTargetBackup:
		rootDir = config.BackupDir
		internalPrefix = cfg.DownloadAccelBackupPrefix
	case downloadTargetData:
		rootDir = config.DataDir
		internalPrefix = cfg.DownloadAccelDataPrefix
	default:
		return "", fmt.Errorf("unsupported download target: %s", target)
	}

	relativePath, err := filepath.Rel(filepath.Clean(rootDir), filepath.Clean(fullPath))
	if err != nil {
		return "", err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", errors.New("download target is outside allowed root")
	}

	cleanPrefix := normalizeAccelPrefix(internalPrefix)
	if cleanPrefix == "" {
		return "", errors.New("download accel prefix is empty")
	}

	segments := strings.Split(filepath.ToSlash(relativePath), "/")
	escapedSegments := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" || segment == "." {
			continue
		}
		escapedSegments = append(escapedSegments, url.PathEscape(segment))
	}

	if len(escapedSegments) == 0 {
		return cleanPrefix, nil
	}

	return cleanPrefix + "/" + strings.Join(escapedSegments, "/"), nil
}

func normalizeAccelPrefix(prefix string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(strings.ReplaceAll(prefix, "\\", "/")))
	if cleaned == "." {
		return ""
	}
	return strings.TrimRight(cleaned, "/")
}

func buildAttachmentDisposition(downloadName string) string {
	contentDisposition := mime.FormatMediaType("attachment", map[string]string{
		"filename": downloadName,
	})
	if strings.TrimSpace(contentDisposition) == "" {
		escapedName := strings.ReplaceAll(downloadName, "\"", "")
		return fmt.Sprintf("attachment; filename=\"%s\"", escapedName)
	}
	return contentDisposition
}

func detectDownloadContentType(downloadName string) string {
	if strings.HasSuffix(strings.ToLower(downloadName), ".zip") {
		return "application/zip"
	}
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(downloadName))); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func getStaticDownloadMode(target downloadTargetRoot, fullPath string) string {
	cfg := config.Load()
	if !cfg.DownloadAccelEnabled || !strings.EqualFold(cfg.DownloadAccelType, downloadAccelTypeNginx) {
		return "direct"
	}
	if _, err := buildAccelRedirectPath(cfg, target, fullPath); err != nil {
		return "direct"
	}
	return "proxy"
}

func getDownloadTicketUserID(c *gin.Context) (int, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}

	switch value := userID.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func issueDownloadTicket(kind string, target string, archive bool, fileName string, userID int) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(getDownloadTicketTTL())
	nonce, err := generateDownloadTicketNonce()
	if err != nil {
		return "", time.Time{}, err
	}

	claims := downloadTicketClaims{
		Kind:     kind,
		Target:   target,
		Archive:  archive,
		FileName: fileName,
		UserID:   userID,
		Nonce:    nonce,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			Issuer:    downloadTicketIssuer,
			Subject:   fmt.Sprintf("%s:%d", kind, userID),
		},
	}

	ticket, err := middleware.SignJWT(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	return ticket, expiresAt, nil
}

func parseDownloadTicket(ticket string, expectedKind string) (*downloadTicketClaims, error) {
	claims := &downloadTicketClaims{}
	if err := middleware.ParseJWTWithClaims(ticket, claims, jwt.WithIssuer(downloadTicketIssuer), jwt.WithLeeway(5*time.Second)); err != nil {
		return nil, err
	}
	if claims.Kind != expectedKind || claims.UserID <= 0 || strings.TrimSpace(claims.Target) == "" {
		return nil, errors.New("invalid download ticket claims")
	}
	return claims, nil
}

func buildTicketDownloadURL(kind string, ticket string) string {
	switch kind {
	case downloadKindBackup:
		return "/api/downloads/backups/" + url.PathEscape(ticket)
	case downloadKindFile:
		return "/api/downloads/files/" + url.PathEscape(ticket)
	default:
		return ""
	}
}

func getDownloadTicketTTL() time.Duration {
	cfg := config.Load()
	ttl := cfg.DownloadTicketTTL
	switch {
	case ttl <= 0:
		ttl = defaultDownloadTicketTTL
	case ttl < minDownloadTicketTTL:
		ttl = minDownloadTicketTTL
	case ttl > maxDownloadTicketTTL:
		ttl = maxDownloadTicketTTL
	}
	return time.Duration(ttl) * time.Second
}

func generateDownloadTicketNonce() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce), nil
}
