package api
import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/storage"
	"time"
	"github.com/gin-gonic/gin"
)
func GetBackups(c *gin.Context) {
	entries, err := os.ReadDir(config.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取备份列表失败"))
		return
	}
	backups := []gin.H{}
	re := regexp.MustCompile(`^room-(\d+)_(.+)_(\d{8}_\d{6})\.zip$`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		matches := re.FindStringSubmatch(entry.Name())
		var roomID int
		var roomName string
		var timestamp string
		if len(matches) == 4 {
			roomID, _ = strconv.Atoi(matches[1])
			roomName = matches[2]
			timestamp = matches[3]
		}
		createdAt := info.ModTime().Format("2006-01-02 15:04:05")
		if timestamp != "" {
			if t, err := time.Parse("20060102_150405", timestamp); err == nil {
				createdAt = t.Format("2006-01-02 15:04:05")
			}
		}
		backups = append(backups, gin.H{
			"id":        strings.TrimSuffix(entry.Name(), ".zip"),
			"name":      entry.Name(),
			"roomId":    roomID,
			"roomName":  roomName,
			"type":      "full",
			"size":      info.Size(),
			"createdAt": createdAt,
		})
	}
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
	if req.Type == "" {
		req.Type = "full"
	}
	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	room, err := roomStorage.GetByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}
	timestamp := time.Now().Format("20060102_150405")
	zipName := fmt.Sprintf("room-%d_%s_%s.zip", room.ID, room.Name, timestamp)
	zipPath := filepath.Join(config.BackupDir, zipName)
	log.Printf("[Backup] Creating backup for room #%d: %s", room.ID, zipName)
	zipFile, err := os.Create(zipPath)
	if err != nil {
		log.Printf("[Backup] Failed to create ZIP file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建备份文件失败"))
		return
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if _, err := os.Stat(roomDir); os.IsNotExist(err) {
		log.Printf("[Backup] Room directory does not exist: %s", roomDir)
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间目录不存在"))
		return
	}
	if err := addDirToZip(zipWriter, roomDir, ""); err != nil {
		log.Printf("[Backup] Failed to add room directory to ZIP: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("备份房间数据失败: "+err.Error()))
		return
	}
	log.Printf("[Backup] Backup created successfully: %s (size: %d bytes)", zipName, getFileSize(zipPath))
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"id":   strings.TrimSuffix(zipName, ".zip"),
		"name": zipName,
		"size": getFileSize(zipPath),
		"message": "备份创建成功",
	}))
}
func RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")
	var req struct {
		TargetRoomID int  `json:"targetRoomId" binding:"required"`
		CreateNew    bool `json:"createNew"`
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
	if room.Status == "running" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请先停止房间再恢复备份"))
		return
	}
	backupPath := filepath.Join(config.BackupDir, backupID+".zip")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
		return
	}
	log.Printf("[Backup] Restoring backup %s to room #%d", backupID, room.ID)
	zipReader, err := zip.OpenReader(backupPath)
	if err != nil {
		log.Printf("[Backup] Failed to open backup file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("打开备份文件失败"))
		return
	}
	defer zipReader.Close()
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		log.Printf("[Backup] Failed to create room directory: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建房间目录失败"))
		return
	}
	for _, file := range zipReader.File {
		destPath := filepath.Join(roomDir, file.Name)
		if file.FileInfo().IsDir() {
			os.MkdirAll(destPath, file.Mode())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			log.Printf("[Backup] Failed to create directory: %v", err)
			continue
		}
		srcFile, err := file.Open()
		if err != nil {
			log.Printf("[Backup] Failed to open file in ZIP: %v", err)
			continue
		}
		dstFile, err := os.Create(destPath)
		if err != nil {
			log.Printf("[Backup] Failed to create destination file: %v", err)
			srcFile.Close()
			continue
		}
		_, err = io.Copy(dstFile, srcFile)
		dstFile.Close()
		srcFile.Close()
		if err != nil {
			log.Printf("[Backup] Failed to copy file: %v", err)
		}
	}
	log.Printf("[Backup] Backup restored successfully to room #%d", room.ID)
	c.JSON(http.StatusOK, models.MessageResponse("备份恢复成功"))
}
func DeleteBackup(c *gin.Context) {
	backupID := c.Param("id")
	backupPath := filepath.Join(config.BackupDir, backupID+".zip")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
		return
	}
	log.Printf("[Backup] Deleting backup: %s", backupID)
	if err := os.Remove(backupPath); err != nil {
		log.Printf("[Backup] Failed to delete backup file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除备份文件失败"))
		return
	}
	log.Printf("[Backup] Backup deleted successfully: %s", backupID)
	c.JSON(http.StatusOK, models.MessageResponse("备份删除成功"))
}
func DownloadBackup(c *gin.Context) {
	backupID := c.Param("id")
	backupPath := filepath.Join(config.BackupDir, backupID+".zip")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("备份文件不存在"))
		return
	}
	log.Printf("[Backup] Downloading backup: %s", backupID)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.zip", backupID))
	c.Header("Content-Type", "application/zip")
	c.File(backupPath)
}
func addDirToZip(zipWriter *zip.Writer, sourceDir, baseInZip string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		zipPath := filepath.Join(baseInZip, relPath)
		zipPath = filepath.ToSlash(zipPath)
		if zipPath == "." || zipPath == "" {
			return nil
		}
		if info.IsDir() {
			_, err := zipWriter.Create(zipPath + "/")
			return err
		}
		return addFileToZip(zipWriter, path, zipPath)
	})
}
func addFileToZip(zipWriter *zip.Writer, filePath, fileName string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	writer, err := zipWriter.Create(fileName)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
}
func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
