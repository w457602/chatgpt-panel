package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
	"gorm.io/gorm"
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

	if accounts, ok := result.Data.([]models.Account); ok && len(accounts) > 0 {
		cliproxy := services.GetCliproxySyncService()
		if cliproxy.Enabled() {
			if remote, err := cliproxy.RemoteAuthFileNames(c.Request.Context()); err == nil {
				for i := range accounts {
					accounts[i].CliproxySynced = cliproxy.RemoteHasAccount(remote, &accounts[i])
					if !accounts[i].CliproxySynced {
						accounts[i].CliproxySyncedAt = nil
					}
				}
				result.Data = accounts
			}
		}
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
	planType := ""
	if account.AccessToken != "" {
		planType = services.ExtractSubscriptionStatusFromToken(account.AccessToken)
	}
	if planType == "" && req.SubscriptionStatus != "" {
		planType = services.NormalizeSubscriptionStatus(req.SubscriptionStatus)
	}
	account.SubscriptionStatus = planType

	if err := h.accountService.Create(account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	services.GetCliproxySyncService().Enqueue(account)
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

	tokenChanged := false
	if req.AccessToken != "" && req.AccessToken != account.AccessToken {
		tokenChanged = true
	}
	if req.RefreshToken != "" && req.RefreshToken != account.RefreshToken {
		tokenChanged = true
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
	planType := ""
	if account.AccessToken != "" {
		planType = services.ExtractSubscriptionStatusFromToken(account.AccessToken)
	}
	if planType == "" && req.SubscriptionStatus != "" {
		planType = services.NormalizeSubscriptionStatus(req.SubscriptionStatus)
	}
	if planType != "" {
		account.SubscriptionStatus = planType
	}
	if tokenChanged {
		account.CliproxySynced = false
		account.CliproxySyncedAt = nil
	}

	if err := h.accountService.Update(account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	services.GetCliproxySyncService().Enqueue(account)
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

	if account, err := h.accountService.GetByID(uint(id)); err == nil {
		if account.AccessToken != "" {
			if plan := services.ExtractSubscriptionStatusFromToken(account.AccessToken); plan != "" && plan != account.SubscriptionStatus {
				account.SubscriptionStatus = plan
				_ = h.accountService.Update(account)
			}
		}
		services.GetCliproxySyncService().Enqueue(account)
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
	payloads, err := decodeImportPayloads(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(payloads) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empty import payload"})
		return
	}

	var (
		imported    int
		errMessages []string
		lastAccount *models.Account
	)

	for i, payload := range payloads {
		account, meta, err := normalizeImportPayload(payload)
		if err != nil {
			errMessages = append(errMessages, fmt.Sprintf("item %d: %v", i+1, err))
			continue
		}

		existing, err := h.accountService.GetByEmail(account.Email)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			errMessages = append(errMessages, fmt.Sprintf("item %d: %v", i+1, err))
			continue
		}

		if err == nil && existing != nil {
			merged := mergeImportedAccount(existing, account, meta)
			if !meta.HasStatus && account.AccessToken != "" {
				merged.Status = "active"
			}
			if err := h.accountService.Update(merged); err != nil {
				errMessages = append(errMessages, fmt.Sprintf("item %d: %v", i+1, err))
				continue
			}
			lastAccount = merged
			services.GetCliproxySyncService().Enqueue(merged)
		} else {
			applyImportDefaults(account, meta)
			if err := h.accountService.Create(account); err != nil {
				errMessages = append(errMessages, fmt.Sprintf("item %d: %v", i+1, err))
				continue
			}
			lastAccount = account
			services.GetCliproxySyncService().Enqueue(account)
		}

		imported++
	}

	if len(payloads) == 1 && imported == 1 && lastAccount != nil {
		c.JSON(http.StatusOK, gin.H{"message": "Account imported", "id": lastAccount.ID})
		return
	}

	response := gin.H{
		"message":  "Accounts imported",
		"imported": imported,
		"failed":   len(errMessages),
	}
	if len(errMessages) > 0 {
		response["errors"] = errMessages
	}
	c.JSON(http.StatusOK, response)
}

func (h *AccountHandler) SyncCliproxy(c *gin.Context) {
	var accounts []models.Account
	if err := models.GetDB().Where("refresh_token IS NOT NULL AND refresh_token != ''").Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !services.GetCliproxySyncService().Enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliproxy sync is not configured"})
		return
	}

	var success int
	var failures []string
	for _, account := range accounts {
		accountCopy := account
		if !services.GetCliproxySyncService().Eligible(&accountCopy) {
			continue
		}
		if err := services.GetCliproxySyncService().SyncAccount(c.Request.Context(), &accountCopy); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", account.Email, err))
			continue
		}
		success++
	}

	resp := gin.H{
		"message":  "Cliproxy sync completed",
		"total":    len(accounts),
		"success":  success,
		"failed":   len(failures),
		"failures": failures,
	}
	c.JSON(http.StatusOK, resp)
}

type importAccountPayload struct {
	Email              string          `json:"email"`
	Password           string          `json:"password"`
	AccessToken        string          `json:"access_token"`
	RefreshToken       string          `json:"refresh_token"`
	CheckoutURL        string          `json:"checkout_url"`
	AccountID          string          `json:"account_id"`
	SessionCookies     json.RawMessage `json:"session_cookies"`
	Cookies            json.RawMessage `json:"cookies"`
	Status             string          `json:"status"`
	Name               string          `json:"name"`
	CreatedAt          string          `json:"created_at"`
	LastRefresh        string          `json:"last_refresh"`
	Expired            string          `json:"expired"`
	Type               string          `json:"type"`
	SubscriptionStatus string          `json:"subscription_status"`
	Notes              string          `json:"notes"`
}

type importAccountMeta struct {
	HasPassword           bool
	HasStatus             bool
	HasCookies            bool
	HasNotes              bool
	HasRegisteredAt       bool
	HasSubscriptionStatus bool
}

func decodeImportPayloads(r io.Reader) ([]importAccountPayload, error) {
	reader := bufio.NewReader(r)
	for {
		b, err := reader.Peek(1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil
			}
			return nil, err
		}
		if unicode.IsSpace(rune(b[0])) {
			_, _ = reader.ReadByte()
			continue
		}
		if b[0] == '[' {
			var payloads []importAccountPayload
			dec := json.NewDecoder(reader)
			if err := dec.Decode(&payloads); err != nil {
				return nil, err
			}
			return payloads, nil
		}

		var payloads []importAccountPayload
		dec := json.NewDecoder(reader)
		for {
			var item importAccountPayload
			if err := dec.Decode(&item); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, err
			}
			payloads = append(payloads, item)
		}
		return payloads, nil
	}
}

func normalizeImportPayload(payload importAccountPayload) (*models.Account, importAccountMeta, error) {
	meta := importAccountMeta{}
	email := strings.TrimSpace(payload.Email)
	if email == "" {
		return nil, meta, errors.New("email is required")
	}

	account := &models.Account{
		Email:        email,
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		CheckoutURL:  strings.TrimSpace(payload.CheckoutURL),
		AccountID:    strings.TrimSpace(payload.AccountID),
		Name:         strings.TrimSpace(payload.Name),
	}

	planType := ""
	if account.AccessToken != "" {
		planType = services.ExtractSubscriptionStatusFromToken(account.AccessToken)
	}
	if planType == "" {
		planType = services.NormalizeSubscriptionStatus(payload.SubscriptionStatus)
	}
	if planType == "" {
		planType = services.NormalizeSubscriptionStatus(payload.Type)
	}
	if planType != "" {
		account.SubscriptionStatus = planType
		meta.HasSubscriptionStatus = true
	}

	password := strings.TrimSpace(payload.Password)
	if password != "" {
		account.Password = password
		meta.HasPassword = true
	}

	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status != "" {
		if !isValidImportStatus(status) {
			return nil, meta, fmt.Errorf("invalid status: %s", status)
		}
		account.Status = status
		meta.HasStatus = true
	}

	sessionCookies := payload.SessionCookies
	if isNullJSON(sessionCookies) || len(sessionCookies) == 0 {
		sessionCookies = payload.Cookies
	}
	if !isNullJSON(sessionCookies) && len(sessionCookies) != 0 {
		account.SessionCookies = sessionCookies
		meta.HasCookies = true
	}

	notes := strings.TrimSpace(payload.Notes)
	if planType != "" {
		typeNote := "plan=" + planType
		if notes == "" {
			notes = typeNote
		} else if !strings.Contains(notes, typeNote) {
			notes = notes + "; " + typeNote
		}
	}
	if notes != "" {
		account.Notes = notes
		meta.HasNotes = true
	}

	if payload.CreatedAt != "" {
		createdAt, err := parseImportTime(payload.CreatedAt)
		if err != nil {
			return nil, meta, fmt.Errorf("created_at: %w", err)
		}
		account.RegisteredAt = *createdAt
		account.CreatedAt = *createdAt
		meta.HasRegisteredAt = true
	}

	if payload.LastRefresh != "" {
		lastRefresh, err := parseImportTime(payload.LastRefresh)
		if err != nil {
			return nil, meta, fmt.Errorf("last_refresh: %w", err)
		}
		account.UpdatedAt = *lastRefresh
	}

	if payload.Expired != "" {
		expiredAt, err := parseImportTime(payload.Expired)
		if err != nil {
			return nil, meta, fmt.Errorf("expired: %w", err)
		}
		account.TokenExpired = expiredAt
	}

	return account, meta, nil
}

func applyImportDefaults(account *models.Account, meta importAccountMeta) {
	if !meta.HasPassword || account.Password == "" {
		account.Password = "imported"
	}
	if account.Status == "" {
		if account.AccessToken != "" {
			account.Status = "active"
		} else {
			account.Status = "pending"
		}
	}
	if account.SubscriptionStatus == "" {
		account.SubscriptionStatus = "free"
	}
}

func mergeImportedAccount(existing *models.Account, incoming *models.Account, meta importAccountMeta) *models.Account {
	merged := *existing

	if incoming.AccessToken != "" {
		merged.AccessToken = incoming.AccessToken
	}
	if incoming.RefreshToken != "" {
		merged.RefreshToken = incoming.RefreshToken
	}
	if incoming.CheckoutURL != "" {
		merged.CheckoutURL = incoming.CheckoutURL
	}
	if incoming.AccountID != "" {
		merged.AccountID = incoming.AccountID
	}
	if incoming.Name != "" {
		merged.Name = incoming.Name
	}
	if !incoming.RegisteredAt.IsZero() {
		merged.RegisteredAt = incoming.RegisteredAt
	}
	if incoming.TokenExpired != nil {
		merged.TokenExpired = incoming.TokenExpired
	}
	if meta.HasCookies {
		merged.SessionCookies = incoming.SessionCookies
	}
	if meta.HasPassword {
		merged.Password = incoming.Password
	}
	if meta.HasStatus {
		merged.Status = incoming.Status
	}
	if meta.HasSubscriptionStatus {
		merged.SubscriptionStatus = incoming.SubscriptionStatus
	}
	if meta.HasNotes {
		merged.Notes = mergeImportNotes(existing.Notes, incoming.Notes)
	}

	tokenChanged := false
	if incoming.AccessToken != "" && incoming.AccessToken != existing.AccessToken {
		tokenChanged = true
	}
	if incoming.RefreshToken != "" && incoming.RefreshToken != existing.RefreshToken {
		tokenChanged = true
	}
	if tokenChanged {
		merged.CliproxySynced = false
		merged.CliproxySyncedAt = nil
	}

	return &merged
}

func mergeImportNotes(existing string, incoming string) string {
	if incoming == "" {
		return existing
	}
	if existing == "" {
		return incoming
	}
	if strings.Contains(existing, incoming) {
		return existing
	}
	return existing + "; " + incoming
}

func isNullJSON(raw json.RawMessage) bool {
	return len(raw) != 0 && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isValidImportStatus(status string) bool {
	switch status {
	case "pending", "active", "failed", "expired":
		return true
	default:
		return false
	}
}

func parseImportTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	layoutsWithZone := []string{time.RFC3339Nano, time.RFC3339}
	for _, layout := range layoutsWithZone {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	layoutsNoZone := []string{
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layoutsNoZone {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("unsupported time format: %s", value)
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

// BatchTestAllAccounts 一键检测全部账号
func (h *AccountHandler) BatchTestAllAccounts(c *gin.Context) {
	var ids []uint
	if err := models.GetDB().Model(&models.Account{}).
		Where("access_token IS NOT NULL AND access_token != ''").
		Pluck("id", &ids).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no accounts found"})
		return
	}

	taskID, err := h.accountTestService.BatchTestAccounts(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"message": "批量测试已开始",
		"total":   len(ids),
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
		"message":      "Token 刷新成功",
		"access_token": result.AccessToken[:50] + "...",
	})
}

// RefreshSubscriptionStatus 从现有 access_token 中刷新订阅状态
func (h *AccountHandler) RefreshSubscriptionStatus(c *gin.Context) {
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

	if account.AccessToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号无 access_token"})
		return
	}

	// 从 access_token 中提取订阅状态
	plan := services.ExtractSubscriptionStatusFromToken(account.AccessToken)
	if plan == "" {
		plan = "free"
	}

	oldStatus := account.SubscriptionStatus
	if plan != oldStatus {
		account.SubscriptionStatus = plan
		if err := h.accountService.Update(account); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":             "订阅状态已刷新",
		"subscription_status": plan,
		"old_status":          oldStatus,
		"changed":             plan != oldStatus,
	})
}
