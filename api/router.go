package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"terraria-panel/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter(webFS embed.FS) *gin.Engine {
	r := gin.Default()
	r.Use(CORSMiddleware())
	r.Use(GzipMiddleware())
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
		protected := apiGroup.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			protected.GET("/system/info", GetSystemInfo)
			protected.GET("/system/cpu", GetCPU)
			protected.GET("/system/memory", GetMemory)
			protected.GET("/system/detail", GetSystemInfoDetail)
			protected.GET("/system/panel-status", GetPanelStatus)
			protected.GET("/system/update-info", GetUpdateInfo)
			protected.POST("/system/upgrade", SelfUpgrade)
			protected.GET("/game/check", CheckGameInstalled)
			protected.GET("/game/install-info", GetGameInstallInfo)
			protected.GET("/game/install-progress", GetInstallProgress)
			protected.POST("/game/install", InstallGame)
			protected.POST("/game/runtime-repair", RepairGameRuntime)
			protected.POST("/game/update", UpdateGame)
			protected.POST("/game/uninstall", UninstallGame)
			protected.GET("/rooms", GetRooms)
			protected.GET("/rooms/worlds", GetWorldsForRoom)
			protected.POST("/rooms", CreateRoom)
			protected.GET("/mods", GetMods)
			protected.GET("/mods/search", SearchWorkshopMods)
			protected.GET("/mods/downloading", GetDownloadingMods)
			protected.POST("/mods", InstallMod)
			protected.GET("/modconfig/profiles", GetModProfiles)
			protected.POST("/modconfig/profiles", CreateModProfile)
			protected.PUT("/modconfig/profiles/:id", UpdateModProfile)
			protected.DELETE("/modconfig/profiles/:id", DeleteModProfile)
			protected.GET("/steamcmd/check", CheckSteamCMD)
			protected.GET("/steamcmd/status", GetSteamCMDStatus)
			protected.POST("/steamcmd/install", InstallSteamCMDAPI)
			protected.POST("/steamcmd/install-deps", InstallDepsAPI)
			protected.GET("/steamcmd/install-deps-status", GetDepsInstallStatus)
			protected.GET("/logs/panel", GetPanelLogs)
			protected.GET("/logs/server/:id", GetServerLogs)
			protected.GET("/logs/server/:id/files", GetServerLogFiles)
			protected.GET("/logs/activity", GetRecentActivities)
			protected.GET("/tasks", GetTasks)
			protected.GET("/tasks/:id", GetTask)
			protected.GET("/tasks/:id/logs", GetTaskLogs)
			protected.GET("/stats/overview", GetStatsOverview)
			protected.GET("/stats/rankings", GetRankings)
			protected.GET("/stats/players", GetPlayerList)
			protected.GET("/stats/trends", GetTrends)
			protected.GET("/stats/distribution", GetDistribution)
			protected.GET("/stats/sessions/:id", GetPlayerSessions)
			protected.GET("/tshock-db/stats", GetTShockStats)
			protected.GET("/auth/me", GetCurrentUser)
			protected.PUT("/auth/profile", UpdateCurrentUserProfile)
			protected.PUT("/auth/password", ChangeCurrentUserPassword)
			protected.GET("/worlds", ListWorlds)
			protected.POST("/worlds", CreateWorld)
			protected.DELETE("/worlds/:filename", DeleteWorld)
			protected.PUT("/rooms/:id", UpdateRoom)
			protected.DELETE("/rooms/:id", DeleteRoom)
			protected.POST("/rooms/:id/start", StartRoom)
			protected.POST("/rooms/:id/stop", StopRoom)
			protected.POST("/rooms/:id/restart", RestartRoom)
			protected.POST("/rooms/import-world", ImportWorld)
			protected.DELETE("/rooms/:id/admin-token", DeleteAdminToken)
			protected.POST("/rooms/:id/admin-token/regenerate", RegenerateAdminToken)
			protected.GET("/rooms/:id/plugins", GetRoomPlugins)
			protected.POST("/rooms/:id/plugins", AddRoomPlugin)
			protected.DELETE("/rooms/:id/plugins/:plugin", DeleteRoomPlugin)
			protected.POST("/rooms/:id/plugins/copy", CopyPluginFromShared)
			protected.GET("/plugins/shared", GetSharedPlugins)
			protected.GET("/plugin-server", GetPluginServer)
			protected.GET("/plugin-server/bootstrap-status", GetPluginServerBootstrapStatus)
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
			protected.GET("/plugin-server/plugins", GetPluginServerPlugins)
			protected.POST("/plugin-server/plugins", UploadPluginServerPlugin)
			protected.DELETE("/plugin-server/plugins/:name", DeletePluginServerPlugin)
			protected.PUT("/plugin-server/plugins/:name/toggle", TogglePluginServerPlugin)
			protected.POST("/plugin-server/plugins/:name/copy-to-room", CopyPluginServerPluginToRoom)
			protected.GET("/players", GetPlayers)
			protected.GET("/players/banned", GetBannedPlayers)
			protected.POST("/players/:id/kick", KickPlayer)
			protected.POST("/players/:id/ban", BanPlayer)
			protected.POST("/players/:id/unban", UnbanPlayer)
			protected.GET("/tshock-db/users", GetTShockUsers)
			protected.POST("/tshock-db/users", middleware.AdminMiddleware(), CreateTShockUser)
			protected.PUT("/tshock-db/users", middleware.AdminMiddleware(), UpdateTShockUser)
			protected.DELETE("/tshock-db/users/:id", middleware.AdminMiddleware(), DeleteTShockUser)
			protected.GET("/tshock-db/bans", GetTShockBans)
			protected.POST("/tshock-db/bans", middleware.AdminMiddleware(), AddTShockBan)
			protected.DELETE("/tshock-db/bans/:ticketNumber", middleware.AdminMiddleware(), RemoveTShockBan)
			protected.GET("/tshock-db/regions", GetTShockRegions)
			protected.GET("/tshock-db/warps", GetTShockWarps)
			protected.GET("/tshock-db/logs", GetTShockLogs)
			protected.GET("/user/server-mode", GetServerMode)
			protected.PUT("/user/server-mode", UpdateServerMode)
			protected.GET("/plugin-server/tshock-version", DetectTShockVersion)
			protected.GET("/files", ListFiles)
			protected.GET("/files/read", ReadFile)
			protected.GET("/files/download", DownloadFile)
			protected.POST("/files/write", WriteFile)
			protected.POST("/files/upload", UploadFile)
			protected.POST("/files/extract", ExtractFile)
			protected.POST("/files/rename", RenameFile)
			protected.DELETE("/files", DeleteFile)
			protected.GET("/backups", GetBackups)
			protected.POST("/backups", CreateBackup)
			protected.POST("/backups/upload", UploadBackup)
			protected.POST("/backups/:id/sync", SyncBackupToRemote)
			protected.POST("/backups/:id/verify", VerifyBackupRemote)
			protected.POST("/backups/:id/analyze", AnalyzeBackup)
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
			protected.GET("/plugin-configs/content", GetPluginConfigContentByQuery)
			protected.PUT("/plugin-configs/content", SavePluginConfigByQuery)
			protected.GET("/plugin-configs/:filename", GetPluginConfigContent)
			protected.PUT("/plugin-configs/:filename", SavePluginConfig)
		}
		apiGroup.GET("/ws", HandleWebSocket)
		apiGroup.GET("/ws/rooms/:id/logs", HandleRoomLogsWS)
		apiGroup.GET("/ws/logs/:id", HandleRoomLogsWS)
		apiGroup.GET("/ws/server/logs", HandleServerLogsWS)
	}
	// Backward-compatible WebSocket routes for older frontend bundles
	// that still connect to /ws instead of /api/ws.
	r.GET("/ws", HandleWebSocket)
	r.GET("/ws/rooms/:id/logs", HandleRoomLogsWS)
	r.GET("/ws/logs/:id", HandleRoomLogsWS)
	r.GET("/ws/server/logs", HandleServerLogsWS)
	distFS, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		panic("Failed to load frontend files: " + err.Error())
	}
	r.GET("/assets/*filepath", func(c *gin.Context) {
		filepath := c.Param("filepath")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.FileFromFS("assets"+filepath, http.FS(distFS))
	})
	r.NoRoute(func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if requestPath != "/" && requestPath != "" {
			cleanPath := requestPath
			if len(cleanPath) > 0 && cleanPath[0] == '/' {
				cleanPath = cleanPath[1:]
			}
			if fileInfo, err := fs.Stat(distFS, cleanPath); err == nil && !fileInfo.IsDir() {
				if strings.EqualFold(path.Base(cleanPath), "index.html") {
					c.Header("Cache-Control", "no-cache")
				} else {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				c.FileFromFS(cleanPath, http.FS(distFS))
				return
			}
		}
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
	return r
}
