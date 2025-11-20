package models
import "time"
type Room struct {
	ID          int        `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	ServerType  string     `json:"serverType" db:"server_type"`
	WorldFile   string     `json:"worldFile" db:"world_file"`
	Port        int        `json:"port" db:"port"`
	MaxPlayers  int        `json:"maxPlayers" db:"max_players"`
	Password    string     `json:"password,omitempty" db:"password"`
	ModProfile  string     `json:"modProfile,omitempty" db:"mod_profile"`
	WorldSize   string     `json:"worldSize,omitempty" db:"world_size"`
	Difficulty  string     `json:"difficulty,omitempty" db:"difficulty"`
	EvilType    string     `json:"evilType,omitempty" db:"evil_type"`
	Status      string     `json:"status" db:"status"`
	PID         int        `json:"pid,omitempty" db:"pid"`
	StartTime   *time.Time `json:"startTime,omitempty" db:"start_time"`
	AdminToken  string     `json:"adminToken,omitempty" db:"admin_token"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time  `json:"updatedAt" db:"updated_at"`
	CustomHome  string     `json:"-" db:"-"`
}
type Player struct {
	ID        int       `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	IP        string    `json:"ip" db:"ip"`
	RoomID    int       `json:"roomId" db:"room_id"`
	RoomName  string    `json:"roomName" db:"room_name"`
	Status    string    `json:"status" db:"status"`
	Team      int       `json:"team" db:"team"`
	IsBanned  bool      `json:"isBanned" db:"is_banned"`
	LastSeen  time.Time `json:"lastSeen" db:"last_seen"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}
type User struct {
	ID        int       `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	Password  string    `json:"-" db:"password"`
	Role      string    `json:"role" db:"role"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
type OperationLog struct {
	ID         int       `json:"id" db:"id"`
	UserID     int       `json:"userId" db:"user_id"`
	Action     string    `json:"action" db:"action"`
	TargetType string    `json:"targetType" db:"target_type"`
	TargetID   int       `json:"targetId" db:"target_id"`
	Details    string    `json:"details" db:"details"`
	IPAddress  string    `json:"ipAddress" db:"ip_address"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
}
