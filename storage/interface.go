package storage

import (
	"terraria-panel/models"
	"time"
)
type RoomStorage interface {
	GetAll() ([]models.Room, error)
	GetByID(id int) (*models.Room, error)
	Create(room *models.Room) error
	Update(room *models.Room) error
	Delete(id int) error
	UpdateStatus(id int, status string, pid int) error
	UpdateAdminToken(id int, token string) error
}
type PlayerStorage interface {
	GetAll() ([]models.Player, error)
	GetByID(id int) (*models.Player, error)
	Create(player *models.Player) error
	Update(player *models.Player) error
	Ban(id int) error
	Unban(id int) error
}
type UserStorage interface {
	GetByUsername(username string) (*models.User, error)
	GetByCustomUID(customUID string) (*models.User, error)
	Create(user *models.User) error
	Update(user *models.User) error
	Count() (int, error)
}
type OperationLogStorage interface {
	Create(log *models.OperationLog) error
	GetByUserID(userID int, limit int) ([]models.OperationLog, error)
	GetRecent(limit int) ([]models.OperationLog, error)
}

type BackupRecordStorage interface {
	Upsert(record *models.BackupRecord) error
	GetByID(id string) (*models.BackupRecord, error)
	GetByIDs(ids []string) (map[string]models.BackupRecord, error)
	UpdateRemoteState(id string, storageType string, uploadStatus string, uploadError string, remoteBucket string, remoteKey string, remoteETag string, remoteURL string, uploadedAt *time.Time) error
	UpdateVerification(id string, verifiedAt time.Time) error
	Delete(id string) error
}
