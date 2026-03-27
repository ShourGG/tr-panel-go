package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"terraria-panel/db"

	"github.com/gin-gonic/gin"
)

type playerListContractResponse struct {
	Success bool             `json:"success"`
	Data    []PlayerListItem `json:"data"`
	Error   string           `json:"error,omitempty"`
}

type playerDBContractFixture struct {
	roomAlphaID int
	roomBetaID  int
}

func TestGetPlayersDefaultFilterReturnsOnlyOnlineUnbannedPlayersContract(t *testing.T) {
	fixture := setupPlayerDBContractFixture(t)

	response := performPlayerContractRequest(t, "/players", "/players", GetPlayers)
	players := decodePlayerListContractResponse(t, response)

	if len(players) != 1 {
		t.Fatalf("unexpected player count: got %d want 1, body=%s", len(players), response.Body.String())
	}

	alice := players[0]
	if alice.ID != 1 || alice.RoomID != fixture.roomAlphaID || alice.RoomTypeText != "TShock" || !alice.Online || alice.Banned {
		t.Fatalf("unexpected player payload: %#v", alice)
	}
}

func TestGetPlayersOfflineFilterReturnsOnlyOfflineUnbannedPlayersContract(t *testing.T) {
	fixture := setupPlayerDBContractFixture(t)

	response := performPlayerContractRequest(t, "/players", "/players?status=offline", GetPlayers)
	players := decodePlayerListContractResponse(t, response)

	if len(players) != 2 {
		t.Fatalf("unexpected player count: got %d want 2, body=%s", len(players), response.Body.String())
	}

	byID := indexPlayersByID(players)
	assertPlayerListItemContract(t, byID, 2, fixture.roomAlphaID, "TShock", false, false)
	assertPlayerListItemContract(t, byID, 4, fixture.roomBetaID, "原版", false, false)
	if _, exists := byID[3]; exists {
		t.Fatalf("offline filter should exclude banned players: %#v", byID[3])
	}
}

func TestGetPlayersAllByRoomExcludesBannedPlayersContract(t *testing.T) {
	fixture := setupPlayerDBContractFixture(t)

	response := performPlayerContractRequest(t, "/players", fmt.Sprintf("/players?status=all&roomId=%d", fixture.roomAlphaID), GetPlayers)
	players := decodePlayerListContractResponse(t, response)

	if len(players) != 2 {
		t.Fatalf("unexpected player count: got %d want 2, body=%s", len(players), response.Body.String())
	}

	byID := indexPlayersByID(players)
	assertPlayerListItemContract(t, byID, 1, fixture.roomAlphaID, "TShock", true, false)
	assertPlayerListItemContract(t, byID, 2, fixture.roomAlphaID, "TShock", false, false)
	if _, exists := byID[3]; exists {
		t.Fatalf("room-scoped all filter should exclude banned players: %#v", byID[3])
	}
	if _, exists := byID[4]; exists {
		t.Fatalf("room-scoped all filter should exclude players from other rooms: %#v", byID[4])
	}
}

func TestGetBannedPlayersReturnsBanMetadataContract(t *testing.T) {
	fixture := setupPlayerDBContractFixture(t)

	response := performPlayerContractRequest(t, "/players/banned", "/players/banned", GetBannedPlayers)
	players := decodePlayerListContractResponse(t, response)

	if len(players) != 2 {
		t.Fatalf("unexpected player count: got %d want 2, body=%s", len(players), response.Body.String())
	}

	byID := indexPlayersByID(players)
	assertPlayerListItemContract(t, byID, 3, fixture.roomAlphaID, "TShock", true, true)
	assertPlayerListItemContract(t, byID, 5, fixture.roomBetaID, "原版", false, true)

	eve := byID[3]
	if eve.BanReason != "griefing" || eve.BanTime != "2025-01-02 03:04:05" {
		t.Fatalf("unexpected latest ban info for player 3: %#v", eve)
	}
	trent := byID[5]
	if trent.BanReason != "abuse" || trent.BanTime != "2025-01-03 04:05:06" {
		t.Fatalf("unexpected latest ban info for player 5: %#v", trent)
	}
}

func setupPlayerDBContractFixture(t *testing.T) playerDBContractFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	oldDB := db.DB
	dbPath := filepath.Join(t.TempDir(), "panel.db")
	if err := db.Init(dbPath); err != nil {
		t.Fatalf("init test database: %v", err)
	}
	newDB := db.DB
	t.Cleanup(func() {
		if newDB != nil {
			_ = newDB.Close()
		}
		db.DB = oldDB
	})

	mustExecPlayerContractSQL(t, `
		INSERT INTO rooms (id, name, server_type, world_file, port, status)
		VALUES
			(1, 'Alpha Room', 'tshock', 'alpha.wld', 7777, 'running'),
			(2, 'Beta Room', 'vanilla', 'beta.wld', 7778, 'stopped')
	`)
	mustExecPlayerContractSQL(t, `
		INSERT INTO players (id, name, ip, is_banned, room_id, status, last_seen, created_at)
		VALUES
			(1, 'Alice', '10.0.0.1', 0, 1, 'online', '2025-01-02 01:00:00', '2025-01-01 00:00:00'),
			(2, 'Bob', '10.0.0.2', 0, 1, 'offline', '2025-01-02 00:30:00', '2025-01-01 00:10:00'),
			(3, 'Eve', '10.0.0.3', 1, 1, 'online', '2025-01-02 00:20:00', '2025-01-01 00:20:00'),
			(4, 'Mallory', '10.0.0.4', 0, 2, 'offline', '2025-01-01 23:50:00', '2025-01-01 00:30:00'),
			(5, 'Trent', '10.0.0.5', 1, 2, 'offline', '2025-01-01 23:40:00', '2025-01-01 00:40:00')
	`)
	mustExecPlayerContractSQL(t, `
		INSERT INTO player_stats (player_id, login_count, last_login_time, last_logout_time, first_seen, updated_at)
		VALUES
			(1, 10, '2025-01-02 01:00:00', NULL, '2025-01-01 00:00:00', '2025-01-02 01:00:00'),
			(2, 6, '2025-01-02 00:30:00', '2025-01-02 00:45:00', '2025-01-01 00:10:00', '2025-01-02 00:45:00'),
			(3, 8, '2025-01-02 00:20:00', NULL, '2025-01-01 00:20:00', '2025-01-02 00:20:00'),
			(4, 4, '2025-01-01 23:50:00', '2025-01-02 00:05:00', '2025-01-01 00:30:00', '2025-01-02 00:05:00'),
			(5, 3, '2025-01-01 23:40:00', '2025-01-01 23:55:00', '2025-01-01 00:40:00', '2025-01-01 23:55:00')
	`)
	mustExecPlayerContractSQL(t, `
		INSERT INTO activity_logs (type, title, description, room_id, player_name, color, created_at)
		VALUES
			('player_ban', 'Ban Eve', '原因: spawn kill', 1, 'Eve', 'red', '2025-01-01 02:03:04'),
			('player_ban', 'Ban Eve again', '原因: griefing', 1, 'Eve', 'red', '2025-01-02 03:04:05'),
			('player_ban', 'Ban Trent', '原因: abuse', 2, 'Trent', 'red', '2025-01-03 04:05:06')
	`)

	return playerDBContractFixture{roomAlphaID: 1, roomBetaID: 2}
}

func mustExecPlayerContractSQL(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.DB.Exec(query, args...); err != nil {
		t.Fatalf("exec sql: %v\nquery=%s", err, query)
	}
}

func performPlayerContractRequest(t *testing.T, route, target string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	router := gin.New()
	router.GET(route, handler)

	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodePlayerListContractResponse(t *testing.T, response *httptest.ResponseRecorder) []PlayerListItem {
	t.Helper()

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d, body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload playerListContractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("expected success=true, body=%s", response.Body.String())
	}
	return payload.Data
}

func indexPlayersByID(players []PlayerListItem) map[int]PlayerListItem {
	indexed := make(map[int]PlayerListItem, len(players))
	for _, player := range players {
		indexed[player.ID] = player
	}
	return indexed
}

func assertPlayerListItemContract(t *testing.T, players map[int]PlayerListItem, id int, wantRoomID int, wantRoomTypeText string, wantOnline bool, wantBanned bool) {
	t.Helper()

	player, ok := players[id]
	if !ok {
		t.Fatalf("missing player %d in payload: %#v", id, players)
	}
	if player.RoomID != wantRoomID || player.RoomTypeText != wantRoomTypeText || player.Online != wantOnline || player.Banned != wantBanned {
		t.Fatalf("unexpected player payload for %d: %#v", id, player)
	}
}
