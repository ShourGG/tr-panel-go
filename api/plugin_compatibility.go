package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"terraria-panel/config"
)

const maxPluginFrameworkInspectionBytes = 16 * 1024 * 1024

type pluginRuntimeDetection struct {
	Major     string
	Evidence  string
	Ambiguous bool
}

func requiredTShockPluginRuntime() (string, string, error) {
	installation := inspectTShockInstallation(filepath.Join(config.ServersDir, "tshock"))
	if !installation.Installed {
		return "", "", fmt.Errorf("无法验证插件兼容性：%s", installation.Message)
	}

	switch installation.Version {
	case "5":
		return "6", "TShock 5（.NET 6）", nil
	case "6":
		return "9", "TShock 6（.NET 9）", nil
	default:
		return "", "", fmt.Errorf("无法验证插件兼容性：当前 TShock 版本未确认")
	}
}

func validateUploadedPluginForCurrentTShock(file *multipart.FileHeader) error {
	reader, err := file.Open()
	if err != nil {
		return fmt.Errorf("读取上传插件失败: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxPluginFrameworkInspectionBytes+1))
	if err != nil {
		return fmt.Errorf("读取上传插件失败: %w", err)
	}
	if len(data) > maxPluginFrameworkInspectionBytes {
		return fmt.Errorf("插件文件过大，无法安全识别目标 .NET 框架")
	}
	return validatePluginBytesForCurrentTShock(data, file.Filename)
}

func validatePluginFileForCurrentTShock(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("读取插件文件失败: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPluginFrameworkInspectionBytes+1))
	if err != nil {
		return fmt.Errorf("读取插件文件失败: %w", err)
	}
	if len(data) > maxPluginFrameworkInspectionBytes {
		return fmt.Errorf("插件 %s 过大，无法安全识别目标 .NET 框架", filepath.Base(path))
	}
	return validatePluginBytesForCurrentTShock(data, filepath.Base(path))
}

func validatePluginBytesForCurrentTShock(data []byte, pluginName string) error {
	requiredMajor, targetLabel, err := requiredTShockPluginRuntime()
	if err != nil {
		return err
	}
	return validatePluginBytesForRuntime(data, pluginName, requiredMajor, targetLabel)
}

func validatePluginBytesForRuntime(data []byte, pluginName, requiredMajor, targetLabel string) error {
	detection := detectPluginRuntime(data)
	if detection.Ambiguous {
		return fmt.Errorf("插件 %s 的 .NET 目标框架信息冲突，%s 不会加载该插件", pluginName, targetLabel)
	}
	if detection.Major == "" {
		return fmt.Errorf("无法识别插件 %s 的目标 .NET 框架；%s 仅允许 .NET %s 插件", pluginName, targetLabel, requiredMajor)
	}
	if detection.Major != requiredMajor {
		return fmt.Errorf("插件 %s 使用 .NET %s（%s），与 %s 不兼容；请使用 .NET %s 版本", pluginName, detection.Major, detection.Evidence, targetLabel, requiredMajor)
	}
	return nil
}

func detectPluginRuntime(data []byte) pluginRuntimeDetection {
	content := strings.ToLower(string(data))
	has6, evidence6 := containsPluginRuntimeMarker(content, "6")
	has9, evidence9 := containsPluginRuntimeMarker(content, "9")
	if has6 && has9 {
		return pluginRuntimeDetection{Ambiguous: true, Evidence: evidence6 + ", " + evidence9}
	}
	if has6 {
		return pluginRuntimeDetection{Major: "6", Evidence: evidence6}
	}
	if has9 {
		return pluginRuntimeDetection{Major: "9", Evidence: evidence9}
	}
	return pluginRuntimeDetection{}
}

func containsPluginRuntimeMarker(content, major string) (bool, string) {
	markers := []string{
		"netcoreapp,version=v" + major + ".0",
		"netcoreapp, version=v" + major + ".0",
		"net" + major + ".0",
		"system.runtime, version=" + major + ".0",
		"system.runtime,version=" + major + ".0",
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true, marker
		}
	}
	return false, ""
}

func validateEnabledTShockPlugins(pluginsDir string) error {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取插件目录失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".dll") {
			continue
		}
		if err := validatePluginFileForCurrentTShock(filepath.Join(pluginsDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
