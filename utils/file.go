package utils
import (
	"encoding/json"
	"os"
	"path/filepath"
)
func ReadJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}
func WriteJSON(path string, v interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
