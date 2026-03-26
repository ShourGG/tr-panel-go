package api

import (
	"strings"
	"sync"
	"terraria-panel/models"
	"terraria-panel/utils"
)

type roomPreparingTracker struct {
	mu      sync.RWMutex
	roomIDs map[int]struct{}
}

var preparingRooms = &roomPreparingTracker{
	roomIDs: make(map[int]struct{}),
}

var roomReadyKeywords = []string{
	"服务器已启动",
	"server started",
	"listening on port",
	"type 'help' for a list of commands",
}

func markRoomPreparing(roomID int) {
	preparingRooms.mu.Lock()
	defer preparingRooms.mu.Unlock()
	preparingRooms.roomIDs[roomID] = struct{}{}
}

func clearRoomPreparing(roomID int) {
	preparingRooms.mu.Lock()
	defer preparingRooms.mu.Unlock()
	delete(preparingRooms.roomIDs, roomID)
}

func isRoomPreparing(roomID int) bool {
	preparingRooms.mu.RLock()
	defer preparingRooms.mu.RUnlock()
	_, exists := preparingRooms.roomIDs[roomID]
	return exists
}

func isRoomActiveStatus(status string) bool {
	return status == "running" || status == "preparing"
}

func containsAnyKeyword(line string, keywords []string) bool {
	lowerLine := strings.ToLower(line)
	for _, keyword := range keywords {
		if strings.Contains(lowerLine, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func promotePreparingRoomToRunning(roomID int) {
	clearRoomPreparing(roomID)

	if roomStorage == nil {
		return
	}

	if process, exists := utils.GetProcess(roomID); exists && process.IsRunning() {
		_ = roomStorage.UpdateStatus(roomID, "running", process.GetPID())
	}
}

func syncRoomRuntimeState(room *models.Room) bool {
	previousStatus := room.Status
	previousPID := room.PID

	if process, exists := utils.GetProcess(room.ID); exists && process.IsRunning() {
		desiredStatus := "running"
		if isRoomPreparing(room.ID) {
			desiredStatus = "preparing"
		}

		room.Status = desiredStatus
		room.PID = process.GetPID()

		if roomStorage != nil && (previousStatus != desiredStatus || previousPID != room.PID) {
			_ = roomStorage.UpdateStatus(room.ID, desiredStatus, room.PID)
		}

		return true
	}

	clearRoomPreparing(room.ID)
	room.Status = "stopped"
	room.PID = 0

	if roomStorage != nil && (previousStatus != "stopped" || previousPID != 0) {
		finalizeRoomPlayerActivity(room.ID)
		_ = roomStorage.UpdateStatus(room.ID, "stopped", 0)
	}

	return false
}
