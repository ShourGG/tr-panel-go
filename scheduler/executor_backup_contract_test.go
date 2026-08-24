package scheduler

import (
	"sort"
	"testing"

	"terraria-panel/models"
)

type backupContractRoomStorage struct {
	rooms []models.Room
}

func (s *backupContractRoomStorage) GetAll() ([]models.Room, error) {
	return append([]models.Room(nil), s.rooms...), nil
}
func (s *backupContractRoomStorage) GetByID(id int) (*models.Room, error) {
	for _, room := range s.rooms {
		if room.ID == id {
			copy := room
			return &copy, nil
		}
	}
	return nil, nil
}
func (s *backupContractRoomStorage) Create(*models.Room) error           { return nil }
func (s *backupContractRoomStorage) Update(*models.Room) error           { return nil }
func (s *backupContractRoomStorage) Delete(int) error                    { return nil }
func (s *backupContractRoomStorage) UpdateStatus(int, string, int) error { return nil }
func (s *backupContractRoomStorage) UpdateAdminToken(int, string) error  { return nil }

type backupContractHandler struct {
	roomIDs []int
}

func (h *backupContractHandler) CreateBackup(roomID int, _, _ string) error {
	h.roomIDs = append(h.roomIDs, roomID)
	return nil
}

func TestExecuteBackupTreatsZeroOrEmptyAsAllRooms(t *testing.T) {
	store := &backupContractRoomStorage{rooms: []models.Room{{ID: 3}, {ID: 8}}}
	testCases := []struct {
		name string
		raw  interface{}
	}{
		{"explicit zero sentinel", []interface{}{float64(0), float64(3)}},
		{"empty selection", []interface{}{}},
		{"missing selection", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &backupContractHandler{}
			executor := &TaskExecutor{roomStorage: store, backupHandler: handler}
			if err := executor.executeBackup(map[string]interface{}{"roomIds": tc.raw}); err != nil {
				t.Fatalf("execute backup: %v", err)
			}
			sort.Ints(handler.roomIDs)
			if got, want := len(handler.roomIDs), 2; got != want || handler.roomIDs[0] != 3 || handler.roomIDs[1] != 8 {
				t.Fatalf("backup room IDs=%v, want [3 8]", handler.roomIDs)
			}
		})
	}
}

func TestExecuteBackupUsesOnlyRequestedPositiveUniqueRooms(t *testing.T) {
	store := &backupContractRoomStorage{rooms: []models.Room{{ID: 3}, {ID: 8}}}
	handler := &backupContractHandler{}
	executor := &TaskExecutor{roomStorage: store, backupHandler: handler}
	if err := executor.executeBackup(map[string]interface{}{
		"roomIds": []int{8, 8, -4},
	}); err != nil {
		t.Fatalf("execute backup: %v", err)
	}
	if len(handler.roomIDs) != 1 || handler.roomIDs[0] != 8 {
		t.Fatalf("backup room IDs=%v, want [8]", handler.roomIDs)
	}
}
