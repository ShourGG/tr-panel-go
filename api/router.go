package api
import (
	"embed"
	"io/fs"
	"net/http"
	"terraria-panel/middleware"
	"github.com/gin-gonic/gin"
)
func SetupRouter(webFS embed.FS) *gin.Engine {
	r := gin.Default()
	r.Use(CORSMiddleware())
	apiGroup := r.Group("/api")
	apiGroup.Use(middleware.RateLimitMiddleware())
	{
		authGroup := apiGroup.Group("/auth")
		authGroup.Use(middleware.StrictRateLimitMiddleware())
		{
			authGroup.GET("/check-users", CheckHasUsers)
			authGroup.POST("/login", Login)
			authGroup.POST("/register", Register)
		}
		apiGroup.GET("/system/info", GetSystemInfo)
		apiGroup.GET("/system/cpu", GetCPU)
		apiGroup.GET("/system/memory", GetMemory)
		apiGroup.GET("/system/detail", GetSystemInfoDetail)
		apiGroup.GET("/game/check", CheckGameInstalled)
		apiGroup.GET("/game/install-info", GetGameInstallInfo)
		apiGroup.POST("/game/install", InstallGame)
		apiGroup.POST("/game/uninstall", UninstallGame)
		apiGroup.GET("/game/install-progress", GetInstallProgress)
		apiGroup.GET("/rooms", GetRooms)
		apiGroup.GET("/rooms/worlds", GetWorldsForRoom)
		apiGroup.POST("/rooms", CreateRoom)
		apiGroup.GET("/mods", GetMods)
		apiGroup.GET("/mods/search", SearchWorkshopMods)
		apiGroup.GET("/mods/downloading", GetDownloadingMods)
		apiGroup.POST("/mods", InstallMod)
		apiGroup.GET("/modconfig/profiles", GetModProfiles)
		apiGroup.POST("/modconfig/profiles", CreateModProfile)
		apiGroup.PUT("/modconfig/profiles/:id", UpdateModProfile)
		apiGroup.DELETE("/modconfig/profiles/:id", DeleteModProfile)
		apiGroup.GET("/steamcmd/check", CheckSteamCMD)
		apiGroup.GET("/steamcmd/status", GetSteamCMDStatus)
		apiGroup.POST("/steamcmd/install", InstallSteamCMDAPI)
		apiGroup.GET("/logs/panel", GetPanelLogs)
		apiGroup.GET("/logs/server/:id", GetServerLogs)
		apiGroup.GET("/logs/server/:id/files", GetServerLogFiles)
		apiGroup.GET("/logs/activity", GetRecentActivities)
		apiGroup.GET("/tasks", GetTasks)
		apiGroup.GET("/tasks/:id", GetTask)
		apiGroup.GET("/tasks/:id/logs", GetTaskLogs)
		apiGroup.GET("/stats/overview", GetStatsOverview)
		apiGroup.GET("/stats/rankings", GetRankings)
		apiGroup.GET("/stats/players", GetPlayerList)
		apiGroup.GET("/stats/trends", GetTrends)
		apiGroup.GET("/stats/distribution", GetDistribution)
		apiGroup.GET("/stats/sessions/:id", GetPlayerSessions)
		apiGroup.GET("/tshock-db/stats", GetTShockStats)
		protected := apiGroup.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			protected.GET("/worlds", ListWorlds)
			protected.POST("/worlds", CreateWorld)
			protected.DELETE("/worlds/:filename", DeleteWorld)
			protected.PUT("/rooms/:id", UpdateRoom)
			protected.DELETE("/rooms/:id", DeleteRoom)
			protected.POST("/rooms/:id/start", StartRoom)
			protected.POST("/rooms/:id/stop", StopRoom)
			protected.POST("/rooms/:id/restart", RestartRoom)
			protected.DELETE("/rooms/:id/admin-token", DeleteAdminToken)
			protected.POST("/rooms/:id/admin-token/regenerate", RegenerateAdminToken)
			protected.GET("/rooms/:id/plugins", GetRoomPlugins)
			protected.POST("/rooms/:id/plugins", AddRoomPlugin)
			protected.DELETE("/rooms/:id/plugins/:plugin", DeleteRoomPlugin)
			protected.POST("/rooms/:id/plugins/copy", CopyPluginFromShared)
			protected.GET("/plugins/shared", GetSharedPlugins)
			protected.GET("/plugin-server", GetPluginServer)
			protected.POST("/plugin-server/start", StartPluginServer)
			protected.POST("/plugin-server/stop", StopPluginServer)
			protected.POST("/plugin-server/restart", RestartPluginServer)
			protected.POST("/plugin-server/command", SendPluginServerCommand)
			protected.GET("/plugin-server/logs", GetPluginServerLogs)
			protected.PUT("/plugin-server/config", UpdatePluginServerConfig)
			protected.GET("/plugin-server/tshock-config/check", CheckPluginServerConfig)
			protected.POST("/plugin-server/tshock-config/initialize", InitializePluginServerConfig)
			protected.GET("/plugin-server/tshock-config", GetPluginServerConfig)
			protected.PUT("/plugin-server/tshock-config", SavePluginServerConfig)
			protected.GET("/plugins", GetPluginServerPlugins)
			protected.POST("/plugins", UploadPluginToServer)
			protected.DELETE("/plugins/:name", DeletePluginFromServer)
			protected.PUT("/plugins/:name/toggle", TogglePluginServer)
			protected.POST("/plugins/:name/copy-to-room", CopyPluginToRoom)
			protected.GET("/players", GetPlayers)
			protected.GET("/players/banned", GetBannedPlayers)
			protected.POST("/players/:id/kick", KickPlayer)
			protected.POST("/players/:id/ban", BanPlayer)
			protected.POST("/players/:id/unban", UnbanPlayer)
			protected.GET("/tshock-db/users", GetTShockUsers)
			protected.PUT("/tshock-db/users", UpdateTShockUser)
			protected.DELETE("/tshock-db/users/:id", DeleteTShockUser)
			protected.GET("/tshock-db/bans", GetTShockBans)
			protected.POST("/tshock-db/bans", AddTShockBan)
			protected.DELETE("/tshock-db/bans/:ticketNumber", RemoveTShockBan)
			protected.GET("/tshock-db/regions", GetTShockRegions)
			protected.GET("/tshock-db/warps", GetTShockWarps)
			protected.GET("/tshock-db/logs", GetTShockLogs)
			protected.GET("/user/server-mode", GetServerMode)
			protected.PUT("/user/server-mode", UpdateServerMode)
			protected.GET("/plugin-server/tshock-version", DetectTShockVersion)
			protected.GET("/files", ListFiles)
			protected.GET("/files/read", ReadFile)
			protected.POST("/files/write", WriteFile)
			protected.POST("/files/upload", UploadFile)
			protected.DELETE("/files", DeleteFile)
			protected.GET("/backups", GetBackups)
			protected.POST("/backups", CreateBackup)
			protected.POST("/backups/:id/restore", RestoreBackup)
			protected.DELETE("/backups/:id", DeleteBackup)
			protected.GET("/backups/:id/download", DownloadBackup)
			protected.POST("/tasks", CreateTask)
			protected.PUT("/tasks/:id", UpdateTask)
			protected.DELETE("/tasks/:id", DeleteTask)
			protected.POST("/tasks/:id/toggle", ToggleTask)
			protected.POST("/tasks/:id/execute", ExecuteTask)
			protected.DELETE("/tasks/:id/logs", DeleteTaskLogs)
			protected.POST("/mods/upload", UploadMod)
			protected.POST("/mods/:name/enable", EnableMod)
			protected.POST("/mods/:name/disable", DisableMod)
			protected.DELETE("/mods/:name", DeleteMod)
			protected.GET("/plugins/store", GetPluginStore)
			protected.POST("/rooms/:id/plugins/store/:pluginId/install", InstallPluginFromStore)
			protected.GET("/plugins/install-progress/:progressId", GetPluginInstallProgress)
			protected.GET("/plugin-configs", GetPluginConfigs)
			protected.GET("/plugin-configs/:filename", GetPluginConfigContent)
			protected.PUT("/plugin-configs/:filename", SavePluginConfig)
		}
		apiGroup.GET("/ws", HandleWebSocket)
		apiGroup.GET("/ws/rooms/:id/logs", HandleRoomLogsWS)
		apiGroup.GET("/ws/logs/:id", HandleRoomLogsWS)
	}
	distFS, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		panic("Failed to load frontend files: " + err.Error())
	}
	r.GET("/assets/*filepath", func(c *gin.Context) {
		filepath := c.Param("filepath")
		c.FileFromFS("assets"+filepath, http.FS(distFS))
	})
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path != "/" && path != "" {
			cleanPath := path
			if len(cleanPath) > 0 && cleanPath[0] == '/' {
				cleanPath = cleanPath[1:]
			}
			if fileInfo, err := fs.Stat(distFS, cleanPath); err == nil && !fileInfo.IsDir() {
				c.FileFromFS(cleanPath, http.FS(distFS))
				return
			}
		}
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
	return r
}
