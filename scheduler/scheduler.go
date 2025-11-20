package scheduler
import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"terraria-panel/models"
	"terraria-panel/storage"
	"time"
	"github.com/robfig/cron/v3"
)
type Scheduler struct {
	cron        *cron.Cron
	taskStorage storage.TaskStorage
	executor    *TaskExecutor
	entryMap    map[int]cron.EntryID
	mu          sync.RWMutex
}
func NewScheduler(taskStorage storage.TaskStorage, executor *TaskExecutor) *Scheduler {
	return &Scheduler{
		cron:        cron.New(cron.WithSeconds()),
		taskStorage: taskStorage,
		executor:    executor,
		entryMap:    make(map[int]cron.EntryID),
	}
}
func (s *Scheduler) Start() error {
	log.Println("[Scheduler] Starting scheduler...")
	tasks, err := s.taskStorage.GetEnabled()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}
	log.Printf("[Scheduler] Loaded %d enabled tasks", len(tasks))
	for _, task := range tasks {
		if err := s.AddTask(&task); err != nil {
			log.Printf("[Scheduler] Failed to add task %d: %v", task.ID, err)
		}
	}
	s.cron.Start()
	log.Println("[Scheduler] Scheduler started successfully")
	return nil
}
func (s *Scheduler) Stop() {
	log.Println("[Scheduler] Stopping scheduler...")
	s.cron.Stop()
	log.Println("[Scheduler] Scheduler stopped")
}
func (s *Scheduler) AddTask(task *models.ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, exists := s.entryMap[task.ID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryMap, task.ID)
	}
	entryID, err := s.cron.AddFunc(task.CronExpression, func() {
		s.executeTask(task.ID)
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}
	s.entryMap[task.ID] = entryID
	entry := s.cron.Entry(entryID)
	nextRunAt := entry.Next
	task.NextRunAt = &nextRunAt
	s.taskStorage.Update(task)
	log.Printf("[Scheduler] Added task %d (%s) with cron expression: %s", task.ID, task.Name, task.CronExpression)
	return nil
}
func (s *Scheduler) RemoveTask(taskID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, exists := s.entryMap[taskID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryMap, taskID)
		log.Printf("[Scheduler] Removed task %d", taskID)
	}
}
func (s *Scheduler) ExecuteTaskNow(taskID int) error {
	go s.executeTask(taskID)
	return nil
}
func (s *Scheduler) executeTask(taskID int) {
	log.Printf("[Scheduler] Executing task %d...", taskID)
	task, err := s.taskStorage.GetByID(taskID)
	if err != nil {
		log.Printf("[Scheduler] Failed to get task %d: %v", taskID, err)
		return
	}
	now := time.Now()
	task.LastRunAt = &now
	task.LastRunStatus = "running"
	s.taskStorage.Update(task)
	logEntry := &models.TaskExecutionLog{
		TaskID:    taskID,
		Status:    "running",
		StartedAt: now,
	}
	s.taskStorage.CreateLog(logEntry)
	startTime := time.Now()
	err = s.executor.Execute(task)
	duration := int(time.Since(startTime).Milliseconds())
	finishedAt := time.Now()
	logEntry.FinishedAt = &finishedAt
	logEntry.Duration = duration
	if err != nil {
		task.LastRunStatus = "failed"
		task.LastRunError = err.Error()
		task.FailedCount++
		logEntry.Status = "failed"
		logEntry.ErrorMessage = err.Error()
		log.Printf("[Scheduler] Task %d failed: %v", taskID, err)
	} else {
		task.LastRunStatus = "success"
		task.LastRunError = ""
		task.SuccessCount++
		logEntry.Status = "success"
		log.Printf("[Scheduler] Task %d completed successfully in %dms", taskID, duration)
	}
	task.RunCount++
	s.mu.RLock()
	if entryID, exists := s.entryMap[taskID]; exists {
		entry := s.cron.Entry(entryID)
		nextRunAt := entry.Next
		task.NextRunAt = &nextRunAt
	}
	s.mu.RUnlock()
	s.taskStorage.Update(task)
	s.taskStorage.CreateLog(logEntry)
}
func (s *Scheduler) ReloadTask(taskID int) error {
	task, err := s.taskStorage.GetByID(taskID)
	if err != nil {
		return err
	}
	if task.Enabled {
		return s.AddTask(task)
	} else {
		s.RemoveTask(taskID)
		return nil
	}
}
func ParseTaskParams(paramsJSON string) (map[string]interface{}, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return nil, err
	}
	return params, nil
}
