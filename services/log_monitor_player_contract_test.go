package services

import (
	"path/filepath"
	"testing"

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
