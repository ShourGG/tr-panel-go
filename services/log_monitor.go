package services
import (
	"bufio"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"terraria-panel/config"
	"terraria-panel/models"
	"terraria-panel/storage"
	"time"
)
type LogMonitor struct {
	db                  *sql.DB
	roomStorage         storage.RoomStorage
	sessionStorage      storage.PlayerSessionStorage
	statsStorage        storage.PlayerStatsStorage
	dailyStatsStorage   storage.PlayerDailyStatsStorage
	playerNameToID      map[string]int
	activeRooms         map[int]bool
	mu                  sync.RWMutex
	lastReadPos         map[string]int64
	positionMutex       sync.RWMutex
	stopChan            chan struct{}
	wg                  sync.WaitGroup
}
func NewLogMonitor(
	db *sql.DB,
	roomStorage storage.RoomStorage,
	sessionStorage storage.PlayerSessionStorage,
	statsStorage storage.PlayerStatsStorage,
	dailyStatsStorage storage.PlayerDailyStatsStorage,
) *LogMonitor {
	return &LogMonitor{
		db:                db,
		roomStorage:       roomStorage,
		sessionStorage:    sessionStorage,
		statsStorage:      statsStorage,
		dailyStatsStorage: dailyStatsStorage,
		playerNameToID:    make(map[string]int),
		activeRooms:       make(map[int]bool),
		lastReadPos:       make(map[string]int64),
		stopChan:          make(chan struct{}),
	}
}
func (m *LogMonitor) Start() {
	log.Println("📊 Starting log monitor service...")
	m.loadPlayerNameCache()
	m.wg.Add(1)
	go m.monitorLoop()
	log.Println("✅ Log monitor service started")
}
func (m *LogMonitor) Stop() {
	log.Println("🛑 Stopping log monitor service...")
	close(m.stopChan)
	m.wg.Wait()
	log.Println("✅ Log monitor service stopped")
}
func (m *LogMonitor) loadPlayerNameCache() {
	query := `SELECT id, name FROM players`
	rows, err := m.db.Query(query)
	if err != nil {
		log.Printf("Failed to load player name cache: %v", err)
		return
	}
	defer rows.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		m.playerNameToID[name] = id
	}
	log.Printf("Loaded %d player names into cache", len(m.playerNameToID))
}
func (m *LogMonitor) monitorLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.checkLogs()
		}
	}
}
func (m *LogMonitor) checkLogs() {
	rooms, err := m.roomStorage.GetAll()
	if err != nil {
		log.Printf("Failed to get rooms: %v", err)
		return
	}
	for _, room := range rooms {
		if room.Status == "running" {
			m.processRoomLog(&room)
		}
	}
}
func (m *LogMonitor) processRoomLog(room *models.Room) {
	logFile := filepath.Join(config.DataDir, "logs", "room-"+strconv.Itoa(room.ID)+".log")
	fileInfo, err := os.Stat(logFile)
	if os.IsNotExist(err) {
		return
	}
	currentSize := fileInfo.Size()
	m.positionMutex.RLock()
	lastPos, exists := m.lastReadPos[logFile]
	m.positionMutex.RUnlock()
	if currentSize < lastPos {
		lastPos = 0
	}
	if exists && currentSize == lastPos {
		return
	}
	file, err := os.Open(logFile)
	if err != nil {
		log.Printf("Failed to open log file %s: %v", logFile, err)
		return
	}
	defer file.Close()
	if lastPos > 0 {
		file.Seek(lastPos, 0)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		m.parseLine(line, room.ID)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading log file %s: %v", logFile, err)
	}
	m.positionMutex.Lock()
	m.lastReadPos[logFile] = currentSize
	m.positionMutex.Unlock()
}
func (m *LogMonitor) parseLine(line string, roomID int) {
	joinPattern := regexp.MustCompile(`:\s*(.+?)\s*\(([0-9.]+):(\d+)\)\s*has joined`)
	if matches := joinPattern.FindStringSubmatch(line); matches != nil {
		playerName := strings.TrimSpace(matches[1])
		ipAddress := matches[2]
		m.handlePlayerJoin(playerName, ipAddress, roomID)
		return
	}
	leavePattern := regexp.MustCompile(`:\s*(.+?)\s*has left`)
	if matches := leavePattern.FindStringSubmatch(line); matches != nil {
		playerName := strings.TrimSpace(matches[1])
		m.handlePlayerLeave(playerName, roomID)
		return
	}
}
func (m *LogMonitor) handlePlayerJoin(playerName, ipAddress string, roomID int) {
	playerID := m.getOrCreatePlayerID(playerName, ipAddress, roomID)
	if playerID == 0 {
		return
	}
	activeSession, err := m.sessionStorage.GetActiveSession(playerID, roomID)
	if err != nil {
		log.Printf("Failed to check active session: %v", err)
		return
	}
	if activeSession != nil {
		return
	}
	session := &models.PlayerSession{
		PlayerID:  playerID,
		RoomID:    roomID,
		JoinTime:  time.Now(),
		IPAddress: ipAddress,
	}
	if err := m.sessionStorage.Create(session); err != nil {
		log.Printf("Failed to create session: %v", err)
		return
	}
	m.updatePlayerStatsOnJoin(playerID)
	m.updatePlayerStatus(playerID, roomID, "online")
	log.Printf("Player %s joined room %d", playerName, roomID)
}
func (m *LogMonitor) handlePlayerLeave(playerName string, roomID int) {
	playerID := m.getPlayerID(playerName)
	if playerID == 0 {
		return
	}
	activeSession, err := m.sessionStorage.GetActiveSession(playerID, roomID)
	if err != nil {
		log.Printf("Failed to get active session: %v", err)
		return
	}
	if activeSession == nil {
		return
	}
	leaveTime := time.Now()
	duration := int(leaveTime.Sub(activeSession.JoinTime).Seconds())
	if err := m.sessionStorage.UpdateLeaveTime(activeSession.ID, leaveTime, duration); err != nil {
		log.Printf("Failed to update session: %v", err)
		return
	}
	m.updatePlayerStatsOnLeave(playerID, duration)
	m.updatePlayerStatus(playerID, roomID, "offline")
	log.Printf("Player %s left room %d (duration: %d seconds)", playerName, roomID, duration)
}
func (m *LogMonitor) getOrCreatePlayerID(playerName, ipAddress string, roomID int) int {
	m.mu.RLock()
	if id, exists := m.playerNameToID[playerName]; exists {
		m.mu.RUnlock()
		return id
	}
	m.mu.RUnlock()
	query := `SELECT id FROM players WHERE name = ?`
	var id int
	err := m.db.QueryRow(query, playerName).Scan(&id)
	if err == nil {
		m.mu.Lock()
		m.playerNameToID[playerName] = id
		m.mu.Unlock()
		return id
	}
	insertQuery := `INSERT INTO players (name, ip, room_id, status) VALUES (?, ?, ?, 'offline')`
	result, err := m.db.Exec(insertQuery, playerName, ipAddress, roomID)
	if err != nil {
		log.Printf("Failed to create player: %v", err)
		return 0
	}
	newID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Failed to get new player ID: %v", err)
		return 0
	}
	id = int(newID)
	m.mu.Lock()
	m.playerNameToID[playerName] = id
	m.mu.Unlock()
	stats := &models.PlayerStats{
		PlayerID:  id,
		FirstSeen: time.Now(),
	}
	m.statsStorage.Create(stats)
	return id
}
func (m *LogMonitor) getPlayerID(playerName string) int {
	m.mu.RLock()
	if id, exists := m.playerNameToID[playerName]; exists {
		m.mu.RUnlock()
		return id
	}
	m.mu.RUnlock()
	query := `SELECT id FROM players WHERE name = ?`
	var id int
	err := m.db.QueryRow(query, playerName).Scan(&id)
	if err != nil {
		return 0
	}
	m.mu.Lock()
	m.playerNameToID[playerName] = id
	m.mu.Unlock()
	return id
}
func (m *LogMonitor) updatePlayerStatsOnJoin(playerID int) {
	now := time.Now()
	m.statsStorage.IncrementLoginCount(playerID)
	m.statsStorage.UpdateLastLogin(playerID, now)
}
func (m *LogMonitor) updatePlayerStatsOnLeave(playerID int, duration int) {
	now := time.Now()
	m.statsStorage.IncrementPlayTime(playerID, duration)
	m.statsStorage.UpdateLastLogout(playerID, now)
}
func (m *LogMonitor) updatePlayerStatus(playerID, roomID int, status string) {
	query := `UPDATE players SET status = ?, room_id = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?`
	m.db.Exec(query, status, roomID, playerID)
}
