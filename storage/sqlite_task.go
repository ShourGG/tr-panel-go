package storage
import (
	"database/sql"
	"terraria-panel/models"
	"time"
)
type TaskStorage interface {
	GetAll() ([]models.ScheduledTask, error)
	GetByID(id int) (*models.ScheduledTask, error)
	GetEnabled() ([]models.ScheduledTask, error)
	Create(task *models.ScheduledTask) error
	Update(task *models.ScheduledTask) error
	Delete(id int) error
	CreateLog(log *models.TaskExecutionLog) error
	GetLogs(taskID int, limit int) ([]models.TaskExecutionLog, error)
	DeleteLogs(taskID int) error
}
type SQLiteTaskStorage struct {
	db *sql.DB
}
func NewSQLiteTaskStorage(db *sql.DB) TaskStorage {
	return &SQLiteTaskStorage{db: db}
}
func (s *SQLiteTaskStorage) GetAll() ([]models.ScheduledTask, error) {
	query := `
		SELECT id, name, type, enabled, cron_expression, params, description,
		       created_at, updated_at, last_run_at, next_run_at, last_run_status,
		       last_run_error, run_count, success_count, failed_count
		FROM scheduled_tasks
		ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []models.ScheduledTask
	for rows.Next() {
		var task models.ScheduledTask
		var lastRunAt, nextRunAt sql.NullTime
		var lastRunStatus, lastRunError sql.NullString
		err := rows.Scan(
			&task.ID, &task.Name, &task.Type, &task.Enabled, &task.CronExpression,
			&task.Params, &task.Description, &task.CreatedAt, &task.UpdatedAt,
			&lastRunAt, &nextRunAt, &lastRunStatus, &lastRunError,
			&task.RunCount, &task.SuccessCount, &task.FailedCount,
		)
		if err != nil {
			return nil, err
		}
		if lastRunAt.Valid {
			task.LastRunAt = &lastRunAt.Time
		}
		if nextRunAt.Valid {
			task.NextRunAt = &nextRunAt.Time
		}
		if lastRunStatus.Valid {
			task.LastRunStatus = lastRunStatus.String
		}
		if lastRunError.Valid {
			task.LastRunError = lastRunError.String
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}
func (s *SQLiteTaskStorage) GetByID(id int) (*models.ScheduledTask, error) {
	query := `
		SELECT id, name, type, enabled, cron_expression, params, description,
		       created_at, updated_at, last_run_at, next_run_at, last_run_status,
		       last_run_error, run_count, success_count, failed_count
		FROM scheduled_tasks
		WHERE id = ?
	`
	var task models.ScheduledTask
	var lastRunAt, nextRunAt sql.NullTime
	var lastRunStatus, lastRunError sql.NullString
	err := s.db.QueryRow(query, id).Scan(
		&task.ID, &task.Name, &task.Type, &task.Enabled, &task.CronExpression,
		&task.Params, &task.Description, &task.CreatedAt, &task.UpdatedAt,
		&lastRunAt, &nextRunAt, &lastRunStatus, &lastRunError,
		&task.RunCount, &task.SuccessCount, &task.FailedCount,
	)
	if err != nil {
		return nil, err
	}
	if lastRunAt.Valid {
		task.LastRunAt = &lastRunAt.Time
	}
	if nextRunAt.Valid {
		task.NextRunAt = &nextRunAt.Time
	}
	if lastRunStatus.Valid {
		task.LastRunStatus = lastRunStatus.String
	}
	if lastRunError.Valid {
		task.LastRunError = lastRunError.String
	}
	return &task, nil
}
func (s *SQLiteTaskStorage) GetEnabled() ([]models.ScheduledTask, error) {
	query := `
		SELECT id, name, type, enabled, cron_expression, params, description,
		       created_at, updated_at, last_run_at, next_run_at, last_run_status,
		       last_run_error, run_count, success_count, failed_count
		FROM scheduled_tasks
		WHERE enabled = 1
		ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []models.ScheduledTask
	for rows.Next() {
		var task models.ScheduledTask
		var lastRunAt, nextRunAt sql.NullTime
		var lastRunStatus, lastRunError sql.NullString
		err := rows.Scan(
			&task.ID, &task.Name, &task.Type, &task.Enabled, &task.CronExpression,
			&task.Params, &task.Description, &task.CreatedAt, &task.UpdatedAt,
			&lastRunAt, &nextRunAt, &lastRunStatus, &lastRunError,
			&task.RunCount, &task.SuccessCount, &task.FailedCount,
		)
		if err != nil {
			return nil, err
		}
		if lastRunAt.Valid {
			task.LastRunAt = &lastRunAt.Time
		}
		if nextRunAt.Valid {
			task.NextRunAt = &nextRunAt.Time
		}
		if lastRunStatus.Valid {
			task.LastRunStatus = lastRunStatus.String
		}
		if lastRunError.Valid {
			task.LastRunError = lastRunError.String
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}
func (s *SQLiteTaskStorage) Create(task *models.ScheduledTask) error {
	query := `
		INSERT INTO scheduled_tasks (
			name, type, enabled, cron_expression, params, description,
			created_at, updated_at, run_count, success_count, failed_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0)
	`
	now := time.Now()
	result, err := s.db.Exec(
		query,
		task.Name, task.Type, task.Enabled, task.CronExpression,
		task.Params, task.Description, now, now,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	task.ID = int(id)
	task.CreatedAt = now
	task.UpdatedAt = now
	task.RunCount = 0
	task.SuccessCount = 0
	task.FailedCount = 0
	return nil
}
func (s *SQLiteTaskStorage) Update(task *models.ScheduledTask) error {
	query := `
		UPDATE scheduled_tasks
		SET name = ?, type = ?, enabled = ?, cron_expression = ?, params = ?,
		    description = ?, updated_at = ?, last_run_at = ?, next_run_at = ?,
		    last_run_status = ?, last_run_error = ?, run_count = ?,
		    success_count = ?, failed_count = ?
		WHERE id = ?
	`
	now := time.Now()
	_, err := s.db.Exec(
		query,
		task.Name, task.Type, task.Enabled, task.CronExpression, task.Params,
		task.Description, now, task.LastRunAt, task.NextRunAt,
		task.LastRunStatus, task.LastRunError, task.RunCount,
		task.SuccessCount, task.FailedCount, task.ID,
	)
	if err == nil {
		task.UpdatedAt = now
	}
	return err
}
func (s *SQLiteTaskStorage) Delete(id int) error {
	if err := s.DeleteLogs(id); err != nil {
		return err
	}
	query := `DELETE FROM scheduled_tasks WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}
func (s *SQLiteTaskStorage) CreateLog(log *models.TaskExecutionLog) error {
	query := `
		INSERT INTO task_execution_logs (
			task_id, status, started_at, finished_at, duration, error_message, output
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(
		query,
		log.TaskID, log.Status, log.StartedAt, log.FinishedAt,
		log.Duration, log.ErrorMessage, log.Output,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	log.ID = int(id)
	return nil
}
func (s *SQLiteTaskStorage) GetLogs(taskID int, limit int) ([]models.TaskExecutionLog, error) {
	query := `
		SELECT id, task_id, status, started_at, finished_at, duration, error_message, output
		FROM task_execution_logs
		WHERE task_id = ?
		ORDER BY started_at DESC
	`
	if limit > 0 {
		query += ` LIMIT ?`
	}
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(query, taskID, limit)
	} else {
		rows, err = s.db.Query(query, taskID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []models.TaskExecutionLog
	for rows.Next() {
		var log models.TaskExecutionLog
		var finishedAt sql.NullTime
		var duration sql.NullInt64
		var errorMessage, output sql.NullString
		err := rows.Scan(
			&log.ID, &log.TaskID, &log.Status, &log.StartedAt,
			&finishedAt, &duration, &errorMessage, &output,
		)
		if err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			log.FinishedAt = &finishedAt.Time
		}
		if duration.Valid {
			log.Duration = int(duration.Int64)
		}
		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}
		if output.Valid {
			log.Output = output.String
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
func (s *SQLiteTaskStorage) DeleteLogs(taskID int) error {
	query := `DELETE FROM task_execution_logs WHERE task_id = ?`
	_, err := s.db.Exec(query, taskID)
	return err
}
