package api

import (
	"reflect"
	"terraria-panel/models"
	"testing"
)

func TestRoomWorldCreationValuesContract(t *testing.T) {
	if got := roomWorldSizeValue("large"); got != "3" {
		t.Fatalf("large world size = %q, want 3", got)
	}
	if got := roomDifficultyValue("expert"); got != "1" {
		t.Fatalf("expert difficulty = %q, want 1", got)
	}
	if got := roomEvilValue("crimson"); got != "crimson" {
		t.Fatalf("crimson evil = %q, want crimson", got)
	}
}

func TestRoomAutocreateValueContract(t *testing.T) {
	tests := []struct {
		name        string
		worldExists bool
		worldSize   string
		want        string
	}{
		{name: "existing world is not recreated", worldExists: true, worldSize: "large", want: "0"},
		{name: "new small world", worldSize: "small", want: "1"},
		{name: "new medium world", worldSize: "medium", want: "2"},
		{name: "new large world", worldSize: "large", want: "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roomAutocreateValue(tt.worldExists, tt.worldSize); got != tt.want {
				t.Fatalf("roomAutocreateValue(%v, %q) = %q, want %q", tt.worldExists, tt.worldSize, got, tt.want)
			}
		})
	}
}

func TestAppendRoomWorldCreationArgsContract(t *testing.T) {
	room := &models.Room{
		Difficulty: "master",
		EvilType:   "corruption",
		Seed:       "05162020",
	}

	got := appendRoomWorldCreationArgs([]string{"-autocreate", "2"}, room)
	want := []string{"-autocreate", "2", "-difficulty", "2", "-worldevil", "corrupt", "-seed", "05162020"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("world creation args = %#v, want %#v", got, want)
	}
}
