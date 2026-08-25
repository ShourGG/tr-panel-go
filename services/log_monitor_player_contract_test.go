package services

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/storage"
)

func TestHandlePlayerJoinRefreshesExistingPlayerIPAndRoom(t *testing.T) {
	oldDB := db.DB
	if err := db.Init(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatalf("initialize temporary database: %v", err)
	}
	testDB := db.DB
	t.Cleanup(func() {
		_ = testDB.Close()
		db.DB = oldDB
	})

	roomStore := storage.NewSQLiteRoomStorage(testDB)
	room := &models.Room{
		Name:       "new-room",
		ServerType: "vanilla",
		WorldFile:  "new-room.wld",
		Port:       17777,
		MaxPlayers: 8,
		Status:     "stopped",
	}
	if err := roomStore.Create(room); err != nil {
		t.Fatalf("create room: %v", err)
	}
	result, err := testDB.Exec(`INSERT INTO players (name, ip, room_id, status) VALUES (?, ?, ?, 'offline')`, "Alice", "198.51.100.10", 0)
	if err != nil {
		t.Fatalf("insert existing player: %v", err)
	}
	playerID, _ := result.LastInsertId()
	if _, err := testDB.Exec(`INSERT INTO player_stats (player_id) VALUES (?)`, playerID); err != nil {
		t.Fatalf("insert player stats: %v", err)
	}

	monitor := NewLogMonitor(
		testDB,
		roomStore,
		storage.NewSQLitePlayerSessionStorage(testDB),
		storage.NewSQLitePlayerStatsStorage(testDB),
		storage.NewSQLitePlayerDailyStatsStorage(testDB),
	)
	monitor.handlePlayerJoin("Alice", "203.0.113.25", room.ID)

	var ip, status string
	var roomID int
	if err := testDB.QueryRow(`SELECT ip, room_id, status FROM players WHERE id = ?`, playerID).Scan(&ip, &roomID, &status); err != nil {
		t.Fatalf("read updated player: %v", err)
	}
	if ip != "203.0.113.25" || roomID != room.ID || status != "online" {
		t.Fatalf("player connection=%q/%d/%q, want refreshed ip/current room/online", ip, roomID, status)
	}

	monitor.handlePlayerJoin("Alice", "203.0.113.26", room.ID)
	if err := testDB.QueryRow(`SELECT ip FROM players WHERE id = ?`, playerID).Scan(&ip); err != nil {
		t.Fatalf("read duplicate join player: %v", err)
	}
	if ip != "203.0.113.26" {
		t.Fatalf("duplicate join did not refresh player IP: %q", ip)
	}
}

func TestParseLineRecognizesTerrariaJoinLeaveLogVariantsAndRecordsPlayTime(t *testing.T) {
	oldDB := db.DB
	if err := db.Init(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatalf("initialize temporary database: %v", err)
	}
	testDB := db.DB
	t.Cleanup(func() {
		_ = testDB.Close()
		db.DB = oldDB
	})

	roomStore := storage.NewSQLiteRoomStorage(testDB)
	room := &models.Room{
		Name:       "log-contract-room",
		ServerType: "vanilla",
		WorldFile:  "log-contract-room.wld",
		Port:       17778,
		MaxPlayers: 8,
		Status:     "running",
	}
	if err := roomStore.Create(room); err != nil {
		t.Fatalf("create room: %v", err)
	}
	monitor := NewLogMonitor(
		testDB,
		roomStore,
		storage.NewSQLitePlayerSessionStorage(testDB),
		storage.NewSQLitePlayerStatsStorage(testDB),
		storage.NewSQLitePlayerDailyStatsStorage(testDB),
	)

	monitor.parseLine("[STDOUT] [12:34:56] [Server thread/INFO] [Terraria]: Player Alice has joined.", room.ID)
	var playerID int
	if err := testDB.QueryRow(`SELECT id FROM players WHERE name = ?`, "Alice").Scan(&playerID); err != nil {
		t.Fatalf("join log did not create Alice: %v", err)
	}
	var sessionID int
	if err := testDB.QueryRow(`SELECT id FROM player_sessions WHERE player_id = ? AND leave_time IS NULL`, playerID).Scan(&sessionID); err != nil {
		t.Fatalf("join log did not create active session: %v", err)
	}
	joinTime := time.Now().Add(-5 * time.Minute)
	if _, err := testDB.Exec(`UPDATE player_sessions SET join_time = ? WHERE id = ?`, joinTime, sessionID); err != nil {
		t.Fatalf("rewind fixture session: %v", err)
	}

	monitor.parseLine("[STDOUT] Alice has left.", room.ID)
	var leaveTime sql.NullTime
	var duration int
	if err := testDB.QueryRow(`SELECT leave_time, duration FROM player_sessions WHERE id = ?`, sessionID).Scan(&leaveTime, &duration); err != nil {
		t.Fatalf("leave log did not close active session: %v", err)
	}
	if !leaveTime.Valid || duration < 299 {
		t.Fatalf("closed session=%v duration=%d, want about five minutes", leaveTime.Valid, duration)
	}
	var totalPlayTime int
	if err := testDB.QueryRow(`SELECT total_play_time FROM player_stats WHERE player_id = ?`, playerID).Scan(&totalPlayTime); err != nil {
		t.Fatalf("read player stats: %v", err)
	}
	if totalPlayTime < 299 {
		t.Fatalf("total play time=%d, want at least 299 seconds", totalPlayTime)
	}

	variants := []string{
		"[17:01:02] [Terraria]: Bob (198.51.100.7:4123) has joined.",
		"Player Carol has joined",
		"Dave已加入。",
		"[17:01:05] [Terraria]: Bob has left.",
		"Player Carol has left",
		"Dave已离开",
	}
	for _, line := range variants {
		if normalized := normalizeLogMonitorLine(line); strings.Contains(normalized, "[Terraria]") {
			t.Fatalf("logger prefix was not normalized from %q: %q", line, normalized)
		}
	}
}

func TestNormalizeLogMonitorLineRemovesPrefixesAndAnsi(t *testing.T) {
	got := normalizeLogMonitorLine(fmt.Sprintf("\x1b[32m[STDOUT] [01:02:03] [Info]: Player Alice has joined.\x1b[0m"))
	if got != "Player Alice has joined." {
		t.Fatalf("normalized log line=%q, want %q", got, "Player Alice has joined.")
	}
}
