package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"terraria-panel/config"
)

func CheckTShockInstalled() (bool, string) {
	tshockDir := filepath.Join(config.ServersDir, "tshock")
	var exePath string
	if runtime.GOOS == "windows" {
		exePath = filepath.Join(tshockDir, "TShock.Server.exe")
	} else {
		exePath = filepath.Join(tshockDir, "TShock.Server")
	}
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return false, exePath
	}
	return true, exePath
}
func CheckTShockCorePlugins() (bool, []string) {
	tshockDir := filepath.Join(config.ServersDir, "tshock")
	pluginsDir := filepath.Join(tshockDir, "ServerPlugins")
	corePlugins := []string{
		"TShockAPI.dll",
		"LazyAPI.dll",
		"linq2db.dll",
	}
	missingPlugins := []string{}
	for _, plugin := range corePlugins {
		pluginPath := filepath.Join(pluginsDir, plugin)
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			missingPlugins = append(missingPlugins, plugin)
		}
	}
	if len(missingPlugins) > 0 {
		return false, missingPlugins
	}
	return true, nil
}
func CheckDotNetInstalled() (bool, string) {
	hasNet6, _, err := CheckDotNetRuntime6()
	if err != nil {
		return false, ""
	}
	return hasNet6, "6.0"
}
func CheckDotNetRuntime() (bool, string, error) {
	return CheckDotNetRuntime6()
}
func CheckDotNetRuntime6() (bool, string, error) {
	return CheckDotNetRuntimeVersion("6.0")
}
func CheckDotNetRuntimeVersion(required string) (bool, string, error) {
	required = strings.TrimSpace(required)
	if required == "" {
		required = "6.0"
	}
	cmd := exec.Command("dotnet", "--list-runtimes")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", err
	}
	runtimes := string(output)
	hasRequired := false
	for _, line := range strings.Split(runtimes, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Microsoft.NETCore.App "+required+".") || line == "Microsoft.NETCore.App "+required {
			hasRequired = true
			break
		}
	}
	if !hasRequired && required == "6.0" {
		dpkgCmd := exec.Command("dpkg", "-l", "dotnet-runtime-6.0")
		dpkgOutput, dpkgErr := dpkgCmd.CombinedOutput()
		if dpkgErr == nil && strings.Contains(string(dpkgOutput), "ii") {
			runtimes += "\n[警告] 检测到 Ubuntu 系统包 dotnet-runtime-6.0 已安装，但未注册到 dotnet 命令"
			runtimes += "\n[建议] 请卸载系统包并从 Microsoft 官方仓库重新安装"
			return false, runtimes, nil
		}
	}
	return hasRequired, runtimes, nil
}
func GetInstalledDotNetRuntimes() ([]string, error) {
	cmd := exec.Command("dotnet", "--list-runtimes")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	var runtimes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, "Microsoft.NETCore.App") {
			runtimes = append(runtimes, line)
		}
	}
	return runtimes, nil
}
func DetectLinuxDistro() (string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", err
	}
	content := strings.ToLower(string(data))
	if strings.Contains(content, "ubuntu") {
		return "ubuntu", nil
	} else if strings.Contains(content, "debian") {
		return "debian", nil
	} else if strings.Contains(content, "centos") {
		return "centos", nil
	} else if strings.Contains(content, "rhel") {
		return "rhel", nil
	} else if strings.Contains(content, "fedora") {
		return "fedora", nil
	}
	return "unknown", nil
}
func GetDotNet6InstallCommand() ([]string, error) {
	return GetDotNetInstallCommand("6.0")
}
func GetDotNetInstallCommand(required string) ([]string, error) {
	required = strings.TrimSpace(required)
	if required == "" {
		required = "6.0"
	}
	packageName := "dotnet-runtime-" + required
	downloadURL := "https://dotnet.microsoft.com/download/dotnet/" + required
	distro, err := DetectLinuxDistro()
	if err != nil {
		distro = "unknown"
	}
	switch distro {
	case "ubuntu", "debian":
		return []string{
			"# Ubuntu/Debian 安装 .NET " + required + " Runtime（自动化模式，无交互提示）",
			"sudo apt-get update",
			"sudo DEBIAN_FRONTEND=noninteractive apt-get install -y " + packageName,
			"",
			"# 验证安装",
			"dotnet --list-runtimes",
		}, nil
	case "centos", "rhel":
		return []string{
			"# CentOS/RHEL 安装 .NET " + required + " Runtime",
			"sudo yum install -y " + packageName,
			"",
			"# 验证安装",
			"dotnet --list-runtimes",
		}, nil
	case "fedora":
		return []string{
			"# Fedora 安装 .NET " + required + " Runtime",
			"sudo dnf install -y " + packageName,
			"",
			"# 验证安装",
			"dotnet --list-runtimes",
		}, nil
	default:
		return []string{
			"# 通用安装方法",
			"# 访问：" + downloadURL,
			"# 下载并安装 .NET " + required + " Runtime",
		}, nil
	}
}
func GetTShockVersion() string {
	return "Unknown"
}
