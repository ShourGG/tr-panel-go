package models
import "time"
type PlayerSession struct {
	ID         int       `json:"id"`
	PlayerID   int       `json:"playerId"`
	PlayerName string    `json:"playerName,omitempty"`
	RoomID     int       `json:"roomId"`
	RoomName   string    `json:"roomName,omitempty"`
	JoinTime   time.Time `json:"joinTime"`
	LeaveTime  *time.Time `json:"leaveTime,omitempty"`
	Duration   int       `json:"duration"`
	IPAddress  string    `json:"ipAddress,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
func (s *PlayerSession) IsOnline() bool {
	return s.LeaveTime == nil
}
func (s *PlayerSession) GetDurationString() string {
	if s.Duration == 0 && s.IsOnline() {
		duration := int(time.Since(s.JoinTime).Seconds())
		return formatDuration(duration)
	}
	return formatDuration(s.Duration)
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
			return formatInt(days) + "天" + formatInt(remainingHours) + "小时"
		}
		return formatInt(days) + "天"
	}
	if hours > 0 {
		remainingMinutes := minutes % 60
		if remainingMinutes > 0 {
			return formatInt(hours) + "小时" + formatInt(remainingMinutes) + "分钟"
		}
		return formatInt(hours) + "小时"
	}
	return formatInt(minutes) + "分钟"
}
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune(n%10 + '0')) + result
		n /= 10
	}
	return result
}
