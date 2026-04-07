package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"

	"github.com/gin-gonic/gin"
)

type tshockVersionDetection struct {
	Version    string
	RawVersion string
	Detected   bool
	Message    string
}

func DetectTShockVersion(c *gin.Context) {
	detection := detectInstalledTShockVersion(filepath.Join(config.ServersDir, "tshock"))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version":  detection.Version,
			"detected": detection.Detected,
			"message":  detection.Message,
		},
	})
}

func detectInstalledTShockVersion(tshockPath string) tshockVersionDetection {
	if raw, ok := readTrimmedFile(filepath.Join(tshockPath, ".tshock_version")); ok {
		if version := normalizeTShockMajor(raw); version != "unknown" {
			return tshockVersionDetection{
				Version:    version,
				RawVersion: raw,
				Detected:   true,
				Message:    formatDetectedVersionMessage(version, raw, "版本标记文件 .tshock_version"),
			}
		}
	}

	if raw, ok := readTrimmedFile(filepath.Join(tshockPath, "TShock.version.txt")); ok {
		if version := normalizeTShockMajor(raw); version != "unknown" {
			return tshockVersionDetection{
				Version:    version,
				RawVersion: raw,
				Detected:   true,
				Message:    formatDetectedVersionMessage(version, raw, "官方版本文件 TShock.version.txt"),
			}
		}
	}

	if _, err := os.Stat(filepath.Join(tshockPath, "TShock.Compatibility.dll")); err == nil {
		return tshockVersionDetection{
			Version:    "6",
			RawVersion: "6",
			Detected:   true,
			Message:    "检测到 TShock 6（通过官方兼容组件 TShock.Compatibility.dll）",
		}
	}

	if hasInstalledTShockBinary(tshockPath) {
		return tshockVersionDetection{
			Version:  "unknown",
			Detected: false,
			Message:  "检测到 TShock 程序文件，但缺少官方版本标记，无法可靠区分 TShock 5 / 6",
		}
	}

	return tshockVersionDetection{
		Version:  "unknown",
		Detected: false,
		Message:  "未检测到 TShock 安装或官方版本标记",
	}
}

func normalizeTShockMajor(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")

	switch {
	case raw == "5", strings.HasPrefix(raw, "5."):
		return "5"
	case raw == "6", strings.HasPrefix(raw, "6."):
		return "6"
	default:
		return "unknown"
	}
}

func formatDetectedVersionMessage(version, raw, source string) string {
	if raw == "" {
		return "检测到 TShock " + version + "（通过" + source + "）"
	}

	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")
	if raw == version {
		return "检测到 TShock " + version + "（通过" + source + "）"
	}
	return "检测到 TShock " + version + "（版本 " + raw + "，通过" + source + "）"
}

func readTrimmedFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", false
	}
	return value, true
}

func hasInstalledTShockBinary(tshockPath string) bool {
	candidates := []string{
		"TShock.Server.exe",
		"TShock.Server",
		"TShock.Server.dll",
		"TShock.dll",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(tshockPath, candidate)); err == nil {
			return true
		}
	}

	return false
}
