package models

import (
	"log"

	"github.com/webauto/chatgpt-panel/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	var err error

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	if config.AppConfig.GinMode == "release" {
		gormConfig.Logger = logger.Default.LogMode(logger.Error)
	}

	DB, err = gorm.Open(postgres.Open(config.AppConfig.GetDSN()), gormConfig)
	if err != nil {
		return err
	}

	// 自动迁移
	if err := DB.AutoMigrate(&Account{}, &User{}, &InviteCode{}); err != nil {
		return err
	}
	// 允许 invite_codes.account_id 为空（兼容已存在的表结构）
	if err := DB.Exec("ALTER TABLE invite_codes ALTER COLUMN account_id DROP NOT NULL").Error; err != nil {
		log.Printf("ℹ️ invite_codes.account_id 可能已允许为空或表不存在: %v", err)
	}

	// 创建默认管理员账号
	createDefaultAdmin()

	log.Println("✅ Database connected and migrated successfully")
	return nil
}

func createDefaultAdmin() {
	var count int64
	DB.Model(&User{}).Count(&count)
	if count == 0 {
		admin := &User{
			Username: "admin",
			Email:    "admin@chatgpt-panel.local",
			Role:     "admin",
			Status:   "active",
		}
		admin.SetPassword("admin123")
		DB.Create(admin)
		log.Println("✅ Default admin user created (admin/admin123)")
	}
}

func GetDB() *gorm.DB {
	return DB
}
