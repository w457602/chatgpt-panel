package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/webauto/chatgpt-panel/internal/models"
	"gorm.io/gorm"
)

const (
	chatgptBaseURL   = "https://chatgpt.com"
	openaiTokenURL   = "https://auth.openai.com/oauth/token"
	openaiClientID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	testTimeout      = 15 * time.Second
	batchConcurrency = 5
)

// AccountTestResult 账号测试结果
type AccountTestResult struct {
	ID             uint      `json:"id"`
	Email          string    `json:"email"`
	Status         string    `json:"status"` // active, expired, banned, error
	Valid          bool      `json:"valid"`
	Message        string    `json:"message"`
	Models         []string  `json:"models,omitempty"`
	TokenRefreshed bool      `json:"token_refreshed,omitempty"`
	TestedAt       time.Time `json:"tested_at"`
}

// BatchTestResult 批量测试结果
type BatchTestResult struct {
	TaskID     string              `json:"task_id"`
	Total      int                 `json:"total"`
	Completed  int                 `json:"completed"`
	InProgress bool                `json:"in_progress"`
	Results    []AccountTestResult `json:"results"`
	StartedAt  time.Time           `json:"started_at"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
}

// RefreshTokenResult Token刷新结果
type RefreshTokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
}

type AccountTestService struct {
	db         *gorm.DB
	httpClient *http.Client

	// 批量测试任务管理
	mu         sync.RWMutex
	batchTasks map[string]*BatchTestResult
}

func NewAccountTestService() *AccountTestService {
	return &AccountTestService{
		db: models.GetDB(),
		httpClient: &http.Client{
			Timeout: testTimeout,
		},
		batchTasks: make(map[string]*BatchTestResult),
	}
}

// TestAccount 测试单个账号
func (s *AccountTestService) TestAccount(ctx context.Context, id uint) (*AccountTestResult, error) {
	var account models.Account
	if err := s.db.First(&account, id).Error; err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	result := &AccountTestResult{
		ID:       account.ID,
		Email:    account.Email,
		TestedAt: time.Now(),
	}

	// 检查是否有 access_token
	if strings.TrimSpace(account.AccessToken) == "" {
		// 尝试使用 refresh_token 刷新
		if strings.TrimSpace(account.RefreshToken) != "" {
			refreshed, err := s.RefreshAccessToken(ctx, account.RefreshToken)
			if err != nil {
				result.Status = "expired"
				result.Valid = false
				result.Message = "无 access_token 且刷新失败: " + err.Error()
				s.updateAccountStatus(account.ID, "expired")
				return result, nil
			}
			// 更新账号的 token
			s.updateAccountTokens(account.ID, refreshed)
			account.AccessToken = refreshed.AccessToken
			result.TokenRefreshed = true
		} else {
			result.Status = "expired"
			result.Valid = false
			result.Message = "无 access_token 且无 refresh_token"
			s.updateAccountStatus(account.ID, "expired")
			return result, nil
		}
	}

	// 调用 ChatGPT API 测试 token 有效性
	testResult := s.testAccessToken(ctx, account.AccessToken)

	if testResult.Valid {
		result.Status = "active"
		result.Valid = true
		result.Message = "账号可用"
		result.Models = testResult.Models
		s.updateAccountStatus(account.ID, "active")
	} else {
		// Token 无效，尝试刷新
		if strings.TrimSpace(account.RefreshToken) != "" && !result.TokenRefreshed {
			refreshed, err := s.RefreshAccessToken(ctx, account.RefreshToken)
			if err == nil {
				s.updateAccountTokens(account.ID, refreshed)
				// 重新测试
				testResult = s.testAccessToken(ctx, refreshed.AccessToken)
				if testResult.Valid {
					result.Status = "active"
					result.Valid = true
					result.Message = "Token 已刷新，账号可用"
					result.Models = testResult.Models
					result.TokenRefreshed = true
					s.updateAccountStatus(account.ID, "active")
					return result, nil
				}
			}
		}
		result.Status = testResult.Status
		result.Valid = false
		result.Message = testResult.Message
		s.updateAccountStatus(account.ID, testResult.Status)
	}

	return result, nil
}

// tokenTestResult 内部测试结果
type tokenTestResult struct {
	Valid   bool
	Status  string
	Message string
	Models  []string
}

// testAccessToken 测试 access_token 有效性
func (s *AccountTestService) testAccessToken(ctx context.Context, accessToken string) *tokenTestResult {
	endpoint := chatgptBaseURL + "/backend-api/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return &tokenTestResult{Valid: false, Status: "error", Message: err.Error()}
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &tokenTestResult{Valid: false, Status: "error", Message: "请求失败: " + err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var modelsResp struct {
			Models []struct {
				Slug string `json:"slug"`
			} `json:"models"`
		}
		var models []string
		if json.Unmarshal(body, &modelsResp) == nil {
			for _, m := range modelsResp.Models {
				models = append(models, m.Slug)
			}
		}
		return &tokenTestResult{Valid: true, Status: "active", Message: "Token 有效", Models: models}
	case http.StatusUnauthorized:
		return &tokenTestResult{Valid: false, Status: "expired", Message: "Token 已过期 (401)"}
	case http.StatusForbidden:
		return &tokenTestResult{Valid: false, Status: "banned", Message: "账号被封禁 (403)"}
	case http.StatusTooManyRequests:
		return &tokenTestResult{Valid: false, Status: "rate_limited", Message: "请求过于频繁 (429)"}
	default:
		return &tokenTestResult{Valid: false, Status: "error", Message: fmt.Sprintf("未知错误 (%d): %s", resp.StatusCode, string(body)[:min(200, len(body))])}
	}
}

// RefreshAccessToken 使用 refresh_token 刷新 access_token
func (s *AccountTestService) RefreshAccessToken(ctx context.Context, refreshToken string) (*RefreshTokenResult, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	data := url.Values{
		"client_id":     {openaiClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"scope":         {"openid profile email offline_access"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("刷新失败 (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp RefreshTokenResult
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &tokenResp, nil
}

func (s *AccountTestService) updateAccountStatus(id uint, status string) {
	s.db.Model(&models.Account{}).Where("id = ?", id).Update("status", status)
}

func (s *AccountTestService) updateAccountTokens(id uint, tokens *RefreshTokenResult) {
	updates := map[string]interface{}{
		"access_token": tokens.AccessToken,
		"status":       "active",
	}
	if tokens.IDToken != "" {
		if plan := ExtractSubscriptionStatusFromToken(tokens.IDToken); plan != "" {
			updates["subscription_status"] = plan
		}
	} else if plan := ExtractSubscriptionStatusFromToken(tokens.AccessToken); plan != "" {
		updates["subscription_status"] = plan
	}
	if tokens.RefreshToken != "" {
		updates["refresh_token"] = tokens.RefreshToken
	}
	if tokens.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		updates["token_expired"] = &expiresAt
	}
	s.db.Model(&models.Account{}).Where("id = ?", id).Updates(updates)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BatchTestAccounts 批量测试账号
func (s *AccountTestService) BatchTestAccounts(ctx context.Context, ids []uint) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("no account ids provided")
	}

	taskID := fmt.Sprintf("batch_%d", time.Now().UnixNano())

	task := &BatchTestResult{
		TaskID:     taskID,
		Total:      len(ids),
		Completed:  0,
		InProgress: true,
		Results:    make([]AccountTestResult, 0, len(ids)),
		StartedAt:  time.Now(),
	}

	s.mu.Lock()
	s.batchTasks[taskID] = task
	s.mu.Unlock()

	// 异步执行批量测试
	go s.runBatchTest(ctx, taskID, ids)

	return taskID, nil
}

func (s *AccountTestService) runBatchTest(ctx context.Context, taskID string, ids []uint) {
	sem := make(chan struct{}, batchConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, id := range ids {
		wg.Add(1)
		go func(accountID uint) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := s.TestAccount(ctx, accountID)
			if err != nil {
				result = &AccountTestResult{
					ID:       accountID,
					Status:   "error",
					Valid:    false,
					Message:  err.Error(),
					TestedAt: time.Now(),
				}
			}

			mu.Lock()
			s.mu.Lock()
			if task, ok := s.batchTasks[taskID]; ok {
				task.Results = append(task.Results, *result)
				task.Completed++
			}
			s.mu.Unlock()
			mu.Unlock()
		}(id)
	}

	wg.Wait()

	// 标记任务完成
	s.mu.Lock()
	if task, ok := s.batchTasks[taskID]; ok {
		task.InProgress = false
		now := time.Now()
		task.FinishedAt = &now
	}
	s.mu.Unlock()
}

// GetBatchTestResult 获取批量测试结果
func (s *AccountTestService) GetBatchTestResult(taskID string) (*BatchTestResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.batchTasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return task, nil
}

// CleanupOldTasks 清理旧任务（保留1小时内的任务）
func (s *AccountTestService) CleanupOldTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for taskID, task := range s.batchTasks {
		if task.FinishedAt != nil && task.FinishedAt.Before(cutoff) {
			delete(s.batchTasks, taskID)
		}
	}
}
