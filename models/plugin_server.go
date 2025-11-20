package models
import "time"
type PluginServer struct {
	ID         int        `json:"id" db:"id"`
	Name       string     `json:"name" db:"name"`
	Port       int        `json:"port" db:"port"`
	MaxPlayers int        `json:"maxPlayers" db:"max_players"`
	Password   string     `json:"password,omitempty" db:"password"`
	WorldFile  string     `json:"worldFile" db:"world_file"`
	Status     string     `json:"status" db:"status"`
	PID        int        `json:"pid,omitempty" db:"pid"`
	StartTime  *time.Time `json:"startTime,omitempty" db:"start_time"`
	CreatedAt  time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time  `json:"updatedAt" db:"updated_at"`
	WorldSize  int    `json:"worldSize" db:"world_size"`
	WorldName  string `json:"worldName" db:"world_name"`
	Difficulty int    `json:"difficulty" db:"difficulty"`
	Seed       string `json:"seed" db:"seed"`
	WorldEvil  string `json:"worldEvil" db:"world_evil"`
	ServerName string `json:"serverName" db:"server_name"`
}
