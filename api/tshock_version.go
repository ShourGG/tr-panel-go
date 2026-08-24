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

const tshockInstallCompleteMarker = ".tshock_install_complete"

var tshockRuntimeInstalled = isRequiredDotNetRuntimeInstalled

type tshockInstallationDetection struct {
	State           string
	Version         string
	RawVersion      string
	Installed       bool
	Complete        bool
	RuntimeReady    bool
	VersionDetected bool
	Message         string
}

func DetectTShockVersion(c *gin.Context) {
	detection := detectInstalledTShockVersion(filepath.Join(config.ServersDir, "tshock"))
	installation := inspectTShockInstallation(filepath.Join(config.ServersDir, "tshock"))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version":           detection.Version,
			"detected":          detection.Detected,
			"message":           installation.Message,
			"installationState": installation.State,
			"installed":         installation.Installed,
			"complete":          installation.Complete,
			"runtimeReady":      installation.RuntimeReady,
		},
	})
}

func inspectTShockInstallation(tshockPath string) tshockInstallationDetection {
	info, err := os.Stat(tshockPath)
	if err != nil || !info.IsDir() {
		return tshockInstallationDetection{
			State:   "not-installed",
			Version: "unknown",
			Message: "未检测到 TShock 安装目录",
		}
	}

	if !hasInstalledTShockBinary(tshockPath) {
		return tshockInstallationDetection{
			State:   "incomplete",
			Version: "unknown",
			Message: "检测到 TShock 目录残留，但缺少可启动的 TShock.Server 核心文件。可以重新安装或卸载残留目录。",
		}
	}

	version := detectInstalledTShockVersion(tshockPath)
	if !version.Detected || version.Version == "unknown" {
		return tshockInstallationDetection{
			State:   "unverified",
			Version: "unknown",
			Message: "检测到 TShock 核心文件，但缺少可验证的版本标记，无法判断 TShock 5 / 6。请重新安装或先卸载残留目录。",
		}
	}

	requiredRuntime := getRequiredDotNetRuntime(version.Version)
	runtimeReady := tshockRuntimeInstalled(requiredRuntime)
	if !runtimeReady {
		return tshockInstallationDetection{
			State:           "runtime-missing",
			Version:         version.Version,
			RawVersion:      version.RawVersion,
			RuntimeReady:    false,
			VersionDetected: true,
			Message:         "检测到 " + version.Message + "，但缺少 .NET " + requiredRuntime + " Runtime；安装尚未完成。",
		}
	}

	if tshockInstallCompleteMarkerMatches(tshockPath, version.Version) {
		return tshockInstallationDetection{
			State:           "installed",
			Version:         version.Version,
			RawVersion:      version.RawVersion,
			Installed:       true,
			Complete:        true,
			RuntimeReady:    true,
			VersionDetected: true,
			Message:         version.Message + "，核心文件、版本标记和所需 Runtime 已验证。",
		}
	}

	// Existing installations from earlier panel versions did not have the
	// completion marker. They remain usable once the core, version and runtime
	// are all verified, but the UI can distinguish them from a staged install.
	return tshockInstallationDetection{
		State:           "legacy-installed",
		Version:         version.Version,
		RawVersion:      version.RawVersion,
		Installed:       true,
		RuntimeReady:    true,
		VersionDetected: true,
		Message:         version.Message + "，核心文件和 Runtime 已验证；这是旧版安装，缺少本面板的完成标记。",
	}
}

func tshockInstallCompleteMarkerMatches(tshockPath, expectedMajor string) bool {
	raw, ok := readTrimmedFile(filepath.Join(tshockPath, tshockInstallCompleteMarker))
	return ok && normalizeTShockMajor(raw) == expectedMajor
}

func writeTShockInstallCompleteMarker(tshockPath, version string) error {
	if normalizeTShockMajor(version) == "unknown" {
		return os.ErrInvalid
	}
	return os.WriteFile(filepath.Join(tshockPath, tshockInstallCompleteMarker), []byte(strings.TrimSpace(version)+"\n"), 0644)
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
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(tshockPath, candidate)); err == nil {
			return true
		}
	}

	return false
}
