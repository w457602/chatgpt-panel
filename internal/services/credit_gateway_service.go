package services

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/webauto/chatgpt-panel/internal/config"
	"github.com/webauto/chatgpt-panel/internal/models"
)

// CreditGatewayService Linux DO Credit 支付网关服务
type CreditGatewayService struct{}

var creditGatewayServiceInstance *CreditGatewayService

// GetCreditGatewayService 获取 Credit 网关服务单例
func GetCreditGatewayService() *CreditGatewayService {
	if creditGatewayServiceInstance == nil {
		creditGatewayServiceInstance = &CreditGatewayService{}
	}
	return creditGatewayServiceInstance
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	LinuxDoUID  string
	Email       string
	Amount      string
	ProductName string
	NotifyURL   string
	ReturnURL   string
}

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	OrderNo       string
	PayURL        string
	Amount        string
	ProductName   string
	PayingOrderNo string
}

// GenerateSign 生成 MD5 签名
func (s *CreditGatewayService) GenerateSign(params map[string]string, key string) string {
	// 按键名排序
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" && k != "sign_type" && params[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	// 拼接字符串
	var signStr strings.Builder
	for i, k := range keys {
		if i > 0 {
			signStr.WriteString("&")
		}
		signStr.WriteString(k)
		signStr.WriteString("=")
		signStr.WriteString(params[k])
	}
	signStr.WriteString(key)

	// MD5 加密
	hash := md5.Sum([]byte(signStr.String()))
	return hex.EncodeToString(hash[:])
}

// VerifySign 验证签名
func (s *CreditGatewayService) VerifySign(params map[string]string, key string) bool {
	receivedSign, ok := params["sign"]
	if !ok {
		return false
	}

	calculatedSign := s.GenerateSign(params, key)
	return receivedSign == calculatedSign
}

// CreateOrder 创建支付订单
func (s *CreditGatewayService) CreateOrder(req CreateOrderRequest) (*CreateOrderResponse, error) {
	db := models.GetDB()
	pid := models.GetConfigValue(db, "linuxdo_credit_pid", config.AppConfig.LinuxDoCreditPID)
	key := models.GetConfigValue(db, "linuxdo_credit_key", config.AppConfig.LinuxDoCreditKey)
	baseURL := config.AppConfig.LinuxDoCreditBaseURL

	if pid == "" || key == "" {
		return nil, errors.New("Linux DO Credit 支付配置不完整")
	}

	if baseURL == "" {
		baseURL = "https://credit.linux.do/epay"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 生成订单号
	orderNo := fmt.Sprintf("LDC%d", time.Now().UnixNano()/1000)

	// 构建请求参数
	params := map[string]string{
		"pid":          pid,
		"type":         "epay",
		"out_trade_no": orderNo,
		"name":         req.ProductName,
		"money":        req.Amount,
	}

	if req.NotifyURL != "" {
		params["notify_url"] = req.NotifyURL
	}
	if req.ReturnURL != "" {
		params["return_url"] = req.ReturnURL
	}

	// 生成签名
	params["sign"] = s.GenerateSign(params, key)
	params["sign_type"] = "MD5"

	// 发送请求
	formData := url.Values{}
	for k, v := range params {
		formData.Set(k, v)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	submitURL := baseURL + "/pay/submit.php"
	resp, err := client.PostForm(submitURL, formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 获取重定向 URL
	location := resp.Header.Get("Location")
	if location == "" {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create order failed: %s", string(body))
	}

	// 提取 paying_order_no
	payingOrderNo := ""
	if parsedURL, err := url.Parse(location); err == nil {
		payingOrderNo = parsedURL.Query().Get("order_no")
	}

	// 保存订单到数据库
	expiredAt := time.Now().Add(30 * time.Minute)
	order := models.CreditOrder{
		OrderNo:       orderNo,
		LinuxDoUID:    req.LinuxDoUID,
		Email:         req.Email,
		Amount:        req.Amount,
		ProductName:   req.ProductName,
		Status:        "pending",
		PayURL:        location,
		PayingOrderNo: payingOrderNo,
		ExpiredAt:     &expiredAt,
	}

	if err := db.Create(&order).Error; err != nil {
		return nil, err
	}

	return &CreateOrderResponse{
		OrderNo:       orderNo,
		PayURL:        location,
		Amount:        req.Amount,
		ProductName:   req.ProductName,
		PayingOrderNo: payingOrderNo,
	}, nil
}

// QueryOrder 查询订单状态
func (s *CreditGatewayService) QueryOrder(orderNo string) (map[string]interface{}, error) {
	db := models.GetDB()
	pid := models.GetConfigValue(db, "linuxdo_credit_pid", config.AppConfig.LinuxDoCreditPID)
	key := models.GetConfigValue(db, "linuxdo_credit_key", config.AppConfig.LinuxDoCreditKey)
	baseURL := config.AppConfig.LinuxDoCreditBaseURL

	if pid == "" || key == "" {
		return nil, errors.New("Linux DO Credit 支付配置不完整")
	}

	if baseURL == "" {
		baseURL = "https://credit.linux.do/epay"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 构建查询参数
	params := map[string]string{
		"pid":          pid,
		"type":         "epay",
		"out_trade_no": orderNo,
	}

	params["sign"] = s.GenerateSign(params, key)
	params["sign_type"] = "MD5"

	// 构建查询 URL
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	queryURL := fmt.Sprintf("%s/api/query.php?%s", baseURL, values.Encode())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(queryURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// HandleNotify 处理支付回调
func (s *CreditGatewayService) HandleNotify(params map[string]string) error {
	db := models.GetDB()
	key := models.GetConfigValue(db, "linuxdo_credit_key", config.AppConfig.LinuxDoCreditKey)
	if key == "" {
		return errors.New("支付密钥未配置")
	}

	// 验证签名
	if !s.VerifySign(params, key) {
		return errors.New("签名验证失败")
	}

	orderNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	money := params["money"]
	status := params["trade_status"]

	if orderNo == "" {
		return errors.New("订单号为空")
	}

	// 更新订单状态
	var order models.CreditOrder
	if err := db.Where("order_no = ?", orderNo).First(&order).Error; err != nil {
		return err
	}

	// 防止重复处理
	if order.Status == "paid" {
		return nil
	}

	// 更新订单
	updates := map[string]interface{}{
		"trade_no": tradeNo,
	}

	payloadJSON, _ := json.Marshal(params)
	updates["notify_payload"] = string(payloadJSON)
	now := time.Now()
	updates["notify_at"] = &now

	if status == "TRADE_SUCCESS" {
		// 验证金额
		if money != order.Amount {
			updates["refund_message"] = fmt.Sprintf("money_mismatch:%s", money)
			updates["status"] = "refunded"
		} else {
			updates["status"] = "paid"
			updates["paid_at"] = &now
		}
	}

	return db.Model(&order).Updates(updates).Error
}

