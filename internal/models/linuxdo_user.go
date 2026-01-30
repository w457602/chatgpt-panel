package models

import (
	"time"

	"gorm.io/gorm"
)

// LinuxDoUser Linux DO 用户模型
type LinuxDoUser struct {
	ID                   uint           `gorm:"primarykey" json:"id"`
	UID                  string         `gorm:"uniqueIndex;not null" json:"uid"`                    // Linux DO 用户 ID
	Username             string         `gorm:"index" json:"username"`                              // Linux DO 用户名
	Name                 string         `json:"name"`                                               // 显示名称
	Email                string         `gorm:"index" json:"email"`                                 // 邮箱
	TrustLevel           int            `json:"trust_level"`                                        // 信任等级
	CurrentOpenAccountID *uint          `json:"current_open_account_id"`                            // 当前关联的开放账号 ID
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName 指定表名
func (LinuxDoUser) TableName() string {
	return "linuxdo_users"
}

// BeforeCreate GORM 钩子：创建前
func (u *LinuxDoUser) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	return nil
}

// BeforeUpdate GORM 钩子：更新前
func (u *LinuxDoUser) BeforeUpdate(tx *gorm.DB) error {
	u.UpdatedAt = time.Now()
	return nil
}

