package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/services"
)

type OAuthHandler struct {
	codex *services.CodexOAuthService
}

func NewOAuthHandler() *OAuthHandler {
	return &OAuthHandler{codex: services.GetCodexOAuthService()}
}

type codexStartRequest struct {
	AccountID uint `json:"account_id" binding:"required"`
}

func (h *OAuthHandler) StartCodex(c *gin.Context) {
	var req codexStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	url, state, err := h.codex.Start(req.AccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":   url,
		"state": state,
	})
}

// GetStatus 获取 OAuth 授权状态
func (h *OAuthHandler) GetStatus(c *gin.Context) {
	state := c.Param("state")
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "state is required"})
		return
	}

	result := h.codex.GetStatus(state)
	c.JSON(http.StatusOK, result)
}

// SubmitCallback 手动提交回调 URL
func (h *OAuthHandler) SubmitCallback(c *gin.Context) {
	var req struct {
		CallbackURL string `json:"callback_url" binding:"required"`
		State       string `json:"state" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.codex.ProcessCallbackURL(req.State, req.CallbackURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
