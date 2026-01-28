package handlers

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
	"gorm.io/gorm"
)

type InviteHandler struct {
	db                 *gorm.DB
	accountService     *services.AccountService
	accountTestService *services.AccountTestService
	inviteService      *services.InviteService
}

func NewInviteHandler() *InviteHandler {
	return &InviteHandler{
		db:                 models.GetDB(),
		accountService:     services.NewAccountService(),
		accountTestService: services.NewAccountTestService(),
		inviteService:      services.NewInviteService(),
	}
}

type joinRequest struct {
	Code  string `json:"code" form:"code"`
	Email string `json:"email" form:"email"`
}

type inviteAttemptResult struct {
	Success       bool
	Message       string
	StatusCode    int
	RetryWithNext bool
	InviteID      string
	ConsumeCode   bool
}

// JoinByInviteCode 用户提交邀请码加入 Team
func (h *InviteHandler) JoinByInviteCode(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprint(r)})
		}
	}()
	c.Header("X-Invite-Handler", "1")

	var req joinRequest
	if err := c.ShouldBind(&req); err != nil {
		// fallback: form/query
		req.Code = strings.TrimSpace(c.PostForm("code"))
		req.Email = strings.TrimSpace(c.PostForm("email"))
		if req.Code == "" || req.Email == "" {
			req.Code = strings.TrimSpace(c.Query("code"))
			req.Email = strings.TrimSpace(c.Query("email"))
		}
		if req.Code == "" || req.Email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "detail": err.Error()})
			return
		}
	}

	code := strings.ToUpper(strings.TrimSpace(req.Code))
	email := strings.TrimSpace(req.Email)
	if code == "" || email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码和邮箱不能为空"})
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱格式不正确"})
		return
	}

	var inviteCode models.InviteCode
	if err := h.db.Where("code = ?", code).First(&inviteCode).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "邀请码无效"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if inviteCode.Disabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码已禁用"})
		return
	}
	if inviteCode.ExpiresAt != nil && time.Now().After(*inviteCode.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码已过期"})
		return
	}
	if inviteCode.MaxUses > 0 && inviteCode.UsedCount >= inviteCode.MaxUses {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码已用完"})
		return
	}

	var candidates []models.Account
	if inviteCode.AccountID > 0 {
		account, err := h.accountService.GetByID(inviteCode.AccountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "邀请码对应账号不存在"})
			return
		}
		candidates = append(candidates, *account)
	} else {
		if err := h.db.Model(&models.Account{}).
			Where("(access_token IS NOT NULL AND access_token != '') OR (refresh_token IS NOT NULL AND refresh_token != '')").
			Where("status NOT IN ?", []string{"banned", "expired"}).
			Order("updated_at desc").
			Limit(20).
			Find(&candidates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "无法获取可用账号"})
			return
		}
		if len(candidates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "暂无可用账号"})
			return
		}
	}

	var lastErr *inviteAttemptResult
	var successAttempt *inviteAttemptResult
	for idx := range candidates {
		account := candidates[idx]
		attempt, err := h.attemptInvite(c, &account, email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "邀请请求失败"})
			return
		}
		if attempt.Success {
			successAttempt = attempt
			lastErr = nil
			break
		}
		lastErr = attempt
		if !attempt.RetryWithNext {
			break
		}
	}
	if lastErr != nil {
		if lastErr.StatusCode != 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": lastErr.Message, "status_code": lastErr.StatusCode})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": lastErr.Message})
		}
		return
	}

	if successAttempt != nil && successAttempt.ConsumeCode {
		now := time.Now()
		updateQuery := h.db.Model(&models.InviteCode{}).Where("id = ?", inviteCode.ID)
		if inviteCode.MaxUses > 0 {
			updateQuery = updateQuery.Where("used_count < max_uses")
		}
		resultUpdate := updateQuery.Updates(map[string]interface{}{
			"used_count":  gorm.Expr("used_count + 1"),
			"last_used_at": &now,
		})
		if inviteCode.MaxUses > 0 && resultUpdate.RowsAffected == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码已用完"})
			return
		}
	}

	message := "邀请已发送，请查收邮件"
	if successAttempt != nil && successAttempt.Message != "" {
		message = successAttempt.Message
	}
	resp := gin.H{"message": message}
	if successAttempt != nil && successAttempt.InviteID != "" {
		resp["invite_id"] = successAttempt.InviteID
	}
	if successAttempt != nil && successAttempt.StatusCode != 0 {
		resp["status_code"] = successAttempt.StatusCode
	}
	c.JSON(http.StatusOK, resp)
}

func (h *InviteHandler) attemptInvite(c *gin.Context, account *models.Account, email string) (*inviteAttemptResult, error) {
	accessToken := strings.TrimSpace(account.AccessToken)
	if accessToken == "" && strings.TrimSpace(account.RefreshToken) != "" {
		refreshed, err := h.accountTestService.RefreshAccessToken(c.Request.Context(), account.RefreshToken)
		if err != nil {
			return &inviteAttemptResult{Success: false, Message: "账号无有效 access_token，且刷新失败", RetryWithNext: true}, nil
		}
		account.AccessToken = refreshed.AccessToken
		if refreshed.RefreshToken != "" {
			account.RefreshToken = refreshed.RefreshToken
		}
		_ = h.accountService.Update(account)
		accessToken = account.AccessToken
	}
	if accessToken == "" {
		return &inviteAttemptResult{Success: false, Message: "账号无有效 access_token", RetryWithNext: true}, nil
	}

	accountID := strings.TrimSpace(account.AccountID)
	if accountID == "" {
		accountID = services.ExtractAccountIDFromToken(accessToken)
		if accountID != "" {
			account.AccountID = accountID
			_ = h.accountService.Update(account)
		}
	}
	if accountID == "" {
		return &inviteAttemptResult{Success: false, Message: "账号缺少 account_id", RetryWithNext: true}, nil
	}

	membersResult, err := h.inviteService.GetMembers(c.Request.Context(), accessToken, accountID)
	if err != nil {
		return &inviteAttemptResult{Success: false, Message: "获取成员列表失败", RetryWithNext: true}, nil
	}
	if membersResult.StatusCode == http.StatusUnauthorized && strings.TrimSpace(account.RefreshToken) != "" {
		refreshed, err := h.accountTestService.RefreshAccessToken(c.Request.Context(), account.RefreshToken)
		if err == nil && refreshed.AccessToken != "" {
			account.AccessToken = refreshed.AccessToken
			if refreshed.RefreshToken != "" {
				account.RefreshToken = refreshed.RefreshToken
			}
			_ = h.accountService.Update(account)
			accessToken = refreshed.AccessToken
			membersResult, err = h.inviteService.GetMembers(c.Request.Context(), accessToken, accountID)
			if err != nil {
				return &inviteAttemptResult{Success: false, Message: "获取成员列表失败", RetryWithNext: true}, nil
			}
		}
	}
	if !membersResult.Success {
		msg := membersResult.Message
		if msg == "" {
			msg = "无法获取成员列表"
		}
		return &inviteAttemptResult{Success: false, Message: msg, StatusCode: membersResult.StatusCode, RetryWithNext: true}, nil
	}

	nonOwnerCount := 0
	for _, m := range membersResult.Members {
		if strings.ToLower(strings.TrimSpace(m.Role)) != "account-owner" {
			nonOwnerCount++
		}
		if strings.EqualFold(strings.TrimSpace(m.Email), email) {
			return &inviteAttemptResult{Success: true, Message: "该邮箱已在团队中"}, nil
		}
	}

	pendingResult, err := h.inviteService.GetPendingInvites(c.Request.Context(), accessToken, accountID)
	if err != nil {
		return &inviteAttemptResult{Success: false, Message: "获取邀请列表失败", RetryWithNext: true}, nil
	}
	if pendingResult.StatusCode == http.StatusUnauthorized && strings.TrimSpace(account.RefreshToken) != "" {
		refreshed, err := h.accountTestService.RefreshAccessToken(c.Request.Context(), account.RefreshToken)
		if err == nil && refreshed.AccessToken != "" {
			account.AccessToken = refreshed.AccessToken
			if refreshed.RefreshToken != "" {
				account.RefreshToken = refreshed.RefreshToken
			}
			_ = h.accountService.Update(account)
			accessToken = refreshed.AccessToken
			pendingResult, err = h.inviteService.GetPendingInvites(c.Request.Context(), accessToken, accountID)
			if err != nil {
				return &inviteAttemptResult{Success: false, Message: "获取邀请列表失败", RetryWithNext: true}, nil
			}
		}
	}
	if pendingResult.Success {
		for _, inv := range pendingResult.Invites {
			if strings.EqualFold(strings.TrimSpace(inv.EmailAddress), email) {
				return &inviteAttemptResult{Success: true, Message: "该邮箱已发送邀请，请查收邮件"}, nil
			}
		}
	}

	if nonOwnerCount >= 4 {
		return &inviteAttemptResult{
			Success:       false,
			Message:       "Team 成员已满(4人)",
			StatusCode:    http.StatusConflict,
			RetryWithNext: true,
		}, nil
	}

	result, err := h.inviteService.SendInvite(c.Request.Context(), accessToken, accountID, email)
	if err != nil {
		return &inviteAttemptResult{Success: false, Message: "邀请请求失败", RetryWithNext: true}, nil
	}
	if result.StatusCode == http.StatusUnauthorized && strings.TrimSpace(account.RefreshToken) != "" {
		refreshed, err := h.accountTestService.RefreshAccessToken(c.Request.Context(), account.RefreshToken)
		if err == nil && refreshed.AccessToken != "" {
			account.AccessToken = refreshed.AccessToken
			if refreshed.RefreshToken != "" {
				account.RefreshToken = refreshed.RefreshToken
			}
			_ = h.accountService.Update(account)
			result, err = h.inviteService.SendInvite(c.Request.Context(), refreshed.AccessToken, accountID, email)
			if err != nil {
				return &inviteAttemptResult{Success: false, Message: "邀请请求失败", RetryWithNext: true}, nil
			}
		}
	}
	if !result.Success {
		membersResult, _ = h.inviteService.GetMembers(c.Request.Context(), accessToken, accountID)
		if membersResult != nil && membersResult.Success {
			for _, m := range membersResult.Members {
				if strings.EqualFold(strings.TrimSpace(m.Email), email) {
					return &inviteAttemptResult{Success: true, Message: "邀请已发送，请查收邮件"}, nil
				}
			}
		}
		pendingResult, _ = h.inviteService.GetPendingInvites(c.Request.Context(), accessToken, accountID)
		if pendingResult != nil && pendingResult.Success {
			for _, inv := range pendingResult.Invites {
				if strings.EqualFold(strings.TrimSpace(inv.EmailAddress), email) {
					return &inviteAttemptResult{Success: true, Message: "邀请已发送，请查收邮件"}, nil
				}
			}
		}

		msg := result.Message
		if msg == "" {
			msg = "邀请失败"
		}
		return &inviteAttemptResult{Success: false, Message: msg, StatusCode: result.StatusCode, RetryWithNext: true}, nil
	}

	return &inviteAttemptResult{
		Success:    true,
		Message:    "邀请已发送，请查收邮件",
		StatusCode: result.StatusCode,
		InviteID:   result.InviteID,
		ConsumeCode: true,
	}, nil
}

type createInviteCodeRequest struct {
	Code      string `json:"code"`
	AccountID uint   `json:"account_id" binding:"required"`
	MaxUses   int    `json:"max_uses"`
	ExpiresAt string `json:"expires_at"`
	Disabled  bool   `json:"disabled"`
}

// CreateInviteCode 创建邀请码（需登录）
func (h *InviteHandler) CreateInviteCode(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprint(r)})
		}
	}()

	var req createInviteCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.accountService.GetByID(req.AccountID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号不存在"})
		return
	}

	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		code = generateInviteCode(8)
	}

	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expires_at 需为 RFC3339 格式"})
			return
		}
		expiresAt = &parsed
	}

	inviteCode := &models.InviteCode{
		Code:      code,
		AccountID: req.AccountID,
		MaxUses:   req.MaxUses,
		Disabled:  req.Disabled,
		ExpiresAt: expiresAt,
	}

	if err := h.db.Create(inviteCode).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, inviteCode)
}

// ListInviteCodes 获取邀请码列表（需登录）
func (h *InviteHandler) ListInviteCodes(c *gin.Context) {
	var codes []models.InviteCode
	if err := h.db.Order("id desc").Find(&codes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": codes})
}

func generateInviteCode(length int) string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	if length < 4 {
		length = 4
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	code := make([]byte, length)
	for i := 0; i < length; i++ {
		code[i] = charset[rng.Intn(len(charset))]
	}
	return string(code)
}
