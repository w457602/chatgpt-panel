package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/webauto/chatgpt-panel/internal/config"
	"github.com/webauto/chatgpt-panel/internal/models"
)

// LinuxDoOAuthService Linux DO OAuth 服务
type LinuxDoOAuthService struct{}

var linuxDoOAuthServiceInstance *LinuxDoOAuthService

// GetLinuxDoOAuthService 获取 Linux DO OAuth 服务单例
func GetLinuxDoOAuthService() *LinuxDoOAuthService {
	if linuxDoOAuthServiceInstance == nil {
		linuxDoOAuthServiceInstance = &LinuxDoOAuthService{}
	}
	return linuxDoOAuthServiceInstance
}

// LinuxDoUser Linux DO 用户信息
type LinuxDoUser struct {
	ID         int    `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	TrustLevel int    `json:"trust_level"`
}

// LinuxDoTokenResponse Linux DO Token 响应
type LinuxDoTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// GenerateState 生成随机 state
func (s *LinuxDoOAuthService) GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetAuthorizeURL 获取授权 URL
func (s *LinuxDoOAuthService) GetAuthorizeURL(redirectURI string) (string, error) {
	db := models.GetDB()
	clientID := models.GetConfigValue(db, "linuxdo_oauth_client_id", config.AppConfig.LinuxDoClientID)
	if clientID == "" {
		return "", errors.New("Linux DO Client ID 未配置")
	}

	state, err := s.GenerateState()
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "read")
	params.Set("state", state)

	return fmt.Sprintf("https://connect.linux.do/oauth2/authorize?%s", params.Encode()), nil
}

// ExchangeCode 用授权码换取 Access Token
func (s *LinuxDoOAuthService) ExchangeCode(code, redirectURI string) (*LinuxDoTokenResponse, error) {
	db := models.GetDB()
	clientID := models.GetConfigValue(db, "linuxdo_oauth_client_id", config.AppConfig.LinuxDoClientID)
	clientSecret := models.GetConfigValue(db, "linuxdo_oauth_client_secret", config.AppConfig.LinuxDoClientSecret)

	if clientID == "" || clientSecret == "" {
		return nil, errors.New("Linux DO OAuth 配置不完整")
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	req, err := http.NewRequest("POST", "https://connect.linux.do/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokenResp LinuxDoTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// GetUser 获取用户信息
func (s *LinuxDoOAuthService) GetUser(accessToken string) (*LinuxDoUser, error) {
	req, err := http.NewRequest("GET", "https://connect.linux.do/api/user", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "ChatGPT-Panel/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user failed: %s", string(body))
	}

	var user LinuxDoUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

// UpsertLinuxDoUser 创建或更新 Linux DO 用户
func (s *LinuxDoOAuthService) UpsertLinuxDoUser(uid, username, name string, trustLevel int) error {
	db := models.GetDB()

	var existing models.LinuxDoUser
	result := db.Where("uid = ?", uid).First(&existing)

	if result.Error != nil {
		// 创建新用户
		newUser := models.LinuxDoUser{
			UID:        uid,
			Username:   username,
			Name:       name,
			TrustLevel: trustLevel,
		}
		return db.Create(&newUser).Error
	}

	// 更新现有用户
	updates := map[string]interface{}{
		"username":    username,
		"trust_level": trustLevel,
	}
	if name != "" {
		updates["name"] = name
	}

	return db.Model(&existing).Updates(updates).Error
}

// SignSessionToken 签发 Session Token (JWT)
func (s *LinuxDoOAuthService) SignSessionToken(uid, username, name string, trustLevel int) (string, error) {
	jwtSecret := config.AppConfig.JWTSecret
	if jwtSecret == "" {
		return "", errors.New("JWT Secret 未配置")
	}

	claims := jwt.MapClaims{
		"uid":         uid,
		"username":    username,
		"name":        name,
		"trust_level": trustLevel,
		"exp":         time.Now().Add(30 * 24 * time.Hour).Unix(), // 30天有效期
		"iat":         time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// VerifySessionToken 验证 Session Token
func (s *LinuxDoOAuthService) VerifySessionToken(tokenString string) (*jwt.MapClaims, error) {
	jwtSecret := config.AppConfig.JWTSecret
	if jwtSecret == "" {
		return nil, errors.New("JWT Secret 未配置")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("无效的 Session Token")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return &claims, nil
	}

	return nil, errors.New("无法解析 Token Claims")
}

