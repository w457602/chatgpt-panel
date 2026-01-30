package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
)

// SystemConfigHandler 系统配置处理器
type SystemConfigHandler struct {
	service *services.SystemConfigService
}

// NewSystemConfigHandler 创建系统配置处理器
func NewSystemConfigHandler() *SystemConfigHandler {
	return &SystemConfigHandler{
		service: services.GetSystemConfigService(),
	}
}

// GetLinuxDoOAuthConfig 获取 Linux DO OAuth 配置
func (h *SystemConfigHandler) GetLinuxDoOAuthConfig(c *gin.Context) {
	db := models.GetDB()
	config := h.service.GetLinuxDoOAuthConfig(db)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateLinuxDoOAuthConfig 更新 Linux DO OAuth 配置
func (h *SystemConfigHandler) UpdateLinuxDoOAuthConfig(c *gin.Context) {
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}

	db := models.GetDB()
	if err := h.service.UpdateLinuxDoOAuthConfig(db, req.ClientID, req.ClientSecret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置更新成功",
	})
}

// GetLinuxDoCreditConfig 获取 Linux DO Credit 配置
func (h *SystemConfigHandler) GetLinuxDoCreditConfig(c *gin.Context) {
	db := models.GetDB()
	config := h.service.GetLinuxDoCreditConfig(db)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateLinuxDoCreditConfig 更新 Linux DO Credit 配置
func (h *SystemConfigHandler) UpdateLinuxDoCreditConfig(c *gin.Context) {
	var req struct {
		PID string `json:"pid"`
		Key string `json:"key"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}

	db := models.GetDB()
	if err := h.service.UpdateLinuxDoCreditConfig(db, req.PID, req.Key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置更新成功",
	})
}

// GetAllConfigs 获取所有配置（用于导出）
func (h *SystemConfigHandler) GetAllConfigs(c *gin.Context) {
	category := c.Query("category")

	db := models.GetDB()
	configs, err := h.service.GetAllConfigs(db, category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    configs,
	})
}

// DeleteConfig 删除配置项
func (h *SystemConfigHandler) DeleteConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "配置键名不能为空",
		})
		return
	}

	db := models.GetDB()
	if err := h.service.DeleteConfig(db, key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "删除配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置删除成功",
	})
}

