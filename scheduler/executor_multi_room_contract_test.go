package scheduler

import (
	"reflect"
	"testing"

	"terraria-panel/models"
)

type multiRoomRestartHandler struct{ roomIDs []int }

func (h *multiRoomRestartHandler) RestartRoom(roomID int) error {
	h.roomIDs = append(h.roomIDs, roomID)
	return nil
}

type multiRoomCleanupBackupHandler struct{ roomIDs []int }

func (h *multiRoomCleanupBackupHandler) CleanupOldBackups(roomID, _ int) error {
	h.roomIDs = append(h.roomIDs, roomID)
	return nil
}

type multiRoomCleanupLogHandler struct{ roomIDs []int }

func (h *multiRoomCleanupLogHandler) CleanupOldLogs(roomID, _ int) error {
	h.roomIDs = append(h.roomIDs, roomID)
	return nil
}

type multiRoomBroadcastHandler struct{ roomIDs []int }

func (h *multiRoomBroadcastHandler) SendBroadcast(roomID int, _ string) error {
	h.roomIDs = append(h.roomIDs, roomID)
	return nil
}

type multiRoomCommandHandler struct{ roomIDs []int }

func (h *multiRoomCommandHandler) ExecuteCommand(roomID int, _ string) error {
	h.roomIDs = append(h.roomIDs, roomID)
	return nil
}

func TestScheduledNonBackupTasksAcceptMultipleRoomsAndAllRooms(t *testing.T) {
	store := &backupContractRoomStorage{rooms: []models.Room{{ID: 3}, {ID: 8}}}
	restart := &multiRoomRestartHandler{}
	broadcast := &multiRoomBroadcastHandler{}
	command := &multiRoomCommandHandler{}
	executor := &TaskExecutor{
		roomStorage:          store,
		restartHandler:       restart,
		broadcastHandler:     broadcast,
		customCommandHandler: command,
	}

	if err := executor.executeRestart(map[string]interface{}{"roomIds": []interface{}{float64(3), float64(8)}}); err != nil {
		t.Fatalf("execute multi-room restart: %v", err)
	}
	if !reflect.DeepEqual(restart.roomIDs, []int{3, 8}) {
		t.Fatalf("restart room IDs=%v, want [3 8]", restart.roomIDs)
	}

	if err := executor.executeBroadcast(map[string]interface{}{
		"roomIds": []interface{}{float64(0)},
		"message": "server announcement",
	}); err != nil {
		t.Fatalf("execute all-room broadcast: %v", err)
	}
	if !reflect.DeepEqual(broadcast.roomIDs, []int{3, 8}) {
		t.Fatalf("broadcast room IDs=%v, want [3 8]", broadcast.roomIDs)
	}

	if err := executor.executeCustomCommand(map[string]interface{}{
		"roomIds": []interface{}{float64(3), float64(8)},
		"command": "save",
	}); err != nil {
		t.Fatalf("execute multi-room command: %v", err)
	}
	if !reflect.DeepEqual(command.roomIDs, []int{3, 8}) {
		t.Fatalf("command room IDs=%v, want [3 8]", command.roomIDs)
	}
}

func TestScheduledCleanupTargetsPreserveLegacyRoomIDAndSupportRoomIds(t *testing.T) {
	store := &backupContractRoomStorage{rooms: []models.Room{{ID: 3}, {ID: 8}}}
	backup := &multiRoomCleanupBackupHandler{}
	logs := &multiRoomCleanupLogHandler{}
	executor := &TaskExecutor{
		roomStorage:          store,
		cleanupBackupHandler: backup,
		cleanupLogHandler:    logs,
	}

	if err := executor.executeCleanupBackup(map[string]interface{}{
		"roomIds":    []interface{}{float64(8)},
		"daysToKeep": float64(7),
	}); err != nil {
		t.Fatalf("execute selected backup cleanup: %v", err)
	}
	if !reflect.DeepEqual(backup.roomIDs, []int{8}) {
		t.Fatalf("backup cleanup room IDs=%v, want [8]", backup.roomIDs)
	}

	if err := executor.executeCleanupLog(map[string]interface{}{
		"roomIds":    []interface{}{float64(0)},
		"daysToKeep": float64(7),
	}); err != nil {
		t.Fatalf("execute all-room log cleanup: %v", err)
	}
	if !reflect.DeepEqual(logs.roomIDs, []int{0, 3, 8}) {
		t.Fatalf("log cleanup room IDs=%v, want [0 3 8]", logs.roomIDs)
	}

	if err := executor.executeCleanupLog(map[string]interface{}{
		"roomId":     float64(0),
		"daysToKeep": float64(7),
	}); err != nil {
		t.Fatalf("execute legacy panel log cleanup: %v", err)
	}
	if !reflect.DeepEqual(logs.roomIDs, []int{0, 3, 8, 0}) {
		t.Fatalf("legacy log cleanup room IDs=%v, want [0 3 8 0]", logs.roomIDs)
	}
}
