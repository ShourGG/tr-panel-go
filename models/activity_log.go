package models
import "time"
type ActivityLog struct {
	ID          int       `json:"id" db:"id"`
	Type        string    `json:"type" db:"type"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description,omitempty" db:"description"`
	RoomID      *int      `json:"roomId,omitempty" db:"room_id"`
	PlayerName  string    `json:"playerName,omitempty" db:"player_name"`
	Color       string    `json:"color" db:"color"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}
const (
	ActivityTypeRoomStart    = "room_start"
	ActivityTypeRoomStop     = "room_stop"
	ActivityTypeRoomRestart  = "room_restart"
	ActivityTypePlayerJoin   = "player_join"
	ActivityTypePlayerLeave  = "player_leave"
	ActivityTypePlayerBan    = "player_ban"
	ActivityTypePlayerUnban  = "player_unban"
	ActivityTypePlayerKick   = "player_kick"
	ActivityTypeBackup       = "backup"
	ActivityTypeSystem       = "system"
	ActivityTypeModInstall   = "mod_install"
	ActivityTypeModDelete    = "mod_delete"
)
const (
	ColorGreen  = "green"
	ColorBlue   = "blue"
	ColorRed    = "red"
	ColorOrange = "orange"
	ColorPurple = "purple"
	ColorGray   = "gray"
)
