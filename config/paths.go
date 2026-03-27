package config

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	DataDir         string
	RoomsFile       string
	PlayersFile     string
	BackupDir       string
	LogsDir         string
	WorldsDir       string
	SharedWorldsDir string
	ServersDir      string
)

func init() {
	DataDir = os.Getenv("DATA_DIR")
	if DataDir == "" {
		execPath, err := os.Executable()
		if err != nil {
			DataDir, _ = os.Getwd()
		} else {
			DataDir = filepath.Dir(execPath)
		}
		DataDir = filepath.Join(DataDir, "data")
	}
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		panic("无法创建数据目录: " + err.Error())
	}
	RoomsFile = filepath.Join(DataDir, "rooms.json")
	PlayersFile = filepath.Join(DataDir, "players.json")
	BackupDir = filepath.Join(DataDir, "backups")
	LogsDir = filepath.Join(DataDir, "logs")
	WorldsDir = filepath.Join(DataDir, "worlds")
	SharedWorldsDir = filepath.Join(DataDir, "shared-worlds")
	ServersDir = filepath.Join(DataDir, "servers")
	os.MkdirAll(BackupDir, 0755)
	os.MkdirAll(LogsDir, 0755)
	os.MkdirAll(WorldsDir, 0755)
	os.MkdirAll(SharedWorldsDir, 0755)
	os.MkdirAll(ServersDir, 0755)
	os.MkdirAll(filepath.Join(ServersDir, "vanilla"), 0755)
	os.MkdirAll(filepath.Join(ServersDir, "tModLoader"), 0755)
	os.MkdirAll(filepath.Join(ServersDir, "tshock"), 0755)
	os.MkdirAll(filepath.Join(DataDir, "tModLoader", "Mods"), 0755)
}

func PanelLogFile() string {
	return filepath.Join(LogsDir, "panel.log")
}

func RoomLogFile(roomID int) string {
	return filepath.Join(LogsDir, fmt.Sprintf("room-%d.log", roomID))
}

func PluginServerLogsDir() string {
	return filepath.Join(ServersDir, "tshock", "logs")
}

func PluginServerLogFile() string {
	return filepath.Join(PluginServerLogsDir(), "plugin-server.log")
}
