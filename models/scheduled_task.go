package models
import "time"
type ScheduledTask struct {
	ID              int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name            string    `json:"name" gorm:"type:varchar(255);not null"`
	Type            string    `json:"type" gorm:"type:varchar(50);not null"`
	Enabled         bool      `json:"enabled" gorm:"default:true"`
	CronExpression  string    `json:"cronExpression" gorm:"type:varchar(100);not null"`
	Params          string    `json:"params" gorm:"type:text"`
	Description     string    `json:"description" gorm:"type:text"`
	CreatedAt       time.Time `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
	LastRunAt       *time.Time `json:"lastRunAt"`
	NextRunAt       *time.Time `json:"nextRunAt"`
	LastRunStatus   string    `json:"lastRunStatus" gorm:"type:varchar(20)"`
	LastRunError    string    `json:"lastRunError" gorm:"type:text"`
	RunCount        int       `json:"runCount" gorm:"default:0"`
	SuccessCount    int       `json:"successCount" gorm:"default:0"`
	FailedCount     int       `json:"failedCount" gorm:"default:0"`
}
type TaskExecutionLog struct {
	ID           int       `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID       int       `json:"taskId" gorm:"not null;index"`
	Status       string    `json:"status" gorm:"type:varchar(20);not null"`
	StartedAt    time.Time `json:"startedAt" gorm:"not null"`
	FinishedAt   *time.Time `json:"finishedAt"`
	Duration     int       `json:"duration"`
	ErrorMessage string    `json:"errorMessage" gorm:"type:text"`
	Output       string    `json:"output" gorm:"type:text"`
}
func (ScheduledTask) TableName() string {
	return "scheduled_tasks"
}
func (TaskExecutionLog) TableName() string {
	return "task_execution_logs"
}
