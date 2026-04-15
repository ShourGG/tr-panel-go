package api

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"terraria-panel/config"
	"terraria-panel/models"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxArchiveEntries          = 5000
	maxArchiveSingleFileSize   = 256 << 20
	maxArchiveTotalExtractSize = 512 << 20
)

func resolveDataPath(paths ...string) (string, error) {
	base := filepath.Clean(config.DataDir)
	target := base

	for _, p := range paths {
		target = filepath.Join(target, p)
	}

	target = filepath.Clean(target)
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("illegal path")
	}

	return target, nil
}

func isConflictError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "同名文件")
}

func sanitizeZipEntryName(name string) (string, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if normalized == "" {
		return "", errors.New("empty entry")
	}

	cleanName := filepath.Clean(filepath.FromSlash(normalized))
	if cleanName == "." || cleanName == "" {
		return "", errors.New("empty entry")
	}
	if filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
		return "", errors.New("illegal entry path")
	}

	return cleanName, nil
}

func createZipHeaderFromFile(info os.FileInfo, archivePath string) (*zip.FileHeader, error) {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return nil, err
	}

	header.Name = filepath.ToSlash(archivePath)
	if info.IsDir() {
		header.Name += "/"
		header.Method = zip.Store
	} else {
		header.Method = zip.Deflate
	}

	return header, nil
}

func addPathToZip(zipWriter *zip.Writer, sourcePath string, archivePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	header, err := createZipHeaderFromFile(info, archivePath)
	if err != nil {
		return err
	}

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	return err
}

func streamZipArchive(c *gin.Context, sourcePath string, includeRoot bool) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", info.Name()))

	zipWriter := zip.NewWriter(c.Writer)
	defer zipWriter.Close()

	if !info.IsDir() {
		return addPathToZip(zipWriter, sourcePath, info.Name())
	}

	rootName := info.Name()
	if includeRoot {
		if err := addPathToZip(zipWriter, sourcePath, rootName); err != nil {
			return err
		}
	}

	return filepath.Walk(sourcePath, func(path string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourcePath {
			return nil
		}

		relativePath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}

		archivePath := relativePath
		if includeRoot {
			archivePath = filepath.Join(rootName, relativePath)
		}

		return addPathToZip(zipWriter, path, archivePath)
	})
}

func extractZipArchive(archivePath string, destinationDir string, overwrite bool) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return 0, errors.New("压缩包为空")
	}

	var (
		entryCount       int
		totalExtractSize uint64
		conflicts        []string
	)

	for _, file := range reader.File {
		cleanName, err := sanitizeZipEntryName(file.Name)
		if err != nil {
			return 0, fmt.Errorf("压缩包包含非法路径: %s", file.Name)
		}

		entryCount++
		if entryCount > maxArchiveEntries {
			return 0, fmt.Errorf("压缩包文件数量超过限制（最多 %d 个）", maxArchiveEntries)
		}

		if !file.FileInfo().IsDir() {
			if file.UncompressedSize64 > maxArchiveSingleFileSize {
				return 0, fmt.Errorf("压缩包内文件 %s 超过大小限制", cleanName)
			}
			totalExtractSize += file.UncompressedSize64
			if totalExtractSize > maxArchiveTotalExtractSize {
				return 0, fmt.Errorf("压缩包解压后总大小超过限制（最多 %d MB）", maxArchiveTotalExtractSize>>20)
			}
		}

		outputPath := filepath.Join(destinationDir, cleanName)
		relativeOutputPath, err := filepath.Rel(config.DataDir, outputPath)
		if err != nil {
			return 0, errors.New("压缩包包含越界路径")
		}
		if _, err := resolveDataPath(relativeOutputPath); err != nil {
			return 0, errors.New("压缩包包含越界路径")
		}

		if !overwrite {
			if _, err := os.Stat(outputPath); err == nil {
				conflicts = append(conflicts, cleanName)
				if len(conflicts) >= 5 {
					break
				}
			}
		}
	}

	if len(conflicts) > 0 {
		return 0, fmt.Errorf("同名文件已存在：%s", strings.Join(conflicts, "、"))
	}

	extractedFiles := 0
	for _, file := range reader.File {
		cleanName, err := sanitizeZipEntryName(file.Name)
		if err != nil {
			return extractedFiles, err
		}

		outputPath := filepath.Join(destinationDir, cleanName)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(outputPath, 0755); err != nil {
				return extractedFiles, err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return extractedFiles, err
		}

		readerHandle, err := file.Open()
		if err != nil {
			return extractedFiles, err
		}

		openFlags := os.O_CREATE | os.O_WRONLY
		if overwrite {
			openFlags |= os.O_TRUNC
		} else {
			openFlags |= os.O_EXCL
		}

		fileMode := file.Mode().Perm()
		if fileMode == 0 {
			fileMode = 0644
		}

		writerHandle, err := os.OpenFile(outputPath, openFlags, fileMode)
		if err != nil {
			readerHandle.Close()
			return extractedFiles, err
		}

		if _, err := io.Copy(writerHandle, readerHandle); err != nil {
			writerHandle.Close()
			readerHandle.Close()
			return extractedFiles, err
		}

		writerHandle.Close()
		readerHandle.Close()
		extractedFiles++
	}

	return extractedFiles, nil
}

func ListFiles(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		relativePath = "."
	}

	fullPath, err := resolveDataPath(relativePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法目录路径"))
		return
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
			"path":   relativePath,
			"files":  []gin.H{},
			"exists": false,
		}))
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取目录失败: "+err.Error()))
		return
	}

	files := []gin.H{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, gin.H{
			"name":        entry.Name(),
			"isDir":       entry.IsDir(),
			"isDirectory": entry.IsDir(),
			"size":        info.Size(),
			"modifiedAt":  info.ModTime().Format(time.RFC3339),
			"path":        filepath.ToSlash(filepath.Join(strings.TrimPrefix(relativePath, "./"), entry.Name())),
		})
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"path":   relativePath,
		"files":  files,
		"exists": true,
	}))
}

func ReadFile(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少文件路径"))
		return
	}

	fullPath, err := resolveDataPath(relativePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法文件路径"))
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取文件失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"path":    relativePath,
		"content": string(content),
	}))
}

func WriteFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}

	fullPath, err := resolveDataPath(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法文件路径"))
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建目录失败"))
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("写入文件失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse("文件保存成功"))
}

func UploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("获取文件失败"))
		return
	}

	targetPath := c.PostForm("path")
	if targetPath == "" {
		targetPath = "."
	}
	extractAfterUpload := strings.EqualFold(c.PostForm("extract"), "true")

	fileName := filepath.Base(file.Filename)
	if fileName == "" || fileName == "." {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法文件名"))
		return
	}
	if extractAfterUpload && !strings.EqualFold(filepath.Ext(fileName), ".zip") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("仅支持对 zip 文件执行自动解压"))
		return
	}

	fullPath, err := resolveDataPath(targetPath, fileName)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法上传路径"))
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建目录失败"))
		return
	}

	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存文件失败: "+err.Error()))
		return
	}

	responseData := gin.H{
		"path": fileName,
	}

	if extractAfterUpload {
		extractedFiles, err := extractZipArchive(fullPath, filepath.Dir(fullPath), false)
		if err != nil {
			status := http.StatusInternalServerError
			if isConflictError(err) {
				status = http.StatusConflict
			}
			c.JSON(status, models.ErrorResponse("文件已上传，但自动解压失败: "+err.Error()))
			return
		}

		responseData["extractedFiles"] = extractedFiles
		responseData["uploadedArchive"] = fileName
		c.JSON(http.StatusOK, models.SuccessResponse(responseData))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(responseData))
}

func RenameFile(c *gin.Context) {
	var req struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	if strings.TrimSpace(req.OldPath) == "" || strings.TrimSpace(req.NewPath) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少路径参数"))
		return
	}
	oldFull, err := resolveDataPath(req.OldPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法源路径"))
		return
	}
	newFull, err := resolveDataPath(req.NewPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法目标路径"))
		return
	}
	if _, err := os.Stat(oldFull); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, models.ErrorResponse("源文件不存在"))
		return
	}
	if _, err := os.Stat(newFull); err == nil {
		c.JSON(http.StatusConflict, models.ErrorResponse("目标文件已存在"))
		return
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("重命名失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("重命名成功"))
}

func DeleteFile(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少文件路径"))
		return
	}

	fullPath, err := resolveDataPath(relativePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法文件路径"))
		return
	}

	if err := os.Remove(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除文件失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse("文件删除成功"))
}

func ExtractFile(c *gin.Context) {
	var req struct {
		Path      string `json:"path"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少压缩包路径"))
		return
	}
	if !strings.EqualFold(filepath.Ext(req.Path), ".zip") {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("当前仅支持解压 zip 文件"))
		return
	}

	fullPath, err := resolveDataPath(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法压缩包路径"))
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("压缩包不存在"))
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("只能解压 zip 文件"))
		return
	}

	extractedFiles, err := extractZipArchive(fullPath, filepath.Dir(fullPath), req.Overwrite)
	if err != nil {
		status := http.StatusInternalServerError
		if isConflictError(err) {
			status = http.StatusConflict
		}
		c.JSON(status, models.ErrorResponse("解压失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"path":           req.Path,
		"extractedFiles": extractedFiles,
	}))
}

func DownloadFile(c *gin.Context) {
	relativePath := c.Query("path")
	if strings.TrimSpace(relativePath) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("缺少文件路径"))
		return
	}

	fullPath, err := resolveDataPath(relativePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("非法文件路径"))
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("文件不存在"))
		return
	}

	archiveDownload := strings.EqualFold(c.DefaultQuery("archive", "false"), "true")
	if info.IsDir() || archiveDownload {
		if err := streamZipArchive(c, fullPath, info.IsDir()); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, models.ErrorResponse("打包下载失败: "+err.Error()))
		}
		return
	}

	c.FileAttachment(fullPath, info.Name())
}
