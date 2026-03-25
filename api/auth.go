package api

import (
	"net/http"
	"strings"
	"terraria-panel/middleware"
	"terraria-panel/models"
	"terraria-panel/storage"
	"unicode"
	"unicode/utf8"

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

type UpdateProfileRequest struct {
	CustomUID string `json:"customUid" binding:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required"`
}

func buildUserPayload(user *models.User) gin.H {
	return gin.H{
		"id":        user.ID,
		"username":  user.Username,
		"role":      user.Role,
		"customUid": user.CustomUID,
		"createdAt": user.CreatedAt,
		"updatedAt": user.UpdatedAt,
	}
}

func validateUsername(username string) string {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		return "用户名不能为空"
	}

	length := utf8.RuneCountInString(trimmed)
	if length < 3 || length > 20 {
		return "用户名长度需在 3 到 20 个字符之间"
	}

	for _, ch := range trimmed {
		if unicode.IsSpace(ch) {
			return "用户名不能包含空白字符"
		}
	}

	return ""
}

func validateCustomUID(customUID string) string {
	trimmed := strings.TrimSpace(customUID)
	if trimmed == "" {
		return "自定义UID不能为空"
	}

	length := utf8.RuneCountInString(trimmed)
	if length < 3 || length > 32 {
		return "自定义UID长度需在 3 到 32 个字符之间"
	}

	for _, ch := range trimmed {
		if unicode.IsSpace(ch) {
			return "自定义UID不能包含空白字符"
		}
	}

	return ""
}

func validatePassword(password string) string {
	if strings.TrimSpace(password) == "" {
		return "密码不能为空"
	}

	if utf8.RuneCountInString(password) < 8 {
		return "密码至少需要 8 个字符"
	}

	if len([]byte(password)) > 72 {
		return "密码长度不能超过 72 字节"
	}

	return ""
}

func getUserByLoginIdentifier(identifier string) (*models.User, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil, nil
	}

	user, err := userStorage.GetByUsername(trimmed)
	if err != nil || user != nil {
		return user, err
	}

	return userStorage.GetByCustomUID(trimmed)
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}

	loginIdentifier := strings.TrimSpace(req.Username)
	if loginIdentifier == "" || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("账号和密码不能为空"))
		return
	}

	user, err := getUserByLoginIdentifier(loginIdentifier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("服务器错误"))
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("账号或密码错误"))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("账号或密码错误"))
		return
	}

	token, err := middleware.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("生成令牌失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"token": token,
		"user":  buildUserPayload(user),
	}))
}

func CheckHasUsers(c *gin.Context) {
	count, err := userStorage.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("检查失败"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"hasUsers":  count > 0,
		"userCount": count,
	})
}

func Register(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if message := validateUsername(req.Username); message != "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(message))
		return
	}
	if message := validatePassword(req.Password); message != "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(message))
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

	conflictUIDUser, err := userStorage.GetByCustomUID(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("服务器错误"))
		return
	}
	if conflictUIDUser != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("用户名已被占用，请更换"))
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
		Username:  req.Username,
		Password:  string(hashedPassword),
		Role:      role,
		CustomUID: req.Username,
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
				"user":  buildUserPayload(user),
			},
		})
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse("注册成功"))
}

func GetCurrentUser(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("登录状态无效"))
		return
	}

	user, err := userStorage.GetByUsername(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("获取用户信息失败"))
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("用户不存在"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(buildUserPayload(user)))
}

func UpdateCurrentUserProfile(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("登录状态无效"))
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}

	if message := validateCustomUID(req.CustomUID); message != "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(message))
		return
	}

	user, err := userStorage.GetByUsername(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("获取用户信息失败"))
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("用户不存在"))
		return
	}

	trimmedUID := strings.TrimSpace(req.CustomUID)

	usernameOwner, err := userStorage.GetByUsername(trimmedUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("检查自定义UID失败"))
		return
	}
	if usernameOwner != nil && usernameOwner.ID != user.ID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("该自定义UID与现有用户名冲突，请更换"))
		return
	}

	existing, err := userStorage.GetByCustomUID(trimmedUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("检查自定义UID失败"))
		return
	}
	if existing != nil && existing.ID != user.ID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("该自定义UID已被占用"))
		return
	}

	user.CustomUID = trimmedUID
	if err := userStorage.Update(user); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("保存用户资料失败"))
		return
	}

	updatedUser, err := userStorage.GetByUsername(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("读取最新用户资料失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(buildUserPayload(updatedUser)))
}

func ChangeCurrentUserPassword(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse("登录状态无效"))
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("参数错误"))
		return
	}

	if strings.TrimSpace(req.CurrentPassword) == "" || strings.TrimSpace(req.NewPassword) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("密码不能为空"))
		return
	}
	if message := validatePassword(req.NewPassword); message != "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(message))
		return
	}
	if req.CurrentPassword == req.NewPassword {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("新密码不能与原密码相同"))
		return
	}

	user, err := userStorage.GetByUsername(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("获取用户信息失败"))
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse("用户不存在"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse("原始密码错误"))
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("密码加密失败"))
		return
	}

	user.Password = string(hashedPassword)
	if err := userStorage.Update(user); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse("修改密码失败"))
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse("密码修改成功"))
}
