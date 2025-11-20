package api
import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"github.com/gin-gonic/gin"
)
func GetPanelLogs(c *gin.Context) {
	lines := c.DefaultQuery("lines", "500")
	logFile := "logs/panel.log"
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"logs": []string{"No panel logs yet"},
		}))
		return
	}
	logs, err := readLastNLines(logFile, lines)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to read panel logs"))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"logs": logs,
	}))
}
func GetServerLogs(c *gin.Context) {
	roomID := c.Param("id")
	lines := c.DefaultQuery("lines", "500")
	logFileName := c.DefaultQuery("file", "")
	var logFile string
	if roomID == "0" {
		if logFileName != "" {
			logFile = filepath.Join(config.ServersDir, "tshock", "logs", logFileName)
		} else {
			logFile = filepath.Join(config.ServersDir, "tshock", "logs", "plugin-server.log")
		}
	} else {
		logFile = filepath.Join("data", "logs", "room-"+roomID+".log")
	}
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"logs": "服务器尚未启动或暂无日志",
		}))
		return
	}
	logsArray, err := readLastNLines(logFile, lines)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取服务器日志失败"))
		return
	}
	logsString := strings.Join(logsArray, "\n")
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"logs": logsString,
	}))
}
func GetServerLogFiles(c *gin.Context) {
	roomID := c.Param("id")
	if roomID != "0" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("此功能仅支持插件服"))
		return
	}
	logsDir := filepath.Join(config.ServersDir, "tshock", "logs")
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"files": []interface{}{},
		}))
		return
	}
	files, err := os.ReadDir(logsDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取日志目录失败: " + err.Error()))
		return
	}
	type LogFile struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		ModTime  int64  `json:"modTime"`
	}
	var logFiles []LogFile
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) != ".log" {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		logFiles = append(logFiles, LogFile{
			Name:    file.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"files": logFiles,
	}))
}
func readLastNLines(filename string, n string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
	}
	return lines, nil
}
