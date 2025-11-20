package api
import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"terraria-panel/services"
	"time"
	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/go-sqlite"
)
func getTShockDBPath() string {
	possiblePaths := []string{
		filepath.Join(services.GetGlobalTShockDir(), "tshock.sqlite"),
		filepath.Join(services.GetPluginServerDir(), "tshock", "tshock.sqlite"),
		filepath.Join("data", "servers", "tshock", "tshock.sqlite"),
		filepath.Join("servers", "tshock", "tshock.sqlite"),
		filepath.Join("tshock", "tshock.sqlite"),
		"tshock.sqlite",
	}
	for _, path := range possiblePaths {
		if fileExists(path) {
			return path
		}
	}
	return possiblePaths[0]
}
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
type TShockUser struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	UUID         string `json:"uuid"`
	Usergroup    string `json:"usergroup"`
	Registered   string `json:"registered"`
	LastAccessed string `json:"lastAccessed"`
	KnownIPs     string `json:"knownIPs"`
}
type TShockBan struct {
	TicketNumber int    `json:"ticketNumber"`
	Identifier   string `json:"identifier"`
	Reason       string `json:"reason"`
	BanningUser  string `json:"banningUser"`
	Date         int64  `json:"date"`
	Expiration   int64  `json:"expiration"`
	DateStr       string `json:"dateStr"`
	ExpirationStr string `json:"expirationStr"`
	IsActive      bool   `json:"isActive"`
	BanType       string `json:"banType"`
}
type TShockRegion struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Z      int    `json:"z"`
	Owner  string `json:"owner"`
	Groups string `json:"groups"`
	Users  string `json:"users"`
	Locked int    `json:"locked"`
}
type TShockWarp struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	WorldID int    `json:"worldId"`
}
type TShockLog struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	IP       string `json:"ip"`
	Command  string `json:"command"`
	Date     string `json:"date"`
}
func GetTShockUsers(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法打开TShock数据库",
			"error":   err.Error(),
			"dbPath":  dbPath,
		})
		return
	}
	defer db.Close()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	search := c.Query("search")
	group := c.Query("group")
	offset := (page - 1) * pageSize
	query := "SELECT ID, Username, Password, UUID, Usergroup, Registered, LastAccessed, KnownIPs FROM Users WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM Users WHERE 1=1"
	args := []interface{}{}
	if search != "" {
		query += " AND (Username LIKE ? OR UUID LIKE ?)"
		countQuery += " AND (Username LIKE ? OR UUID LIKE ?)"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
	}
	if group != "" {
		query += " AND Usergroup = ?"
		countQuery += " AND Usergroup = ?"
		args = append(args, group)
	}
	var total int
	countArgs := args
	err = db.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	query += " ORDER BY ID DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	defer rows.Close()
	users := []TShockUser{}
	for rows.Next() {
		var user TShockUser
		err := rows.Scan(&user.ID, &user.Username, &user.Password, &user.UUID, 
			&user.Usergroup, &user.Registered, &user.LastAccessed, &user.KnownIPs)
		if err != nil {
			continue
		}
		users = append(users, user)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"users": users,
			"total": total,
			"page":  page,
			"pageSize": pageSize,
		},
	})
}
func GetTShockBans(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	activeOnly := c.DefaultQuery("activeOnly", "false") == "true"
	banType := c.Query("banType")
	offset := (page - 1) * pageSize
	query := "SELECT TicketNumber, Identifier, Reason, BanningUser, Date, Expiration FROM PlayerBans WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM PlayerBans WHERE 1=1"
	args := []interface{}{}
	if activeOnly {
		now := time.Now().Unix() * 10000000
		query += " AND Expiration > ?"
		countQuery += " AND Expiration > ?"
		args = append(args, now)
	}
	if banType != "" {
		query += " AND Identifier LIKE ?"
		countQuery += " AND Identifier LIKE ?"
		args = append(args, banType+":%")
	}
	var total int
	countArgs := args
	err = db.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	query += " ORDER BY Date DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	defer rows.Close()
	bans := []TShockBan{}
	for rows.Next() {
		var ban TShockBan
		err := rows.Scan(&ban.TicketNumber, &ban.Identifier, &ban.Reason, 
			&ban.BanningUser, &ban.Date, &ban.Expiration)
		if err != nil {
			continue
		}
		ban.DateStr = ticksToTime(ban.Date).Format("2006-01-02 15:04:05")
		expirationTime := ticksToTime(ban.Expiration)
		if expirationTime.Year() > 9000 {
			ban.ExpirationStr = "永久"
		} else {
			ban.ExpirationStr = expirationTime.Format("2006-01-02 15:04:05")
		}
		ban.IsActive = time.Now().Before(expirationTime)
		if len(ban.Identifier) > 0 {
			if ban.Identifier[0:3] == "ip:" {
				ban.BanType = "ip"
			} else if ban.Identifier[0:5] == "uuid:" {
				ban.BanType = "uuid"
			} else if ban.Identifier[0:5] == "name:" {
				ban.BanType = "name"
			} else if ban.Identifier[0:4] == "acc:" {
				ban.BanType = "acc"
			}
		}
		bans = append(bans, ban)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"bans":     bans,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}
func GetTShockRegions(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	rows, err := db.Query("SELECT ID, Name, X, Y, Width, Height, Z, Owner, Groups, Users, Locked FROM Regions ORDER BY ID DESC")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"regions": []TShockRegion{},
				"total":   0,
			},
			"message": "Regions表不存在或查询失败",
		})
		return
	}
	defer rows.Close()
	regions := []TShockRegion{}
	for rows.Next() {
		var region TShockRegion
		err := rows.Scan(&region.ID, &region.Name, &region.X, &region.Y, 
			&region.Width, &region.Height, &region.Z, &region.Owner, 
			&region.Groups, &region.Users, &region.Locked)
		if err != nil {
			continue
		}
		regions = append(regions, region)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"regions": regions,
			"total":   len(regions),
		},
	})
}
func GetTShockWarps(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	rows, err := db.Query("SELECT ID, Name, X, Y, WorldID FROM Warps ORDER BY Name")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"warps": []TShockWarp{},
				"total": 0,
			},
			"message": "Warps表不存在或查询失败",
		})
		return
	}
	defer rows.Close()
	warps := []TShockWarp{}
	for rows.Next() {
		var warp TShockWarp
		err := rows.Scan(&warp.ID, &warp.Name, &warp.X, &warp.Y, &warp.WorldID)
		if err != nil {
			continue
		}
		warps = append(warps, warp)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"warps": warps,
			"total": len(warps),
		},
	})
}
func GetTShockLogs(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	username := c.Query("username")
	offset := (page - 1) * pageSize
	query := "SELECT ID, Username, IP, Command, Date FROM Logs WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM Logs WHERE 1=1"
	args := []interface{}{}
	if username != "" {
		query += " AND Username LIKE ?"
		countQuery += " AND Username LIKE ?"
		args = append(args, "%"+username+"%")
	}
	var total int
	countArgs := args
	err = db.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"logs":     []TShockLog{},
				"total":    0,
				"page":     page,
				"pageSize": pageSize,
			},
			"message": "Logs表不存在或查询失败",
		})
		return
	}
	query += " ORDER BY ID DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	defer rows.Close()
	logs := []TShockLog{}
	for rows.Next() {
		var log TShockLog
		err := rows.Scan(&log.ID, &log.Username, &log.IP, &log.Command, &log.Date)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"logs":     logs,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}
func UpdateTShockUser(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	var req struct {
		ID        int    `json:"id"`
		UUID      string `json:"uuid"`
		Usergroup string `json:"usergroup"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	_, err = db.Exec("UPDATE Users SET UUID = ?, Usergroup = ? WHERE ID = ?", 
		req.UUID, req.Usergroup, req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}
func DeleteTShockUser(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	id := c.Param("id")
	_, err = db.Exec("DELETE FROM Users WHERE ID = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}
func RemoveTShockBan(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	ticketNumber := c.Param("ticketNumber")
	now := time.Now().Unix() * 10000000
	_, err = db.Exec("UPDATE PlayerBans SET Expiration = ? WHERE TicketNumber = ?", now, ticketNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解封失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "解封成功",
	})
}
func AddTShockBan(c *gin.Context) {
	dbPath := getTShockDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开TShock数据库: " + err.Error()})
		return
	}
	defer db.Close()
	var req struct {
		Identifier  string `json:"identifier"`
		Reason      string `json:"reason"`
		BanningUser string `json:"banningUser"`
		Duration    int    `json:"duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	now := time.Now()
	dateTicksSince1970 := int64(now.Unix() * 10000000) + 621355968000000000
	var expirationTicks int64
	if req.Duration == 0 {
		expirationTicks = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC).Unix() * 10000000
	} else {
		expirationTime := now.Add(time.Duration(req.Duration) * time.Minute)
		expirationTicks = int64(expirationTime.Unix() * 10000000) + 621355968000000000
	}
	_, err = db.Exec("INSERT INTO PlayerBans (Identifier, Reason, BanningUser, Date, Expiration) VALUES (?, ?, ?, ?, ?)",
		req.Identifier, req.Reason, req.BanningUser, dateTicksSince1970, expirationTicks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "添加封禁失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "封禁成功",
	})
}
func ticksToTime(ticks int64) time.Time {
	const ticksToUnixEpoch = 621355968000000000
	unixSeconds := (ticks - ticksToUnixEpoch) / 10000000
	return time.Unix(unixSeconds, 0)
}
func GetTShockStats(c *gin.Context) {
	dbPath := getTShockDBPath()
	if !fileExists(dbPath) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "TShock数据库文件不存在",
			"dbPath":  dbPath,
			"hint":    "请先启动插件服，TShock会自动创建数据库文件",
		})
		return
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法打开TShock数据库",
			"error":   err.Error(),
			"dbPath":  dbPath,
		})
		return
	}
	defer db.Close()
	stats := gin.H{}
	var userCount int
	db.QueryRow("SELECT COUNT(*) FROM Users").Scan(&userCount)
	stats["userCount"] = userCount
	now := time.Now().Unix() * 10000000
	var activeBanCount int
	db.QueryRow("SELECT COUNT(*) FROM PlayerBans WHERE Expiration > ?", now).Scan(&activeBanCount)
	stats["activeBanCount"] = activeBanCount
	var totalBanCount int
	db.QueryRow("SELECT COUNT(*) FROM PlayerBans").Scan(&totalBanCount)
	stats["totalBanCount"] = totalBanCount
	var regionCount int
	db.QueryRow("SELECT COUNT(*) FROM Regions").Scan(&regionCount)
	stats["regionCount"] = regionCount
	var warpCount int
	db.QueryRow("SELECT COUNT(*) FROM Warps").Scan(&warpCount)
	stats["warpCount"] = warpCount
	rows, _ := db.Query("SELECT Usergroup, COUNT(*) as count FROM Users GROUP BY Usergroup")
	groupStats := []gin.H{}
	for rows.Next() {
		var group string
		var count int
		rows.Scan(&group, &count)
		groupStats = append(groupStats, gin.H{
			"group": group,
			"count": count,
		})
	}
	rows.Close()
	stats["groupStats"] = groupStats
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
