package api
import (
	"net/http"
	"os"
	"path/filepath"
	"terraria-panel/config"
	"terraria-panel/models"
	"github.com/gin-gonic/gin"
)
func ListFiles(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		relativePath = "."
	}
	fullPath := filepath.Join(config.DataDir, relativePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"path":   relativePath,
			"files":  []gin.H{},
			"exists": false,
		}))
		return
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取目录失败: "+err.Error()))
		return
	}
	files := []gin.H{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, gin.H{
			"name":  entry.Name(),
			"isDir": entry.IsDir(),
			"size":  info.Size(),
		})
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"path":   relativePath,
		"files":  files,
		"exists": true,
	}))
}
func ReadFile(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少文件路径"))
		return
	}
	fullPath := filepath.Join(config.DataDir, relativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取文件失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"path":    relativePath,
		"content": string(content),
	}))
}
func WriteFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	fullPath := filepath.Join(config.DataDir, req.Path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建目录失败"))
		return
	}
	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("写入文件失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("文件保存成功"))
}
func UploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("获取文件失败"))
		return
	}
	targetPath := c.PostForm("path")
	if targetPath == "" {
		targetPath = "."
	}
	fullPath := filepath.Join(config.DataDir, targetPath, file.Filename)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建目录失败"))
		return
	}
	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存文件失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("文件上传成功"))
}
func DeleteFile(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少文件路径"))
		return
	}
	fullPath := filepath.Join(config.DataDir, relativePath)
	if err := os.Remove(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除文件失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("文件删除成功"))
}
