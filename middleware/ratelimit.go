package middleware
import (
	"net/http"
	"sync"
	"time"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}
var (
	visitors = make(map[string]*visitor)
	mu       sync.Mutex
)
func cleanupVisitors() {
	for {
		time.Sleep(time.Minute)
		mu.Lock()
		for ip, v := range visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(visitors, ip)
			}
		}
		mu.Unlock()
	}
}
func init() {
	go cleanupVisitors()
}
func getVisitor(ip string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()
	v, exists := visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(2, 20)
		visitors[ip] = &visitor{limiter, time.Now()}
		return limiter
	}
	v.lastSeen = time.Now()
	return v.limiter
}
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := getVisitor(ip)
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
func StrictRateLimitMiddleware() gin.HandlerFunc {
	visitors := make(map[string]*rate.Limiter)
	mu := sync.Mutex{}
	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		limiter, exists := visitors[ip]
		if !exists {
			limiter = rate.NewLimiter(rate.Every(time.Minute), 5)
			visitors[ip] = limiter
		}
		mu.Unlock()
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "登录尝试次数过多，请1分钟后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
