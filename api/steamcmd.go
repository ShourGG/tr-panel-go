package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"

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
		type step struct {
			name string
			args []string
		}
		steps := []step{
			{"添加 i386 架构", []string{"dpkg", "--add-architecture", "i386"}},
			{"更新软件包列表", []string{"apt-get", "update", "-y"}},
			{"安装 32 位依赖库", []string{"apt-get", "install", "-y", "lib32gcc-s1", "lib32stdc++6"}},
		}
		for i, s := range steps {
			depsState.mu.Lock()
			depsState.Step = i + 1
			depsState.StepName = s.name
			depsState.mu.Unlock()

			log.Printf("[依赖安装] 步骤 %d/%d: %s", i+1, len(steps), s.name)
			cmd := exec.Command(s.args[0], s.args[1:]...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				errMsg := fmt.Sprintf("步骤 [%s] 失败: %v\n%s", s.name, err, string(output))
				log.Printf("[依赖安装] %s", errMsg)
				depsState.mu.Lock()
				depsState.Running = false
				depsState.Done = true
				depsState.Success = false
				depsState.Error = errMsg
				depsState.mu.Unlock()
				return
			}
		}
		log.Printf("[依赖安装] 全部完成")
		depsState.mu.Lock()
		depsState.Running = false
		depsState.Done = true
		depsState.Success = true
		depsState.Step = depsState.Total
		depsState.StepName = "安装完成"
		depsState.Error = ""
		depsState.mu.Unlock()
	}()

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "开始安装依赖...", "running": true})
}

func InstallSteamCMDAPI(c *gin.Context) {
	installed, ready, _, _, _ := getSteamCMDState()
	if ready {
		c.JSON(http.StatusOK, models.MessageResponse("SteamCMD 已安装"))
		return
	}
	if runtime.GOOS == "linux" {
		depCheckCmd := exec.Command("dpkg", "-l", "lib32gcc-s1")
		if err := depCheckCmd.Run(); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse(
				"缺少32位库依赖。请先运行：\nsudo dpkg --add-architecture i386\nsudo apt-get update\nsudo apt-get install lib32gcc-s1 lib32stdc++6",
			))
			return
		}
	}
	if installed {
		log.Printf("检测到 SteamCMD 安装不完整，开始修复...")
	} else {
		log.Printf("开始安装 SteamCMD...")
	}
	if err := installSteamCMD(); err != nil {
		log.Printf("SteamCMD 安装失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(
			fmt.Sprintf("安装失败: %v", err),
		))
		return
	}
	log.Printf("SteamCMD 安装成功")
	c.JSON(http.StatusOK, models.MessageResponse("SteamCMD 安装成功，现在可以下载创意工坊模组了"))
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
