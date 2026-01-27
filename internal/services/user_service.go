package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/webauto/chatgpt-panel/internal/config"
	"github.com/webauto/chatgpt-panel/internal/models"
)

type UserService struct {
	db *models.User
}

func NewUserService() *UserService {
	return &UserService{}
}

// Login 用户登录
func (s *UserService) Login(username, password string) (*models.LoginResponse, error) {
	var user models.User
	if err := models.GetDB().Where("username = ?", username).First(&user).Error; err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	if !user.CheckPassword(password) {
		return nil, errors.New("用户名或密码错误")
	}

	if user.Status != "active" {
		return nil, errors.New("用户已被禁用")
	}

	// 更新最后登录时间
	now := time.Now()
	user.LastLogin = &now
	models.GetDB().Save(&user)

	// 生成 JWT Token
	token, err := s.GenerateToken(&user)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		Token: token,
		User:  &user,
	}, nil
}

// GenerateToken 生成 JWT Token
func (s *UserService) GenerateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.AppConfig.JWTSecret))
}

// ValidateToken 验证 Token
func (s *UserService) ValidateToken(tokenString string) (*jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("无效的签名方法")
		}
		return []byte(config.AppConfig.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return &claims, nil
	}

	return nil, errors.New("无效的Token")
}

// GetByID 根据 ID 获取用户
func (s *UserService) GetByID(id uint) (*models.User, error) {
	var user models.User
	if err := models.GetDB().First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Create 创建用户
func (s *UserService) Create(req *models.CreateUserRequest) (*models.User, error) {
	user := &models.User{
		Username: req.Username,
		Email:    req.Email,
		Role:     req.Role,
		Status:   "active",
	}
	if user.Role == "" {
		user.Role = "user"
	}
	if err := user.SetPassword(req.Password); err != nil {
		return nil, err
	}
	if err := models.GetDB().Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

// Update 更新用户
func (s *UserService) Update(id uint, req *models.UpdateUserRequest) error {
	updates := make(map[string]interface{})
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.Role != "" {
		updates["role"] = req.Role
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.Password != "" {
		user := &models.User{}
		user.SetPassword(req.Password)
		updates["password"] = user.Password
	}
	return models.GetDB().Model(&models.User{}).Where("id = ?", id).Updates(updates).Error
}

// GetByUsername 根据用户名获取用户
func (s *UserService) GetByUsername(username string) (*models.User, error) {
	var user models.User
	if err := models.GetDB().Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateLastLogin 更新最后登录时间
func (s *UserService) UpdateLastLogin(id uint) error {
	now := time.Now()
	return models.GetDB().Model(&models.User{}).Where("id = ?", id).Update("last_login", now).Error
}

// UpdatePassword 更新密码
func (s *UserService) UpdatePassword(id uint, password string) error {
	user := &models.User{}
	if err := user.SetPassword(password); err != nil {
		return err
	}
	return models.GetDB().Model(&models.User{}).Where("id = ?", id).Update("password", user.Password).Error
}

