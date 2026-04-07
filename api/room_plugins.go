package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/services"

	"github.com/gin-gonic/gin"
)

func GetRoomPlugins(c *gin.Context) {
	id, ok := parsePluginRoomID(c)
	if !ok {
		return
	}

	pluginsDir, ok := resolveRoomPluginsDir(c, id)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    collectPluginsFromDir(pluginsDir),
	})
}

func AddRoomPlugin(c *gin.Context) {
	id, ok := parsePluginRoomID(c)
	if !ok {
		return
	}

	pluginsDir, ok := resolveRoomPluginsDir(c, id)
	if !ok {
		return
	}

	file, ok := getUploadedPluginFile(c)
	if !ok {
		return
	}

	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件目录失败: "+err.Error()))
		return
	}

	dst := filepath.Join(pluginsDir, file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存插件失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已添加到房间 %d", file.Filename, id)))
}

func DeleteRoomPlugin(c *gin.Context) {
	id, ok := parsePluginRoomID(c)
	if !ok {
		return
	}

	pluginName := c.Param("plugin")
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请指定插件名称"))
		return
	}

	roomPluginsDir, ok := resolveRoomPluginsDir(c, id)
	if !ok {
		return
	}

	pluginPath := filepath.Join(roomPluginsDir, pluginName)
	if err := os.Remove(pluginPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除插件失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已从房间 %d 删除", pluginName, id)))
}

func CopyPluginFromShared(c *gin.Context) {
	id, ok := parsePluginRoomID(c)
	if !ok {
		return
	}

	var req struct {
		PluginName string `json:"pluginName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请求参数错误"))
		return
	}

	roomPluginsDir, ok := resolveRoomPluginsDir(c, id)
	if !ok {
		return
	}

	sharedPluginsDir := services.GetPluginServerPluginsDir()
	srcPath := filepath.Join(sharedPluginsDir, req.PluginName)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("共享插件不存在: "+req.PluginName))
		return
	}

	if err := os.MkdirAll(roomPluginsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件目录失败: "+err.Error()))
		return
	}

	dstPath := filepath.Join(roomPluginsDir, req.PluginName)
	srcFile, err := os.Open(srcPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取共享插件失败"))
		return
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建插件文件失败"))
		return
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("复制插件失败"))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("插件 %s 已从共享目录复制到房间 %d", req.PluginName, id)))
}

func GetSharedPlugins(c *gin.Context) {
	sharedPluginsDir := services.GetPluginServerPluginsDir()
	var plugins []map[string]interface{}
	if files, err := os.ReadDir(sharedPluginsDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".dll") {
				info, _ := file.Info()
				plugins = append(plugins, map[string]interface{}{
					"name": file.Name(),
					"size": info.Size(),
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"plugins": plugins,
		"count":   len(plugins),
	})
}

func parsePluginRoomID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return 0, false
	}

	return id, true
}

func resolveRoomPluginsDir(c *gin.Context, id int) (string, bool) {
	room, err := roomStorage.GetByID(id)
	if err != nil || room == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return "", false
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只有 TShock 服务器支持插件管理"))
		return "", false
	}

	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", id))
	roomTshockDir := filepath.Join(roomDir, "tshock")
	return filepath.Join(roomTshockDir, "ServerPlugins"), true
}

func collectPluginsFromDir(pluginsDir string) []map[string]interface{} {
	plugins := make([]map[string]interface{}, 0)
	disabledDir := filepath.Join(pluginsDir, "Disabled")

	if files, err := os.ReadDir(pluginsDir); err == nil {
		for _, file := range files {
			if file.IsDir() && file.Name() == "Disabled" {
				continue
			}
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".dll") {
				info, _ := file.Info()
				plugins = append(plugins, map[string]interface{}{
					"name":       file.Name(),
					"size":       info.Size(),
					"enabled":    true,
					"uploadTime": info.ModTime().Format("2006-01-02 15:04:05"),
				})
			}
		}
	}

	if files, err := os.ReadDir(disabledDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".dll") {
				info, _ := file.Info()
				plugins = append(plugins, map[string]interface{}{
					"name":       file.Name(),
					"size":       info.Size(),
					"enabled":    false,
					"uploadTime": info.ModTime().Format("2006-01-02 15:04:05"),
				})
			}
		}
	}

	return plugins
}

func getUploadedPluginFile(c *gin.Context) (*multipart.FileHeader, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		file, err = c.FormFile("plugin")
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse("请上传插件文件"))
			return nil, false
		}
	}

	if !strings.HasSuffix(strings.ToLower(file.Filename), ".dll") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只支持 .dll 文件"))
		return nil, false
	}

	return file, true
}
