package storage
import (
	"database/sql"
	"terraria-panel/models"
	"time"
)
type PlayerStatsStorage interface {
	Create(stats *models.PlayerStats) error
	GetByPlayerID(playerID int) (*models.PlayerStats, error)
	Update(stats *models.PlayerStats) error
	IncrementPlayTime(playerID int, seconds int) error
	IncrementLoginCount(playerID int) error
	UpdateLastLogin(playerID int, loginTime time.Time) error
	UpdateLastLogout(playerID int, logoutTime time.Time) error
	GetTopByPlayTime(limit int) ([]*models.PlayerStats, error)
	GetTopByLoginCount(limit int) ([]*models.PlayerStats, error)
	GetRecentActive(limit int) ([]*models.PlayerStats, error)
	GetAll(limit, offset int) ([]*models.PlayerStats, int, error)
	Delete(playerID int) error
}
type SQLitePlayerStatsStorage struct {
	db *sql.DB
}
func NewSQLitePlayerStatsStorage(db *sql.DB) *SQLitePlayerStatsStorage {
	return &SQLitePlayerStatsStorage{db: db}
}
func (s *SQLitePlayerStatsStorage) Create(stats *models.PlayerStats) error {
	query := `
		INSERT INTO player_stats (player_id, total_play_time, login_count, last_login_time, last_logout_time, first_seen)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query,
		stats.PlayerID,
		stats.TotalPlayTime,
		stats.LoginCount,
		stats.LastLoginTime,
		stats.LastLogoutTime,
		stats.FirstSeen,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	stats.ID = int(id)
	return nil
}
func (s *SQLitePlayerStatsStorage) GetByPlayerID(playerID int) (*models.PlayerStats, error) {
	query := `
		SELECT id, player_id, total_play_time, login_count, last_login_time, last_logout_time, first_seen, updated_at
		FROM player_stats
		WHERE player_id = ?
	`
	stats := &models.PlayerStats{}
	err := s.db.QueryRow(query, playerID).Scan(
		&stats.ID,
		&stats.PlayerID,
		&stats.TotalPlayTime,
		&stats.LoginCount,
		&stats.LastLoginTime,
		&stats.LastLogoutTime,
		&stats.FirstSeen,
		&stats.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return stats, nil
}
func (s *SQLitePlayerStatsStorage) Update(stats *models.PlayerStats) error {
	query := `
		UPDATE player_stats
		SET total_play_time = ?, login_count = ?, last_login_time = ?, last_logout_time = ?, updated_at = CURRENT_TIMESTAMP
		WHERE player_id = ?
	`
	_, err := s.db.Exec(query,
		stats.TotalPlayTime,
		stats.LoginCount,
		stats.LastLoginTime,
		stats.LastLogoutTime,
		stats.PlayerID,
	)
	return err
}
func (s *SQLitePlayerStatsStorage) IncrementPlayTime(playerID int, seconds int) error {
	query := `
		UPDATE player_stats
		SET total_play_time = total_play_time + ?, updated_at = CURRENT_TIMESTAMP
		WHERE player_id = ?
	`
	_, err := s.db.Exec(query, seconds, playerID)
	return err
}
func (s *SQLitePlayerStatsStorage) IncrementLoginCount(playerID int) error {
	query := `
		UPDATE player_stats
		SET login_count = login_count + 1, updated_at = CURRENT_TIMESTAMP
		WHERE player_id = ?
	`
	_, err := s.db.Exec(query, playerID)
	return err
}
func (s *SQLitePlayerStatsStorage) UpdateLastLogin(playerID int, loginTime time.Time) error {
	query := `
		UPDATE player_stats
		SET last_login_time = ?, updated_at = CURRENT_TIMESTAMP
		WHERE player_id = ?
	`
	_, err := s.db.Exec(query, loginTime, playerID)
	return err
}
func (s *SQLitePlayerStatsStorage) UpdateLastLogout(playerID int, logoutTime time.Time) error {
	query := `
		UPDATE player_stats
		SET last_logout_time = ?, updated_at = CURRENT_TIMESTAMP
		WHERE player_id = ?
	`
	_, err := s.db.Exec(query, logoutTime, playerID)
	return err
}
func (s *SQLitePlayerStatsStorage) GetTopByPlayTime(limit int) ([]*models.PlayerStats, error) {
	query := `
		SELECT ps.id, ps.player_id, ps.total_play_time, ps.login_count, ps.last_login_time, ps.last_logout_time, ps.first_seen, ps.updated_at, p.name
		FROM player_stats ps
		JOIN players p ON ps.player_id = p.id
		ORDER BY ps.total_play_time DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statsList := []*models.PlayerStats{}
	for rows.Next() {
		stats := &models.PlayerStats{}
		err := rows.Scan(
			&stats.ID,
			&stats.PlayerID,
			&stats.TotalPlayTime,
			&stats.LoginCount,
			&stats.LastLoginTime,
			&stats.LastLogoutTime,
			&stats.FirstSeen,
			&stats.UpdatedAt,
			&stats.PlayerName,
		)
		if err != nil {
			return nil, err
		}
		statsList = append(statsList, stats)
	}
	return statsList, nil
}
func (s *SQLitePlayerStatsStorage) GetTopByLoginCount(limit int) ([]*models.PlayerStats, error) {
	query := `
		SELECT ps.id, ps.player_id, ps.total_play_time, ps.login_count, ps.last_login_time, ps.last_logout_time, ps.first_seen, ps.updated_at, p.name
		FROM player_stats ps
		JOIN players p ON ps.player_id = p.id
		ORDER BY ps.login_count DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statsList := []*models.PlayerStats{}
	for rows.Next() {
		stats := &models.PlayerStats{}
		err := rows.Scan(
			&stats.ID,
			&stats.PlayerID,
			&stats.TotalPlayTime,
			&stats.LoginCount,
			&stats.LastLoginTime,
			&stats.LastLogoutTime,
			&stats.FirstSeen,
			&stats.UpdatedAt,
			&stats.PlayerName,
		)
		if err != nil {
			return nil, err
		}
		statsList = append(statsList, stats)
	}
	return statsList, nil
}
func (s *SQLitePlayerStatsStorage) GetRecentActive(limit int) ([]*models.PlayerStats, error) {
	query := `
		SELECT ps.id, ps.player_id, ps.total_play_time, ps.login_count, ps.last_login_time, ps.last_logout_time, ps.first_seen, ps.updated_at, p.name
		FROM player_stats ps
		JOIN players p ON ps.player_id = p.id
		WHERE ps.last_login_time IS NOT NULL
		ORDER BY ps.last_login_time DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statsList := []*models.PlayerStats{}
	for rows.Next() {
		stats := &models.PlayerStats{}
		err := rows.Scan(
			&stats.ID,
			&stats.PlayerID,
			&stats.TotalPlayTime,
			&stats.LoginCount,
			&stats.LastLoginTime,
			&stats.LastLogoutTime,
			&stats.FirstSeen,
			&stats.UpdatedAt,
			&stats.PlayerName,
		)
		if err != nil {
			return nil, err
		}
		statsList = append(statsList, stats)
	}
	return statsList, nil
}
func (s *SQLitePlayerStatsStorage) GetAll(limit, offset int) ([]*models.PlayerStats, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM player_stats`
	err := s.db.QueryRow(countQuery).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	query := `
		SELECT ps.id, ps.player_id, ps.total_play_time, ps.login_count, ps.last_login_time, ps.last_logout_time, ps.first_seen, ps.updated_at, p.name
		FROM player_stats ps
		JOIN players p ON ps.player_id = p.id
		ORDER BY ps.total_play_time DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	statsList := []*models.PlayerStats{}
	for rows.Next() {
		stats := &models.PlayerStats{}
		err := rows.Scan(
			&stats.ID,
			&stats.PlayerID,
			&stats.TotalPlayTime,
			&stats.LoginCount,
			&stats.LastLoginTime,
			&stats.LastLogoutTime,
			&stats.FirstSeen,
			&stats.UpdatedAt,
			&stats.PlayerName,
		)
		if err != nil {
			return nil, 0, err
		}
		statsList = append(statsList, stats)
	}
	return statsList, total, nil
}
func (s *SQLitePlayerStatsStorage) Delete(playerID int) error {
	query := `DELETE FROM player_stats WHERE player_id = ?`
	_, err := s.db.Exec(query, playerID)
	return err
}
