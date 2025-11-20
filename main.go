package main
import (
	"embed"
	"log"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"terraria-panel/api"
	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/scheduler"
	"terraria-panel/services"
	"terraria-panel/storage"
	"terraria-panel/utils"
	"github.com/gin-gonic/gin"
)
//go:embed all:web/dist
var webFS embed.FS
func main() {
	debug.SetGCPercent(200)
	runtime.GOMAXPROCS(runtime.NumCPU())
	if err := utils.InitLogger(); err != nil {
		log.Fatal("❌ 日志系统初始化失败:", err)
	}
	defer utils.CloseLogger()
	cfg := config.Load()
	utils.LogInfo("========================================")
	utils.LogInfo("🚀 泰拉瑞亚服务器管理面板启动中...")
	utils.LogInfo("========================================")
	utils.LogInfo("📂 数据目录: %s", config.DataDir)
	utils.LogInfo("🌐 监听端口: %s", cfg.Port)
	utils.LogInfo("🔧 运行模式: %s", cfg.Env)
	dbPath := filepath.Join(config.DataDir, "panel.db")
	log.Printf("💾 初始化数据库: %s", dbPath)
	if err := db.Init(dbPath); err != nil {
		log.Fatal("❌ 数据库初始化失败:", err)
	}
	defer db.Close()
	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	userStorage := storage.NewSQLiteUserStorage(db.DB)
	taskStorage := storage.NewSQLiteTaskStorage(db.DB)
	sessionStorage := storage.NewSQLitePlayerSessionStorage(db.DB)
	statsStorage := storage.NewSQLitePlayerStatsStorage(db.DB)
	dailyStatsStorage := storage.NewSQLitePlayerDailyStatsStorage(db.DB)
	api.SetRoomStorage(roomStorage)
	api.SetUserStorage(userStorage)
	api.InitStatsStorage(db.DB)
	var userCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	log.Printf("👥 数据库用户数: %d", userCount)
	var roomCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&roomCount)
	log.Printf("🏠 数据库房间数: %d", roomCount)
	log.Println("📦 初始化模组配置表...")
	if err := api.InitModProfilesTable(); err != nil {
		log.Printf("⚠️  模组配置表初始化失败: %v", err)
	} else {
		log.Println("✅ 模组配置表初始化成功")
	}
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	log.Println("📊 初始化系统监控...")
	api.InitSystemMonitoring()
	log.Println("⏰ 初始化定时任务调度器...")
	backupHandler := scheduler.NewBackupHandler(roomStorage)
	restartHandler := scheduler.NewRestartHandler(roomStorage)
	cleanupBackupHandler := scheduler.NewCleanupBackupHandler(roomStorage)
	cleanupLogHandler := scheduler.NewCleanupLogHandler(roomStorage)
	broadcastHandler := scheduler.NewBroadcastHandler(roomStorage)
	customCommandHandler := scheduler.NewCustomCommandHandler(roomStorage)
	executor := scheduler.NewTaskExecutor(
		roomStorage,
		taskStorage,
		backupHandler,
		restartHandler,
		cleanupBackupHandler,
		cleanupLogHandler,
		broadcastHandler,
		customCommandHandler,
	)
	taskScheduler := scheduler.NewScheduler(taskStorage, executor)
	api.InitTaskScheduler(taskStorage, taskScheduler)
	if err := taskScheduler.Start(); err != nil {
		log.Printf("⚠️  定时任务调度器启动失败: %v", err)
	} else {
		log.Println("✅ 定时任务调度器启动成功")
	}
	log.Println("📊 初始化玩家统计服务...")
	logMonitor := services.NewLogMonitor(db.DB, roomStorage, sessionStorage, statsStorage, dailyStatsStorage)
	logMonitor.Start()
	defer logMonitor.Stop()
	log.Println("✅ 玩家统计服务启动成功")
	log.Println("🔌 初始化插件服...")
	if err := api.InitializePluginServerOnStartup(db.DB); err != nil {
		log.Printf("⚠️  插件服初始化失败: %v", err)
	} else {
		log.Println("✅ 插件服初始化成功")
	}
	log.Println("⚙️  初始化配置服务...")
	tshockPath := filepath.Join(config.ServersDir, "tshock")
	api.InitConfigService(tshockPath)
	log.Println("✅ 配置服务初始化成功")
	r := api.SetupRouter(webFS)
	log.Println("========================================")
	log.Println("✅ 服务器启动成功！")
	log.Println("========================================")
	log.Printf("🔗 访问地址: http://localhost:%s", cfg.Port)
	log.Printf("🔗 外网访问: http://YOUR_IP:%s", cfg.Port)
	if userCount == 0 {
		log.Println("🚀 首次使用，请访问面板注册管理员账号")
	} else {
		log.Printf("👤 系统已有 %d 个用户", userCount)
	}
	log.Println("========================================")
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal("❌ 启动失败:", err)
	}
}
