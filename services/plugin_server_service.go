package services
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
)
const (
	PluginServerID = 0
	PluginServerPort = 7778
	PluginServerName = "Plugin Server"
)
type PluginServerService struct {
	db *sql.DB
}
func NewPluginServerService(db *sql.DB) *PluginServerService {
	return &PluginServerService{
		db: db,
	}
}
func (s *PluginServerService) InitializePluginServer() error {
	log.Printf("[INFO] Initializing plugin server...")
	pluginServer, err := s.GetPluginServer()
	if err != nil {
		return fmt.Errorf("failed to check plugin server: %v", err)
	}
	if pluginServer != nil {
		log.Printf("[INFO] Plugin server already exists (Port: %d)", pluginServer.Port)
		if err := s.ensurePluginServerDirectories(); err != nil {
			return fmt.Errorf("failed to ensure plugin server directories: %v", err)
		}
		if err := s.InitializeConfigFile(); err != nil {
			return fmt.Errorf("failed to initialize config file: %v", err)
		}
		return nil
	}
	log.Printf("[WARN] Plugin server configuration not found, creating default...")
	if err := s.ensurePluginServerDirectories(); err != nil {
		return fmt.Errorf("failed to create plugin server directories: %v", err)
	}
	if err := s.InitializeConfigFile(); err != nil {
		return fmt.Errorf("failed to initialize config file: %v", err)
	}
	log.Printf("[INFO] Plugin server initialization complete")
	return nil
}
func (s *PluginServerService) ensurePluginServerDirectories() error {
	pluginServerDir := filepath.Join(config.DataDir, "plugin-server")
	globalTshockDir := filepath.Join(config.ServersDir, "tshock")
	globalPluginsDir := filepath.Join(globalTshockDir, "ServerPlugins")
	dirs := []string{
		pluginServerDir,
		globalTshockDir,
		globalPluginsDir,
		filepath.Join(globalTshockDir, "logs"),
		filepath.Join(globalTshockDir, "backups"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	log.Printf("[INFO] Plugin server directories ensured:")
	log.Printf("[INFO]   - Plugin server dir: %s", pluginServerDir)
	log.Printf("[INFO]   - Global TShock dir: %s", globalTshockDir)
	log.Printf("[INFO]   - Global plugins dir: %s", globalPluginsDir)
	return nil
}
func (s *PluginServerService) GetPluginServer() (*models.PluginServer, error) {
	var pluginServer models.PluginServer
	err := s.db.QueryRow(`
		SELECT id, name, port, max_players, password, world_file, status, pid, start_time, created_at, updated_at,
		       world_size, world_name, difficulty, seed, world_evil, server_name
		FROM plugin_server
		WHERE id = 1
	`).Scan(
		&pluginServer.ID,
		&pluginServer.Name,
		&pluginServer.Port,
		&pluginServer.MaxPlayers,
		&pluginServer.Password,
		&pluginServer.WorldFile,
		&pluginServer.Status,
		&pluginServer.PID,
		&pluginServer.StartTime,
		&pluginServer.CreatedAt,
		&pluginServer.UpdatedAt,
		&pluginServer.WorldSize,
		&pluginServer.WorldName,
		&pluginServer.Difficulty,
		&pluginServer.Seed,
		&pluginServer.WorldEvil,
		&pluginServer.ServerName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin server: %v", err)
	}
	log.Printf("[DEBUG] GetPluginServer - Database values:")
	log.Printf("[DEBUG]   Port: %d", pluginServer.Port)
	log.Printf("[DEBUG]   MaxPlayers: %d", pluginServer.MaxPlayers)
	log.Printf("[DEBUG]   ServerName: %s", pluginServer.ServerName)
	return &pluginServer, nil
}
func (s *PluginServerService) syncConfigValuesFromFile(pluginServer *models.PluginServer) {
	configPath := filepath.Join(config.ServersDir, "tshock", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("[DEBUG] Config file not found, using database values")
		return
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("[WARN] Failed to read config file for sync: %v", err)
		return
	}
	var configData map[string]interface{}
	if err := json.Unmarshal(data, &configData); err != nil {
		log.Printf("[WARN] Failed to parse config file for sync: %v", err)
		return
	}
	settings, ok := configData["Settings"].(map[string]interface{})
	if !ok {
		log.Printf("[WARN] Settings section not found in config file")
		return
	}
	if serverPort, ok := settings["ServerPort"].(float64); ok {
		pluginServer.Port = int(serverPort)
	}
	if maxSlots, ok := settings["MaxSlots"].(float64); ok {
		pluginServer.MaxPlayers = int(maxSlots)
	}
	if serverName, ok := settings["ServerName"].(string); ok {
		pluginServer.ServerName = serverName
	}
	if serverPassword, ok := settings["ServerPassword"].(string); ok {
		pluginServer.Password = serverPassword
	}
	log.Printf("[DEBUG] Synced FROM config.json - Port: %d, MaxPlayers: %d, ServerName: %s",
		pluginServer.Port, pluginServer.MaxPlayers, pluginServer.ServerName)
}
func (s *PluginServerService) SyncDatabaseToConfigFile(pluginServer *models.PluginServer) error {
	configPath := filepath.Join(config.ServersDir, "tshock", "config.json")
	log.Printf("[DEBUG] Syncing database config to: %s", configPath)
	log.Printf("[DEBUG] Database values - Port: %d, MaxPlayers: %d, ServerName: %s",
		pluginServer.Port, pluginServer.MaxPlayers, pluginServer.ServerName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}
	var configData map[string]interface{}
	if err := json.Unmarshal(data, &configData); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}
	settings, ok := configData["Settings"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("Settings section not found in config file")
	}
	needsCleaning := false
	for key := range configData {
		if key != "Settings" {
			needsCleaning = true
			log.Printf("[WARN] Found invalid top-level field in config.json: %s (will be cleaned)", key)
		}
	}
	var configStr string
	if needsCleaning {
		originalStr := string(data)
		settingsStart := strings.Index(originalStr, "\"Settings\"")
		if settingsStart == -1 {
			return fmt.Errorf("failed to find Settings in config.json")
		}
		braceStart := strings.Index(originalStr[settingsStart:], "{")
		if braceStart == -1 {
			return fmt.Errorf("failed to find opening brace for Settings")
		}
		braceStart += settingsStart
		braceCount := 0
		inString := false
		escaped := false
		braceEnd := -1
		for i := braceStart; i < len(originalStr); i++ {
			ch := originalStr[i]
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if !inString {
				if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
					if braceCount == 0 {
						braceEnd = i
						break
					}
				}
			}
		}
		if braceEnd == -1 {
			return fmt.Errorf("failed to find closing brace for Settings")
		}
		settingsContent := originalStr[braceStart : braceEnd+1]
		configStr = "{\n  \"Settings\": " + settingsContent + "\n}"
		log.Printf("[INFO] Cleaned config.json, removed top-level fields (preserved field order)")
	} else {
		configStr = string(data)
	}
	_, settingsHasPort := settings["ServerPort"]
	if !settingsHasPort {
		settingsRegex := regexp.MustCompile(`("Settings"\s*:\s*\{)`)
		configStr = settingsRegex.ReplaceAllString(configStr, fmt.Sprintf(`${1}%s"ServerPort": %d,`, "\n    ", pluginServer.Port))
		log.Printf("[INFO] Added ServerPort to Settings section: %d", pluginServer.Port)
	} else {
		portRegex := regexp.MustCompile(`("ServerPort"\s*:\s*)\d+`)
		configStr = portRegex.ReplaceAllString(configStr, fmt.Sprintf("${1}%d", pluginServer.Port))
		log.Printf("[INFO] Updated ServerPort in Settings section: %d", pluginServer.Port)
	}
	slotsRegex := regexp.MustCompile(`("MaxSlots"\s*:\s*)\d+`)
	configStr = slotsRegex.ReplaceAllString(configStr, fmt.Sprintf("${1}%d", pluginServer.MaxPlayers))
	escapedName := strings.ReplaceAll(pluginServer.ServerName, `"`, `\"`)
	nameRegex := regexp.MustCompile(`("ServerName"\s*:\s*)"[^"]*"`)
	configStr = nameRegex.ReplaceAllString(configStr, fmt.Sprintf(`${1}"%s"`, escapedName))
	escapedPassword := strings.ReplaceAll(pluginServer.Password, `"`, `\"`)
	passwordRegex := regexp.MustCompile(`("ServerPassword"\s*:\s*)"[^"]*"`)
	configStr = passwordRegex.ReplaceAllString(configStr, fmt.Sprintf(`${1}"%s"`, escapedPassword))
	log.Printf("[DEBUG] Updated config values - ServerPort: %d, MaxSlots: %d, ServerName: %s",
		pluginServer.Port, pluginServer.MaxPlayers, pluginServer.ServerName)
	if err := os.WriteFile(configPath, []byte(configStr), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	log.Printf("[INFO] Successfully synced database configuration to config.json")
	return nil
}
func (s *PluginServerService) UpdatePluginServerStatus(status string, pid int) error {
	var query string
	if status == "running" {
		query = `
			UPDATE plugin_server
			SET status = ?, pid = ?, start_time = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`
	} else {
		query = `
			UPDATE plugin_server
			SET status = ?, pid = ?, start_time = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`
	}
	_, err := s.db.Exec(query, status, pid)
	if err != nil {
		return fmt.Errorf("failed to update plugin server status: %v", err)
	}
	return nil
}
func (s *PluginServerService) UpdatePluginServerConfig(
	port, maxPlayers int,
	password, worldName string,
	worldSize, difficulty int,
	seed, worldEvil, serverName string,
	syncToFile bool,
) error {
	log.Printf("[DEBUG] UpdatePluginServerConfig SQL parameters:")
	log.Printf("[DEBUG]   port = %d", port)
	log.Printf("[DEBUG]   max_players = %d", maxPlayers)
	log.Printf("[DEBUG]   password = %s", password)
	log.Printf("[DEBUG]   world_name = %s", worldName)
	log.Printf("[DEBUG]   world_size = %d", worldSize)
	log.Printf("[DEBUG]   difficulty = %d", difficulty)
	log.Printf("[DEBUG]   seed = %s", seed)
	log.Printf("[DEBUG]   world_evil = %s", worldEvil)
	log.Printf("[DEBUG]   server_name = %s", serverName)
	worldFile := worldName + ".wld"
	log.Printf("[DEBUG]   world_file (generated) = %s", worldFile)
	result, err := s.db.Exec(`
		UPDATE plugin_server
		SET port = ?, max_players = ?, password = ?,
		    world_name = ?, world_size = ?, difficulty = ?,
		    seed = ?, world_evil = ?, server_name = ?,
		    world_file = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, port, maxPlayers, password, worldName, worldSize, difficulty, seed, worldEvil, serverName, worldFile)
	if err != nil {
		log.Printf("[ERROR] SQL execution failed: %v", err)
		return fmt.Errorf("failed to update plugin server config: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("[ERROR] Failed to get rows affected: %v", err)
		return fmt.Errorf("failed to get rows affected: %v", err)
	}
	log.Printf("[DEBUG] SQL UPDATE affected %d rows", rowsAffected)
	if rowsAffected == 0 {
		log.Printf("[WARNING] No rows were updated! Check if record with id=1 exists")
		return fmt.Errorf("no rows were updated, record with id=1 may not exist")
	}
	var verifyPort, verifyMaxPlayers int
	var verifyServerName string
	err = s.db.QueryRow(`
		SELECT port, max_players, server_name
		FROM plugin_server
		WHERE id = 1
	`).Scan(&verifyPort, &verifyMaxPlayers, &verifyServerName)
	if err != nil {
		log.Printf("[ERROR] Failed to verify update: %v", err)
		return fmt.Errorf("failed to verify update: %v", err)
	}
	log.Printf("[DEBUG] Verification query results:")
	log.Printf("[DEBUG]   port = %d (expected: %d)", verifyPort, port)
	log.Printf("[DEBUG]   max_players = %d (expected: %d)", verifyMaxPlayers, maxPlayers)
	log.Printf("[DEBUG]   server_name = %s (expected: %s)", verifyServerName, serverName)
	if verifyPort != port || verifyMaxPlayers != maxPlayers || verifyServerName != serverName {
		log.Printf("[ERROR] Verification failed! Database values don't match expected values")
		return fmt.Errorf("verification failed: database values don't match")
	}
	log.Printf("[INFO] Plugin server configuration updated and verified successfully")
	if !syncToFile {
		log.Printf("[INFO] Skipping config.json sync (syncToFile=false)")
		return nil
	}
	log.Printf("[INFO] Syncing updated configuration to config.json...")
	pluginServer, err := s.GetPluginServer()
	if err != nil {
		log.Printf("[ERROR] Failed to get plugin server for sync: %v", err)
		log.Printf("[WARNING] Configuration updated in database but not synced to config.json")
		return nil
	}
	if err := s.SyncDatabaseToConfigFile(pluginServer); err != nil {
		log.Printf("[ERROR] Failed to sync configuration to config.json: %v", err)
		log.Printf("[WARNING] Configuration updated in database but not synced to config.json")
		return nil
	}
	log.Printf("[INFO] Configuration synced to config.json successfully")
	return nil
}
func (s *PluginServerService) InitializeConfigFile() error {
	globalTshockDir := filepath.Join(config.ServersDir, "tshock")
	configPath := filepath.Join(globalTshockDir, "config.json")
	if err := os.MkdirAll(globalTshockDir, 0755); err != nil {
		log.Printf("[ERROR] Failed to create tshock directory: %v", err)
		return fmt.Errorf("failed to create tshock directory: %v", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		log.Printf("[INFO] TShock config.json already exists: %s", configPath)
		return nil
	}
	log.Printf("[INFO] TShock config.json not found, initializing from template...")
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("[ERROR] Failed to get executable path: %v", err)
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)
	log.Printf("[DEBUG] Executable directory: %s", execDir)
	possiblePaths := []string{
		filepath.Join(execDir, "config.json.template"),
		filepath.Join(execDir, "services", "templates", "config.json.template"),
		"services/templates/config.json.template",
	}
	var templateData []byte
	var templatePath string
	var foundTemplate bool
	for _, path := range possiblePaths {
		log.Printf("[DEBUG] Trying template path: %s", path)
		data, err := os.ReadFile(path)
		if err == nil {
			templateData = data
			templatePath = path
			foundTemplate = true
			log.Printf("[INFO] Template found at: %s", path)
			break
		} else {
			log.Printf("[DEBUG] Template not found at %s: %v", path, err)
		}
	}
	if !foundTemplate {
		log.Printf("[ERROR] Template file not found in any of the following locations:")
		for _, path := range possiblePaths {
			log.Printf("[ERROR]   - %s", path)
		}
		return fmt.Errorf("template file not found. Please ensure config.json.template exists in one of the expected locations")
	}
	log.Printf("[INFO] Using template from: %s", templatePath)
	log.Printf("[DEBUG] Writing config file to: %s", configPath)
	if err := os.WriteFile(configPath, templateData, 0644); err != nil {
		log.Printf("[ERROR] Failed to write config file: %v", err)
		return fmt.Errorf("failed to write config file: %v", err)
	}
	log.Printf("[INFO] TShock config.json initialized successfully: %s", configPath)
	return nil
}
func GetPluginServerDir() string {
	return filepath.Join(config.DataDir, "plugin-server")
}
func GetPluginServerPluginsDir() string {
	return filepath.Join(config.ServersDir, "tshock", "ServerPlugins")
}
func GetGlobalTShockDir() string {
	return filepath.Join(config.ServersDir, "tshock")
}
