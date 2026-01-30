package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
	"gorm.io/gorm"
)

type RedemptionHandler struct {
	service *services.RedemptionService
	db      *gorm.DB
}

func NewRedemptionHandler(db *gorm.DB) *RedemptionHandler {
	return &RedemptionHandler{
		service: services.NewRedemptionService(db),
		db:      db,
	}
}

// Redeem 兑换兑换码（公开接口）
func (h *RedemptionHandler) Redeem(c *gin.Context) {
	var req services.RedemptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.RedeemCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": result.Message,
		"data":    result,
	})
}

// AdminRedeem 管理员兑换（需要认证）
func (h *RedemptionHandler) AdminRedeem(c *gin.Context) {
	var req services.RedemptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.RedeemCode(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": result.Message,
		"data":    result,
	})
}

// BatchCreate 批量创建兑换码（需要认证）
func (h *RedemptionHandler) BatchCreate(c *gin.Context) {
	var req struct {
		Count     int    `json:"count" binding:"required,min=1,max=100"`
		Channel   string `json:"channel"`
		OrderType string `json:"order_type"`
		AccountID *uint  `json:"account_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	codes, err := h.service.BatchCreateRedemptionCodes(req.Count, req.Channel, req.OrderType, req.AccountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "批量创建成功",
		"count":   len(codes),
		"codes":   codes,
	})
}

// List 获取兑换码列表（需要认证）
func (h *RedemptionHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	channel := c.Query("channel")
	isRedeemed := c.Query("is_redeemed")

	query := h.db.Model(&models.RedemptionCode{})

	if channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if isRedeemed != "" {
		if isRedeemed == "true" {
			query = query.Where("is_redeemed = ?", true)
		} else if isRedeemed == "false" {
			query = query.Where("is_redeemed = ?", false)
		}
	}

	var total int64
	query.Count(&total)

	var codes []models.RedemptionCode
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&codes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        codes,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (int(total) + pageSize - 1) / pageSize,
	})
}

// Get 获取单个兑换码详情（需要认证）
func (h *RedemptionHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	var code models.RedemptionCode
	if err := h.db.First(&code, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "兑换码不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, code)
}

// Delete 删除兑换码（需要认证）
func (h *RedemptionHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	if err := h.db.Delete(&models.RedemptionCode{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

