package models

import (
	"time"

	"gorm.io/gorm"
)

// CreditOrder Linux DO Credit 订单模型
type CreditOrder struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	OrderNo       string         `gorm:"uniqueIndex;not null" json:"order_no"`          // 订单号
	LinuxDoUID    string         `gorm:"index" json:"linuxdo_uid"`                      // Linux DO 用户 ID
	Email         string         `gorm:"index" json:"email"`                            // 邮箱
	Amount        string         `json:"amount"`                                        // 金额
	ProductName   string         `json:"product_name"`                                  // 商品名称
	Status        string         `gorm:"index;default:'pending'" json:"status"`         // pending, paid, failed, expired, refunded
	PayURL        string         `json:"pay_url"`                                       // 支付链接
	PayingOrderNo string         `json:"paying_order_no"`                               // 支付平台订单号
	TradeNo       string         `json:"trade_no"`                                      // 支付流水号
	PaidAt        *time.Time     `json:"paid_at"`                                       // 支付时间
	ExpiredAt     *time.Time     `json:"expired_at"`                                    // 过期时间
	RefundedAt    *time.Time     `json:"refunded_at"`                                   // 退款时间
	RefundMessage string         `json:"refund_message"`                                // 退款原因
	NotifyPayload string         `gorm:"type:text" json:"notify_payload"`               // 回调数据
	NotifyAt      *time.Time     `json:"notify_at"`                                     // 回调时间
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName 指定表名
func (CreditOrder) TableName() string {
	return "credit_orders"
}

// BeforeCreate GORM 钩子：创建前
func (o *CreditOrder) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	o.CreatedAt = now
	o.UpdatedAt = now
	if o.Status == "" {
		o.Status = "pending"
	}
	return nil
}

// BeforeUpdate GORM 钩子：更新前
func (o *CreditOrder) BeforeUpdate(tx *gorm.DB) error {
	o.UpdatedAt = time.Now()
	return nil
}

