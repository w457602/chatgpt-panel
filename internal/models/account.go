package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// Account ChatGPT账号模型
type Account struct {
	ID                 uint            `gorm:"primaryKey" json:"id"`
	Email              string          `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Password           string          `gorm:"size:255;not null" json:"password"`
	AccessToken        string          `gorm:"type:text" json:"access_token"`
	RefreshToken       string          `gorm:"type:text" json:"refresh_token"`
	CheckoutURL        string          `gorm:"size:1000" json:"checkout_url"`
	TeamCheckoutURL    string          `gorm:"size:1000" json:"team_checkout_url"`
	PlusBound          bool            `gorm:"default:false" json:"plus_bound"`  // Plus绑卡是否成功
	TeamBound          bool            `gorm:"default:false" json:"team_bound"`  // Team绑卡是否成功
	AccountID          string          `gorm:"index;size:100" json:"account_id"`
	SubscriptionStatus string          `gorm:"index;size:50" json:"subscription_status"`
	TokenExpired       *time.Time      `json:"token_expired"`
	CliproxySynced     bool            `gorm:"default:false" json:"cliproxy_synced"`
	CliproxySyncedAt   *time.Time      `json:"cliproxy_synced_at"`
	SessionCookies     json.RawMessage `gorm:"type:jsonb" json:"session_cookies"`
	Status             string          `gorm:"index;size:50;default:pending" json:"status"` // pending, active, bound, failed, expired
	Name               string          `gorm:"size:100" json:"name"`
	Notes              string          `gorm:"type:text" json:"notes"`
	RegisteredAt       time.Time       `gorm:"index" json:"registered_at"`
	LastUsedAt         *time.Time      `json:"last_used_at"`
	IsDemoted          bool            `gorm:"default:false" json:"is_demoted"` // 账号是否已降级
	DemotedAt          *time.Time      `json:"demoted_at"`                      // 降级时间
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	DeletedAt          gorm.DeletedAt  `gorm:"index" json:"-"`
}

func (Account) TableName() string {
	return "accounts"
}

// AccountFilter 查询过滤器
type AccountFilter struct {
	Search   string `form:"search"`
	Status   string `form:"status"`
	Domain   string `form:"domain"`
	HasRT    string `form:"has_rt"` // 是否有 refresh_token: yes/no
	CliproxySynced string `form:"cliproxy_synced"` // 是否已同步: yes/no
	DateFrom string `form:"date_from"`
	DateTo   string `form:"date_to"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	SortBy   string `form:"sort_by,default=created_at"`
	SortDir  string `form:"sort_dir,default=desc"`
}

// AccountStats 统计信息
type AccountStats struct {
	Total               int64            `json:"total"`
	Pending             int64            `json:"pending"`
	Active              int64            `json:"active"`
	Bound               int64            `json:"bound"`
	Failed              int64            `json:"failed"`
	Expired             int64            `json:"expired"`
	Banned              int64            `json:"banned"`
	RateLimited         int64            `json:"rate_limited"`
	TodayCount          int64            `json:"today_count"`
	WeekCount           int64            `json:"week_count"`
	MonthCount          int64            `json:"month_count"`
	ByDomain            map[string]int64 `json:"by_domain"`
	ByDate              []DateCount      `json:"by_date"`
	WithToken           int64            `json:"with_token"`
	WithRefreshToken    int64            `json:"with_refresh_token"`
	WithCheckoutURL     int64            `json:"with_checkout_url"`
	WithTeamCheckoutURL int64            `json:"with_team_checkout_url"`
	CliproxySynced      int64            `json:"cliproxy_synced"`
}

// DateCount 按日期统计
type DateCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// PaginatedResult 分页结果
type PaginatedResult struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	Email              string          `json:"email" binding:"required,email"`
	Password           string          `json:"password" binding:"required"`
	AccessToken        string          `json:"access_token"`
	RefreshToken       string          `json:"refresh_token"`
	CheckoutURL        string          `json:"checkout_url"`
	TeamCheckoutURL    string          `json:"team_checkout_url"`
	AccountID          string          `json:"account_id"`
	SubscriptionStatus string          `json:"subscription_status"`
	SessionCookies     json.RawMessage `json:"session_cookies"`
	Status             string          `json:"status"`
	Name               string          `json:"name"`
}

// UpdateRefreshTokenRequest 更新 RT 请求
type UpdateRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
