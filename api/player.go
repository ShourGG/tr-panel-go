package api
import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/utils"
	"github.com/gin-gonic/gin"
)
type BannedPlayer struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Reason string `json:"reason"`
}
func GetPlayers(c *gin.Context) {
	players := parseOnlinePlayersFromLogs()
	c.JSON(http.StatusOK, models.SuccessResponse(players))
}
func parseOnlinePlayersFromLogs() []models.Player {
	players := []models.Player{}
	playerID := 1
	rooms, err := roomStorage.GetAll()
	if err != nil {
		return players
	}
	for _, room := range rooms {
		if room.Status != "running" {
			continue
		}
		logFile := filepath.Join(config.DataDir, "logs", "room-"+strconv.Itoa(room.ID)+".log")
		content, err := os.ReadFile(logFile)
		if err != nil {
			continue
		}
		lines := string(content)
		playerMap := make(map[string]bool)
		for _, line := range splitLines(lines) {
			if contains(line, "has joined") {
				playerName := extractPlayerName(line, "has joined")
				if playerName != "" {
					playerMap[playerName] = true
				}
			} else if contains(line, "has left") || contains(line, "disconnected") {
				playerName := extractPlayerName(line, "has left")
				if playerName != "" {
					playerMap[playerName] = false
				}
			}
		}
		for name, isOnline := range playerMap {
			if isOnline {
				players = append(players, models.Player{
					ID:       playerID,
					Name:     name,
					IP:       "未知",
					RoomID:   room.ID,
					RoomName: room.Name,
					Status:   "在线",
					IsBanned: false,
				})
				playerID++
			}
		}
	}
	return players
}
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
func extractPlayerName(line, keyword string) string {
	idx := strings.Index(line, keyword)
	if idx == -1 {
		return ""
	}
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return ""
	}
	nameStart := colonIdx + 1
	nameEnd := idx
	if nameStart >= nameEnd {
		return ""
	}
	name := strings.TrimSpace(line[nameStart:nameEnd])
	if parenIdx := strings.Index(name, "("); parenIdx != -1 {
		name = strings.TrimSpace(name[:parenIdx])
	}
	return name
}
func KickPlayer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的玩家ID"))
		return
	}
	var players []models.Player
	if err := utils.ReadJSON(config.PlayersFile, &players); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取玩家列表失败"))
		return
	}
	var playerName string
	for _, p := range players {
		if p.ID == id {
			playerName = p.Name
			break
		}
	}
	if playerName == "" {
		c.JSON(http.StatusNotFound, models.ErrorResponse("玩家不存在"))
		return
	}
	commandFile := filepath.Join(config.DataDir, "commands.txt")
	command := "kick " + playerName + "\n"
	f, err := os.OpenFile(commandFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(command)
		f.Close()
	}
	c.JSON(http.StatusOK, models.MessageResponse("已发送踢出命令: "+playerName))
}
func BanPlayer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的玩家ID"))
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&req)
	var players []models.Player
	if err := utils.ReadJSON(config.PlayersFile, &players); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取玩家列表失败"))
		return
	}
	var targetPlayer *models.Player
	for i := range players {
		if players[i].ID == id {
			targetPlayer = &players[i]
			players[i].IsBanned = true
			break
		}
	}
	if targetPlayer == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("玩家不存在"))
		return
	}
	if err := utils.WriteJSON(config.PlayersFile, players); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存失败"))
		return
	}
	banFile := filepath.Join(config.DataDir, "banned.json")
	var bannedList []BannedPlayer
	utils.ReadJSON(banFile, &bannedList)
	exists := false
	for _, banned := range bannedList {
		if banned.ID == id {
			exists = true
			break
		}
	}
	if !exists {
		bannedList = append(bannedList, BannedPlayer{
			ID:     targetPlayer.ID,
			Name:   targetPlayer.Name,
			IP:     targetPlayer.IP,
			Reason: req.Reason,
		})
		utils.WriteJSON(banFile, bannedList)
	}
	commandFile := filepath.Join(config.DataDir, "commands.txt")
	command := "ban " + targetPlayer.Name + " " + req.Reason + "\n"
	f, err := os.OpenFile(commandFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(command)
		f.Close()
	}
	c.JSON(http.StatusOK, models.MessageResponse("玩家 "+targetPlayer.Name+" 已封禁"))
}
func GetBannedPlayers(c *gin.Context) {
	banFile := filepath.Join(config.DataDir, "banned.json")
	var bannedList []BannedPlayer
	if err := utils.ReadJSON(banFile, &bannedList); err != nil {
		bannedList = []BannedPlayer{}
	}
	c.JSON(http.StatusOK, models.SuccessResponse(bannedList))
}
func UnbanPlayer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的玩家ID"))
		return
	}
	var players []models.Player
	if err := utils.ReadJSON(config.PlayersFile, &players); err == nil {
		for i := range players {
			if players[i].ID == id {
				players[i].IsBanned = false
				break
			}
		}
		utils.WriteJSON(config.PlayersFile, players)
	}
	banFile := filepath.Join(config.DataDir, "banned.json")
	var bannedList []BannedPlayer
	utils.ReadJSON(banFile, &bannedList)
	newList := []BannedPlayer{}
	var playerName string
	for _, banned := range bannedList {
		if banned.ID != id {
			newList = append(newList, banned)
		} else {
			playerName = banned.Name
		}
	}
	utils.WriteJSON(banFile, newList)
	if playerName != "" {
		commandFile := filepath.Join(config.DataDir, "commands.txt")
		command := "unban " + playerName + "\n"
		f, err := os.OpenFile(commandFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString(command)
			f.Close()
		}
	}
	c.JSON(http.StatusOK, models.MessageResponse("玩家已解封"))
}
