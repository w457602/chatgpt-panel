package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
)

type AccountHandler struct {
	accountService     *services.AccountService
	accountTestService *services.AccountTestService
}

func NewAccountHandler() *AccountHandler {
	return &AccountHandler{
		accountService:     services.NewAccountService(),
		accountTestService: services.NewAccountTestService(),
	}
}

func (h *AccountHandler) List(c *gin.Context) {
	var filter models.AccountFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.accountService.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *AccountHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	account, err := h.accountService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	c.JSON(http.StatusOK, account)
}

func (h *AccountHandler) Create(c *gin.Context) {
	var req models.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := &models.Account{
		Email:          req.Email,
		Password:       req.Password,
		AccessToken:    req.AccessToken,
		RefreshToken:   req.RefreshToken,
		CheckoutURL:    req.CheckoutURL,
		AccountID:      req.AccountID,
		SessionCookies: req.SessionCookies,
		Status:         req.Status,
		Name:           req.Name,
	}

	if err := h.accountService.Create(account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, account)
}

func (h *AccountHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	account, err := h.accountService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	var req models.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account.Email = req.Email
	account.Password = req.Password
	account.AccessToken = req.AccessToken
	account.RefreshToken = req.RefreshToken
	account.CheckoutURL = req.CheckoutURL
	account.AccountID = req.AccountID
	account.SessionCookies = req.SessionCookies
	account.Status = req.Status
	account.Name = req.Name

	if err := h.accountService.Update(account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, account)
}

func (h *AccountHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := h.accountService.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}

func (h *AccountHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.accountService.BatchDelete(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Accounts deleted", "count": len(req.IDs)})
}

func (h *AccountHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.accountService.UpdateStatus(uint(id), req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

func (h *AccountHandler) BatchUpdateStatus(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.accountService.BatchUpdateStatus(req.IDs, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated", "count": len(req.IDs)})
}

func (h *AccountHandler) UpdateRefreshToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req models.UpdateRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.accountService.UpdateRefreshToken(uint(id), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Refresh token updated"})
}

func (h *AccountHandler) GetStats(c *gin.Context) {
	stats, err := h.accountService.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *AccountHandler) Import(c *gin.Context) {
	var req models.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := &models.Account{
		Email:          req.Email,
		Password:       req.Password,
		AccessToken:    req.AccessToken,
		RefreshToken:   req.RefreshToken,
		CheckoutURL:    req.CheckoutURL,
		AccountID:      req.AccountID,
		SessionCookies: req.SessionCookies,
		Status:         req.Status,
		Name:           req.Name,
	}

	if err := h.accountService.CreateOrUpdate(account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account imported", "id": account.ID})
}

// TestAccount 测试单个账号
func (h *AccountHandler) TestAccount(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	result, err := h.accountTestService.TestAccount(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// BatchTestAccounts 批量测试账号
func (h *AccountHandler) BatchTestAccounts(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	taskID, err := h.accountTestService.BatchTestAccounts(c.Request.Context(), req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"message": "批量测试已开始",
	})
}

// GetBatchTestResult 获取批量测试结果
func (h *AccountHandler) GetBatchTestResult(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	result, err := h.accountTestService.GetBatchTestResult(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// RefreshToken 刷新单个账号的 Token
func (h *AccountHandler) RefreshAccountToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	account, err := h.accountService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	if account.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号无 refresh_token"})
		return
	}

	result, err := h.accountTestService.RefreshAccessToken(c.Request.Context(), account.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 更新数据库
	updates := map[string]interface{}{
		"access_token": result.AccessToken,
		"status":       "active",
	}
	if result.RefreshToken != "" {
		updates["refresh_token"] = result.RefreshToken
	}
	h.accountService.Update(account)

	c.JSON(http.StatusOK, gin.H{
		"message": "Token 刷新成功",
		"access_token": result.AccessToken[:50] + "...",
	})
}

