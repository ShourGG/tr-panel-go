package models
import "time"
type PlayerStats struct {
	ID             int        `json:"id"`
	PlayerID       int        `json:"playerId"`
	PlayerName     string     `json:"playerName,omitempty"`
	TotalPlayTime  int        `json:"totalPlayTime"`
	LoginCount     int        `json:"loginCount"`
	LastLoginTime  *time.Time `json:"lastLoginTime,omitempty"`
	LastLogoutTime *time.Time `json:"lastLogoutTime,omitempty"`
	FirstSeen      time.Time  `json:"firstSeen"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
func (s *PlayerStats) GetPlayTimeString() string {
	return formatDuration(s.TotalPlayTime)
}
type PlayerDailyStats struct {
	ID            int       `json:"id"`
	Date          string    `json:"date"`
	TotalPlayers  int       `json:"totalPlayers"`
	ActivePlayers int       `json:"activePlayers"`
	NewPlayers    int       `json:"newPlayers"`
	TotalPlayTime int       `json:"totalPlayTime"`
	CreatedAt     time.Time `json:"createdAt"`
}
type StatsOverview struct {
	TotalPlayers   int `json:"totalPlayers"`
	OnlinePlayers  int `json:"onlinePlayers"`
	TodayActive    int `json:"todayActive"`
	WeekActive     int `json:"weekActive"`
	MonthActive    int `json:"monthActive"`
	BannedPlayers  int `json:"bannedPlayers"`
}
type PlayerRanking struct {
	Rank       int    `json:"rank"`
	PlayerID   int    `json:"playerId"`
	PlayerName string `json:"playerName"`
	Value      int    `json:"value"`
	ValueStr   string `json:"valueStr,omitempty"`
}
type TrendData struct {
	Dates          []string `json:"dates"`
	ActivePlayers  []int    `json:"activePlayers"`
	TotalPlayTime  []int    `json:"totalPlayTime"`
}
type RoomDistribution struct {
	RoomID      int    `json:"roomId"`
	RoomName    string `json:"roomName"`
	PlayerCount int    `json:"playerCount"`
}
type PlayerDetail struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	IP             string     `json:"ip,omitempty"`
	RoomID         int        `json:"roomId"`
	RoomName       string     `json:"roomName,omitempty"`
	Status         string     `json:"status"`
	TotalPlayTime  int        `json:"totalPlayTime"`
	PlayTimeStr    string     `json:"playTimeStr,omitempty"`
	LoginCount     int        `json:"loginCount"`
	LastLoginTime  *time.Time `json:"lastLoginTime,omitempty"`
	LastLogoutTime *time.Time `json:"lastLogoutTime,omitempty"`
	FirstSeen      time.Time  `json:"firstSeen"`
	IsBanned       bool       `json:"isBanned"`
}
