package storage
import "terraria-panel/models"
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
	Create(user *models.User) error
	Update(user *models.User) error
	Count() (int, error)
}
type OperationLogStorage interface {
	Create(log *models.OperationLog) error
	GetByUserID(userID int, limit int) ([]models.OperationLog, error)
	GetRecent(limit int) ([]models.OperationLog, error)
}
