package services

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"terraria-panel/config"
	"terraria-panel/models"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type BackupRemoteSyncResult struct {
	StorageType string
	Bucket      string
	Key         string
	ETag        string
	RemoteURL   string
	UploadedAt  time.Time
}

type BackupRemoteVerifyResult struct {
	Bucket       string
	Key          string
	RemoteURL    string
	RemoteETag   string
	RemoteSize   int64
	ChecksumSHA256 string
	VerifiedAt   time.Time
}

type BackupRemoteService interface {
	Enabled() bool
	SyncBackup(ctx context.Context, record *models.BackupRecord) (*BackupRemoteSyncResult, error)
	VerifyBackup(ctx context.Context, record *models.BackupRecord) (*BackupRemoteVerifyResult, error)
}

type disabledBackupRemoteService struct{}

func (s *disabledBackupRemoteService) Enabled() bool {
	return false
}

func (s *disabledBackupRemoteService) SyncBackup(ctx context.Context, record *models.BackupRecord) (*BackupRemoteSyncResult, error) {
	return nil, fmt.Errorf("remote backup is disabled")
}

func (s *disabledBackupRemoteService) VerifyBackup(ctx context.Context, record *models.BackupRecord) (*BackupRemoteVerifyResult, error) {
	return nil, fmt.Errorf("remote backup is disabled")
}

type r2BackupRemoteService struct {
	client        *minio.Client
	bucket        string
	prefix        string
	timeout       time.Duration
	publicBaseURL string
}

func NewBackupRemoteService(cfg *config.Config) (BackupRemoteService, error) {
	if cfg == nil || !cfg.BackupRemoteEnabled {
		return &disabledBackupRemoteService{}, nil
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.BackupRemoteProvider))
	if provider == "" {
		provider = "r2"
	}
	if provider != "r2" {
		return nil, fmt.Errorf("unsupported remote backup provider: %s", provider)
	}

	endpoint := strings.TrimSpace(cfg.R2Endpoint)
	if endpoint == "" {
		if strings.TrimSpace(cfg.R2AccountID) == "" {
			return nil, fmt.Errorf("R2_ACCOUNT_ID is required when BACKUP_REMOTE_ENABLED=true")
		}
		endpoint = fmt.Sprintf("%s.r2.cloudflarestorage.com", strings.TrimSpace(cfg.R2AccountID))
	}

	secure := true
	if strings.HasPrefix(endpoint, "http://") {
		secure = false
		endpoint = strings.TrimPrefix(endpoint, "http://")
	} else {
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	if strings.TrimSpace(cfg.R2AccessKeyID) == "" || strings.TrimSpace(cfg.R2SecretAccessKey) == "" {
		return nil, fmt.Errorf("R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are required when remote backup is enabled")
	}
	if strings.TrimSpace(cfg.R2Bucket) == "" {
		return nil, fmt.Errorf("R2_BUCKET is required when remote backup is enabled")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(strings.TrimSpace(cfg.R2AccessKeyID), strings.TrimSpace(cfg.R2SecretAccessKey), ""),
		Secure: secure,
		Region: "auto",
	})
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(cfg.BackupRemoteTimeout) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}

	return &r2BackupRemoteService{
		client:        client,
		bucket:        strings.TrimSpace(cfg.R2Bucket),
		prefix:        strings.Trim(strings.TrimSpace(cfg.BackupRemotePrefix), "/"),
		timeout:       timeout,
		publicBaseURL: strings.TrimRight(strings.TrimSpace(cfg.R2PublicBaseURL), "/"),
	}, nil
}

func (s *r2BackupRemoteService) Enabled() bool {
	return true
}

func (s *r2BackupRemoteService) SyncBackup(ctx context.Context, record *models.BackupRecord) (*BackupRemoteSyncResult, error) {
	if record == nil {
		return nil, fmt.Errorf("backup record is required")
	}
	if strings.TrimSpace(record.LocalPath) == "" {
		return nil, fmt.Errorf("backup local path is empty")
	}

	bucketExists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to verify remote bucket: %w", err)
	}
	if !bucketExists {
		return nil, fmt.Errorf("remote bucket does not exist: %s", s.bucket)
	}

	objectKey := s.buildObjectKey(record)
	putOptions := minio.PutObjectOptions{
		ContentType: "application/zip",
		UserMetadata: map[string]string{
			"room-id":     fmt.Sprintf("%d", record.RoomID),
			"server-type": record.ServerType,
			"backup-type": record.BackupType,
		},
	}
	if record.ChecksumSHA256 != "" {
		putOptions.UserMetadata["sha256"] = record.ChecksumSHA256
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	info, err := s.client.FPutObject(ctx, s.bucket, objectKey, record.LocalPath, putOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to upload backup to remote storage: %w", err)
	}

	result := &BackupRemoteSyncResult{
		StorageType: "local+r2",
		Bucket:      s.bucket,
		Key:         objectKey,
		ETag:        strings.Trim(info.ETag, "\""),
		UploadedAt:  time.Now(),
	}
	if s.publicBaseURL != "" {
		baseURL, err := url.Parse(s.publicBaseURL + "/")
		if err == nil {
			relativeURL := &url.URL{Path: objectKey}
			result.RemoteURL = baseURL.ResolveReference(relativeURL).String()
		}
	}
	return result, nil
}

func (s *r2BackupRemoteService) VerifyBackup(ctx context.Context, record *models.BackupRecord) (*BackupRemoteVerifyResult, error) {
	if record == nil {
		return nil, fmt.Errorf("backup record is required")
	}
	if strings.TrimSpace(record.RemoteKey) == "" {
		return nil, fmt.Errorf("remote backup key is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	stat, err := s.client.StatObject(ctx, s.bucket, record.RemoteKey, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat remote backup object: %w", err)
	}

	if record.FileSize > 0 && stat.Size != record.FileSize {
		return nil, fmt.Errorf("remote backup size mismatch: remote=%d local=%d", stat.Size, record.FileSize)
	}

	remoteChecksum := getObjectMetadataValue(stat.UserMetadata, "sha256")
	if record.ChecksumSHA256 != "" {
		if remoteChecksum != "" && !strings.EqualFold(remoteChecksum, record.ChecksumSHA256) {
			return nil, fmt.Errorf("remote backup checksum mismatch")
		}
	}

	result := &BackupRemoteVerifyResult{
		Bucket:         s.bucket,
		Key:            record.RemoteKey,
		RemoteURL:      record.RemoteURL,
		RemoteETag:     strings.Trim(stat.ETag, "\""),
		RemoteSize:     stat.Size,
		ChecksumSHA256: remoteChecksum,
		VerifiedAt:     time.Now(),
	}
	return result, nil
}

func getObjectMetadataValue(metadata map[string]string, key string) string {
	if len(metadata) == 0 {
		return ""
	}

	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	for metaKey, value := range metadata {
		candidate := strings.ToLower(strings.TrimSpace(metaKey))
		candidate = strings.TrimPrefix(candidate, "x-amz-meta-")
		if candidate == normalizedKey {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func (s *r2BackupRemoteService) buildObjectKey(record *models.BackupRecord) string {
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	segments := []string{}
	if s.prefix != "" {
		segments = append(segments, s.prefix)
	}
	segments = append(segments,
		record.ServerType,
		fmt.Sprintf("room-%d", record.RoomID),
		createdAt.Format("2006"),
		createdAt.Format("01"),
		createdAt.Format("02"),
		record.FileName,
	)
	return path.Join(segments...)
}
