package storage

import (
	"database/sql"
	"testing"
	"time"

	"terraria-panel/models"

	_ "github.com/glebarez/go-sqlite"
)

func TestSQLitePlayerSessionStorageIncludesPlayerAndRoomNames(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })

	_, err = database.Exec(`
		CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE rooms (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE player_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			player_id INTEGER NOT NULL,
			room_id INTEGER NOT NULL,
			join_time DATETIME NOT NULL,
			leave_time DATETIME,
			duration INTEGER DEFAULT 0,
			ip_address TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO players (id, name) VALUES (11, 'Alice');
		INSERT INTO rooms (id, name) VALUES (7, 'Alpha Room');
	`)
	if err != nil {
		t.Fatalf("create session fixture: %v", err)
	}

	baseTime := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	leaveTime := baseTime.Add(2 * time.Minute)
	store := NewSQLitePlayerSessionStorage(database)
	closed := &models.PlayerSession{
		PlayerID:  11,
		RoomID:    7,
		JoinTime:  baseTime,
		LeaveTime: &leaveTime,
		Duration:  120,
		IPAddress: "10.0.0.1",
	}
	if err := store.Create(closed); err != nil {
		t.Fatalf("create closed session: %v", err)
	}

	active := &models.PlayerSession{
		PlayerID:  11,
		RoomID:    7,
		JoinTime:  baseTime.Add(time.Hour),
		IPAddress: "10.0.0.2",
	}
	if err := store.Create(active); err != nil {
		t.Fatalf("create active session: %v", err)
	}

	sessions, total, err := store.GetByPlayerID(11, 1, 0)
	if err != nil {
		t.Fatalf("get player sessions: %v", err)
	}
	if total != 2 || len(sessions) != 1 {
		t.Fatalf("unexpected paged sessions: total=%d len=%d", total, len(sessions))
	}
	if sessions[0].PlayerName != "Alice" || sessions[0].RoomName != "Alpha Room" || sessions[0].IPAddress != "10.0.0.2" {
		t.Fatalf("session names or address not populated: %#v", sessions[0])
	}

	byID, err := store.GetByID(closed.ID)
	if err != nil {
		t.Fatalf("get session by id: %v", err)
	}
	if byID == nil || byID.PlayerName != "Alice" || byID.RoomName != "Alpha Room" {
		t.Fatalf("session by id missing names: %#v", byID)
	}

	activeSession, err := store.GetActiveSession(11, 7)
	if err != nil {
		t.Fatalf("get active session: %v", err)
	}
	if activeSession == nil || activeSession.ID != active.ID || activeSession.RoomName != "Alpha Room" {
		t.Fatalf("unexpected active session: %#v", activeSession)
	}

	all, allTotal, err := store.GetAll(10, 0)
	if err != nil {
		t.Fatalf("get all sessions: %v", err)
	}
	if allTotal != 2 || len(all) != 2 || all[0].PlayerName != "Alice" || all[0].RoomName != "Alpha Room" {
		t.Fatalf("unexpected all sessions: total=%d sessions=%#v", allTotal, all)
	}
}
