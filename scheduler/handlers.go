package scheduler
import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"terraria-panel/storage"
	"terraria-panel/utils"
	"time"
	"archive/zip"
)
type BackupHandlerImpl struct {
	roomStorage storage.RoomStorage
}
func NewBackupHandler(roomStorage storage.RoomStorage) BackupHandler {
	return &BackupHandlerImpl{
		roomStorage: roomStorage,
	}
}
func (h *BackupHandlerImpl) CreateBackup(roomID int, backupType string, note string) error {
	log.Printf("[BackupHandler] Creating backup for room %d...", roomID)
	room, err := h.roomStorage.GetByID(roomID)
	if err != nil {
		return fmt.Errorf("failed to get room: %w", err)
	}
	timestamp := time.Now().Format("20060102_150405")
	zipName := fmt.Sprintf("room-%d_%s_%s.zip", room.ID, room.Name, timestamp)
	zipPath := filepath.Join(config.BackupDir, zipName)
	log.Printf("[BackupHandler] Creating backup file: %s", zipName)
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create ZIP file: %w", err)
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if _, err := os.Stat(roomDir); os.IsNotExist(err) {
		return fmt.Errorf("room directory does not exist: %s", roomDir)
	}
	if err := addDirToZip(zipWriter, roomDir, ""); err != nil {
		return fmt.Errorf("failed to add room directory to ZIP: %w", err)
	}
	log.Printf("[BackupHandler] Backup created successfully: %s", zipName)
	return nil
}
func addDirToZip(zipWriter *zip.Writer, sourceDir string, baseInZip string) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDir, entry.Name())
		zipPath := filepath.Join(baseInZip, entry.Name())
		if entry.IsDir() {
			if err := addDirToZip(zipWriter, sourcePath, zipPath); err != nil {
				return err
			}
		} else {
			if err := addFileToZip(zipWriter, sourcePath, zipPath); err != nil {
				return err
			}
		}
	}
	return nil
}
func addFileToZip(zipWriter *zip.Writer, filePath string, zipPath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	writer, err := zipWriter.Create(zipPath)
	if err != nil {
		return err
	}
	_, err = file.WriteTo(writer)
	return err
}
type RestartHandlerImpl struct {
	roomStorage storage.RoomStorage
}
func NewRestartHandler(roomStorage storage.RoomStorage) RestartHandler {
	return &RestartHandlerImpl{
		roomStorage: roomStorage,
	}
}
func (h *RestartHandlerImpl) RestartRoom(roomID int) error {
	log.Printf("[RestartHandler] Restarting room %d...", roomID)
	room, err := h.roomStorage.GetByID(roomID)
	if err != nil {
		return fmt.Errorf("failed to get room: %w", err)
	}
	if p, exists := utils.GetProcess(roomID); exists && p.IsRunning() {
		log.Printf("[RestartHandler] Stopping room %d...", roomID)
		if err := p.Stop(); err != nil {
			return fmt.Errorf("failed to stop room: %w", err)
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("[RestartHandler] Starting room %d...", roomID)
	var cmd string
	var args []string
	var workDir string
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	switch room.ServerType {
	case "vanilla":
		cmd = filepath.Join(config.ServersDir, "vanilla", "TerrariaServer.exe")
		args = []string{
			"-config", filepath.Join(roomDir, "config.txt"),
		}
		workDir = filepath.Join(config.ServersDir, "vanilla")
	case "tmodloader":
		cmd = filepath.Join(config.ServersDir, "tModLoader", "start-tModLoaderServer.bat")
		args = []string{
			"-config", filepath.Join(roomDir, "config.txt"),
		}
		workDir = filepath.Join(config.ServersDir, "tModLoader")
	case "tshock":
		cmd = filepath.Join(config.ServersDir, "tshock", "TShock.Server.exe")
		args = []string{
			"-config", filepath.Join(roomDir, "config.txt"),
		}
		workDir = filepath.Join(config.ServersDir, "tshock")
	default:
		return fmt.Errorf("unsupported server type: %s", room.ServerType)
	}
	logFile, err := os.OpenFile(
		filepath.Join(roomDir, "server.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()
	process, err := utils.StartProcess(
		roomID,
		cmd,
		args,
		workDir,
		nil,
		logFile,
		room.ServerType,
	)
	if err != nil {
		return fmt.Errorf("failed to start room: %w", err)
	}
	h.roomStorage.UpdateStatus(roomID, "running", process.GetPID())
	log.Printf("[RestartHandler] Room %d restarted successfully (PID: %d)", roomID, process.GetPID())
	return nil
}
type CleanupBackupHandlerImpl struct {
	roomStorage storage.RoomStorage
}
func NewCleanupBackupHandler(roomStorage storage.RoomStorage) CleanupBackupHandler {
	return &CleanupBackupHandlerImpl{
		roomStorage: roomStorage,
	}
}
func (h *CleanupBackupHandlerImpl) CleanupOldBackups(roomID int, daysToKeep int) error {
	log.Printf("[CleanupBackupHandler] Cleaning up backups older than %d days for room %d...", daysToKeep, roomID)
	backupDir := filepath.Join(config.DataDir, "backups")
	if roomID > 0 {
		backupDir = filepath.Join(backupDir, fmt.Sprintf("room-%d", roomID))
	}
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		log.Printf("[CleanupBackupHandler] Backup directory does not exist: %s", backupDir)
		return nil
	}
	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)
	files, err := ioutil.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}
	deletedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".zip") {
			continue
		}
		if file.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(backupDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				log.Printf("[CleanupBackupHandler] Failed to delete backup file %s: %v", filePath, err)
				continue
			}
			log.Printf("[CleanupBackupHandler] Deleted old backup: %s", file.Name())
			deletedCount++
		}
	}
	log.Printf("[CleanupBackupHandler] Cleanup completed. Deleted %d old backup files.", deletedCount)
	return nil
}
type CleanupLogHandlerImpl struct {
	roomStorage storage.RoomStorage
}
func NewCleanupLogHandler(roomStorage storage.RoomStorage) CleanupLogHandler {
	return &CleanupLogHandlerImpl{
		roomStorage: roomStorage,
	}
}
func (h *CleanupLogHandlerImpl) CleanupOldLogs(roomID int, daysToKeep int) error {
	log.Printf("[CleanupLogHandler] Cleaning up logs older than %d days for room %d...", daysToKeep, roomID)
	var logDir string
	if roomID > 0 {
		logDir = filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", roomID), "logs")
	} else {
		logDir = filepath.Join(config.DataDir, "logs")
	}
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		log.Printf("[CleanupLogHandler] Log directory does not exist: %s", logDir)
		return nil
	}
	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)
	files, err := ioutil.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}
	deletedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".log") && !strings.HasSuffix(file.Name(), ".txt") {
			continue
		}
		if file.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(logDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				log.Printf("[CleanupLogHandler] Failed to delete log file %s: %v", filePath, err)
				continue
			}
			log.Printf("[CleanupLogHandler] Deleted old log: %s", file.Name())
			deletedCount++
		}
	}
	log.Printf("[CleanupLogHandler] Cleanup completed. Deleted %d old log files.", deletedCount)
	return nil
}
type BroadcastHandlerImpl struct {
	roomStorage storage.RoomStorage
}
func NewBroadcastHandler(roomStorage storage.RoomStorage) BroadcastHandler {
	return &BroadcastHandlerImpl{
		roomStorage: roomStorage,
	}
}
func (h *BroadcastHandlerImpl) SendBroadcast(roomID int, message string) error {
	log.Printf("[BroadcastHandler] Sending broadcast to room %d: %s", roomID, message)
	room, err := h.roomStorage.GetByID(roomID)
	if err != nil {
		return fmt.Errorf("failed to get room info: %w", err)
	}
	if room.Status != "running" {
		return fmt.Errorf("room %d is not running (status: %s)", roomID, room.Status)
	}
	process, exists := utils.GetProcess(roomID)
	if !exists || process == nil {
		return fmt.Errorf("process not found for room %d", roomID)
	}
	var command string
	switch room.ServerType {
	case "tshock":
		command = fmt.Sprintf("broadcast %s\n", message)
	case "vanilla", "tmodloader":
		command = fmt.Sprintf("say %s\n", message)
	default:
		return fmt.Errorf("unsupported server type: %s", room.ServerType)
	}
	if err := process.SendCommand(command); err != nil {
		return fmt.Errorf("failed to send broadcast command: %w", err)
	}
	log.Printf("[BroadcastHandler] Broadcast sent successfully to room %d", roomID)
	return nil
}
type CustomCommandHandlerImpl struct {
	roomStorage storage.RoomStorage
}
func NewCustomCommandHandler(roomStorage storage.RoomStorage) CustomCommandHandler {
	return &CustomCommandHandlerImpl{
		roomStorage: roomStorage,
	}
}
func (h *CustomCommandHandlerImpl) ExecuteCommand(roomID int, command string) error {
	log.Printf("[CustomCommandHandler] Executing command on room %d: %s", roomID, command)
	room, err := h.roomStorage.GetByID(roomID)
	if err != nil {
		return fmt.Errorf("failed to get room info: %w", err)
	}
	if room.Status != "running" {
		return fmt.Errorf("room %d is not running (status: %s)", roomID, room.Status)
	}
	process, exists := utils.GetProcess(roomID)
	if !exists || process == nil {
		return fmt.Errorf("process not found for room %d", roomID)
	}
	if !strings.HasSuffix(command, "\n") {
		command += "\n"
	}
	if err := process.SendCommand(command); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}
	log.Printf("[CustomCommandHandler] Command executed successfully on room %d", roomID)
	return nil
}
