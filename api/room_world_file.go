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
	entries, err := os.ReadDir(roomDir)
	if err != nil {
		return ""
	}

	worldExt := roomWorldExtension(serverType)
	detected := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), worldExt) {
			continue
		}
		if detected != "" {
			return ""
		}
		detected = entry.Name()
	}
	return detected
}

func normalizeRoomWorldFileForDir(serverType string, worldFile string, roomDir string) string {
	if strings.TrimSpace(worldFile) == "" {
		if detected := detectSingleRoomWorldFile(roomDir, serverType); detected != "" {
			return normalizeRoomWorldFile(serverType, detected)
		}
	}
	return normalizeRoomWorldFile(serverType, worldFile)
}
