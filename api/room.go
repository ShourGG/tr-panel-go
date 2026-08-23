package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"terraria-panel/config"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/storage"
	"terraria-panel/utils"
	"time"
)

var roomStorage storage.RoomStorage

func SetRoomStorage(s storage.RoomStorage) {
	roomStorage = s
}

type WorldInfo struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type updateRoomRequest struct {
	Name       *string `json:"name"`
	ServerType *string `json:"serverType"`
	WorldFile  *string `json:"worldFile"`
	Port       *int    `json:"port"`
	MaxPlayers *int    `json:"maxPlayers"`
	Password   *string `json:"password"`
	ModProfile *string `json:"modProfile"`
	WorldSize  *string `json:"worldSize"`
	Difficulty *string `json:"difficulty"`
	EvilType   *string `json:"evilType"`
	Seed       *string `json:"seed"`
}

func GetWorldsForRoom(c *gin.Context) {
	serverType := c.Query("serverType")
	var worldExt string
	switch serverType {
	case "tmodloader":
		worldExt = ".twld"
	case "vanilla", "tshock":
		worldExt = ".wld"
	default:
		worldExt = ".wld"
	}
	worldMap := make(map[string]WorldInfo)
	roomsDir := filepath.Join(config.DataDir, "rooms")
	if roomDirs, err := os.ReadDir(roomsDir); err == nil {
		for _, roomDir := range roomDirs {
			if roomDir.IsDir() {
				roomPath := filepath.Join(roomsDir, roomDir.Name())
				for _, worldDir := range roomWorldDirectories(roomPath, serverType) {
					if files, err := os.ReadDir(worldDir); err == nil {
						for _, file := range files {
							if !file.IsDir() && strings.EqualFold(filepath.Ext(file.Name()), worldExt) {
								if _, exists := worldMap[file.Name()]; !exists {
									worldMap[file.Name()] = WorldInfo{
										Name:   file.Name(),
										Source: "房间: " + roomDir.Name(),
										Path:   filepath.Join(worldDir, file.Name()),
									}
								}
							}
						}
					}
				}
			}
		}
	}
	os.MkdirAll(config.SharedWorldsDir, 0755)
	if files, err := os.ReadDir(config.SharedWorldsDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.EqualFold(filepath.Ext(file.Name()), worldExt) {
				if _, exists := worldMap[file.Name()]; !exists {
					worldMap[file.Name()] = WorldInfo{
						Name:   file.Name(),
						Source: "共享世界",
						Path:   filepath.Join(config.SharedWorldsDir, file.Name()),
					}
				}
			}
		}
	}
	var worlds []WorldInfo
	for _, world := range worldMap {
		worlds = append(worlds, world)
	}
	sort.Slice(worlds, func(i, j int) bool {
		return strings.ToLower(worlds[i].Name) < strings.ToLower(worlds[j].Name)
	})
	log.Printf("[INFO] 找到 %d 个可用世界文件（类型：%s）", len(worlds), serverType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    worlds,
	})
}

func ImportWorld(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("请选择要导入的世界文件"))
		return
	}

	filename := filepath.Base(strings.TrimSpace(file.Filename))
	if filename == "" || filename == "." {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("文件名不合法"))
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".wld" && ext != ".twld" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("仅支持导入 .wld 或 .twld 世界文件"))
		return
	}

	if err := os.MkdirAll(config.SharedWorldsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建共享世界目录失败"))
		return
	}

	targetPath := filepath.Join(config.SharedWorldsDir, filename)
	if _, err := os.Stat(targetPath); err == nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("同名世界文件已存在"))
		return
	}

	if err := c.SaveUploadedFile(file, targetPath); err != nil {
		log.Printf("[ERROR] 导入世界失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存世界文件失败: "+err.Error()))
		return
	}

	log.Printf("[INFO] 世界导入成功: %s -> %s", filename, targetPath)
	c.JSON(http.StatusOK, models.Response{
		Success: true,
		Message: "世界导入成功",
		Data: gin.H{
			"name": filename,
			"path": targetPath,
		},
	})
}

func GetRooms(c *gin.Context) {
	rooms, err := roomStorage.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取房间列表失败: "+err.Error()))
		return
	}
	for i := range rooms {
		syncRoomRuntimeState(&rooms[i])
	}
	c.JSON(http.StatusOK, models.SuccessResponse(rooms))
}
func CreateRoom(c *gin.Context) {
	var room models.Room
	if err := c.ShouldBindJSON(&room); err != nil {
		fmt.Printf("[DEBUG] 创建房间参数绑定失败: %v\n", err)
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误: "+err.Error()))
		return
	}
	fmt.Printf("[DEBUG] 创建房间请求: Name=%s, Type=%s, World=%s, Port=%d\n",
		room.Name, room.ServerType, room.WorldFile, room.Port)
	room.WorldFile = normalizeRoomWorldFile(room.ServerType, room.WorldFile)
	room.Status = "stopped"
	room.PID = 0
	if err := roomStorage.Create(&room); err != nil {
		fmt.Printf("[DEBUG] 数据库创建失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建失败: "+err.Error()))
		return
	}
	fmt.Printf("[DEBUG] 房间创建成功: ID=%d\n", room.ID)
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		log.Printf("[ERROR] 创建房间目录失败: %v", err)
	} else {
		log.Printf("[INFO] 房间目录已创建: %s", roomDir)
	}
	c.JSON(http.StatusOK, models.Response{
		Success: true,
		Message: "房间创建成功",
		Data:    room,
	})
}
func UpdateRoom(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}
	var req updateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	existingRoom, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取房间失败: "+err.Error()))
		return
	}
	if existingRoom == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}
	updatedRoom := *existingRoom
	updatedRoom.ID = id
	if req.Name != nil {
		updatedRoom.Name = *req.Name
	}
	if req.ServerType != nil {
		updatedRoom.ServerType = *req.ServerType
	}
	if req.WorldFile != nil {
		updatedRoom.WorldFile = *req.WorldFile
	}
	if req.Port != nil {
		updatedRoom.Port = *req.Port
	}
	if req.MaxPlayers != nil {
		updatedRoom.MaxPlayers = *req.MaxPlayers
	}
	if req.Password != nil {
		updatedRoom.Password = *req.Password
	}
	if req.ModProfile != nil {
		updatedRoom.ModProfile = *req.ModProfile
	}
	if req.WorldSize != nil {
		updatedRoom.WorldSize = *req.WorldSize
	}
	if req.Difficulty != nil {
		updatedRoom.Difficulty = *req.Difficulty
	}
	if req.EvilType != nil {
		updatedRoom.EvilType = *req.EvilType
	}
	if req.Seed != nil {
		updatedRoom.Seed = *req.Seed
	}
	updatedRoom.WorldFile = normalizeRoomWorldFile(updatedRoom.ServerType, updatedRoom.WorldFile)
	if err := roomStorage.Update(&updatedRoom); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("更新失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("房间更新成功"))
}

func removeGeneratedRoomConfigs(roomID int) {
	configDir := filepath.Join(config.DataDir, "configs")
	paths := []string{
		filepath.Join(configDir, fmt.Sprintf("room-%d-config.txt", roomID)),
		filepath.Join(configDir, fmt.Sprintf("room-%d-tshock.properties", roomID)),
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("[WARN] 删除房间生成配置失败: %s: %v", path, err)
			continue
		}
		log.Printf("[INFO] 房间生成配置已删除: %s", path)
	}
}

func DeleteRoom(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}
	room, err := roomStorage.GetByID(id)
	if err != nil {
		log.Printf("[ERROR] 获取房间信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("获取房间信息失败: "+err.Error()))
		return
	}
	if room == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}
	log.Printf("[INFO] 开始删除房间: ID=%d, Name=%s, Type=%s, World=%s",
		room.ID, room.Name, room.ServerType, room.WorldFile)
	if p, exists := utils.GetProcess(id); exists && p.IsRunning() {
		log.Printf("[INFO] 停止房间进程: PID=%d", p.GetPID())
		utils.StopProcess(id)
	}
	clearRoomPreparing(id)
	finalizeRoomPlayerActivity(id)
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if err := os.RemoveAll(roomDir); err != nil {
		log.Printf("[ERROR] 删除房间目录失败: %v", err)
	} else {
		log.Printf("[INFO] 房间目录已删除: %s", roomDir)
	}
	logFile := config.RoomLogFile(room.ID)
	if err := os.Remove(logFile); err == nil {
		log.Printf("[INFO] 日志文件已删除: %s", logFile)
	}
	removeGeneratedRoomConfigs(room.ID)
	if err := roomStorage.Delete(id); err != nil {
		log.Printf("[ERROR] 删除房间记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("删除失败: "+err.Error()))
		return
	}
	log.Printf("[INFO] 房间删除成功: ID=%d", id)
	c.JSON(http.StatusOK, models.MessageResponse("房间删除成功"))
}
func copyWorldFileFromSource(worldFileName string, targetPath string, serverType string) bool {
	var worldExt string
	switch serverType {
	case "tmodloader":
		worldExt = ".twld"
	case "vanilla", "tshock":
		worldExt = ".wld"
	default:
		worldExt = ".wld"
	}
	roomsDir := filepath.Join(config.DataDir, "rooms")
	if roomDirs, err := os.ReadDir(roomsDir); err == nil {
		for _, roomDir := range roomDirs {
			if roomDir.IsDir() {
				roomPath := filepath.Join(roomsDir, roomDir.Name())
				for _, worldDir := range roomWorldDirectories(roomPath, serverType) {
					sourcePath := filepath.Join(worldDir, worldFileName)
					if _, err := os.Stat(sourcePath); err == nil {
						log.Printf("[INFO] 找到源世界文件: %s", sourcePath)
						if err := copyFile(sourcePath, targetPath); err == nil {
							log.Printf("[INFO] 世界文件复制成功: %s -> %s", sourcePath, targetPath)
							copyBackupFiles(worldDir, filepath.Dir(targetPath), worldFileName, worldExt)
							return true
						} else {
							log.Printf("[ERROR] 复制世界文件失败: %v", err)
						}
					}
				}
			}
		}
	}
	sourcePath := filepath.Join(config.SharedWorldsDir, worldFileName)
	if _, err := os.Stat(sourcePath); err == nil {
		log.Printf("[INFO] 找到共享世界文件: %s", sourcePath)
		if err := copyFile(sourcePath, targetPath); err == nil {
			log.Printf("[INFO] 世界文件复制成功: %s -> %s", sourcePath, targetPath)
			copyBackupFiles(config.SharedWorldsDir, filepath.Dir(targetPath), worldFileName, worldExt)
			return true
		} else {
			log.Printf("[ERROR] 复制世界文件失败: %v", err)
		}
	}
	return false
}
func copyBackupFiles(srcDir string, dstDir string, worldFileName string, worldExt string) {
	worldBaseName := strings.TrimSuffix(worldFileName, worldExt)
	pattern := filepath.Join(srcDir, worldBaseName+"*")
	backupFiles, _ := filepath.Glob(pattern)
	for _, srcBackupPath := range backupFiles {
		if filepath.Base(srcBackupPath) == worldFileName {
			continue
		}
		fileName := filepath.Base(srcBackupPath)
		dstBackupPath := filepath.Join(dstDir, fileName)
		if err := copyFile(srcBackupPath, dstBackupPath); err == nil {
			log.Printf("[INFO] 备份文件已复制: %s", fileName)
		}
	}
}
func migrateOldWorldFile(room *models.Room, roomDir string, newWorldPath string, worldExt string) {
	var oldWorldsDir string
	switch room.ServerType {
	case "tmodloader":
		oldWorldsDir = filepath.Join(config.DataDir, ".local", "share", "Terraria", "tModLoader", "Worlds")
	case "vanilla", "tshock":
		oldWorldsDir = config.WorldsDir
	default:
		oldWorldsDir = config.WorldsDir
	}
	oldWorldPath := filepath.Join(oldWorldsDir, room.WorldFile)
	if _, err := os.Stat(oldWorldPath); err != nil {
		return
	}
	log.Printf("[INFO] 发现旧世界文件，开始迁移: %s -> %s", oldWorldPath, newWorldPath)
	data, err := os.ReadFile(oldWorldPath)
	if err != nil {
		log.Printf("[ERROR] 读取旧世界文件失败: %v", err)
		return
	}
	if err := os.WriteFile(newWorldPath, data, 0644); err != nil {
		log.Printf("[ERROR] 写入新世界文件失败: %v", err)
		return
	}
	log.Printf("[INFO] 世界文件迁移成功")
	worldBaseName := strings.TrimSuffix(room.WorldFile, worldExt)
	pattern := filepath.Join(oldWorldsDir, worldBaseName+"*")
	backupFiles, _ := filepath.Glob(pattern)
	for _, oldBackupPath := range backupFiles {
		if oldBackupPath == oldWorldPath {
			continue
		}
		fileName := filepath.Base(oldBackupPath)
		newBackupPath := filepath.Join(roomDir, fileName)
		if backupData, err := os.ReadFile(oldBackupPath); err == nil {
			if err := os.WriteFile(newBackupPath, backupData, 0644); err == nil {
				log.Printf("[INFO] 备份文件已迁移: %s", fileName)
			}
		}
	}
}

func roomWorldSizeValue(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "small", "1":
		return "1"
	case "large", "3":
		return "3"
	default:
		return "2"
	}
}

func roomDifficultyValue(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "expert", "1":
		return "1"
	case "master", "2":
		return "2"
	case "journey", "3":
		return "3"
	default:
		return "0"
	}
}

func roomEvilValue(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "corrupt", "corruption", "1":
		return "corrupt"
	case "crimson", "2":
		return "crimson"
	default:
		return "random"
	}
}

func appendRoomWorldCreationArgs(args []string, room *models.Room) []string {
	args = append(args, "-difficulty", roomDifficultyValue(room.Difficulty))
	args = append(args, "-worldevil", roomEvilValue(room.EvilType))
	if seed := strings.TrimSpace(room.Seed); seed != "" {
		args = append(args, "-seed", seed)
	}
	return args
}

func StartRoom(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("[ERROR] 启动房间失败 - 无效的房间ID: %s", idStr)
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}
	if p, exists := utils.GetProcess(id); exists && p.IsRunning() {
		log.Printf("[WARN] 启动房间 %d 失败 - 房间已在运行中 (PID: %d)", id, p.GetPID())
		c.JSON(http.StatusBadRequest, models.ErrorResponse("房间已在运行中"))
		return
	}
	room, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取房间失败: "+err.Error()))
		return
	}
	if room == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}
	roomDir := filepath.Join(config.DataDir, "rooms", fmt.Sprintf("room-%d", room.ID))
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		log.Printf("[ERROR] 创建房间目录失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建房间目录失败"))
		return
	}
	roomTshockDir := filepath.Join(roomDir, "tshock")
	worldExt := roomWorldExtension(room.ServerType)
	originalWorldFile := room.WorldFile
	room.WorldFile = normalizeRoomWorldFileForDir(room.ServerType, room.WorldFile, roomDir)
	if room.WorldFile != originalWorldFile {
		if err := roomStorage.Update(room); err != nil {
			log.Printf("[ERROR] 保存房间世界文件失败: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存房间世界文件失败"))
			return
		}
	}
	worldPath := filepath.Join(roomDir, room.WorldFile)
	worldExists := false
	if _, err := os.Stat(worldPath); err == nil {
		worldExists = true
		log.Printf("[INFO] 使用已有世界文件: %s", worldPath)
	} else {
		log.Printf("[INFO] 世界文件不存在，尝试查找源文件...")
		if copyWorldFileFromSource(room.WorldFile, worldPath, room.ServerType) {
			worldExists = true
			log.Printf("[INFO] 已从源位置复制世界文件")
		} else {
			migrateOldWorldFile(room, roomDir, worldPath, worldExt)
			if _, err := os.Stat(worldPath); err == nil {
				worldExists = true
				log.Printf("[INFO] 已从旧位置迁移世界文件")
			} else {
				log.Printf("[INFO] 未找到源文件，首次启动将自动创建: %s", worldPath)
			}
		}
	}
	var command string
	var args []string
	var vanillaServerWorkDir string
	switch room.ServerType {
	case "tmodloader":
		tmodDir := filepath.Join(config.ServersDir, "tModLoader")
		dllPath := filepath.Join(tmodDir, "tModLoader.dll")
		if _, err := os.Stat(dllPath); os.IsNotExist(err) {
			log.Printf("[ERROR] tModLoader服务器文件不存在: %s", dllPath)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(
				"tModLoader服务器未安装。请先在【游戏安装】页面安装tModLoader服务器"))
			return
		}
		if err := os.Chmod(dllPath, 0755); err != nil {
			log.Printf("[WARN] 无法设置文件权限: %v", err)
		}
		tmlSaveDir := roomDir
		worldsSubDir := filepath.Join(tmlSaveDir, "Worlds")
		os.MkdirAll(worldsSubDir, 0755)
		userWorldPath := filepath.Join(worldsSubDir, room.WorldFile)
		defaultWorldPath := filepath.Join(worldsSubDir, "World.twld")
		log.Printf("[INFO] tModLoader 保存目录: %s", tmlSaveDir)
		log.Printf("[INFO] 用户指定的世界路径: %s", userWorldPath)
		log.Printf("[INFO] tModLoader 默认世界路径: %s", defaultWorldPath)
		worldExists = false
		actualWorldPath := userWorldPath
		if stat, err := os.Stat(userWorldPath); err == nil {
			actualWorldPath = userWorldPath
			log.Printf("[INFO] 找到用户指定的世界文件: %s", userWorldPath)
			isCorrupted := false
			if stat.Size() < 10*1024 {
				log.Printf("[WARN] 世界文件大小异常（%d bytes），可能损坏", stat.Size())
				isCorrupted = true
			}
			if !isCorrupted {
				vanillaWorldPath := strings.Replace(actualWorldPath, ".twld", ".wld", 1)
				if vanillaStat, err := os.Stat(vanillaWorldPath); err == nil {
					if vanillaStat.Size() > stat.Size()*10 {
						log.Printf("[WARN] 检测到 vanilla 世界文件 (%d bytes) 远大于 tModLoader 世界文件 (%d bytes)",
							vanillaStat.Size(), stat.Size())
						log.Printf("[WARN] 这说明 tModLoader 世界文件可能损坏或转换不完整")
						isCorrupted = true
					}
				}
			}
			if isCorrupted {
				log.Printf("[WARN] 世界文件损坏，尝试从备份恢复")
				backupRestored := false
				backupFiles := []string{
					actualWorldPath + ".bak",
					actualWorldPath + ".backup",
					actualWorldPath + ".twld.bak",
				}
				for _, backupPath := range backupFiles {
					if backupStat, err := os.Stat(backupPath); err == nil {
						if backupStat.Size() >= 10*1024 {
							log.Printf("[INFO] 发现有效备份文件: %s (大小: %d bytes)", backupPath, backupStat.Size())
							os.Rename(actualWorldPath, actualWorldPath+".corrupted")
							if data, err := os.ReadFile(backupPath); err == nil {
								if err := os.WriteFile(actualWorldPath, data, 0644); err == nil {
									log.Printf("[INFO] 成功从备份恢复世界文件: %s", backupPath)
									backupRestored = true
									worldExists = true
									break
								}
							}
						}
					}
				}
				if !backupRestored {
					vanillaWorldPath := strings.Replace(actualWorldPath, ".twld", ".wld", 1)
					if vanillaStat, err := os.Stat(vanillaWorldPath); err == nil {
						if vanillaStat.Size() >= 10*1024 {
							log.Printf("[INFO] 发现 vanilla 世界文件: %s (大小: %d bytes)", vanillaWorldPath, vanillaStat.Size())
							log.Printf("[INFO] tModLoader 可以加载 vanilla 世界文件并自动转换")
							log.Printf("[INFO] 删除损坏的 tModLoader 世界文件: %s", actualWorldPath)
							os.Remove(actualWorldPath)
							os.Remove(actualWorldPath + ".bak")
							worldExists = true
							log.Printf("[INFO] 将使用 vanilla 世界文件，tModLoader 会自动转换")
							log.Printf("[INFO] 启动参数将使用 .twld 路径: %s", actualWorldPath)
						} else {
							log.Printf("[WARN] vanilla 世界文件也损坏: %s (大小: %d bytes)", vanillaWorldPath, vanillaStat.Size())
							log.Printf("[WARN] 世界文件损坏且无有效备份，将自动删除并重新创建")
							log.Printf("[INFO] 删除损坏的世界文件: %s", actualWorldPath)
							os.Remove(actualWorldPath)
							os.Remove(actualWorldPath + ".bak")
							os.Remove(actualWorldPath + ".backup")
							os.Remove(vanillaWorldPath)
							os.Remove(vanillaWorldPath + ".bak")
							worldExists = false
							log.Printf("[INFO] 将创建新世界文件")
						}
					} else {
						log.Printf("[WARN] 世界文件损坏且无有效备份，将自动删除并重新创建")
						log.Printf("[INFO] 删除损坏的世界文件: %s", actualWorldPath)
						os.Remove(actualWorldPath)
						os.Remove(actualWorldPath + ".bak")
						os.Remove(actualWorldPath + ".backup")
						worldExists = false
						log.Printf("[INFO] 将创建新世界文件")
					}
				}
			} else {
				worldExists = true
				log.Printf("[INFO] 使用已有世界文件: %s (大小: %d bytes)", actualWorldPath, stat.Size())
			}
		} else if _, err := os.Stat(defaultWorldPath); err == nil {
			log.Printf("[INFO] 发现 tModLoader 默认世界文件: %s", defaultWorldPath)
			log.Printf("[INFO] 将其重命名为用户指定的文件名: %s", userWorldPath)
			if err := os.Rename(defaultWorldPath, userWorldPath); err == nil {
				actualWorldPath = userWorldPath
				worldExists = true
				log.Printf("[INFO] .twld 文件重命名成功")
				defaultWldPath := strings.Replace(defaultWorldPath, ".twld", ".wld", 1)
				userWldPath := strings.Replace(userWorldPath, ".twld", ".wld", 1)
				if _, err := os.Stat(defaultWldPath); err == nil {
					if err := os.Rename(defaultWldPath, userWldPath); err == nil {
						log.Printf("[INFO] .wld 文件重命名成功: %s -> %s", defaultWldPath, userWldPath)
					} else {
						log.Printf("[ERROR] .wld 文件重命名失败: %v", err)
					}
				} else {
					log.Printf("[WARN] 未找到对应的 .wld 文件: %s", defaultWldPath)
				}
				if _, err := os.Stat(defaultWorldPath + ".bak"); err == nil {
					os.Rename(defaultWorldPath+".bak", userWorldPath+".bak")
				}
				if _, err := os.Stat(defaultWorldPath + ".backup"); err == nil {
					os.Rename(defaultWorldPath+".backup", userWorldPath+".backup")
				}
				if _, err := os.Stat(defaultWldPath + ".bak"); err == nil {
					os.Rename(defaultWldPath+".bak", userWldPath+".bak")
				}
			} else {
				log.Printf("[ERROR] 重命名失败: %v", err)
				actualWorldPath = defaultWorldPath
				worldExists = true
			}
		} else {
			if _, err := os.Stat(worldPath); err == nil {
				log.Printf("[INFO] 发现旧位置的世界文件，复制到新位置: %s -> %s", worldPath, userWorldPath)
				if data, err := os.ReadFile(worldPath); err == nil {
					os.WriteFile(userWorldPath, data, 0644)
					actualWorldPath = userWorldPath
					worldExists = true
				}
			}
		}
		command = "dotnet"
		args = []string{
			dllPath,
			"-server",
		}
		roomModsDir := filepath.Join(roomDir, "Mods")
		args = append(args, "-modpath", roomModsDir)
		log.Printf("[INFO] tModLoader 模组目录: %s", roomModsDir)
		if room.ModProfile != "" {
			log.Printf("[INFO] 房间 #%d 应用模组配置: %s", room.ID, room.ModProfile)
			if err := applyModConfigToRoom(room.ID, room.ModProfile, roomDir); err != nil {
				log.Printf("[ERROR] 应用模组配置失败: %v", err)
				c.JSON(http.StatusInternalServerError, models.ErrorResponse("应用模组配置失败: "+err.Error()))
				return
			}
		} else {
			log.Printf("[INFO] 房间 #%d 使用纯净版（无模组）", room.ID)
			os.MkdirAll(roomModsDir, 0755)
			enabledJsonPath := filepath.Join(roomModsDir, "enabled.json")
			os.WriteFile(enabledJsonPath, []byte("[]"), 0644)
		}
		args = append(args, "-tmlsavedirectory", tmlSaveDir)
		args = append(args, "-port", fmt.Sprintf("%d", room.Port))
		args = append(args, "-maxplayers", fmt.Sprintf("%d", room.MaxPlayers))
		args = append(args, "-nosteam")
		worldName := strings.TrimSuffix(room.WorldFile, ".twld")
		worldPathForParam := strings.Replace(actualWorldPath, ".twld", ".wld", 1)
		args = append(args, "-world", worldPathForParam)
		if !worldExists {
			autocreateSize := roomWorldSizeValue(room.WorldSize)
			args = append(args, "-autocreate", autocreateSize)
			args = append(args, "-worldname", worldName)
			args = appendRoomWorldCreationArgs(args, room)
			log.Printf("[INFO] 世界不存在，将自动创建 (autocreate=%s, worldname=%s)", autocreateSize, worldName)
		} else {
			args = append(args, "-autocreate", "0")
			log.Printf("[INFO] 世界已存在，直接加载 (autocreate=0): %s", worldPathForParam)
		}
		if room.Password != "" {
			args = append(args, "-password", room.Password)
		}
	case "vanilla":
		vanillaDir := filepath.Join(config.ServersDir, "vanilla")
		serverBin, err := findVanillaServerBinary(vanillaDir)
		if err != nil {
			log.Printf("[ERROR] Vanilla服务器文件不存在: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(
				"Vanilla服务器未安装。请先在【游戏安装】页面安装Vanilla服务器"))
			return
		}
		if err := os.Chmod(serverBin, 0755); err != nil {
			log.Printf("[WARN] 无法设置执行权限: %v", err)
		}
		vanillaServerWorkDir = filepath.Dir(serverBin)
		configDir := filepath.Join(config.DataDir, "configs")
		os.MkdirAll(configDir, 0755)
		configPath := filepath.Join(configDir, fmt.Sprintf("room-%d-config.txt", room.ID))
		worldName := strings.TrimSuffix(room.WorldFile, ".wld")
		autocreateValue := 0
		if !worldExists {
			autocreateValue = 2
		}
		log.Printf("[INFO] 世界存在: %v, autocreate: %d", worldExists, autocreateValue)
		configContent := fmt.Sprintf(`maxplayers=%d
world=%s
worldpath=%s/
port=%d
password=%s
worldname=%s
autocreate=%d
difficulty=%s
worldevil=%s
worldrollbackstokeep=10
language=zh-Hans
seed=%s
`, room.MaxPlayers, worldPath, roomDir, room.Port, room.Password, worldName, autocreateValue,
			roomDifficultyValue(room.Difficulty), roomEvilValue(room.EvilType), strings.TrimSpace(room.Seed))
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			log.Printf("[ERROR] 创建配置文件失败: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建配置文件失败"))
			return
		}
		log.Printf("[INFO] 配置文件已创建: %s", configPath)
		command = serverBin
		args = []string{
			"-config", configPath,
		}
	case "tshock":
		tshockDir := filepath.Join(config.ServersDir, "tshock")
		var exePath string
		var useDotNet bool = false
		linuxExe := filepath.Join(tshockDir, "TShock.Server")
		if _, err := os.Stat(linuxExe); err == nil {
			exePath = linuxExe
			useDotNet = false
			log.Printf("[INFO] 找到 TShock Linux 原生可执行文件: %s", exePath)
		} else if runtime.GOOS == "windows" {
			exePath = filepath.Join(tshockDir, "TShock.Server.exe")
			useDotNet = false
		} else {
			exePath = filepath.Join(tshockDir, "TShock.Server.dll")
			useDotNet = true
		}
		if _, err := os.Stat(exePath); os.IsNotExist(err) {
			log.Printf("[ERROR] TShock服务器文件不存在: %s", exePath)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(
				"TShock服务器未安装。请先在【游戏安装】页面安装TShock服务器"))
			return
		}
		if useDotNet {
			detection := detectInstalledTShockVersion(tshockDir)
			requiredRuntime := getRequiredDotNetRuntime(detection.Version)
			hasRequiredRuntime, allRuntimes, err := utils.CheckDotNetRuntimeVersion(requiredRuntime)
			if err != nil {
				errMsg := fmt.Sprintf("无法检测 .NET Runtime: %v", err)
				log.Printf("[ERROR] %s", errMsg)
				c.JSON(http.StatusInternalServerError, models.ErrorResponse(errMsg))
				return
			}
			if !hasRequiredRuntime {
				installedRuntimes, _ := utils.GetInstalledDotNetRuntimes()
				installCommands, _ := utils.GetDotNetInstallCommand(requiredRuntime)
				versionLabel := detection.Version
				if versionLabel == "unknown" {
					versionLabel = "未知版本"
				}
				errMsg := fmt.Sprintf(`TShock 启动失败：缺少 .NET %s Runtime
当前系统已安装的 .NET Runtime：
%s
检测到 TShock %s，当前版本需要 .NET %s Runtime，但系统未安装此版本
解决方案：
%s
安装完成后，请重新启动房间。
参考文档：https://dotnet.microsoft.com/download/dotnet/%s`,
					requiredRuntime,
					formatRuntimeList(installedRuntimes),
					versionLabel, requiredRuntime,
					strings.Join(installCommands, "\n"), requiredRuntime)
				log.Printf("[ERROR] %s", errMsg)
				c.JSON(http.StatusInternalServerError, models.ErrorResponse(errMsg))
				return
			}
			log.Printf("[INFO] .NET %s Runtime 检查通过", requiredRuntime)
			log.Printf("[DEBUG] 已安装的 Runtime:\n%s", allRuntimes)
		}
		if err := os.Chmod(exePath, 0755); err != nil {
			log.Printf("[WARN] 无法设置执行权限: %v", err)
		}
		os.MkdirAll(roomTshockDir, 0755)
		os.MkdirAll(filepath.Join(roomTshockDir, "logs"), 0755)
		os.MkdirAll(filepath.Join(roomTshockDir, "backups"), 0755)
		roomExePath := filepath.Join(roomTshockDir, filepath.Base(exePath))
		needsInitialization := false
		if _, err := os.Stat(roomExePath); os.IsNotExist(err) {
			needsInitialization = true
			log.Printf("[INFO] 房间 TShock 目录未初始化，开始复制所有文件...")
		}
		if needsInitialization {
			err := filepath.Walk(tshockDir, func(srcPath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				relPath, err := filepath.Rel(tshockDir, srcPath)
				if err != nil {
					return err
				}
				dstPath := filepath.Join(roomTshockDir, relPath)
				if info.IsDir() {
					return os.MkdirAll(dstPath, info.Mode())
				}
				srcFile, err := os.Open(srcPath)
				if err != nil {
					return err
				}
				defer srcFile.Close()
				dstFile, err := os.Create(dstPath)
				if err != nil {
					return err
				}
				defer dstFile.Close()
				if _, err := io.Copy(dstFile, srcFile); err != nil {
					return err
				}
				return os.Chmod(dstPath, info.Mode())
			})
			if err != nil {
				log.Printf("[ERROR] 复制 TShock 目录失败: %v", err)
				c.JSON(http.StatusInternalServerError, models.ErrorResponse("初始化房间 TShock 目录失败: "+err.Error()))
				return
			}
			log.Printf("[INFO] TShock 目录已完整复制到房间目录: %s", roomTshockDir)
			log.Printf("[INFO] 房间现在拥有独立的 TShock 实例（完全隔离）")
		} else {
			log.Printf("[INFO] 房间 TShock 目录已存在，跳过初始化")
		}
		exePath = roomExePath
		log.Printf("[INFO] 使用房间专属 TShock 可执行文件: %s", exePath)
		defaultConfigPath := filepath.Join(tshockDir, "config.json")
		roomConfigPath := filepath.Join(roomTshockDir, "config.json")
		if _, err := os.Stat(roomConfigPath); os.IsNotExist(err) {
			if data, err := os.ReadFile(defaultConfigPath); err == nil {
				os.WriteFile(roomConfigPath, data, 0644)
				log.Printf("[INFO] 已复制默认 TShock 配置")
			} else {
				log.Printf("[WARN] 无法复制默认配置: %v", err)
			}
		}
		configDir := filepath.Join(config.DataDir, "configs")
		os.MkdirAll(configDir, 0755)
		configPath := filepath.Join(configDir, fmt.Sprintf("room-%d-tshock.properties", room.ID))
		worldPath := filepath.Join(roomDir, room.WorldFile)
		worldName := strings.TrimSuffix(room.WorldFile, ".wld")
		autocreateValue := 0
		if _, err := os.Stat(worldPath); os.IsNotExist(err) {
			autocreateValue = 2
		}
		configContent := fmt.Sprintf(`# TShock Server Configuration - Room %d
config=%s/
world=%s
worldpath=%s/
port=%d
maxplayers=%d
password=%s
worldname=%s
autocreate=%d
difficulty=%s
worldevil=%s
language=zh-Hans
upnp=0
priority=1
motd=%s/motd.txt
seed=%s
`, room.ID, roomTshockDir, worldPath, roomDir, room.Port, room.MaxPlayers,
			room.Password, worldName, autocreateValue, roomDifficultyValue(room.Difficulty),
			roomEvilValue(room.EvilType), roomTshockDir, strings.TrimSpace(room.Seed))
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			log.Printf("[ERROR] 创建配置文件失败: %v", err)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建配置文件失败"))
			return
		}
		log.Printf("[INFO] TShock 配置文件已创建: %s", configPath)
		roomPluginsDir := filepath.Join(roomTshockDir, "ServerPlugins")
		sharedPluginsDir := filepath.Join(tshockDir, "ServerPlugins")
		os.MkdirAll(roomPluginsDir, 0755)
		roomPluginFiles, _ := os.ReadDir(roomPluginsDir)
		if len(roomPluginFiles) == 0 {
			log.Printf("[INFO] 房间插件目录为空，从共享目录复制默认插件...")
			if files, err := os.ReadDir(sharedPluginsDir); err == nil {
				copiedCount := 0
				for _, file := range files {
					if !file.IsDir() && strings.HasSuffix(file.Name(), ".dll") {
						src := filepath.Join(sharedPluginsDir, file.Name())
						dst := filepath.Join(roomPluginsDir, file.Name())
						if data, err := os.ReadFile(src); err == nil {
							if err := os.WriteFile(dst, data, 0644); err == nil {
								copiedCount++
								log.Printf("[INFO] 已复制插件: %s", file.Name())
							}
						}
					}
				}
				log.Printf("[INFO] 共复制 %d 个插件到房间目录", copiedCount)
			} else {
				log.Printf("[WARN] 无法读取共享插件目录: %v", err)
			}
		} else {
			log.Printf("[INFO] 房间已有 %d 个插件文件", len(roomPluginFiles))
		}
		log.Printf("[INFO] 房间插件目录准备完毕: %s", roomPluginsDir)
		if useDotNet {
			command = "dotnet"
			args = []string{
				exePath,
				"-lang", "7",
				"-config", configPath,
				"-configpath", roomTshockDir,
				"-worldpath", roomDir,
				"-port", fmt.Sprintf("%d", room.Port),
			}
			if !worldExists {
				args = appendRoomWorldCreationArgs(args, room)
			}
			log.Printf("[INFO] TShock 启动方式: .NET Runtime")
		} else {
			command = exePath
			args = []string{
				"-lang", "7",
				"-config", configPath,
				"-configpath", roomTshockDir,
				"-worldpath", roomDir,
				"-port", fmt.Sprintf("%d", room.Port),
			}
			if !worldExists {
				args = appendRoomWorldCreationArgs(args, room)
			}
			log.Printf("[INFO] TShock 启动方式: 原生可执行文件")
		}
		log.Printf("[INFO] TShock 配置目录: %s (-configpath)", roomTshockDir)
		log.Printf("[INFO] TShock 插件目录: %s (房间独立，通过 -configpath 加载)", roomPluginsDir)
		log.Printf("[INFO] TShock 可执行文件: %s", exePath)
		log.Printf("[INFO] TShock 启动命令: %s %v", command, args)
	default:
		c.JSON(http.StatusBadRequest, models.ErrorResponse("不支持的服务器类型"))
		return
	}
	logFile := config.RoomLogFile(id)
	log.Printf("[DEBUG] 创建日志文件: %s", logFile)
	logWriter, err := os.Create(logFile)
	if err != nil {
		log.Printf("[ERROR] 创建日志文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建日志文件失败: "+err.Error()))
		return
	}
	var workDir string
	envVars := make(map[string]string)
	switch room.ServerType {
	case "tmodloader":
		workDir = filepath.Join(config.ServersDir, "tModLoader")
		log.Printf("[INFO] tModLoader 工作目录: %s", workDir)
	case "vanilla":
		workDir = vanillaServerWorkDir
	case "tshock":
		workDir = roomTshockDir
		log.Printf("[INFO] TShock 工作目录: %s (房间独立 tshock 目录)", workDir)
	}
	log.Printf("[DEBUG] 启动命令: %s %v", command, args)
	log.Printf("[DEBUG] 工作目录: %s", workDir)
	log.Printf("[DEBUG] 服务器类型: %s", room.ServerType)
	process, err := utils.StartProcess(id, command, args, workDir, envVars, logWriter, room.ServerType)
	if err != nil {
		log.Printf("[ERROR] 启动进程失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("启动失败: "+err.Error()))
		return
	}
	time.Sleep(500 * time.Millisecond)
	if !process.IsRunning() {
		log.Printf("[ERROR] 房间 %d 进程启动后立即退出，请检查日志文件: %s", id, logFile)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("服务器启动失败，进程立即退出。请检查游戏文件是否完整，世界文件是否存在"))
		return
	}
	log.Printf("[DEBUG] 房间 %d 启动成功，PID: %d", id, process.GetPID())
	if !worldExists {
		markRoomPreparing(id)
		_ = roomStorage.UpdateStatus(id, "preparing", process.GetPID())
		go captureWorldGenerationProgress(id, logFile, room.ServerType)
	} else {
		clearRoomPreparing(id)
		_ = roomStorage.UpdateStatus(id, "running", process.GetPID())
	}
	if room.ServerType == "tshock" {
		go captureAdminToken(id, logFile)
	}
	LogRoomStart(id, room.Name, room.ServerType, room.Port)
	c.JSON(http.StatusOK, models.MessageResponse("房间启动成功"))
}
func StopRoom(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}
	room, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}
	if err := utils.StopProcess(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("停止失败: "+err.Error()))
		return
	}
	clearRoomPreparing(id)
	finalizeRoomPlayerActivity(id)
	_ = roomStorage.UpdateStatus(id, "stopped", 0)
	LogRoomStop(id, room.Name)
	c.JSON(http.StatusOK, models.MessageResponse("房间停止成功"))
}
func RestartRoom(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("无效的房间ID"))
		return
	}
	room, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("房间不存在"))
		return
	}
	if p, exists := utils.GetProcess(id); exists && p.IsRunning() {
		if err := utils.StopProcess(id); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("停止失败"))
			return
		}
		clearRoomPreparing(id)
		finalizeRoomPlayerActivity(id)
		_ = roomStorage.UpdateStatus(id, "stopped", 0)
	}
	StartRoom(c)
	LogRoomRestart(id, room.Name)
}
func applyModConfigToRoom(roomID int, modProfileID string, roomDir string) error {
	log.Printf("[INFO] 开始应用模组配置: roomID=%d, modProfileID=%s", roomID, modProfileID)
	profileID, err := strconv.Atoi(modProfileID)
	if err != nil {
		return fmt.Errorf("无效的模组配置ID: %s", modProfileID)
	}
	var profile struct {
		ID          int    `db:"id"`
		Name        string `db:"name"`
		Description string `db:"description"`
		Mods        string `db:"mods"`
	}
	err = db.DB.QueryRow(`
		SELECT id, name, description, mods
		FROM mod_profiles
		WHERE id = ?
	`, profileID).Scan(&profile.ID, &profile.Name, &profile.Description, &profile.Mods)
	if err != nil {
		return fmt.Errorf("查询模组配置失败: %v", err)
	}
	log.Printf("[INFO] 找到模组配置: %s", profile.Name)
	var mods []struct {
		Name       string `json:"name"`
		FileName   string `json:"fileName"`
		WorkshopID string `json:"workshopId"`
		Enabled    bool   `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(profile.Mods), &mods); err != nil {
		return fmt.Errorf("解析模组列表失败: %v", err)
	}
	log.Printf("[INFO] 模组配置包含 %d 个模组", len(mods))
	for i, mod := range mods {
		log.Printf("[DEBUG] 模组 #%d: name=%s, fileName=%s, workshopId=%s, enabled=%v",
			i+1, mod.Name, mod.FileName, mod.WorkshopID, mod.Enabled)
	}
	roomModsDir := filepath.Join(roomDir, "Mods")
	if _, err := os.Stat(roomModsDir); err == nil {
		log.Printf("[INFO] 删除旧的 Mods 目录: %s", roomModsDir)
		if err := os.RemoveAll(roomModsDir); err != nil {
			log.Printf("[WARN] 删除旧的 Mods 目录失败: %v", err)
		}
	}
	if err := os.MkdirAll(roomModsDir, 0755); err != nil {
		return fmt.Errorf("创建 Mods 目录失败: %v", err)
	}
	log.Printf("[INFO] 已创建新的 Mods 目录: %s", roomModsDir)
	globalModsDir := filepath.Join(config.DataDir, "tModLoader", "Mods")
	log.Printf("[INFO] 全局模组目录: %s", globalModsDir)
	enabledMods := []string{}
	for _, mod := range mods {
		if !mod.Enabled {
			log.Printf("[INFO] 跳过未启用的模组: %s", mod.Name)
			continue
		}
		modFileName := mod.FileName
		if modFileName == "" {
			modFileName = mod.Name
		}
		if modFileName == "" {
			log.Printf("[WARN] 模组名称和文件名都为空，跳过")
			continue
		}
		if !strings.HasSuffix(modFileName, ".tmod") {
			modFileName += ".tmod"
		}
		srcPath := filepath.Join(globalModsDir, modFileName)
		dstPath := filepath.Join(roomModsDir, modFileName)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			log.Printf("[WARN] 模组文件不存在: %s", srcPath)
			continue
		}
		log.Printf("[INFO] 复制模组文件: %s -> %s", srcPath, dstPath)
		if err := copyFile(srcPath, dstPath); err != nil {
			log.Printf("[ERROR] 复制模组文件失败: %v", err)
			continue
		}
		modInternalName := strings.TrimSuffix(modFileName, ".tmod")
		enabledMods = append(enabledMods, modInternalName)
		log.Printf("[INFO] 启用模组: %s", modInternalName)
	}
	enabledJsonPath := filepath.Join(roomModsDir, "enabled.json")
	enabledJsonContent, err := json.MarshalIndent(enabledMods, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 enabled.json 失败: %v", err)
	}
	if err := os.WriteFile(enabledJsonPath, enabledJsonContent, 0644); err != nil {
		return fmt.Errorf("写入 enabled.json 失败: %v", err)
	}
	log.Printf("[INFO] enabled.json 已生成，包含 %d 个模组", len(enabledMods))
	log.Printf("[INFO] 文件路径: %s", enabledJsonPath)
	return nil
}
func captureWorldGenerationProgress(roomID int, logFilePath string, serverType string) {
	log.Printf("[INFO] 开始监听房间 %d 的世界生成进度...", roomID)
	maxRetries := 10
	var file *os.File
	var err error
	for i := 0; i < maxRetries; i++ {
		file, err = os.Open(logFilePath)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		log.Printf("[ERROR] 无法打开日志文件 %s: %v", logFilePath, err)
		return
	}
	defer file.Close()
	file.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(file)
	maxWaitTime := 10 * time.Minute
	startTime := time.Now()
	progressKeywords := map[string]string{
		"正在生成世界版图": "生成地形",
		"正在添加沙子":   "添加沙子",
		"正在添加泥土":   "添加泥土",
		"正在添加岩石":   "添加岩石",
		"正在添加水":    "添加水",
		"正在放置宝箱":   "放置宝箱",
		"正在生成地牢":   "生成地牢",
		"正在生成丛林":   "生成丛林",
		"正在生成腐化之地": "生成腐化之地",
		"正在生成猩红之地": "生成猩红之地",
		"正在生成神圣之地": "生成神圣之地",
		"正在生成雪原":   "生成雪原",
		"正在生成沙漠":   "生成沙漠",
		"正在生成海洋":   "生成海洋",
		"正在生成地下世界": "生成地下世界",
		"正在生成洞穴":   "生成洞穴",
		"正在放置生命水晶": "放置生命水晶",
		"正在放置祭坛":   "放置祭坛",
		"世界生成完成":   "世界生成完成",
	}
	log.Printf("[INFO] 开始持续监听世界生成进度: %s", logFilePath)
	for {
		if time.Since(startTime) > maxWaitTime {
			log.Printf("[INFO] 房间 %d 世界生成进度监听超时（10分钟），停止监听", roomID)
			break
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Printf("[ERROR] 读取日志文件失败: %v", err)
			break
		}
		for keyword, progressText := range progressKeywords {
			if strings.Contains(line, keyword) {
				log.Printf("[INFO] 房间 %d 世界生成进度: %s", roomID, progressText)
				progressMsg := map[string]interface{}{
					"type":     "world_generation_progress",
					"roomId":   roomID,
					"progress": progressText,
					"message":  fmt.Sprintf("%s...", progressText),
				}
				if jsonData, err := json.Marshal(progressMsg); err == nil {
					BroadcastMessage(jsonData)
				}
				break
			}
		}

		if containsAnyKeyword(line, roomReadyKeywords) {
			log.Printf("[INFO] 房间 %d 检测到启动完成关键字，准备状态结束（类型：%s）", roomID, serverType)
			promotePreparingRoomToRunning(roomID)

			progressMsg := map[string]interface{}{
				"type":     "world_generation_progress",
				"roomId":   roomID,
				"progress": "服务器启动完成",
				"message":  "世界准备完成，服务器可连接",
			}
			if jsonData, err := json.Marshal(progressMsg); err == nil {
				BroadcastMessage(jsonData)
			}

			return
		}
	}

	if isRoomPreparing(roomID) {
		if process, exists := utils.GetProcess(roomID); exists && process.IsRunning() {
			log.Printf("[WARN] 房间 %d 世界生成进度监听超时，按兜底策略切换为运行中", roomID)
			promotePreparingRoomToRunning(roomID)
		} else {
			clearRoomPreparing(roomID)
		}
	}

	log.Printf("[INFO] 房间 %d 世界生成进度监听结束", roomID)
}
func captureAdminToken(roomID int, logFilePath string) {
	log.Printf("[INFO] 开始监听房间 %d 的管理员令牌...", roomID)
	maxRetries := 10
	var file *os.File
	var err error
	for i := 0; i < maxRetries; i++ {
		file, err = os.Open(logFilePath)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		log.Printf("[ERROR] 无法打开日志文件 %s: %v", logFilePath, err)
		return
	}
	defer file.Close()
	file.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(file)
	tokenFound := false
	maxWaitTime := 10 * time.Minute
	startTime := time.Now()
	log.Printf("[INFO] 开始持续监听日志文件: %s", logFilePath)
	for {
		if time.Since(startTime) > maxWaitTime {
			log.Printf("[INFO] 房间 %d 令牌监听超时（10分钟），停止监听", roomID)
			break
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Printf("[ERROR] 读取日志文件失败: %v", err)
			break
		}
		if strings.Contains(line, "[ADMIN_TOKEN]") {
			re := regexp.MustCompile(`/setup\s+(\d+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				token := "/setup " + matches[1]
				log.Printf("[INFO] 捕获到房间 %d 的管理员令牌: %s", roomID, token)
				if err := roomStorage.UpdateAdminToken(roomID, token); err != nil {
					log.Printf("[ERROR] 保存管理员令牌失败 (房间 %d): %v", roomID, err)
				} else {
					log.Printf("[SUCCESS] 管理员令牌已保存到数据库 (房间 %d)", roomID)
					tokenFound = true
					break
				}
			}
		}
	}
	if !tokenFound {
		log.Printf("[INFO] 房间 %d 未检测到管理员令牌", roomID)
	}
}
func DeleteAdminToken(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的房间ID"})
		return
	}
	room, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "房间不存在"})
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "只有 TShock 服务器才有管理员令牌"})
		return
	}
	if room.Status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请先停止服务器再删除令牌文件"})
		return
	}
	roomDir := filepath.Join(config.DataDir, "rooms", room.Name)
	tshockDir := filepath.Join(roomDir, "tshock")
	setupCodePath := filepath.Join(tshockDir, "setup-code.txt")
	authCodePath := filepath.Join(tshockDir, "authcode.txt")
	deleted := false
	var deletedFiles []string
	if _, err := os.Stat(setupCodePath); err == nil {
		if err := os.Remove(setupCodePath); err != nil {
			log.Printf("[ERROR] 删除 setup-code.txt 失败: %v", err)
		} else {
			log.Printf("[SUCCESS] 已删除 setup-code.txt")
			deleted = true
			deletedFiles = append(deletedFiles, "setup-code.txt")
		}
	}
	if _, err := os.Stat(authCodePath); err == nil {
		if err := os.Remove(authCodePath); err != nil {
			log.Printf("[ERROR] 删除 authcode.txt 失败: %v", err)
		} else {
			log.Printf("[SUCCESS] 已删除 authcode.txt")
			deleted = true
			deletedFiles = append(deletedFiles, "authcode.txt")
		}
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "未找到令牌文件"})
		return
	}
	if err := roomStorage.UpdateAdminToken(id, ""); err != nil {
		log.Printf("[ERROR] 清空数据库令牌失败: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("已删除令牌文件: %s", strings.Join(deletedFiles, ", ")),
		"data": gin.H{
			"deletedFiles": deletedFiles,
		},
	})
}
func RegenerateAdminToken(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的房间ID"})
		return
	}
	room, err := roomStorage.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "房间不存在"})
		return
	}
	if room.ServerType != "tshock" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "只有 TShock 服务器才有管理员令牌"})
		return
	}
	if room.Status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "服务器正在运行，请先停止服务器",
			"action":  "stop_required",
		})
		return
	}
	roomDir := filepath.Join(config.DataDir, "rooms", room.Name)
	tshockDir := filepath.Join(roomDir, "tshock")
	setupCodePath := filepath.Join(tshockDir, "setup-code.txt")
	authCodePath := filepath.Join(tshockDir, "authcode.txt")
	os.Remove(setupCodePath)
	os.Remove(authCodePath)
	if err := roomStorage.UpdateAdminToken(id, ""); err != nil {
		log.Printf("[ERROR] 清空数据库令牌失败: %v", err)
	}
	log.Printf("[INFO] 已删除旧令牌文件，准备重启服务器生成新令牌")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已删除旧令牌文件，请重新启动服务器以生成新令牌",
		"data": gin.H{
			"action": "restart_required",
		},
	})
}
func formatRuntimeList(runtimes []string) string {
	if len(runtimes) == 0 {
		return "（未检测到已安装的 Runtime）"
	}
	return strings.Join(runtimes, "\n")
}
