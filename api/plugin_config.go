package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/utils"

	"github.com/gin-gonic/gin"
)

var pluginConfigAllowedExtensions = map[string]bool{
	".json":       true,
	".xml":        true,
	".txt":        true,
	".yml":        true,
	".yaml":       true,
	".toml":       true,
	".ini":        true,
	".conf":       true,
	".properties": true,
}

var pluginConfigExcludedDirs = map[string]bool{
	"serverplugins": true,
	"logs":          true,
	"backups":       true,
}

type pluginConfigFile struct {
	Name         string  `json:"name"`
	RelativePath string  `json:"relativePath"`
	Size         int64   `json:"size"`
	ModTime      int64   `json:"modTime"`
	Scope        string  `json:"scope"`
	Format       string  `json:"format"`
	PluginName   *string `json:"pluginName"`
	ReloadHint   string  `json:"reloadHint"`
	IsMain       bool    `json:"isMain"`
}

type pluginConfigContentResponse struct {
	Filename     string  `json:"filename"`
	RelativePath string  `json:"relativePath"`
	Scope        string  `json:"scope"`
	Format       string  `json:"format"`
	PluginName   *string `json:"pluginName"`
	ReloadHint   string  `json:"reloadHint"`
	IsMain       bool    `json:"isMain"`
	Content      string  `json:"content"`
	Size         int     `json:"size"`
}

type pluginConfigClassification struct {
	Scope      string
	PluginName *string
	ReloadHint string
	IsMain     bool
}

func GetPluginConfigs(c *gin.Context) {
	files, err := discoverPluginConfigFiles(pluginConfigRoot())
	if err != nil {
		log.Printf("[ERROR] Failed to scan plugin config files: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取配置目录失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"files": files,
	}))
}

func GetPluginConfigContent(c *gin.Context) {
	getPluginConfigContentByPath(c, c.Param("filename"))
}

func GetPluginConfigContentByQuery(c *gin.Context) {
	getPluginConfigContentByPath(c, c.Query("path"))
}

func SavePluginConfig(c *gin.Context) {
	savePluginConfigByPath(c, c.Param("filename"))
}

func SavePluginConfigByQuery(c *gin.Context) {
	savePluginConfigByPath(c, c.Query("path"))
}

func getPluginConfigContentByPath(c *gin.Context, requestedPath string) {
	configPath, relativePath, err := resolvePluginConfigPath(requestedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
		return
	}

	fileInfo, err := os.Stat(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("配置文件不存在"))
			return
		}
		log.Printf("[ERROR] Failed to stat config file %s: %v", relativePath, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取配置文件失败"))
		return
	}
	if fileInfo.IsDir() {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("不能读取目录"))
		return
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("[ERROR] Failed to read config file %s: %v", relativePath, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取配置文件失败"))
		return
	}

	metadata := buildPluginConfigFile(relativePath, fileInfo)
	if metadata.Format == "json" {
		var jsonCheck interface{}
		if err := json.Unmarshal(content, &jsonCheck); err != nil {
			log.Printf("[WARN] Config file %s is not valid JSON: %v", relativePath, err)
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(pluginConfigContentResponse{
		Filename:     metadata.Name,
		RelativePath: metadata.RelativePath,
		Scope:        metadata.Scope,
		Format:       metadata.Format,
		PluginName:   metadata.PluginName,
		ReloadHint:   metadata.ReloadHint,
		IsMain:       metadata.IsMain,
		Content:      string(content),
		Size:         len(content),
	}))
}

func savePluginConfigByPath(c *gin.Context, requestedPath string) {
	configPath, relativePath, err := resolvePluginConfigPath(requestedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
		return
	}

	var req struct {
		Content   string `json:"content" binding:"required"`
		HotReload bool   `json:"hotReload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请求参数错误: "+err.Error()))
		return
	}

	fileInfo, err := os.Stat(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, models.ErrorResponse("配置文件不存在"))
			return
		}
		log.Printf("[ERROR] Failed to stat config file %s: %v", relativePath, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存配置文件失败"))
		return
	}
	if fileInfo.IsDir() {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("不能保存目录"))
		return
	}

	metadata := buildPluginConfigFile(relativePath, fileInfo)
	if metadata.Format == "json" {
		var jsonCheck interface{}
		if err := json.Unmarshal([]byte(req.Content), &jsonCheck); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse("JSON 格式错误: "+err.Error()))
			return
		}
	}

	backupPath := configPath + ".backup"
	if err := copyFile(configPath, backupPath); err != nil {
		log.Printf("[WARN] Failed to backup config file %s: %v", relativePath, err)
	} else {
		log.Printf("[INFO] Backup created for config file %s", relativePath)
	}

	if err := os.WriteFile(configPath, []byte(req.Content), fileInfo.Mode()); err != nil {
		log.Printf("[ERROR] Failed to save config file %s: %v", relativePath, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存配置文件失败"))
		return
	}

	log.Printf("[INFO] Config file saved: %s", relativePath)

	if req.HotReload && metadata.ReloadHint == "reload-attempt" {
		p, exists := utils.GetProcess(0)
		if exists && p.IsRunning() {
			if err := p.SendCommand("reload"); err != nil {
				log.Printf("[ERROR] Failed to send reload command after saving %s: %v", relativePath, err)
				c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
					"message":  "配置已保存，但发送 reload 命令失败: " + err.Error(),
					"saved":    true,
					"reloaded": false,
				}))
				return
			}

			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"message":  "配置已保存，并已发送 reload 命令。是否真正生效取决于该插件是否支持重载。",
				"saved":    true,
				"reloaded": true,
			}))
			return
		}

		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"message":  "配置已保存（服务器未运行，未执行 reload 命令）",
			"saved":    true,
			"reloaded": false,
		}))
		return
	}

	messageText := "配置已保存"
	if metadata.ReloadHint == "restart-recommended" {
		messageText = "配置已保存，建议回到插件服首页重启服务端后生效"
	} else if metadata.ReloadHint == "unknown" {
		messageText = "配置已保存，是否需要 reload 或重启取决于该文件的实际用途"
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"message":  messageText,
		"saved":    true,
		"reloaded": false,
	}))
}

func discoverPluginConfigFiles(root string) ([]pluginConfigFile, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []pluginConfigFile{}, nil
		}
		return nil, err
	}
	if !rootInfo.IsDir() {
		return nil, errors.New("配置目录不存在")
	}

	var files []pluginConfigFile
	err = filepath.WalkDir(root, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			if currentPath != root && pluginConfigExcludedDirs[strings.ToLower(entry.Name())] {
				return filepath.SkipDir
			}
			return nil
		}

		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(root, currentPath)
		if err != nil {
			return err
		}

		normalizedPath := normalizePluginConfigRelativePath(relativePath)
		if normalizedPath == "" || normalizedPath == "." {
			return nil
		}

		if !pluginConfigAllowedExtensions[strings.ToLower(filepath.Ext(normalizedPath))] {
			return nil
		}

		files = append(files, buildPluginConfigFile(normalizedPath, fileInfo))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		leftRank := pluginConfigScopeRank(files[i].Scope)
		rightRank := pluginConfigScopeRank(files[j].Scope)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return strings.ToLower(files[i].RelativePath) < strings.ToLower(files[j].RelativePath)
	})

	return files, nil
}

func buildPluginConfigFile(relativePath string, fileInfo os.FileInfo) pluginConfigFile {
	normalizedPath := normalizePluginConfigRelativePath(relativePath)
	classification := classifyPluginConfig(normalizedPath)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(normalizedPath)), ".")

	return pluginConfigFile{
		Name:         path.Base(normalizedPath),
		RelativePath: normalizedPath,
		Size:         fileInfo.Size(),
		ModTime:      fileInfo.ModTime().Unix(),
		Scope:        classification.Scope,
		Format:       ext,
		PluginName:   classification.PluginName,
		ReloadHint:   classification.ReloadHint,
		IsMain:       classification.IsMain,
	}
}

func classifyPluginConfig(relativePath string) pluginConfigClassification {
	normalizedPath := strings.ToLower(normalizePluginConfigRelativePath(relativePath))

	switch normalizedPath {
	case "config.json":
		return pluginConfigClassification{
			Scope:      "tshock-core",
			ReloadHint: "restart-recommended",
			IsMain:     true,
		}
	case "sscconfig.json":
		return pluginConfigClassification{
			Scope:      "ssc",
			ReloadHint: "restart-recommended",
			IsMain:     true,
		}
	}

	pluginName := inferPluginName(relativePath)
	if pluginName != nil {
		return pluginConfigClassification{
			Scope:      "plugin",
			PluginName: pluginName,
			ReloadHint: "reload-attempt",
		}
	}

	return pluginConfigClassification{
		Scope:      "unknown",
		ReloadHint: "unknown",
	}
}

func inferPluginName(relativePath string) *string {
	normalizedPath := normalizePluginConfigRelativePath(relativePath)
	segments := strings.Split(normalizedPath, "/")
	if len(segments) > 1 && segments[0] != "" {
		name := segments[0]
		return &name
	}

	baseName := strings.TrimSuffix(path.Base(normalizedPath), path.Ext(normalizedPath))
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		return nil
	}

	return &baseName
}

func pluginConfigRoot() string {
	return filepath.Join(config.ServersDir, "tshock")
}

func resolvePluginConfigPath(requestedPath string) (string, string, error) {
	relativePath := normalizePluginConfigRelativePath(requestedPath)
	if relativePath == "" || relativePath == "." {
		return "", "", errors.New("缺少配置文件路径")
	}

	if !pluginConfigAllowedExtensions[strings.ToLower(filepath.Ext(relativePath))] {
		return "", "", errors.New("只允许读取和保存文本配置文件")
	}

	root := filepath.Clean(pluginConfigRoot())
	resolvedPath := filepath.Clean(filepath.Join(root, filepath.FromSlash(relativePath)))
	if resolvedPath != root && !strings.HasPrefix(resolvedPath, root+string(os.PathSeparator)) {
		return "", "", errors.New("非法的文件路径")
	}

	return resolvedPath, relativePath, nil
}

func normalizePluginConfigRelativePath(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if normalized == "" {
		return ""
	}

	cleaned := path.Clean("/" + normalized)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return ""
	}

	return cleaned
}

func pluginConfigScopeRank(scope string) int {
	switch scope {
	case "tshock-core":
		return 0
	case "ssc":
		return 1
	case "plugin":
		return 2
	default:
		return 3
	}
}
