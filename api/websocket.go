package api

import (
	"log"
	"net/http"
	"sync"
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
