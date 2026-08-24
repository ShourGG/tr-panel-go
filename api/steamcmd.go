package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- 依赖安装后台任务 ----------

type depsInstallState struct {
	mu       sync.Mutex
	Running  bool   `json:"running"`
	Step     int    `json:"step"`     // 当前步骤 (1-based)
	Total    int    `json:"total"`    // 总步骤数
	StepName string `json:"stepName"` // 当前步骤描述
	Done     bool   `json:"done"`
	Success  bool   `json:"success"`
	Error    string `json:"error"`
}

var depsState = &depsInstallState{}

type steamCMDSetupState struct {
	mu       sync.Mutex
	Running  bool   `json:"running"`
	Step     int    `json:"step"`
	Total    int    `json:"total"`
	StepName string `json:"stepName"`
	Done     bool   `json:"done"`
	Success  bool   `json:"success"`
	Error    string `json:"error"`
}

var steamSetupState = &steamCMDSetupState{}
var steamCMDPrepareMu sync.Mutex

func GetDepsInstallStatus(c *gin.Context) {
	depsState.mu.Lock()
	defer depsState.mu.Unlock()
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"running":  depsState.Running,
		"step":     depsState.Step,
		"total":    depsState.Total,
		"stepName": depsState.StepName,
		"done":     depsState.Done,
		"success":  depsState.Success,
		"error":    depsState.Error,
	}))
}

func getSteamCMDSetupStatus() gin.H {
	steamSetupState.mu.Lock()
	defer steamSetupState.mu.Unlock()
	return gin.H{
		"running":  steamSetupState.Running,
		"step":     steamSetupState.Step,
		"total":    steamSetupState.Total,
		"stepName": steamSetupState.StepName,
		"done":     steamSetupState.Done,
		"success":  steamSetupState.Success,
		"error":    steamSetupState.Error,
	}
}

func getSteamCMDPaths() (string, string, string) {
	steamcmdDir := filepath.Join(config.DataDir, "steamcmd")
	launcherPath := filepath.Join(steamcmdDir, "steamcmd.sh")
	runtimePath := filepath.Join(steamcmdDir, "linux32", "steamcmd")
	if runtime.GOOS == "windows" {
		launcherPath = filepath.Join(steamcmdDir, "steamcmd.exe")
		runtimePath = launcherPath
	}
	return steamcmdDir, launcherPath, runtimePath
}

const steamCMDReadyMarkerName = ".ready"

func getSteamCMDReadyMarkerPath() string {
	steamcmdDir, _, _ := getSteamCMDPaths()
	return filepath.Join(steamcmdDir, steamCMDReadyMarkerName)
}

func steamCMDReadyMarkerExists() bool {
	info, err := os.Stat(getSteamCMDReadyMarkerPath())
	return err == nil && !info.IsDir()
}

func markSteamCMDReady() error {
	markerPath := getSteamCMDReadyMarkerPath()
	temporaryPath := markerPath + ".tmp"
	if err := os.WriteFile(temporaryPath, []byte("SteamCMD initialization completed\n"), 0644); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, markerPath); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

func getSteamCMDState() (bool, bool, string, string, string) {
	_, launcherPath, runtimePath := getSteamCMDPaths()

	launcherExists := false
	if info, err := os.Stat(launcherPath); err == nil && !info.IsDir() {
		launcherExists = true
	}

	runtimeExists := false
	if info, err := os.Stat(runtimePath); err == nil && !info.IsDir() {
		runtimeExists = true
	}

	installed := launcherExists
	readyMarkerExists := steamCMDReadyMarkerExists()
	ready := launcherExists && runtimeExists && readyMarkerExists

	if ready {
		return installed, ready, launcherPath, runtimePath, "SteamCMD 已安装"
	}
	if installed {
		if !runtimeExists {
			return installed, ready, launcherPath, runtimePath, fmt.Sprintf("SteamCMD 安装不完整，缺少运行文件: %s", runtimePath)
		}
		if !readyMarkerExists {
			return installed, ready, launcherPath, runtimePath, "SteamCMD 尚未完成首次初始化，请等待 +quit 成功后重试"
		}
		return installed, ready, launcherPath, runtimePath, "SteamCMD 就绪标记无效，请重新初始化"
	}
	return installed, ready, launcherPath, runtimePath, "SteamCMD 未安装，可以自动安装"
}

func CheckSteamCMD(c *gin.Context) {
	installed, ready, steamcmdPath, runtimePath, stateMessage := getSteamCMDState()
	if ready {
		c.JSON(http.StatusOK, gin.H{
			"installed": true,
			"ready":     true,
			"path":      steamcmdPath,
			"message":   "SteamCMD 已安装",
		})
		return
	}
	if runtime.GOOS == "linux" {
		depCheckCmd := exec.Command("dpkg", "-l", "lib32gcc-s1")
		if err := depCheckCmd.Run(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"installed":    false,
				"deps_missing": true,
				"deps_commands": []string{
					"sudo dpkg --add-architecture i386",
					"sudo apt-get update",
					"sudo apt-get install -y lib32gcc-s1 lib32stdc++6",
				},
				"message": "缺少32位库依赖",
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"installed":    installed,
		"ready":        false,
		"path":         steamcmdPath,
		"runtime_path": runtimePath,
		"needs_repair": installed,
		"can_install":  true,
		"message":      stateMessage,
	})
}
func InstallDepsAPI(c *gin.Context) {
	if runtime.GOOS != "linux" {
		c.JSON(http.StatusOK, models.MessageResponse("非 Linux 系统，不需要安装依赖"))
		return
	}

	depsState.mu.Lock()
	if depsState.Running {
		depsState.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "安装已在进行中", "running": true})
		return
	}
	depsState.Running = true
	depsState.Step = 0
	depsState.Total = 3
	depsState.StepName = "准备中..."
	depsState.Done = false
	depsState.Success = false
	depsState.Error = ""
	depsState.mu.Unlock()

	go func() {
		err := installSteamCMDDependencies(func(step, total int, stepName string) {
			depsState.mu.Lock()
			depsState.Step = step
			depsState.Total = total
			depsState.StepName = stepName
			depsState.mu.Unlock()
		})

		depsState.mu.Lock()
		defer depsState.mu.Unlock()
		depsState.Running = false
		depsState.Done = true
		depsState.Success = err == nil
		if err != nil {
			depsState.Error = err.Error()
			return
		}
		depsState.Step = depsState.Total
		depsState.StepName = "依赖安装完成"
		depsState.Error = ""
	}()

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "开始安装依赖...", "running": true})
}

func InstallSteamCMDAPI(c *gin.Context) {
	installed, ready, _, _, _ := getSteamCMDState()
	if ready {
		c.JSON(http.StatusOK, models.MessageResponse("SteamCMD 已安装"))
		return
	}

	steamSetupState.mu.Lock()
	if steamSetupState.Running {
		steamSetupState.mu.Unlock()
		c.JSON(http.StatusAccepted, gin.H{
			"success": true,
			"message": "SteamCMD 正在准备中，请查看当前进度",
			"data":    getSteamCMDSetupStatus(),
		})
		return
	}
	steamSetupState.Running = true
	steamSetupState.Step = 0
	steamSetupState.Total = 5
	steamSetupState.StepName = "准备 SteamCMD 环境"
	steamSetupState.Done = false
	steamSetupState.Success = false
	steamSetupState.Error = ""
	steamSetupState.mu.Unlock()

	go func(wasInstalled bool) {
		err := prepareSteamCMD(func(step, total int, stepName string) {
			steamSetupState.mu.Lock()
			steamSetupState.Step = step
			steamSetupState.Total = total
			steamSetupState.StepName = stepName
			steamSetupState.mu.Unlock()
		})

		steamSetupState.mu.Lock()
		defer steamSetupState.mu.Unlock()
		steamSetupState.Running = false
		steamSetupState.Done = true
		steamSetupState.Success = err == nil
		if err != nil {
			steamSetupState.Error = err.Error()
			steamSetupState.StepName = "SteamCMD 准备失败"
			log.Printf("SteamCMD 准备失败: %v", err)
			return
		}
		steamSetupState.Step = steamSetupState.Total
		steamSetupState.StepName = "SteamCMD 已准备完成"
		steamSetupState.Error = ""
		if wasInstalled {
			log.Printf("SteamCMD 修复完成")
		} else {
			log.Printf("SteamCMD 安装完成")
		}
	}(installed)

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "已开始一键准备 SteamCMD、32 位依赖和创意工坊组件",
		"data":    getSteamCMDSetupStatus(),
	})
}
func GetSteamCMDStatus(c *gin.Context) {
	steamcmdDir, steamcmdPath, runtimePath := getSteamCMDPaths()
	installed, ready, _, _, stateMessage := getSteamCMDState()
	status := gin.H{
		"os": runtime.GOOS,
	}
	status["installed"] = ready
	status["launcher_exists"] = installed
	status["ready"] = ready
	status["path"] = steamcmdPath
	status["runtime_path"] = runtimePath
	status["message"] = stateMessage
	status["needs_repair"] = installed && !ready
	status["setup"] = getSteamCMDSetupStatus()
	if installed {
		if info, err := os.Stat(steamcmdDir); err == nil {
			status["install_time"] = info.ModTime()
		}
		var totalSize int64
		filepath.Walk(steamcmdDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				totalSize += info.Size()
			}
			return nil
		})
		status["size_mb"] = totalSize / 1024 / 1024
	}
	if runtime.GOOS == "linux" {
		depCheckCmd := exec.Command("dpkg", "-l", "lib32gcc-s1")
		if err := depCheckCmd.Run(); err != nil {
			status["deps_installed"] = false
			status["deps_commands"] = []string{
				"sudo dpkg --add-architecture i386",
				"sudo apt-get update",
				"sudo apt-get install -y lib32gcc-s1 lib32stdc++6",
			}
		} else {
			status["deps_installed"] = true
		}
	} else {
		status["deps_installed"] = true
	}
	workshopDir := filepath.Join(steamcmdDir, "steamapps", "workshop", "content", "1281930")
	if entries, err := os.ReadDir(workshopDir); err == nil {
		status["downloaded_mods"] = len(entries)
	} else {
		status["downloaded_mods"] = 0
	}
	c.JSON(http.StatusOK, models.SuccessResponse(status))
}

func prepareSteamCMD(report func(step, total int, stepName string)) error {
	steamCMDPrepareMu.Lock()
	defer steamCMDPrepareMu.Unlock()

	if _, ready, _, _, _ := getSteamCMDState(); ready {
		report(5, 5, "SteamCMD 已就绪")
		return nil
	}

	if runtime.GOOS == "linux" {
		if err := installSteamCMDDependencies(func(step, total int, stepName string) {
			report(step, 5, stepName)
		}); err != nil {
			return err
		}
	} else {
		report(3, 5, "当前系统不需要 32 位 Linux 依赖")
	}

	report(4, 5, "下载并初始化 SteamCMD")
	if err := installSteamCMD(); err != nil {
		return err
	}

	if _, ready, _, _, stateMessage := getSteamCMDState(); !ready {
		return fmt.Errorf("SteamCMD 初始化后仍未就绪: %s", stateMessage)
	}

	report(5, 5, "SteamCMD 和 Workshop 组件已准备完成")
	return nil
}

func steamCMDDependenciesInstalled() bool {
	if runtime.GOOS != "linux" {
		return true
	}
	for _, pkg := range []string{"lib32gcc-s1", "lib32stdc++6"} {
		output, err := exec.Command("dpkg-query", "-W", "-f=${Status}", pkg).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "install ok installed") {
			return false
		}
	}
	return true
}

func installSteamCMDDependencies(report func(step, total int, stepName string)) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if steamCMDDependenciesInstalled() {
		report(3, 3, "32 位运行库已安装")
		return nil
	}

	type step struct {
		name string
		args []string
	}
	steps := []step{
		{name: "添加 i386 架构", args: []string{"dpkg", "--add-architecture", "i386"}},
		{name: "更新软件包列表", args: []string{"apt-get", "update", "-y"}},
		{name: "安装 SteamCMD 32 位运行库", args: []string{"apt-get", "install", "-y", "lib32gcc-s1", "lib32stdc++6"}},
	}

	for index, current := range steps {
		report(index+1, len(steps), current.name)
		if output, err := runSteamCMDCommandWithRetry(current.name, current.args...); err != nil {
			return fmt.Errorf("%s 失败: %v%s", current.name, err, formatSteamCMDCommandOutput(output))
		}
	}

	if !steamCMDDependenciesInstalled() {
		return fmt.Errorf("32 位运行库安装后仍未检测到 lib32gcc-s1 和 lib32stdc++6")
	}
	return nil
}

func runSteamCMDCommandWithRetry(stepName string, args ...string) (string, error) {
	const maxRetries = 24
	const retryDelay = 5 * time.Second

	var lastOutput string
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		cmd := exec.Command(args[0], args[1:]...)
		if args[0] == "apt-get" {
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
		log.Printf("[SteamCMD] %s 遇到 apt/dpkg 锁，%d 秒后重试（%d/%d）", stepName, int(retryDelay.Seconds()), attempt, maxRetries)
		time.Sleep(retryDelay)
	}
	return lastOutput, lastErr
}

func formatSteamCMDCommandOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	if len(output) > 1000 {
		output = output[len(output)-1000:]
	}
	return "\n" + output
}
