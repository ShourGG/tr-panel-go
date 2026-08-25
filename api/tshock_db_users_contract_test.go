package api

import (
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestScanTShockUsersPreservesRowsWithNullableFields(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(`
		CREATE TABLE Users (
			ID INTEGER PRIMARY KEY,
			Username TEXT,
			Password TEXT,
			UUID TEXT,
			Usergroup TEXT,
			Registered TEXT,
			LastAccessed TEXT,
			KnownIPs TEXT
		);
		INSERT INTO Users (ID, Username, Password, UUID, Usergroup, Registered, LastAccessed, KnownIPs)
		VALUES (1, 'qa-user', '$2a$07$hash', NULL, 'admin', '2026-08-25T00:00:00', NULL, NULL);
	`)
	if err != nil {
		t.Fatalf("seed nullable TShock user: %v", err)
	}

	rows, err := database.Query(`
		SELECT ID, COALESCE(Username, ''), COALESCE(Password, ''), COALESCE(UUID, ''),
		       COALESCE(Usergroup, ''), COALESCE(Registered, ''), COALESCE(LastAccessed, ''), COALESCE(KnownIPs, '')
		FROM Users
	`)
	if err != nil {
		t.Fatalf("query TShock users: %v", err)
	}
	defer rows.Close()

	users, err := scanTShockUsers(rows)
	if err != nil {
		t.Fatalf("scan TShock users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("scanned users=%d, want 1", len(users))
	}
	if users[0].Username != "qa-user" || users[0].Usergroup != "admin" {
		t.Fatalf("unexpected user: %#v", users[0])
	}
	if users[0].UUID != "" || users[0].LastAccessed != "" || users[0].KnownIPs != "" {
		t.Fatalf("nullable fields were not normalized: %#v", users[0])
	}
}
