package storage
import (
	"database/sql"
	"terraria-panel/models"
)
type SQLiteUserStorage struct {
	db *sql.DB
}
func NewSQLiteUserStorage(db *sql.DB) *SQLiteUserStorage {
	return &SQLiteUserStorage{db: db}
}
func (s *SQLiteUserStorage) GetByUsername(username string) (*models.User, error) {
	query := `
		SELECT id, username, password, role, COALESCE(custom_uid, ''), created_at, updated_at
		FROM users
		WHERE username = ?
	`
	var user models.User
	err := s.db.QueryRow(query, username).Scan(
		&user.ID, &user.Username, &user.Password, &user.Role, &user.CustomUID,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func (s *SQLiteUserStorage) GetByCustomUID(customUID string) (*models.User, error) {
	query := `
		SELECT id, username, password, role, COALESCE(custom_uid, ''), created_at, updated_at
		FROM users
		WHERE custom_uid = ?
	`
	var user models.User
	err := s.db.QueryRow(query, customUID).Scan(
		&user.ID, &user.Username, &user.Password, &user.Role, &user.CustomUID,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func (s *SQLiteUserStorage) Create(user *models.User) error {
	query := `
		INSERT INTO users (username, password, role, custom_uid)
		VALUES (?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, user.Username, user.Password, user.Role, user.CustomUID)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = int(id)
	return nil
}
func (s *SQLiteUserStorage) Update(user *models.User) error {
	query := `
		UPDATE users
		SET password = ?, role = ?, custom_uid = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := s.db.Exec(query, user.Password, user.Role, user.CustomUID, user.ID)
	return err
}
func (s *SQLiteUserStorage) Count() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM users`
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}
