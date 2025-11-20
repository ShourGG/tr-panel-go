package services
import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)
//go:embed templates/*
var templatesFS embed.FS
type ConfigService struct {
	tshockPath string
}
func NewConfigService(tshockPath string) *ConfigService {
	return &ConfigService{
		tshockPath: tshockPath,
	}
}
func (s *ConfigService) CheckConfigExists() bool {
	configPath := filepath.Join(s.tshockPath, "config.json")
	_, err := os.Stat(configPath)
	return err == nil
}
func (s *ConfigService) InitializeConfig() error {
	if err := os.MkdirAll(s.tshockPath, 0755); err != nil {
		return fmt.Errorf("failed to create tshock directory: %v", err)
	}
	if err := s.copyTemplate("config.json.template", "config.json"); err != nil {
		return fmt.Errorf("failed to copy config.json template: %v", err)
	}
	if err := s.copyTemplate("sscconfig.json.template", "sscconfig.json"); err != nil {
		return fmt.Errorf("failed to copy sscconfig.json template: %v", err)
	}
	if err := s.copyTemplate("motd.txt.template", "motd.txt"); err != nil {
		return fmt.Errorf("failed to copy motd.txt template: %v", err)
	}
	dirs := []string{"logs", "backups"}
	for _, dir := range dirs {
		dirPath := filepath.Join(s.tshockPath, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %v", dir, err)
		}
	}
	return nil
}
func (s *ConfigService) copyTemplate(templateName, targetName string) error {
	targetPath := filepath.Join(s.tshockPath, targetName)
	templatePath := filepath.Join("templates", templateName)
	data, err := templatesFS.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %v", templateName, err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write target file %s: %v", targetName, err)
	}
	return nil
}
func (s *ConfigService) GetConfig() (map[string]interface{}, error) {
	configPath := filepath.Join(s.tshockPath, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}
	return config, nil
}
func (s *ConfigService) GetConfigRaw() (string, error) {
	configPath := filepath.Join(s.tshockPath, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf("config file not found")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %v", err)
	}
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return "", fmt.Errorf("invalid JSON format: %v", err)
	}
	return string(data), nil
}
func (s *ConfigService) SaveConfig(config map[string]interface{}) error {
	configPath := filepath.Join(s.tshockPath, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		timestamp := time.Now().Format("20060102_150405")
		backupPath := configPath + ".backup." + timestamp
		if err := copyFile(configPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup config: %v", err)
		}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	return nil
}
func (s *ConfigService) SaveConfigRaw(rawConfig []byte) error {
	configPath := filepath.Join(s.tshockPath, "config.json")
	var temp interface{}
	if err := json.Unmarshal(rawConfig, &temp); err != nil {
		return fmt.Errorf("invalid JSON format: %v", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		timestamp := time.Now().Format("20060102_150405")
		backupPath := configPath + ".backup." + timestamp
		if err := copyFile(configPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup config: %v", err)
		}
	}
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, rawConfig, "", "  "); err != nil {
		return fmt.Errorf("failed to format JSON: %v", err)
	}
	if err := os.WriteFile(configPath, prettyJSON.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	return nil
}
func (s *ConfigService) ValidateConfig(config map[string]interface{}) []string {
	var errors []string
	settings, ok := config["Settings"].(map[string]interface{})
	if !ok {
		errors = append(errors, "Invalid config format: missing Settings field")
		return errors
	}
	if maxSlots, ok := settings["MaxSlots"].(float64); ok {
		if maxSlots < 1 || maxSlots > 255 {
			errors = append(errors, "MaxSlots must be between 1 and 255")
		}
	}
	if serverPort, ok := settings["ServerPort"].(float64); ok {
		if serverPort < 1024 || serverPort > 65535 {
			errors = append(errors, "ServerPort must be between 1024 and 65535")
		}
	}
	if respawnSeconds, ok := settings["RespawnSeconds"].(float64); ok {
		if respawnSeconds < 0 || respawnSeconds > 60 {
			errors = append(errors, "RespawnSeconds must be between 0 and 60")
		}
	}
	if maxHP, ok := settings["MaxHP"].(float64); ok {
		if maxHP < 100 || maxHP > 9999 {
			errors = append(errors, "MaxHP must be between 100 and 9999")
		}
	}
	if maxMP, ok := settings["MaxMP"].(float64); ok {
		if maxMP < 20 || maxMP > 9999 {
			errors = append(errors, "MaxMP must be between 20 and 9999")
		}
	}
	return errors
}
func (s *ConfigService) EnableRESTAPI() error {
	configPath := filepath.Join(s.tshockPath, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}
	settings, ok := config["Settings"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid config format: Settings not found")
	}
	settings["RestApiEnabled"] = true
	settings["RestApiPort"] = float64(7878)
	appTokens, ok := settings["ApplicationRestTokens"].(map[string]interface{})
	if !ok {
		appTokens = make(map[string]interface{})
		settings["ApplicationRestTokens"] = appTokens
	}
	tokenName := "panel-admin-token-2024"
	if _, exists := appTokens[tokenName]; !exists {
		appTokens[tokenName] = map[string]interface{}{
			"Username":      "",
			"UserGroupName": "superadmin",
		}
	}
	return s.SaveConfig(config)
}
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	return err
}
