package websocket

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"terraria-panel/config"
	"terraria-panel/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hpcloud/tail"
)

func init() {
	utils.BroadcastPluginServerLog = BroadcastPluginServerLog
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	conn   *websocket.Conn
	roomID int
	send   chan []byte
}

var (
	clients   = make(map[*Client]bool)
	clientsMu sync.RWMutex
	broadcast = make(chan []byte, 256)
)

func BroadcastMessage(data []byte) {
	clientsMu.RLock()
	clientCount := len(clients)
	clientsMu.RUnlock()
	log.Printf("[WebSocket] 准备广播消息到 %d 个客户端: %s\n", clientCount, string(data))
	select {
	case broadcast <- data:
		log.Println("[WebSocket] 消息已放入广播队列")
	default:
		log.Println("[WebSocket] ⚠️ 广播通道已满，消息被丢弃")
	}
}
func init() {
	go handleBroadcast()
}
func handleBroadcast() {
	for {
		message := <-broadcast
		log.Printf("[广播] 从队列取出消息，准备发送给所有客户端\n")
		clientsMu.RLock()
		clientCount := len(clients)
		log.Printf("[广播] 当前连接客户端数: %d\n", clientCount)
		successCount := 0
		for client := range clients {
			select {
			case client.send <- message:
				successCount++
			default:
				log.Println("[广播] 客户端发送队列已满，关闭连接")
				close(client.send)
				delete(clients, client)
			}
		}
		clientsMu.RUnlock()
		log.Printf("[广播] 消息发送完成: 成功 %d/%d\n", successCount, clientCount)
	}
}
func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket升级失败:", err)
		return
	}
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}
	clientsMu.Lock()
	clients[client] = true
	clientsMu.Unlock()
	log.Println("WebSocket客户端连接成功")
	welcomeMsg := map[string]interface{}{
		"type":    "connected",
		"message": "🎮 连接成功！实时日志已启动",
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}
	data, _ := json.Marshal(welcomeMsg)
	conn.WriteMessage(websocket.TextMessage, data)
	go client.writePump()
	client.readPump()
}
func websocketLogFile(roomID int) string {
	if roomID == 0 {
		return config.PluginServerLogFile()
	}
	return config.RoomLogFile(roomID)
}

func (c *Client) readPump() {
	defer func() {
		clientsMu.Lock()
		delete(clients, c)
		clientsMu.Unlock()
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败:", err)
			break
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		if msg["type"] == "subscribe" {
			if roomID, ok := msg["roomId"].(float64); ok {
				c.roomID = int(roomID)
				log.Printf("客户端订阅房间 %d 的日志", c.roomID)
				go c.sendHistoryLogs()
			}
		}
	}
}
func (c *Client) writePump() {
	defer c.conn.Close()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}
func (c *Client) sendHistoryLogs() {
	logFile := websocketLogFile(c.roomID)
	file, err := os.Open(logFile)
	if err != nil {
		return
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 100 {
			lines = lines[1:]
		}
	}
	for _, line := range lines {
		logMsg := map[string]interface{}{
			"type":    "log",
			"roomId":  c.roomID,
			"message": line,
			"time":    time.Now().Format("15:04:05"),
		}
		data, _ := json.Marshal(logMsg)
		c.send <- data
	}
}
func BroadcastLog(roomID int, message string) {
	logMsg := map[string]interface{}{
		"type":    "log",
		"roomId":  roomID,
		"message": message,
		"time":    time.Now().Format("15:04:05"),
	}
	data, _ := json.Marshal(logMsg)
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for client := range clients {
		if client.roomID == roomID {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

type LogClient struct {
	conn   *websocket.Conn
	roomID int
	send   chan []byte
	tail   *tail.Tail
	mu     sync.Mutex
	done   chan struct{}
}

var (
	logClients   = make(map[*LogClient]bool)
	logClientsMu sync.RWMutex
)

func HandleRoomLogs(c *gin.Context) {
	roomIDStr := c.Param("id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid room ID"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] Failed to upgrade connection: %v", err)
		return
	}
	client := &LogClient{
		conn:   conn,
		roomID: roomID,
		send:   make(chan []byte, 256),
		done:   make(chan struct{}),
	}
	logClientsMu.Lock()
	logClients[client] = true
	logClientsMu.Unlock()
	log.Printf("[WebSocket] Client connected to room %d logs", roomID)
	welcomeMsg := map[string]interface{}{
		"type":    "connected",
		"message": fmt.Sprintf("🎮 已连接到房间 %d 的日志流", roomID),
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}
	data, _ := json.Marshal(welcomeMsg)
	conn.WriteMessage(websocket.TextMessage, data)
	if roomID == 0 {
		buffer := utils.GetPluginServerOutputBuffer()
		if buffer != "" {
			bufferMsg := map[string]interface{}{
				"type":         "log",
				"message":      buffer,
				"lineComplete": false,
				"time":         time.Now().Format("15:04:05"),
			}
			data, _ := json.Marshal(bufferMsg)
			conn.WriteMessage(websocket.TextMessage, data)
			log.Printf("[WebSocket] Sent output buffer to client (%d chars)", len(buffer))
		}
	}
	go client.writePump()
	go client.tailLogs()
	client.readPump()
}

func queueLogClientMessage(c *LogClient, payload map[string]interface{}) {
	data, _ := json.Marshal(payload)
	select {
	case <-c.done:
		return
	case c.send <- data:
	default:
		log.Println("[WebSocket] Send buffer full, dropping log payload")
	}
}

func waitForLogFile(c *LogClient, logFile string) bool {
	queueLogClientMessage(c, map[string]interface{}{
		"type":    "info",
		"message": "⏳ 等待服务器启动并生成日志文件...",
		"time":    time.Now().Format("15:04:05"),
	})

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		if _, err := os.Stat(logFile); err == nil {
			queueLogClientMessage(c, map[string]interface{}{
				"type":    "info",
				"message": "✅ 已检测到日志文件，开始读取日志...",
				"time":    time.Now().Format("15:04:05"),
			})
			return true
		}

		select {
		case <-c.done:
			return false
		case <-ticker.C:
		}
	}
}

func (c *LogClient) readPump() {
	defer func() {
		logClientsMu.Lock()
		delete(logClients, c)
		logClientsMu.Unlock()
		select {
		case <-c.done:
		default:
			close(c.done)
		}
		if c.tail != nil {
			c.tail.Stop()
		}
		c.conn.Close()
		log.Printf("[WebSocket] Client disconnected from room %d logs", c.roomID)
	}()
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] Read error: %v", err)
			}
			break
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		if msg["type"] == "ping" {
			pongMsg := map[string]interface{}{
				"type": "pong",
				"time": time.Now().Format("2006-01-02 15:04:05"),
			}
			data, _ := json.Marshal(pongMsg)
			c.send <- data
		}
	}
}
func (c *LogClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case <-c.done:
			return
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
func (c *LogClient) tailLogs() {
	logFile := websocketLogFile(c.roomID)
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		if !waitForLogFile(c, logFile) {
			return
		}
	}
	c.sendHistoryLogs(logFile)
	t, err := tail.TailFile(logFile, tail.Config{
		Follow: true,
		ReOpen: true,
		Poll:   true,
		Location: &tail.SeekInfo{
			Offset: 0,
			Whence: io.SeekEnd,
		},
	})
	if err != nil {
		log.Printf("[WebSocket] Failed to tail log file: %v", err)
		queueLogClientMessage(c, map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("❌ 无法读取日志文件: %v", err),
			"time":    time.Now().Format("15:04:05"),
		})
		return
	}
	c.mu.Lock()
	c.tail = t
	c.mu.Unlock()
	queueLogClientMessage(c, map[string]interface{}{
		"type":    "info",
		"message": "✅ 开始实时推送日志...",
		"time":    time.Now().Format("15:04:05"),
	})
	for line := range t.Lines {
		select {
		case <-c.done:
			return
		default:
		}
		if line.Err != nil {
			log.Printf("[WebSocket] Error reading log line: %v", line.Err)
			continue
		}
		logMsg := map[string]interface{}{
			"type":         "log",
			"message":      line.Text,
			"lineComplete": true,
			"time":         time.Now().Format("15:04:05"),
		}
		data, _ := json.Marshal(logMsg)
		select {
		case <-c.done:
			return
		case c.send <- data:
		default:
			log.Println("[WebSocket] Send buffer full, dropping log line")
		}
	}
}
func (c *LogClient) sendHistoryLogs(logFile string) {
	file, err := os.Open(logFile)
	if err != nil {
		return
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 100 {
			lines = lines[1:]
		}
	}
	queueLogClientMessage(c, map[string]interface{}{
		"type":    "info",
		"message": fmt.Sprintf("📜 加载最近 %d 条历史日志", len(lines)),
		"time":    time.Now().Format("15:04:05"),
	})
	for _, line := range lines {
		logMsg := map[string]interface{}{
			"type":         "log",
			"message":      line,
			"lineComplete": true,
			"time":         time.Now().Format("15:04:05"),
		}
		queueLogClientMessage(c, logMsg)
		time.Sleep(1 * time.Millisecond)
	}
}
func HandleServerLogs(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] Failed to upgrade server logs connection: %v", err)
		return
	}
	send := make(chan []byte, 512)
	done := make(chan struct{})
	type roomLogFile struct {
		id   int
		path string
	}
	tails := make(map[string]*tail.Tail)
	var tailsMu sync.Mutex

	welcomeMsg := map[string]interface{}{
		"type":    "connected",
		"message": "🎮 已连接到全局服务器日志流",
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}
	data, _ := json.Marshal(welcomeMsg)
	conn.WriteMessage(websocket.TextMessage, data)

	queueServerMessage := func(payload map[string]interface{}) {
		d, _ := json.Marshal(payload)
		select {
		case <-done:
			return
		case send <- d:
		default:
			log.Println("[WebSocket] Send buffer full for server logs")
		}
	}

	sendRoomHistoryLogs := func(roomID int, logFile string) {
		file, err := os.Open(logFile)
		if err != nil {
			return
		}
		defer file.Close()

		lines := []string{}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			if len(lines) > 50 {
				lines = lines[1:]
			}
		}
		for _, line := range lines {
			queueServerMessage(map[string]interface{}{
				"type":     "log",
				"message":  line,
				"roomName": fmt.Sprintf("Room %d", roomID),
				"time":     time.Now().Format("15:04:05"),
			})
		}
	}

	attachRoomTail := func(roomID int, logFile string) {
		tailsMu.Lock()
		if _, exists := tails[logFile]; exists {
			tailsMu.Unlock()
			return
		}
		tailsMu.Unlock()
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			return
		}

		sendRoomHistoryLogs(roomID, logFile)

		t, err := tail.TailFile(logFile, tail.Config{
			Follow:   true,
			ReOpen:   true,
			Poll:     true,
			Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
		})
		if err != nil {
			log.Printf("[WebSocket] Failed to tail %s: %v", logFile, err)
			return
		}
		tailsMu.Lock()
		tails[logFile] = t
		tailsMu.Unlock()

		go func(t *tail.Tail, rid int, path string) {
			defer func() {
				tailsMu.Lock()
				delete(tails, path)
				tailsMu.Unlock()
			}()
			for {
				select {
				case <-done:
					return
				case line, ok := <-t.Lines:
					if !ok {
						return
					}
					if line.Err != nil {
						continue
					}
					queueServerMessage(map[string]interface{}{
						"type":     "log",
						"message":  line.Text,
						"roomName": fmt.Sprintf("Room %d", rid),
						"time":     time.Now().Format("15:04:05"),
					})
				}
			}
		}(t, roomID, logFile)
	}

	collectRoomFiles := func() []roomLogFile {
		logDir := config.LogsDir
		entries, err := os.ReadDir(logDir)
		if err != nil {
			return nil
		}
		files := make([]roomLogFile, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			var rid int
			if _, err := fmt.Sscanf(name, "room-%d.log", &rid); err == nil && rid > 0 {
				files = append(files, roomLogFile{id: rid, path: config.RoomLogFile(rid)})
			}
		}
		return files
	}

	waitingAnnounced := false
	scanRoomFiles := func() {
		roomFiles := collectRoomFiles()
		if len(roomFiles) == 0 {
			if !waitingAnnounced {
				waitingAnnounced = true
				queueServerMessage(map[string]interface{}{
					"type":    "info",
					"message": "⏳ 暂无房间日志文件，请先启动至少一个房间",
					"time":    time.Now().Format("15:04:05"),
				})
			}
			return
		}

		if waitingAnnounced {
			waitingAnnounced = false
			queueServerMessage(map[string]interface{}{
				"type":    "info",
				"message": fmt.Sprintf("✅ 已检测到 %d 个房间日志文件，开始实时监听", len(roomFiles)),
				"time":    time.Now().Format("15:04:05"),
			})
		}

		for _, roomFile := range roomFiles {
			attachRoomTail(roomFile.id, roomFile.path)
		}
	}

	scanRoomFiles()
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				scanRoomFiles()
			}
		}
	}()

	// Write pump
	go func() {
		ticker := time.NewTicker(54 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case msg, ok := <-send:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read pump (keep alive)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	// Cleanup
	close(done)
	tailsMu.Lock()
	for _, t := range tails {
		t.Stop()
	}
	tailsMu.Unlock()
	conn.Close()
	log.Println("[WebSocket] Server logs client disconnected")
}

func BroadcastPluginServerLog(message string) {
	logClientsMu.RLock()
	defer logClientsMu.RUnlock()
	for client := range logClients {
		if client.roomID == 0 {
			logMsg := map[string]interface{}{
				"type":         "log",
				"message":      message,
				"lineComplete": false,
				"time":         time.Now().Format("15:04:05"),
			}
			data, _ := json.Marshal(logMsg)
			select {
			case client.send <- data:
			default:
				log.Println("[WebSocket] Send buffer full for plugin server log")
			}
		}
	}
}
