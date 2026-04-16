package models

import "time"

type BackupRecord struct {
	ID              string     `json:"id" db:"id"`
	FileName        string     `json:"fileName" db:"file_name"`
	RoomID          int        `json:"roomId" db:"room_id"`
	RoomName        string     `json:"roomName" db:"room_name"`
	ServerType      string     `json:"serverType" db:"server_type"`
	WorldFile       string     `json:"worldFile" db:"world_file"`
	BackupType      string     `json:"backupType" db:"backup_type"`
	Note            string     `json:"note" db:"note"`
	LocalPath       string     `json:"localPath" db:"local_path"`
	FileSize        int64      `json:"fileSize" db:"file_size"`
	ChecksumSHA256  string     `json:"checksumSha256" db:"checksum_sha256"`
	StorageType     string     `json:"storageType" db:"storage_type"`
	RemoteBucket    string     `json:"remoteBucket" db:"remote_bucket"`
	RemoteKey       string     `json:"remoteKey" db:"remote_key"`
	RemoteETag      string     `json:"remoteEtag" db:"remote_etag"`
	RemoteURL       string     `json:"remoteUrl" db:"remote_url"`
	UploadStatus    string     `json:"uploadStatus" db:"upload_status"`
	UploadError     string     `json:"uploadError" db:"upload_error"`
	UploadedAt      *time.Time `json:"uploadedAt,omitempty" db:"uploaded_at"`
	LastVerifiedAt  *time.Time `json:"lastVerifiedAt,omitempty" db:"last_verified_at"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
}
