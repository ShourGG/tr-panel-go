package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"terraria-panel/config"
	"terraria-panel/models"

	"github.com/gin-gonic/gin"
)

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
	ready := launcherExists && runtimeExists
	if runtime.GOOS == "windows" {
		ready = launcherExists
	}

	if ready {
		return installed, ready, launcherPath, runtimePath, "SteamCMD 已安装"
	}
	if installed {
		return installed, ready, launcherPath, runtimePath, fmt.Sprintf("SteamCMD 安装不完整，缺少运行文件: %s", runtimePath)
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
		c.JSON(http.StatusOK, models.MessageResponse("非 Linux 系统，无需安装依赖"))
		return
	}
	commands := [][]string{
		{"dpkg", "--add-architecture", "i386"},
		{"apt-get", "update", "-y"},
		{"apt-get", "install", "-y", "lib32gcc-s1", "lib32stdc++6"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(
				fmt.Sprintf("命令 [%s] 执行失败: %v\n%s", args[0], err, string(output)),
			))
			return
		}
	}
	c.JSON(http.StatusOK, models.MessageResponse("32位依赖安装成功，请刷新状态"))
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
	c.JSON(http.StatusOK, models.MessageResponse("SteamCMD 安装成功！现在可以下载创意工坊模组了"))
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
