package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
	"gorm.io/gorm"
)

type ExtensionHandler struct {
	accountService *services.AccountService
}

func NewExtensionHandler() *ExtensionHandler {
	return &ExtensionHandler{
		accountService: services.NewAccountService(),
	}
}

func getURLFromRequest(c *gin.Context) string {
	url := strings.TrimSpace(c.Query("url"))
	if url != "" {
		return url
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err == nil {
		return strings.TrimSpace(body.URL)
	}
	return ""
}

func (h *ExtensionHandler) LookupAccountByURL(c *gin.Context) {
	url := getURLFromRequest(c)
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	account, err := h.accountService.GetByCheckoutURL(url)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                account.ID,
		"email":             account.Email,
		"status":            account.Status,
		"checkout_url":      account.CheckoutURL,
		"team_checkout_url": account.TeamCheckoutURL,
	})
}

func (h *ExtensionHandler) BillingSuccess(c *gin.Context) {
	var req struct {
		URL       string `json:"url"`
		AccountID uint   `json:"account_id"`
		Status    string `json:"status"`
		Type      string `json:"type"` // "plus" or "team"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		account *models.Account
		err     error
	)
	url := strings.TrimSpace(req.URL)
	if req.AccountID > 0 {
		account, err = h.accountService.GetByID(req.AccountID)
	} else {
		if url == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url or account_id is required"})
			return
		}
		account, err = h.accountService.GetByCheckoutURL(url)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 判断绑卡类型：通过 type 参数或 URL 匹配
	bindType := strings.ToLower(strings.TrimSpace(req.Type))
	if bindType == "" && url != "" {
		// 根据 URL 判断是 Plus 还是 Team
		if account.TeamCheckoutURL != "" && strings.HasPrefix(url, strings.TrimSuffix(account.TeamCheckoutURL, "?")[:50]) {
			bindType = "team"
		} else {
			bindType = "plus"
		}
	}
	if bindType == "" {
		bindType = "plus" // 默认 Plus
	}

	// 更新对应的绑卡状态
	if bindType == "team" {
		if err := h.accountService.SetTeamBound(account.ID, true); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := h.accountService.SetPlusBound(account.ID, true); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// 更新状态
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "bound"
	}
	subscriptionStatus := bindType
	if err := h.accountService.UpdateStatusAndSubscription(account.ID, status, subscriptionStatus); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":             "Status updated",
		"id":                  account.ID,
		"email":               account.Email,
		"status":              status,
		"subscription_status": subscriptionStatus,
		"bind_type":           bindType,
	})
}
