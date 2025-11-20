package api
import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"terraria-panel/models"
	"terraria-panel/scheduler"
	"terraria-panel/storage"
	"github.com/gin-gonic/gin"
)
var (
	taskStorage storage.TaskStorage
	taskScheduler *scheduler.Scheduler
)
func InitTaskScheduler(ts storage.TaskStorage, sch *scheduler.Scheduler) {
	taskStorage = ts
	taskScheduler = sch
}
func GetTasks(c *gin.Context) {
	tasks, err := taskStorage.GetAll()
	if err != nil {
		log.Printf("[Task API] Failed to get tasks: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("获取任务列表失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(tasks))
}
func GetTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	task, err := taskStorage.GetByID(id)
	if err != nil {
		log.Printf("[Task API] Failed to get task %d: %v", id, err)
		c.JSON(http.StatusNotFound, models.ErrorResponse("任务不存在"))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(task))
}
func CreateTask(c *gin.Context) {
	var req struct {
		Name           string                 `json:"name" binding:"required"`
		Type           string                 `json:"type" binding:"required"`
		CronExpression string                 `json:"cronExpression" binding:"required"`
		Params         map[string]interface{} `json:"params"`
		Description    string                 `json:"description"`
		Enabled        *bool                  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}
	if req.Type != "backup" && req.Type != "restart" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务类型"))
		return
	}
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数格式错误"))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	task := &models.ScheduledTask{
		Name:           req.Name,
		Type:           req.Type,
		Enabled:        enabled,
		CronExpression: req.CronExpression,
		Params:         string(paramsJSON),
		Description:    req.Description,
	}
	if err := taskStorage.Create(task); err != nil {
		log.Printf("[Task API] Failed to create task: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建任务失败: "+err.Error()))
		return
	}
	if task.Enabled {
		if err := taskScheduler.AddTask(task); err != nil {
			log.Printf("[Task API] Failed to add task to scheduler: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("添加任务到调度器失败: "+err.Error()))
			return
		}
	}
	log.Printf("[Task API] Task created successfully: %d (%s)", task.ID, task.Name)
	c.JSON(http.StatusOK, models.SuccessResponse(task))
}
func UpdateTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	var req struct {
		Name           string                 `json:"name"`
		Type           string                 `json:"type"`
		CronExpression string                 `json:"cronExpression"`
		Params         map[string]interface{} `json:"params"`
		Description    string                 `json:"description"`
		Enabled        *bool                  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}
	task, err := taskStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("任务不存在"))
		return
	}
	if req.Name != "" {
		task.Name = req.Name
	}
	if req.Type != "" {
		if req.Type != "backup" && req.Type != "restart" {
			c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务类型"))
			return
		}
		task.Type = req.Type
	}
	if req.CronExpression != "" {
		task.CronExpression = req.CronExpression
	}
	if req.Params != nil {
		paramsJSON, err := json.Marshal(req.Params)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse("参数格式错误"))
			return
		}
		task.Params = string(paramsJSON)
	}
	if req.Description != "" {
		task.Description = req.Description
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if err := taskStorage.Update(task); err != nil {
		log.Printf("[Task API] Failed to update task: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("更新任务失败: "+err.Error()))
		return
	}
	if err := taskScheduler.ReloadTask(id); err != nil {
		log.Printf("[Task API] Failed to reload task in scheduler: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("重新加载任务失败: "+err.Error()))
		return
	}
	log.Printf("[Task API] Task updated successfully: %d (%s)", task.ID, task.Name)
	c.JSON(http.StatusOK, models.SuccessResponse(task))
}
func DeleteTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	taskScheduler.RemoveTask(id)
	if err := taskStorage.Delete(id); err != nil {
		log.Printf("[Task API] Failed to delete task: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除任务失败: "+err.Error()))
		return
	}
	log.Printf("[Task API] Task deleted successfully: %d", id)
	c.JSON(http.StatusOK, models.MessageResponse("任务删除成功"))
}
func ToggleTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}
	task, err := taskStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("任务不存在"))
		return
	}
	task.Enabled = req.Enabled
	if err := taskStorage.Update(task); err != nil {
		log.Printf("[Task API] Failed to update task: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("更新任务失败: "+err.Error()))
		return
	}
	if err := taskScheduler.ReloadTask(id); err != nil {
		log.Printf("[Task API] Failed to reload task in scheduler: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("重新加载任务失败: "+err.Error()))
		return
	}
	status := "禁用"
	if req.Enabled {
		status = "启用"
	}
	log.Printf("[Task API] Task %s successfully: %d", status, id)
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"enabled": req.Enabled,
		"message": fmt.Sprintf("任务已%s", status),
	}))
}
func ExecuteTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	_, err = taskStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("任务不存在"))
		return
	}
	if err := taskScheduler.ExecuteTaskNow(id); err != nil {
		log.Printf("[Task API] Failed to execute task: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("执行任务失败: "+err.Error()))
		return
	}
	log.Printf("[Task API] Task execution triggered: %d", id)
	c.JSON(http.StatusOK, models.MessageResponse("任务已触发执行"))
}
func GetTaskLogs(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	logs, err := taskStorage.GetLogs(id, limit)
	if err != nil {
		log.Printf("[Task API] Failed to get task logs: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("获取任务日志失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(logs))
}
func DeleteTaskLogs(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的任务 ID"))
		return
	}
	if err := taskStorage.DeleteLogs(id); err != nil {
		log.Printf("[Task API] Failed to delete task logs: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除任务日志失败: "+err.Error()))
		return
	}
	log.Printf("[Task API] Task logs deleted successfully: %d", id)
	c.JSON(http.StatusOK, models.MessageResponse("任务日志已清空"))
}
