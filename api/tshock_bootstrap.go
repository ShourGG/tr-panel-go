package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"terraria-panel/config"
	"terraria-panel/services"
	"terraria-panel/utils"

	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/go-sqlite"
)

type TShockBootstrapStatus struct {
	Installed     bool     `json:"installed"`
	RuntimeReady  bool     `json:"runtimeReady"`
	ConfigExists  bool     `json:"configExists"`
	StorageType   string   `json:"storageType"`
	DBPath        string   `json:"dbPath"`
	DBExists      bool     `json:"dbExists"`
	SchemaReady   bool     `json:"schemaReady"`
	MissingTables []string `json:"missingTables"`
	ServerRunning bool     `json:"serverRunning"`
	SetupToken    string   `json:"setupToken"`
	SetupStep     string   `json:"setupStep"`
	Hint          string   `json:"hint"`
	TShockVersion string   `json:"tshockVersion"`
}

type tshockConfigSnapshot struct {
	Settings struct {
		StorageType  string `json:"StorageType"`
		SqliteDBPath string `json:"SqliteDBPath"`
	} `json:"Settings"`
}

var setupTokenRegexp = regexp.MustCompile(`(?i)(/(?:setup|auth)\s+\d+)`)

func GetPluginServerBootstrapStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    getTShockBootstrapStatus(),
	})
}

func getTShockBootstrapStatus() TShockBootstrapStatus {
	status := TShockBootstrapStatus{
		Installed:     checkTShockInstalled(),
		StorageType:   "sqlite",
		DBPath:        filepath.Join(services.GetGlobalTShockDir(), "tshock.sqlite"),
		MissingTables: []string{},
		ServerRunning: isPluginServerRunning(),
		TShockVersion: getTShockVersion(),
	}

	if status.Installed {
		status.RuntimeReady = isRequiredDotNetRuntimeInstalled(getRequiredDotNetRuntime(status.TShockVersion))
	}

	storageType, dbPath, configExists, configErr := getTShockStorageInfo()
	status.StorageType = storageType
	status.DBPath = dbPath
	status.ConfigExists = configExists

	if storageType == "sqlite" && dbPath != "" {
		status.DBExists = fileExists(dbPath)
	}

	userCount := 0
	if status.StorageType == "sqlite" && status.DBExists {
		status.SchemaReady, status.MissingTables, userCount = inspectSQLiteSchema(dbPath)
	}

	if userCount == 0 {
		status.SetupToken = getLatestSetupToken()
	}

	status.SetupStep, status.Hint = determineBootstrapStep(status, userCount, configErr)
	return status
}

func isPluginServerRunning() bool {
	if p, exists := utils.GetProcess(0); exists && p.IsRunning() {
		return true
	}
	return false
}

func getTShockStorageInfo() (storageType, dbPath string, configExists bool, err error) {
	configPath := filepath.Join(config.ServersDir, "tshock", "config.json")
	defaultDBPath := filepath.Join(services.GetGlobalTShockDir(), "tshock.sqlite")

	if !fileExists(configPath) {
		return "sqlite", defaultDBPath, false, nil
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		return "sqlite", defaultDBPath, true, readErr
	}

	var snapshot tshockConfigSnapshot
	if unmarshalErr := json.Unmarshal(data, &snapshot); unmarshalErr != nil {
		return "sqlite", defaultDBPath, true, unmarshalErr
	}

	storageType = strings.ToLower(strings.TrimSpace(snapshot.Settings.StorageType))
	if storageType == "" {
		storageType = "sqlite"
	}

	if storageType != "sqlite" {
		return storageType, "", true, nil
	}

	dbPath = strings.TrimSpace(snapshot.Settings.SqliteDBPath)
	if dbPath == "" {
		dbPath = "tshock.sqlite"
	}
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(services.GetGlobalTShockDir(), dbPath)
	}

	return storageType, dbPath, true, nil
}

func inspectSQLiteSchema(dbPath string) (schemaReady bool, missingTables []string, userCount int) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return false, []string{"Users"}, 0
	}
	defer db.Close()

	requiredTables := []string{"Users", "PlayerBans", "Regions", "Warps", "Logs"}
	missingTables = make([]string, 0, len(requiredTables))

	for _, table := range requiredTables {
		if !sqliteTableExists(db, table) {
			missingTables = append(missingTables, table)
		}
	}

	if len(missingTables) == 0 || !containsString(missingTables, "Users") {
		_ = db.QueryRow("SELECT COUNT(*) FROM Users").Scan(&userCount)
	}

	return !containsString(missingTables, "Users"), missingTables, userCount
}

func sqliteTableExists(db *sql.DB, table string) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count)
	return err == nil && count > 0
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func getLatestSetupToken() string {
	if token := extractSetupToken(utils.GetPluginServerOutputBuffer()); token != "" {
		return token
	}

	logFile := config.PluginServerLogFile()
	if !fileExists(logFile) {
		return ""
	}

	tail, err := readFileTail(logFile, 256*1024)
	if err != nil {
		return ""
	}

	return extractSetupToken(tail)
}

func extractSetupToken(text string) string {
	if text == "" {
		return ""
	}
	if marker := strings.LastIndex(text, "========== 服务器启动 =========="); marker >= 0 {
		text = text[marker:]
	}
	matches := setupTokenRegexp.FindAllString(text, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func readFileTail(path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	size := info.Size()
	start := int64(0)
	if size > limit {
		start = size - limit
	}

	buf := make([]byte, size-start)
	if _, err := file.ReadAt(buf, start); err != nil {
		return "", err
	}

	return string(buf), nil
}

func getRequiredDotNetRuntime(version string) string {
	if strings.HasPrefix(strings.TrimSpace(version), "6") {
		return "9.0"
	}
	return "6.0"
}

func isRequiredDotNetRuntimeInstalled(required string) bool {
	if required == "" {
		return true
	}

	dotnetPath, err := exec.LookPath("dotnet")
	if err != nil {
		return false
	}

	output, err := exec.Command(dotnetPath, "--list-runtimes").CombinedOutput()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), "Microsoft.NETCore.App "+required)
}

func getRuntimeInstallHint(version string) string {
	required := getRequiredDotNetRuntime(version)
	return "请先安装对应的 .NET Runtime：" + buildManualDotNetInstallCommand(required, "")
}

func determineBootstrapStep(status TShockBootstrapStatus, userCount int, configErr error) (string, string) {
	if !status.Installed {
		return "install-required", "请先在安装游戏页面安装 TShock 插件服。"
	}

	if !status.RuntimeReady {
		return "runtime-required", getRuntimeInstallHint(status.TShockVersion)
	}

	if configErr != nil {
		return "config-error", "配置文件解析失败，请检查 config.json 是否损坏。"
	}

	if status.StorageType != "sqlite" {
		return "unsupported-storage", fmt.Sprintf("当前数据库类型为 %s，面板暂不支持直接可视化该类型数据库。", status.StorageType)
	}

	if !status.ConfigExists && !status.ServerRunning {
		return "start-required", "请点击“开始初始化”，首次启动后会生成配置和数据库。"
	}

	if status.ServerRunning && !status.DBExists {
		return "creating-database", "插件服正在首次启动，等待 TShock 生成数据库文件。"
	}

	if status.DBExists && !status.SchemaReady {
		return "schema-initializing", "数据库文件已创建，但初始化尚未完成，请等待插件服继续启动。"
	}

	if status.SchemaReady && userCount == 0 && status.SetupToken != "" {
		return "awaiting-admin-binding", "按控制台提示的初始化口令进入游戏完成管理员绑定。"
	}

	if status.SchemaReady && userCount > 0 {
		return "ready", "初始化已完成，现在可以进入数据库管理和插件管理。"
	}

	return "start-required", "请点击“开始初始化”，完成插件服首次启动。"
}

func respondTShockDBStateError(c *gin.Context, httpStatus int, code, message string, status TShockBootstrapStatus) {
	c.JSON(httpStatus, gin.H{
		"success": false,
		"code":    code,
		"message": message,
		"data": gin.H{
			"bootstrap": status,
		},
	})
}

func openReadyTShockSQLite(c *gin.Context) (*sql.DB, TShockBootstrapStatus, bool) {
	status := getTShockBootstrapStatus()
	if status.StorageType != "sqlite" {
		respondTShockDBStateError(c, http.StatusConflict, "TSHOCK_DB_UNSUPPORTED_STORAGE", "当前数据库类型不是 SQLite，暂不支持可视化管理。", status)
		return nil, status, false
	}

	if !status.DBExists || !status.SchemaReady {
		respondTShockDBStateError(c, http.StatusConflict, "TSHOCK_DB_NOT_READY", "TShock 数据库尚未初始化完成，请先完成插件服首次启动。", status)
		return nil, status, false
	}

	db, err := sql.Open("sqlite", status.DBPath)
	if err != nil {
		respondTShockDBStateError(c, http.StatusInternalServerError, "TSHOCK_DB_OPEN_FAILED", "无法打开 TShock 数据库。", status)
		return nil, status, false
	}

	return db, status, true
}
