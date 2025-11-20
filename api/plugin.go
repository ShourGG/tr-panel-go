package api
import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"
	"time"
	"github.com/gin-gonic/gin"
)
const (
	PluginsJSONURLOriginal = "https://raw.githubusercontent.com/UnrealMultiple/TShockPlugin/master/Plugins.json"
	PluginsZipURLOriginal  = "https://github.com/UnrealMultiple/TShockPlugin/releases/download/V1.0.0.0/Plugins.zip"
	PluginsCacheDir      = "plugin-store"
	PluginsCacheFile     = "plugins-cache.json"
	PluginsCacheDuration = 72 * time.Hour
)
var githubMirrors = []string{
	"https://ghproxy.com/",
	"https://gh-proxy.com/",
	"https://mirror.ghproxy.com/",
	"https://ghps.cc/",
}
var (
	downloadProgress = make(map[string]*models.DownloadProgress)
	progressMutex    sync.RWMutex
)
var (
	pluginStoreCache      []models.PluginStoreItem
	pluginStoreCacheTime  time.Time
	pluginStoreCacheMutex sync.RWMutex
)
func GetPlugins(c *gin.Context) {
	roomIDStr := c.Param("id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid room ID"))
		return
	}
	room, err := roomStorage.GetByID(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Room not found"))
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Only TShock servers support plugins"))
		return
	}
	pluginsDir := getPluginsDir(roomID)
	disabledDir := filepath.Join(pluginsDir, "Disabled")
	os.MkdirAll(pluginsDir, 0755)
	os.MkdirAll(disabledDir, 0755)
	enabledPlugins, err := scanPluginsDir(pluginsDir, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to scan plugins directory"))
		return
	}
	disabledPlugins, err := scanPluginsDir(disabledDir, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to scan disabled plugins directory"))
		return
	}
	allPlugins := append(enabledPlugins, disabledPlugins...)
	c.JSON(http.StatusOK, gin.H{
		"plugins": allPlugins,
		"total":   len(allPlugins),
	})
}
func UploadPlugin(c *gin.Context) {
	roomIDStr := c.Param("id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid room ID"))
		return
	}
	room, err := roomStorage.GetByID(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Room not found"))
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Only TShock servers support plugins"))
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("No file uploaded"))
		return
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".dll") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Only .dll files are allowed"))
		return
	}
	const maxFileSize = 10 * 1024 * 1024
	if file.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("File size exceeds 10 MB limit"))
		return
	}
	filename := filepath.Base(file.Filename)
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid filename"))
		return
	}
	pluginsDir := getPluginsDir(roomID)
	os.MkdirAll(pluginsDir, 0755)
	destPath := filepath.Join(pluginsDir, filename)
	if err := c.SaveUploadedFile(file, destPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to save file"))
		return
	}
	fileInfo, err := os.Stat(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get file info"))
		return
	}
	plugin := models.Plugin{
		Name:       filename,
		FilePath:   destPath,
		Size:       fileInfo.Size(),
		Enabled:    true,
		UploadTime: fileInfo.ModTime(),
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Plugin uploaded successfully",
		"plugin":  plugin,
	})
}
func DeletePlugin(c *gin.Context) {
	roomIDStr := c.Param("id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid room ID"))
		return
	}
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}
	room, err := roomStorage.GetByID(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Room not found"))
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Only TShock servers support plugins"))
		return
	}
	pluginsDir := getPluginsDir(roomID)
	enabledPath := filepath.Join(pluginsDir, pluginName)
	if _, err := os.Stat(enabledPath); err == nil {
		if err := os.Remove(enabledPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to delete plugin"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Plugin deleted successfully"})
		return
	}
	disabledPath := filepath.Join(pluginsDir, "Disabled", pluginName)
	if _, err := os.Stat(disabledPath); err == nil {
		if err := os.Remove(disabledPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to delete plugin"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Plugin deleted successfully"})
		return
	}
	c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin not found"))
}
func TogglePlugin(c *gin.Context) {
	roomIDStr := c.Param("id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid room ID"))
		return
	}
	pluginName := c.Param("name")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin name is required"))
		return
	}
	var req models.PluginToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid request body"))
		return
	}
	room, err := roomStorage.GetByID(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Room not found"))
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Only TShock servers support plugins"))
		return
	}
	pluginsDir := getPluginsDir(roomID)
	disabledDir := filepath.Join(pluginsDir, "Disabled")
	os.MkdirAll(disabledDir, 0755)
	if req.Enabled {
		srcPath := filepath.Join(disabledDir, pluginName)
		destPath := filepath.Join(pluginsDir, pluginName)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin not found in disabled directory"))
			return
		}
		if err := moveFile(srcPath, destPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to enable plugin"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Plugin enabled successfully"})
	} else {
		srcPath := filepath.Join(pluginsDir, pluginName)
		destPath := filepath.Join(disabledDir, pluginName)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("Plugin not found in enabled directory"))
			return
		}
		if err := moveFile(srcPath, destPath); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to disable plugin"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Plugin disabled successfully"})
	}
}
func getPluginsDir(roomID int) string {
	return filepath.Join(config.DataDir, "servers", fmt.Sprintf("tshock-%d", roomID), "ServerPlugins")
}
func scanPluginsDir(dir string, enabled bool) ([]models.Plugin, error) {
	var plugins []models.Plugin
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return plugins, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".dll") {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}
		plugin := models.Plugin{
			Name:       entry.Name(),
			FilePath:   filePath,
			Size:       fileInfo.Size(),
			Enabled:    enabled,
			UploadTime: fileInfo.ModTime(),
		}
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}
func moveFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}
	return os.Remove(src)
}
func GetPluginStore(c *gin.Context) {
	forceRefresh := c.Query("refresh") == "true"
	if !forceRefresh {
		plugins, fromCache := getPluginStoreFromCache()
		if fromCache {
			fmt.Printf("[Plugin Store] ✅ Serving from cache (%d plugins)\n", len(plugins))
			c.JSON(http.StatusOK, gin.H{
				"plugins":   plugins,
				"total":     len(plugins),
				"fromCache": true,
				"cacheTime": pluginStoreCacheTime.Format(time.RFC3339),
			})
			return
		}
	}
	fmt.Println("[Plugin Store] 🔄 Fetching plugin store from GitHub...")
	plugins, err := fetchPluginStoreFromGitHub()
	if err != nil {
		fmt.Printf("[Plugin Store] ❌ Failed to fetch from GitHub: %v\n", err)
		pluginStoreCacheMutex.RLock()
		if len(pluginStoreCache) > 0 {
			plugins := pluginStoreCache
			pluginStoreCacheMutex.RUnlock()
			fmt.Printf("[Plugin Store] ⚠️ Using stale cache as fallback (%d plugins)\n", len(plugins))
			c.JSON(http.StatusOK, gin.H{
				"plugins":   plugins,
				"total":     len(plugins),
				"fromCache": true,
				"stale":     true,
				"cacheTime": pluginStoreCacheTime.Format(time.RFC3339),
				"warning":   "Using cached data due to network issues",
			})
			return
		}
		pluginStoreCacheMutex.RUnlock()
		errorMsg := fmt.Sprintf("Failed to fetch plugin store: %v", err)
		fmt.Printf("[Plugin Store] 💥 Returning error to client: %s\n", errorMsg)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch plugin store",
			"details": errorMsg,
			"suggestion": "Please check your network connection or try again later. The plugin store data is fetched from GitHub.",
		})
		return
	}
	updatePluginStoreCache(plugins)
	fmt.Printf("[Plugin Store] ✅ Successfully loaded %d plugins from GitHub\n", len(plugins))
	c.JSON(http.StatusOK, gin.H{
		"plugins":   plugins,
		"total":     len(plugins),
		"fromCache": false,
		"cacheTime": pluginStoreCacheTime.Format(time.RFC3339),
	})
}
func InstallPluginFromStore(c *gin.Context) {
	roomIDStr := c.Param("id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid room ID"))
		return
	}
	pluginID := c.Param("pluginId")
	if pluginID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Plugin ID is required"))
		return
	}
	room, err := roomStorage.GetByID(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Room not found"))
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Only TShock servers support plugins"))
		return
	}
	progressID := fmt.Sprintf("%d-%s-%d", roomID, pluginID, time.Now().Unix())
	progress := &models.DownloadProgress{
		ID:          progressID,
		PluginName:  pluginID,
		Status:      "downloading",
		Progress:    0,
		Message:     "Starting download...",
		StartTime:   time.Now(),
	}
	progressMutex.Lock()
	downloadProgress[progressID] = progress
	progressMutex.Unlock()
	go func() {
		defer func() {
			time.Sleep(5 * time.Minute)
			progressMutex.Lock()
			delete(downloadProgress, progressID)
			progressMutex.Unlock()
		}()
		if err := downloadAndInstallPlugin(roomID, pluginID, progress); err != nil {
			progress.Status = "failed"
			progress.Message = err.Error()
			progress.Progress = 0
		} else {
			progress.Status = "completed"
			progress.Message = "Plugin installed successfully"
			progress.Progress = 100
		}
	}()
	c.JSON(http.StatusOK, gin.H{
		"message":    "Plugin installation started",
		"progressId": progressID,
	})
}
func GetPluginInstallProgress(c *gin.Context) {
	progressID := c.Param("progressId")
	progressMutex.RLock()
	progress, exists := downloadProgress[progressID]
	progressMutex.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse("Progress not found"))
		return
	}
	c.JSON(http.StatusOK, progress)
}
func downloadAndInstallPlugin(roomID int, pluginID string, progress *models.DownloadProgress) error {
	cacheDir := filepath.Join(config.DataDir, PluginsCacheDir)
	os.MkdirAll(cacheDir, 0755)
	zipPath := filepath.Join(cacheDir, "Plugins.zip")
	extractDir := filepath.Join(cacheDir, "extracted")
	needDownload := true
	if fileInfo, err := os.Stat(zipPath); err == nil {
		if time.Since(fileInfo.ModTime()) < 24*time.Hour {
			needDownload = false
			progress.Message = "Using cached plugin package..."
			progress.Progress = 30
		}
	}
	if needDownload {
		progress.Message = "Downloading plugin package..."
		progress.Progress = 10
		cfg := config.Load()
		downloadURL := buildPluginZipURL(cfg)
		fmt.Printf("[Plugin Install] Downloading from: %s\n", downloadURL)
		if err := downloadPluginFileWithProgress(downloadURL, zipPath, progress); err != nil {
			return fmt.Errorf("failed to download plugin package: %v", err)
		}
		progress.Progress = 50
	}
	progress.Message = "Extracting plugin package..."
	progress.Progress = 60
	if err := extractZip(zipPath, extractDir); err != nil {
		return fmt.Errorf("failed to extract plugin package: %v", err)
	}
	progress.Progress = 70
	progress.Message = "Installing plugin..."
	progress.Progress = 80
	pluginDLL := pluginID + ".dll"
	srcPath := filepath.Join(extractDir, pluginDLL)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin %s not found in package", pluginID)
	}
	pluginsDir := getPluginsDir(roomID)
	os.MkdirAll(pluginsDir, 0755)
	destPath := filepath.Join(pluginsDir, pluginDLL)
	if err := copyFile(srcPath, destPath); err != nil {
		return fmt.Errorf("failed to install plugin: %v", err)
	}
	progress.Progress = 100
	progress.Message = "Plugin installed successfully"
	return nil
}
func downloadPluginFileWithProgress(url, destPath string, progress *models.DownloadProgress) error {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "Terraria-Panel/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: failed to download file", resp.StatusCode)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()
	fileSize := resp.ContentLength
	reader := &progressReader{
		reader:       resp.Body,
		total:        fileSize,
		progress:     progress,
		baseProgress: 10,
		maxProgress:  50,
	}
	_, err = io.Copy(out, reader)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}
	return nil
}
type progressReader struct {
	reader       io.Reader
	total        int64
	current      int64
	progress     *models.DownloadProgress
	baseProgress int
	maxProgress  int
}
func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
	if pr.total > 0 {
		percentage := float64(pr.current) / float64(pr.total)
		pr.progress.Progress = pr.baseProgress + int(percentage*float64(pr.maxProgress-pr.baseProgress))
	}
	return n, err
}
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	os.MkdirAll(destDir, 0755)
	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outFile, err := os.Create(fpath)
		if err != nil {
			rc.Close()
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
func getPluginStoreFromCache() ([]models.PluginStoreItem, bool) {
	pluginStoreCacheMutex.RLock()
	defer pluginStoreCacheMutex.RUnlock()
	if len(pluginStoreCache) > 0 && time.Since(pluginStoreCacheTime) < PluginsCacheDuration {
		return pluginStoreCache, true
	}
	cacheDir := filepath.Join(config.DataDir, PluginsCacheDir)
	cacheFile := filepath.Join(cacheDir, PluginsCacheFile)
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}
	var cacheData struct {
		Plugins   []models.PluginStoreItem `json:"plugins"`
		CacheTime time.Time                `json:"cacheTime"`
	}
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return nil, false
	}
	if time.Since(cacheData.CacheTime) < PluginsCacheDuration {
		pluginStoreCacheMutex.RUnlock()
		pluginStoreCacheMutex.Lock()
		pluginStoreCache = cacheData.Plugins
		pluginStoreCacheTime = cacheData.CacheTime
		pluginStoreCacheMutex.Unlock()
		pluginStoreCacheMutex.RLock()
		return cacheData.Plugins, true
	}
	return nil, false
}
func fetchPluginStoreFromGitHub() ([]models.PluginStoreItem, error) {
	cfg := config.Load()
	urls := buildPluginStoreURLs(cfg)
	var lastErr error
	for i, url := range urls {
		fmt.Printf("[Plugin Store] Attempt %d/%d: Trying URL: %s\n", i+1, len(urls), url)
		plugins, err := fetchPluginStoreFromURL(url)
		if err == nil {
			fmt.Printf("[Plugin Store] ✅ Successfully fetched from: %s\n", url)
			return plugins, nil
		}
		fmt.Printf("[Plugin Store] ❌ Failed to fetch from %s: %v\n", url, err)
		lastErr = err
		if i < len(urls)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil, fmt.Errorf("failed to fetch plugin store from all sources (tried %d URLs): %v", len(urls), lastErr)
}
func buildPluginStoreURLs(cfg *config.Config) []string {
	urls := []string{}
	if cfg.UseGitHubMirror {
		if cfg.GitHubMirrorURL != "" && cfg.GitHubMirrorURL != "https://ghproxy.com/" {
			urls = append(urls, cfg.GitHubMirrorURL+PluginsJSONURLOriginal)
		}
		for _, mirror := range githubMirrors {
			mirrorURL := mirror + PluginsJSONURLOriginal
			isDuplicate := false
			for _, existing := range urls {
				if existing == mirrorURL {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				urls = append(urls, mirrorURL)
			}
		}
	}
	urls = append(urls, PluginsJSONURLOriginal)
	return urls
}
func buildPluginZipURL(cfg *config.Config) string {
	if cfg.UseGitHubMirror && cfg.GitHubMirrorURL != "" {
		return cfg.GitHubMirrorURL + PluginsZipURLOriginal
	}
	return PluginsZipURLOriginal
}
func fetchPluginStoreFromURL(url string) ([]models.PluginStoreItem, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "Terraria-Panel/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, bodyPreview)
	}
	var plugins []models.PluginStoreItem
	if err := json.NewDecoder(resp.Body).Decode(&plugins); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}
	if len(plugins) == 0 {
		return nil, fmt.Errorf("empty plugin list received")
	}
	return plugins, nil
}
func updatePluginStoreCache(plugins []models.PluginStoreItem) {
	now := time.Now()
	pluginStoreCacheMutex.Lock()
	pluginStoreCache = plugins
	pluginStoreCacheTime = now
	pluginStoreCacheMutex.Unlock()
	cacheDir := filepath.Join(config.DataDir, PluginsCacheDir)
	os.MkdirAll(cacheDir, 0755)
	cacheFile := filepath.Join(cacheDir, PluginsCacheFile)
	cacheData := struct {
		Plugins   []models.PluginStoreItem `json:"plugins"`
		CacheTime time.Time                `json:"cacheTime"`
	}{
		Plugins:   plugins,
		CacheTime: now,
	}
	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		fmt.Printf("[Plugin Store] Failed to marshal cache data: %v\n", err)
		return
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		fmt.Printf("[Plugin Store] Failed to write cache file: %v\n", err)
		return
	}
	fmt.Printf("[Plugin Store] Cache updated successfully (%d plugins)\n", len(plugins))
}
