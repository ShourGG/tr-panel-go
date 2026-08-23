package api

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

func roomWorldExtension(serverType string) string {
	if strings.EqualFold(strings.TrimSpace(serverType), "tmodloader") {
		return ".twld"
	}
	return ".wld"
}

func normalizeRoomWorldFile(serverType string, worldFile string) string {
	worldExt := roomWorldExtension(serverType)
	name := strings.TrimSpace(worldFile)
	if name == "" {
		return worldExt
	}

	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(path.Base(name))
	if name == "" || name == "." || name == "/" {
		return worldExt
	}

	lowerName := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lowerName, ".twld"):
		name = name[:len(name)-len(".twld")]
	case strings.HasSuffix(lowerName, ".wld"):
		name = name[:len(name)-len(".wld")]
	}
	return name + worldExt
}

func detectSingleRoomWorldFile(roomDir string, serverType string) string {
	worldExt := roomWorldExtension(serverType)
	detected := make(map[string]string)
	for _, worldDir := range roomWorldDirectories(roomDir, serverType) {
		entries, err := os.ReadDir(worldDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), worldExt) {
				continue
			}
			key := strings.ToLower(entry.Name())
			if _, exists := detected[key]; !exists {
				detected[key] = entry.Name()
			}
		}
	}
	if len(detected) != 1 {
		return ""
	}
	for _, name := range detected {
		return name
	}
	return ""
}

func roomWorldDirectories(roomDir string, serverType string) []string {
	if strings.EqualFold(strings.TrimSpace(serverType), "tmodloader") {
		return []string{filepath.Join(roomDir, "Worlds"), roomDir}
	}
	return []string{roomDir}
}

func normalizeRoomWorldFileForDir(serverType string, worldFile string, roomDir string) string {
	if strings.TrimSpace(worldFile) == "" {
		if detected := detectSingleRoomWorldFile(roomDir, serverType); detected != "" {
			return normalizeRoomWorldFile(serverType, detected)
		}
	}
	return normalizeRoomWorldFile(serverType, worldFile)
}
