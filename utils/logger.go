package utils

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"terraria-panel/config"
	"time"
)

var panelLogFile *os.File

func InitLogger() error {
	if err := os.MkdirAll(config.LogsDir, 0755); err != nil {
		return err
	}
	logPath := config.PanelLogFile()
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	panelLogFile = file
	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	LogInfo("Panel logger initialized")
	return nil
}
func CloseLogger() {
	if panelLogFile != nil {
		panelLogFile.Close()
	}
}
func LogInfo(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMsg := fmt.Sprintf("[%s] [INFO] %s\n", timestamp, msg)
	fmt.Print(logMsg)
	if panelLogFile != nil {
		panelLogFile.WriteString(logMsg)
	}
}
func LogDebug(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMsg := fmt.Sprintf("[%s] [DEBUG] %s\n", timestamp, msg)
	fmt.Print(logMsg)
	if panelLogFile != nil {
		panelLogFile.WriteString(logMsg)
	}
}
func LogError(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMsg := fmt.Sprintf("[%s] [ERROR] %s\n", timestamp, msg)
	fmt.Print(logMsg)
	if panelLogFile != nil {
		panelLogFile.WriteString(logMsg)
	}
}
func LogServerOutput(roomID, line string) {
	roomIDInt, err := strconv.Atoi(roomID)
	if err != nil {
		LogError("Failed to parse room log ID: %v", err)
		return
	}

	logPath := config.RoomLogFile(roomIDInt)
	logDir := config.LogsDir
	if roomIDInt == 0 {
		logPath = config.PluginServerLogFile()
		logDir = config.PluginServerLogsDir()
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		LogError("Failed to create server log directory: %v", err)
		return
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		LogError("Failed to open server log file: %v", err)
		return
	}
	defer file.Close()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMsg := fmt.Sprintf("[%s] %s\n", timestamp, line)
	file.WriteString(logMsg)
}
