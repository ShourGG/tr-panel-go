package api
import (
	"database/sql"
	"net/http"
	"strconv"
	"terraria-panel/models"
	"terraria-panel/storage"
	"time"
	"github.com/gin-gonic/gin"
)
var (
	sessionStorage    storage.PlayerSessionStorage
	statsStorage      storage.PlayerStatsStorage
	dailyStatsStorage storage.PlayerDailyStatsStorage
	statsDB           *sql.DB
)
func InitStatsStorage(database *sql.DB) {
	statsDB = database
	sessionStorage = storage.NewSQLitePlayerSessionStorage(database)
	statsStorage = storage.NewSQLitePlayerStatsStorage(database)
	dailyStatsStorage = storage.NewSQLitePlayerDailyStatsStorage(database)
}
func GetStatsOverview(c *gin.Context) {
	var totalPlayers int
	err := statsDB.QueryRow("SELECT COUNT(*) FROM players").Scan(&totalPlayers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get total players"))
		return
	}
	var onlinePlayers int
	err = statsDB.QueryRow("SELECT COUNT(*) FROM players WHERE status = 'online'").Scan(&onlinePlayers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get online players"))
		return
	}
	today := time.Now().Format("2006-01-02")
	var todayActive int
	query := `
		SELECT COUNT(DISTINCT player_id)
		FROM player_sessions
		WHERE DATE(join_time) = ?
	`
	err = statsDB.QueryRow(query, today).Scan(&todayActive)
	if err != nil {
		todayActive = 0
	}
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var weekActive int
	query = `
		SELECT COUNT(DISTINCT player_id)
		FROM player_sessions
		WHERE DATE(join_time) >= ?
	`
	err = statsDB.QueryRow(query, weekAgo).Scan(&weekActive)
	if err != nil {
		weekActive = 0
	}
	monthAgo := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	var monthActive int
	query = `
		SELECT COUNT(DISTINCT player_id)
		FROM player_sessions
		WHERE DATE(join_time) >= ?
	`
	err = statsDB.QueryRow(query, monthAgo).Scan(&monthActive)
	if err != nil {
		monthActive = 0
	}
	var bannedPlayers int
	err = statsDB.QueryRow("SELECT COUNT(*) FROM players WHERE is_banned = 1").Scan(&bannedPlayers)
	if err != nil {
		bannedPlayers = 0
	}
	overview := models.StatsOverview{
		TotalPlayers:  totalPlayers,
		OnlinePlayers: onlinePlayers,
		TodayActive:   todayActive,
		WeekActive:    weekActive,
		MonthActive:   monthActive,
		BannedPlayers: bannedPlayers,
	}
	c.JSON(http.StatusOK, models.SuccessResponse(overview))
}
func GetRankings(c *gin.Context) {
	rankType := c.DefaultQuery("type", "playtime")
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 10
	}
	rankings := []*models.PlayerRanking{}
	if statsStorage == nil {
		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"rankings": rankings,
			"type":     rankType,
		}))
		return
	}
	if rankType == "playtime" {
		statsList, err := statsStorage.GetTopByPlayTime(limit)
		if err != nil {
			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"rankings": rankings,
				"type":     rankType,
			}))
			return
		}
		for i, stats := range statsList {
			rankings = append(rankings, &models.PlayerRanking{
				Rank:       i + 1,
				PlayerID:   stats.PlayerID,
				PlayerName: stats.PlayerName,
				Value:      stats.TotalPlayTime,
				ValueStr:   stats.GetPlayTimeString(),
			})
		}
	} else if rankType == "logincount" {
		statsList, err := statsStorage.GetTopByLoginCount(limit)
		if err != nil {
			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"rankings": rankings,
				"type":     rankType,
			}))
			return
		}
		for i, stats := range statsList {
			rankings = append(rankings, &models.PlayerRanking{
				Rank:       i + 1,
				PlayerID:   stats.PlayerID,
				PlayerName: stats.PlayerName,
				Value:      stats.LoginCount,
				ValueStr:   strconv.Itoa(stats.LoginCount) + " 次",
			})
		}
	} else if rankType == "recent" {
		statsList, err := statsStorage.GetRecentActive(limit)
		if err != nil {
			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"rankings": rankings,
				"type":     rankType,
			}))
			return
		}
		for i, stats := range statsList {
			valueStr := "从未登录"
			if stats.LastLoginTime != nil {
				valueStr = stats.LastLoginTime.Format("2006-01-02 15:04:05")
			}
			rankings = append(rankings, &models.PlayerRanking{
				Rank:       i + 1,
				PlayerID:   stats.PlayerID,
				PlayerName: stats.PlayerName,
				Value:      0,
				ValueStr:   valueStr,
			})
		}
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"rankings": rankings,
		"type":     rankType,
	}))
}
func GetPlayerList(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")
	sortBy := c.DefaultQuery("sortBy", "totalPlayTime")
	order := c.DefaultQuery("order", "desc")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	query := `
		SELECT 
			p.id, p.name, p.ip, p.room_id, p.status, p.is_banned, p.created_at,
			r.name as room_name,
			COALESCE(ps.total_play_time, 0) as total_play_time,
			COALESCE(ps.login_count, 0) as login_count,
			ps.last_login_time,
			ps.last_logout_time,
			COALESCE(ps.first_seen, p.created_at) as first_seen
		FROM players p
		LEFT JOIN rooms r ON p.room_id = r.id
		LEFT JOIN player_stats ps ON p.id = ps.player_id
	`
	orderClause := " ORDER BY "
	switch sortBy {
	case "totalPlayTime":
		orderClause += "total_play_time"
	case "loginCount":
		orderClause += "login_count"
	case "lastLogin":
		orderClause += "last_login_time"
	case "name":
		orderClause += "p.name"
	default:
		orderClause += "total_play_time"
	}
	if order == "asc" {
		orderClause += " ASC"
	} else {
		orderClause += " DESC"
	}
	query += orderClause + " LIMIT ? OFFSET ?"
	var total int
	countQuery := `SELECT COUNT(*) FROM players`
	err = statsDB.QueryRow(countQuery).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get total count"))
		return
	}
	rows, err := statsDB.Query(query, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get players"))
		return
	}
	defer rows.Close()
	players := []*models.PlayerDetail{}
	for rows.Next() {
		player := &models.PlayerDetail{}
		var roomName sql.NullString
		err := rows.Scan(
			&player.ID,
			&player.Name,
			&player.IP,
			&player.RoomID,
			&player.Status,
			&player.IsBanned,
			&player.FirstSeen,
			&roomName,
			&player.TotalPlayTime,
			&player.LoginCount,
			&player.LastLoginTime,
			&player.LastLogoutTime,
			&player.FirstSeen,
		)
		if err != nil {
			continue
		}
		if roomName.Valid {
			player.RoomName = roomName.String
		}
		player.PlayTimeStr = formatDuration(player.TotalPlayTime)
		players = append(players, player)
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"players":  players,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	}))
}
func GetTrends(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 || days > 90 {
		days = 7
	}
	trend := &models.TrendData{
		Dates:         []string{},
		ActivePlayers: []int{},
		TotalPlayTime: []int{},
	}
	if dailyStatsStorage == nil {
		c.JSON(http.StatusOK, models.SuccessResponse(trend))
		return
	}
	dailyStats, err := dailyStatsStorage.GetRecent(days)
	if err != nil {
		c.JSON(http.StatusOK, models.SuccessResponse(trend))
		return
	}
	for i := days - 1; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		trend.Dates = append(trend.Dates, date)
		found := false
		for _, stats := range dailyStats {
			if stats.Date == date {
				trend.ActivePlayers = append(trend.ActivePlayers, stats.ActivePlayers)
				trend.TotalPlayTime = append(trend.TotalPlayTime, stats.TotalPlayTime)
				found = true
				break
			}
		}
		if !found {
			trend.ActivePlayers = append(trend.ActivePlayers, 0)
			trend.TotalPlayTime = append(trend.TotalPlayTime, 0)
		}
	}
	c.JSON(http.StatusOK, models.SuccessResponse(trend))
}
func GetDistribution(c *gin.Context) {
	query := `
		SELECT r.id, r.name, COUNT(p.id) as player_count
		FROM rooms r
		LEFT JOIN players p ON r.id = p.room_id AND p.status != 'offline'
		GROUP BY r.id, r.name
		ORDER BY player_count DESC
	`
	rows, err := statsDB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get distribution"))
		return
	}
	defer rows.Close()
	distribution := []*models.RoomDistribution{}
	for rows.Next() {
		dist := &models.RoomDistribution{}
		err := rows.Scan(&dist.RoomID, &dist.RoomName, &dist.PlayerCount)
		if err != nil {
			continue
		}
		distribution = append(distribution, dist)
	}
	c.JSON(http.StatusOK, models.SuccessResponse(distribution))
}
func GetPlayerSessions(c *gin.Context) {
	playerIDStr := c.Param("id")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid player ID"))
		return
	}
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	sessions, total, err := sessionStorage.GetByPlayerID(playerID, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get sessions"))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"sessions": sessions,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	}))
}
func formatDuration(seconds int) string {
	if seconds < 60 {
		return "< 1分钟"
	}
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24
	if days > 0 {
		remainingHours := hours % 24
		if remainingHours > 0 {
			return strconv.Itoa(days) + "天" + strconv.Itoa(remainingHours) + "小时"
		}
		return strconv.Itoa(days) + "天"
	}
	if hours > 0 {
		remainingMinutes := minutes % 60
		if remainingMinutes > 0 {
			return strconv.Itoa(hours) + "小时" + strconv.Itoa(remainingMinutes) + "分钟"
		}
		return strconv.Itoa(hours) + "小时"
	}
	return strconv.Itoa(minutes) + "分钟"
}
