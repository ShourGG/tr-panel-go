package api
import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"github.com/gin-gonic/gin"
)
func DetectTShockVersion(c *gin.Context) {
	tshockPath := filepath.Join(config.ServersDir, "tshock")
	dllPath := filepath.Join(tshockPath, "TShock.dll")
	if _, err := os.Stat(dllPath); err == nil {
		versionFilePath := filepath.Join(tshockPath, "TShock.version.txt")
		if versionData, err := os.ReadFile(versionFilePath); err == nil {
			versionStr := string(versionData)
			if strings.HasPrefix(versionStr, "6.") {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data": gin.H{
						"version":  "6",
						"detected": true,
						"message":  "检测到TShock 6（通过版本文件）",
					},
				})
				return
			} else if strings.HasPrefix(versionStr, "5.") {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data": gin.H{
						"version":  "5",
						"detected": true,
						"message":  "检测到TShock 5（通过版本文件）",
					},
				})
				return
			}
		}
	}
	compatPath := filepath.Join(tshockPath, "TShock.Compatibility.dll")
	if _, err := os.Stat(compatPath); err == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"version":  "6",
				"detected": true,
				"message":  "检测到TShock 6（通过特征文件）",
			},
		})
		return
	}
	configTemplatePath := filepath.Join(tshockPath, "config.json.template")
	if data, err := os.ReadFile(configTemplatePath); err == nil {
		var templateConfig map[string]interface{}
		if json.Unmarshal(data, &templateConfig) == nil {
			if settings, ok := templateConfig["Settings"].(map[string]interface{}); ok {
				if _, hasChinese := settings["服务器端口"]; hasChinese {
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"data": gin.H{
							"version":  "6",
							"detected": true,
							"message":  "检测到TShock 6（中文配置模板）",
						},
					})
					return
				}
				if _, hasEnglish := settings["ServerPort"]; hasEnglish {
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"data": gin.H{
							"version":  "5",
							"detected": true,
							"message":  "检测到TShock 5（英文配置模板）",
						},
					})
					return
				}
			}
		}
	}
	configPath := filepath.Join(tshockPath, "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var config map[string]interface{}
		if json.Unmarshal(data, &config) == nil {
			if settings, ok := config["Settings"].(map[string]interface{}); ok {
				if _, hasChinese := settings["服务器端口"]; hasChinese {
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"data": gin.H{
							"version":  "6",
							"detected": true,
							"message":  "检测到TShock 6配置文件（可能不准确，建议检查TShock程序版本）",
						},
					})
					return
				}
				if _, hasEnglish := settings["ServerPort"]; hasEnglish {
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"data": gin.H{
							"version":  "5",
							"detected": true,
							"message":  "检测到TShock 5配置文件（可能不准确，建议检查TShock程序版本）",
						},
					})
					return
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version":  "5",
			"detected": false,
			"message":  "无法检测版本，默认使用TShock 5配置",
		},
	})
}
