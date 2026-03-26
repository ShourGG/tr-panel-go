package utils

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const BackupManifestName = ".terraria-panel-backup.json"

var (
	backupArchiveNamePattern = regexp.MustCompile(`^room-(\d+)_(.+)_(\d{8}_\d{6})\.zip$`)
	backupUnsafeNamePattern  = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]+`)
)

type BackupManifest struct {
	Version    int    `json:"version"`
	CreatedAt  string `json:"createdAt"`
	RoomID     int    `json:"roomId"`
	RoomName   string `json:"roomName"`
	ServerType string `json:"serverType"`
	WorldFile  string `json:"worldFile"`
	BackupType string `json:"backupType"`
	Note       string `json:"note,omitempty"`
}

type BackupSummary struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	RoomID             int      `json:"roomId"`
	RoomName           string   `json:"roomName"`
	Type               string   `json:"type"`
	Size               int64    `json:"size"`
	CreatedAt          string   `json:"createdAt"`
	ServerType         string   `json:"serverType,omitempty"`
	WorldFile          string   `json:"worldFile,omitempty"`
	HasManifest        bool     `json:"hasManifest"`
	MetadataSource     string   `json:"metadataSource"`
	DetectedWorldFiles []string `json:"detectedWorldFiles,omitempty"`
}

func NewBackupManifest(roomID int, roomName, serverType, worldFile, backupType, note string, createdAt time.Time) BackupManifest {
	if backupType == "" {
		backupType = "full"
	}

	return BackupManifest{
		Version:    1,
		CreatedAt:  createdAt.Format(time.RFC3339),
		RoomID:     roomID,
		RoomName:   roomName,
		ServerType: serverType,
		WorldFile:  worldFile,
		BackupType: backupType,
		Note:       strings.TrimSpace(note),
	}
}

func BuildBackupArchiveName(roomID int, roomName string, createdAt time.Time) string {
	safeRoomName := sanitizeBackupBaseName(roomName)
	if safeRoomName == "" {
		safeRoomName = "room"
	}

	return fmt.Sprintf("room-%d_%s_%s.zip", roomID, safeRoomName, createdAt.Format("20060102_150405"))
}

func SanitizeBackupUploadName(name string) string {
	baseName := filepath.Base(strings.TrimSpace(name))
	if baseName == "" || baseName == "." {
		baseName = "backup.zip"
	}

	ext := filepath.Ext(baseName)
	nameOnly := strings.TrimSuffix(baseName, ext)
	nameOnly = sanitizeBackupBaseName(nameOnly)
	if nameOnly == "" {
		nameOnly = "backup"
	}
	if ext == "" {
		ext = ".zip"
	}

	return nameOnly + strings.ToLower(ext)
}

func NormalizeArchiveEntryPath(entryName string) (string, error) {
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(entryName), "\\", "/"))
	if cleaned == "." || cleaned == "" {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe archive path: %s", entryName)
	}

	return cleaned, nil
}

func ResolveArchiveExtractionPath(baseDir string, entryName string) (string, error) {
	relativePath, err := NormalizeArchiveEntryPath(entryName)
	if err != nil {
		return "", err
	}
	if relativePath == "" {
		return "", nil
	}

	baseDir = filepath.Clean(baseDir)
	targetPath := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(relativePath)))
	relativeToBase, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return "", err
	}
	if relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeToBase) {
		return "", fmt.Errorf("unsafe archive path: %s", entryName)
	}

	return targetPath, nil
}

func CreateBackupArchive(zipPath string, sourceDir string, manifest BackupManifest) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	if err := writeBackupManifest(zipWriter, manifest); err != nil {
		return err
	}

	return addDirToZip(zipWriter, sourceDir, "")
}

func AddDirToZip(zipWriter *zip.Writer, sourceDir string, baseInZip string) error {
	return addDirToZip(zipWriter, sourceDir, baseInZip)
}

func InspectBackupArchive(zipPath string) (BackupSummary, *BackupManifest, error) {
	info, err := os.Stat(zipPath)
	if err != nil {
		return BackupSummary{}, nil, err
	}

	fileName := filepath.Base(zipPath)
	summary := BackupSummary{
		ID:             strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		Name:           fileName,
		Type:           "full",
		Size:           info.Size(),
		CreatedAt:      info.ModTime().Format("2006-01-02 15:04:05"),
		MetadataSource: "unknown",
	}

	if roomID, roomName, createdAt, ok := parseBackupName(fileName); ok {
		summary.RoomID = roomID
		summary.RoomName = roomName
		summary.CreatedAt = createdAt
		summary.MetadataSource = "filename"
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return summary, nil, err
	}
	defer reader.Close()

	var manifest *BackupManifest
	worldFilesSet := map[string]struct{}{}
	entryNames := make([]string, 0, len(reader.File))

	for _, file := range reader.File {
		relativePath, err := NormalizeArchiveEntryPath(file.Name)
		if err != nil {
			return summary, manifest, err
		}
		if relativePath == "" {
			continue
		}

		entryNames = append(entryNames, relativePath)
		if file.FileInfo().IsDir() {
			continue
		}

		if relativePath == BackupManifestName {
			openedFile, err := file.Open()
			if err != nil {
				return summary, manifest, err
			}

			var decoded BackupManifest
			decodeErr := json.NewDecoder(openedFile).Decode(&decoded)
			openedFile.Close()
			if decodeErr != nil {
				return summary, manifest, decodeErr
			}

			manifest = &decoded
			continue
		}

		ext := strings.ToLower(filepath.Ext(relativePath))
		if ext == ".wld" || ext == ".twld" {
			worldFilesSet[filepath.Base(relativePath)] = struct{}{}
		}
	}

	if len(worldFilesSet) > 0 {
		summary.DetectedWorldFiles = make([]string, 0, len(worldFilesSet))
		for worldFile := range worldFilesSet {
			summary.DetectedWorldFiles = append(summary.DetectedWorldFiles, worldFile)
		}
		slices.Sort(summary.DetectedWorldFiles)
	}

	if manifest != nil {
		summary.HasManifest = true
		summary.MetadataSource = "manifest"
		summary.RoomID = manifest.RoomID
		summary.RoomName = manifest.RoomName
		summary.ServerType = manifest.ServerType
		summary.WorldFile = manifest.WorldFile
		if manifest.BackupType != "" {
			summary.Type = manifest.BackupType
		}
		if manifest.CreatedAt != "" {
			if parsedTime, err := time.Parse(time.RFC3339, manifest.CreatedAt); err == nil {
				summary.CreatedAt = parsedTime.Local().Format("2006-01-02 15:04:05")
			}
		}
	} else {
		summary.ServerType = detectBackupServerType(entryNames)
		if len(summary.DetectedWorldFiles) == 1 {
			summary.WorldFile = summary.DetectedWorldFiles[0]
		}
		if summary.ServerType != "" || summary.WorldFile != "" {
			summary.MetadataSource = "archive"
		}
	}

	return summary, manifest, nil
}

func writeBackupManifest(zipWriter *zip.Writer, manifest BackupManifest) error {
	manifest.Version = max(manifest.Version, 1)

	writer, err := zipWriter.Create(BackupManifestName)
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	_, err = writer.Write(encoded)
	return err
}

func addDirToZip(zipWriter *zip.Writer, sourceDir string, baseInZip string) error {
	return filepath.Walk(sourceDir, func(currentPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if currentPath == sourceDir {
			return nil
		}

		relativePath, err := filepath.Rel(sourceDir, currentPath)
		if err != nil {
			return err
		}

		archivePath := path.Join(baseInZip, filepath.ToSlash(relativePath))
		if info.IsDir() {
			_, err := zipWriter.Create(strings.TrimSuffix(archivePath, "/") + "/")
			return err
		}

		return addFileToZip(zipWriter, currentPath, archivePath)
	})
}

func addFileToZip(zipWriter *zip.Writer, filePath string, archivePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer, err := zipWriter.Create(archivePath)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

func detectBackupServerType(entryNames []string) string {
	for _, entryName := range entryNames {
		lowerPath := strings.ToLower(entryName)
		switch {
		case strings.Contains(lowerPath, "serverplugins/"),
			strings.HasSuffix(lowerPath, "config.json"),
			strings.HasSuffix(lowerPath, "motd.txt"),
			strings.Contains(lowerPath, "tshock.server"):
			return "tshock"
		}
	}

	for _, entryName := range entryNames {
		lowerPath := strings.ToLower(entryName)
		switch {
		case strings.Contains(lowerPath, "mods/"),
			strings.HasSuffix(lowerPath, "enabled.json"),
			strings.Contains(lowerPath, "tmodloader"):
			return "tmodloader"
		}
	}

	for _, entryName := range entryNames {
		lowerPath := strings.ToLower(entryName)
		if strings.HasSuffix(lowerPath, ".wld") || strings.HasSuffix(lowerPath, ".twld") || strings.HasSuffix(lowerPath, "config.txt") {
			return "vanilla"
		}
	}

	return ""
}

func parseBackupName(fileName string) (int, string, string, bool) {
	matches := backupArchiveNamePattern.FindStringSubmatch(fileName)
	if len(matches) != 4 {
		return 0, "", "", false
	}

	roomID, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", "", false
	}

	createdAt := ""
	if parsedTime, err := time.Parse("20060102_150405", matches[3]); err == nil {
		createdAt = parsedTime.Format("2006-01-02 15:04:05")
	}

	return roomID, matches[2], createdAt, true
}

func sanitizeBackupBaseName(name string) string {
	cleaned := backupUnsafeNamePattern.ReplaceAllString(strings.TrimSpace(name), "_")
	cleaned = strings.Trim(cleaned, " ._")
	return cleaned
}
