package api
import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"github.com/gin-gonic/gin"
)
type WorldCreateRequest struct {
	Name     string `json:"name" binding:"required"`
	Size     int    `json:"size" binding:"required"`
	Seed     string `json:"seed"`
	Filename string `json:"filename" binding:"required"`
}
func ListWorlds(c *gin.Context) {
	files, err := os.ReadDir(config.WorldsDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取世界目录失败"))
		return
	}
	var worlds []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".wld") {
			info, _ := file.Info()
			worlds = append(worlds, map[string]interface{}{
				"name": file.Name(),
				"size": info.Size(),
				"time": info.ModTime(),
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": worlds,
	})
}
func CreateWorld(c *gin.Context) {
	var req WorldCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}
	if req.Size < 1 || req.Size > 3 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("世界大小必须是 1(小)、2(中) 或 3(大)"))
		return
	}
	req.Filename = strings.TrimSpace(req.Filename)
	if req.Filename == "" || strings.ContainsAny(req.Filename, "/\\") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("文件名不合法"))
		return
	}
	if !strings.HasSuffix(req.Filename, ".wld") {
		req.Filename += ".wld"
	}
	worldPath := filepath.Join(config.WorldsDir, req.Filename)
	if _, err := os.Stat(worldPath); err == nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("世界文件已存在"))
		return
	}
	vanillaDir := filepath.Join(config.ServersDir, "vanilla")
	serverBin := filepath.Join(vanillaDir, "TerrariaServer.bin.x86_64")
	if _, err := os.Stat(serverBin); os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("未找到Terraria服务器文件，请先安装游戏"))
		return
	}
	args := []string{
		"-world", worldPath,
		"-autocreate", fmt.Sprintf("%d", req.Size),
		"-worldname", req.Name,
	}
	if req.Seed != "" {
		args = append(args, "-seed", req.Seed)
	}
	log.Printf("[DEBUG] 创建世界命令: %s %v", serverBin, args)
	cmd := exec.Command(serverBin, args...)
	cmd.Dir = vanillaDir
	logFile := filepath.Join(config.LogsDir, fmt.Sprintf("world-create-%s.log", req.Filename))
	logWriter, err := os.Create(logFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建日志文件失败"))
		return
	}
	defer logWriter.Close()
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	if err := cmd.Start(); err != nil {
		log.Printf("[ERROR] 启动世界创建进程失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("启动失败: "+err.Error()))
		return
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("[ERROR] 世界创建失败: %v", err)
		} else {
			log.Printf("[INFO] 世界创建完成: %s", req.Filename)
		}
	}()
	c.JSON(http.StatusOK, models.MessageResponse(fmt.Sprintf("正在创建世界 '%s'，这可能需要1-2分钟...", req.Name)))
}
func DeleteWorld(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("文件名不能为空"))
		return
	}
	worldPath := filepath.Join(config.WorldsDir, filename)
	if _, err := os.Stat(worldPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("世界文件不存在"))
		return
	}
	if err := os.Remove(worldPath); err != nil {
		log.Printf("[ERROR] 删除世界文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除失败: "+err.Error()))
		return
	}
	log.Printf("[INFO] 删除世界文件: %s", filename)
	c.JSON(http.StatusOK, models.MessageResponse("世界删除成功"))
}
