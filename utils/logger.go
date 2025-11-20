package utils
import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)
var panelLogFile *os.File
func InitLogger() error {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return err
	}
	logPath := "logs/panel.log"
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
	serverLogDir := filepath.Join("servers", roomID)
	os.MkdirAll(serverLogDir, 0755)
	logPath := filepath.Join(serverLogDir, "server.log")
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
