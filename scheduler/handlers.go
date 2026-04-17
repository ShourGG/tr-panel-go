package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/services"
	"terraria-panel/storage"
	"terraria-panel/utils"
)

func computeBackupFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseBackupCreatedAt(value string) time.Time {
	parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return time.Now()
	}
	return parsedTime
}

func buildBackupRecord(summary utils.BackupSummary, localPath string, note string, remoteEnabled bool) *models.BackupRecord {
	uploadStatus := "local_only"
	if remoteEnabled {
		uploadStatus = "pending"
	}

	return &models.BackupRecord{
		ID:             summary.ID,
		FileName:       summary.Name,
		RoomID:         summary.RoomID,
		RoomName:       summary.RoomName,
		ServerType:     summary.ServerType,
		WorldFile:      summary.WorldFile,
		BackupType:     summary.Type,
		Note:           note,
		LocalPath:      localPath,
		FileSize:       summary.Size,
		ChecksumSHA256: summary.ChecksumSHA256,
		StorageType:    "local",
		UploadStatus:   uploadStatus,
		CreatedAt:      parseBackupCreatedAt(summary.CreatedAt),
		UpdatedAt:      time.Now(),
	}
}

func syncScheduledBackupRecord(record *models.BackupRecord, recordStorage storage.BackupRecordStorage) {
	if record == nil || recordStorage == nil {
		return
	}

	remoteService, err := services.NewBackupRemoteService(config.Load())
	if err != nil {
		log.Printf("[BackupHandler] Remote backup service init failed: %v", err)
		_ = recordStorage.UpdateRemoteState(record.ID, record.StorageType, "failed", err.Error(), record.RemoteBucket, record.RemoteKey, record.RemoteETag, record.RemoteURL, record.UploadedAt)
		return
	}
	if !remoteService.Enabled() {
		return
	}

	result, err := remoteService.SyncBackup(context.Background(), record)
	if err != nil {
		log.Printf("[BackupHandler] Remote backup sync failed for %s: %v", record.ID, err)
		_ = recordStorage.UpdateRemoteState(record.ID, record.StorageType, "failed", err.Error(), record.RemoteBucket, record.RemoteKey, record.RemoteETag, record.RemoteURL, record.UploadedAt)
		return
	}

	if err := recordStorage.UpdateRemoteState(record.ID, result.StorageType, "uploaded", "", result.Bucket, result.Key, result.ETag, result.RemoteURL, &result.UploadedAt); err != nil {
		log.Printf("[BackupHandler] Failed to persist remote backup sync result for %s: %v", record.ID, err)
	}
}

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
	if room == nil {
		return fmt.Errorf("room %d not found", roomID)
	}
	createdAt := time.Now()
	zipName := utils.BuildBackupArchiveName(room.ID, room.Name, createdAt)
	zipPath := filepath.Join(config.BackupDir, zipName)
	log.Printf("[BackupHandler] Creating backup file: %s", zipName)
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if _, err := os.Stat(roomDir); os.IsNotExist(err) {
		return fmt.Errorf("room directory does not exist: %s", roomDir)
	}

	manifest := utils.NewBackupManifest(room.ID, room.Name, room.ServerType, room.WorldFile, backupType, note, createdAt)
	if err := utils.CreateBackupArchive(zipPath, roomDir, manifest); err != nil {
		return fmt.Errorf("failed to create ZIP file: %w", err)
	}

	summary, _, err := utils.InspectBackupArchive(zipPath)
	if err != nil {
		return fmt.Errorf("backup created but inspect failed: %w", err)
	}

	checksum, err := computeBackupFileSHA256(zipPath)
	if err != nil {
		return fmt.Errorf("backup created but checksum failed: %w", err)
	}
	summary.ChecksumSHA256 = checksum

	remoteEnabled := config.Load().BackupRemoteEnabled
	recordStorage := storage.NewSQLiteBackupRecordStorage(db.DB)
	record := buildBackupRecord(summary, zipPath, note, remoteEnabled)
	if err := recordStorage.Upsert(record); err != nil {
		return fmt.Errorf("backup created but record persistence failed: %w", err)
	}
	if remoteEnabled {
		go syncScheduledBackupRecord(record, recordStorage)
	}

	log.Printf("[BackupHandler] Backup created successfully: %s", zipName)
	return nil
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
		config.RoomLogFile(room.ID),
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
	backupDir := config.BackupDir
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		log.Printf("[CleanupBackupHandler] Backup directory does not exist: %s", backupDir)
		return nil
	}
	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)
	files, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}
	deletedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		if !strings.HasSuffix(strings.ToLower(fileName), ".zip") {
			continue
		}
		if roomID > 0 && !strings.HasPrefix(fileName, fmt.Sprintf("room-%d_", roomID)) {
			continue
		}

		fileInfo, err := file.Info()
		if err != nil {
			log.Printf("[CleanupBackupHandler] Failed to stat backup file %s: %v", fileName, err)
			continue
		}
		if !fileInfo.ModTime().Before(cutoffTime) {
			continue
		}

		filePath := filepath.Join(backupDir, fileName)
		if err := os.Remove(filePath); err != nil {
			log.Printf("[CleanupBackupHandler] Failed to delete backup file %s: %v", filePath, err)
			continue
		}
		log.Printf("[CleanupBackupHandler] Deleted old backup: %s", fileName)
		deletedCount++
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
	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)

	logFile := config.PanelLogFile()
	logTarget := "panel"
	if roomID > 0 {
		logFile = config.RoomLogFile(roomID)
		logTarget = fmt.Sprintf("room %d", roomID)
	}

	fileInfo, err := os.Stat(logFile)
	if os.IsNotExist(err) {
		log.Printf("[CleanupLogHandler] %s log file does not exist: %s", logTarget, logFile)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	if !fileInfo.ModTime().Before(cutoffTime) {
		log.Printf("[CleanupLogHandler] %s log file is newer than cutoff: %s", logTarget, logFile)
		return nil
	}

	if err := os.Remove(logFile); err != nil {
		return fmt.Errorf("failed to delete log file %s: %w", logFile, err)
	}

	log.Printf("[CleanupLogHandler] Deleted old %s log file: %s", logTarget, logFile)
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
