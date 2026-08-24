package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/storage"

	"github.com/gin-gonic/gin"
)

func setupLivePlayerStatsContractDB(t *testing.T) {
	t.Helper()

	oldDB := db.DB
	oldStatsDB := statsDB
	oldSessionStorage := sessionStorage
	oldStatsStorage := statsStorage
	oldDailyStatsStorage := dailyStatsStorage
	oldRoomStorage := roomStorage

	if err := db.Init(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatalf("initialize temporary database: %v", err)
	}
	testDB := db.DB
	InitStatsStorage(testDB)
	roomStorage = storage.NewSQLiteRoomStorage(testDB)

	t.Cleanup(func() {
		_ = testDB.Close()
		db.DB = oldDB
		statsDB = oldStatsDB
		sessionStorage = oldSessionStorage
		statsStorage = oldStatsStorage
		dailyStatsStorage = oldDailyStatsStorage
		roomStorage = oldRoomStorage
	})
}

func createLivePlayerStatsRoom(t *testing.T, name string) *models.Room {
	t.Helper()
	room := &models.Room{
		Name:       name,
		ServerType: "vanilla",
		WorldFile:  name + ".wld",
		Port:       18000 + time.Now().Nanosecond()%1000,
		MaxPlayers: 8,
		Status:     "stopped",
	}
	if err := roomStorage.Create(room); err != nil {
		t.Fatalf("create room: %v", err)
	}
	return room
}

func TestGetRoomsIncludesAggregatedOnlinePlayerCount(t *testing.T) {
	setupLivePlayerStatsContractDB(t)
	firstRoom := createLivePlayerStatsRoom(t, "first-room")
	secondRoom := createLivePlayerStatsRoom(t, "second-room")

	for _, player := range []struct {
		name   string
		roomID int
		status string
	}{
		{"Alice", firstRoom.ID, "online"},
		{"Bob", firstRoom.ID, "online"},
		{"Carol", firstRoom.ID, "offline"},
		{"Dave", secondRoom.ID, "offline"},
	} {
		if _, err := db.DB.Exec(`INSERT INTO players (name, room_id, status) VALUES (?, ?, ?)`, player.name, player.roomID, player.status); err != nil {
			t.Fatalf("insert player %s: %v", player.name, err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/rooms", GetRooms)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rooms", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GetRooms status=%d body=%s", response.Code, response.Body.String())
	}

	var payload struct {
		Success bool          `json:"success"`
		Data    []models.Room `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || len(payload.Data) != 2 {
		t.Fatalf("unexpected room payload: %#v", payload)
	}
	counts := make(map[int]int)
	for _, room := range payload.Data {
		counts[room.ID] = room.OnlinePlayers
	}
	if counts[firstRoom.ID] != 2 || counts[secondRoom.ID] != 0 {
		t.Fatalf("online player counts=%v, want first=2 second=0", counts)
	}
}

func TestGetPlayerListIncludesOpenSessionPlayTimeAndSortsByIt(t *testing.T) {
	setupLivePlayerStatsContractDB(t)
	room := createLivePlayerStatsRoom(t, "playtime-room")

	result, err := db.DB.Exec(`INSERT INTO players (name, room_id, status) VALUES (?, ?, 'online')`, "Active", room.ID)
	if err != nil {
		t.Fatalf("insert active player: %v", err)
	}
	activeID, _ := result.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO player_stats (player_id, total_play_time, login_count) VALUES (?, ?, 1)`, activeID, 60); err != nil {
		t.Fatalf("insert active stats: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO player_sessions (player_id, room_id, join_time, ip_address) VALUES (?, ?, ?, ?)`, activeID, room.ID, time.Now().Add(-5*time.Minute), "203.0.113.10"); err != nil {
		t.Fatalf("insert active session: %v", err)
	}

	result, err = db.DB.Exec(`INSERT INTO players (name, room_id, status) VALUES (?, ?, 'offline')`, "Stored", room.ID)
	if err != nil {
		t.Fatalf("insert stored player: %v", err)
	}
	storedID, _ := result.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO player_stats (player_id, total_play_time, login_count) VALUES (?, ?, 1)`, storedID, 180); err != nil {
		t.Fatalf("insert stored stats: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/players", GetPlayerList)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/players?sortBy=totalPlayTime", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GetPlayerList status=%d body=%s", response.Code, response.Body.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Players []models.PlayerDetail `json:"players"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || len(payload.Data.Players) != 2 {
		t.Fatalf("unexpected player payload: %#v", payload)
	}
	first := payload.Data.Players[0]
	if first.Name != "Active" {
		t.Fatalf("first player=%q, want active player ordered by live play time", first.Name)
	}
	if first.TotalPlayTime < 350 {
		t.Fatalf("active player totalPlayTime=%d, want stored 60 plus open session about 300 seconds", first.TotalPlayTime)
	}
	if first.PlayTimeStr == "< 1分钟" || first.PlayTimeStr == "" {
		t.Fatalf("unexpected active player display play time: %q", first.PlayTimeStr)
	}
}

func TestBuildTrendBucketsSupportsHourDayWeekAndMonthRanges(t *testing.T) {
	now := time.Date(2026, time.August, 24, 15, 37, 0, 0, time.Local)
	testCases := []struct {
		rangeValue  string
		granularity string
		wantCount   int
		wantFirst   string
	}{
		{"24h", "hour", 24, "2026-08-23 16:00"},
		{"7d", "day", 7, "2026-08-18"},
		{"12w", "week", 12, "2026-06-08"},
		{"12m", "month", 12, "2025-09"},
	}

	for _, tc := range testCases {
		t.Run(tc.rangeValue+"-"+tc.granularity, func(t *testing.T) {
			rangeValue, granularity, starts := buildTrendBuckets(now, tc.rangeValue, tc.granularity, "")
			if rangeValue != tc.rangeValue || granularity != tc.granularity || len(starts) != tc.wantCount {
				t.Fatalf("bucket metadata=%q/%q/%d, want %q/%q/%d", rangeValue, granularity, len(starts), tc.rangeValue, tc.granularity, tc.wantCount)
			}
			if got := formatTrendBucket(starts[0], granularity); got != tc.wantFirst {
				t.Fatalf("first bucket=%q, want %q", got, tc.wantFirst)
			}
		})
	}
}

func TestLivePlayTimeIsIncludedInRankingAndCurrentTrendBucket(t *testing.T) {
	setupLivePlayerStatsContractDB(t)
	room := createLivePlayerStatsRoom(t, "trend-room")

	result, err := db.DB.Exec(`INSERT INTO players (name, room_id, status) VALUES (?, ?, 'online')`, "Live Leader", room.ID)
	if err != nil {
		t.Fatalf("insert player: %v", err)
	}
	playerID, _ := result.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO player_stats (player_id, total_play_time) VALUES (?, 0)`, playerID); err != nil {
		t.Fatalf("insert player stats: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO player_sessions (player_id, room_id, join_time) VALUES (?, ?, ?)`, playerID, room.ID, time.Now().Add(-4*time.Minute)); err != nil {
		t.Fatalf("insert active session: %v", err)
	}

	rankings, err := getLivePlaytimeRankings(10)
	if err != nil {
		t.Fatalf("get live rankings: %v", err)
	}
	if len(rankings) != 1 || rankings[0].PlayerName != "Live Leader" || rankings[0].Value < 230 {
		t.Fatalf("unexpected live rankings: %#v", rankings)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/trends", GetTrends)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/trends?range=24h&granularity=hour", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GetTrends status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Success bool             `json:"success"`
		Data    models.TrendData `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode trend response: %v", err)
	}
	if !payload.Success || payload.Data.Range != "24h" || payload.Data.Granularity != "hour" || len(payload.Data.Dates) != 24 {
		t.Fatalf("unexpected trend payload: %#v", payload)
	}
	last := len(payload.Data.TotalPlayTime) - 1
	if last < 0 || payload.Data.TotalPlayTime[last] <= 0 || payload.Data.ActivePlayers[last] != 1 {
		t.Fatalf("current trend bucket did not include active session: %#v", payload.Data)
	}
}
