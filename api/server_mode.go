package api
import (
	"net/http"
	"terraria-panel/db"
	"github.com/gin-gonic/gin"
)
func GetServerMode(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}
	var serverMode string
	err := db.DB.QueryRow("SELECT server_mode FROM users WHERE username = ?", username).Scan(&serverMode)
	if err != nil {
		serverMode = "rooms"
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"mode": serverMode,
		},
	})
}
func UpdateServerMode(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}
	var req struct {
		Mode string `json:"mode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	if req.Mode != "rooms" && req.Mode != "plugin-server" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的模式，仅支持 rooms 或 plugin-server",
		})
		return
	}
	_, err := db.DB.Exec("UPDATE users SET server_mode = ? WHERE username = ?", req.Mode, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新失败",
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "模式切换成功",
		"data": gin.H{
			"mode": req.Mode,
		},
	})
}
