package services

import (
	"github.com/webauto/chatgpt-panel/internal/config"
	"github.com/webauto/chatgpt-panel/internal/models"
	"gorm.io/gorm"
)

// SystemConfigService 系统配置服务
type SystemConfigService struct{}

var systemConfigServiceInstance *SystemConfigService

// GetSystemConfigService 获取系统配置服务单例
func GetSystemConfigService() *SystemConfigService {
	if systemConfigServiceInstance == nil {
		systemConfigServiceInstance = &SystemConfigService{}
	}
	return systemConfigServiceInstance
}

// GetLinuxDoOAuthConfig 获取 Linux DO OAuth 配置（优先数据库）
func (s *SystemConfigService) GetLinuxDoOAuthConfig(db *gorm.DB) map[string]interface{} {
	clientID := models.GetConfigValue(db, "linuxdo_oauth_client_id", config.AppConfig.LinuxDoClientID)
	clientSecret := models.GetConfigValue(db, "linuxdo_oauth_client_secret", config.AppConfig.LinuxDoClientSecret)

	return map[string]interface{}{
		"client_id":        clientID,
		"client_id_stored": clientID != "" && clientID != config.AppConfig.LinuxDoClientID,
		"secret_set":       clientSecret != "",
		"secret_stored":    clientSecret != "" && clientSecret != config.AppConfig.LinuxDoClientSecret,
	}
}

// UpdateLinuxDoOAuthConfig 更新 Linux DO OAuth 配置
func (s *SystemConfigService) UpdateLinuxDoOAuthConfig(db *gorm.DB, clientID, clientSecret string) error {
	if clientID != "" {
		if err := models.UpsertConfig(db, "linuxdo_oauth_client_id", clientID, "Linux DO OAuth Client ID", "linuxdo_oauth", false); err != nil {
			return err
		}
	}

	if clientSecret != "" {
		if err := models.UpsertConfig(db, "linuxdo_oauth_client_secret", clientSecret, "Linux DO OAuth Client Secret", "linuxdo_oauth", true); err != nil {
			return err
		}
	}

	return nil
}

// GetLinuxDoCreditConfig 获取 Linux DO Credit 配置（优先数据库）
func (s *SystemConfigService) GetLinuxDoCreditConfig(db *gorm.DB) map[string]interface{} {
	pid := models.GetConfigValue(db, "linuxdo_credit_pid", config.AppConfig.LinuxDoCreditPID)
	key := models.GetConfigValue(db, "linuxdo_credit_key", config.AppConfig.LinuxDoCreditKey)

	return map[string]interface{}{
		"pid":        pid,
		"pid_stored": pid != "" && pid != config.AppConfig.LinuxDoCreditPID,
		"key_set":    key != "",
		"key_stored": key != "" && key != config.AppConfig.LinuxDoCreditKey,
	}
}

// UpdateLinuxDoCreditConfig 更新 Linux DO Credit 配置
func (s *SystemConfigService) UpdateLinuxDoCreditConfig(db *gorm.DB, pid, key string) error {
	if pid != "" {
		if err := models.UpsertConfig(db, "linuxdo_credit_pid", pid, "Linux DO Credit PID", "linuxdo_credit", false); err != nil {
			return err
		}
	}

	if key != "" {
		if err := models.UpsertConfig(db, "linuxdo_credit_key", key, "Linux DO Credit Key", "linuxdo_credit", true); err != nil {
			return err
		}
	}

	return nil
}

// GetAllConfigs 获取所有配置（用于导出/备份）
func (s *SystemConfigService) GetAllConfigs(db *gorm.DB, category string) ([]models.SystemConfig, error) {
	var configs []models.SystemConfig
	query := db.Order("category, key")

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Find(&configs).Error; err != nil {
		return nil, err
	}

	// 脱敏处理
	for i := range configs {
		if configs[i].IsSecret && configs[i].Value != "" {
			configs[i].Value = "********"
		}
	}

	return configs, nil
}

// DeleteConfig 删除配置项
func (s *SystemConfigService) DeleteConfig(db *gorm.DB, key string) error {
	return db.Where("key = ?", key).Delete(&models.SystemConfig{}).Error
}

// GetConfigByKey 获取单个配置值
func (s *SystemConfigService) GetConfigByKey(db *gorm.DB, key string) (string, error) {
	var config models.SystemConfig
	if err := db.Where("key = ?", key).First(&config).Error; err != nil {
		return "", err
	}
	return config.Value, nil
}

