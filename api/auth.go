package api
import (
	"net/http"
	"terraria-panel/middleware"
	"terraria-panel/models"
	"terraria-panel/storage"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)
var (
	userStorage storage.UserStorage
)
func SetUserStorage(s storage.UserStorage) {
	userStorage = s
}
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	user, err := userStorage.GetByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("服务器错误"))
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("用户名或密码错误"))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("用户名或密码错误"))
		return
	}
	token, err := middleware.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("生成令牌失败"))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	}))
}
func CheckHasUsers(c *gin.Context) {
	count, err := userStorage.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("检查失败"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"hasUsers": count > 0,
		"userCount": count,
	})
}
func Register(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}
	userCount, err := userStorage.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("服务器错误"))
		return
	}
	if userCount > 0 {
		c.JSON(http.StatusForbidden, models.ErrorResponse("系统已初始化完成，不允许注册新用户"))
		return
	}
	existingUser, err := userStorage.GetByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("服务器错误"))
		return
	}
	if existingUser != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("用户名已存在"))
		return
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("密码加密失败"))
		return
	}
	role := "user"
	if userCount == 0 {
		role = "admin"
	}
	user := &models.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Role:     role,
	}
	if err := userStorage.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("创建用户失败"))
		return
	}
	if userCount == 0 {
		token, err := middleware.GenerateToken(user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse("生成令牌失败"))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "注册成功，已自动登录",
			"data": gin.H{
				"token": token,
				"user": gin.H{
					"id":       user.ID,
					"username": user.Username,
					"role":     user.Role,
				},
			},
		})
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse("注册成功"))
}
