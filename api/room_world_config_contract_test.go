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
