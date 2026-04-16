package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"terraria-panel/models"
	"time"
)

type SQLiteBackupRecordStorage struct {
	db *sql.DB
}

func NewSQLiteBackupRecordStorage(db *sql.DB) BackupRecordStorage {
	return &SQLiteBackupRecordStorage{db: db}
}

func (s *SQLiteBackupRecordStorage) Upsert(record *models.BackupRecord) error {
	query := `
		INSERT INTO backup_records (
			id, file_name, room_id, room_name, server_type, world_file, backup_type, note,
			local_path, file_size, checksum_sha256, storage_type, remote_bucket, remote_key,
			remote_etag, remote_url, upload_status, upload_error, uploaded_at, last_verified_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			file_name = excluded.file_name,
			room_id = excluded.room_id,
			room_name = excluded.room_name,
			server_type = excluded.server_type,
			world_file = excluded.world_file,
			backup_type = excluded.backup_type,
			note = excluded.note,
			local_path = excluded.local_path,
			file_size = excluded.file_size,
			checksum_sha256 = excluded.checksum_sha256,
			storage_type = excluded.storage_type,
			remote_bucket = excluded.remote_bucket,
			remote_key = excluded.remote_key,
			remote_etag = excluded.remote_etag,
			remote_url = excluded.remote_url,
			upload_status = excluded.upload_status,
			upload_error = excluded.upload_error,
			uploaded_at = excluded.uploaded_at,
			last_verified_at = excluded.last_verified_at,
			updated_at = excluded.updated_at
	`

	_, err := s.db.Exec(
		query,
		record.ID, record.FileName, record.RoomID, record.RoomName, record.ServerType, record.WorldFile, record.BackupType, record.Note,
		record.LocalPath, record.FileSize, record.ChecksumSHA256, record.StorageType, record.RemoteBucket, record.RemoteKey,
		record.RemoteETag, record.RemoteURL, record.UploadStatus, record.UploadError, record.UploadedAt, record.LastVerifiedAt,
		record.CreatedAt, record.UpdatedAt,
	)
	return err
}

func (s *SQLiteBackupRecordStorage) GetByID(id string) (*models.BackupRecord, error) {
	query := `
		SELECT id, file_name, room_id, room_name, server_type, world_file, backup_type, note,
		       local_path, file_size, checksum_sha256, storage_type, remote_bucket, remote_key,
		       remote_etag, remote_url, upload_status, upload_error, uploaded_at, last_verified_at,
		       created_at, updated_at
		FROM backup_records
		WHERE id = ?
	`

	record, err := s.scanBackupRecord(s.db.QueryRow(query, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *SQLiteBackupRecordStorage) GetByIDs(ids []string) (map[string]models.BackupRecord, error) {
	result := map[string]models.BackupRecord{}
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT id, file_name, room_id, room_name, server_type, world_file, backup_type, note,
		       local_path, file_size, checksum_sha256, storage_type, remote_bucket, remote_key,
		       remote_etag, remote_url, upload_status, upload_error, uploaded_at, last_verified_at,
		       created_at, updated_at
		FROM backup_records
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		record, err := s.scanBackupRecord(rows)
		if err != nil {
			return nil, err
		}
		result[record.ID] = *record
	}

	return result, rows.Err()
}

func (s *SQLiteBackupRecordStorage) UpdateRemoteState(id string, storageType string, uploadStatus string, uploadError string, remoteBucket string, remoteKey string, remoteETag string, remoteURL string, uploadedAt *time.Time) error {
	query := `
		UPDATE backup_records
		SET storage_type = ?, upload_status = ?, upload_error = ?, remote_bucket = ?, remote_key = ?,
		    remote_etag = ?, remote_url = ?, uploaded_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := s.db.Exec(query, storageType, uploadStatus, uploadError, remoteBucket, remoteKey, remoteETag, remoteURL, uploadedAt, id)
	return err
}

func (s *SQLiteBackupRecordStorage) UpdateVerification(id string, verifiedAt time.Time) error {
	query := `
		UPDATE backup_records
		SET last_verified_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := s.db.Exec(query, verifiedAt, id)
	return err
}

func (s *SQLiteBackupRecordStorage) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM backup_records WHERE id = ?`, id)
	return err
}

type backupRecordScanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteBackupRecordStorage) scanBackupRecord(scanner backupRecordScanner) (*models.BackupRecord, error) {
	record := &models.BackupRecord{}
	var uploadedAt sql.NullTime
	var lastVerifiedAt sql.NullTime
	err := scanner.Scan(
		&record.ID, &record.FileName, &record.RoomID, &record.RoomName, &record.ServerType, &record.WorldFile, &record.BackupType, &record.Note,
		&record.LocalPath, &record.FileSize, &record.ChecksumSHA256, &record.StorageType, &record.RemoteBucket, &record.RemoteKey,
		&record.RemoteETag, &record.RemoteURL, &record.UploadStatus, &record.UploadError, &uploadedAt, &lastVerifiedAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if uploadedAt.Valid {
		record.UploadedAt = &uploadedAt.Time
	}
	if lastVerifiedAt.Valid {
		record.LastVerifiedAt = &lastVerifiedAt.Time
	}
	return record, nil
}
