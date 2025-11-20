package api
import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"terraria-panel/services"
	"github.com/gin-gonic/gin"
)
var configService *services.ConfigService
func InitConfigService(tshockPath string) {
	configService = services.NewConfigService(tshockPath)
}
func CheckPluginServerConfig(c *gin.Context) {
	exists := configService.CheckConfigExists()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"exists": exists,
		},
	})
}
func InitializePluginServerConfig(c *gin.Context) {
	if configService.CheckConfigExists() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "配置文件已存在，无需初始化",
		})
		return
	}
	if err := configService.InitializeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化配置文件失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置文件初始化成功",
	})
}
func GetPluginServerConfig(c *gin.Context) {
	if !configService.CheckConfigExists() {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "配置文件不存在",
			"code":    "CONFIG_NOT_FOUND",
		})
		return
	}
	configJson, err := configService.GetConfigRaw()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取配置文件失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    json.RawMessage(configJson),
	})
}
func SavePluginServerConfig(c *gin.Context) {
	rawBody, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无法读取请求数据: " + err.Error(),
		})
		return
	}
	var config map[string]interface{}
	if err := json.Unmarshal(rawBody, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的JSON格式: " + err.Error(),
		})
		return
	}
	errors := configService.ValidateConfig(config)
	if len(errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "配置验证失败",
			"errors":  errors,
		})
		return
	}
	log.Printf("[INFO] Checking if config.json needs cleaning...")
	settingsRaw, hasSettings := config["Settings"]
	if !hasSettings {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "配置文件必须包含Settings对象",
		})
		return
	}
	settings, ok := settingsRaw.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Settings必须是一个对象",
		})
		return
	}
	hasTopLevelFields := false
	topLevelFields := []string{}
	for key := range config {
		if key != "Settings" {
			hasTopLevelFields = true
			topLevelFields = append(topLevelFields, key)
		}
	}
	var finalJSON []byte
	if hasTopLevelFields {
		log.Printf("[WARN] Found %d invalid top-level fields: %v", len(topLevelFields), topLevelFields)
		log.Printf("[INFO] Rebuilding config with only Settings (extracting from original JSON to preserve order)")
		configStr := string(rawBody)
		settingsStart := strings.Index(configStr, "\"Settings\"")
		if settingsStart == -1 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to find Settings in JSON",
			})
			return
		}
		braceStart := strings.Index(configStr[settingsStart:], "{")
		if braceStart == -1 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to find opening brace for Settings",
			})
			return
		}
		braceStart += settingsStart
		braceCount := 0
		inString := false
		escaped := false
		braceEnd := -1
		for i := braceStart; i < len(configStr); i++ {
			ch := configStr[i]
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if !inString {
				if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
					if braceCount == 0 {
						braceEnd = i
						break
					}
				}
			}
		}
		if braceEnd == -1 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to find closing brace for Settings",
			})
			return
		}
		settingsContent := configStr[braceStart : braceEnd+1]
		finalJSON = []byte("{\n  \"Settings\": " + settingsContent + "\n}")
		log.Printf("[INFO] Extracted Settings from original JSON, field order preserved")
	} else {
		finalJSON = rawBody
		log.Printf("[INFO] Config is clean, no top-level fields found")
	}
	if err := configService.SaveConfigRaw(finalJSON); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存配置文件失败: " + err.Error(),
		})
		return
	}
	port := 7777
	maxPlayers := 8
	serverName := ""
	password := ""
	if serverPort, ok := settings["ServerPort"].(float64); ok {
		port = int(serverPort)
	}
	if maxSlots, ok := settings["MaxSlots"].(float64); ok {
		maxPlayers = int(maxSlots)
	}
	if srvName, ok := settings["ServerName"].(string); ok {
		serverName = srvName
	}
	if srvPassword, ok := settings["ServerPassword"].(string); ok {
		password = srvPassword
	}
	if port == 7777 {
		if serverPort, ok := settings["服务器端口"].(float64); ok {
			port = int(serverPort)
		}
	}
	if maxPlayers == 8 {
		if maxSlots, ok := settings["最大人数"].(float64); ok {
			maxPlayers = int(maxSlots)
		}
	}
	if serverName == "" {
		if srvName, ok := settings["服务器名称"].(string); ok {
			serverName = srvName
		}
	}
	if password == "" {
		if pwd, ok := settings["服务器密码"].(string); ok {
			password = pwd
		}
	}
	log.Printf("[DEBUG] SavePluginServerConfig - Extracted from config.json:")
	log.Printf("[DEBUG]   Port: %d, MaxPlayers: %d, ServerName: %s", port, maxPlayers, serverName)
	if pluginServerService == nil {
		log.Printf("[ERROR] pluginServerService is nil, cannot sync to database")
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"message":          "配置文件已保存，但无法同步到数据库（服务未初始化）",
			"requires_restart": true,
			"warning":          "数据库未同步",
		})
		return
	}
	currentPS, err := pluginServerService.GetPluginServer()
	if err != nil {
		log.Printf("[ERROR] Failed to get current plugin server config: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"message":          "配置文件已保存，但无法读取当前配置: " + err.Error(),
			"requires_restart": true,
			"warning":          "数据库未同步",
		})
		return
	}
	if currentPS == nil {
		log.Printf("[ERROR] Plugin server config is nil")
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"message":          "配置文件已保存，但插件服配置不存在",
			"requires_restart": true,
			"warning":          "数据库未同步",
		})
		return
	}
	log.Printf("[DEBUG] Current database values:")
	log.Printf("[DEBUG]   Port: %d, MaxPlayers: %d, ServerName: %s", currentPS.Port, currentPS.MaxPlayers, currentPS.ServerName)
	log.Printf("[INFO] Syncing config.json changes to database...")
	if err := pluginServerService.UpdatePluginServerConfig(
		port, maxPlayers, password,
		currentPS.WorldName, currentPS.WorldSize, currentPS.Difficulty,
		currentPS.Seed, currentPS.WorldEvil, serverName,
		false,
	); err != nil {
		log.Printf("[ERROR] Failed to sync to database: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"message":          "配置文件已保存，但数据库同步失败: " + err.Error(),
			"requires_restart": true,
			"warning":          "数据库未同步",
		})
		return
	}
	log.Printf("[INFO] Successfully synced config.json to database")
	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          "配置已保存并同步到数据库",
		"requires_restart": true,
	})
}
