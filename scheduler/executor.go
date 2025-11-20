package scheduler
import (
	"encoding/json"
	"fmt"
	"log"
	"terraria-panel/models"
	"terraria-panel/storage"
)
type TaskExecutor struct {
	roomStorage            storage.RoomStorage
	taskStorage            storage.TaskStorage
	backupHandler          BackupHandler
	restartHandler         RestartHandler
	cleanupBackupHandler   CleanupBackupHandler
	cleanupLogHandler      CleanupLogHandler
	broadcastHandler       BroadcastHandler
	customCommandHandler   CustomCommandHandler
}
type BackupHandler interface {
	CreateBackup(roomID int, backupType string, note string) error
}
type RestartHandler interface {
	RestartRoom(roomID int) error
}
type CleanupBackupHandler interface {
	CleanupOldBackups(roomID int, daysToKeep int) error
}
type CleanupLogHandler interface {
	CleanupOldLogs(roomID int, daysToKeep int) error
}
type BroadcastHandler interface {
	SendBroadcast(roomID int, message string) error
}
type CustomCommandHandler interface {
	ExecuteCommand(roomID int, command string) error
}
func NewTaskExecutor(
	roomStorage storage.RoomStorage,
	taskStorage storage.TaskStorage,
	backupHandler BackupHandler,
	restartHandler RestartHandler,
	cleanupBackupHandler CleanupBackupHandler,
	cleanupLogHandler CleanupLogHandler,
	broadcastHandler BroadcastHandler,
	customCommandHandler CustomCommandHandler,
) *TaskExecutor {
	return &TaskExecutor{
		roomStorage:          roomStorage,
		taskStorage:          taskStorage,
		backupHandler:        backupHandler,
		restartHandler:       restartHandler,
		cleanupBackupHandler: cleanupBackupHandler,
		cleanupLogHandler:    cleanupLogHandler,
		broadcastHandler:     broadcastHandler,
		customCommandHandler: customCommandHandler,
	}
}
func (e *TaskExecutor) Execute(task *models.ScheduledTask) error {
	log.Printf("[Executor] Executing task %d (%s) of type %s", task.ID, task.Name, task.Type)
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
		return fmt.Errorf("failed to parse task params: %w", err)
	}
	switch task.Type {
	case "backup":
		return e.executeBackup(params)
	case "restart":
		return e.executeRestart(params)
	case "cleanup_backup":
		return e.executeCleanupBackup(params)
	case "cleanup_log":
		return e.executeCleanupLog(params)
	case "broadcast":
		return e.executeBroadcast(params)
	case "custom_command":
		return e.executeCustomCommand(params)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}
func (e *TaskExecutor) executeBackup(params map[string]interface{}) error {
	log.Println("[Executor] Executing backup task...")
	roomIDs := []int{}
	if roomIDsRaw, ok := params["roomIds"].([]interface{}); ok {
		for _, idRaw := range roomIDsRaw {
			if id, ok := idRaw.(float64); ok {
				roomIDs = append(roomIDs, int(id))
			}
		}
	}
	backupType := "full"
	if bt, ok := params["backupType"].(string); ok {
		backupType = bt
	}
	note := ""
	if n, ok := params["note"].(string); ok {
		note = n
	}
	if len(roomIDs) == 0 {
		log.Println("[Executor] Backing up all rooms...")
		rooms, err := e.roomStorage.GetAll()
		if err != nil {
			return fmt.Errorf("failed to get rooms: %w", err)
		}
		for _, room := range rooms {
			roomIDs = append(roomIDs, room.ID)
		}
	}
	for _, roomID := range roomIDs {
		log.Printf("[Executor] Backing up room %d...", roomID)
		if err := e.backupHandler.CreateBackup(roomID, backupType, note); err != nil {
			log.Printf("[Executor] Failed to backup room %d: %v", roomID, err)
			return fmt.Errorf("failed to backup room %d: %w", roomID, err)
		}
		log.Printf("[Executor] Room %d backed up successfully", roomID)
	}
	log.Printf("[Executor] Backup task completed, backed up %d rooms", len(roomIDs))
	return nil
}
func (e *TaskExecutor) executeRestart(params map[string]interface{}) error {
	log.Println("[Executor] Executing restart task...")
	roomID := 0
	if id, ok := params["roomId"].(float64); ok {
		roomID = int(id)
	}
	if roomID == 0 {
		return fmt.Errorf("room ID is required for restart task")
	}
	log.Printf("[Executor] Restarting room %d...", roomID)
	if err := e.restartHandler.RestartRoom(roomID); err != nil {
		return fmt.Errorf("failed to restart room %d: %w", roomID, err)
	}
	log.Printf("[Executor] Room %d restarted successfully", roomID)
	return nil
}
func (e *TaskExecutor) executeCleanupBackup(params map[string]interface{}) error {
	log.Println("[Executor] Executing cleanup backup task...")
	roomID := 0
	if id, ok := params["roomId"].(float64); ok {
		roomID = int(id)
	}
	daysToKeep := 7
	if days, ok := params["daysToKeep"].(float64); ok {
		daysToKeep = int(days)
	}
	log.Printf("[Executor] Cleaning up backups older than %d days for room %d...", daysToKeep, roomID)
	if err := e.cleanupBackupHandler.CleanupOldBackups(roomID, daysToKeep); err != nil {
		return fmt.Errorf("failed to cleanup old backups: %w", err)
	}
	log.Println("[Executor] Cleanup backup task completed successfully")
	return nil
}
func (e *TaskExecutor) executeCleanupLog(params map[string]interface{}) error {
	log.Println("[Executor] Executing cleanup log task...")
	roomID := 0
	if id, ok := params["roomId"].(float64); ok {
		roomID = int(id)
	}
	daysToKeep := 7
	if days, ok := params["daysToKeep"].(float64); ok {
		daysToKeep = int(days)
	}
	log.Printf("[Executor] Cleaning up logs older than %d days for room %d...", daysToKeep, roomID)
	if err := e.cleanupLogHandler.CleanupOldLogs(roomID, daysToKeep); err != nil {
		return fmt.Errorf("failed to cleanup old logs: %w", err)
	}
	log.Println("[Executor] Cleanup log task completed successfully")
	return nil
}
func (e *TaskExecutor) executeBroadcast(params map[string]interface{}) error {
	log.Println("[Executor] Executing broadcast task...")
	roomID := 0
	if id, ok := params["roomId"].(float64); ok {
		roomID = int(id)
	}
	if roomID == 0 {
		return fmt.Errorf("room ID is required for broadcast task")
	}
	message := ""
	if msg, ok := params["message"].(string); ok {
		message = msg
	}
	if message == "" {
		return fmt.Errorf("message is required for broadcast task")
	}
	log.Printf("[Executor] Sending broadcast to room %d: %s", roomID, message)
	if err := e.broadcastHandler.SendBroadcast(roomID, message); err != nil {
		return fmt.Errorf("failed to send broadcast: %w", err)
	}
	log.Println("[Executor] Broadcast task completed successfully")
	return nil
}
func (e *TaskExecutor) executeCustomCommand(params map[string]interface{}) error {
	log.Println("[Executor] Executing custom command task...")
	roomID := 0
	if id, ok := params["roomId"].(float64); ok {
		roomID = int(id)
	}
	if roomID == 0 {
		return fmt.Errorf("room ID is required for custom command task")
	}
	command := ""
	if cmd, ok := params["command"].(string); ok {
		command = cmd
	}
	if command == "" {
		return fmt.Errorf("command is required for custom command task")
	}
	log.Printf("[Executor] Executing command on room %d: %s", roomID, command)
	if err := e.customCommandHandler.ExecuteCommand(roomID, command); err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	log.Println("[Executor] Custom command task completed successfully")
	return nil
}
