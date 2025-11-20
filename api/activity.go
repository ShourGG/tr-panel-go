package api
import (
	"fmt"
	"net/http"
	"terraria-panel/db"
	"terraria-panel/models"
	"github.com/gin-gonic/gin"
)
func GetRecentActivities(c *gin.Context) {
	limit := 10
	query := `
		SELECT id, type, title, description, room_id, player_name, color, created_at
		FROM activity_logs
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := db.DB.Query(query, limit)
	if err != nil {
		fmt.Printf("[ERROR] Failed to query activity logs: %v\n", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("查询活动日志失败"))
		return
	}
	defer rows.Close()
	var logs []models.ActivityLog
	for rows.Next() {
		var log models.ActivityLog
		err := rows.Scan(
			&log.ID, &log.Type, &log.Title, &log.Description,
			&log.RoomID, &log.PlayerName, &log.Color, &log.CreatedAt,
		)
		if err != nil {
			fmt.Printf("[ERROR] Failed to scan activity log: %v\n", err)
			continue
		}
		logs = append(logs, log)
	}
	if logs == nil {
		logs = []models.ActivityLog{}
	}
	c.JSON(http.StatusOK, models.SuccessResponse(logs))
}
func LogActivity(logType, title, description string, roomID *int, playerName, color string) error {
	query := `
		INSERT INTO activity_logs (type, title, description, room_id, player_name, color)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := db.DB.Exec(query, logType, title, description, roomID, playerName, color)
	if err != nil {
		fmt.Printf("[ERROR] Failed to log activity: %v\n", err)
		return err
	}
	fmt.Printf("[INFO] Activity logged: %s - %s\n", logType, title)
	return nil
}
func LogRoomStart(roomID int, roomName, serverType string, port int) {
	LogActivity(
		models.ActivityTypeRoomStart,
		fmt.Sprintf("房间 \"%s\" 已启动", roomName),
		fmt.Sprintf("端口: %d, 类型: %s", port, serverType),
		&roomID,
		"",
		models.ColorGreen,
	)
}
func LogRoomStop(roomID int, roomName string) {
	LogActivity(
		models.ActivityTypeRoomStop,
		fmt.Sprintf("房间 \"%s\" 已停止", roomName),
		"",
		&roomID,
		"",
		models.ColorOrange,
	)
}
func LogRoomRestart(roomID int, roomName string) {
	LogActivity(
		models.ActivityTypeRoomRestart,
		fmt.Sprintf("房间 \"%s\" 已重启", roomName),
		"",
		&roomID,
		"",
		models.ColorBlue,
	)
}
func LogPlayerJoin(roomID int, roomName, playerName string) {
	LogActivity(
		models.ActivityTypePlayerJoin,
		fmt.Sprintf("玩家 \"%s\" 加入了游戏", playerName),
		fmt.Sprintf("房间: %s", roomName),
		&roomID,
		playerName,
		models.ColorBlue,
	)
}
func LogPlayerLeave(roomID int, roomName, playerName string) {
	LogActivity(
		models.ActivityTypePlayerLeave,
		fmt.Sprintf("玩家 \"%s\" 离开了游戏", playerName),
		fmt.Sprintf("房间: %s", roomName),
		&roomID,
		playerName,
		models.ColorGray,
	)
}
func LogPlayerBan(playerName, reason string) {
	LogActivity(
		models.ActivityTypePlayerBan,
		fmt.Sprintf("玩家 \"%s\" 已被封禁", playerName),
		fmt.Sprintf("原因: %s", reason),
		nil,
		playerName,
		models.ColorRed,
	)
}
func LogPlayerUnban(playerName string) {
	LogActivity(
		models.ActivityTypePlayerUnban,
		fmt.Sprintf("玩家 \"%s\" 已被解封", playerName),
		"",
		nil,
		playerName,
		models.ColorGreen,
	)
}
func LogPlayerKick(roomID int, roomName, playerName, reason string) {
	LogActivity(
		models.ActivityTypePlayerKick,
		fmt.Sprintf("玩家 \"%s\" 已被踢出", playerName),
		fmt.Sprintf("房间: %s, 原因: %s", roomName, reason),
		&roomID,
		playerName,
		models.ColorOrange,
	)
}
func LogBackup(roomID int, roomName string) {
	LogActivity(
		models.ActivityTypeBackup,
		fmt.Sprintf("房间 \"%s\" 备份完成", roomName),
		"",
		&roomID,
		"",
		models.ColorGreen,
	)
}
func LogSystem(title, description string) {
	LogActivity(
		models.ActivityTypeSystem,
		title,
		description,
		nil,
		"",
		models.ColorBlue,
	)
}
func LogModInstall(modName string) {
	LogActivity(
		models.ActivityTypeModInstall,
		fmt.Sprintf("MOD \"%s\" 已安装", modName),
		"",
		nil,
		"",
		models.ColorGreen,
	)
}
func LogModDelete(modName string) {
	LogActivity(
		models.ActivityTypeModDelete,
		fmt.Sprintf("MOD \"%s\" 已删除", modName),
		"",
		nil,
		"",
		models.ColorRed,
	)
}
