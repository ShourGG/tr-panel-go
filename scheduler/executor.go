package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"terraria-panel/models"
	"terraria-panel/storage"
)

type TaskExecutor struct {
	roomStorage          storage.RoomStorage
	taskStorage          storage.TaskStorage
	backupHandler        BackupHandler
	restartHandler       RestartHandler
	cleanupBackupHandler CleanupBackupHandler
	cleanupLogHandler    CleanupLogHandler
	broadcastHandler     BroadcastHandler
	customCommandHandler CustomCommandHandler
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
	roomIDs, allRooms := normalizeBackupRoomIDs(params["roomIds"])
	backupType := "full"
	if bt, ok := params["backupType"].(string); ok {
		backupType = bt
	}
	note := ""
	if n, ok := params["note"].(string); ok {
		note = n
	}
	if allRooms || len(roomIDs) == 0 {
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

// normalizeBackupRoomIDs accepts the JSON-decoded and programmatic forms used
// by scheduled tasks. A zero ID is the explicit "all rooms" sentinel.
func normalizeBackupRoomIDs(raw interface{}) ([]int, bool) {
	values := make([]interface{}, 0)
	switch typed := raw.(type) {
	case nil:
		return nil, true
	case []interface{}:
		values = append(values, typed...)
	case []int:
		for _, value := range typed {
			values = append(values, value)
		}
	case []float64:
		for _, value := range typed {
			values = append(values, value)
		}
	case int, int64, float64, json.Number, string:
		values = append(values, typed)
	default:
		return nil, true
	}

	seen := make(map[int]struct{})
	roomIDs := make([]int, 0, len(values))
	for _, value := range values {
		roomID, ok := parseBackupRoomID(value)
		if !ok || roomID < 0 {
			continue
		}
		if roomID == 0 {
			return nil, true
		}
		if _, exists := seen[roomID]; exists {
			continue
		}
		seen[roomID] = struct{}{}
		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, len(roomIDs) == 0
}

func parseBackupRoomID(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int64:
		return int(value), int64(int(value)) == value
	case float64:
		roomID := int(value)
		return roomID, float64(roomID) == value
	case json.Number:
		valueInt, err := value.Int64()
		return int(valueInt), err == nil && int64(int(valueInt)) == valueInt
	case string:
		valueInt, err := strconv.Atoi(strings.TrimSpace(value))
		return valueInt, err == nil
	default:
		return 0, false
	}
}
func (e *TaskExecutor) executeRestart(params map[string]interface{}) error {
	log.Println("[Executor] Executing restart task...")
	roomIDs, err := e.resolveTaskRoomIDs(params, false)
	if err != nil {
		return fmt.Errorf("%w for restart task", err)
	}
	for _, roomID := range roomIDs {
		log.Printf("[Executor] Restarting room %d...", roomID)
		if err := e.restartHandler.RestartRoom(roomID); err != nil {
			return fmt.Errorf("failed to restart room %d: %w", roomID, err)
		}
		log.Printf("[Executor] Room %d restarted successfully", roomID)
	}
	return nil
}
func (e *TaskExecutor) executeCleanupBackup(params map[string]interface{}) error {
	log.Println("[Executor] Executing cleanup backup task...")
	daysToKeep := 7
	if days, ok := params["daysToKeep"].(float64); ok {
		daysToKeep = int(days)
	}
	selection := normalizeTaskRoomSelection(params)
	if selection.all || selection.legacyZero {
		log.Printf("[Executor] Cleaning up backups older than %d days for all rooms...", daysToKeep)
		if err := e.cleanupBackupHandler.CleanupOldBackups(0, daysToKeep); err != nil {
			return fmt.Errorf("failed to cleanup old backups: %w", err)
		}
		return nil
	}
	if len(selection.roomIDs) == 0 {
		return fmt.Errorf("room ID is required for cleanup backup task")
	}
	for _, roomID := range selection.roomIDs {
		log.Printf("[Executor] Cleaning up backups older than %d days for room %d...", daysToKeep, roomID)
		if err := e.cleanupBackupHandler.CleanupOldBackups(roomID, daysToKeep); err != nil {
			return fmt.Errorf("failed to cleanup old backups for room %d: %w", roomID, err)
		}
	}
	log.Println("[Executor] Cleanup backup task completed successfully")
	return nil
}
func (e *TaskExecutor) executeCleanupLog(params map[string]interface{}) error {
	log.Println("[Executor] Executing cleanup log task...")
	daysToKeep := 7
	if days, ok := params["daysToKeep"].(float64); ok {
		daysToKeep = int(days)
	}
	selection := normalizeTaskRoomSelection(params)
	if selection.all {
		// Keep the panel log covered by the existing roomID=0 handler, then
		// clean each room log independently because roomID=0 historically means
		// the panel log for this handler.
		if err := e.cleanupLogHandler.CleanupOldLogs(0, daysToKeep); err != nil {
			return fmt.Errorf("failed to cleanup panel logs: %w", err)
		}
		roomIDs, err := e.getAllRoomIDs()
		if err != nil {
			return fmt.Errorf("failed to get rooms: %w", err)
		}
		selection.roomIDs = roomIDs
	} else if selection.legacyZero {
		if err := e.cleanupLogHandler.CleanupOldLogs(0, daysToKeep); err != nil {
			return fmt.Errorf("failed to cleanup old logs: %w", err)
		}
		return nil
	} else if len(selection.roomIDs) == 0 {
		return fmt.Errorf("room ID is required for cleanup log task")
	}
	for _, roomID := range selection.roomIDs {
		log.Printf("[Executor] Cleaning up logs older than %d days for room %d...", daysToKeep, roomID)
		if err := e.cleanupLogHandler.CleanupOldLogs(roomID, daysToKeep); err != nil {
			return fmt.Errorf("failed to cleanup old logs for room %d: %w", roomID, err)
		}
	}
	log.Println("[Executor] Cleanup log task completed successfully")
	return nil
}
func (e *TaskExecutor) executeBroadcast(params map[string]interface{}) error {
	log.Println("[Executor] Executing broadcast task...")
	message := ""
	if msg, ok := params["message"].(string); ok {
		message = msg
	}
	if message == "" {
		return fmt.Errorf("message is required for broadcast task")
	}
	roomIDs, err := e.resolveTaskRoomIDs(params, false)
	if err != nil {
		return fmt.Errorf("%w for broadcast task", err)
	}
	for _, roomID := range roomIDs {
		log.Printf("[Executor] Sending broadcast to room %d: %s", roomID, message)
		if err := e.broadcastHandler.SendBroadcast(roomID, message); err != nil {
			return fmt.Errorf("failed to send broadcast to room %d: %w", roomID, err)
		}
	}
	log.Println("[Executor] Broadcast task completed successfully")
	return nil
}
func (e *TaskExecutor) executeCustomCommand(params map[string]interface{}) error {
	log.Println("[Executor] Executing custom command task...")
	command := ""
	if cmd, ok := params["command"].(string); ok {
		command = cmd
	}
	if command == "" {
		return fmt.Errorf("command is required for custom command task")
	}
	roomIDs, err := e.resolveTaskRoomIDs(params, false)
	if err != nil {
		return fmt.Errorf("%w for custom command task", err)
	}
	for _, roomID := range roomIDs {
		log.Printf("[Executor] Executing command on room %d: %s", roomID, command)
		if err := e.customCommandHandler.ExecuteCommand(roomID, command); err != nil {
			return fmt.Errorf("failed to execute command on room %d: %w", roomID, err)
		}
	}
	log.Println("[Executor] Custom command task completed successfully")
	return nil
}

type taskRoomSelection struct {
	roomIDs    []int
	all        bool
	legacyZero bool
}

// New tasks use roomIds so every task type can target multiple rooms. The
// scalar roomId form remains supported for tasks created by older releases.
func normalizeTaskRoomSelection(params map[string]interface{}) taskRoomSelection {
	if raw, ok := params["roomIds"]; ok {
		roomIDs, all := normalizeBackupRoomIDs(raw)
		return taskRoomSelection{roomIDs: roomIDs, all: all}
	}

	raw, ok := params["roomId"]
	if !ok {
		return taskRoomSelection{legacyZero: true}
	}
	roomID, ok := parseBackupRoomID(raw)
	if !ok || roomID < 0 {
		return taskRoomSelection{}
	}
	if roomID == 0 {
		return taskRoomSelection{legacyZero: true}
	}
	return taskRoomSelection{roomIDs: []int{roomID}}
}

func (e *TaskExecutor) resolveTaskRoomIDs(params map[string]interface{}, allowLegacyZero bool) ([]int, error) {
	selection := normalizeTaskRoomSelection(params)
	if selection.all {
		roomIDs, err := e.getAllRoomIDs()
		if err != nil {
			return nil, fmt.Errorf("failed to get rooms: %w", err)
		}
		return roomIDs, nil
	}
	if selection.legacyZero && !allowLegacyZero {
		return nil, fmt.Errorf("room ID is required")
	}
	if len(selection.roomIDs) == 0 {
		return nil, fmt.Errorf("room ID is required")
	}
	return selection.roomIDs, nil
}

func (e *TaskExecutor) getAllRoomIDs() ([]int, error) {
	rooms, err := e.roomStorage.GetAll()
	if err != nil {
		return nil, err
	}
	roomIDs := make([]int, 0, len(rooms))
	for _, room := range rooms {
		if room.ID > 0 {
			roomIDs = append(roomIDs, room.ID)
		}
	}
	return roomIDs, nil
}
