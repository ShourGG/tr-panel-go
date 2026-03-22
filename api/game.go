package api

import (
	"archive/tar"
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/storage"
	"terraria-panel/utils"
	"time"
)

type TShockReleaseInfo struct {
	Version         string
	TerrariaVersion string
	DownloadURL     string
	PublishedAt     string
	Prerelease      bool
	RuntimeMajor    string
	DisplayName     string
}

type LinuxDistroInfo struct {
	ID              string
	VersionID       string
	VersionCodename string
}

type GamePackageSpec struct {
	GameType      string
	DownloadURL   string
	TargetDir     string
	LatestVersion string
	UpdateChannel string
	RuntimeMajor  string
}

type DotNetRuntimeHealth struct {
	Healthy          bool   `json:"healthy"`
	NeedsRepair      bool   `json:"needsRepair"`
	IssueType        string `json:"issueType"`
	Message          string `json:"message"`
	RuntimeVersion   string `json:"runtimeVersion"`
	DetectedDotnet   string `json:"detectedDotnet"`
	DetectedRoot     string `json:"detectedRoot"`
	Runtimes         string `json:"runtimes"`
	RepairSuggestion string `json:"repairSuggestion"`
}

type ActiveGameTask struct {
	GameType  string `json:"gameType"`
	Action    string `json:"action"`
	Progress  int    `json:"progress"`
	Message   string `json:"message"`
	StartedAt string `json:"startedAt"`
	UpdatedAt string `json:"updatedAt"`
}

var numericVersionRegexp = regexp.MustCompile(`\d+`)

var activeGameTasks = struct {
	sync.RWMutex
	tasks map[string]ActiveGameTask
}{
	tasks: make(map[string]ActiveGameTask),
}

func setActiveGameTask(gameType, action, message string, progress int) {
	now := time.Now().Format(time.RFC3339)

	activeGameTasks.Lock()
	defer activeGameTasks.Unlock()

	task, exists := activeGameTasks.tasks[gameType]
	if !exists {
		task = ActiveGameTask{
			GameType:  gameType,
			Action:    action,
			StartedAt: now,
		}
	}

	task.Action = action
	task.Message = message
	task.Progress = progress
	task.UpdatedAt = now
	if task.StartedAt == "" {
		task.StartedAt = now
	}

	activeGameTasks.tasks[gameType] = task
}

func clearActiveGameTask(gameType string) {
	activeGameTasks.Lock()
	defer activeGameTasks.Unlock()
	delete(activeGameTasks.tasks, gameType)
}

func getActiveGameTasksSnapshot() map[string]ActiveGameTask {
	activeGameTasks.RLock()
	defer activeGameTasks.RUnlock()

	snapshot := make(map[string]ActiveGameTask, len(activeGameTasks.tasks))
	for key, task := range activeGameTasks.tasks {
		snapshot[key] = task
	}
	return snapshot
}

func getBlockingGameTaskMessage(requestedGameType string) string {
	activeGameTasks.RLock()
	defer activeGameTasks.RUnlock()

	for _, task := range activeGameTasks.tasks {
		if task.GameType == requestedGameType {
			actionText := mapGameTaskActionText(task.Action)
			return fmt.Sprintf("%s 当前正在%s，请等待任务完成后再操作", requestedGameType, actionText)
		}
	}

	for _, task := range activeGameTasks.tasks {
		actionText := mapGameTaskActionText(task.Action)
		return fmt.Sprintf("%s 正在%s中，请等待当前任务完成后再继续", task.GameType, actionText)
	}

	return ""
}

func mapGameTaskActionText(action string) string {
	switch action {
	case "update":
		return "更新"
	case "repair":
		return "修复"
	default:
		return "安装"
	}
}

func CheckGameInstalled(c *gin.Context) {
	vanillaServer := filepath.Join(config.ServersDir, "vanilla", "TerrariaServer.exe")
	vanillaServerLinux := filepath.Join(config.ServersDir, "vanilla", "TerrariaServer")
	vanillaInstalled := false
	if _, err := os.Stat(vanillaServer); err == nil {
		vanillaInstalled = true
	} else if _, err := os.Stat(vanillaServerLinux); err == nil {
		vanillaInstalled = true
	}
	tmodDll := filepath.Join(config.ServersDir, "tModLoader", "tModLoader.dll")
	tmodServerExe := filepath.Join(config.ServersDir, "tModLoader", "tModLoaderServer.exe")
	tmodInstalled := false
	if _, err := os.Stat(tmodDll); err == nil {
		tmodInstalled = true
	} else if _, err := os.Stat(tmodServerExe); err == nil {
		tmodInstalled = true
	}
	tshockServer := filepath.Join(config.ServersDir, "tshock", "TerrariaServer.exe")
	tshockServerLinux := filepath.Join(config.ServersDir, "tshock", "TShockServer")
	tshockInstalled := false
	if _, err := os.Stat(tshockServer); err == nil {
		tshockInstalled = true
	} else if _, err := os.Stat(tshockServerLinux); err == nil {
		tshockInstalled = true
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"vanilla":      vanillaInstalled,
			"tmodloader":   tmodInstalled,
			"tshock":       tshockInstalled,
			"anyInstalled": vanillaInstalled || tmodInstalled || tshockInstalled,
		},
	})
}
func GetGameInstallInfo(c *gin.Context) {
	osType := runtime.GOOS
	vanillaUrl := "https://terraria.org/api/download/pc-dedicated-server/terraria-server-1449.zip"
	vanillaVersion := "1.4.4.9"
	tmodUrl, tmodVersion := getLatestTModLoaderRelease()
	tshock5Info := getLatestTShock5ReleaseInfo()
	tshock6Info := getLatestTShock6ReleaseInfo()
	activeTasks := getActiveGameTasksSnapshot()
	tshock5RuntimeHealth := assessDotNetRuntimeHealth("6.0")
	tshock6RuntimeHealth := assessDotNetRuntimeHealth("9.0")
	vanillaInstalled := checkVanillaInstalled()
	tmodInstalled := checkTModLoaderInstalled()
	tshock5Installed := checkTShockInstalled() && !isTShock6()
	tshock6Installed := checkTShockInstalled() && isTShock6()
	vanillaInstalledVersion, _ := getInstalledGameVersion("vanilla")
	tmodInstalledVersion, _ := getInstalledGameVersion("tmodloader")
	tshock5InstalledVersion, _ := getInstalledGameVersion("tshock5")
	tshock6InstalledVersion, _ := getInstalledGameVersion("tshock6")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"os":          osType,
			"activeTasks": activeTasks,
			"vanilla": gin.H{
				"name":             "Terraria 原版服务器",
				"version":          vanillaVersion,
				"path":             filepath.Join(config.ServersDir, "vanilla"),
				"downloadUrl":      vanillaUrl,
				"size":             "约 40 MB",
				"installed":        vanillaInstalled,
				"installedVersion": vanillaInstalledVersion,
				"latestVersion":    vanillaVersion,
				"updateAvailable":  vanillaInstalled && isGameUpdateAvailable(vanillaInstalledVersion, vanillaVersion),
				"updateSupported":  true,
				"updateChannel":    "vanilla",
			},
			"tmodloader": gin.H{
				"name":             "tModLoader 服务器",
				"version":          tmodVersion,
				"path":             filepath.Join(config.ServersDir, "tModLoader"),
				"downloadUrl":      tmodUrl,
				"size":             "约 50 MB",
				"installed":        tmodInstalled,
				"installedVersion": tmodInstalledVersion,
				"latestVersion":    tmodVersion,
				"updateAvailable":  tmodInstalled && isGameUpdateAvailable(tmodInstalledVersion, tmodVersion),
				"updateSupported":  true,
				"updateChannel":    "tmodloader",
			},
			"tshock5": gin.H{
				"name":             "TShock 5 稳定版",
				"version":          tshock5Info.Version,
				"path":             filepath.Join(config.ServersDir, "tshock"),
				"downloadUrl":      tshock5Info.DownloadURL,
				"size":             "约 24 MB",
				"installed":        tshock5Installed,
				"installedVersion": tshock5InstalledVersion,
				"latestVersion":    tshock5Info.Version,
				"updateAvailable":  tshock5Installed && isGameUpdateAvailable(tshock5InstalledVersion, tshock5Info.Version),
				"updateSupported":  true,
				"updateChannel":    "tshock5",
				"requiresNet":      tshock5Info.RuntimeMajor,
				"releaseVersion":   tshock5Info.Version,
				"terrariaVersion":  tshock5Info.TerrariaVersion,
				"publishedAt":      tshock5Info.PublishedAt,
				"prerelease":       tshock5Info.Prerelease,
				"runtimeHealth":    tshock5RuntimeHealth,
			},
			"tshock6": gin.H{
				"name":             "TShock 6 新版稳定版",
				"version":          tshock6Info.Version,
				"path":             filepath.Join(config.ServersDir, "tshock"),
				"downloadUrl":      tshock6Info.DownloadURL,
				"size":             "约 25 MB",
				"installed":        tshock6Installed,
				"installedVersion": tshock6InstalledVersion,
				"latestVersion":    tshock6Info.Version,
				"updateAvailable":  tshock6Installed && isGameUpdateAvailable(tshock6InstalledVersion, tshock6Info.Version),
				"updateSupported":  true,
				"updateChannel":    "tshock6",
				"requiresNet":      tshock6Info.RuntimeMajor,
				"releaseVersion":   tshock6Info.Version,
				"terrariaVersion":  tshock6Info.TerrariaVersion,
				"publishedAt":      tshock6Info.PublishedAt,
				"prerelease":       tshock6Info.Prerelease,
				"runtimeHealth":    tshock6RuntimeHealth,
			},
		},
	})
}
func InstallGame(c *gin.Context) {
	var req struct {
		GameType string `json:"gameType" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	validTypes := []string{"vanilla", "tmodloader", "tshock", "tshock5", "tshock6"}
	valid := false
	for _, t := range validTypes {
		if req.GameType == t {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "不支持的游戏类型",
		})
		return
	}
	if req.GameType == "tshock" {
		req.GameType = "tshock5"
	}
	if blockingMessage := getBlockingGameTaskMessage(req.GameType); blockingMessage != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": blockingMessage,
		})
		return
	}
	if req.GameType == "vanilla" && checkVanillaInstalled() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Terraria 原版服务器已安装",
		})
		return
	}
	if req.GameType == "tmodloader" && checkTModLoaderInstalled() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "tModLoader 服务器已安装",
		})
		return
	}
	if (req.GameType == "tshock5" || req.GameType == "tshock6") && checkTShockInstalled() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "TShock 服务器已安装，请先卸载当前版本",
		})
		return
	}
	if req.GameType == "tshock5" || req.GameType == "tshock6" {
		targetRuntime := "6.0"
		if req.GameType == "tshock6" {
			targetRuntime = "9.0"
		}
		health := assessDotNetRuntimeHealth(targetRuntime)
		if health.NeedsRepair {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": health.Message,
				"data": gin.H{
					"runtimeHealth": health,
				},
			})
			return
		}
	}
	fmt.Printf("\n========================================\n")
	fmt.Printf("[安装开始] 游戏类型: %s\n", req.GameType)
	fmt.Printf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("========================================\n\n")
	setActiveGameTask(req.GameType, "install", "准备开始安装...", 0)
	go installGameServer(req.GameType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("开始安装 %s 服务器，请稍等...", req.GameType),
	})
}

func RepairGameRuntime(c *gin.Context) {
	var req struct {
		GameType        string `json:"gameType" binding:"required"`
		ContinueInstall bool   `json:"continueInstall"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	if req.GameType != "tshock5" && req.GameType != "tshock6" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "仅支持为 TShock 5 或 TShock 6 修复 .NET 环境",
		})
		return
	}

	if blockingMessage := getBlockingGameTaskMessage(req.GameType); blockingMessage != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": blockingMessage,
		})
		return
	}

	setActiveGameTask(req.GameType, "repair", "准备修复 .NET 运行时环境...", 0)
	go repairGameRuntime(req.GameType, req.ContinueInstall)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已开始修复 .NET 环境，请等待...",
	})
}

func UpdateGame(c *gin.Context) {
	var req struct {
		GameType     string `json:"gameType" binding:"required"`
		CreateBackup bool   `json:"createBackup"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	validTypes := map[string]bool{
		"vanilla":    true,
		"tmodloader": true,
		"tshock5":    true,
		"tshock6":    true,
	}
	if !validTypes[req.GameType] {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "不支持的更新类型",
		})
		return
	}

	if !isGameInstalledForType(req.GameType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "该服务端尚未安装，无法更新",
		})
		return
	}

	running, message := isGameTypeRunning(req.GameType)
	if running {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": message,
		})
		return
	}
	if blockingMessage := getBlockingGameTaskMessage(req.GameType); blockingMessage != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": blockingMessage,
		})
		return
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("[更新开始] 游戏类型: %s\n", req.GameType)
	fmt.Printf("[更新开始] 备份: %v\n", req.CreateBackup)
	fmt.Printf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("========================================\n\n")

	setActiveGameTask(req.GameType, "update", "准备开始更新...", 0)
	go updateGameServer(req.GameType, req.CreateBackup)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("开始更新 %s 服务器，请稍等...", req.GameType),
	})
}
func GetInstallProgress(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"status":   "installing",
			"progress": 50,
			"message":  "正在下载...",
		},
	})
}
func checkVanillaInstalled() bool {
	vanillaDir := filepath.Join(config.ServersDir, "vanilla")
	if info, err := os.Stat(vanillaDir); err == nil && info.IsDir() {
		files, err := os.ReadDir(vanillaDir)
		if err == nil && len(files) > 0 {
			fmt.Printf("[检测] Vanilla已安装，目录包含 %d 个文件\n", len(files))
			return true
		}
	}
	fmt.Printf("[检测] Vanilla未安装\n")
	return false
}
func checkTModLoaderInstalled() bool {
	tmodDir := filepath.Join(config.ServersDir, "tModLoader")
	if info, err := os.Stat(tmodDir); err == nil && info.IsDir() {
		files, err := os.ReadDir(tmodDir)
		if err == nil && len(files) > 0 {
			fmt.Printf("[检测] tModLoader已安装，目录包含 %d 个文件\n", len(files))
			return true
		}
	}
	fmt.Printf("[检测] tModLoader未安装\n")
	return false
}
func checkTShockInstalled() bool {
	tshockDir := filepath.Join(config.ServersDir, "tshock")
	if info, err := os.Stat(tshockDir); err != nil || !info.IsDir() {
		fmt.Printf("[检测] TShock目录不存在\n")
		return false
	}
	coreFiles := []string{
		"TShock.Server",
		"TShock.Server.dll",
	}
	for _, file := range coreFiles {
		filePath := filepath.Join(tshockDir, file)
		if _, err := os.Stat(filePath); err == nil {
			fmt.Printf("[检测] TShock已安装，找到核心文件: %s\n", file)
			return true
		}
	}
	fmt.Printf("[检测] TShock未安装（核心程序文件不存在）\n")
	return false
}
func isTShock6() bool {
	if version := readTShockVersionMarker(); version != "" {
		isV6 := version == "6" || strings.HasPrefix(version, "6.")
		fmt.Printf("[检测] TShock 版本标记: %s (是否为6: %v)\n", version, isV6)
		return isV6
	}
	fmt.Printf("[检测] 未找到版本标记文件，默认为 TShock 5\n")
	return false
}
func sendInstallProgress(gameType string, message string, progress int) {
	setActiveGameTask(gameType, "install", message, progress)
	status := map[string]interface{}{
		"type":     "install_progress",
		"gameType": gameType,
		"message":  message,
		"progress": progress,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	fmt.Printf("[%s] %s (%d%%)\n", gameType, message, progress)
}
func sendInstallError(gameType string, message string) {
	status := map[string]interface{}{
		"type":     "install_error",
		"gameType": gameType,
		"message":  message,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	clearActiveGameTask(gameType)
	fmt.Printf("[%s] 错误: %s\n", gameType, message)
}

func sendUpdateProgress(gameType string, message string, progress int) {
	setActiveGameTask(gameType, "update", message, progress)
	status := map[string]interface{}{
		"type":     "update_progress",
		"gameType": gameType,
		"message":  message,
		"progress": progress,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	fmt.Printf("[更新:%s] %s (%d%%)\n", gameType, message, progress)
}

func sendUpdateError(gameType string, message string) {
	status := map[string]interface{}{
		"type":     "update_error",
		"gameType": gameType,
		"message":  message,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	clearActiveGameTask(gameType)
	fmt.Printf("[更新:%s] 错误: %s\n", gameType, message)
}

func sendUpdateComplete(gameType string, message string) {
	status := map[string]interface{}{
		"type":     "update_complete",
		"gameType": gameType,
		"message":  message,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	clearActiveGameTask(gameType)
	fmt.Printf("[更新:%s] 完成: %s\n", gameType, message)
}

func sendRepairProgress(gameType string, message string, progress int) {
	setActiveGameTask(gameType, "repair", message, progress)
	status := map[string]interface{}{
		"type":     "runtime_repair_progress",
		"gameType": gameType,
		"message":  message,
		"progress": progress,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	fmt.Printf("[修复:%s] %s (%d%%)\n", gameType, message, progress)
}

func sendRepairError(gameType string, message string) {
	status := map[string]interface{}{
		"type":     "runtime_repair_error",
		"gameType": gameType,
		"message":  message,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	clearActiveGameTask(gameType)
	fmt.Printf("[修复:%s] 错误: %s\n", gameType, message)
}

func sendRepairComplete(gameType string, message string, continueInstall bool) {
	status := map[string]interface{}{
		"type":            "runtime_repair_complete",
		"gameType":        gameType,
		"message":         message,
		"continueInstall": continueInstall,
	}
	jsonData, err := json.Marshal(status)
	if err == nil {
		BroadcastMessage(jsonData)
	}
	if !continueInstall {
		clearActiveGameTask(gameType)
	}
	fmt.Printf("[修复:%s] 完成: %s\n", gameType, message)
}
func installGameServer(gameType string) {
	var downloadUrl string
	var targetDir string
	var resolvedVersion string
	sendProgress := func(message string, progress int) {
		sendInstallProgress(gameType, message, progress)
	}
	sendError := func(message string) {
		sendInstallError(gameType, message)
	}
	sendProgress("开始准备安装", 0)
	if gameType == "vanilla" {
		downloadUrl = "https://terraria.org/api/download/pc-dedicated-server/terraria-server-1449.zip"
		resolvedVersion = "1.4.4.9"
		targetDir = filepath.Join(config.ServersDir, "vanilla")
	} else if gameType == "tmodloader" {
		url, version := getLatestTModLoaderRelease()
		downloadUrl = url
		resolvedVersion = version
		fmt.Printf("准备安装 tModLoader %s\n", version)
		targetDir = filepath.Join(config.ServersDir, "tModLoader")
	} else if gameType == "tshock5" {
		release := getLatestTShock5ReleaseInfo()
		downloadUrl = release.DownloadURL
		resolvedVersion = release.Version
		fmt.Printf("准备安装 TShock %s (稳定版 - .NET %s)\n", release.Version, release.RuntimeMajor)
		targetDir = filepath.Join(config.ServersDir, "tshock")
	} else if gameType == "tshock6" {
		release := getLatestTShock6ReleaseInfo()
		downloadUrl = release.DownloadURL
		resolvedVersion = release.Version
		fmt.Printf("准备安装 TShock %s (新版稳定版 - .NET %s)\n", release.Version, release.RuntimeMajor)
		targetDir = filepath.Join(config.ServersDir, "tshock")
	} else {
		fmt.Printf("不支持的游戏类型: %s\n", gameType)
		return
	}
	sendProgress("创建目录", 5)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		sendError(fmt.Sprintf("创建目录失败: %v", err))
		return
	}
	tempFile := filepath.Join(targetDir, "download.tmp")
	sendProgress("开始下载游戏文件", 10)
	fmt.Printf("[下载] URL: %s\n", downloadUrl)
	fmt.Printf("[下载] 临时文件: %s\n", tempFile)
	cfg := config.Load()
	downloadOpts := utils.GetDownloadConfig(cfg, downloadUrl, tempFile, func(percent int) {
		actualProgress := 10 + (percent * 50 / 100)
		msg := fmt.Sprintf("正在下载游戏文件... %d%%", percent)
		sendProgress(msg, actualProgress)
	})
	err := utils.DownloadWithRetry(downloadOpts)
	if err != nil {
		sendError(fmt.Sprintf("下载失败: %v", err))
		return
	}
	sendProgress("下载完成，检测文件格式", 60)
	file, err := os.Open(tempFile)
	if err != nil {
		sendError(fmt.Sprintf("打开文件失败: %v", err))
		return
	}
	header := make([]byte, 512)
	n, _ := file.Read(header)
	file.Close()
	isZip := false
	isTar := false
	if n >= 4 {
		isZip = header[0] == 0x50 && header[1] == 0x4B && header[2] == 0x03 && header[3] == 0x04
	}
	if n >= 262 {
		tarMagic := string(header[257:262])
		isTar = tarMagic == "ustar"
		fmt.Printf("[检测] TAR标记位置257-262: %q (期望: \"ustar\")\n", tarMagic)
	}
	if !isZip && !isTar {
		fmt.Println("[检测] 无法通过文件头识别，检查目录中的文件...")
		files, _ := os.ReadDir(targetDir)
		for _, f := range files {
			name := strings.ToLower(f.Name())
			fmt.Printf("[检测] 找到文件: %s\n", f.Name())
			if strings.HasSuffix(name, ".tar") {
				isTar = true
				fmt.Println("[检测] 根据文件名判断为TAR")
				break
			}
			if strings.HasSuffix(name, ".zip") {
				isZip = true
				fmt.Println("[检测] 根据文件名判断为ZIP")
				break
			}
		}
	}
	fmt.Printf("[检测] 读取字节数: %d\n", n)
	fmt.Printf("[检测] 文件头(hex): % X\n", header[:16])
	fmt.Printf("[检测] 文件头(ascii): %q\n", string(header[:16]))
	fmt.Printf("[检测] 最终判断 - ZIP: %v, TAR: %v\n", isZip, isTar)
	var downloadFile string
	if isZip {
		downloadFile = filepath.Join(targetDir, "download.zip")
		os.Rename(tempFile, downloadFile)
		fmt.Println("[格式] 检测为 ZIP 文件")
	} else if isTar {
		downloadFile = filepath.Join(targetDir, "download.tar")
		os.Rename(tempFile, downloadFile)
		fmt.Println("[格式] 检测为 TAR 文件")
	} else {
		downloadFile = filepath.Join(targetDir, "download.zip")
		os.Rename(tempFile, downloadFile)
		fmt.Println("[格式] 未知格式，尝试作为 ZIP 处理")
	}
	sendProgress("开始解压文件", 65)
	if isZip {
		fmt.Println("[解压] 使用 ZIP 解压")
		if err := unzipFile(downloadFile, targetDir); err != nil {
			sendError(fmt.Sprintf("ZIP解压失败: %v", err))
			return
		}
	} else if isTar {
		fmt.Println("[解压] 使用 TAR 解压")
		if err := extractTarFile(downloadFile, targetDir); err != nil {
			sendError(fmt.Sprintf("TAR解压失败: %v", err))
			return
		}
	} else {
		if err := unzipFile(downloadFile, targetDir); err != nil {
			fmt.Println("[解压] ZIP失败，尝试 TAR")
			if err2 := extractTarFile(downloadFile, targetDir); err2 != nil {
				sendError(fmt.Sprintf("解压失败，ZIP错误: %v, TAR错误: %v", err, err2))
				return
			}
		}
	}
	sendProgress("解压完成", 80)
	fmt.Println("[验证] 检查解压后的文件...")
	extractedFiles, err := os.ReadDir(targetDir)
	if err == nil {
		fmt.Printf("[验证] 目录中有 %d 个文件/目录\n", len(extractedFiles))
		var nestedArchive string
		for _, f := range extractedFiles {
			name := strings.ToLower(f.Name())
			fmt.Printf("[验证]   - %s (是目录: %v)\n", f.Name(), f.IsDir())
			if strings.HasPrefix(name, "download.") {
				continue
			}
			if strings.HasSuffix(name, ".tar") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
				nestedArchive = filepath.Join(targetDir, f.Name())
				fmt.Printf("[验证] 发现嵌套压缩包: %s\n", f.Name())
				break
			}
		}
		if nestedArchive != "" {
			sendProgress("发现嵌套压缩包，继续解压", 82)
			fmt.Printf("[嵌套解压] 解压文件: %s\n", nestedArchive)
			if strings.HasSuffix(strings.ToLower(nestedArchive), ".tar") {
				if err := extractTarFile(nestedArchive, targetDir); err != nil {
					sendError(fmt.Sprintf("嵌套TAR解压失败: %v", err))
					return
				}
				fmt.Println("[嵌套解压] TAR解压完成")
			} else if strings.HasSuffix(strings.ToLower(nestedArchive), ".zip") {
				if err := unzipFile(nestedArchive, targetDir); err != nil {
					sendError(fmt.Sprintf("嵌套ZIP解压失败: %v", err))
					return
				}
				fmt.Println("[嵌套解压] ZIP解压完成")
			}
			os.Remove(nestedArchive)
			fmt.Println("[验证] 最终检查解压结果...")
			finalFiles, _ := os.ReadDir(targetDir)
			fmt.Printf("[验证] 最终有 %d 个文件/目录\n", len(finalFiles))
			for i, f := range finalFiles {
				if i < 15 && !strings.HasPrefix(strings.ToLower(f.Name()), "download.") {
					fmt.Printf("[验证]   - %s\n", f.Name())
				}
			}
		}
	}
	os.Remove(downloadFile)
	if gameType == "vanilla" {
		sendProgress("整理文件结构", 85)
		linuxDir := filepath.Join(targetDir, "1449", "Linux")
		windowsDir := filepath.Join(targetDir, "1449", "Windows")
		sourceDir := ""
		if runtime.GOOS == "linux" {
			sourceDir = linuxDir
		} else {
			sourceDir = windowsDir
		}
		if _, err := os.Stat(sourceDir); err == nil {
			moveFiles(sourceDir, targetDir)
			os.RemoveAll(filepath.Join(targetDir, "1449"))
		}
		if runtime.GOOS == "linux" {
			terrariaServer := filepath.Join(targetDir, "TerrariaServer")
			os.Chmod(terrariaServer, 0755)
		}
		if err := writeInstalledVersionMarker(gameType, targetDir, resolvedVersion); err != nil {
			sendError(fmt.Sprintf("写入版本标记失败: %v", err))
			return
		}
	} else if gameType == "tmodloader" {
		sendProgress("配置tModLoader", 90)
		if runtime.GOOS == "linux" {
			startScript := filepath.Join(targetDir, "start-tModLoaderServer.sh")
			os.Chmod(startScript, 0755)
			sendProgress("检查.NET运行时", 95)
			installDotNetIfNeeded(gameType)
		}
		if err := writeInstalledVersionMarker(gameType, targetDir, resolvedVersion); err != nil {
			sendError(fmt.Sprintf("写入版本标记失败: %v", err))
			return
		}
	} else if gameType == "tshock5" || gameType == "tshock6" {
		sendProgress("配置 TShock", 90)
		if runtime.GOOS == "linux" {
			tshockServer := filepath.Join(targetDir, "TShock.Server")
			if _, err := os.Stat(tshockServer); err == nil {
				os.Chmod(tshockServer, 0755)
			}
			tshockDll := filepath.Join(targetDir, "TShock.Server.dll")
			if _, err := os.Stat(tshockDll); err == nil {
				os.Chmod(tshockDll, 0755)
			}
			if gameType == "tshock6" {
				sendProgress("检查并安装 .NET 9.0 运行时", 91)
				if err := installDotNet9(gameType); err != nil {
					fmt.Printf("[.NET安装] 自动安装 .NET 9.0 失败: %v\n", err)
					sendError(fmt.Sprintf(".NET 9.0 自动安装失败: %v", err))
					return
				}
				if resolvedVersion == "" {
					resolvedVersion = "6"
				}
				if err := writeInstalledVersionMarker(gameType, targetDir, resolvedVersion); err != nil {
					sendError(fmt.Sprintf("写入版本标记失败: %v", err))
					return
				}
				fmt.Printf("[版本标记] 已创建 TShock 6 版本标记文件\n")
			} else {
				sendProgress("检查并安装 .NET 6.0 运行时", 91)
				if err := installDotNet6(gameType); err != nil {
					fmt.Printf("[.NET安装] 自动安装 .NET 6.0 失败: %v\n", err)
					sendError(fmt.Sprintf(".NET 6.0 自动安装失败: %v", err))
					return
				}
				if resolvedVersion == "" {
					resolvedVersion = "5"
				}
				if err := writeInstalledVersionMarker(gameType, targetDir, resolvedVersion); err != nil {
					sendError(fmt.Sprintf("写入版本标记失败: %v", err))
					return
				}
				fmt.Printf("[版本标记] 已创建 TShock 5 版本标记文件\n")
			}
		}
	}
	sendProgress("安装完成！", 100)
	fmt.Printf("%s 安装完成！\n", gameType)
	completeMsg := map[string]interface{}{
		"type":     "install_complete",
		"gameType": gameType,
		"message":  "安装成功完成",
	}
	jsonData, err := json.Marshal(completeMsg)
	if err != nil {
		fmt.Printf("[WebSocket] JSON序列化失败: %v\n", err)
	} else {
		fmt.Printf("[WebSocket] 发送完成消息: %s\n", string(jsonData))
		BroadcastMessage(jsonData)
		fmt.Println("[WebSocket] 完成消息已广播")
	}
	clearActiveGameTask(gameType)
}

func updateGameServer(gameType string, createBackup bool) {
	sendProgress := func(message string, progress int) {
		sendUpdateProgress(gameType, message, progress)
	}
	sendError := func(message string) {
		sendUpdateError(gameType, message)
	}

	spec, err := resolveGamePackageSpec(gameType)
	if err != nil {
		sendError(err.Error())
		return
	}

	targetDir := spec.TargetDir
	installedVersion, _ := getInstalledGameVersion(gameType)
	fromVersion := installedVersion
	if fromVersion == "" {
		fromVersion = "unknown"
	}

	sendProgress("开始准备更新", 0)
	if createBackup {
		sendProgress("正在创建更新前备份", 5)
		if _, err := createGameUpdateBackup(gameType, targetDir, fromVersion); err != nil {
			sendError(fmt.Sprintf("创建更新备份失败: %v", err))
			return
		}
	}

	tempRoot := filepath.Join(config.DataDir, "tmp", "game-updates", fmt.Sprintf("%s-%d", gameType, time.Now().UnixNano()))
	extractDir := filepath.Join(tempRoot, "payload")
	defer os.RemoveAll(tempRoot)

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		sendError(fmt.Sprintf("创建临时目录失败: %v", err))
		return
	}

	if err := downloadAndExtractGamePackage(gameType, spec.DownloadURL, extractDir, sendProgress); err != nil {
		sendError(err.Error())
		return
	}

	if err := finalizePreparedGameFiles(gameType, extractDir, spec.LatestVersion, sendProgress); err != nil {
		sendError(err.Error())
		return
	}

	sendProgress("正在替换旧版本程序文件", 94)
	switch gameType {
	case "tshock5", "tshock6":
		if err := uninstallTShockKeepData(targetDir); err != nil {
			sendError(fmt.Sprintf("准备插件服更新目录失败: %v", err))
			return
		}
	default:
		if err := os.RemoveAll(targetDir); err != nil {
			sendError(fmt.Sprintf("清理旧版本目录失败: %v", err))
			return
		}
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			sendError(fmt.Sprintf("重建目标目录失败: %v", err))
			return
		}
	}

	if err := copyDirContents(extractDir, targetDir); err != nil {
		sendError(fmt.Sprintf("写入新版本文件失败: %v", err))
		return
	}

	if err := writeInstalledVersionMarker(gameType, targetDir, spec.LatestVersion); err != nil {
		sendError(fmt.Sprintf("写入更新版本标记失败: %v", err))
		return
	}

	sendProgress("更新完成！", 100)
	sendUpdateComplete(gameType, "更新成功完成")
}

func repairGameRuntime(gameType string, continueInstall bool) {
	sendProgress := func(message string, progress int) {
		sendRepairProgress(gameType, message, progress)
	}
	sendError := func(message string) {
		sendRepairError(gameType, message)
	}

	targetRuntime := "6.0"
	if gameType == "tshock6" {
		targetRuntime = "9.0"
	}

	health := assessDotNetRuntimeHealth(targetRuntime)
	if !health.NeedsRepair {
		sendProgress(fmt.Sprintf(".NET %s 环境正常，无需修复", targetRuntime), 100)
		sendRepairComplete(gameType, "当前 .NET 环境正常，无需修复", continueInstall)
		if continueInstall {
			installGameServer(gameType)
		}
		return
	}

	info, repoName, err := detectSupportedUbuntuRepo()
	if err != nil {
		sendError(fmt.Sprintf("当前系统暂不支持自动修复：%v", err))
		return
	}

	systemLabel := fmt.Sprintf("%s %s", strings.ToLower(info.ID), info.VersionID)
	sendProgress(fmt.Sprintf("检测到 %s，开始修复 .NET 环境", systemLabel), 5)

	repoPackageName := "packages-microsoft-prod"
	sendProgress("移除冲突的 Microsoft .NET 源配置", 10)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 10, "移除 packages-microsoft-prod", "dpkg", "-r", repoPackageName); stepErr != nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" && !strings.Contains(trimmed, "is not installed") && !strings.Contains(trimmed, "没有安装") {
			sendProgress("packages-microsoft-prod 未完全移除，继续清理源文件", 12)
		}
	}

	microsoftSourceFiles := []string{
		"/etc/apt/sources.list.d/microsoft-prod.list",
		"/etc/apt/sources.list.d/microsoft-prod.sources",
	}
	for _, sourceFile := range microsoftSourceFiles {
		_ = os.Remove(sourceFile)
	}

	sendProgress("清理旧的 .NET 包", 18)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 18, "卸载旧 .NET 运行时", "apt-get", "remove", "-y", "dotnet*", "aspnet*", "netstandard*"); stepErr != nil {
		sendError(fmt.Sprintf("卸载旧 .NET 包失败 | output=%s", strings.TrimSpace(output)))
		return
	}

	sendProgress("执行自动清理", 24)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 24, "清理无用依赖", "apt-get", "autoremove", "-y"); stepErr != nil {
		fmt.Printf("[修复:%s] autoremove 警告: %v\n%s\n", gameType, stepErr, output)
	}

	sendProgress("安装 software-properties-common", 32)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 32, "安装 software-properties-common", "apt-get", "install", "-y", "software-properties-common"); stepErr != nil {
		sendError(fmt.Sprintf("安装 software-properties-common 失败 | output=%s", strings.TrimSpace(output)))
		return
	}

	sendProgress("添加 Ubuntu .NET backports 源", 40)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 40, "添加 Ubuntu .NET backports 源", "add-apt-repository", "-y", repoName); stepErr != nil {
		sendError(fmt.Sprintf("添加 %s 失败 | output=%s", repoName, strings.TrimSpace(output)))
		return
	}

	sendProgress("写入 .NET 源保护规则", 46)
	if err := writeDotNetBackportsPreference(); err != nil {
		sendError(fmt.Sprintf("写入 .NET 源保护规则失败: %v", err))
		return
	}

	sendProgress("更新软件包列表", 55)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 55, "更新软件包列表", "apt-get", "update", "-qq"); stepErr != nil {
		sendError(fmt.Sprintf("apt-get update 失败 | output=%s", strings.TrimSpace(output)))
		return
	}

	sendProgress("重新安装 .NET 6/8/9 运行时", 72)
	if output, stepErr := runPackageManagerCommandWithRetryForAction(gameType, "repair", 72, "安装 .NET 6/8/9 运行时", "apt-get", "install", "-y", "dotnet-runtime-6.0", "dotnet-runtime-8.0", "dotnet-runtime-9.0"); stepErr != nil {
		sendError(fmt.Sprintf("安装 .NET 运行时失败 | output=%s", strings.TrimSpace(output)))
		return
	}

	sendProgress("验证修复结果", 90)
	health = assessDotNetRuntimeHealth(targetRuntime)
	if health.NeedsRepair || !health.Healthy {
		sendError(fmt.Sprintf("修复后仍未通过验证：%s", health.Message))
		return
	}

	sendProgress(fmt.Sprintf(".NET %s 环境修复完成", targetRuntime), 100)
	sendRepairComplete(gameType, fmt.Sprintf(".NET %s 环境修复成功", targetRuntime), continueInstall)
	if continueInstall {
		installGameServer(gameType)
	}
}
func downloadFileFromURL(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
func downloadFileWithProgress(filepath string, url string, onProgress func(int)) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	totalSize := resp.ContentLength
	if totalSize > 0 {
		fmt.Printf("文件大小: %.2f MB\n", float64(totalSize)/1024/1024)
	}
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	var downloaded int64
	buf := make([]byte, 128*1024)
	lastPercent := -1
	lastReportTime := time.Now()
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)
			if totalSize > 0 {
				percent := int(downloaded * 100 / totalSize)
				if percent != lastPercent || time.Since(lastReportTime) > time.Second {
					lastPercent = percent
					lastReportTime = time.Now()
					if onProgress != nil {
						onProgress(percent)
					}
					fmt.Printf("下载进度: %d%% (%.2f/%.2f MB)\n",
						percent,
						float64(downloaded)/1024/1024,
						float64(totalSize)/1024/1024)
				}
			} else {
				if downloaded%(1024*1024) == 0 {
					if onProgress != nil {
						virtualPercent := int(downloaded / (1024 * 1024))
						if virtualPercent > 99 {
							virtualPercent = 99
						}
						onProgress(virtualPercent)
					}
					fmt.Printf("已下载: %.2f MB\n", float64(downloaded)/1024/1024)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if onProgress != nil {
		onProgress(100)
	}
	fmt.Printf("下载完成: %.2f MB\n", float64(downloaded)/1024/1024)
	return nil
}
func unzipFile(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("非法的文件路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func moveFiles(src, dst string) error {
	files, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, file := range files {
		srcPath := filepath.Join(src, file.Name())
		dstPath := filepath.Join(dst, file.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
			os.Remove(srcPath)
		}
	}
	return nil
}
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}
	sourceInfo, _ := os.Stat(src)
	return os.Chmod(dst, sourceInfo.Mode())
}
func checkDotNetVersion() (bool, string, error) {
	dotnetPath, err := exec.LookPath("dotnet")
	if err != nil {
		return false, "", fmt.Errorf("dotnet command not found")
	}
	cmd := exec.Command(dotnetPath, "--list-runtimes")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", fmt.Errorf("failed to execute dotnet --list-runtimes: %v", err)
	}
	outputStr := string(output)
	fmt.Printf("[.NET检测] 已安装的运行时:\n%s\n", outputStr)
	hasNet8 := strings.Contains(outputStr, "Microsoft.NETCore.App 8.0")
	return hasNet8, outputStr, nil
}
func installDotNet8(gameType string) error {
	fmt.Println("\n========================================")
	fmt.Println("[.NET 8.0] 开始检测和安装流程")
	fmt.Println("========================================")
	sendInstallProgress(gameType, "检测 .NET 运行时...", 91)
	hasNet8, installedVersions, err := checkDotNetVersion()
	if err != nil {
		fmt.Printf("[.NET检测] 错误: %v\n", err)
		sendInstallProgress(gameType, "未检测到 dotnet 命令，开始安装...", 92)
	} else if hasNet8 {
		fmt.Println("[.NET检测] ✓ 已安装 .NET 8.0 运行时，跳过安装")
		sendInstallProgress(gameType, "✓ 已安装 .NET 8.0，跳过安装", 95)
		return nil
	} else {
		fmt.Printf("[.NET检测] 未检测到 .NET 8.0\n当前已安装:\n%s\n", installedVersions)
		sendInstallProgress(gameType, "未检测到 .NET 8.0，开始安装...", 92)
	}
	if _, err := os.Stat("/etc/debian_version"); err != nil {
		errMsg := "不支持的Linux发行版，仅支持 Debian/Ubuntu"
		fmt.Printf("[.NET安装] 错误: %s\n", errMsg)
		sendInstallProgress(gameType, fmt.Sprintf("警告: %s", errMsg), 95)
		return fmt.Errorf(errMsg)
	}
	sendInstallProgress(gameType, "添加 Microsoft 包仓库...", 93)
	fmt.Println("[.NET安装] 添加 Microsoft 包仓库...")
	downloadCmd := exec.Command("wget", "-q",
		"https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb",
		"-O", "/tmp/packages-microsoft-prod.deb")
	if output, err := downloadCmd.CombinedOutput(); err != nil {
		errMsg := fmt.Sprintf("下载 Microsoft 包配置失败: %v\n%s", err, string(output))
		fmt.Printf("[.NET安装] 错误: %s\n", errMsg)
		sendInstallProgress(gameType, "警告: Microsoft 包仓库添加失败", 95)
		return fmt.Errorf(errMsg)
	}
	if output, err := runPackageManagerCommandWithRetry(gameType, 93, "安装 Microsoft 包仓库配置", "dpkg", "-i", "/tmp/packages-microsoft-prod.deb"); err != nil {
		fmt.Printf("[.NET安装] dpkg 警告: %v\n%s\n", err, string(output))
	}
	os.Remove("/tmp/packages-microsoft-prod.deb")
	sendInstallProgress(gameType, "更新包列表...", 94)
	fmt.Println("[.NET安装] 更新包列表...")
	if output, err := runPackageManagerCommandWithRetry(gameType, 94, "更新包列表", "apt-get", "update", "-qq"); err != nil {
		fmt.Printf("[.NET安装] apt-get update 警告: %v\n%s\n", err, string(output))
	}
	sendInstallProgress(gameType, "安装 .NET 8.0 运行时（可能需要几分钟）...", 95)
	fmt.Println("[.NET安装] 安装 .NET 8.0 运行时...")
	output, err := runPackageManagerCommandWithRetry(gameType, 95, "安装 .NET 8.0 运行时", "apt-get", "install", "-y", "dotnet-runtime-8.0")
	fmt.Printf("[.NET安装] 安装输出:\n%s\n", string(output))
	if err != nil {
		errMsg := fmt.Sprintf("安装 .NET 8.0 失败: %v", err)
		fmt.Printf("[.NET安装] 错误: %s\n", errMsg)
		sendInstallProgress(gameType, "警告: .NET 8.0 自动安装失败，请手动安装", 95)
		return fmt.Errorf(errMsg)
	}
	sendInstallProgress(gameType, "验证 .NET 8.0 安装...", 97)
	fmt.Println("[.NET安装] 验证安装...")
	hasNet8, installedVersions, err = checkDotNetVersion()
	if err != nil {
		errMsg := fmt.Sprintf("验证失败: %v", err)
		fmt.Printf("[.NET安装] 错误: %s\n", errMsg)
		sendInstallProgress(gameType, "警告: .NET 8.0 验证失败", 98)
		return fmt.Errorf(errMsg)
	}
	if !hasNet8 {
		errMsg := "安装后未检测到 .NET 8.0"
		fmt.Printf("[.NET安装] 错误: %s\n当前已安装:\n%s\n", errMsg, installedVersions)
		sendInstallProgress(gameType, "警告: .NET 8.0 安装验证失败", 98)
		return fmt.Errorf(errMsg)
	}
	fmt.Println("[.NET安装] ✓ .NET 8.0 安装成功！")
	fmt.Printf("已安装的运行时:\n%s\n", installedVersions)
	sendInstallProgress(gameType, "✓ .NET 8.0 安装成功", 98)
	return nil
}
func installDotNetIfNeeded(gameType string) {
	if err := installDotNet8(gameType); err != nil {
		fmt.Printf("[.NET安装] 自动安装失败: %v\n", err)
		fmt.Println("[.NET安装] 请手动执行以下命令安装:")
		fmt.Println("  wget https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb")
		fmt.Println("  sudo dpkg -i packages-microsoft-prod.deb")
		fmt.Println("  sudo apt-get update")
		fmt.Println("  sudo apt-get install -y dotnet-runtime-8.0")
	}
}

func resolveGamePackageSpec(gameType string) (GamePackageSpec, error) {
	switch gameType {
	case "vanilla":
		return GamePackageSpec{
			GameType:      gameType,
			DownloadURL:   "https://terraria.org/api/download/pc-dedicated-server/terraria-server-1449.zip",
			TargetDir:     filepath.Join(config.ServersDir, "vanilla"),
			LatestVersion: "1.4.4.9",
			UpdateChannel: "vanilla",
		}, nil
	case "tmodloader":
		downloadURL, latestVersion := getLatestTModLoaderRelease()
		return GamePackageSpec{
			GameType:      gameType,
			DownloadURL:   downloadURL,
			TargetDir:     filepath.Join(config.ServersDir, "tModLoader"),
			LatestVersion: latestVersion,
			UpdateChannel: "tmodloader",
		}, nil
	case "tshock5":
		info := getLatestTShock5ReleaseInfo()
		return GamePackageSpec{
			GameType:      gameType,
			DownloadURL:   info.DownloadURL,
			TargetDir:     filepath.Join(config.ServersDir, "tshock"),
			LatestVersion: info.Version,
			UpdateChannel: "tshock5",
			RuntimeMajor:  info.RuntimeMajor,
		}, nil
	case "tshock6":
		info := getLatestTShock6ReleaseInfo()
		return GamePackageSpec{
			GameType:      gameType,
			DownloadURL:   info.DownloadURL,
			TargetDir:     filepath.Join(config.ServersDir, "tshock"),
			LatestVersion: info.Version,
			UpdateChannel: "tshock6",
			RuntimeMajor:  info.RuntimeMajor,
		}, nil
	default:
		return GamePackageSpec{}, fmt.Errorf("不支持的游戏类型: %s", gameType)
	}
}

func getVersionMarkerPath(gameType, targetDir string) string {
	if gameType == "tshock5" || gameType == "tshock6" {
		return filepath.Join(targetDir, ".tshock_version")
	}
	return filepath.Join(targetDir, ".installed_version")
}

func writeInstalledVersionMarker(gameType, targetDir, version string) error {
	if version == "" {
		return nil
	}
	return os.WriteFile(getVersionMarkerPath(gameType, targetDir), []byte(version), 0644)
}

func readTShockVersionMarker() string {
	versionFile := filepath.Join(config.ServersDir, "tshock", ".tshock_version")
	if data, err := os.ReadFile(versionFile); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

func getInstalledGameVersion(gameType string) (string, bool) {
	switch gameType {
	case "vanilla":
		if !checkVanillaInstalled() {
			return "", false
		}
		data, err := os.ReadFile(filepath.Join(config.ServersDir, "vanilla", ".installed_version"))
		if err != nil {
			return "", false
		}
		version := strings.TrimSpace(string(data))
		return version, version != ""
	case "tmodloader":
		if !checkTModLoaderInstalled() {
			return "", false
		}
		data, err := os.ReadFile(filepath.Join(config.ServersDir, "tModLoader", ".installed_version"))
		if err != nil {
			return "", false
		}
		version := strings.TrimSpace(string(data))
		return version, version != ""
	case "tshock5":
		if !checkTShockInstalled() || isTShock6() {
			return "", false
		}
		version := readTShockVersionMarker()
		if version == "" || version == "5" {
			return "", false
		}
		if strings.HasPrefix(version, "5.") {
			return version, true
		}
		return "", false
	case "tshock6":
		if !checkTShockInstalled() || !isTShock6() {
			return "", false
		}
		version := readTShockVersionMarker()
		if version == "" || version == "6" {
			return "", false
		}
		if strings.HasPrefix(version, "6.") {
			return version, true
		}
		return "", false
	default:
		return "", false
	}
}

func isGameInstalledForType(gameType string) bool {
	switch gameType {
	case "vanilla":
		return checkVanillaInstalled()
	case "tmodloader":
		return checkTModLoaderInstalled()
	case "tshock5":
		return checkTShockInstalled() && !isTShock6()
	case "tshock6":
		return checkTShockInstalled() && isTShock6()
	default:
		return false
	}
}

func isGameUpdateAvailable(installedVersion, latestVersion string) bool {
	if latestVersion == "" {
		return false
	}
	if strings.TrimSpace(installedVersion) == "" {
		return true
	}
	return compareVersionStrings(installedVersion, latestVersion) < 0
}

func compareVersionStrings(a, b string) int {
	aNums := numericVersionRegexp.FindAllString(a, -1)
	bNums := numericVersionRegexp.FindAllString(b, -1)
	maxLen := len(aNums)
	if len(bNums) > maxLen {
		maxLen = len(bNums)
	}
	for i := 0; i < maxLen; i++ {
		aVal := 0
		bVal := 0
		if i < len(aNums) {
			fmt.Sscanf(aNums[i], "%d", &aVal)
		}
		if i < len(bNums) {
			fmt.Sscanf(bNums[i], "%d", &bVal)
		}
		if aVal < bVal {
			return -1
		}
		if aVal > bVal {
			return 1
		}
	}
	return 0
}

func isGameTypeRunning(gameType string) (bool, string) {
	if gameType == "tshock5" || gameType == "tshock6" {
		if p, exists := utils.GetProcess(0); exists && p.IsRunning() {
			return true, "插件服正在运行，请先停止后再更新"
		}
		return false, ""
	}

	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	rooms, err := roomStorage.GetAll()
	if err != nil {
		fmt.Printf("[更新检测] 读取房间列表失败: %v\n", err)
		return false, ""
	}

	expectedType := gameType
	for _, room := range rooms {
		if room.ServerType != expectedType {
			continue
		}
		if room.Status == "running" {
			if p, exists := utils.GetProcess(room.ID); !exists || p.IsRunning() {
				return true, fmt.Sprintf("房间 %s 正在运行，请先停止后再更新", room.Name)
			}
		}
		if p, exists := utils.GetProcess(room.ID); exists && p.IsRunning() {
			return true, fmt.Sprintf("房间 %s 正在运行，请先停止后再更新", room.Name)
		}
	}

	return false, ""
}

func createGameUpdateBackup(gameType, sourceDir, fromVersion string) (string, error) {
	if _, err := os.Stat(sourceDir); err != nil {
		return "", fmt.Errorf("更新源目录不存在: %v", err)
	}

	backupDir := filepath.Join(config.BackupDir, "game-updates")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	safeVersion := regexp.MustCompile(`[^0-9A-Za-z._-]+`).ReplaceAllString(fromVersion, "_")
	if safeVersion == "" {
		safeVersion = "unknown"
	}

	fileName := fmt.Sprintf("%s_%s_%s.zip", gameType, safeVersion, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(backupDir, fileName)

	zipFile, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	if err := addDirToZip(zipWriter, sourceDir, filepath.Base(sourceDir)); err != nil {
		return "", err
	}

	return backupPath, nil
}

func downloadAndExtractGamePackage(gameType, downloadURL, targetDir string, sendProgress func(string, int)) error {
	tempFile := filepath.Join(targetDir, "download.tmp")
	sendProgress("开始下载更新包", 10)

	cfg := config.Load()
	downloadOpts := utils.GetDownloadConfig(cfg, downloadURL, tempFile, func(percent int) {
		actualProgress := 10 + (percent * 45 / 100)
		sendProgress(fmt.Sprintf("正在下载更新包... %d%%", percent), actualProgress)
	})
	if err := utils.DownloadWithRetry(downloadOpts); err != nil {
		return fmt.Errorf("下载更新包失败: %v", err)
	}

	sendProgress("下载完成，检测文件格式", 58)
	file, err := os.Open(tempFile)
	if err != nil {
		return fmt.Errorf("打开更新包失败: %v", err)
	}
	header := make([]byte, 512)
	n, _ := file.Read(header)
	file.Close()

	isZip := n >= 4 && header[0] == 0x50 && header[1] == 0x4B && header[2] == 0x03 && header[3] == 0x04
	isTar := n >= 262 && string(header[257:262]) == "ustar"

	var downloadFile string
	if isZip {
		downloadFile = filepath.Join(targetDir, "download.zip")
	} else if isTar {
		downloadFile = filepath.Join(targetDir, "download.tar")
	} else {
		downloadFile = filepath.Join(targetDir, "download.zip")
	}
	if err := os.Rename(tempFile, downloadFile); err != nil {
		return fmt.Errorf("整理更新包失败: %v", err)
	}

	sendProgress("开始解压更新包", 62)
	if isZip {
		if err := unzipFile(downloadFile, targetDir); err != nil {
			return fmt.Errorf("ZIP 解压失败: %v", err)
		}
	} else if isTar {
		if err := extractTarFile(downloadFile, targetDir); err != nil {
			return fmt.Errorf("TAR 解压失败: %v", err)
		}
	} else {
		if err := unzipFile(downloadFile, targetDir); err != nil {
			if err2 := extractTarFile(downloadFile, targetDir); err2 != nil {
				return fmt.Errorf("解压失败，ZIP错误: %v, TAR错误: %v", err, err2)
			}
		}
	}

	files, err := os.ReadDir(targetDir)
	if err == nil {
		for _, f := range files {
			name := strings.ToLower(f.Name())
			if strings.HasPrefix(name, "download.") {
				continue
			}
			if strings.HasSuffix(name, ".tar") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
				nestedArchive := filepath.Join(targetDir, f.Name())
				sendProgress("发现嵌套压缩包，继续解压", 66)
				if strings.HasSuffix(name, ".tar") {
					if err := extractTarFile(nestedArchive, targetDir); err != nil {
						return fmt.Errorf("嵌套 TAR 解压失败: %v", err)
					}
				} else {
					if err := unzipFile(nestedArchive, targetDir); err != nil {
						return fmt.Errorf("嵌套 ZIP 解压失败: %v", err)
					}
				}
				_ = os.Remove(nestedArchive)
				break
			}
		}
	}

	_ = os.Remove(downloadFile)
	return nil
}

func finalizePreparedGameFiles(gameType, targetDir, resolvedVersion string, sendProgress func(string, int)) error {
	switch gameType {
	case "vanilla":
		sendProgress("整理原版服务端文件", 78)
		linuxDir := filepath.Join(targetDir, "1449", "Linux")
		windowsDir := filepath.Join(targetDir, "1449", "Windows")
		sourceDir := windowsDir
		if runtime.GOOS == "linux" {
			sourceDir = linuxDir
		}
		if _, err := os.Stat(sourceDir); err == nil {
			if err := moveFiles(sourceDir, targetDir); err != nil {
				return err
			}
			_ = os.RemoveAll(filepath.Join(targetDir, "1449"))
		}
		if runtime.GOOS == "linux" {
			_ = os.Chmod(filepath.Join(targetDir, "TerrariaServer"), 0755)
		}
	case "tmodloader":
		sendProgress("配置 tModLoader 运行环境", 82)
		if runtime.GOOS == "linux" {
			_ = os.Chmod(filepath.Join(targetDir, "start-tModLoaderServer.sh"), 0755)
			sendProgress("检查 .NET 8 运行时", 88)
			installDotNetIfNeeded(gameType)
		}
	case "tshock5", "tshock6":
		sendProgress("配置 TShock 运行环境", 82)
		if runtime.GOOS == "linux" {
			if _, err := os.Stat(filepath.Join(targetDir, "TShock.Server")); err == nil {
				_ = os.Chmod(filepath.Join(targetDir, "TShock.Server"), 0755)
			}
			if _, err := os.Stat(filepath.Join(targetDir, "TShock.Server.dll")); err == nil {
				_ = os.Chmod(filepath.Join(targetDir, "TShock.Server.dll"), 0755)
			}
			if gameType == "tshock6" {
				sendProgress("检查并安装 .NET 9.0 运行时", 88)
				if err := installDotNet9(gameType); err != nil {
					return fmt.Errorf(".NET 9.0 自动安装失败: %v", err)
				}
			} else {
				sendProgress("检查并安装 .NET 6.0 运行时", 88)
				if err := installDotNet6(gameType); err != nil {
					return fmt.Errorf(".NET 6.0 自动安装失败: %v", err)
				}
			}
			if err := writeInstalledVersionMarker(gameType, targetDir, resolvedVersion); err != nil {
				return err
			}
		}
	}

	if gameType != "tshock5" && gameType != "tshock6" {
		if err := writeInstalledVersionMarker(gameType, targetDir, resolvedVersion); err != nil {
			return err
		}
	}

	return nil
}

func copyDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if err := copyRecursive(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func extractTarFile(src string, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()
	tarReader := tar.NewReader(file)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("非法的文件路径: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}
func getLatestTModLoaderRelease() (string, string) {
	apiUrl := "https://api.github.com/repos/tModLoader/tModLoader/releases/latest"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		return "https://github.com/tModLoader/tModLoader/releases/download/v2025.08.3.1/tModLoader.zip", "2025.08.3.1"
	}
	req.Header.Set("User-Agent", "Terraria-Panel")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求GitHub API失败: %v\n", err)
		return "https://github.com/tModLoader/tModLoader/releases/download/v2025.08.3.1/tModLoader.zip", "2025.08.3.1"
	}
	defer resp.Body.Close()
	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Printf("解析GitHub响应失败: %v\n", err)
		return "https://github.com/tModLoader/tModLoader/releases/download/v2025.08.3.1/tModLoader.zip", "2025.08.3.1"
	}
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "example") || strings.Contains(name, "source") {
			continue
		}
		if name == "tmodloader.zip" ||
			strings.Contains(name, "tmodloader") && strings.HasSuffix(name, ".zip") ||
			strings.Contains(name, "linux") && strings.HasSuffix(name, ".zip") {
			version := strings.TrimPrefix(release.TagName, "v")
			fmt.Printf("获取到 tModLoader 最新版本: %s (%s)\n", version, asset.Name)
			return asset.BrowserDownloadURL, version
		}
	}
	fmt.Printf("未找到合适的tModLoader文件，使用默认值\n")
	return "https://github.com/tModLoader/tModLoader/releases/download/v2025.08.3.1/tModLoader.zip", "2025.08.3.1"
}
func getLatestTShockRelease() (string, string) {
	info := getLatestTShock5ReleaseInfo()
	return info.DownloadURL, info.Version
}

func getLatestTShock6Release() (string, string) {
	info := getLatestTShock6ReleaseInfo()
	return info.DownloadURL, info.Version
}

func getLatestTShock5ReleaseInfo() TShockReleaseInfo {
	return getLatestTShockReleaseInfoForMajor("5", TShockReleaseInfo{
		Version:         "5.2.4",
		TerrariaVersion: "1.4.4.9",
		DownloadURL:     "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip",
		PublishedAt:     "2025-05-09T09:23:08Z",
		Prerelease:      false,
		RuntimeMajor:    "6.0",
		DisplayName:     "TShock 5 稳定版",
	})
}

func getLatestTShock6ReleaseInfo() TShockReleaseInfo {
	return getLatestTShockReleaseInfoForMajor("6", TShockReleaseInfo{
		Version:         "6.1.0",
		TerrariaVersion: "1.4.5.6",
		DownloadURL:     "https://github.com/Pryaxis/TShock/releases/download/v6.1.0/TShock-6.1.0-for-Terraria-1.4.5.6-linux-x64-Release.zip",
		PublishedAt:     "2026-03-11T18:42:52Z",
		Prerelease:      false,
		RuntimeMajor:    "9.0",
		DisplayName:     "TShock 6 新版稳定版",
	})
}

func getLatestTShockReleaseInfoForMajor(major string, fallback TShockReleaseInfo) TShockReleaseInfo {
	apiUrl := "https://api.github.com/repos/Pryaxis/TShock/releases?per_page=30"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		fmt.Printf("[TShock %s] 创建请求失败: %v\n", major, err)
		return fallback
	}
	req.Header.Set("User-Agent", "Terraria-Panel")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[TShock %s] 请求 GitHub API 失败: %v\n", major, err)
		return fallback
	}
	defer resp.Body.Close()

	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		fmt.Printf("[TShock %s] 解析 GitHub 响应失败: %v\n", major, err)
		return fallback
	}

	for _, release := range releases {
		version := strings.TrimPrefix(release.TagName, "v")
		if !strings.HasPrefix(version, major+".") || release.Prerelease {
			continue
		}

		bestAsset := ""
		for _, asset := range release.Assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, "linux") &&
				(strings.Contains(name, "x64") || strings.Contains(name, "amd64")) &&
				strings.HasSuffix(name, ".zip") {
				bestAsset = asset.BrowserDownloadURL
				break
			}
		}
		if bestAsset == "" {
			continue
		}

		info := fallback
		info.Version = version
		info.DownloadURL = bestAsset
		info.PublishedAt = release.PublishedAt
		info.Prerelease = release.Prerelease
		if major == "6" {
			info.RuntimeMajor = "9.0"
		} else {
			info.RuntimeMajor = "6.0"
		}
		if terrariaVersion := parseTerrariaVersion(release.Name + " " + bestAsset); terrariaVersion != "" {
			info.TerrariaVersion = terrariaVersion
		}
		return info
	}

	fmt.Printf("[TShock %s] 未找到更合适的正式版，使用默认值 %s\n", major, fallback.Version)
	return fallback
}

func parseTerrariaVersion(input string) string {
	re := regexp.MustCompile(`Terraria-([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)
	if matches := re.FindStringSubmatch(input); len(matches) > 1 {
		return matches[1]
	}
	re = regexp.MustCompile(`Terraria\s+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)
	if matches := re.FindStringSubmatch(input); len(matches) > 1 {
		return matches[1]
	}
	return ""
}
func UninstallGame(c *gin.Context) {
	var req struct {
		GameType string `json:"gameType"`
		Mode     string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}
	if req.Mode == "" {
		req.Mode = "full"
	}
	if req.GameType != "vanilla" && req.GameType != "tmodloader" && req.GameType != "tshock" && req.GameType != "tshock5" && req.GameType != "tshock6" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的游戏类型",
		})
		return
	}
	installed := false
	switch req.GameType {
	case "vanilla":
		installed = checkVanillaInstalled()
	case "tmodloader":
		installed = checkTModLoaderInstalled()
	case "tshock", "tshock5", "tshock6":
		installed = checkTShockInstalled()
	}
	if !installed {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该游戏尚未安装",
		})
		return
	}
	var targetDir string
	switch req.GameType {
	case "vanilla":
		targetDir = filepath.Join(config.ServersDir, "vanilla")
	case "tmodloader":
		targetDir = filepath.Join(config.ServersDir, "tModLoader")
	case "tshock", "tshock5", "tshock6":
		targetDir = filepath.Join(config.ServersDir, "tshock")
	}
	fmt.Printf("\n========================================\n")
	fmt.Printf("[卸载开始] 游戏类型: %s, 模式: %s\n", req.GameType, req.Mode)
	fmt.Printf("[卸载] 目标目录: %s\n", targetDir)
	fmt.Printf("========================================\n\n")
	var err error
	if req.Mode == "keep-data" && (req.GameType == "tshock" || req.GameType == "tshock5" || req.GameType == "tshock6") {
		err = uninstallTShockKeepData(targetDir)
	} else {
		err = os.RemoveAll(targetDir)
	}
	if err != nil {
		fmt.Printf("[卸载失败] %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("卸载失败: %v", err),
		})
		return
	}
	modeDesc := "完全"
	if req.Mode == "keep-data" {
		modeDesc = "保留数据"
	}
	fmt.Printf("[卸载成功] %s 已%s卸载\n", req.GameType, modeDesc)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("%s卸载成功", modeDesc),
	})
}
func uninstallTShockKeepData(targetDir string) error {
	fmt.Println("[保留数据卸载] 开始备份重要数据...")
	tempBackup := filepath.Join(os.TempDir(), fmt.Sprintf("tshock_backup_%d", time.Now().Unix()))
	if err := os.MkdirAll(tempBackup, 0755); err != nil {
		return fmt.Errorf("创建备份目录失败: %v", err)
	}
	defer os.RemoveAll(tempBackup)
	fmt.Printf("[备份] 临时目录: %s\n", tempBackup)
	itemsToKeep := map[string]string{
		"ServerPlugins":         "ServerPlugins",
		"tshock/config.json":    "tshock/config.json",
		"tshock/sscconfig.json": "tshock/sscconfig.json",
		"tshock/motd.txt":       "tshock/motd.txt",
		"tshock":                "tshock",
	}
	backupCount := 0
	for srcPath, dstPath := range itemsToKeep {
		srcFull := filepath.Join(targetDir, srcPath)
		dstFull := filepath.Join(tempBackup, dstPath)
		if _, err := os.Stat(srcFull); os.IsNotExist(err) {
			fmt.Printf("[备份] 跳过不存在的项: %s\n", srcPath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstFull), 0755); err != nil {
			return fmt.Errorf("创建备份子目录失败: %v", err)
		}
		if err := copyRecursive(srcFull, dstFull); err != nil {
			return fmt.Errorf("备份 %s 失败: %v", srcPath, err)
		}
		backupCount++
		fmt.Printf("[备份] ✓ %s\n", srcPath)
	}
	fmt.Printf("[备份] 完成，共备份 %d 项\n", backupCount)
	fmt.Println("[删除] 删除旧版本...")
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("删除目录失败: %v", err)
	}
	fmt.Println("[删除] ✓ 旧版本已删除")
	fmt.Println("[恢复] 重建目录结构...")
	dirsToCreate := []string{
		targetDir,
		filepath.Join(targetDir, "ServerPlugins"),
		filepath.Join(targetDir, "tshock"),
	}
	for _, dir := range dirsToCreate {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %v", dir, err)
		}
	}
	fmt.Println("[恢复] ✓ 目录结构已重建")
	fmt.Println("[恢复] 恢复数据...")
	restoreCount := 0
	err := filepath.Walk(tempBackup, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(tempBackup, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(targetDir, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			if err := copyFile(path, dstPath); err != nil {
				return err
			}
			restoreCount++
			return nil
		}
	})
	if err != nil {
		return fmt.Errorf("恢复数据失败: %v", err)
	}
	fmt.Printf("[恢复] ✓ 完成，共恢复 %d 个文件\n", restoreCount)
	versionFile := filepath.Join(targetDir, ".tshock_version")
	if err := os.Remove(versionFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[警告] 删除版本标记文件失败: %v\n", err)
	} else {
		fmt.Println("[清理] ✓ 已删除版本标记文件")
	}
	fmt.Println("[保留数据卸载] ✓ 完成！插件、配置、数据库已保留")
	return nil
}
func copyRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		return copyDir(src, dst)
	} else {
		return copyFile(src, dst)
	}
}
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
func installDotNet6(gameType string) error {
	return installDotNetRuntime(gameType, "6.0")
}
func installDotNet9(gameType string) error {
	return installDotNetRuntime(gameType, "9.0")
}

func assessDotNetRuntimeHealth(runtimeVersion string) DotNetRuntimeHealth {
	health := DotNetRuntimeHealth{
		Healthy:          true,
		RuntimeVersion:   runtimeVersion,
		RepairSuggestion: "点击面板中的“一键修复 .NET 环境”即可自动清理混装并重装运行时。",
	}

	dotnetPath, err := exec.LookPath("dotnet")
	if err != nil {
		health.Healthy = false
		health.NeedsRepair = false
		health.IssueType = "missing-dotnet"
		health.Message = fmt.Sprintf("未找到 dotnet 命令，需要先安装 .NET %s", runtimeVersion)
		return health
	}
	health.DetectedDotnet = dotnetPath

	if resolvedPath, resolveErr := filepath.EvalSymlinks(dotnetPath); resolveErr == nil {
		health.DetectedRoot = filepath.Dir(resolvedPath)
	}

	hasRuntime, runtimesOutput, runtimeErr := checkSpecificDotNetRuntime(runtimeVersion)
	if runtimeErr == nil {
		health.Runtimes = strings.TrimSpace(runtimesOutput)
	}

	if hasRuntime {
		return health
	}

	altRoots := []string{"/usr/share/dotnet", "/usr/lib/dotnet"}
	var altRuntimeLocations []string
	for _, root := range altRoots {
		candidate := filepath.Join(root, "shared", "Microsoft.NETCore.App")
		entries, err := os.ReadDir(candidate)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), runtimeVersion) {
				altRuntimeLocations = append(altRuntimeLocations, filepath.Join(candidate, entry.Name()))
			}
		}
	}

	pkgOutput, _ := runCommandWithOutput("dpkg-query", "-W", "-f=${Status}", "dotnet-runtime-"+runtimeVersion)
	packageInstalled := strings.Contains(pkgOutput, "install ok installed")

	if len(altRuntimeLocations) > 0 || packageInstalled {
		health.Healthy = false
		health.NeedsRepair = true
		health.IssueType = "mixed-packages"
		health.Message = fmt.Sprintf("检测到 .NET %s 包已安装，但当前 dotnet 无法识别，系统可能存在 .NET 源混装。", runtimeVersion)
		if len(altRuntimeLocations) > 0 {
			health.Message += fmt.Sprintf(" 发现运行时目录：%s", strings.Join(altRuntimeLocations, ", "))
		}
		return health
	}

	health.Healthy = false
	health.NeedsRepair = false
	health.IssueType = "missing-runtime"
	health.Message = fmt.Sprintf("当前未检测到 .NET %s 运行时。", runtimeVersion)
	return health
}

func installDotNetRuntime(gameType, runtimeVersion string) error {
	fmt.Printf("[.NET %s] 开始检测...\n", runtimeVersion)
	health := assessDotNetRuntimeHealth(runtimeVersion)
	if health.NeedsRepair {
		return fmt.Errorf("%s | repair=required", health.Message)
	}
	hasRuntime, runtimesOutput, err := checkSpecificDotNetRuntime(runtimeVersion)
	if err == nil && hasRuntime {
		fmt.Printf("[.NET %s] ✓ 已安装，跳过\n", runtimeVersion)
		sendInstallProgress(gameType, fmt.Sprintf("✓ 已安装 .NET %s", runtimeVersion), 93)
		return nil
	}

	if err != nil {
		fmt.Printf("[.NET %s] 运行时检测失败: %v\n", runtimeVersion, err)
	} else {
		fmt.Printf("[.NET %s] 未检测到目标运行时，当前已安装:\n%s\n", runtimeVersion, runtimesOutput)
	}

	info, repoName, err := detectSupportedUbuntuRepo()
	if err != nil {
		errMsg := fmt.Sprintf("当前系统暂不支持自动安装 .NET %s: %v", runtimeVersion, err)
		sendInstallProgress(gameType, errMsg, 93)
		return fmt.Errorf("%s | 手动安装：%s", errMsg, buildManualDotNetInstallCommand(runtimeVersion, ""))
	}

	systemLabel := fmt.Sprintf("%s %s", strings.ToLower(info.ID), info.VersionID)
	sendInstallProgress(gameType, fmt.Sprintf("检测到系统 %s，准备安装 .NET %s", systemLabel, runtimeVersion), 91)
	fmt.Printf("[.NET %s] 检测到系统: %s, 使用源: %s\n", runtimeVersion, systemLabel, repoName)

	sendInstallProgress(gameType, "正在准备 Ubuntu .NET 源...", 91)
	if output, stepErr := runPackageManagerCommandWithRetry(gameType, 91, "安装 software-properties-common", "apt-get", "install", "-y", "software-properties-common"); stepErr != nil {
		return fmt.Errorf("安装 software-properties-common 失败 | system=%s | repo=%s | output=%s | manual=%s",
			systemLabel, repoName, strings.TrimSpace(output), buildManualDotNetInstallCommand(runtimeVersion, repoName))
	}

	if output, stepErr := runPackageManagerCommandWithRetry(gameType, 91, "添加 Ubuntu .NET backports 源", "add-apt-repository", "-y", "ppa:dotnet/backports"); stepErr != nil {
		return fmt.Errorf("添加 Ubuntu .NET backports 源失败 | system=%s | repo=%s | output=%s | manual=%s",
			systemLabel, repoName, strings.TrimSpace(output), buildManualDotNetInstallCommand(runtimeVersion, repoName))
	}

	sendInstallProgress(gameType, fmt.Sprintf("已添加 %s 源，更新软件包列表...", repoName), 92)
	if output, stepErr := runPackageManagerCommandWithRetry(gameType, 92, "更新软件包列表", "apt-get", "update", "-qq"); stepErr != nil {
		return fmt.Errorf("apt-get update 失败 | system=%s | repo=%s | output=%s | manual=%s",
			systemLabel, repoName, strings.TrimSpace(output), buildManualDotNetInstallCommand(runtimeVersion, repoName))
	}

	sendInstallProgress(gameType, fmt.Sprintf("正在安装 .NET %s Runtime...", runtimeVersion), 92)
	output, stepErr := runPackageManagerCommandWithRetry(gameType, 92, fmt.Sprintf("安装 .NET %s Runtime", runtimeVersion), "apt-get", "install", "-y", "dotnet-runtime-"+runtimeVersion)
	if stepErr != nil {
		return fmt.Errorf("安装 .NET %s 失败 | system=%s | repo=%s | output=%s | manual=%s",
			runtimeVersion, systemLabel, repoName, strings.TrimSpace(string(output)), buildManualDotNetInstallCommand(runtimeVersion, repoName))
	}

	hasRuntime, runtimesOutput, err = checkSpecificDotNetRuntime(runtimeVersion)
	if err != nil {
		return fmt.Errorf("安装后验证 .NET %s 失败 | system=%s | repo=%s | err=%v | manual=%s",
			runtimeVersion, systemLabel, repoName, err, buildManualDotNetInstallCommand(runtimeVersion, repoName))
	}
	if !hasRuntime {
		return fmt.Errorf("安装后仍未检测到 .NET %s | system=%s | repo=%s | runtimes=%s | manual=%s",
			runtimeVersion, systemLabel, repoName, strings.TrimSpace(runtimesOutput), buildManualDotNetInstallCommand(runtimeVersion, repoName))
	}

	fmt.Printf("[.NET %s] ✓ 安装成功\n", runtimeVersion)
	sendInstallProgress(gameType, fmt.Sprintf("✓ .NET %s 安装成功", runtimeVersion), 93)
	return nil
}

func checkSpecificDotNetRuntime(runtimeVersion string) (bool, string, error) {
	cmd := exec.Command("dotnet", "--list-runtimes")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", err
	}
	outputStr := string(output)
	return strings.Contains(outputStr, "Microsoft.NETCore.App "+runtimeVersion), outputStr, nil
}

func detectSupportedUbuntuRepo() (LinuxDistroInfo, string, error) {
	info, err := detectLinuxDistroInfo()
	if err != nil {
		return LinuxDistroInfo{}, "", err
	}

	if strings.ToLower(info.ID) != "ubuntu" {
		return info, "", fmt.Errorf("仅支持 Ubuntu 22.04/24.04，当前系统为 %s %s", info.ID, info.VersionID)
	}

	switch info.VersionID {
	case "22.04":
		return info, "ppa:dotnet/backports", nil
	case "24.04":
		return info, "ppa:dotnet/backports", nil
	default:
		return info, "", fmt.Errorf("仅支持 Ubuntu 22.04/24.04，当前版本为 %s", info.VersionID)
	}
}

func detectLinuxDistroInfo() (LinuxDistroInfo, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return LinuxDistroInfo{}, err
	}

	info := LinuxDistroInfo{}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := strings.Trim(parts[1], `"`)
		switch key {
		case "ID":
			info.ID = value
		case "VERSION_ID":
			info.VersionID = value
		case "VERSION_CODENAME":
			info.VersionCodename = value
		}
	}

	if info.ID == "" || info.VersionID == "" {
		return info, fmt.Errorf("无法从 /etc/os-release 解析系统版本")
	}

	return info, nil
}

func runCommandWithOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runPackageManagerCommandWithRetry(gameType string, progress int, stepLabel string, name string, args ...string) (string, error) {
	return runPackageManagerCommandWithRetryForAction(gameType, "install", progress, stepLabel, name, args...)
}

func runPackageManagerCommandWithRetryForAction(gameType, action string, progress int, stepLabel string, name string, args ...string) (string, error) {
	const maxRetries = 24
	const retryDelay = 5 * time.Second

	var lastOutput string
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		cmd := exec.Command(name, args...)
		if name == "apt-get" {
			cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		}

		output, err := cmd.CombinedOutput()
		lastOutput = string(output)
		lastErr = err

		if err == nil {
			return lastOutput, nil
		}

		if !isPackageManagerLockError(lastOutput) || attempt == maxRetries {
			return lastOutput, lastErr
		}

		if action == "repair" {
			sendRepairProgress(gameType, fmt.Sprintf("%s：检测到 apt 正在被占用，等待后重试（%d/%d）", stepLabel, attempt, maxRetries), progress)
		} else {
			sendInstallProgress(gameType, fmt.Sprintf("%s：检测到 apt 正在被占用，等待后重试（%d/%d）", stepLabel, attempt, maxRetries), progress)
		}
		fmt.Printf("[包管理器锁] %s 被占用，%s %v，%d 秒后重试（%d/%d）\n", stepLabel, name, args, int(retryDelay.Seconds()), attempt, maxRetries)
		time.Sleep(retryDelay)
	}

	return lastOutput, lastErr
}

func isPackageManagerLockError(output string) bool {
	lockMarkers := []string{
		"Could not get lock /var/lib/dpkg/lock-frontend",
		"Unable to acquire the dpkg frontend lock",
		"Could not get lock /var/lib/dpkg/lock",
		"Unable to lock the administration directory",
		"is another process using it?",
	}
	for _, marker := range lockMarkers {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

func buildManualDotNetInstallCommand(runtimeVersion, repoName string) string {
	if repoName == "" {
		repoName = "ppa:dotnet/backports"
	}
	return fmt.Sprintf("sudo apt-get install -y software-properties-common && sudo add-apt-repository -y %s && sudo apt-get update && sudo apt-get install -y dotnet-runtime-%s", repoName, runtimeVersion)
}

func writeDotNetBackportsPreference() error {
	preferencePath := "/etc/apt/preferences.d/terraria-panel-dotnet.pref"
	preferenceContent := strings.Join([]string{
		"Package: dotnet* aspnet* netstandard*",
		"Pin: origin packages.microsoft.com",
		"Pin-Priority: -10",
		"",
	}, "\n")
	return os.WriteFile(preferencePath, []byte(preferenceContent), 0644)
}
