package api
import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/utils"
	"github.com/gin-gonic/gin"
)
func GetPluginConfigs(c *gin.Context) {
	tshockDir := filepath.Join(config.ServersDir, "tshock")
	files, err := ioutil.ReadDir(tshockDir)
	if err != nil {
		log.Printf("[ERROR] Failed to read tshock directory: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取配置目录失败"))
		return
	}
	type ConfigFile struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		ModTime  int64  `json:"modTime"`
		IsMain   bool   `json:"isMain"`
	}
	var configFiles []ConfigFile
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		isMain := file.Name() == "config.json"
		configFiles = append(configFiles, ConfigFile{
			Name:    file.Name(),
			Size:    file.Size(),
			ModTime: file.ModTime().Unix(),
			IsMain:  isMain,
		})
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"files": configFiles,
	}))
}
func GetPluginConfigContent(c *gin.Context) {
	filename := c.Param("filename")
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法的文件名"))
		return
	}
	if filepath.Ext(filename) != ".json" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只能读取 JSON 配置文件"))
		return
	}
	configPath := filepath.Join(config.ServersDir, "tshock", filename)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("配置文件不存在"))
		return
	}
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Printf("[ERROR] Failed to read config file %s: %v", filename, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取配置文件失败"))
		return
	}
	var jsonCheck interface{}
	if err := json.Unmarshal(content, &jsonCheck); err != nil {
		log.Printf("[WARN] Config file %s is not valid JSON: %v", filename, err)
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"filename": filename,
		"content":  string(content),
		"size":     len(content),
	}))
}
func SavePluginConfig(c *gin.Context) {
	filename := c.Param("filename")
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法的文件名"))
		return
	}
	if filepath.Ext(filename) != ".json" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只能保存 JSON 配置文件"))
		return
	}
	var req struct {
		Content    string `json:"content" binding:"required"`
		HotReload  bool   `json:"hotReload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请求参数错误: "+err.Error()))
		return
	}
	var jsonCheck interface{}
	if err := json.Unmarshal([]byte(req.Content), &jsonCheck); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("JSON 格式错误: "+err.Error()))
		return
	}
	configPath := filepath.Join(config.ServersDir, "tshock", filename)
	if _, err := os.Stat(configPath); err == nil {
		backupPath := configPath + ".backup"
		if err := copyFile(configPath, backupPath); err != nil {
			log.Printf("[WARN] Failed to backup config file: %v", err)
		} else {
			log.Printf("[INFO] Backup created: %s", backupPath)
		}
	}
	if err := ioutil.WriteFile(configPath, []byte(req.Content), 0644); err != nil {
		log.Printf("[ERROR] Failed to save config file %s: %v", filename, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存配置文件失败"))
		return
	}
	log.Printf("[INFO] Config file saved: %s", filename)
	if req.HotReload {
		p, exists := utils.GetProcess(0)
		if exists && p.IsRunning() {
			log.Printf("[INFO] Hot reloading plugins after config change...")
			if err := p.SendCommand("reload"); err != nil {
				log.Printf("[ERROR] Failed to send reload command: %v", err)
				c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
					"message": "配置已保存，但热重载失败: " + err.Error(),
					"saved":   true,
					"reloaded": false,
				}))
				return
			}
			log.Printf("[INFO] Reload command sent successfully")
			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"message": "配置已保存并成功热重载",
				"saved":   true,
				"reloaded": true,
			}))
			return
		} else {
			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"message": "配置已保存（服务器未运行，无需热重载）",
				"saved":   true,
				"reloaded": false,
			}))
			return
		}
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"message": "配置已保存",
		"saved":   true,
		"reloaded": false,
	}))
}
