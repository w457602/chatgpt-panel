package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL    string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	ServerPort     string
	GinMode        string
	JWTSecret      string
	ExtensionToken string
}

var AppConfig *Config

func Load() error {
	_ = godotenv.Load()

	serverPort := getEnv("PORT", "")
	if serverPort == "" {
		serverPort = getEnv("SERVER_PORT", "8080")
	}

	AppConfig = &Config{
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     getEnv("DB_PASSWORD", "postgres"),
		DBName:         getEnv("DB_NAME", "chatgpt_panel"),
		DBSSLMode:      getEnv("DB_SSLMODE", "disable"),
		ServerPort:     serverPort,
		GinMode:        getEnv("GIN_MODE", "debug"),
		JWTSecret:      getEnv("JWT_SECRET", "chatgpt-panel-secret-key-2024"),
		ExtensionToken: getEnv("EXTENSION_TOKEN", ""),
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
