package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"terraria-panel/models"
	"terraria-panel/utils"
)

const maxRoomCommandLength = 4096

// SendRoomCommand forwards one console line to a running vanilla, tModLoader,
// or room-scoped TShock process. The process transport is deliberately kept in
// utils so the API does not need to know whether the process uses stdin or a PTY.
func SendRoomCommand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间 ID"))
		return
	}

	var req struct {
		Command string `json:"command" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("命令不能为空"))
		return
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("命令不能为空"))
		return
	}
	if len([]rune(command)) > maxRoomCommandLength {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(fmt.Sprintf("命令长度不能超过 %d 个字符", maxRoomCommandLength)))
		return
	}

	room, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取房间失败: "+err.Error()))
		return
	}
	if room == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}

	process, exists := utils.GetProcess(id)
	if !exists || process == nil || !process.IsRunning() {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("房间当前未运行"))
		return
	}
	if err := process.SendCommand(command); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("命令发送失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "命令已发送",
		"roomId":     id,
		"serverType": normalizeRoomRuntimeType(room.ServerType),
	})
}
