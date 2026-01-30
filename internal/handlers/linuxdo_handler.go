package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
)

// LinuxDoHandler Linux DO 相关处理器
type LinuxDoHandler struct {
	oauthService   *services.LinuxDoOAuthService
	gatewayService *services.CreditGatewayService
}

// NewLinuxDoHandler 创建 Linux DO 处理器
func NewLinuxDoHandler() *LinuxDoHandler {
	return &LinuxDoHandler{
		oauthService:   services.GetLinuxDoOAuthService(),
		gatewayService: services.GetCreditGatewayService(),
	}
}

// GetAuthorizeURL 获取授权 URL
func (h *LinuxDoHandler) GetAuthorizeURL(c *gin.Context) {
	redirectURI := c.Query("redirectUri")
	if redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 redirectUri 参数"})
		return
	}

	url, err := h.oauthService.GetAuthorizeURL(redirectURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ExchangeCode 用授权码换取用户信息
func (h *LinuxDoHandler) ExchangeCode(c *gin.Context) {
	var req struct {
		Code        string `json:"code" binding:"required"`
		RedirectURI string `json:"redirectUri" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	// 1. 换取 Access Token
	tokenResp, err := h.oauthService.ExchangeCode(req.Code, req.RedirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "授权码无效或已过期"})
		return
	}

	// 2. 获取用户信息
	user, err := h.oauthService.GetUser(tokenResp.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户信息失败"})
		return
	}

	// 3. 存储/更新用户信息
	uid := strconv.Itoa(user.ID)
	if err := h.oauthService.UpsertLinuxDoUser(uid, user.Username, user.Name, user.TrustLevel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存用户信息失败"})
		return
	}

	// 4. 生成 Session Token
	sessionToken, err := h.oauthService.SignSessionToken(uid, user.Username, user.Name, user.TrustLevel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成会话失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":         user,
		"sessionToken": sessionToken,
	})
}

// GetUserInfo 获取当前用户信息（需要 Session Token）
func (h *LinuxDoHandler) GetUserInfo(c *gin.Context) {
	tokenString := c.GetHeader("X-LinuxDo-Token")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Session Token"})
		return
	}

	claims, err := h.oauthService.VerifySessionToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session Token 无效"})
		return
	}

	uid, _ := (*claims)["uid"].(string)
	username, _ := (*claims)["username"].(string)

	// 查询数据库获取完整信息
	db := models.GetDB()
	var user models.LinuxDoUser
	if err := db.Where("uid = ?", uid).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"uid":                    user.UID,
		"username":               username,
		"email":                  user.Email,
		"currentOpenAccountId":   user.CurrentOpenAccountID,
	})
}

// UpdateEmail 更新用户邮箱
func (h *LinuxDoHandler) UpdateEmail(c *gin.Context) {
	tokenString := c.GetHeader("X-LinuxDo-Token")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Session Token"})
		return
	}

	claims, err := h.oauthService.VerifySessionToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session Token 无效"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱格式不正确"})
		return
	}

	uid, _ := (*claims)["uid"].(string)

	db := models.GetDB()
	if err := db.Model(&models.LinuxDoUser{}).Where("uid = ?", uid).Update("email", req.Email).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"uid":      uid,
		"email":    req.Email,
	})
}

// CreateCreditOrder 创建 Credit 订单
func (h *LinuxDoHandler) CreateCreditOrder(c *gin.Context) {
	tokenString := c.GetHeader("X-LinuxDo-Token")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Session Token", "code": "LINUXDO_SESSION_REQUIRED"})
		return
	}

	claims, err := h.oauthService.VerifySessionToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session Token 无效"})
		return
	}

	var req struct {
		Email       string `json:"email" binding:"required,email"`
		Amount      string `json:"amount" binding:"required"`
		ProductName string `json:"productName"`
		NotifyURL   string `json:"notifyUrl"`
		ReturnURL   string `json:"returnUrl"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	uid, _ := (*claims)["uid"].(string)

	// 创建订单
	orderReq := services.CreateOrderRequest{
		LinuxDoUID:  uid,
		Email:       req.Email,
		Amount:      req.Amount,
		ProductName: req.ProductName,
		NotifyURL:   req.NotifyURL,
		ReturnURL:   req.ReturnURL,
	}

	if orderReq.ProductName == "" {
		orderReq.ProductName = "ChatGPT Team 订阅"
	}

	order, err := h.gatewayService.CreateOrder(orderReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建订单失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orderNo":       order.OrderNo,
		"payUrl":        order.PayURL,
		"amount":        order.Amount,
		"productName":   order.ProductName,
		"payingOrderNo": order.PayingOrderNo,
	})
}

// QueryCreditOrder 查询订单状态
func (h *LinuxDoHandler) QueryCreditOrder(c *gin.Context) {
	orderNo := c.Param("orderNo")
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少订单号"})
		return
	}

	// 从数据库查询
	db := models.GetDB()
	var order models.CreditOrder
	if err := db.Where("order_no = ?", orderNo).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order": order,
	})
}

// CreditNotify 处理支付回调
func (h *LinuxDoHandler) CreditNotify(c *gin.Context) {
	params := make(map[string]string)
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	if err := h.gatewayService.HandleNotify(params); err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}

	c.String(http.StatusOK, "success")
}

// ListCreditOrders 查询用户的订单列表
func (h *LinuxDoHandler) ListCreditOrders(c *gin.Context) {
	tokenString := c.GetHeader("X-LinuxDo-Token")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Session Token"})
		return
	}

	claims, err := h.oauthService.VerifySessionToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session Token 无效"})
		return
	}

	uid, _ := (*claims)["uid"].(string)

	db := models.GetDB()
	var orders []models.CreditOrder
	if err := db.Where("linuxdo_uid = ?", uid).Order("created_at DESC").Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
	})
}

