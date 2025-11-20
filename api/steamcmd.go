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
func CheckSteamCMD(c *gin.Context) {
	steamcmdPath := filepath.Join(config.DataDir, "steamcmd", "steamcmd.sh")
	if runtime.GOOS == "windows" {
		steamcmdPath = filepath.Join(config.DataDir, "steamcmd", "steamcmd.exe")
	}
	if _, err := os.Stat(steamcmdPath); err == nil {
		c.JSON(http.StatusOK, gin.H{
			"installed": true,
			"path":      steamcmdPath,
			"message":   "SteamCMD 已安装",
		})
		return
	}
	if runtime.GOOS == "linux" {
		depCheckCmd := exec.Command("dpkg", "-l", "lib32gcc-s1")
		if err := depCheckCmd.Run(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"installed":       false,
				"deps_missing":    true,
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
		"installed": false,
		"can_install": true,
		"message":   "SteamCMD 未安装，可以自动安装",
	})
}
func InstallSteamCMDAPI(c *gin.Context) {
	steamcmdPath := filepath.Join(config.DataDir, "steamcmd", "steamcmd.sh")
	if runtime.GOOS == "windows" {
		steamcmdPath = filepath.Join(config.DataDir, "steamcmd", "steamcmd.exe")
	}
	if _, err := os.Stat(steamcmdPath); err == nil {
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
	log.Printf("开始安装 SteamCMD...")
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
	steamcmdDir := filepath.Join(config.DataDir, "steamcmd")
	steamcmdPath := filepath.Join(steamcmdDir, "steamcmd.sh")
	if runtime.GOOS == "windows" {
		steamcmdPath = filepath.Join(steamcmdDir, "steamcmd.exe")
	}
	status := gin.H{
		"os": runtime.GOOS,
	}
	if _, err := os.Stat(steamcmdPath); err == nil {
		status["installed"] = true
		status["path"] = steamcmdPath
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
	} else {
		status["installed"] = false
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
