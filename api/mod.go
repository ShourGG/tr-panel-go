package api
import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"
	"github.com/gin-gonic/gin"
)
const (
	STEAM_API_KEY    = "0CC4D444D75574B25716B13C2C95258B"
	TMODLOADER_APPID = "1281930"
)
var (
	downloadingMods  = make(map[string]bool)
	downloadingMutex sync.RWMutex
)
func BroadcastModProgress(workshopID, message string) {
	log.Printf("[MOD进度] WorkshopID=%s, 消息=%s", workshopID, message)
	progressData := map[string]interface{}{
		"type":       "mod_progress",
		"workshopId": workshopID,
		"message":    message,
	}
	jsonData, err := json.Marshal(progressData)
	if err == nil {
		BroadcastMessage(jsonData)
	}
}
type ModMappingData struct {
	ModName    string `json:"modName"`
	PreviewURL string `json:"previewUrl,omitempty"`
}
type ModFileInfo struct {
	Path    string
	Size    int64
	Version string
}
func saveWorkshopMapping(workshopID, modName, previewURL string) {
	mappingFile := filepath.Join(config.DataDir, "tModLoader", "workshop_mapping.json")
	mapping := make(map[string]ModMappingData)
	if data, err := os.ReadFile(mappingFile); err == nil {
		if err := json.Unmarshal(data, &mapping); err != nil {
			oldMapping := make(map[string]string)
			if err := json.Unmarshal(data, &oldMapping); err == nil {
				for k, v := range oldMapping {
					mapping[k] = ModMappingData{ModName: v}
				}
			}
		}
	}
	mapping[workshopID] = ModMappingData{
		ModName:    modName,
		PreviewURL: previewURL,
	}
	data, _ := json.MarshalIndent(mapping, "", "  ")
	os.WriteFile(mappingFile, data, 0644)
	log.Printf("保存映射: WorkshopID=%s → ModName=%s", workshopID, modName)
}
func loadWorkshopMapping() map[string]ModMappingData {
	mappingFile := filepath.Join(config.DataDir, "tModLoader", "workshop_mapping.json")
	mapping := make(map[string]ModMappingData)
	if data, err := os.ReadFile(mappingFile); err == nil {
		if err := json.Unmarshal(data, &mapping); err != nil {
			oldMapping := make(map[string]string)
			if err := json.Unmarshal(data, &oldMapping); err == nil {
				for k, v := range oldMapping {
					mapping[k] = ModMappingData{ModName: v}
				}
			}
		}
	}
	return mapping
}
type SteamWorkshopItem struct {
	PublishedFileID string   `json:"publishedfileid"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	FileSize        int64    `json:"file_size"`
	PreviewURL      string   `json:"preview_url"`
	TimeCreated     int64    `json:"time_created"`
	TimeUpdated     int64    `json:"time_updated"`
	Subscriptions   int      `json:"subscriptions"`
	Tags            []string `json:"tags"`
}
func GetMods(c *gin.Context) {
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	enabledFile := filepath.Join(modDir, "enabled.json")
	var enabledMods []string
	if data, err := os.ReadFile(enabledFile); err == nil {
		json.Unmarshal(data, &enabledMods)
	}
	workshopMapping := loadWorkshopMapping()
	reverseMapping := make(map[string]ModMappingData)
	for wid, data := range workshopMapping {
		reverseMapping[data.ModName] = ModMappingData{
			ModName:    data.ModName,
			PreviewURL: data.PreviewURL,
		}
		if _, ok := reverseMapping[data.ModName]; !ok {
			reverseMapping[data.ModName] = data
		}
		reverseMapping[data.ModName] = ModMappingData{
			ModName:    data.ModName,
			PreviewURL: data.PreviewURL,
		}
		tempData := reverseMapping[data.ModName]
		tempData.ModName = wid
		reverseMapping[data.ModName] = tempData
	}
	installedMods := []gin.H{}
	if files, err := os.ReadDir(modDir); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".tmod") {
				modName := strings.TrimSuffix(file.Name(), ".tmod")
				info, _ := file.Info()
				enabled := false
				for _, enabledMod := range enabledMods {
					if enabledMod == modName {
						enabled = true
						break
					}
				}
				var workshopId string
				var previewUrl string
				for wid, data := range workshopMapping {
					if data.ModName == modName {
						workshopId = wid
						previewUrl = data.PreviewURL
						break
					}
				}
				modItem := gin.H{
					"name":       modName,
					"fileName":   file.Name(),
					"enabled":    enabled,
					"size":       info.Size(),
					"workshopId": workshopId,
				}
				if previewUrl != "" {
					modItem["preview_url"] = previewUrl
				}
				installedMods = append(installedMods, modItem)
			}
		}
	}
	c.JSON(http.StatusOK, models.SuccessResponse(installedMods))
}
func SearchWorkshopMods(c *gin.Context) {
	query := c.Query("query")
	searchText := c.Query("searchText")
	if searchText != "" {
		query = searchText
	}
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("pageSize", "20")
	sortBy := c.DefaultQuery("sortBy", "trend_days")
	var queryType string
	switch sortBy {
	case "total_subscriptions":
		queryType = "13"
	case "playtime_stats":
		queryType = "14"
	case "trend_days":
		fallthrough
	default:
		queryType = "3"
	}
	var url string
	if query == "" {
		url = fmt.Sprintf(
			"https://api.steampowered.com/IPublishedFileService/QueryFiles/v1/?key=%s&query_type=%s&page=%s&numperpage=%s&appid=%s&return_tags=true&return_vote_data=true&return_previews=true&return_short_description=true",
			STEAM_API_KEY, queryType, page, pageSize, TMODLOADER_APPID,
		)
	} else {
		url = fmt.Sprintf(
			"https://api.steampowered.com/IPublishedFileService/QueryFiles/v1/?key=%s&query_type=%s&page=%s&numperpage=%s&appid=%s&search_text=%s&return_tags=true&return_vote_data=true&return_previews=true&return_short_description=true",
			STEAM_API_KEY, queryType, page, pageSize, TMODLOADER_APPID, query,
		)
	}
	log.Printf("🔍 Steam API请求: sortBy=%s, query=%s, page=%s, pageSize=%s", sortBy, query, page, pageSize)
	resp, err := http.Get(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Steam API 请求失败"))
		return
	}
	defer resp.Body.Close()
	var result struct {
		Response struct {
			Total          int `json:"total"`
			PublishedFiles []struct {
				PublishedFileID string `json:"publishedfileid"`
				Title           string `json:"title"`
				Description     string `json:"short_description"`
				FileSize        string `json:"file_size"`
				PreviewURL      string `json:"preview_url"`
				Subscriptions   int    `json:"subscriptions"`
				Favorited       int    `json:"favorited"`
				Views           int    `json:"views"`
				VoteData        struct {
					Score     float64 `json:"score"`
					VotesUp   int     `json:"votes_up"`
					VotesDown int     `json:"votes_down"`
				} `json:"vote_data"`
				Tags []struct {
					Tag string `json:"tag"`
				} `json:"tags"`
				TimeCreated int64 `json:"time_created"`
				TimeUpdated int64 `json:"time_updated"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("解析 Steam API 响应失败"))
		return
	}
	items := []gin.H{}
	for _, file := range result.Response.PublishedFiles {
		tags := []string{}
		for _, tag := range file.Tags {
			tags = append(tags, tag.Tag)
		}
		modType := "unknown"
		modTypeDisplay := "未知"
		tagsLower := make([]string, len(tags))
		for i, tag := range tags {
			tagsLower[i] = strings.ToLower(tag)
		}
		for _, tag := range tagsLower {
			if strings.Contains(tag, "server") && (strings.Contains(tag, "side") || strings.Contains(tag, "only")) {
				modType = "server"
				modTypeDisplay = "服务端"
				break
			} else if strings.Contains(tag, "client") && (strings.Contains(tag, "side") || strings.Contains(tag, "only")) {
				modType = "client"
				modTypeDisplay = "客户端"
				break
			} else if strings.Contains(tag, "both") {
				modType = "both"
				modTypeDisplay = "双端"
				break
			}
		}
		if modType == "unknown" {
			for _, tag := range tagsLower {
				if strings.Contains(tag, "ui") || strings.Contains(tag, "quality of life") ||
					strings.Contains(tag, "qol") || strings.Contains(tag, "visual") ||
					strings.Contains(tag, "cosmetic") || strings.Contains(tag, "minimap") {
					modType = "client"
					modTypeDisplay = "客户端（推测）"
					break
				}
				if strings.Contains(tag, "gameplay") || strings.Contains(tag, "content") ||
					strings.Contains(tag, "boss") || strings.Contains(tag, "weapon") ||
					strings.Contains(tag, "item") || strings.Contains(tag, "npc") {
					modType = "both"
					modTypeDisplay = "双端（推测）"
					break
				}
			}
		}
		item := gin.H{
			"publishedfileid":  file.PublishedFileID,
			"title":            file.Title,
			"description":      file.Description,
			"file_size":        file.FileSize,
			"preview_url":      file.PreviewURL,
			"subscriptions":    file.Subscriptions,
			"favorited":        file.Favorited,
			"views":            file.Views,
			"time_created":     file.TimeCreated,
			"time_updated":     file.TimeUpdated,
			"tags":             tags,
			"mod_type":         modType,
			"mod_type_display": modTypeDisplay,
		}
		if file.VoteData.Score > 0 {
			item["score"] = file.VoteData.Score
			item["votes_up"] = file.VoteData.VotesUp
			item["votes_down"] = file.VoteData.VotesDown
		} else {
			item["score"] = 0.95
			item["votes_up"] = 0
			item["votes_down"] = 0
		}
		items = append(items, item)
	}
	actualTotal := result.Response.Total
	if len(items) == 0 && actualTotal > 0 {
		pageSizeInt := 20
		if ps, err := c.GetQuery("pageSize"); err && ps != "" {
			fmt.Sscanf(ps, "%d", &pageSizeInt)
		}
		pageInt := 1
		if p, err := c.GetQuery("page"); err && p != "" {
			fmt.Sscanf(p, "%d", &pageInt)
		}
		actualTotal = (pageInt - 1) * pageSizeInt
		log.Printf("⚠️ 第%d页无数据，限制总数为: %d", pageInt, actualTotal)
	}
	if actualTotal > 10000 {
		actualTotal = 10000
		log.Printf("⚠️ 总数超过10000，限制为: 10000")
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"total": actualTotal,
		"items": items,
	}))
}
func GetDownloadingMods(c *gin.Context) {
	downloadingMutex.RLock()
	defer downloadingMutex.RUnlock()
	list := []string{}
	for workshopID := range downloadingMods {
		list = append(list, workshopID)
	}
	log.Printf("📋 查询下载状态: 当前 %d 个MOD正在下载 %v", len(list), list)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"downloading": list,
	}))
}
func InstallMod(c *gin.Context) {
	var req struct {
		WorkshopID string `json:"workshopId" binding:"required"`
		Name       string `json:"name"`
		PreviewURL string `json:"previewUrl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	log.Printf("📥 接收下载请求: WorkshopID=%s, Name=%s, PreviewURL=%s", req.WorkshopID, req.Name, req.PreviewURL)
	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("开始下载 MOD: %s", req.Name)))
	go func() {
		downloadingMutex.Lock()
		downloadingMods[req.WorkshopID] = true
		downloadingMutex.Unlock()
		log.Printf("📝 已添加到下载列表: %s (当前下载数: %d)", req.WorkshopID, len(downloadingMods))
		defer func() {
			downloadingMutex.Lock()
			delete(downloadingMods, req.WorkshopID)
			downloadingMutex.Unlock()
			log.Printf("✅ 从下载列表中移除: %s", req.WorkshopID)
		}()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ 完蛋，panic了: %v", r)
				debug.PrintStack()
				BroadcastModProgress(req.WorkshopID, "下载失败")
			}
		}()
		modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
		os.MkdirAll(modDir, 0755)
		log.Printf("开始下载 MOD: %s (Workshop ID: %s)", req.Name, req.WorkshopID)
		steamcmdPath := filepath.Join(config.DataDir, "steamcmd", "steamcmd.sh")
		if runtime.GOOS == "windows" {
			steamcmdPath = filepath.Join(config.DataDir, "steamcmd", "steamcmd.exe")
		}
		if _, err := os.Stat(steamcmdPath); os.IsNotExist(err) {
			log.Printf("SteamCMD不存在，先安装...")
			if err := installSteamCMD(); err != nil {
				errMsg := fmt.Sprintf("SteamCMD安装失败: %v", err)
				if runtime.GOOS == "linux" {
					errMsg += "\n\n请手动安装依赖：\nsudo dpkg --add-architecture i386\nsudo apt-get update\nsudo apt-get install lib32gcc-s1 lib32stdc++6"
				}
				log.Printf("❌ %s", errMsg)
				BroadcastModProgress(req.WorkshopID, "下载失败: "+errMsg)
				return
			}
		}
		if runtime.GOOS == "linux" {
			depCheckCmd := exec.Command("dpkg", "-l", "lib32gcc-s1")
			if err := depCheckCmd.Run(); err != nil {
				errMsg := "Fuck，缺32位库。运行这个：\nsudo dpkg --add-architecture i386\nsudo apt-get update\nsudo apt-get install lib32gcc-s1 lib32stdc++6"
				log.Printf("❌ %s", errMsg)
				BroadcastModProgress(req.WorkshopID, "下载失败: "+errMsg)
				return
			}
		}
		workshopDirs := []string{
			filepath.Join(config.DataDir, "steamcmd", "steamapps", "workshop", "content", "1281930", req.WorkshopID),
			filepath.Join("/root/Steam/steamapps/workshop/content/1281930", req.WorkshopID),
			filepath.Join(os.Getenv("HOME"), "Steam/steamapps/workshop/content/1281930", req.WorkshopID),
			filepath.Join(os.Getenv("HOME"), ".steam/steam/steamapps/workshop/content/1281930", req.WorkshopID),
		}
		cmd := exec.Command(steamcmdPath,
			"+@ShutdownOnFailedCommand", "1",
			"+@NoPromptForPassword", "1",
			"+login", "anonymous",
			"+workshop_download_item", "1281930", req.WorkshopID,
			"+quit",
		)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("❌ 创建输出管道失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败")
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Printf("❌ 创建错误管道失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败")
			return
		}
		if err := cmd.Start(); err != nil {
			log.Printf("❌ 启动 SteamCMD 失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败")
			return
		}
		log.Printf("🚀 开始下载 MOD (WorkshopID: %s)", req.WorkshopID)
		BroadcastModProgress(req.WorkshopID, "Downloading")
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				log.Printf("SteamCMD: %s", line)
				if strings.Contains(line, "Downloading item") {
					log.Printf("📥 SteamCMD开始下载Workshop ID: %s", req.WorkshopID)
					BroadcastModProgress(req.WorkshopID, "Downloading")
				}
				if strings.Contains(line, "%") {
					BroadcastModProgress(req.WorkshopID, line)
				}
				if strings.Contains(line, "Success") {
					BroadcastModProgress(req.WorkshopID, "下载完成，正在安装...")
				}
			}
		}()
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				log.Printf("SteamCMD (stderr): %s", line)
			}
		}()
		log.Printf("⏳ 等待 SteamCMD 完成...")
		if err := cmd.Wait(); err != nil {
			log.Printf("❌ SteamCMD 执行失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败")
			return
		}
		log.Printf("✅ SteamCMD 命令执行完成")
		log.Printf("🔍 开始查找下载的MOD文件...")
		foundMod := false
		for _, workshopDir := range workshopDirs {
			if _, err := os.Stat(workshopDir); err == nil {
				log.Printf("在目录中查找MOD: %s", workshopDir)
				var tmodFiles []ModFileInfo
				filepath.Walk(workshopDir, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil
					}
					if !info.IsDir() && strings.HasSuffix(info.Name(), ".tmod") {
						version := extractVersionFromPath(path)
						tmodFiles = append(tmodFiles, ModFileInfo{
							Path:    path,
							Size:    info.Size(),
							Version: version,
						})
						log.Printf("  找到文件: %s (版本: %s, 大小: %d bytes)", path, version, info.Size())
					}
					return nil
				})
				log.Printf("共找到 %d 个 .tmod 文件", len(tmodFiles))
				if len(tmodFiles) == 0 {
					continue
				}
				selectedFile := selectLatestModFile(tmodFiles)
				log.Printf("🎯 选择最新版本: %s (版本: %s, 大小: %d bytes)",
					selectedFile.Path, selectedFile.Version, selectedFile.Size)
				srcPath := selectedFile.Path
				fileName := filepath.Base(srcPath)
				dstPath := filepath.Join(modDir, fileName)
				log.Printf("准备复制: %s → %s", srcPath, dstPath)
				fileInfo, err := os.Stat(srcPath)
				if err != nil {
					log.Printf("❌ 获取文件信息失败: %v", err)
					continue
				}
				log.Printf("📦 文件大小: %d bytes (%.2f MB)", fileInfo.Size(), float64(fileInfo.Size())/1024/1024)
				if err := copyModFile(srcPath, dstPath); err != nil {
					log.Printf("❌ 复制MOD文件失败: %v", err)
					continue
				}
				log.Printf("✅ 复制成功")
				foundMod = true
			modName := extractModName(fileName)
			log.Printf("📝 提取的模组名称: %s (原文件名: %s)", modName, fileName)
			log.Printf("启用MOD: %s", modName)
			if err := enableModByName(modName); err != nil {
				log.Printf("⚠️ 启用MOD失败: %v", err)
			}
			fileNameWithoutExt := strings.TrimSuffix(fileName, ".tmod")
			log.Printf("💾 保存映射: WorkshopID=%s → FileName=%s, ModName=%s, PreviewURL=%s",
				req.WorkshopID, fileNameWithoutExt, modName, req.PreviewURL)
			saveWorkshopMapping(req.WorkshopID, fileNameWithoutExt, req.PreviewURL)
			log.Printf("✅ MOD %s 下载并安装成功 (文件: %s, 模组名: %s, WorkshopID: %s)",
				req.Name, fileName, modName, req.WorkshopID)
			BroadcastModProgress(req.WorkshopID, "Downloaded")
			return
		}
		if foundMod {
			break
		}
	}
		if !foundMod {
			log.Printf("Workshop 下载目录未找到 MOD，尝试从本地 Steam 目录查找...")
			workshopDirs := []string{
				"C:/Program Files (x86)/Steam/steamapps/workshop/content/1281930",
				"D:/Steam/steamapps/workshop/content/1281930",
				"E:/Steam/steamapps/workshop/content/1281930",
			}
			for _, workshopDir := range workshopDirs {
				modSourceDir := filepath.Join(workshopDir, req.WorkshopID)
				if _, err := os.Stat(modSourceDir); err == nil {
					files, err := os.ReadDir(modSourceDir)
					if err == nil {
						for _, file := range files {
							if strings.HasSuffix(file.Name(), ".tmod") {
								srcPath := filepath.Join(modSourceDir, file.Name())
								dstPath := filepath.Join(modDir, file.Name())
								if err := copyModFile(srcPath, dstPath); err == nil {
									foundMod = true
									modName := strings.TrimSuffix(file.Name(), ".tmod")
									enableModByName(modName)
									BroadcastModProgress(req.WorkshopID, "Downloaded")
									log.Printf("✅ MOD %s 安装成功", req.Name)
									return
								}
							}
						}
					}
				}
			}
		}
		if !foundMod {
			log.Printf("❌ 未找到 MOD 文件: %s", req.Name)
			BroadcastModProgress(req.WorkshopID, "下载失败: 未找到 MOD 文件")
			return
		}
	}()
}
func extractModName(fileName string) string {
	name := strings.TrimSuffix(fileName, ".tmod")
	versionPatterns := []string{
		`_v\d+(\.\d+)*$`,
		`_\d+(\.\d+)*$`,
		`-v\d+(\.\d+)*$`,
		`-\d+(\.\d+)*$`,
	}
	for _, pattern := range versionPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(name) {
			name = re.ReplaceAllString(name, "")
			break
		}
	}
	return name
}
func extractVersionFromPath(filePath string) string {
	dir := filepath.Dir(filePath)
	dirName := filepath.Base(dir)
	versionPattern := regexp.MustCompile(`^\d+(\.\d+)+$`)
	if versionPattern.MatchString(dirName) {
		return dirName
	}
	return "unknown"
}
func selectLatestModFile(files []ModFileInfo) ModFileInfo {
	if len(files) == 0 {
		return ModFileInfo{}
	}
	if len(files) == 1 {
		return files[0]
	}
	bestFile := files[0]
	for i := 1; i < len(files); i++ {
		current := files[i]
		if current.Version != "unknown" && bestFile.Version == "unknown" {
			bestFile = current
			continue
		}
		if bestFile.Version != "unknown" && current.Version == "unknown" {
			continue
		}
		if current.Version != "unknown" && bestFile.Version != "unknown" {
			if compareVersions(current.Version, bestFile.Version) > 0 {
				bestFile = current
				continue
			} else if compareVersions(current.Version, bestFile.Version) == 0 {
				if current.Size > bestFile.Size {
					bestFile = current
				}
				continue
			}
		}
		if current.Version == "unknown" && bestFile.Version == "unknown" {
			if current.Size > bestFile.Size {
				bestFile = current
			}
		}
	}
	return bestFile
}
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			num1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			num2, _ = strconv.Atoi(parts2[i])
		}
		if num1 > num2 {
			return 1
		} else if num1 < num2 {
			return -1
		}
	}
	return 0
}
func copyModFile(src, dst string) error {
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
	_, err = destFile.ReadFrom(sourceFile)
	return err
}
func enableModByName(modName string) error {
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	enabledFile := filepath.Join(modDir, "enabled.json")
	var enabledMods []string
	if data, err := os.ReadFile(enabledFile); err == nil {
		json.Unmarshal(data, &enabledMods)
	}
	for _, mod := range enabledMods {
		if mod == modName {
			return nil
		}
	}
	enabledMods = append(enabledMods, modName)
	data, _ := json.MarshalIndent(enabledMods, "", "  ")
	return os.WriteFile(enabledFile, data, 0644)
}
func EnableMod(c *gin.Context) {
	modName := c.Param("name")
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	enabledFile := filepath.Join(modDir, "enabled.json")
	var enabledMods []string
	if data, err := os.ReadFile(enabledFile); err == nil {
		json.Unmarshal(data, &enabledMods)
	}
	for _, mod := range enabledMods {
		if mod == modName {
			c.JSON(http.StatusOK, models.MessageResponse("MOD 已启用"))
			return
		}
	}
	enabledMods = append(enabledMods, modName)
	data, _ := json.MarshalIndent(enabledMods, "", "  ")
	os.WriteFile(enabledFile, data, 0644)
	c.JSON(http.StatusOK, models.MessageResponse("MOD 启用成功"))
}
func DisableMod(c *gin.Context) {
	modName := c.Param("name")
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	enabledFile := filepath.Join(modDir, "enabled.json")
	var enabledMods []string
	if data, err := os.ReadFile(enabledFile); err == nil {
		json.Unmarshal(data, &enabledMods)
	}
	newList := []string{}
	for _, mod := range enabledMods {
		if mod != modName {
			newList = append(newList, mod)
		}
	}
	data, _ := json.MarshalIndent(newList, "", "  ")
	os.WriteFile(enabledFile, data, 0644)
	c.JSON(http.StatusOK, models.MessageResponse("MOD 禁用成功"))
}
func DeleteMod(c *gin.Context) {
	modName := c.Param("name")
	log.Printf("🗑️ 开始删除MOD: %s", modName)
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	enabledFile := filepath.Join(modDir, "enabled.json")
	mappingFile := filepath.Join(modDir, "workshop_mapping.json")
	var deletedFile string
	files, err := os.ReadDir(modDir)
	if err != nil {
		log.Printf("❌ 读取MOD目录失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取MOD目录失败"))
		return
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".tmod") {
			extractedName := extractModName(file.Name())
			if extractedName == modName {
				deletedFile = file.Name()
				break
			}
		}
	}
	if deletedFile == "" {
		log.Printf("⚠️ 未找到MOD文件: %s", modName)
		c.JSON(http.StatusNotFound, models.ErrorResponse("MOD 文件不存在"))
		return
	}
	modFile := filepath.Join(modDir, deletedFile)
	log.Printf("📁 找到MOD文件: %s", deletedFile)
	if err := os.Remove(modFile); err != nil {
		log.Printf("❌ 删除MOD文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除 MOD 文件失败"))
		return
	}
	log.Printf("✅ MOD文件已删除: %s", deletedFile)
	var enabledMods []string
	if data, err := os.ReadFile(enabledFile); err == nil {
		json.Unmarshal(data, &enabledMods)
	}
	newList := []string{}
	for _, mod := range enabledMods {
		if mod != modName && extractModName(mod+".tmod") != modName {
			newList = append(newList, mod)
		}
	}
	if len(newList) != len(enabledMods) {
		data, _ := json.MarshalIndent(newList, "", "  ")
		os.WriteFile(enabledFile, data, 0644)
		log.Printf("✅ 已从 enabled.json 移除: %s", modName)
	}
	if data, err := os.ReadFile(mappingFile); err == nil {
		var mapping map[string]ModMappingData
		if err := json.Unmarshal(data, &mapping); err == nil {
			var workshopIdToDelete string
			for workshopId, modData := range mapping {
				if extractModName(modData.ModName+".tmod") == modName {
					workshopIdToDelete = workshopId
					break
				}
			}
			if workshopIdToDelete != "" {
				delete(mapping, workshopIdToDelete)
				data, _ := json.MarshalIndent(mapping, "", "  ")
				os.WriteFile(mappingFile, data, 0644)
				log.Printf("✅ 已从 workshop_mapping.json 移除: WorkshopID=%s", workshopIdToDelete)
			}
		}
	}
	log.Printf("🎉 MOD删除成功: %s", modName)
	c.JSON(http.StatusOK, models.MessageResponse("MOD 删除成功"))
}
func UploadMod(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("未找到文件"))
		return
	}
	if !strings.HasSuffix(file.Filename, ".tmod") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只能上传 .tmod 文件"))
		return
	}
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	os.MkdirAll(modDir, 0755)
	destPath := filepath.Join(modDir, file.Filename)
	if err := c.SaveUploadedFile(file, destPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存文件失败"))
		return
	}
	modName := strings.TrimSuffix(file.Filename, ".tmod")
	enableModByName(modName)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"message": "MOD 上传成功",
		"name":    modName,
		"size":    file.Size,
	}))
}
func installSteamCMD() error {
	steamcmdDir := filepath.Join(config.DataDir, "steamcmd")
	os.MkdirAll(steamcmdDir, 0755)
	if runtime.GOOS == "linux" {
		steamcmdURL := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"
		tarPath := filepath.Join(steamcmdDir, "steamcmd_linux.tar.gz")
		log.Printf("下载 SteamCMD: %s", steamcmdURL)
		resp, err := http.Get(steamcmdURL)
		if err != nil {
			return fmt.Errorf("下载 SteamCMD 失败: %v", err)
		}
		defer resp.Body.Close()
		out, err := os.Create(tarPath)
		if err != nil {
			return fmt.Errorf("创建文件失败: %v", err)
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return fmt.Errorf("保存文件失败: %v", err)
		}
		log.Printf("解压 SteamCMD...")
		tarFile, err := os.Open(tarPath)
		if err != nil {
			return fmt.Errorf("打开 tar 文件失败: %v", err)
		}
		defer tarFile.Close()
		gzReader, err := gzip.NewReader(tarFile)
		if err != nil {
			return fmt.Errorf("创建 gzip reader 失败: %v", err)
		}
		defer gzReader.Close()
		tarReader := tar.NewReader(gzReader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("读取 tar 失败: %v", err)
			}
			path := filepath.Join(steamcmdDir, header.Name)
			if header.Typeflag == tar.TypeDir {
				os.MkdirAll(path, 0755)
				continue
			}
			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("创建文件失败: %v", err)
			}
			_, err = io.Copy(file, tarReader)
			file.Close()
			if err != nil {
				return fmt.Errorf("解压文件失败: %v", err)
			}
			if strings.HasSuffix(header.Name, ".sh") || header.Name == "steamcmd" {
				os.Chmod(path, 0755)
			}
		}
		os.Remove(tarPath)
		log.Printf("初始化 SteamCMD...")
		steamcmdPath := filepath.Join(steamcmdDir, "steamcmd.sh")
		cmd := exec.Command(steamcmdPath, "+quit")
		cmd.Run()
	} else if runtime.GOOS == "windows" {
		steamcmdURL := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"
		zipPath := filepath.Join(steamcmdDir, "steamcmd.zip")
		log.Printf("下载 SteamCMD: %s", steamcmdURL)
		resp, err := http.Get(steamcmdURL)
		if err != nil {
			return fmt.Errorf("下载 SteamCMD 失败: %v", err)
		}
		defer resp.Body.Close()
		out, err := os.Create(zipPath)
		if err != nil {
			return fmt.Errorf("创建文件失败: %v", err)
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return fmt.Errorf("保存文件失败: %v", err)
		}
		cmd := exec.Command("powershell", "-Command",
			fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", zipPath, steamcmdDir))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("解压失败: %v", err)
		}
		os.Remove(zipPath)
		log.Printf("初始化 SteamCMD...")
		steamcmdPath := filepath.Join(steamcmdDir, "steamcmd.exe")
		cmd = exec.Command(steamcmdPath, "+quit")
		cmd.Run()
	} else {
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
	log.Printf("SteamCMD 安装完成")
	return nil
}
