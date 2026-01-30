package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/webauto/chatgpt-panel/internal/models"
	"gorm.io/gorm"
)

var (
	emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	codeRegex  = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`)
)

type RedemptionService struct {
	db            *gorm.DB
	inviteService *InviteService
}

func NewRedemptionService(db *gorm.DB) *RedemptionService {
	return &RedemptionService{
		db:            db,
		inviteService: NewInviteService(),
	}
}

type RedemptionRequest struct {
	Email                    string `json:"email"`
	Code                     string `json:"code"`
	Channel                  string `json:"channel"`
	OrderType                string `json:"order_type"`
	RedeemerUid              string `json:"redeemer_uid"`
	SkipCodeFormatValidation bool   `json:"skip_code_format_validation"`
}

type RedemptionResult struct {
	Success        bool        `json:"success"`
	Message        string      `json:"message"`
	AccountEmail   string      `json:"account_email"`
	InviteID       string      `json:"invite_id"`
	StatusCode     int         `json:"status_code"`
	RedemptionCode interface{} `json:"redemption_code"`
}

// RedeemCode 兑换兑换码
func (s *RedemptionService) RedeemCode(ctx context.Context, req *RedemptionRequest) (*RedemptionResult, error) {
	// 1. 验证邮箱
	normalizedEmail := strings.TrimSpace(req.Email)
	if normalizedEmail == "" {
		return nil, fmt.Errorf("请输入邮箱地址")
	}
	if !emailRegex.MatchString(normalizedEmail) {
		return nil, fmt.Errorf("请输入有效的邮箱地址")
	}

	// 2. 验证兑换码
	sanitizedCode := strings.ToUpper(strings.TrimSpace(req.Code))
	if sanitizedCode == "" {
		return nil, fmt.Errorf("请输入兑换码")
	}
	if !req.SkipCodeFormatValidation && !codeRegex.MatchString(sanitizedCode) {
		return nil, fmt.Errorf("兑换码格式不正确（格式：XXXX-XXXX-XXXX）")
	}

	// 3. 规范化渠道
	channel := normalizeChannel(req.Channel)
	if !models.IsValidChannel(channel) {
		return nil, fmt.Errorf("不支持的渠道类型")
	}

	// 4. Linux DO 渠道需要 UID
	normalizedRedeemerUid := strings.TrimSpace(req.RedeemerUid)
	if channel == "linux-do" && normalizedRedeemerUid == "" {
		return nil, fmt.Errorf("Linux DO 渠道兑换需要填写论坛 UID")
	}

	// 5. 查询兑换码
	var code models.RedemptionCode
	if err := s.db.Where("code = ?", sanitizedCode).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("兑换码不存在或已失效")
		}
		return nil, err
	}

	// 6. 检查兑换码状态
	if code.IsRedeemed {
		return nil, fmt.Errorf("该兑换码已被使用")
	}

	// 7. 检查渠道匹配
	if code.Channel != channel {
		return nil, fmt.Errorf("该兑换码仅能在对应渠道的兑换页使用")
	}

	// 8. 规范化订单类型
	orderType := normalizeOrderType(req.OrderType)
	if orderType == "" {
		orderType = code.OrderType
	}
	if orderType == "" {
		orderType = "warranty"
	}

	// 9. 根据订单类型选择账号
	mustUseUndemotedAccount := orderType == "warranty"
	mustUseDemotedAccount := orderType == "no-warranty"

	var account models.Account
	query := s.db.Model(&models.Account{}).Where("access_token != ?", "")

	if mustUseUndemotedAccount {
		query = query.Where("is_demoted = ?", false)
	}
	if mustUseDemotedAccount {
		query = query.Where("is_demoted = ?", true)
	}

	// 如果兑换码已绑定账号，使用该账号
	if code.AccountID != nil {
		query = query.Where("id = ?", *code.AccountID)
	}

	if err := query.First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if mustUseDemotedAccount {
				return nil, fmt.Errorf("暂无可用已降级账号，请稍后再试或联系管理员")
			}
			return nil, fmt.Errorf("暂无可用账号，请稍后再试或联系管理员")
		}
		return nil, err
	}

	// 10. 检查账号降级状态
	if mustUseUndemotedAccount && account.IsDemoted {
		return nil, fmt.Errorf("该兑换码绑定的账号已降级，暂不可用，请联系管理员")
	}
	if mustUseDemotedAccount && !account.IsDemoted {
		return nil, fmt.Errorf("无质保订单需要使用已降级账号的兑换码，请联系管理员")
	}

	// 11. 发送 Team 邀请
	inviteResult, err := s.inviteService.SendInvite(ctx, account.AccessToken, account.AccountID, normalizedEmail)
	if err != nil {
		return nil, fmt.Errorf("邀请失败: %v", err)
	}

	if !inviteResult.Success {
		return nil, fmt.Errorf("邀请失败: %s", inviteResult.Message)
	}

	// 12. 更新兑换码状态
	redeemerIdentifier := normalizedEmail
	if channel == "linux-do" && normalizedRedeemerUid != "" {
		redeemerIdentifier = fmt.Sprintf("UID:%s | Email:%s", normalizedRedeemerUid, normalizedEmail)
	}

	now := time.Now()
	code.IsRedeemed = true
	code.RedeemedAt = &now
	code.RedeemedBy = redeemerIdentifier
	code.OrderType = orderType
	code.AccountEmail = account.Email
	code.UpdatedAt = now

	if err := s.db.Save(&code).Error; err != nil {
		return nil, fmt.Errorf("更新兑换码状态失败: %v", err)
	}

	return &RedemptionResult{
		Success:        true,
		Message:        "兑换成功",
		AccountEmail:   account.Email,
		InviteID:       inviteResult.InviteID,
		StatusCode:     http.StatusOK,
		RedemptionCode: code,
	}, nil
}

// normalizeChannel 规范化渠道名称
func normalizeChannel(channel string) string {
	normalized := strings.ToLower(strings.TrimSpace(channel))
	switch normalized {
	case "linux-do", "linuxdo":
		return "linux-do"
	case "xhs", "xiaohongshu":
		return "xhs"
	case "xianyu":
		return "xianyu"
	default:
		return "common"
	}
}

// normalizeOrderType 规范化订单类型
func normalizeOrderType(orderType string) string {
	normalized := strings.ToLower(strings.TrimSpace(orderType))
	switch normalized {
	case "warranty":
		return "warranty"
	case "no-warranty", "no_warranty", "nowarranty":
		return "no-warranty"
	default:
		return ""
	}
}

// GenerateRedemptionCode 生成兑换码（格式：XXXX-XXXX-XXXX）
func GenerateRedemptionCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 排除易混淆字符
	code := make([]byte, 12)
	for i := 0; i < 12; i++ {
		code[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	return fmt.Sprintf("%s-%s-%s", string(code[0:4]), string(code[4:8]), string(code[8:12]))
}

// BatchCreateRedemptionCodes 批量创建兑换码
func (s *RedemptionService) BatchCreateRedemptionCodes(count int, channel, orderType string, accountID *uint) ([]*models.RedemptionCode, error) {
	if count <= 0 || count > 100 {
		return nil, fmt.Errorf("批量创建数量必须在 1-100 之间")
	}

	channel = normalizeChannel(channel)
	if !models.IsValidChannel(channel) {
		return nil, fmt.Errorf("不支持的渠道类型")
	}

	if orderType == "" {
		orderType = "warranty"
	}
	orderType = normalizeOrderType(orderType)
	if !models.IsValidOrderType(orderType) {
		return nil, fmt.Errorf("不支持的订单类型")
	}

	codes := make([]*models.RedemptionCode, 0, count)
	for i := 0; i < count; i++ {
		code := &models.RedemptionCode{
			Code:        GenerateRedemptionCode(),
			Channel:     channel,
			ChannelName: models.GetChannelName(channel),
			OrderType:   orderType,
			AccountID:   accountID,
		}
		if err := s.db.Create(code).Error; err != nil {
			return nil, fmt.Errorf("创建兑换码失败: %v", err)
		}
		codes = append(codes, code)
	}

	return codes, nil
}

