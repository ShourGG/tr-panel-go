package storage
import (
	"database/sql"
	"terraria-panel/models"
	"time"
)
type PlayerSessionStorage interface {
	Create(session *models.PlayerSession) error
	GetByID(id int) (*models.PlayerSession, error)
	GetByPlayerID(playerID int, limit, offset int) ([]*models.PlayerSession, int, error)
	GetActiveSession(playerID, roomID int) (*models.PlayerSession, error)
	UpdateLeaveTime(id int, leaveTime time.Time, duration int) error
	GetAll(limit, offset int) ([]*models.PlayerSession, int, error)
	Delete(id int) error
}
type SQLitePlayerSessionStorage struct {
	db *sql.DB
}
func NewSQLitePlayerSessionStorage(db *sql.DB) *SQLitePlayerSessionStorage {
	return &SQLitePlayerSessionStorage{db: db}
}
func (s *SQLitePlayerSessionStorage) Create(session *models.PlayerSession) error {
	query := `
		INSERT INTO player_sessions (player_id, room_id, join_time, leave_time, duration, ip_address)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, 
		session.PlayerID, 
		session.RoomID, 
		session.JoinTime, 
		session.LeaveTime, 
		session.Duration, 
		session.IPAddress,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	session.ID = int(id)
	return nil
}
func (s *SQLitePlayerSessionStorage) GetByID(id int) (*models.PlayerSession, error) {
	query := `
		SELECT id, player_id, room_id, join_time, leave_time, duration, ip_address, created_at
		FROM player_sessions
		WHERE id = ?
	`
	session := &models.PlayerSession{}
	err := s.db.QueryRow(query, id).Scan(
		&session.ID,
		&session.PlayerID,
		&session.RoomID,
		&session.JoinTime,
		&session.LeaveTime,
		&session.Duration,
		&session.IPAddress,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}
func (s *SQLitePlayerSessionStorage) GetByPlayerID(playerID int, limit, offset int) ([]*models.PlayerSession, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM player_sessions WHERE player_id = ?`
	err := s.db.QueryRow(countQuery, playerID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	query := `
		SELECT id, player_id, room_id, join_time, leave_time, duration, ip_address, created_at
		FROM player_sessions
		WHERE player_id = ?
		ORDER BY join_time DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, playerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	sessions := []*models.PlayerSession{}
	for rows.Next() {
		session := &models.PlayerSession{}
		err := rows.Scan(
			&session.ID,
			&session.PlayerID,
			&session.RoomID,
			&session.JoinTime,
			&session.LeaveTime,
			&session.Duration,
			&session.IPAddress,
			&session.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, session)
	}
	return sessions, total, nil
}
func (s *SQLitePlayerSessionStorage) GetActiveSession(playerID, roomID int) (*models.PlayerSession, error) {
	query := `
		SELECT id, player_id, room_id, join_time, leave_time, duration, ip_address, created_at
		FROM player_sessions
		WHERE player_id = ? AND room_id = ? AND leave_time IS NULL
		ORDER BY join_time DESC
		LIMIT 1
	`
	session := &models.PlayerSession{}
	err := s.db.QueryRow(query, playerID, roomID).Scan(
		&session.ID,
		&session.PlayerID,
		&session.RoomID,
		&session.JoinTime,
		&session.LeaveTime,
		&session.Duration,
		&session.IPAddress,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}
func (s *SQLitePlayerSessionStorage) UpdateLeaveTime(id int, leaveTime time.Time, duration int) error {
	query := `
		UPDATE player_sessions
		SET leave_time = ?, duration = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query, leaveTime, duration, id)
	return err
}
func (s *SQLitePlayerSessionStorage) GetAll(limit, offset int) ([]*models.PlayerSession, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM player_sessions`
	err := s.db.QueryRow(countQuery).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	query := `
		SELECT id, player_id, room_id, join_time, leave_time, duration, ip_address, created_at
		FROM player_sessions
		ORDER BY join_time DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	sessions := []*models.PlayerSession{}
	for rows.Next() {
		session := &models.PlayerSession{}
		err := rows.Scan(
			&session.ID,
			&session.PlayerID,
			&session.RoomID,
			&session.JoinTime,
			&session.LeaveTime,
			&session.Duration,
			&session.IPAddress,
			&session.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, session)
	}
	return sessions, total, nil
}
func (s *SQLitePlayerSessionStorage) Delete(id int) error {
	query := `DELETE FROM player_sessions WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}
