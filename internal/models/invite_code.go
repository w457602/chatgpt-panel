package models

import "time"

// InviteCode 邀请码模型
type InviteCode struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Code       string     `gorm:"uniqueIndex;size:64;not null" json:"code"`
	AccountID  *uint      `gorm:"index" json:"account_id,omitempty"`
	MaxUses    int        `gorm:"default:0" json:"max_uses"`    // 0 表示不限次数
	UsedCount  int        `gorm:"default:0" json:"used_count"`
	Disabled   bool       `gorm:"default:false" json:"disabled"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (InviteCode) TableName() string {
	return "invite_codes"
}
