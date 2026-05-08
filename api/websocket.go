package api

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"terraria-panel/config"
	"terraria-panel/middleware"
	wshandler "terraria-panel/websocket"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isOriginAllowed(r.Header.Get("Origin"), r)
	},
}

type WebSocketManager struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

type panelLogWSMessage struct {
	Type    string `json:"type"`
	Level   string `json:"level,omitempty"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

var wsManager = &WebSocketManager{
	clients:    make(map[*websocket.Conn]bool),
	broadcast:  make(chan []byte, 256),
	register:   make(chan *websocket.Conn),
	unregister: make(chan *websocket.Conn),
}

func authorizeWebSocketRequest(c *gin.Context) bool {
	if !isOriginAllowed(c.GetHeader("Origin"), c.Request) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "不允许的来源"})
		return false
	}

	tokenString := middleware.ExtractWebSocketToken(c.Request)
	if tokenString == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少 WebSocket 认证令牌"})
		return false
	}

	claims, err := middleware.ParseAndValidateToken(tokenString)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效的 WebSocket 认证令牌"})
		return false
	}

	c.Set("user_id", claims.UserID)
	c.Set("username", claims.Username)
	c.Set("role", claims.Role)
	return true
}

func (manager *WebSocketManager) Run() {
	for {
		select {
		case client := <-manager.register:
			manager.mu.Lock()
			manager.clients[client] = true
			manager.mu.Unlock()
			log.Printf("[WebSocket] 新客户端连接，当前连接数: %d", len(manager.clients))
		case client := <-manager.unregister:
			manager.mu.Lock()
			if _, ok := manager.clients[client]; ok {
				delete(manager.clients, client)
				client.Close()
			}
			manager.mu.Unlock()
			log.Printf("[WebSocket] 客户端断开，当前连接数: %d", len(manager.clients))
		case message := <-manager.broadcast:
			manager.mu.RLock()
			for client := range manager.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("[WebSocket] 发送消息失败: %v", err)
					client.Close()
					delete(manager.clients, client)
				}
			}
			manager.mu.RUnlock()
		}
	}
}
func HandleWebSocket(c *gin.Context) {
	if !authorizeWebSocketRequest(c) {
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] 升级失败: %v", err)
		return
	}
	wsManager.register <- conn
	go func() {
		defer func() {
			wsManager.unregister <- conn
		}()
		conn.SetCloseHandler(func(code int, text string) error {
			log.Printf("[WebSocket] 收到关闭消息: code=%d, text=%s", code, text)
			return nil
		})
		for {
			messageType, _, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[WebSocket] 读取错误: %v", err)
				break
			}
			if messageType == websocket.CloseMessage {
				break
			}
		}
	}()
}

func HandlePanelLogsWS(c *gin.Context) {
	if !authorizeWebSocketRequest(c) {
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] 面板日志升级失败: %v", err)
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	if err := writePanelLogWSMessage(conn, panelLogWSMessage{
		Type:    "connected",
		Message: "面板日志流已连接",
		Time:    time.Now().Format("15:04:05"),
	}); err != nil {
		return
	}

	logFile := config.PanelLogFile()
	offset, err := sendPanelLogHistory(conn, logFile)
	if err != nil {
		log.Printf("[WebSocket] 读取面板历史日志失败: %v", err)
		if writePanelLogWSMessage(conn, panelLogWSMessage{
			Type:    "error",
			Level:   "error",
			Message: "无法读取面板日志",
			Time:    time.Now().Format("15:04:05"),
		}) != nil {
			return
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			nextOffset, err := sendPanelLogUpdates(conn, logFile, offset)
			if err != nil {
				log.Printf("[WebSocket] 推送面板日志失败: %v", err)
				if writePanelLogWSMessage(conn, panelLogWSMessage{
					Type:    "error",
					Level:   "error",
					Message: "面板日志推送中断",
					Time:    time.Now().Format("15:04:05"),
				}) != nil {
					return
				}
				continue
			}
			offset = nextOffset
		}
	}
}

func BroadcastMessage(message []byte) {
	select {
	case wsManager.broadcast <- message:
	default:
		log.Printf("[WebSocket] 广播通道已满，消息被丢弃")
	}
}
func init() {
	go wsManager.Run()
	log.Println("[WebSocket] 管理器已启动")
}
func HandleRoomLogsWS(c *gin.Context) {
	if !authorizeWebSocketRequest(c) {
		return
	}

	wshandler.HandleRoomLogs(c)
}

func HandleServerLogsWS(c *gin.Context) {
	if !authorizeWebSocketRequest(c) {
		return
	}

	wshandler.HandleServerLogs(c)
}

func sendPanelLogHistory(conn *websocket.Conn, logFile string) (int64, error) {
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return 0, writePanelLogWSMessage(conn, panelLogWSMessage{
			Type:    "info",
			Level:   "info",
			Message: "No panel logs yet",
			Time:    time.Now().Format("15:04:05"),
		})
	} else if err != nil {
		return 0, err
	}

	lines, err := readLastNLines(logFile, "500")
	if err != nil {
		return 0, err
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := writePanelLogWSMessage(conn, panelLogEntryFromLine(line)); err != nil {
			return 0, err
		}
	}

	info, err := os.Stat(logFile)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func sendPanelLogUpdates(conn *websocket.Conn, logFile string, offset int64) (int64, error) {
	info, err := os.Stat(logFile)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return offset, err
	}
	if info.Size() < offset {
		offset = 0
	}
	if info.Size() == offset {
		return offset, nil
	}

	file, err := os.Open(logFile)
	if err != nil {
		return offset, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := writePanelLogWSMessage(conn, panelLogEntryFromLine(line)); err != nil {
			return offset, err
		}
	}
	if err := scanner.Err(); err != nil {
		return offset, err
	}

	newOffset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return info.Size(), nil
	}
	return newOffset, nil
}

func writePanelLogWSMessage(conn *websocket.Conn, message panelLogWSMessage) error {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.WriteJSON(message)
}

func panelLogEntryFromLine(line string) panelLogWSMessage {
	return panelLogWSMessage{
		Type:    "log",
		Level:   inferPanelLogLevel(line),
		Message: line,
		Time:    time.Now().Format("15:04:05"),
	}
}

func inferPanelLogLevel(line string) string {
	lowerLine := strings.ToLower(line)
	switch {
	case strings.Contains(lowerLine, "[fatal]"):
		return "fatal"
	case strings.Contains(lowerLine, "[error]"):
		return "error"
	case strings.Contains(lowerLine, "[warn]") || strings.Contains(lowerLine, "[warning]"):
		return "warn"
	case strings.Contains(lowerLine, "[debug]"):
		return "debug"
	case strings.Contains(lowerLine, "[info]"):
		return "info"
	default:
		return "info"
	}
}
