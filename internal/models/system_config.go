package models

import (
	"time"

	"gorm.io/gorm"
)

// SystemConfig 系统配置模型（Key-Value 结构）
type SystemConfig struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Key       string         `gorm:"uniqueIndex;not null" json:"key"`         // 配置键名
	Value     string         `gorm:"type:text" json:"value"`                  // 配置值
	Label     string         `json:"label"`                                   // 配置项标签（用于显示）
	Category  string         `gorm:"index" json:"category"`                   // 配置分类（linuxdo_oauth, linuxdo_credit, smtp 等）
	IsSecret  bool           `gorm:"default:false" json:"is_secret"`          // 是否为敏感信息（前端不显示完整内容）
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName 指定表名
func (SystemConfig) TableName() string {
	return "system_configs"
}

// BeforeCreate GORM 钩子：创建前
func (c *SystemConfig) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

// BeforeUpdate GORM 钩子：更新前
func (c *SystemConfig) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// GetConfigValue 获取配置值（优先数据库，其次环境变量）
func GetConfigValue(db *gorm.DB, key, envValue string) string {
	var config SystemConfig
	if err := db.Where("key = ?", key).First(&config).Error; err == nil && config.Value != "" {
		return config.Value
	}
	return envValue
}

// UpsertConfig 创建或更新配置
func UpsertConfig(db *gorm.DB, key, value, label, category string, isSecret bool) error {
	var config SystemConfig
	result := db.Where("key = ?", key).First(&config)

	if result.Error != nil {
		// 创建新配置
		config = SystemConfig{
			Key:      key,
			Value:    value,
			Label:    label,
			Category: category,
			IsSecret: isSecret,
		}
		return db.Create(&config).Error
	}

	// 更新现有配置
	updates := map[string]interface{}{
		"value":     value,
		"label":     label,
		"category":  category,
		"is_secret": isSecret,
	}
	return db.Model(&config).Updates(updates).Error
}

