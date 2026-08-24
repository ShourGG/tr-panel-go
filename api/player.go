package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/utils"
	"time"

	"github.com/gin-gonic/gin"
)

type PlayerListItem struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	IP           string     `json:"ip"`
	RoomID       int        `json:"roomId"`
	RoomName     string     `json:"roomName"`
	RoomType     string     `json:"roomType"`
	RoomTypeText string     `json:"roomTypeText"`
	LoginCount   int        `json:"loginCount"`
	PlayTime     int        `json:"playTime"`
	Status       string     `json:"status"`
	Online       bool       `json:"online"`
	Banned       bool       `json:"banned"`
	LastSeen     *time.Time `json:"lastSeen,omitempty"`
	JoinTime     *time.Time `json:"joinTime,omitempty"`
	BanReason    string     `json:"banReason,omitempty"`
	BanTime      string     `json:"banTime,omitempty"`
	Operator     string     `json:"operator,omitempty"`
}

type playerActionTarget struct {
	ID       int
	Name     string
	RoomID   int
	RoomName string
	RoomType string
	Status   string
	IsBanned bool
}

func GetPlayers(c *gin.Context) {
	statusFilter := strings.TrimSpace(strings.ToLower(c.DefaultQuery("status", "online")))
	roomID, err := parseOptionalRoomID(c.Query("roomId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}

	players, err := queryPlayers(statusFilter, roomID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(players))
}

func GetBannedPlayers(c *gin.Context) {
	roomID, err := parseOptionalRoomID(c.Query("roomId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}

	players, err := queryPlayers("banned", roomID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(players))
}

func KickPlayer(c *gin.Context) {
	player, err := getPlayerActionTarget(c.Param("id"))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err == errPlayerNotFound {
			statusCode = http.StatusNotFound
		} else if err == errInvalidPlayerID {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, models.ErrorResponse(err.Error()))
		return
	}

	if player.Status != "online" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("玩家当前不在线"))
		return
	}

	if err := dispatchPlayerCommand(player, fmt.Sprintf("kick %s", player.Name)); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("发送踢出命令失败: "+err.Error()))
		return
	}

	LogPlayerKick(player.RoomID, player.RoomName, player.Name, "面板操作")
	c.JSON(http.StatusOK, models.MessageResponse("已发送踢出命令: "+player.Name))
}

func BanPlayer(c *gin.Context) {
	player, err := getPlayerActionTarget(c.Param("id"))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err == errPlayerNotFound {
			statusCode = http.StatusNotFound
		} else if err == errInvalidPlayerID {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, models.ErrorResponse(err.Error()))
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "面板封禁"
	}

	if err := updatePlayerBanFlag(player.ID, true); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("更新封禁状态失败: "+err.Error()))
		return
	}

	message := "玩家 " + player.Name + " 已标记为封禁"
	if player.Status == "online" {
		if err := dispatchBanCommand(player, reason); err != nil {
			message += "，但未能同步服务器封禁命令: " + err.Error()
		} else {
			message += "，并已同步到运行中的服务器"
		}
	} else {
		message += "，目标玩家当前不在线"
	}

	LogPlayerBan(player.Name, reason)
	c.JSON(http.StatusOK, models.MessageResponse(message))
}

func UnbanPlayer(c *gin.Context) {
	player, err := getPlayerActionTarget(c.Param("id"))
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err == errPlayerNotFound {
			statusCode = http.StatusNotFound
		} else if err == errInvalidPlayerID {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, models.ErrorResponse(err.Error()))
		return
	}

	if err := updatePlayerBanFlag(player.ID, false); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("更新解封状态失败: "+err.Error()))
		return
	}

	message := "玩家 " + player.Name + " 已解除封禁标记"
	if player.Status == "online" {
		if err := dispatchUnbanCommand(player); err != nil {
			message += "，但未能同步服务器解封命令: " + err.Error()
		} else {
			message += "，并已同步到运行中的服务器"
		}
	}

	LogPlayerUnban(player.Name)
	c.JSON(http.StatusOK, models.MessageResponse(message))
}

var (
	errInvalidPlayerID = fmt.Errorf("无效的玩家ID")
	errPlayerNotFound  = fmt.Errorf("玩家不存在")
)

func parseOptionalRoomID(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	roomID, err := strconv.Atoi(raw)
	if err != nil || roomID < 0 {
		return 0, errInvalidPlayerID
	}
	return roomID, nil
}

func queryPlayers(statusFilter string, roomID int) ([]PlayerListItem, error) {
	filters := make([]string, 0, 3)
	args := make([]interface{}, 0, 3)

	if roomID > 0 {
		filters = append(filters, "p.room_id = ?")
		args = append(args, roomID)
	}

	switch statusFilter {
	case "", "online":
		filters = append(filters, "COALESCE(p.status, 'offline') = 'online'", "COALESCE(p.is_banned, 0) = 0")
	case "all":
		filters = append(filters, "COALESCE(p.is_banned, 0) = 0")
	case "banned":
		filters = append(filters, "COALESCE(p.is_banned, 0) = 1")
	case "offline":
		filters = append(filters, "COALESCE(p.status, 'offline') != 'online'", "COALESCE(p.is_banned, 0) = 0")
	default:
		return nil, fmt.Errorf("无效的玩家状态过滤条件")
	}

	query := `
		SELECT
			p.id,
			p.name,
			COALESCE(p.ip, ''),
			COALESCE(p.room_id, 0),
			COALESCE(r.name, ''),
			COALESCE(r.server_type, ''),
			COALESCE(ps.login_count, 0),
			COALESCE(ps.total_play_time, 0) + ` + activeSessionPlayTimeSQL + `,
			COALESCE(p.status, 'offline'),
			COALESCE(p.is_banned, 0),
			COALESCE(p.last_seen, p.created_at),
			(
				SELECT ps2.join_time
				FROM player_sessions ps2
				WHERE ps2.player_id = p.id AND ps2.leave_time IS NULL
				ORDER BY ps2.join_time DESC
				LIMIT 1
			)
		FROM players p
		LEFT JOIN rooms r ON p.room_id = r.id
		LEFT JOIN player_stats ps ON p.id = ps.player_id
	`
	if len(filters) > 0 {
		query += " WHERE " + strings.Join(filters, " AND ")
	}
	query += `
		ORDER BY
			CASE WHEN COALESCE(p.status, 'offline') = 'online' THEN 0 ELSE 1 END,
			COALESCE(ps.last_login_time, p.last_seen, p.created_at) DESC,
			p.name COLLATE NOCASE ASC
	`

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := make([]PlayerListItem, 0)
	for rows.Next() {
		var item PlayerListItem
		var isBanned bool
		var lastSeenRaw sql.NullString
		var joinTimeRaw sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.IP,
			&item.RoomID,
			&item.RoomName,
			&item.RoomType,
			&item.LoginCount,
			&item.PlayTime,
			&item.Status,
			&isBanned,
			&lastSeenRaw,
			&joinTimeRaw,
		); err != nil {
			return nil, err
		}

		item.Banned = isBanned
		item.Online = item.Status == "online"
		if parsed, ok := parsePlayerTime(lastSeenRaw.String); ok {
			item.LastSeen = parsed
		}
		if parsed, ok := parsePlayerTime(joinTimeRaw.String); ok {
			item.JoinTime = parsed
		}
		item.RoomTypeText = formatPlayerRoomType(item.RoomType)
		if item.Banned {
			item.BanReason, item.BanTime = getLatestBanInfo(item.Name)
		}
		players = append(players, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return players, nil
}

func parsePlayerTime(raw string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return &parsed, true
		}
	}
	return nil, false
}

func getLatestBanInfo(playerName string) (string, string) {
	var description sql.NullString
	var createdAt sql.NullTime
	err := db.DB.QueryRow(`
		SELECT description, created_at
		FROM activity_logs
		WHERE type = ? AND player_name = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, models.ActivityTypePlayerBan, playerName).Scan(&description, &createdAt)
	if err != nil {
		return "", ""
	}

	reason := strings.TrimSpace(description.String)
	reason = strings.TrimPrefix(reason, "原因: ")
	banTime := ""
	if createdAt.Valid {
		banTime = createdAt.Time.Format("2006-01-02 15:04:05")
	}
	return reason, banTime
}

func formatPlayerRoomType(roomType string) string {
	switch strings.ToLower(strings.TrimSpace(roomType)) {
	case "vanilla":
		return "原版"
	case "tmodloader", "tmod":
		return "tModLoader"
	case "tshock":
		return "TShock"
	default:
		return roomType
	}
}

func getPlayerActionTarget(rawID string) (*playerActionTarget, error) {
	id, err := strconv.Atoi(strings.TrimSpace(rawID))
	if err != nil || id <= 0 {
		return nil, errInvalidPlayerID
	}

	target := &playerActionTarget{}
	err = db.DB.QueryRow(`
		SELECT p.id, p.name, COALESCE(p.room_id, 0), COALESCE(r.name, ''), COALESCE(r.server_type, ''), COALESCE(p.status, 'offline'), COALESCE(p.is_banned, 0)
		FROM players p
		LEFT JOIN rooms r ON p.room_id = r.id
		WHERE p.id = ?
	`, id).Scan(&target.ID, &target.Name, &target.RoomID, &target.RoomName, &target.RoomType, &target.Status, &target.IsBanned)
	if err == sql.ErrNoRows {
		return nil, errPlayerNotFound
	}
	if err != nil {
		return nil, err
	}
	return target, nil
}

func updatePlayerBanFlag(playerID int, banned bool) error {
	_, err := db.DB.Exec(`UPDATE players SET is_banned = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?`, banned, playerID)
	return err
}

func dispatchPlayerCommand(player *playerActionTarget, command string) error {
	if player.RoomID <= 0 {
		return fmt.Errorf("玩家当前不在可操作的房间中")
	}

	room, err := roomStorage.GetByID(player.RoomID)
	if err != nil {
		return err
	}
	if room == nil {
		return fmt.Errorf("房间不存在")
	}
	if room.Status != "running" {
		return fmt.Errorf("目标房间当前未运行")
	}

	process, exists := utils.GetProcess(player.RoomID)
	if !exists || process == nil || !process.IsRunning() {
		return fmt.Errorf("目标房间进程不可用")
	}

	if !strings.HasSuffix(command, "\n") {
		command += "\n"
	}

	return process.SendCommand(command)
}

func normalizeRoomRuntimeType(roomType string) string {
	switch strings.ToLower(strings.TrimSpace(roomType)) {
	case "tshock":
		return "tshock"
	case "tmodloader", "tmod":
		return "tmodloader"
	default:
		return "vanilla"
	}
}

func dispatchBanCommand(player *playerActionTarget, reason string) error {
	switch normalizeRoomRuntimeType(player.RoomType) {
	case "tshock":
		command := fmt.Sprintf("/ban add %s", player.Name)
		trimmedReason := strings.TrimSpace(reason)
		if trimmedReason != "" {
			command += " " + trimmedReason
		}
		return dispatchPlayerCommand(player, command)
	case "tmodloader", "vanilla":
		return dispatchPlayerCommand(player, fmt.Sprintf("ban %s", player.Name))
	default:
		return dispatchPlayerCommand(player, fmt.Sprintf("ban %s", player.Name))
	}
}

func dispatchUnbanCommand(player *playerActionTarget) error {
	switch normalizeRoomRuntimeType(player.RoomType) {
	case "tshock":
		return dispatchPlayerCommand(player, fmt.Sprintf("/ban del %s", player.Name))
	case "tmodloader", "vanilla":
		return fmt.Errorf("%s 当前不支持通过控制台命令在线解封，请改为手动处理 banlist", formatPlayerRoomType(player.RoomType))
	default:
		return fmt.Errorf("当前服务器类型不支持在线解封")
	}
}
