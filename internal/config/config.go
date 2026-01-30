package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL           string
	DBHost                string
	DBPort                string
	DBUser                string
	DBPassword            string
	DBName                string
	DBSSLMode             string
	ServerPort            string
	GinMode               string
	JWTSecret             string
	ExtensionToken        string
	LinuxDoClientID       string
	LinuxDoClientSecret   string
	LinuxDoCreditPID      string
	LinuxDoCreditKey      string
	LinuxDoCreditBaseURL  string
}

var AppConfig *Config

func Load() error {
	_ = godotenv.Load()

	serverPort := getEnv("PORT", "")
	if serverPort == "" {
		serverPort = getEnv("SERVER_PORT", "8080")
	}

	AppConfig = &Config{
		DatabaseURL:          getEnv("DATABASE_URL", ""),
		DBHost:               getEnv("DB_HOST", "localhost"),
		DBPort:               getEnv("DB_PORT", "5432"),
		DBUser:               getEnv("DB_USER", "postgres"),
		DBPassword:           getEnv("DB_PASSWORD", "postgres"),
		DBName:               getEnv("DB_NAME", "chatgpt_panel"),
		DBSSLMode:            getEnv("DB_SSLMODE", "disable"),
		ServerPort:           serverPort,
		GinMode:              getEnv("GIN_MODE", "debug"),
		JWTSecret:            getEnv("JWT_SECRET", "chatgpt-panel-secret-key-2024"),
		ExtensionToken:       getEnv("EXTENSION_TOKEN", ""),
		LinuxDoClientID:      getEnv("LINUXDO_CLIENT_ID", ""),
		LinuxDoClientSecret:  getEnv("LINUXDO_CLIENT_SECRET", ""),
		LinuxDoCreditPID:     getEnv("LINUXDO_CREDIT_PID", ""),
		LinuxDoCreditKey:     getEnv("LINUXDO_CREDIT_KEY", ""),
		LinuxDoCreditBaseURL: getEnv("LINUXDO_CREDIT_BASE_URL", "https://credit.linux.do/epay"),
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (c *Config) GetDSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
}

// RefreshFromDB 从数据库刷新配置（优先使用数据库配置）
func (c *Config) RefreshFromDB(db interface{}) error {
	// 这个方法将在 services 层调用，由 models.GetConfigValue 提供实际值
	return nil
}

