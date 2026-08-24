package api

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
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

const activeSessionPlayTimeSQL = `
	COALESCE((
		SELECT SUM(
			CASE
				WHEN CAST(strftime('%s', 'now') AS INTEGER) > CAST(strftime('%s', active_session.join_time) AS INTEGER)
				THEN CAST(strftime('%s', 'now') AS INTEGER) - CAST(strftime('%s', active_session.join_time) AS INTEGER)
				ELSE 0
			END
		)
		FROM player_sessions active_session
		WHERE active_session.player_id = p.id AND active_session.leave_time IS NULL
	), 0)`

func InitStatsStorage(database *sql.DB) {
	statsDB = database
	sessionStorage = storage.NewSQLitePlayerSessionStorage(database)
	statsStorage = storage.NewSQLitePlayerStatsStorage(database)
	dailyStatsStorage = storage.NewSQLitePlayerDailyStatsStorage(database)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	dayStart := startOfDay(t)
	weekday := int(dayStart.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return dayStart.AddDate(0, 0, -(weekday - 1))
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func getActivityStatsInRange(start, end time.Time) (int, int, error) {
	if statsDB == nil || !end.After(start) {
		return 0, 0, nil
	}

	rows, err := statsDB.Query(`
		SELECT player_id, join_time, leave_time
		FROM player_sessions
		WHERE join_time < ? AND COALESCE(leave_time, ?) > ?
	`, end, end, start)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	activePlayers := make(map[int]struct{})
	totalPlayTime := 0

	for rows.Next() {
		var playerID int
		var joinTime time.Time
		var leaveTime sql.NullTime
		if err := rows.Scan(&playerID, &joinTime, &leaveTime); err != nil {
			return 0, 0, err
		}

		activePlayers[playerID] = struct{}{}

		effectiveStart := start
		if joinTime.After(effectiveStart) {
			effectiveStart = joinTime
		}

		effectiveEnd := end
		if leaveTime.Valid && leaveTime.Time.Before(effectiveEnd) {
			effectiveEnd = leaveTime.Time
		}

		if effectiveEnd.After(effectiveStart) {
			totalPlayTime += int(effectiveEnd.Sub(effectiveStart).Seconds())
		}
	}

	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	return len(activePlayers), totalPlayTime, nil
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
	now := time.Now()
	todayActive, _, err := getActivityStatsInRange(startOfDay(now), now)
	if err != nil {
		todayActive = 0
	}
	weekActive, _, err := getActivityStatsInRange(startOfWeek(now), now)
	if err != nil {
		weekActive = 0
	}
	monthActive, _, err := getActivityStatsInRange(startOfMonth(now), now)
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
		rankings, err = getLivePlaytimeRankings(limit)
		if err != nil {
			c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
				"rankings": rankings,
				"type":     rankType,
			}))
			return
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

func getLivePlaytimeRankings(limit int) ([]*models.PlayerRanking, error) {
	if statsDB == nil {
		return []*models.PlayerRanking{}, nil
	}

	rows, err := statsDB.Query(`
		SELECT
			p.id,
			p.name,
			COALESCE(player_stats.total_play_time, 0) + `+activeSessionPlayTimeSQL+` AS total_play_time
		FROM players p
		LEFT JOIN player_stats ON player_stats.player_id = p.id
		ORDER BY total_play_time DESC, p.name COLLATE NOCASE ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rankings := make([]*models.PlayerRanking, 0, limit)
	for rows.Next() {
		var playerID, playTime int
		var playerName string
		if err := rows.Scan(&playerID, &playerName, &playTime); err != nil {
			return nil, err
		}
		rankings = append(rankings, &models.PlayerRanking{
			Rank:       len(rankings) + 1,
			PlayerID:   playerID,
			PlayerName: playerName,
			Value:      playTime,
			ValueStr:   formatDuration(playTime),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rankings, nil
}
func GetPlayerList(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")
	sortBy := c.DefaultQuery("sortBy", "totalPlayTime")
	order := c.DefaultQuery("order", "desc")
	search := strings.TrimSpace(c.DefaultQuery("search", ""))
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	filters := make([]string, 0, 1)
	filterArgs := make([]interface{}, 0, 1)
	if search != "" {
		filters = append(filters, "p.name LIKE ? COLLATE NOCASE")
		filterArgs = append(filterArgs, "%"+search+"%")
	}

	query := `
		SELECT 
			p.id, p.name, COALESCE(p.ip, ''), p.room_id, p.status, p.is_banned, CAST(p.created_at AS TEXT),
			r.name as room_name,
			COALESCE(ps.total_play_time, 0) + ` + activeSessionPlayTimeSQL + ` as total_play_time,
			COALESCE(ps.login_count, 0) as login_count,
			ps.last_login_time,
			ps.last_logout_time,
			CAST(COALESCE(ps.first_seen, p.created_at) AS TEXT) as first_seen
		FROM players p
		LEFT JOIN rooms r ON p.room_id = r.id
		LEFT JOIN player_stats ps ON p.id = ps.player_id
	`
	if len(filters) > 0 {
		query += " WHERE " + strings.Join(filters, " AND ")
	}
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

	queryArgs := append(append([]interface{}{}, filterArgs...), pageSize, offset)
	var total int
	countQuery := `SELECT COUNT(*) FROM players p`
	if len(filters) > 0 {
		countQuery += " WHERE " + strings.Join(filters, " AND ")
	}
	err = statsDB.QueryRow(countQuery, filterArgs...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get total count"))
		return
	}
	rows, err := statsDB.Query(query, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("Failed to get players"))
		return
	}
	defer rows.Close()
	players := []*models.PlayerDetail{}
	for rows.Next() {
		player := &models.PlayerDetail{}
		var roomName sql.NullString
		var createdAtRaw sql.NullString
		var firstSeenRaw sql.NullString
		err := rows.Scan(
			&player.ID,
			&player.Name,
			&player.IP,
			&player.RoomID,
			&player.Status,
			&player.IsBanned,
			&createdAtRaw,
			&roomName,
			&player.TotalPlayTime,
			&player.LoginCount,
			&player.LastLoginTime,
			&player.LastLogoutTime,
			&firstSeenRaw,
		)
		if err != nil {
			log.Printf("[ERROR] 读取玩家统计记录失败: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取玩家统计记录失败: "+err.Error()))
			return
		}
		if roomName.Valid {
			player.RoomName = roomName.String
		}
		firstSeenValue := firstSeenRaw.String
		if firstSeenValue == "" {
			firstSeenValue = createdAtRaw.String
		}
		if parsed, ok := parsePlayerTime(firstSeenValue); ok {
			player.FirstSeen = *parsed
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
	rangeValue, granularity, bucketStarts := buildTrendBuckets(time.Now(), c.Query("range"), c.Query("granularity"), c.Query("days"))
	trend := &models.TrendData{
		Dates:         []string{},
		ActivePlayers: []int{},
		TotalPlayTime: []int{},
		Range:         rangeValue,
		Granularity:   granularity,
	}
	now := time.Now()
	for _, bucketStart := range bucketStarts {
		bucketEnd := nextTrendBucket(bucketStart, granularity)
		if bucketEnd.After(now) {
			bucketEnd = now
		}
		trend.Dates = append(trend.Dates, formatTrendBucket(bucketStart, granularity))

		activePlayers, totalPlayTime, statErr := getActivityStatsInRange(bucketStart, bucketEnd)
		if statErr != nil {
			trend.ActivePlayers = append(trend.ActivePlayers, 0)
			trend.TotalPlayTime = append(trend.TotalPlayTime, 0)
			continue
		}

		trend.ActivePlayers = append(trend.ActivePlayers, activePlayers)
		trend.TotalPlayTime = append(trend.TotalPlayTime, totalPlayTime)
	}
	c.JSON(http.StatusOK, models.SuccessResponse(trend))
}

func buildTrendBuckets(now time.Time, rangeRaw, granularityRaw, legacyDaysRaw string) (string, string, []time.Time) {
	rangeRaw = strings.ToLower(strings.TrimSpace(rangeRaw))
	granularity := strings.ToLower(strings.TrimSpace(granularityRaw))

	if rangeRaw == "" {
		days, err := strconv.Atoi(strings.TrimSpace(legacyDaysRaw))
		if err != nil || days <= 0 || days > 90 {
			days = 7
		}
		rangeRaw = strconv.Itoa(days) + "d"
		if granularity == "" {
			granularity = "day"
		}
	}

	count := 0
	unit := ""
	if len(rangeRaw) >= 2 {
		unit = rangeRaw[len(rangeRaw)-1:]
		count, _ = strconv.Atoi(rangeRaw[:len(rangeRaw)-1])
	}
	validRange := count > 0 && ((unit == "h" && count <= 168) || (unit == "d" && count <= 366) || (unit == "w" && count <= 104) || (unit == "m" && count <= 60))
	if !validRange {
		rangeRaw = "7d"
		unit = "d"
		count = 7
	}

	if granularity == "" {
		switch unit {
		case "h":
			granularity = "hour"
		case "w":
			granularity = "week"
		case "m":
			granularity = "month"
		default:
			granularity = "day"
		}
	}
	if granularity != "hour" && granularity != "day" && granularity != "week" && granularity != "month" {
		granularity = "day"
	}

	bucketCount := count
	switch unit {
	case "h":
		if granularity != "hour" {
			bucketCount = 1
		}
	case "d":
		switch granularity {
		case "hour":
			bucketCount = count * 24
		case "week", "month":
			bucketCount = 1
		}
	case "w", "m":
		if (unit == "w" && granularity != "week") || (unit == "m" && granularity != "month") {
			bucketCount = 1
		}
	}

	currentStart := trendBucketStart(now, granularity)
	starts := make([]time.Time, 0, bucketCount)
	for i := bucketCount - 1; i >= 0; i-- {
		starts = append(starts, addTrendBuckets(currentStart, granularity, -i))
	}
	return rangeRaw, granularity, starts
}

func trendBucketStart(value time.Time, granularity string) time.Time {
	switch granularity {
	case "hour":
		return value.Truncate(time.Hour)
	case "week":
		return startOfWeek(value)
	case "month":
		return startOfMonth(value)
	default:
		return startOfDay(value)
	}
}

func addTrendBuckets(value time.Time, granularity string, count int) time.Time {
	switch granularity {
	case "hour":
		return value.Add(time.Duration(count) * time.Hour)
	case "week":
		return value.AddDate(0, 0, count*7)
	case "month":
		return value.AddDate(0, count, 0)
	default:
		return value.AddDate(0, 0, count)
	}
}

func nextTrendBucket(value time.Time, granularity string) time.Time {
	return addTrendBuckets(value, granularity, 1)
}

func formatTrendBucket(value time.Time, granularity string) string {
	switch granularity {
	case "hour":
		return value.Format("2006-01-02 15:00")
	case "month":
		return value.Format("2006-01")
	default:
		return value.Format("2006-01-02")
	}
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
