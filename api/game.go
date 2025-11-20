package api
import (
	"archive/tar"
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"terraria-panel/config"
	"terraria-panel/utils"
	"time"
	"github.com/gin-gonic/gin"
)
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
			"vanilla": vanillaInstalled,
			"tmodloader": tmodInstalled,
			"tshock": tshockInstalled,
			"anyInstalled": vanillaInstalled || tmodInstalled || tshockInstalled,
		},
	})
}
func GetGameInstallInfo(c *gin.Context) {
	osType := runtime.GOOS
	vanillaUrl := "https://terraria.org/api/download/pc-dedicated-server/terraria-server-1449.zip"
	vanillaVersion := "1.4.4.9"
	tmodUrl, tmodVersion := getLatestTModLoaderRelease()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"os": osType,
			"vanilla": gin.H{
				"name": "Terraria 原版服务器",
				"version": vanillaVersion,
				"path": filepath.Join(config.ServersDir, "vanilla"),
				"downloadUrl": vanillaUrl,
				"size": "约 40 MB",
				"installed": checkVanillaInstalled(),
			},
			"tmodloader": gin.H{
				"name": "tModLoader 服务器",
				"version": tmodVersion,
				"path": filepath.Join(config.ServersDir, "tModLoader"),
				"downloadUrl": tmodUrl,
				"size": "约 50 MB",
				"installed": checkTModLoaderInstalled(),
			},
			"tshock5": gin.H{
				"name": "TShock 5 稳定版",
				"version": "5.2.4",
				"path": filepath.Join(config.ServersDir, "tshock"),
				"downloadUrl": "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip",
				"size": "约 24 MB",
				"installed": checkTShockInstalled() && !isTShock6(),
				"requiresNet": "6.0",
			},
			"tshock6": gin.H{
				"name": "TShock 6 预览版",
				"version": "6.0.0-pre1",
				"path": filepath.Join(config.ServersDir, "tshock"),
				"downloadUrl": "https://github.com/Pryaxis/TShock/releases/download/v6.0.0-pre1/TShock-6.0.0-pre1-for-Terraria-1.4.4.9-linux-amd64-Release.zip",
				"size": "约 25 MB",
				"installed": checkTShockInstalled() && isTShock6(),
				"requiresNet": "9.0",
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
	fmt.Printf("\n========================================\n")
	fmt.Printf("[安装开始] 游戏类型: %s\n", req.GameType)
	fmt.Printf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("========================================\n\n")
	go installGameServer(req.GameType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("开始安装 %s 服务器，请稍等...", req.GameType),
	})
}
func GetInstallProgress(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"status": "installing",
			"progress": 50,
			"message": "正在下载...",
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
	versionFile := filepath.Join(config.ServersDir, "tshock", ".tshock_version")
	if data, err := os.ReadFile(versionFile); err == nil {
		version := string(data)
		isV6 := version == "6"
		fmt.Printf("[检测] TShock 版本标记: %s (是否为6: %v)\n", version, isV6)
		return isV6
	}
	fmt.Printf("[检测] 未找到版本标记文件，默认为 TShock 5\n")
	return false
}
func sendInstallProgress(gameType string, message string, progress int) {
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
	fmt.Printf("[%s] 错误: %s\n", gameType, message)
}
func installGameServer(gameType string) {
	var downloadUrl string
	var targetDir string
	sendProgress := func(message string, progress int) {
		sendInstallProgress(gameType, message, progress)
	}
	sendError := func(message string) {
		sendInstallError(gameType, message)
	}
	sendProgress("开始准备安装", 0)
	if gameType == "vanilla" {
		downloadUrl = "https://terraria.org/api/download/pc-dedicated-server/terraria-server-1449.zip"
		targetDir = filepath.Join(config.ServersDir, "vanilla")
	} else if gameType == "tmodloader" {
		url, version := getLatestTModLoaderRelease()
		downloadUrl = url
		fmt.Printf("准备安装 tModLoader %s\n", version)
		targetDir = filepath.Join(config.ServersDir, "tModLoader")
	} else if gameType == "tshock5" {
		downloadUrl = "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip"
		fmt.Printf("准备安装 TShock 5.2.4 (稳定版 - .NET 6.0)\n")
		targetDir = filepath.Join(config.ServersDir, "tshock")
	} else if gameType == "tshock6" {
		url, version := getLatestTShock6Release()
		downloadUrl = url
		fmt.Printf("准备安装 TShock %s (预览版 - .NET 9.0)\n", version)
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
	} else if gameType == "tmodloader" {
		sendProgress("配置tModLoader", 90)
		if runtime.GOOS == "linux" {
			startScript := filepath.Join(targetDir, "start-tModLoaderServer.sh")
			os.Chmod(startScript, 0755)
			sendProgress("检查.NET运行时", 95)
			installDotNetIfNeeded(gameType)
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
					sendProgress("警告: .NET 9.0 安装失败，请手动安装", 93)
				}
				versionFile := filepath.Join(targetDir, ".tshock_version")
				os.WriteFile(versionFile, []byte("6"), 0644)
				fmt.Printf("[版本标记] 已创建 TShock 6 版本标记文件\n")
			} else {
				sendProgress("检查并安装 .NET 6.0 运行时", 91)
				if err := installDotNet6(gameType); err != nil {
					fmt.Printf("[.NET安装] 自动安装 .NET 6.0 失败: %v\n", err)
					sendProgress("警告: .NET 6.0 安装失败，请手动安装", 93)
				}
				versionFile := filepath.Join(targetDir, ".tshock_version")
				os.WriteFile(versionFile, []byte("5"), 0644)
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
	fmt.Println("========================================\n")
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
	dpkgCmd := exec.Command("dpkg", "-i", "/tmp/packages-microsoft-prod.deb")
	if output, err := dpkgCmd.CombinedOutput(); err != nil {
		fmt.Printf("[.NET安装] dpkg 警告: %v\n%s\n", err, string(output))
	}
	os.Remove("/tmp/packages-microsoft-prod.deb")
	sendInstallProgress(gameType, "更新包列表...", 94)
	fmt.Println("[.NET安装] 更新包列表...")
	updateCmd := exec.Command("apt-get", "update", "-qq")
	if output, err := updateCmd.CombinedOutput(); err != nil {
		fmt.Printf("[.NET安装] apt-get update 警告: %v\n%s\n", err, string(output))
	}
	sendInstallProgress(gameType, "安装 .NET 8.0 运行时（可能需要几分钟）...", 95)
	fmt.Println("[.NET安装] 安装 .NET 8.0 运行时...")
	installCmd := exec.Command("apt-get", "install", "-y", "dotnet-runtime-8.0")
	installCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	output, err := installCmd.CombinedOutput()
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
		if (name == "tmodloader.zip" || 
		    strings.Contains(name, "tmodloader") && strings.HasSuffix(name, ".zip") ||
		    strings.Contains(name, "linux") && strings.HasSuffix(name, ".zip")) {
			version := strings.TrimPrefix(release.TagName, "v")
			fmt.Printf("获取到 tModLoader 最新版本: %s (%s)\n", version, asset.Name)
			return asset.BrowserDownloadURL, version
		}
	}
	fmt.Printf("未找到合适的tModLoader文件，使用默认值\n")
	return "https://github.com/tModLoader/tModLoader/releases/download/v2025.08.3.1/tModLoader.zip", "2025.08.3.1"
}
func getLatestTShockRelease() (string, string) {
	apiUrl := "https://api.github.com/repos/Pryaxis/TShock/releases/latest"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		return "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip", "5.2.4"
	}
	req.Header.Set("User-Agent", "Terraria-Panel")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求GitHub API失败: %v\n", err)
		return "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip", "5.2.4"
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
		return "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip", "5.2.4"
	}
	fmt.Printf("TShock版本 %s 包含的文件:\n", release.TagName)
	for _, asset := range release.Assets {
		fmt.Printf("  - %s\n", asset.Name)
	}
	var candidates []struct {
		priority int
		asset    struct {
			Name               string
			BrowserDownloadURL string
		}
	}
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "source") {
			continue
		}
		if strings.Contains(name, "linux") {
			priority := 3
			if strings.Contains(name, "amd64") || strings.Contains(name, "x64") {
				priority = 1
			} else if strings.Contains(name, "arm64") {
				priority = 2
			}
			if strings.HasSuffix(name, ".zip") || 
			   strings.HasSuffix(name, ".tar") || 
			   strings.HasSuffix(name, ".tar.gz") || 
			   strings.HasSuffix(name, ".tgz") {
				candidates = append(candidates, struct {
					priority int
					asset    struct {
						Name               string
						BrowserDownloadURL string
					}
				}{priority: priority, asset: struct {
					Name               string
					BrowserDownloadURL string
				}{
					Name:               asset.Name,
					BrowserDownloadURL: asset.BrowserDownloadURL,
				}})
			}
		}
	}
	if len(candidates) > 0 {
		best := candidates[0]
		for _, c := range candidates[1:] {
			if c.priority < best.priority {
				best = c
			}
		}
		version := strings.TrimPrefix(release.TagName, "v")
		fmt.Printf("获取到 TShock 最新版本: %s (%s)\n", version, best.asset.Name)
		return best.asset.BrowserDownloadURL, version
	}
	fmt.Printf("未找到合适的TShock Linux版本，使用默认值\n")
	return "https://github.com/Pryaxis/TShock/releases/download/v5.2.4/TShock-5.2.4-for-Terraria-1.4.4.9-linux-amd64-Release.zip", "5.2.4"
}
func getLatestTShock6Release() (string, string) {
	apiUrl := "https://api.github.com/repos/Pryaxis/TShock/releases"
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		return "https://github.com/Pryaxis/TShock/releases/download/v6.0.0-pre1/TShock-Beta-linux-x64-Release.zip", "6.0.0-pre1"
	}
	req.Header.Set("User-Agent", "Terraria-Panel")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求GitHub API失败: %v\n", err)
		return "https://github.com/Pryaxis/TShock/releases/download/v6.0.0-pre1/TShock-Beta-linux-x64-Release.zip", "6.0.0-pre1"
	}
	defer resp.Body.Close()
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		fmt.Printf("解析GitHub响应失败: %v\n", err)
		return "https://github.com/Pryaxis/TShock/releases/download/v6.0.0-pre1/TShock-Beta-linux-x64-Release.zip", "6.0.0-pre1"
	}
	for _, release := range releases {
		if !strings.HasPrefix(release.TagName, "v6.0.0-pre") {
			continue
		}
		fmt.Printf("[TShock 6] 检查版本: %s (预览版: %v)\n", release.TagName, release.Prerelease)
		fmt.Printf("[TShock 6] 包含的文件:\n")
		for _, asset := range release.Assets {
			fmt.Printf("  - %s\n", asset.Name)
		}
		for _, asset := range release.Assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, "beta") && 
			   strings.Contains(name, "linux") && 
			   strings.Contains(name, "x64") &&
			   strings.HasSuffix(name, ".zip") {
				version := strings.TrimPrefix(release.TagName, "v")
				fmt.Printf("[TShock 6] ✓ 找到最新版本: %s (%s)\n", version, asset.Name)
				return asset.BrowserDownloadURL, version
			}
		}
	}
	fmt.Printf("[TShock 6] 未找到更新版本，使用默认 pre1\n")
	return "https://github.com/Pryaxis/TShock/releases/download/v6.0.0-pre1/TShock-Beta-linux-x64-Release.zip", "6.0.0-pre1"
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
	fmt.Println("[.NET 6.0] 开始检测...")
	cmd := exec.Command("dotnet", "--list-runtimes")
	output, err := cmd.CombinedOutput()
	if err == nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "Microsoft.NETCore.App 6.0") {
			fmt.Println("[.NET 6.0] ✓ 已安装，跳过")
			sendInstallProgress(gameType, "✓ 已安装 .NET 6.0", 93)
			return nil
		}
	}
	fmt.Println("[.NET 6.0] 未检测到，开始自动安装...")
	sendInstallProgress(gameType, "开始安装 .NET 6.0...", 91)
	if _, err := os.Stat("/etc/debian_version"); err != nil {
		return fmt.Errorf("不支持的Linux发行版（仅支持 Debian/Ubuntu）")
	}
	downloadCmd := exec.Command("wget", "-q",
		"https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb",
		"-O", "/tmp/packages-microsoft-prod.deb")
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("下载 Microsoft 包配置失败: %v", err)
	}
	dpkgCmd := exec.Command("dpkg", "-i", "/tmp/packages-microsoft-prod.deb")
	dpkgCmd.Run()
	os.Remove("/tmp/packages-microsoft-prod.deb")
	updateCmd := exec.Command("apt-get", "update", "-qq")
	updateCmd.Run()
	sendInstallProgress(gameType, "正在安装 .NET 6.0 Runtime...", 92)
	installCmd := exec.Command("apt-get", "install", "-y", "dotnet-runtime-6.0")
	installCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	output, err = installCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[.NET 6.0] 安装失败: %v\n%s\n", err, string(output))
		return fmt.Errorf(".NET 6.0 安装失败: %v", err)
	}
	fmt.Println("[.NET 6.0] ✓ 安装成功")
	sendInstallProgress(gameType, "✓ .NET 6.0 安装成功", 93)
	return nil
}
func installDotNet9(gameType string) error {
	fmt.Println("[.NET 9.0] 开始检测...")
	cmd := exec.Command("dotnet", "--list-runtimes")
	output, err := cmd.CombinedOutput()
	if err == nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "Microsoft.NETCore.App 9.0") {
			fmt.Println("[.NET 9.0] ✓ 已安装，跳过")
			sendInstallProgress(gameType, "✓ 已安装 .NET 9.0", 93)
			return nil
		}
	}
	fmt.Println("[.NET 9.0] 未检测到，开始自动安装...")
	sendInstallProgress(gameType, "开始安装 .NET 9.0...", 91)
	if _, err := os.Stat("/etc/debian_version"); err != nil {
		return fmt.Errorf("不支持的Linux发行版（仅支持 Debian/Ubuntu）")
	}
	downloadCmd := exec.Command("wget", "-q",
		"https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb",
		"-O", "/tmp/packages-microsoft-prod.deb")
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("下载 Microsoft 包配置失败: %v", err)
	}
	dpkgCmd := exec.Command("dpkg", "-i", "/tmp/packages-microsoft-prod.deb")
	dpkgCmd.Run()
	os.Remove("/tmp/packages-microsoft-prod.deb")
	updateCmd := exec.Command("apt-get", "update", "-qq")
	updateCmd.Run()
	sendInstallProgress(gameType, "正在安装 .NET 9.0 Runtime...", 92)
	installCmd := exec.Command("apt-get", "install", "-y", "dotnet-runtime-9.0")
	installCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	output, err = installCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[.NET 9.0] 安装失败: %v\n%s\n", err, string(output))
		return fmt.Errorf(".NET 9.0 安装失败: %v", err)
	}
	fmt.Println("[.NET 9.0] ✓ 安装成功")
	sendInstallProgress(gameType, "✓ .NET 9.0 安装成功", 93)
	return nil
}
