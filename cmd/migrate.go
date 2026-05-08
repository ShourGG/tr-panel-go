package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"log"
	"os"
	"path/filepath"
	"terraria-panel/db"
	"terraria-panel/models"
	"terraria-panel/storage"
)

func migrateFromJSON() error {
	dataDir := filepath.Join(".", "..", "面板泰拉瑞亚情况")
	dbPath := filepath.Join(dataDir, "panel.db")
	if err := db.Init(dbPath); err != nil {
		return fmt.Errorf("初始化数据库失败: %v", err)
	}
	defer db.Close()
	roomStorage := storage.NewSQLiteRoomStorage(db.DB)
	userStorage := storage.NewSQLiteUserStorage(db.DB)
	roomsFile := filepath.Join(dataDir, "rooms.json")
	if _, err := os.Stat(roomsFile); err == nil {
		log.Println("迁移房间数据...")
		data, err := os.ReadFile(roomsFile)
		if err != nil {
			return fmt.Errorf("读取 rooms.json 失败: %v", err)
		}
		var oldRooms []models.Room
		if err := json.Unmarshal(data, &oldRooms); err != nil {
			return fmt.Errorf("解析 rooms.json 失败: %v", err)
		}
		for _, room := range oldRooms {
			room.Status = "stopped"
			room.PID = 0
			if err := roomStorage.Create(&room); err != nil {
				log.Printf("迁移房间 %s 失败: %v", room.Name, err)
			} else {
				log.Printf("迁移房间完成: %s (ID: %d)", room.Name, room.ID)
			}
		}
	}
	log.Println("创建默认管理员...")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("q2e4t6u8"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %v", err)
	}
	admin := &models.User{
		Username: "shour",
		Password: string(hashedPassword),
		Role:     "admin",
	}
	if err := userStorage.Create(admin); err != nil {
		log.Printf("创建管理员失败（可能已存在）: %v", err)
	} else {
		log.Printf("创建管理员完成: %s", admin.Username)
	}
	log.Println("数据迁移完成")
	log.Println("数据库位置:", dbPath)
	log.Println("管理员账号: shour / q2e4t6u8")
	return nil
}
func main() {
	log.Println("开始数据迁移...")
	if err := migrateFromJSON(); err != nil {
		log.Fatalf("迁移失败: %v", err)
	}
}
