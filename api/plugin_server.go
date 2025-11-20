package api
import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
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
	"github.com/gin-gonic/gin"
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
	configComplete := isConfigurationComplete(pluginServer)
	response := gin.H{
		"success":        true,
		"data":           pluginServer,
		"configComplete": configComplete,
		"serverIp":      getServerIP(),
		"logSize":       getLogFileSize(),
		"tshockVersion": getTShockVersion(),
	}
	c.JSON(http.StatusOK, response)
}
func getServerIP() string {
	if ip := os.Getenv("SERVER_IP"); ip != "" {
		return ip
	}
	if publicIP := getPublicIP(); publicIP != "" {
		return publicIP
	}
	return "未配置公网IP"
}
func getPublicIP() string {
	apis := []string{
		"https://api.ipify.org",
		"https://ifconfig.me",
		"https://icanhazip.com",
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	for _, api := range apis {
		resp, err := client.Get(api)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				ip := strings.TrimSpace(string(body))
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}
	return ""
}
func getLogFileSize() string {
	logsDir := filepath.Join(config.ServersDir, "tshock", "logs")
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
	versionFile := filepath.Join(config.ServersDir, "tshock", ".tshock_version")
	if data, err := os.ReadFile(versionFile); err == nil {
		version := strings.TrimSpace(string(data))
		if version == "5" {
			return "5.2.4"
		} else if version == "6" {
			return "6.0.0-pre1"
		}
	}
	tshockDir := filepath.Join(config.ServersDir, "tshock")
	serverExe := filepath.Join(tshockDir, "TShock.Server")
	dllFile := filepath.Join(tshockDir, "TShock.Server.dll")
	if _, err := os.Stat(serverExe); os.IsNotExist(err) {
		if _, err := os.Stat(dllFile); os.IsNotExist(err) {
			return "未安装"
		}
	}
	return "未知版本"
}
func isConfigurationComplete(ps *models.PluginServer) bool {
	if ps.ServerName == "" {
		return false
	}
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
	if !isConfigurationComplete(pluginServer) {
		log.Printf("[WARN] Plugin server configuration is incomplete")
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请先完成插件服配置。点击「快速设置」按钮配置服务器参数。"))
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
	configPath := filepath.Join(globalTshockDir, "config.json")
	if err := os.MkdirAll(globalTshockDir, 0755); err != nil {
		log.Printf("[ERROR] Failed to create tshock directory: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to create config directory: "+err.Error()))
		return
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("[INFO] Config file not found, will be initialized: %s", configPath)
		if err := pluginServerService.InitializeConfigFile(); err != nil {
			log.Printf("[ERROR] Failed to initialize config file: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to initialize config file: "+err.Error()))
			return
		}
		log.Printf("[INFO] Config file initialized successfully")
	} else {
		log.Printf("[INFO] Config file exists: %s", configPath)
	}
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
			"-configpath", globalTshockDir,
			"-worldpath", pluginServerDir,
			"-world", worldPath,
		}
	} else {
		cmdName = exePath
		args = []string{
			"-lang", "7",
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
	logDir := filepath.Join(globalTshockDir, "logs")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "plugin-server.log")
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
	c.Params = append(c.Params, gin.Param{
		Key:   "id",
		Value: "0",
	})
	GetRoomPlugins(c)
}
func UploadPluginToServer(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{
		Key:   "id",
		Value: "0",
	})
	AddRoomPlugin(c)
}
func DeletePluginFromServer(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{
		Key:   "id",
		Value: "0",
	})
	DeleteRoomPlugin(c)
}
func TogglePluginServer(c *gin.Context) {
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}
	c.Params = append(c.Params, gin.Param{
		Key:   "id",
		Value: "0",
	})
	TogglePlugin(c)
}
func CopyPluginToRoom(c *gin.Context) {
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}
	var req struct {
		RoomID int `json:"roomId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid request: "+err.Error()))
		return
	}
	if req.RoomID == services.PluginServerID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Cannot copy plugin to plugin server itself"))
		return
	}
	c.Params = append(c.Params, gin.Param{
		Key:   "id",
		Value: strconv.Itoa(req.RoomID),
	})
	c.Request.Body = http.NoBody
	c.Set("pluginName", pluginName)
	CopyPluginFromShared(c)
}
func InitializePluginServerOnStartup(db *sql.DB) error {
	service := services.NewPluginServerService(db)
	SetPluginServerService(service)
	return service.InitializePluginServer()
}
