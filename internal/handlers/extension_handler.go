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
		"id":           account.ID,
		"email":        account.Email,
		"status":       account.Status,
		"checkout_url": account.CheckoutURL,
	})
}

func (h *ExtensionHandler) BillingSuccess(c *gin.Context) {
	var req struct {
		URL       string `json:"url"`
		AccountID uint   `json:"account_id"`
		Status    string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		account *models.Account
		err     error
	)
	if req.AccountID > 0 {
		account, err = h.accountService.GetByID(req.AccountID)
	} else {
		url := strings.TrimSpace(req.URL)
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

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "bound"
	}
	if err := h.accountService.UpdateStatusAndSubscription(account.ID, status, "team"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":             "Status updated",
		"id":                  account.ID,
		"email":               account.Email,
		"status":              status,
		"subscription_status": "team",
	})
}
