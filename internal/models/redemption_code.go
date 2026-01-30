package models

import (
	"time"

	"gorm.io/gorm"
)

// RedemptionCode 兑换码模型
type RedemptionCode struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	Code                  string         `gorm:"uniqueIndex;size:64;not null" json:"code"`
	IsRedeemed            bool           `gorm:"default:false" json:"is_redeemed"`
	RedeemedAt            *time.Time     `json:"redeemed_at"`
	RedeemedBy            string         `gorm:"size:500" json:"redeemed_by"` // 可能包含 UID 和邮箱
	AccountEmail          string         `gorm:"size:255" json:"account_email"`
	AccountID             *uint          `gorm:"index" json:"account_id,omitempty"` // 关联 gpt_accounts 表
	Channel               string         `gorm:"size:50;default:'common'" json:"channel"` // common, linux-do, xhs, xianyu
	ChannelName           string         `gorm:"size:100" json:"channel_name"`
	OrderType             string         `gorm:"size:50;default:'warranty'" json:"order_type"` // warranty, no-warranty
	ReservedForUID        string         `gorm:"size:100" json:"reserved_for_uid"`
	ReservedForUsername   string         `gorm:"size:255" json:"reserved_for_username"`
	ReservedForEntryID    *uint          `json:"reserved_for_entry_id"`
	ReservedAt            *time.Time     `json:"reserved_at"`
	ReservedForOrderNo    string         `gorm:"size:255" json:"reserved_for_order_no"`
	ReservedForOrderEmail string         `gorm:"size:255" json:"reserved_for_order_email"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (RedemptionCode) TableName() string {
	return "redemption_codes"
}

// GetChannelName 根据渠道代码获取中文名称
func GetChannelName(channel string) string {
	channelNames := map[string]string{
		"common":   "通用",
		"linux-do": "Linux DO",
		"xhs":      "小红书",
		"xianyu":   "闲鱼",
	}
	if name, ok := channelNames[channel]; ok {
		return name
	}
	return "通用"
}

// IsValidChannel 检查渠道是否有效
func IsValidChannel(channel string) bool {
	validChannels := map[string]bool{
		"common":   true,
		"linux-do": true,
		"xhs":      true,
		"xianyu":   true,
	}
	return validChannels[channel]
}

// IsValidOrderType 检查订单类型是否有效
func IsValidOrderType(orderType string) bool {
	validOrderTypes := map[string]bool{
		"warranty":    true,
		"no-warranty": true,
	}
	return validOrderTypes[orderType]
}

