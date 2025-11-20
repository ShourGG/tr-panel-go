package storage
import (
	"database/sql"
	"terraria-panel/models"
	"time"
)
type SQLiteRoomStorage struct {
	db *sql.DB
}
func NewSQLiteRoomStorage(db *sql.DB) *SQLiteRoomStorage {
	return &SQLiteRoomStorage{db: db}
}
func (s *SQLiteRoomStorage) GetAll() ([]models.Room, error) {
	query := `
		SELECT id, name, server_type, world_file, port, max_players,
		       password, mod_profile, COALESCE(world_size, 'medium'), COALESCE(difficulty, 'normal'),
		       COALESCE(evil_type, 'corruption'), status, pid, start_time, COALESCE(admin_token, ''), created_at, updated_at
		FROM rooms
		ORDER BY id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rooms := []models.Room{}
	for rows.Next() {
		var room models.Room
		var startTime sql.NullTime
		err := rows.Scan(
			&room.ID, &room.Name, &room.ServerType, &room.WorldFile,
			&room.Port, &room.MaxPlayers, &room.Password, &room.ModProfile,
			&room.WorldSize, &room.Difficulty, &room.EvilType,
			&room.Status, &room.PID, &startTime, &room.AdminToken, &room.CreatedAt, &room.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if startTime.Valid {
			room.StartTime = &startTime.Time
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}
func (s *SQLiteRoomStorage) GetByID(id int) (*models.Room, error) {
	query := `
		SELECT id, name, server_type, world_file, port, max_players,
		       password, mod_profile, COALESCE(world_size, 'medium'), COALESCE(difficulty, 'normal'),
		       COALESCE(evil_type, 'corruption'), status, pid, start_time, COALESCE(admin_token, ''), created_at, updated_at
		FROM rooms
		WHERE id = ?
	`
	var room models.Room
	var startTime sql.NullTime
	err := s.db.QueryRow(query, id).Scan(
		&room.ID, &room.Name, &room.ServerType, &room.WorldFile,
		&room.Port, &room.MaxPlayers, &room.Password, &room.ModProfile,
		&room.WorldSize, &room.Difficulty, &room.EvilType,
		&room.Status, &room.PID, &startTime, &room.AdminToken, &room.CreatedAt, &room.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if startTime.Valid {
		room.StartTime = &startTime.Time
	}
	return &room, nil
}
func (s *SQLiteRoomStorage) Create(room *models.Room) error {
	query := `
		INSERT INTO rooms (name, server_type, world_file, port, max_players, password, mod_profile, 
		                  world_size, difficulty, evil_type, status, pid)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if room.WorldSize == "" {
		room.WorldSize = "medium"
	}
	if room.Difficulty == "" {
		room.Difficulty = "normal"
	}
	if room.EvilType == "" {
		room.EvilType = "corruption"
	}
	result, err := s.db.Exec(
		query,
		room.Name, room.ServerType, room.WorldFile, room.Port,
		room.MaxPlayers, room.Password, room.ModProfile,
		room.WorldSize, room.Difficulty, room.EvilType,
		room.Status, room.PID,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	room.ID = int(id)
	room.CreatedAt = time.Now()
	room.UpdatedAt = time.Now()
	return nil
}
func (s *SQLiteRoomStorage) Update(room *models.Room) error {
	query := `
		UPDATE rooms
		SET name = ?, server_type = ?, world_file = ?, port = ?, max_players = ?,
		    password = ?, mod_profile = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := s.db.Exec(
		query,
		room.Name, room.ServerType, room.WorldFile, room.Port,
		room.MaxPlayers, room.Password, room.ModProfile, room.ID,
	)
	return err
}
func (s *SQLiteRoomStorage) Delete(id int) error {
	query := `DELETE FROM rooms WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}
func (s *SQLiteRoomStorage) UpdateStatus(id int, status string, pid int) error {
	var query string
	if status == "running" {
		query = `UPDATE rooms SET status = ?, pid = ?, start_time = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	} else {
		query = `UPDATE rooms SET status = ?, pid = ?, start_time = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	}
	_, err := s.db.Exec(query, status, pid, id)
	return err
}
func (s *SQLiteRoomStorage) UpdateAdminToken(id int, token string) error {
	query := `UPDATE rooms SET admin_token = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, token, id)
	return err
}
