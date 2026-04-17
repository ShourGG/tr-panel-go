package api

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	STEAM_API_KEY    = "0CC4D444D75574B25716B13C2C95258B"
	TMODLOADER_APPID = "1281930"
)

var (
	downloadingMods     = make(map[string]bool)
	modDownloadProgress = make(map[string]*models.DownloadProgress)
	downloadingMutex    sync.RWMutex
)

// Steam Workshop API 缓存
type workshopCacheEntry struct {
	data     []byte
	cachedAt time.Time
}

type workshopDiskCacheEntry struct {
	CachedAt time.Time       `json:"cachedAt"`
	Data     json.RawMessage `json:"data"`
}

var (
	workshopCache    = make(map[string]*workshopCacheEntry)
	workshopCacheMu  sync.RWMutex
	workshopCacheTTL = 30 * time.Minute
)

func workshopCacheDir() string {
	return filepath.Join(config.DataDir, "cache", "workshop")
}

func workshopCacheFilePath(key string) string {
	hash := sha1.Sum([]byte(key))
	return filepath.Join(workshopCacheDir(), hex.EncodeToString(hash[:])+".json")
}

func getWorkshopCache(key string) ([]byte, bool) {
	workshopCacheMu.RLock()
	entry, ok := workshopCache[key]
	workshopCacheMu.RUnlock()
	if ok && time.Since(entry.cachedAt) <= workshopCacheTTL {
		return append([]byte(nil), entry.data...), true
	}

	cached, ok := getWorkshopDiskCache(key)
	if !ok {
		return nil, false
	}

	workshopCacheMu.Lock()
	workshopCache[key] = &workshopCacheEntry{data: append([]byte(nil), cached...), cachedAt: time.Now()}
	workshopCacheMu.Unlock()
	return cached, true
}

func setWorkshopCache(key string, data []byte) {
	cachedCopy := append([]byte(nil), data...)

	workshopCacheMu.Lock()
	workshopCache[key] = &workshopCacheEntry{data: cachedCopy, cachedAt: time.Now()}
	if len(workshopCache) > 50 {
		for k, v := range workshopCache {
			if time.Since(v.cachedAt) > workshopCacheTTL {
				delete(workshopCache, k)
			}
		}
	}
	workshopCacheMu.Unlock()

	setWorkshopDiskCache(key, cachedCopy)
}

func getWorkshopDiskCache(key string) ([]byte, bool) {
	cachePath := workshopCacheFilePath(key)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	var entry workshopDiskCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		_ = os.Remove(cachePath)
		return nil, false
	}
	if entry.CachedAt.IsZero() || time.Since(entry.CachedAt) > workshopCacheTTL || len(entry.Data) == 0 {
		if !entry.CachedAt.IsZero() && time.Since(entry.CachedAt) > workshopCacheTTL {
			_ = os.Remove(cachePath)
		}
		return nil, false
	}

	return append([]byte(nil), entry.Data...), true
}

func setWorkshopDiskCache(key string, data []byte) {
	cacheDir := workshopCacheDir()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Printf("⚠️ 创建创意工坊缓存目录失败: %v", err)
		return
	}

	payload, err := json.Marshal(workshopDiskCacheEntry{
		CachedAt: time.Now(),
		Data:     json.RawMessage(data),
	})
	if err != nil {
		log.Printf("⚠️ 序列化创意工坊缓存失败: %v", err)
		return
	}

	cachePath := workshopCacheFilePath(key)
	tempPath := cachePath + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0644); err != nil {
		log.Printf("⚠️ 写入创意工坊缓存失败: %v", err)
		return
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		log.Printf("⚠️ 保存创意工坊缓存失败: %v", err)
		return
	}

	cleanupWorkshopDiskCache(cacheDir)
}

func cleanupWorkshopDiskCache(cacheDir string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil || len(entries) <= 80 {
		return
	}

	cutoff := time.Now().Add(-workshopCacheTTL)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(cacheDir, entry.Name()))
		}
	}
}

type commandLogBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (b *commandLogBuffer) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines = append(b.lines, line)
	if len(b.lines) > 30 {
		b.lines = b.lines[len(b.lines)-30:]
	}
}

func (b *commandLogBuffer) LastRelevant() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := len(b.lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(b.lines[i])
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") ||
			strings.Contains(lower, "failed") ||
			strings.Contains(lower, "failure") ||
			strings.Contains(lower, "denied") ||
			strings.Contains(lower, "timeout") ||
			strings.Contains(lower, "not subscribed") ||
			strings.Contains(lower, "no subscription") ||
			strings.Contains(lower, "no connection") ||
			strings.Contains(lower, "network") ||
			strings.Contains(lower, "disk") ||
			strings.Contains(lower, "login") ||
			strings.Contains(lower, "workshop") ||
			strings.Contains(lower, "0x") {
			return line
		}
	}

	if len(b.lines) == 0 {
		return ""
	}
	return strings.TrimSpace(b.lines[len(b.lines)-1])
}

func buildModDownloadError(base string, err error, buffers ...*commandLogBuffer) string {
	parts := []string{strings.TrimSpace(base)}
	if err != nil {
		parts = append(parts, strings.TrimSpace(err.Error()))
	}

	for _, buffer := range buffers {
		if buffer == nil {
			continue
		}
		if reason := strings.TrimSpace(buffer.LastRelevant()); reason != "" {
			duplicate := false
			for _, existing := range parts {
				if existing == reason {
					duplicate = true
					break
				}
			}
			if !duplicate {
				parts = append(parts, reason)
			}
			break
		}
	}

	return strings.Join(parts, " | ")
}

func clampProgress(progress int) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func updateModProgressState(workshopID, modName, status, message string, progress int) *models.DownloadProgress {
	downloadingMutex.Lock()
	defer downloadingMutex.Unlock()

	item, exists := modDownloadProgress[workshopID]
	if !exists {
		item = &models.DownloadProgress{
			ID:         workshopID,
			PluginName: modName,
			Status:     "downloading",
			Progress:   0,
			Message:    "准备下载...",
			StartTime:  time.Now(),
		}
		modDownloadProgress[workshopID] = item
	}

	if modName != "" {
		item.PluginName = modName
	}
	if status != "" {
		item.Status = status
	}
	if message != "" {
		item.Message = message
	}
	if progress >= 0 {
		item.Progress = clampProgress(progress)
	}

	return &models.DownloadProgress{
		ID:         item.ID,
		PluginName: item.PluginName,
		Status:     item.Status,
		Progress:   item.Progress,
		Message:    item.Message,
		StartTime:  item.StartTime,
	}
}

func parseModProgressMessage(message string) (string, int, string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return "downloading", -1, "下载中..."
	}

	if strings.Contains(msg, "下载失败") || strings.Contains(strings.ToLower(msg), "failed") || strings.Contains(strings.ToLower(msg), "error") {
		return "failed", 0, msg
	}

	if msg == "Downloaded" {
		return "completed", 100, "下载完成"
	}

	if strings.Contains(msg, "正在安装") {
		return "installing", 92, msg
	}

	if strings.Contains(msg, "正在查找") || strings.Contains(msg, "查找下载的MOD文件") {
		return "installing", 88, msg
	}

	if msg == "Downloading" {
		return "downloading", 10, "正在连接 Steam 下载..."
	}

	re := regexp.MustCompile(`(\d{1,3})%`)
	match := re.FindStringSubmatch(msg)
	if len(match) > 1 {
		percent, _ := strconv.Atoi(match[1])
		mapped := 10 + int(float64(clampProgress(percent))*0.75)
		return "downloading", mapped, fmt.Sprintf("正在下载 %d%%", clampProgress(percent))
	}

	return "downloading", -1, msg
}

func scheduleModProgressCleanup(workshopID string, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		downloadingMutex.Lock()
		delete(modDownloadProgress, workshopID)
		downloadingMutex.Unlock()
	}()
}

func BroadcastModProgress(workshopID, message string) {
	log.Printf("[MOD进度] WorkshopID=%s, 消息=%s", workshopID, message)
	status, progress, normalizedMessage := parseModProgressMessage(message)
	item := updateModProgressState(workshopID, "", status, normalizedMessage, progress)
	progressData := map[string]interface{}{
		"type":       "mod_progress",
		"workshopId": workshopID,
		"status":     item.Status,
		"progress":   item.Progress,
		"message":    item.Message,
		"name":       item.PluginName,
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

type workshopSearchCandidate struct {
	item          gin.H
	score         int
	subscriptions int
	views         int
	votesUp       int
	timeUpdated   int64
}

type steamWorkshopVoteData struct {
	Score     float64 `json:"score"`
	VotesUp   int     `json:"votes_up"`
	VotesDown int     `json:"votes_down"`
}

type steamWorkshopTag struct {
	Tag string `json:"tag"`
}

type steamWorkshopQueryFile struct {
	PublishedFileID string                `json:"publishedfileid"`
	Title           string                `json:"title"`
	Description     string                `json:"short_description"`
	FileSize        string                `json:"file_size"`
	PreviewURL      string                `json:"preview_url"`
	Subscriptions   int                   `json:"subscriptions"`
	Favorited       int                   `json:"favorited"`
	Views           int                   `json:"views"`
	VoteData        steamWorkshopVoteData `json:"vote_data"`
	Tags            []steamWorkshopTag    `json:"tags"`
	TimeCreated     int64                 `json:"time_created"`
	TimeUpdated     int64                 `json:"time_updated"`
}

type steamWorkshopQueryResponse struct {
	Response struct {
		Total          int                      `json:"total"`
		PublishedFiles []steamWorkshopQueryFile `json:"publishedfiledetails"`
	} `json:"response"`
}

func parsePositiveIntOrDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func normalizeWorkshopSearchText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	replacer := strings.NewReplacer(
		"_", " ",
		"-", " ",
		".", " ",
		":", " ",
		"/", " ",
		"\\", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		",", " ",
		"!", " ",
		"?", " ",
		"'", " ",
		"\"", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}

func scoreWorkshopSearchCandidate(title, description string, tags []string, query string) int {
	normalizedQuery := normalizeWorkshopSearchText(query)
	if normalizedQuery == "" {
		return 0
	}

	titleText := normalizeWorkshopSearchText(title)
	descriptionText := normalizeWorkshopSearchText(description)
	tagTexts := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagTexts = append(tagTexts, normalizeWorkshopSearchText(tag))
	}

	score := 0
	switch {
	case titleText == normalizedQuery:
		score += 1200
	case strings.HasPrefix(titleText, normalizedQuery):
		score += 800
	case strings.Contains(titleText, normalizedQuery):
		score += 600
	}

	if strings.HasPrefix(descriptionText, normalizedQuery) {
		score += 80
	} else if strings.Contains(descriptionText, normalizedQuery) {
		score += 40
	}

	for _, tagText := range tagTexts {
		if tagText == normalizedQuery {
			score += 120
			break
		}
		if strings.Contains(tagText, normalizedQuery) {
			score += 60
			break
		}
	}

	tokens := strings.Fields(normalizedQuery)
	if len(tokens) == 0 {
		return score
	}

	allTokensInTitle := true
	allTokensInText := true
	for _, token := range tokens {
		inTitle := strings.Contains(titleText, token)
		inDescription := strings.Contains(descriptionText, token)
		inTags := false
		for _, tagText := range tagTexts {
			if strings.Contains(tagText, token) {
				inTags = true
				break
			}
		}

		if inTitle {
			score += 160
		} else {
			allTokensInTitle = false
		}
		if inDescription {
			score += 35
		}
		if inTags {
			score += 45
		}
		if !inTitle && !inDescription && !inTags {
			allTokensInText = false
		}
	}

	if len(tokens) > 1 {
		if allTokensInTitle {
			score += 280
		} else if allTokensInText {
			score += 120
		}
	}

	return score
}

func fetchSteamWorkshopQueryPage(queryType string, apiPage, apiPageSize int, query string) (steamWorkshopQueryResponse, error) {
	apiURL := fmt.Sprintf(
		"https://api.steampowered.com/IPublishedFileService/QueryFiles/v1/?key=%s&query_type=%s&page=%d&numperpage=%d&appid=%s&return_tags=true&return_vote_data=true&return_previews=true&return_short_description=true",
		STEAM_API_KEY, queryType, apiPage, apiPageSize, TMODLOADER_APPID,
	)
	if query != "" {
		apiURL += "&search_text=" + url.QueryEscape(query)
	}

	steamClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := steamClient.Get(apiURL)
	if err != nil {
		return steamWorkshopQueryResponse{}, err
	}
	defer resp.Body.Close()

	var result steamWorkshopQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return steamWorkshopQueryResponse{}, err
	}
	return result, nil
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
	pageInt := parsePositiveIntOrDefault(page, 1)
	pageSizeInt := parsePositiveIntOrDefault(pageSize, 20)
	sortBy := c.DefaultQuery("sortBy", "trend_days")
	var queryType string
	if query != "" {
		// 有搜索关键词时使用文本搜索排序
		queryType = "12"
	} else {
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
	}
	cacheKey := fmt.Sprintf("%s|%s|%s|%s", queryType, query, page, pageSize)
	if cached, ok := getWorkshopCache(cacheKey); ok {
		log.Printf("✅ Steam API 缓存命中: sortBy=%s, query=%s, page=%s", sortBy, query, page)
		c.Header("X-Workshop-Cache", "hit")
		c.Data(http.StatusOK, "application/json", cached)
		return
	}

	log.Printf("🔍 Steam API请求: sortBy=%s, query=%s, page=%s, pageSize=%s", sortBy, query, page, pageSize)
	result := steamWorkshopQueryResponse{}
	if query != "" {
		searchWindowSize := pageSizeInt * (pageInt + 4)
		if searchWindowSize < 50 {
			searchWindowSize = 50
		}
		if searchWindowSize > 500 {
			searchWindowSize = 500
		}

		apiPageSize := 100
		pagesNeeded := (searchWindowSize + apiPageSize - 1) / apiPageSize
		if pagesNeeded < 1 {
			pagesNeeded = 1
		}

		result.Response.PublishedFiles = make([]steamWorkshopQueryFile, 0, pagesNeeded*apiPageSize)
		for apiPage := 1; apiPage <= pagesNeeded; apiPage++ {
			pageResult, err := fetchSteamWorkshopQueryPage(queryType, apiPage, apiPageSize, query)
			if err != nil {
				c.JSON(http.StatusInternalServerError, models.ErrorResponse("Steam API 请求失败: "+err.Error()))
				return
			}
			if apiPage == 1 {
				result.Response.Total = pageResult.Response.Total
			}
			result.Response.PublishedFiles = append(result.Response.PublishedFiles, pageResult.Response.PublishedFiles...)
			if len(result.Response.PublishedFiles) >= searchWindowSize || len(pageResult.Response.PublishedFiles) < apiPageSize {
				break
			}
		}
	} else {
		pageResult, err := fetchSteamWorkshopQueryPage(queryType, pageInt, pageSizeInt, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Steam API 请求失败: "+err.Error()))
			return
		}
		result = pageResult
	}
	items := make([]gin.H, 0, len(result.Response.PublishedFiles))
	candidates := make([]workshopSearchCandidate, 0, len(result.Response.PublishedFiles))
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
		if query != "" {
			candidates = append(candidates, workshopSearchCandidate{
				item:          item,
				score:         scoreWorkshopSearchCandidate(file.Title, file.Description, tags, query),
				subscriptions: file.Subscriptions,
				views:         file.Views,
				votesUp:       file.VoteData.VotesUp,
				timeUpdated:   file.TimeUpdated,
			})
		} else {
			items = append(items, item)
		}
	}
	if query != "" {
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].score != candidates[j].score {
				return candidates[i].score > candidates[j].score
			}
			if candidates[i].subscriptions != candidates[j].subscriptions {
				return candidates[i].subscriptions > candidates[j].subscriptions
			}
			if candidates[i].votesUp != candidates[j].votesUp {
				return candidates[i].votesUp > candidates[j].votesUp
			}
			if candidates[i].views != candidates[j].views {
				return candidates[i].views > candidates[j].views
			}
			return candidates[i].timeUpdated > candidates[j].timeUpdated
		})

		start := (pageInt - 1) * pageSizeInt
		if start < len(candidates) {
			end := start + pageSizeInt
			if end > len(candidates) {
				end = len(candidates)
			}
			for _, candidate := range candidates[start:end] {
				items = append(items, candidate.item)
			}
		}
	}
	actualTotal := result.Response.Total
	if query != "" && len(candidates) > 0 && actualTotal > len(candidates) {
		actualTotal = len(candidates)
	}
	if len(items) == 0 && actualTotal > 0 {
		actualTotal = (pageInt - 1) * pageSizeInt
		log.Printf("⚠️ 第%d页无数据，限制总数为: %d", pageInt, actualTotal)
	}
	if actualTotal > 10000 {
		actualTotal = 10000
		log.Printf("⚠️ 总数超过10000，限制为: 10000")
	}
	responseData := models.SuccessResponse(gin.H{
		"total": actualTotal,
		"items": items,
	})
	c.Header("X-Workshop-Cache", "miss")
	// 缓存响应
	if jsonBytes, err := json.Marshal(responseData); err == nil {
		setWorkshopCache(cacheKey, jsonBytes)
	}
	c.JSON(http.StatusOK, responseData)
}
func GetDownloadingMods(c *gin.Context) {
	downloadingMutex.RLock()
	defer downloadingMutex.RUnlock()

	list := []string{}
	for workshopID := range downloadingMods {
		list = append(list, workshopID)
	}

	items := make([]models.DownloadProgress, 0, len(modDownloadProgress))
	for _, item := range modDownloadProgress {
		items = append(items, models.DownloadProgress{
			ID:         item.ID,
			PluginName: item.PluginName,
			Status:     item.Status,
			Progress:   item.Progress,
			Message:    item.Message,
			StartTime:  item.StartTime,
		})
	}

	log.Printf("📋 查询下载状态: 当前 %d 个MOD正在下载 %v", len(list), list)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"downloading": list,
		"items":       items,
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
		updateModProgressState(req.WorkshopID, req.Name, "downloading", "准备下载...", 0)
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
				BroadcastModProgress(req.WorkshopID, "下载失败: "+buildModDownloadError("下载任务异常崩溃", fmt.Errorf("%v", r)))
				scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
			}
		}()
		modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
		os.MkdirAll(modDir, 0755)
		log.Printf("开始下载 MOD: %s (Workshop ID: %s)", req.Name, req.WorkshopID)
		steamcmdPath := filepath.Join(config.DataDir, "steamcmd", "steamcmd.sh")
		if runtime.GOOS == "windows" {
			steamcmdPath = filepath.Join(config.DataDir, "steamcmd", "steamcmd.exe")
		}
		_, steamcmdReady, _, _, steamcmdStateMessage := getSteamCMDState()
		if !steamcmdReady {
			log.Printf("SteamCMD 不可用，开始安装/修复: %s", steamcmdStateMessage)
			if err := installSteamCMD(); err != nil {
				errMsg := fmt.Sprintf("SteamCMD安装失败: %v", err)
				if runtime.GOOS == "linux" {
					errMsg += "\n\n请手动安装依赖：\nsudo dpkg --add-architecture i386\nsudo apt-get update\nsudo apt-get install lib32gcc-s1 lib32stdc++6"
				}
				log.Printf("❌ %s", errMsg)
				BroadcastModProgress(req.WorkshopID, "下载失败: "+errMsg)
				scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
				return
			}
		}
		if runtime.GOOS == "linux" {
			depCheckCmd := exec.Command("dpkg", "-l", "lib32gcc-s1")
			if err := depCheckCmd.Run(); err != nil {
				errMsg := "缺少 32 位运行库，请先执行：\nsudo dpkg --add-architecture i386\nsudo apt-get update\nsudo apt-get install lib32gcc-s1 lib32stdc++6"
				log.Printf("❌ %s", errMsg)
				BroadcastModProgress(req.WorkshopID, "下载失败: "+errMsg)
				scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
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
		stdoutLog := &commandLogBuffer{}
		stderrLog := &commandLogBuffer{}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("❌ 创建输出管道失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败: "+buildModDownloadError("创建 SteamCMD 输出管道失败", err))
			scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Printf("❌ 创建错误管道失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败: "+buildModDownloadError("创建 SteamCMD 错误管道失败", err))
			scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
			return
		}
		if err := cmd.Start(); err != nil {
			log.Printf("❌ 启动 SteamCMD 失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败: "+buildModDownloadError("启动 SteamCMD 失败", err))
			scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
			return
		}
		log.Printf("🚀 开始下载 MOD (WorkshopID: %s)", req.WorkshopID)
		BroadcastModProgress(req.WorkshopID, "Downloading")
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				stdoutLog.Add(line)
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
				stderrLog.Add(line)
				log.Printf("SteamCMD (stderr): %s", line)
			}
		}()
		log.Printf("⏳ 等待 SteamCMD 完成...")
		if err := cmd.Wait(); err != nil {
			log.Printf("❌ SteamCMD 执行失败: %v", err)
			BroadcastModProgress(req.WorkshopID, "下载失败: "+buildModDownloadError("SteamCMD 执行失败", err, stderrLog, stdoutLog))
			scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
			return
		}
		log.Printf("✅ SteamCMD 命令执行完成")
		log.Printf("🔍 开始查找下载的MOD文件...")
		updateModProgressState(req.WorkshopID, req.Name, "installing", "正在查找模组文件...", 88)
		BroadcastModProgress(req.WorkshopID, "正在查找模组文件...")
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
				updateModProgressState(req.WorkshopID, req.Name, "installing", "正在安装模组文件...", 95)
				BroadcastModProgress(req.WorkshopID, "下载完成，正在安装...")
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
				scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
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
									scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
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
			scheduleModProgressCleanup(req.WorkshopID, 15*time.Second)
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
func normalizeModName(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), ".tmod")
}
func buildModNameAliases(names ...string) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, raw := range names {
		name := normalizeModName(raw)
		if name == "" {
			continue
		}
		aliases[name] = struct{}{}
		aliases[extractModName(name+".tmod")] = struct{}{}
	}
	return aliases
}
func matchesModName(aliases map[string]struct{}, candidate string) bool {
	if len(aliases) == 0 {
		return false
	}
	name := normalizeModName(candidate)
	if name == "" {
		return false
	}
	if _, ok := aliases[name]; ok {
		return true
	}
	_, ok := aliases[extractModName(name+".tmod")]
	return ok
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
	modName := normalizeModName(c.Param("name"))
	log.Printf("🗑️ 开始删除MOD: %s", modName)
	modDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	enabledFile := filepath.Join(modDir, "enabled.json")
	mappingFile := filepath.Join(modDir, "workshop_mapping.json")
	var deletedFile string
	var storedModName string
	requestAliases := buildModNameAliases(modName)
	files, err := os.ReadDir(modDir)
	if err != nil {
		log.Printf("❌ 读取MOD目录失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取MOD目录失败"))
		return
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".tmod") {
			candidateName := strings.TrimSuffix(file.Name(), ".tmod")
			if matchesModName(requestAliases, candidateName) {
				deletedFile = file.Name()
				storedModName = candidateName
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
	deleteAliases := buildModNameAliases(modName, storedModName, deletedFile)
	var enabledMods []string
	if data, err := os.ReadFile(enabledFile); err == nil {
		json.Unmarshal(data, &enabledMods)
	}
	newList := []string{}
	for _, mod := range enabledMods {
		if !matchesModName(deleteAliases, mod) {
			newList = append(newList, mod)
		}
	}
	if len(newList) != len(enabledMods) {
		data, _ := json.MarshalIndent(newList, "", "  ")
		os.WriteFile(enabledFile, data, 0644)
		log.Printf("✅ 已从 enabled.json 移除: %s", storedModName)
	}
	if data, err := os.ReadFile(mappingFile); err == nil {
		var mapping map[string]ModMappingData
		if err := json.Unmarshal(data, &mapping); err == nil {
			var workshopIdToDelete string
			for workshopId, modData := range mapping {
				if matchesModName(deleteAliases, modData.ModName) {
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
	if err := os.MkdirAll(steamcmdDir, 0755); err != nil {
		return fmt.Errorf("创建 SteamCMD 目录失败: %v", err)
	}
	if runtime.GOOS == "linux" {
		for _, name := range []string{"linux32", "linux64", "package", "steamcmd.sh", "steam.sh", "steam", "steamerrorreporter", "steamerrorreporter64"} {
			_ = os.RemoveAll(filepath.Join(steamcmdDir, name))
		}
		steamcmdURL := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"
		tarPath := filepath.Join(steamcmdDir, "steamcmd_linux.tar.gz")
		log.Printf("下载 SteamCMD: %s", steamcmdURL)
		resp, err := http.Get(steamcmdURL)
		if err != nil {
			return fmt.Errorf("下载 SteamCMD 失败: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("下载 SteamCMD 失败: HTTP %d", resp.StatusCode)
		}
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
				dirMode := header.FileInfo().Mode().Perm()
				if dirMode == 0 {
					dirMode = 0755
				}
				if err := os.MkdirAll(path, dirMode); err != nil {
					return fmt.Errorf("创建目录失败: %v", err)
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return fmt.Errorf("创建父目录失败: %v", err)
			}
			fileMode := header.FileInfo().Mode().Perm()
			if fileMode == 0 {
				fileMode = 0644
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
			if err != nil {
				return fmt.Errorf("创建文件失败: %v", err)
			}
			_, err = io.Copy(file, tarReader)
			file.Close()
			if err != nil {
				return fmt.Errorf("解压文件失败: %v", err)
			}
			if err := os.Chmod(path, fileMode); err != nil {
				log.Printf("⚠️ 设置文件权限失败 %s: %v", path, err)
			}
			if strings.HasSuffix(header.Name, ".sh") ||
				header.Name == "steamcmd" ||
				strings.HasSuffix(header.Name, "/steamcmd") ||
				strings.Contains(header.Name, "linux32/steamcmd") ||
				strings.Contains(header.Name, "linux64/steamcmd") {
				if err := os.Chmod(path, 0755); err != nil {
					log.Printf("⚠️ 设置可执行权限失败 %s: %v", path, err)
				}
			}
		}
		os.Remove(tarPath)
		linuxRuntimePath := filepath.Join(steamcmdDir, "linux32", "steamcmd")
		if _, err := os.Stat(linuxRuntimePath); err != nil {
			return fmt.Errorf("解压不完整，缺少运行文件: %s", linuxRuntimePath)
		}
		if err := os.Chmod(filepath.Join(steamcmdDir, "steamcmd.sh"), 0755); err != nil {
			log.Printf("⚠️ 设置 steamcmd.sh 权限失败: %v", err)
		}
		if err := os.Chmod(linuxRuntimePath, 0755); err != nil {
			log.Printf("⚠️ 设置 linux32/steamcmd 权限失败: %v", err)
		}
		log.Printf("初始化 SteamCMD...")
		steamcmdPath := filepath.Join(steamcmdDir, "steamcmd.sh")
		cmd := exec.Command(steamcmdPath, "+quit")
		output, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := strings.TrimSpace(string(output))
			if errMsg != "" {
				return fmt.Errorf("初始化 SteamCMD 失败: %v | %s", err, errMsg)
			}
			return fmt.Errorf("初始化 SteamCMD 失败: %v", err)
		}
	} else if runtime.GOOS == "windows" {
		for _, name := range []string{"steamcmd.exe", "steam.dll", "steamerrorreporter.exe", "steamerrorreporter64.exe", "Steam.dll"} {
			_ = os.RemoveAll(filepath.Join(steamcmdDir, name))
		}
		steamcmdURL := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"
		zipPath := filepath.Join(steamcmdDir, "steamcmd.zip")
		log.Printf("下载 SteamCMD: %s", steamcmdURL)
		resp, err := http.Get(steamcmdURL)
		if err != nil {
			return fmt.Errorf("下载 SteamCMD 失败: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("下载 SteamCMD 失败: HTTP %d", resp.StatusCode)
		}
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
		output, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := strings.TrimSpace(string(output))
			if errMsg != "" {
				return fmt.Errorf("初始化 SteamCMD 失败: %v | %s", err, errMsg)
			}
			return fmt.Errorf("初始化 SteamCMD 失败: %v", err)
		}
	} else {
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
	log.Printf("SteamCMD 安装完成")
	return nil
}
