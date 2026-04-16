package api

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/services"
	"terraria-panel/storage"
	"terraria-panel/utils"

	"github.com/gin-gonic/gin"
)

type backupAnalysisCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type backupTargetRoomInfo struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ServerType string `json:"serverType"`
	WorldFile  string `json:"worldFile"`
	Status     string `json:"status"`
}

type backupRestoreAnalysis struct {
	Backup        utils.BackupSummary   `json:"backup"`
	TargetRoom    backupTargetRoomInfo  `json:"targetRoom"`
	CanRestore    bool                  `json:"canRestore"`
	RequiresForce bool                  `json:"requiresForce"`
	FatalIssues   []string              `json:"fatalIssues"`
	Warnings      []string              `json:"warnings"`
	Checks        []backupAnalysisCheck `json:"checks"`
}

func getBackupRecordStorage() storage.BackupRecordStorage {
	return storage.NewSQLiteBackupRecordStorage(db.DB)
}

func enrichBackupSummariesWithRecords(backups []utils.BackupSummary) {
	ids := make([]string, 0, len(backups))
	for _, backup := range backups {
		if strings.TrimSpace(backup.ID) != "" {
			ids = append(ids, backup.ID)
		}
	}
	if len(ids) == 0 {
		return
	}

	records, err := getBackupRecordStorage().GetByIDs(ids)
	if err != nil {
		log.Printf("[Backup] Failed to load backup records: %v", err)
		return
	}

	for i := range backups {
		record, ok := records[backups[i].ID]
		if !ok {
			continue
		}
		backups[i].StorageType = record.StorageType
		backups[i].RemoteBucket = record.RemoteBucket
		backups[i].RemoteKey = record.RemoteKey
		backups[i].RemoteURL = record.RemoteURL
		backups[i].UploadStatus = record.UploadStatus
		backups[i].UploadError = record.UploadError
		backups[i].ChecksumSHA256 = record.ChecksumSHA256
		if record.UploadedAt != nil {
			backups[i].UploadedAt = record.UploadedAt.Local().Format("2006-01-02 15:04:05")
		}
		if record.LastVerifiedAt != nil {
			backups[i].LastVerifiedAt = record.LastVerifiedAt.Local().Format("2006-01-02 15:04:05")
		}
	}
}

func computeFileSHA256(filePath string) (string, error) {
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

func createBackupRecordFromSummary(summary utils.BackupSummary, localPath string, note string, remoteEnabled bool) *models.BackupRecord {
	createdAt := parseBackupDisplayTime(summary.CreatedAt)
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

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
		CreatedAt:      createdAt,
		UpdatedAt:      time.Now(),
	}
}

func syncBackupRecordToRemote(recordID string) {
	recordStorage := getBackupRecordStorage()
	record, err := recordStorage.GetByID(recordID)
	if err != nil {
		log.Printf("[Backup] Failed to load backup record %s for remote sync: %v", recordID, err)
		return
	}
	if record == nil {
		return
	}

	remoteService, err := services.NewBackupRemoteService(config.Load())
	if err != nil {
		log.Printf("[Backup] Remote backup service init failed: %v", err)
		_ = recordStorage.UpdateRemoteState(recordID, record.StorageType, "failed", err.Error(), record.RemoteBucket, record.RemoteKey, record.RemoteETag, record.RemoteURL, record.UploadedAt)
		return
	}
	if !remoteService.Enabled() {
		return
	}

	result, err := remoteService.SyncBackup(context.Background(), record)
	if err != nil {
		log.Printf("[Backup] Remote backup sync failed for %s: %v", recordID, err)
		_ = recordStorage.UpdateRemoteState(recordID, record.StorageType, "failed", err.Error(), record.RemoteBucket, record.RemoteKey, record.RemoteETag, record.RemoteURL, record.UploadedAt)
		return
	}

	if err := recordStorage.UpdateRemoteState(recordID, result.StorageType, "uploaded", "", result.Bucket, result.Key, result.ETag, result.RemoteURL, &result.UploadedAt); err != nil {
		log.Printf("[Backup] Failed to persist remote backup sync result for %s: %v", recordID, err)
	}
}

func ensureBackupRecordByID(backupID string) (*models.BackupRecord, error) {
	recordStorage := getBackupRecordStorage()
	record, err := recordStorage.GetByID(backupID)
	if err != nil {
		return nil, err
	}
	if record != nil {
		return record, nil
	}

	backupPath, err := resolveBackupPath(backupID)
	if err != nil {
		return nil, err
	}
	summary, _, err := utils.InspectBackupArchive(backupPath)
	if err != nil {
		return nil, err
	}
	checksum, err := computeFileSHA256(backupPath)
	if err != nil {
		return nil, err
	}
	summary.ChecksumSHA256 = checksum

	newRecord := createBackupRecordFromSummary(summary, backupPath, "", config.Load().BackupRemoteEnabled)
	if err := recordStorage.Upsert(newRecord); err != nil {
		return nil, err
	}
	return newRecord, nil
}

func GetBackups(c *gin.Context) {
	entries, err := os.ReadDir(config.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取备份列表失败"))
		return
	}

	backups := make([]utils.BackupSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") {
			continue
		}

		backupPath := filepath.Join(config.BackupDir, entry.Name())
		summary, _, inspectErr := utils.InspectBackupArchive(backupPath)
		if inspectErr != nil {
			info, statErr := entry.Info()
			if statErr != nil {
				continue
			}

			log.Printf("[Backup] Failed to inspect backup %s: %v", entry.Name(), inspectErr)
			backups = append(backups, utils.BackupSummary{
				ID:             strings.TrimSuffix(entry.Name(), ".zip"),
				Name:           entry.Name(),
				Type:           "full",
				Size:           info.Size(),
				CreatedAt:      info.ModTime().Format("2006-01-02 15:04:05"),
				MetadataSource: "invalid",
			})
			continue
		}

		backups = append(backups, summary)
	}

	slices.SortFunc(backups, func(a, b utils.BackupSummary) int {
		at := parseBackupDisplayTime(a.CreatedAt)
		bt := parseBackupDisplayTime(b.CreatedAt)
		switch {
		case at.After(bt):
			return -1
		case at.Before(bt):
			return 1
		default:
			return strings.Compare(a.Name, b.Name)
		}
	})

	enrichBackupSummariesWithRecords(backups)

	c.JSON(http.StatusOK, models.SuccessResponse(backups))
}

func CreateBackup(c *gin.Context) {
	var req struct {
		RoomID int    `json:"roomId" binding:"required"`
		Type   string `json:"type"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	room, err := roomStorage.GetByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}

	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if _, err := os.Stat(roomDir); os.IsNotExist(err) {
		log.Printf("[Backup] Room directory does not exist: %s", roomDir)
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间目录不存在"))
		return
	}

	createdAt := time.Now()
	zipName := utils.BuildBackupArchiveName(room.ID, room.Name, createdAt)
	zipPath := filepath.Join(config.BackupDir, zipName)

	log.Printf("[Backup] Creating backup for room #%d: %s", room.ID, zipName)
	manifest := utils.NewBackupManifest(room.ID, room.Name, room.ServerType, room.WorldFile, req.Type, req.Note, createdAt)
	if err := utils.CreateBackupArchive(zipPath, roomDir, manifest); err != nil {
		log.Printf("[Backup] Failed to create ZIP file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建备份文件失败"))
		return
	}

	summary, _, err := utils.InspectBackupArchive(zipPath)
	if err != nil {
		log.Printf("[Backup] Failed to inspect created ZIP file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份创建成功，但读取元数据失败"))
		return
	}

	checksum, err := computeFileSHA256(zipPath)
	if err != nil {
		log.Printf("[Backup] Failed to compute backup checksum: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份创建成功，但计算校验失败"))
		return
	}
	summary.ChecksumSHA256 = checksum

	remoteEnabled := config.Load().BackupRemoteEnabled
	record := createBackupRecordFromSummary(summary, zipPath, req.Note, remoteEnabled)
	if err := getBackupRecordStorage().Upsert(record); err != nil {
		log.Printf("[Backup] Failed to persist backup record: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份创建成功，但记录备份状态失败"))
		return
	}
	if remoteEnabled {
		go syncBackupRecordToRemote(record.ID)
	}

	log.Printf("[Backup] Backup created successfully: %s (size: %d bytes)", zipName, summary.Size)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"backup":  summary,
		"message": "备份创建成功",
		"remoteSync": gin.H{
			"enabled": remoteEnabled,
			"status":  record.UploadStatus,
		},
	}))
}

func UploadBackup(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("获取上传文件失败"))
		return
	}

	if strings.ToLower(filepath.Ext(file.Filename)) != ".zip" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只支持上传 .zip 备份文件"))
		return
	}

	tempPath := filepath.Join(config.BackupDir, fmt.Sprintf(".upload-%d.zip", time.Now().UnixNano()))
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存上传文件失败: "+err.Error()))
		return
	}

	summary, _, err := utils.InspectBackupArchive(tempPath)
	if err != nil {
		os.Remove(tempPath)
		c.JSON(http.StatusBadRequest, models.ErrorResponse("上传的文件不是有效备份 ZIP: "+err.Error()))
		return
	}

	finalName := uniqueBackupFileName(utils.SanitizeBackupUploadName(file.Filename))
	finalPath := filepath.Join(config.BackupDir, finalName)
	if err := os.Rename(tempPath, finalPath); err != nil {
		os.Remove(tempPath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存备份文件失败: "+err.Error()))
		return
	}

	summary.Name = finalName
	summary.ID = strings.TrimSuffix(finalName, ".zip")
	checksum, err := computeFileSHA256(finalPath)
	if err != nil {
		log.Printf("[Backup] Failed to compute uploaded backup checksum: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份上传成功，但计算校验失败"))
		return
	}
	summary.ChecksumSHA256 = checksum

	remoteEnabled := config.Load().BackupRemoteEnabled
	record := createBackupRecordFromSummary(summary, finalPath, "", remoteEnabled)
	if err := getBackupRecordStorage().Upsert(record); err != nil {
		log.Printf("[Backup] Failed to persist uploaded backup record: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份上传成功，但记录备份状态失败"))
		return
	}
	if remoteEnabled {
		go syncBackupRecordToRemote(record.ID)
	}

	log.Printf("[Backup] Backup uploaded successfully: %s", finalName)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"backup":  summary,
		"message": "备份上传成功",
		"remoteSync": gin.H{
			"enabled": remoteEnabled,
			"status":  record.UploadStatus,
		},
	}))
}

func AnalyzeBackup(c *gin.Context) {
	backupPath, err := resolveBackupPath(c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的备份ID"))
		return
	}

	var req struct {
		TargetRoomID int `json:"targetRoomId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	room, err := roomStorage.GetByID(req.TargetRoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("目标房间不存在"))
		return
	}

	analysis, err := analyzeBackupRestore(backupPath, room)
	if err != nil {
		log.Printf("[Backup] Failed to analyze backup %s: %v", filepath.Base(backupPath), err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse("分析备份失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(analysis))
}

func RestoreBackup(c *gin.Context) {
	backupPath, err := resolveBackupPath(c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的备份ID"))
		return
	}

	var req struct {
		TargetRoomID int  `json:"targetRoomId" binding:"required"`
		CreateNew    bool `json:"createNew"`
		Force        bool `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	room, err := roomStorage.GetByID(req.TargetRoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("目标房间不存在"))
		return
	}

	analysis, err := analyzeBackupRestore(backupPath, room)
	if err != nil {
		log.Printf("[Backup] Failed to analyze backup %s before restore: %v", filepath.Base(backupPath), err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse("恢复前校验失败: "+err.Error()))
		return
	}

	if !analysis.CanRestore {
		c.JSON(http.StatusBadRequest, models.Response{
			Success: false,
			Error:   firstNonEmpty(analysis.FatalIssues, "当前条件下无法恢复备份"),
			Data:    gin.H{"analysis": analysis},
		})
		return
	}

	if analysis.RequiresForce && !req.Force {
		c.JSON(http.StatusConflict, models.Response{
			Success: false,
			Error:   "检测到兼容性风险，请确认后再继续恢复",
			Data:    gin.H{"analysis": analysis},
		})
		return
	}

	log.Printf("[Backup] Restoring backup %s to room #%d", filepath.Base(backupPath), room.ID)
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		log.Printf("[Backup] Failed to create room directory: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建房间目录失败"))
		return
	}

	if err := extractBackupArchive(backupPath, roomDir); err != nil {
		log.Printf("[Backup] Failed to restore backup archive: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("恢复备份失败: "+err.Error()))
		return
	}

	log.Printf("[Backup] Backup restored successfully to room #%d", room.ID)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"message":  "备份恢复成功",
		"analysis": analysis,
	}))
}

func DeleteBackup(c *gin.Context) {
	backupPath, err := resolveBackupPath(c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的备份ID"))
		return
	}

	log.Printf("[Backup] Deleting backup: %s", filepath.Base(backupPath))
	if err := os.Remove(backupPath); err != nil {
		log.Printf("[Backup] Failed to delete backup file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除备份文件失败"))
		return
	}

	log.Printf("[Backup] Backup deleted successfully: %s", filepath.Base(backupPath))
	if err := getBackupRecordStorage().Delete(c.Param("id")); err != nil {
		log.Printf("[Backup] Failed to delete backup record %s: %v", c.Param("id"), err)
	}
	c.JSON(http.StatusOK, models.MessageResponse("备份删除成功"))
}

func DownloadBackup(c *gin.Context) {
	backupPath, err := resolveBackupPath(c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的备份ID"))
		return
	}

	backupID := strings.TrimSuffix(filepath.Base(backupPath), ".zip")
	log.Printf("[Backup] Downloading backup: %s", backupID)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.zip", backupID))
	c.Header("Content-Type", "application/zip")
	c.File(backupPath)
}

func SyncBackupToRemote(c *gin.Context) {
	cfg := config.Load()
	if !cfg.BackupRemoteEnabled {
		c.JSON(http.StatusConflict, models.ErrorResponse("远端备份未启用"))
		return
	}

	record, err := ensureBackupRecordByID(c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("初始化备份记录失败: "+err.Error()))
		return
	}

	recordStorage := getBackupRecordStorage()
	if err := recordStorage.UpdateRemoteState(record.ID, record.StorageType, "pending", "", record.RemoteBucket, record.RemoteKey, record.RemoteETag, record.RemoteURL, record.UploadedAt); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("更新备份同步状态失败"))
		return
	}

	remoteService, err := services.NewBackupRemoteService(cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("初始化远端备份服务失败: "+err.Error()))
		return
	}
	result, err := remoteService.SyncBackup(context.Background(), record)
	if err != nil {
		_ = recordStorage.UpdateRemoteState(record.ID, record.StorageType, "failed", err.Error(), record.RemoteBucket, record.RemoteKey, record.RemoteETag, record.RemoteURL, record.UploadedAt)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("同步到远端失败: "+err.Error()))
		return
	}

	if err := recordStorage.UpdateRemoteState(record.ID, result.StorageType, "uploaded", "", result.Bucket, result.Key, result.ETag, result.RemoteURL, &result.UploadedAt); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存远端备份状态失败"))
		return
	}

	record, _ = recordStorage.GetByID(record.ID)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"message": "远端备份同步成功",
		"record":  record,
	}))
}

func VerifyBackupRemote(c *gin.Context) {
	cfg := config.Load()
	if !cfg.BackupRemoteEnabled {
		c.JSON(http.StatusConflict, models.ErrorResponse("远端备份未启用"))
		return
	}

	record, err := ensureBackupRecordByID(c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse("初始化备份记录失败: "+err.Error()))
		return
	}

	if strings.TrimSpace(record.RemoteKey) == "" {
		c.JSON(http.StatusConflict, models.ErrorResponse("该备份尚未同步到远端"))
		return
	}

	remoteService, err := services.NewBackupRemoteService(cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("初始化远端备份服务失败: "+err.Error()))
		return
	}

	result, err := remoteService.VerifyBackup(context.Background(), record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("远端备份校验失败: "+err.Error()))
		return
	}

	if err := getBackupRecordStorage().UpdateVerification(record.ID, result.VerifiedAt); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存校验时间失败"))
		return
	}

	record, _ = getBackupRecordStorage().GetByID(record.ID)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"message": "远端备份校验成功",
		"record":  record,
		"result":  result,
	}))
}

func analyzeBackupRestore(backupPath string, targetRoom *models.Room) (backupRestoreAnalysis, error) {
	summary, manifest, err := utils.InspectBackupArchive(backupPath)
	if err != nil {
		return backupRestoreAnalysis{}, err
	}

	if summary.WorldFile == "" && len(summary.DetectedWorldFiles) == 1 {
		summary.WorldFile = summary.DetectedWorldFiles[0]
	}

	analysis := backupRestoreAnalysis{
		Backup:     summary,
		TargetRoom: backupTargetRoomInfo{ID: targetRoom.ID, Name: targetRoom.Name, ServerType: targetRoom.ServerType, WorldFile: targetRoom.WorldFile, Status: targetRoom.Status},
		CanRestore: true,
	}

	if targetRoom.Status == "running" {
		appendAnalysisCheck(&analysis, "roomStatus", "目标房间状态", "fatal", "目标房间正在运行，必须先停止后再恢复。")
	} else {
		appendAnalysisCheck(&analysis, "roomStatus", "目标房间状态", "success", "目标房间已停止，可以执行恢复。")
	}

	if manifest != nil {
		appendAnalysisCheck(&analysis, "manifest", "备份元数据", "success", "备份包含面板元数据，可以识别来源房间、服务端类型和世界文件。")
	} else {
		appendAnalysisCheck(&analysis, "manifest", "备份元数据", "warning", "备份不包含面板元数据，来源信息只能通过文件名和压缩包内容推断。")
	}

	if summary.RoomID > 0 || strings.TrimSpace(summary.RoomName) != "" {
		if summary.RoomID == targetRoom.ID {
			appendAnalysisCheck(&analysis, "sourceRoom", "来源房间", "success", fmt.Sprintf("备份来源与目标房间一致：#%d %s。", targetRoom.ID, targetRoom.Name))
		} else {
			sourceLabel := summary.RoomName
			if sourceLabel == "" {
				sourceLabel = "未命名房间"
			}
			appendAnalysisCheck(&analysis, "sourceRoom", "来源房间", "warning", fmt.Sprintf("备份来源为房间 #%d %s，当前准备恢复到房间 #%d %s。", summary.RoomID, sourceLabel, targetRoom.ID, targetRoom.Name))
		}
	} else {
		appendAnalysisCheck(&analysis, "sourceRoom", "来源房间", "warning", "无法识别备份来源房间，请确认你选择的是正确的备份文件。")
	}

	if summary.ServerType == "" {
		appendAnalysisCheck(&analysis, "serverType", "服务端类型", "warning", "无法从备份中识别服务端类型，请手动确认和目标房间一致。")
	} else if summary.ServerType == targetRoom.ServerType {
		appendAnalysisCheck(&analysis, "serverType", "服务端类型", "success", fmt.Sprintf("备份类型为 %s，与目标房间一致。", getServerTypeName(summary.ServerType)))
	} else {
		appendAnalysisCheck(&analysis, "serverType", "服务端类型", "warning", fmt.Sprintf("备份类型为 %s，目标房间为 %s，恢复后可能出现配置错配。", getServerTypeName(summary.ServerType), getServerTypeName(targetRoom.ServerType)))
	}

	switch {
	case summary.WorldFile == "" && len(summary.DetectedWorldFiles) == 0:
		appendAnalysisCheck(&analysis, "worldFile", "世界文件", "warning", "压缩包中没有识别到世界文件，恢复后可能无法直接启动。")
	case summary.WorldFile == "" && len(summary.DetectedWorldFiles) > 1:
		appendAnalysisCheck(&analysis, "worldFile", "世界文件", "warning", fmt.Sprintf("压缩包中检测到多个世界文件：%s。请确认目标房间配置会指向正确的世界文件。", strings.Join(summary.DetectedWorldFiles, "、")))
	case summary.WorldFile == targetRoom.WorldFile:
		appendAnalysisCheck(&analysis, "worldFile", "世界文件", "success", fmt.Sprintf("备份世界文件为 %s，与目标房间配置一致。", targetRoom.WorldFile))
	default:
		if len(summary.DetectedWorldFiles) > 1 && slices.Contains(summary.DetectedWorldFiles, targetRoom.WorldFile) {
			appendAnalysisCheck(&analysis, "worldFile", "世界文件", "warning", fmt.Sprintf("压缩包中包含多个世界文件，其中包含目标房间配置的 %s。", targetRoom.WorldFile))
		} else {
			appendAnalysisCheck(&analysis, "worldFile", "世界文件", "warning", fmt.Sprintf("备份世界文件为 %s，目标房间当前配置为 %s。恢复后如不调整房间配置，可能不会加载到刚恢复的存档。", summary.WorldFile, targetRoom.WorldFile))
		}
	}

	analysis.RequiresForce = analysis.CanRestore && len(analysis.Warnings) > 0
	return analysis, nil
}

func appendAnalysisCheck(analysis *backupRestoreAnalysis, key string, label string, status string, detail string) {
	analysis.Checks = append(analysis.Checks, backupAnalysisCheck{
		Key:    key,
		Label:  label,
		Status: status,
		Detail: detail,
	})

	switch status {
	case "fatal":
		analysis.CanRestore = false
		analysis.FatalIssues = append(analysis.FatalIssues, detail)
	case "warning":
		analysis.Warnings = append(analysis.Warnings, detail)
	}
}

func extractBackupArchive(backupPath string, targetDir string) error {
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		normalizedPath, err := utils.NormalizeArchiveEntryPath(file.Name)
		if err != nil {
			return err
		}
		if normalizedPath == "" || normalizedPath == utils.BackupManifestName {
			continue
		}

		destinationPath, err := utils.ResolveArchiveExtractionPath(targetDir, file.Name)
		if err != nil {
			return err
		}
		if destinationPath == "" {
			continue
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destinationPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
			return err
		}

		sourceFile, err := file.Open()
		if err != nil {
			return err
		}

		mode := file.Mode()
		if mode == 0 {
			mode = 0644
		}
		targetFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			sourceFile.Close()
			return err
		}

		_, copyErr := io.Copy(targetFile, sourceFile)
		closeErr := targetFile.Close()
		sourceFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

func resolveBackupPath(backupID string) (string, error) {
	safeID := filepath.Base(strings.TrimSpace(backupID))
	if safeID == "" || safeID == "." {
		return "", fmt.Errorf("invalid backup id")
	}

	backupPath := filepath.Join(config.BackupDir, safeID+".zip")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return "", os.ErrNotExist
	}

	return backupPath, nil
}

func uniqueBackupFileName(fileName string) string {
	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	ext := filepath.Ext(fileName)
	candidate := fileName
	index := 1

	for {
		if _, err := os.Stat(filepath.Join(config.BackupDir, candidate)); os.IsNotExist(err) {
			return candidate
		}

		candidate = fmt.Sprintf("%s_%d%s", baseName, index, ext)
		index++
	}
}

func parseBackupDisplayTime(value string) time.Time {
	parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return time.Time{}
	}
	return parsedTime
}

func firstNonEmpty(values []string, fallback string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}

func getServerTypeName(serverType string) string {
	switch serverType {
	case "vanilla":
		return "原版"
	case "tmodloader":
		return "tModLoader"
	case "tshock":
		return "TShock"
	default:
		return serverType
	}
}
