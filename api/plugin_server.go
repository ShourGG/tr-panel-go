package api

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/services"
	"terraria-panel/utils"
	"time"
)

var pluginServerService *services.PluginServerService

func SetPluginServerService(service *services.PluginServerService) {
	pluginServerService = service
}
func GetPluginServer(c *gin.Context) {
	pluginServer, err := pluginServerService.GetPluginServer()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get plugin server: "+err.Error()))
		return
	}
	if pluginServer == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin server not found"))
		return
	}
	if pluginServer.Status == "running" {
		if p, exists := utils.GetProcess(0); !exists || !p.IsRunning() {
			log.Printf("[WARN] Plugin server status mismatch: database says 'running' but process doesn't exist")
			pluginServerService.UpdatePluginServerStatus("stopped", 0)
			pluginServer.Status = "stopped"
			pluginServer.PID = 0
		}
	}
	configComplete := isBootstrapConfigurationComplete(pluginServer)
	response := gin.H{
		"success":        true,
		"data":           pluginServer,
		"configComplete": configComplete,
		"serverIp":       getServerIP(),
		"logSize":        getLogFileSize(),
		"tshockVersion":  getTShockVersion(),
	}
	c.JSON(http.StatusOK, response)
}
func getServerIP() string {
	if publicIP := getConfiguredPublicIP(); publicIP != "" && publicIP != "-" {
		return publicIP
	}
	return "未配置公网IP"
}
func getLogFileSize() string {
	logsDir := config.PluginServerLogsDir()
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		return "0 B"
	}
	var totalSize int64 = 0
	files, err := os.ReadDir(logsDir)
	if err != nil {
		return "0 B"
	}
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
		totalSize += info.Size()
	}
	return formatFileSize(totalSize)
}
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
func getTShockVersion() string {
	tshockPath := filepath.Join(config.ServersDir, "tshock")
	detection := detectInstalledTShockVersion(tshockPath)
	if detection.RawVersion != "" {
		raw := strings.TrimSpace(detection.RawVersion)
		raw = strings.TrimPrefix(raw, "v")
		raw = strings.TrimPrefix(raw, "V")
		if raw != "" {
			return raw
		}
	}
	if detection.Version != "unknown" {
		return detection.Version
	}
	if hasInstalledTShockBinary(tshockPath) {
		return "未知版本"
	}
	return "未安装"
}
func isBootstrapConfigurationComplete(ps *models.PluginServer) bool {
	if ps.Port < 1024 || ps.Port > 65535 {
		return false
	}
	if ps.MaxPlayers < 1 || ps.MaxPlayers > 255 {
		return false
	}
	if ps.WorldName == "" {
		return false
	}
	if ps.WorldSize < 1 || ps.WorldSize > 3 {
		return false
	}
	if ps.Difficulty < 0 || ps.Difficulty > 3 {
		return false
	}
	if ps.WorldEvil == "" {
		return false
	}
	return true
}

func isFullConfigurationComplete(ps *models.PluginServer) bool {
	if !isBootstrapConfigurationComplete(ps) {
		return false
	}
	return strings.TrimSpace(ps.ServerName) != ""
}
func StartPluginServer(c *gin.Context) {
	log.Printf("[INFO] Starting plugin server...")
	pluginServer, err := pluginServerService.GetPluginServer()
	if err != nil {
		log.Printf("[ERROR] Failed to get plugin server: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get plugin server: "+err.Error()))
		return
	}
	if pluginServer == nil {
		log.Printf("[ERROR] Plugin server not found")
		c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin server not found"))
		return
	}
	configPath := filepath.Join(config.ServersDir, "tshock", "config.json")
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
	}
	if !isBootstrapConfigurationComplete(pluginServer) {
		log.Printf("[WARN] Plugin server configuration is incomplete")
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请先完成插件服初始化参数配置，再启动插件服。"))
		return
	}
	if p, exists := utils.GetProcess(0); exists && p.IsRunning() {
		log.Printf("[WARN] Plugin server is already running (PID: %d)", p.GetPID())
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin server is already running"))
		return
	}
	executablePath, execErr := os.Executable()
	if execErr != nil {
		log.Printf("[ERROR] Failed to get executable path: %v", execErr)
	} else {
		log.Printf("[DEBUG] Executable path: %s", executablePath)
		log.Printf("[DEBUG] Executable directory: %s", filepath.Dir(executablePath))
	}
	log.Printf("[DEBUG] DataDir: %s", config.DataDir)
	log.Printf("[DEBUG] ServersDir: %s", config.ServersDir)
	log.Printf("[DEBUG] Operating System: %s", runtime.GOOS)
	globalTshockDir := filepath.Join(config.ServersDir, "tshock")
	log.Printf("[DEBUG] TShock directory: %s", globalTshockDir)
	var exePath string
	if runtime.GOOS == "windows" {
		exePath = filepath.Join(globalTshockDir, "TShock.Server.exe")
	} else {
		exePath = filepath.Join(globalTshockDir, "TShock.Server")
	}
	log.Printf("[DEBUG] TShock executable path to check: %s", exePath)
	fileInfo, statErr := os.Stat(exePath)
	if os.IsNotExist(statErr) {
		log.Printf("[ERROR] TShock server not found: %s", exePath)
		log.Printf("[ERROR] File does not exist at the expected path")
		if dirInfo, dirErr := os.Stat(globalTshockDir); dirErr == nil {
			log.Printf("[DEBUG] TShock directory exists: %s (IsDir: %v)", globalTshockDir, dirInfo.IsDir())
			if files, readErr := os.ReadDir(globalTshockDir); readErr == nil {
				log.Printf("[DEBUG] Files in TShock directory (%d total):", len(files))
				for i, file := range files {
					if i < 20 {
						log.Printf("[DEBUG]   - %s (IsDir: %v)", file.Name(), file.IsDir())
					}
				}
				if len(files) > 20 {
					log.Printf("[DEBUG]   ... and %d more files", len(files)-20)
				}
			} else {
				log.Printf("[ERROR] Failed to read TShock directory: %v", readErr)
			}
		} else {
			log.Printf("[ERROR] TShock directory does not exist: %s (error: %v)", globalTshockDir, dirErr)
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("TShock server not found. Please install TShock first."))
		return
	} else if statErr != nil {
		log.Printf("[ERROR] Failed to check TShock executable: %v", statErr)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(fmt.Sprintf("Failed to check TShock installation: %v", statErr)))
		return
	}
	log.Printf("[INFO] TShock executable found: %s (Size: %d bytes)", exePath, fileInfo.Size())
	if err := os.MkdirAll(globalTshockDir, 0755); err != nil {
		log.Printf("[ERROR] Failed to create tshock directory: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to create config directory: "+err.Error()))
		return
	}
	if configExists {
		log.Printf("[INFO] Config file exists: %s", configPath)
		log.Printf("[INFO] Syncing database configuration to config.json...")
		if err := pluginServerService.SyncDatabaseToConfigFile(pluginServer); err != nil {
			log.Printf("[ERROR] Failed to sync configuration: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to sync configuration: "+err.Error()))
			return
		}
		log.Printf("[INFO] Configuration synced successfully")
		log.Printf("[INFO] Enabling REST API...")
		configService := services.NewConfigService(globalTshockDir)
		if err := configService.EnableRESTAPI(); err != nil {
			log.Printf("[WARN] Failed to enable REST API: %v (continuing anyway)", err)
		} else {
			log.Printf("[INFO] REST API enabled successfully")
		}
	} else {
		log.Printf("[INFO] Config file not found, starting with official first-run flow: %s", configPath)
		log.Printf("[INFO] Skipping template initialization and config sync before first start")
	}
	pluginServerDir := filepath.Join(config.DataDir, "plugin-server")
	worldPath := filepath.Join(pluginServerDir, pluginServer.WorldFile)
	worldExists := false
	if _, err := os.Stat(worldPath); err == nil {
		worldExists = true
		log.Printf("[INFO] Using existing world file: %s", worldPath)
	} else {
		log.Printf("[INFO] World file not found, will auto-create: %s", worldPath)
	}
	var cmdName string
	var args []string
	if runtime.GOOS == "windows" {
		cmdName = "dotnet"
		args = []string{
			exePath,
			"-lang", "7",
			"-port", strconv.Itoa(pluginServer.Port),
			"-maxplayers", strconv.Itoa(pluginServer.MaxPlayers),
			"-configpath", globalTshockDir,
			"-worldpath", pluginServerDir,
			"-world", worldPath,
		}
	} else {
		cmdName = exePath
		args = []string{
			"-lang", "7",
			"-port", strconv.Itoa(pluginServer.Port),
			"-maxplayers", strconv.Itoa(pluginServer.MaxPlayers),
			"-configpath", globalTshockDir,
			"-worldpath", pluginServerDir,
			"-world", worldPath,
		}
	}
	log.Printf("[INFO] TShock will use config.json for all settings (port, maxplayers, etc.)")
	if !worldExists {
		args = append(args, "-autocreate", fmt.Sprintf("%d", pluginServer.WorldSize))
		args = append(args, "-worldname", pluginServer.WorldName)
		if pluginServer.Difficulty > 0 {
			args = append(args, "-difficulty", fmt.Sprintf("%d", pluginServer.Difficulty))
		}
		if pluginServer.Seed != "" {
			args = append(args, "-seed", pluginServer.Seed)
		}
		log.Printf("[INFO] World will be auto-created with size=%d, name=%s, difficulty=%d",
			pluginServer.WorldSize, pluginServer.WorldName, pluginServer.Difficulty)
	} else {
		args = append(args, "-autocreate", "0")
		log.Printf("[INFO] Using existing world, autocreate disabled")
	}
	logDir := config.PluginServerLogsDir()
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("[ERROR] Failed to create plugin server log directory: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to create log file"))
		return
	}
	logFile := config.PluginServerLogFile()
	logWriter, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[ERROR] Failed to create log file: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to create log file"))
		return
	}
	log.Printf("[INFO] Log file: %s", logFile)
	startTime := time.Now().Format("2006-01-02 15:04:05")
	startMarker := fmt.Sprintf(`
================================================================================
[%s] ========== 服务器启动 ==========
[%s] 启动时间: %s
[%s] 服务器端口: %d
[%s] 最大玩家: %d
[%s] 服务器名称: %s
[%s] 世界文件: %s
================================================================================
`, startTime, startTime, startTime, startTime, pluginServer.Port, startTime, pluginServer.MaxPlayers,
		startTime, pluginServer.ServerName, startTime, pluginServer.WorldFile)
	if _, err := logWriter.WriteString(startMarker); err != nil {
		log.Printf("[WARN] Failed to write start marker to log: %v", err)
	}
	log.Printf("[INFO] Starting plugin server with PTY mode for stable command input")
	log.Printf("[INFO] Command: %s %v", cmdName, args)
	process, err := utils.StartProcessWithPTY(0, cmdName, args, globalTshockDir, nil, logWriter, "tshock")
	if err != nil {
		log.Printf("[ERROR] Failed to start plugin server with PTY: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to start plugin server: "+err.Error()))
		return
	}
	time.Sleep(500 * time.Millisecond)
	if !process.IsRunning() {
		log.Printf("[ERROR] Plugin server process exited immediately, check log file: %s", logFile)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Plugin server failed to start. Please check log file."))
		return
	}
	pluginServerService.UpdatePluginServerStatus("running", process.GetPID())
	log.Printf("[INFO] Plugin server started successfully (PID: %d)", process.GetPID())
	c.JSON(http.StatusOK, models.MessageResponse("Plugin server started successfully"))
}
func StopPluginServer(c *gin.Context) {
	log.Printf("[INFO] Stopping plugin server...")
	processExists := true
	if err := utils.StopProcess(0); err != nil {
		log.Printf("[WARN] Failed to stop plugin server process: %v", err)
		processExists = false
	}
	if err := pluginServerService.UpdatePluginServerStatus("stopped", 0); err != nil {
		log.Printf("[ERROR] Failed to update plugin server status: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to update status: "+err.Error()))
		return
	}
	if processExists {
		log.Printf("[INFO] Plugin server stopped successfully")
		c.JSON(http.StatusOK, models.MessageResponse("Plugin server stopped successfully"))
	} else {
		log.Printf("[INFO] Plugin server status updated (process was not running)")
		c.JSON(http.StatusOK, models.MessageResponse("Plugin server status updated (process was not running)"))
	}
}
func RestartPluginServer(c *gin.Context) {
	log.Printf("[INFO] Restarting plugin server...")
	if p, exists := utils.GetProcess(0); exists && p.IsRunning() {
		if err := utils.StopProcess(0); err != nil {
			log.Printf("[ERROR] Failed to stop plugin server: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to stop plugin server: "+err.Error()))
			return
		}
		log.Printf("[INFO] Plugin server stopped")
	}
	StartPluginServer(c)
}
func SendPluginServerCommand(c *gin.Context) {
	var req struct {
		Command string `json:"command" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid request: "+err.Error()))
		return
	}
	p, exists := utils.GetProcess(0)
	if !exists || !p.IsRunning() {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("插件服未运行"))
		return
	}
	log.Printf("[INFO] Sending command to TShock via PTY: %s", req.Command)
	if err := p.SendCommand(req.Command); err != nil {
		log.Printf("[ERROR] Failed to send command via PTY: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("命令发送失败: "+err.Error()))
		return
	}
	log.Printf("[INFO] Command sent successfully via PTY: %s", req.Command)
	c.JSON(http.StatusOK, models.MessageResponse("命令已发送"))
}
func GetPluginServerLogs(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{
		Key:   "id",
		Value: "0",
	})
	GetServerLogs(c)
}
func UpdatePluginServerConfig(c *gin.Context) {
	var req struct {
		Port       int    `json:"port"`
		MaxPlayers int    `json:"maxPlayers"`
		Password   string `json:"password"`
		WorldName  string `json:"worldName"`
		WorldSize  int    `json:"worldSize"`
		Difficulty int    `json:"difficulty"`
		Seed       string `json:"seed"`
		WorldEvil  string `json:"worldEvil"`
		ServerName string `json:"serverName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid request: "+err.Error()))
		return
	}
	log.Printf("[DEBUG] UpdatePluginServerConfig received parameters:")
	log.Printf("[DEBUG]   Port: %d", req.Port)
	log.Printf("[DEBUG]   MaxPlayers: %d", req.MaxPlayers)
	log.Printf("[DEBUG]   Password: %s", req.Password)
	log.Printf("[DEBUG]   WorldName: %s", req.WorldName)
	log.Printf("[DEBUG]   WorldSize: %d", req.WorldSize)
	log.Printf("[DEBUG]   Difficulty: %d", req.Difficulty)
	log.Printf("[DEBUG]   Seed: %s", req.Seed)
	log.Printf("[DEBUG]   WorldEvil: %s", req.WorldEvil)
	log.Printf("[DEBUG]   ServerName: %s", req.ServerName)
	if pluginServerService == nil {
		log.Printf("[ERROR] pluginServerService is nil!")
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Plugin server service not initialized"))
		return
	}
	if err := pluginServerService.UpdatePluginServerConfig(
		req.Port, req.MaxPlayers, req.Password,
		req.WorldName, req.WorldSize, req.Difficulty,
		req.Seed, req.WorldEvil, req.ServerName,
		true,
	); err != nil {
		log.Printf("[ERROR] Failed to update plugin server config: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to update config: "+err.Error()))
		return
	}
	log.Printf("[INFO] Plugin server configuration updated successfully")
	c.JSON(http.StatusOK, models.MessageResponse("Plugin server configuration updated successfully"))
}
func GetPluginServerPlugins(c *gin.Context) {
	pluginsDir := services.GetPluginServerPluginsDir()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    collectPluginsFromDir(pluginsDir),
	})
}

func UploadPluginServerPlugin(c *gin.Context) {
	pluginsDir := services.GetPluginServerPluginsDir()
	file, ok := getUploadedPluginFile(c)
	if !ok {
		return
	}

	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件目录失败: "+err.Error()))
		return
	}

	dst := filepath.Join(pluginsDir, file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存插件失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已上传到插件服", file.Filename)))
}

func DeletePluginServerPlugin(c *gin.Context) {
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}

	pluginsDir := services.GetPluginServerPluginsDir()
	enabledPath := filepath.Join(pluginsDir, pluginName)
	disabledPath := filepath.Join(pluginsDir, "Disabled", pluginName)

	if _, err := os.Stat(enabledPath); err == nil {
		if err := os.Remove(enabledPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除插件失败: "+err.Error()))
			return
		}
		c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已从插件服删除", pluginName)))
		return
	}

	if _, err := os.Stat(disabledPath); err == nil {
		if err := os.Remove(disabledPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除插件失败: "+err.Error()))
			return
		}
		c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已从插件服删除", pluginName)))
		return
	}

	c.JSON(http.StatusNotFound, models.ErrorResponse("插件不存在: "+pluginName))
}

func TogglePluginServerPlugin(c *gin.Context) {
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}

	var req models.PluginToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid request body"))
		return
	}

	pluginsDir := services.GetPluginServerPluginsDir()
	disabledDir := filepath.Join(pluginsDir, "Disabled")
	if err := os.MkdirAll(disabledDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件目录失败: "+err.Error()))
		return
	}

	if req.Enabled {
		srcPath := filepath.Join(disabledDir, pluginName)
		destPath := filepath.Join(pluginsDir, pluginName)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin not found in disabled directory"))
			return
		}
		if err := moveFile(srcPath, destPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to enable plugin"))
			return
		}
		c.JSON(http.StatusOK, models.MessageResponse("Plugin enabled successfully"))
		return
	}

	srcPath := filepath.Join(pluginsDir, pluginName)
	destPath := filepath.Join(disabledDir, pluginName)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin not found in enabled directory"))
		return
	}
	if err := moveFile(srcPath, destPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to disable plugin"))
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("Plugin disabled successfully"))
}

func CopyPluginServerPluginToRoom(c *gin.Context) {
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}

	var req struct {
		RoomID int `json:"roomId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid request: "+err.Error()))
		return
	}
	if req.RoomID <= 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}

	roomPluginsDir, ok := resolveRoomPluginsDir(c, req.RoomID)
	if !ok {
		return
	}

	srcPath := filepath.Join(services.GetPluginServerPluginsDir(), pluginName)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("共享插件不存在: "+pluginName))
		return
	}

	if err := os.MkdirAll(roomPluginsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件目录失败: "+err.Error()))
		return
	}

	dstPath := filepath.Join(roomPluginsDir, pluginName)
	srcFile, err := os.Open(srcPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取共享插件失败"))
		return
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件文件失败"))
		return
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("复制插件失败"))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已从插件服复制到房间 %d", pluginName, req.RoomID)))
}
func InitializePluginServerOnStartup(db *sql.DB) error {
	service := services.NewPluginServerService(db)
	SetPluginServerService(service)
	return service.InitializePluginServer()
}
