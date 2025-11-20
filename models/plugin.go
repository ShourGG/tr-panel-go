package models
import "time"
type Plugin struct {
	Name        string    `json:"name"`
	FilePath    string    `json:"filePath"`
	Size        int64     `json:"size"`
	Enabled     bool      `json:"enabled"`
	UploadTime  time.Time `json:"uploadTime"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Author      string    `json:"author"`
}
type PluginStoreItem struct {
	Name         string            `json:"Name"`
	Version      string            `json:"Version"`
	Author       string            `json:"Author"`
	Description  map[string]string `json:"Description"`
	AssemblyName string            `json:"AssemblyName"`
	Path         string            `json:"Path"`
	Dependencies []string          `json:"Dependencies"`
	HotReload    bool              `json:"HotReload"`
	GitHubURL    string            `json:"GitHubURL"`
	Repository   string            `json:"Repository"`
}
type DownloadProgress struct {
	ID         string    `json:"id"`
	PluginName string    `json:"pluginName"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Message    string    `json:"message"`
	StartTime  time.Time `json:"startTime"`
}
type PluginUploadRequest struct {
	RoomID int `json:"roomId" binding:"required"`
}
type PluginToggleRequest struct {
	Enabled bool `json:"enabled" binding:"required"`
}
