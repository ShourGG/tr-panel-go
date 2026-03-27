package config

import (
	"path/filepath"
	"testing"
)

func TestPathHelpersContract(t *testing.T) {
	oldLogsDir := LogsDir
	oldServersDir := ServersDir
	LogsDir = filepath.Join("contract-root", "logs")
	ServersDir = filepath.Join("contract-root", "servers")
	t.Cleanup(func() {
		LogsDir = oldLogsDir
		ServersDir = oldServersDir
	})

	testCases := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "panel log file",
			got:  PanelLogFile(),
			want: filepath.Join(LogsDir, "panel.log"),
		},
		{
			name: "room log file",
			got:  RoomLogFile(12),
			want: filepath.Join(LogsDir, "room-12.log"),
		},
		{
			name: "plugin server logs dir",
			got:  PluginServerLogsDir(),
			want: filepath.Join(ServersDir, "tshock", "logs"),
		},
		{
			name: "plugin server log file",
			got:  PluginServerLogFile(),
			want: filepath.Join(ServersDir, "tshock", "logs", "plugin-server.log"),
		},
	}

	for _, tc := range testCases {
		if tc.got != tc.want {
			t.Fatalf("%s mismatch: got %q want %q", tc.name, tc.got, tc.want)
		}
	}
}
