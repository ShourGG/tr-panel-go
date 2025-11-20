package storage
import (
	"database/sql"
	"terraria-panel/models"
	"time"
)
type PlayerDailyStatsStorage interface {
	Create(stats *models.PlayerDailyStats) error
	GetByDate(date string) (*models.PlayerDailyStats, error)
	Update(stats *models.PlayerDailyStats) error
	GetRange(startDate, endDate string) ([]*models.PlayerDailyStats, error)
	GetRecent(days int) ([]*models.PlayerDailyStats, error)
	Delete(date string) error
}
type SQLitePlayerDailyStatsStorage struct {
	db *sql.DB
}
func NewSQLitePlayerDailyStatsStorage(db *sql.DB) *SQLitePlayerDailyStatsStorage {
	return &SQLitePlayerDailyStatsStorage{db: db}
}
func (s *SQLitePlayerDailyStatsStorage) Create(stats *models.PlayerDailyStats) error {
	query := `
		INSERT INTO player_daily_stats (date, total_players, active_players, new_players, total_play_time)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query,
		stats.Date,
		stats.TotalPlayers,
		stats.ActivePlayers,
		stats.NewPlayers,
		stats.TotalPlayTime,
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
func (s *SQLitePlayerDailyStatsStorage) GetByDate(date string) (*models.PlayerDailyStats, error) {
	query := `
		SELECT id, date, total_players, active_players, new_players, total_play_time, created_at
		FROM player_daily_stats
		WHERE date = ?
	`
	stats := &models.PlayerDailyStats{}
	err := s.db.QueryRow(query, date).Scan(
		&stats.ID,
		&stats.Date,
		&stats.TotalPlayers,
		&stats.ActivePlayers,
		&stats.NewPlayers,
		&stats.TotalPlayTime,
		&stats.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return stats, nil
}
func (s *SQLitePlayerDailyStatsStorage) Update(stats *models.PlayerDailyStats) error {
	query := `
		UPDATE player_daily_stats
		SET total_players = ?, active_players = ?, new_players = ?, total_play_time = ?
		WHERE date = ?
	`
	_, err := s.db.Exec(query,
		stats.TotalPlayers,
		stats.ActivePlayers,
		stats.NewPlayers,
		stats.TotalPlayTime,
		stats.Date,
	)
	return err
}
func (s *SQLitePlayerDailyStatsStorage) GetRange(startDate, endDate string) ([]*models.PlayerDailyStats, error) {
	query := `
		SELECT id, date, total_players, active_players, new_players, total_play_time, created_at
		FROM player_daily_stats
		WHERE date >= ? AND date <= ?
		ORDER BY date ASC
	`
	rows, err := s.db.Query(query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statsList := []*models.PlayerDailyStats{}
	for rows.Next() {
		stats := &models.PlayerDailyStats{}
		err := rows.Scan(
			&stats.ID,
			&stats.Date,
			&stats.TotalPlayers,
			&stats.ActivePlayers,
			&stats.NewPlayers,
			&stats.TotalPlayTime,
			&stats.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		statsList = append(statsList, stats)
	}
	return statsList, nil
}
func (s *SQLitePlayerDailyStatsStorage) GetRecent(days int) ([]*models.PlayerDailyStats, error) {
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	endDate := time.Now().Format("2006-01-02")
	return s.GetRange(startDate, endDate)
}
func (s *SQLitePlayerDailyStatsStorage) Delete(date string) error {
	query := `DELETE FROM player_daily_stats WHERE date = ?`
	_, err := s.db.Exec(query, date)
	return err
}
