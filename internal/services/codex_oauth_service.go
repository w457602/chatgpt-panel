package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/webauto/chatgpt-panel/internal/models"
)

const (
	codexAuthURL         = "https://auth.openai.com/oauth/authorize"
	codexTokenURL        = "https://auth.openai.com/oauth/token"
	codexClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexRedirectURI     = "http://localhost:1455/auth/callback"
	codexScope           = "openid email profile offline_access"
	codexCallbackAddr    = ":1455"
	codexSessionTTL      = 10 * time.Minute
	codexExchangeTimeout = 30 * time.Second
)

type PKCECodes struct {
	CodeVerifier  string
	CodeChallenge string
}

type codexSession struct {
	accountID uint
	pkce      PKCECodes
	createdAt time.Time
	expiresAt time.Time
	status    string // pending, success, failed
	errorMsg  string
}

type codexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type idTokenClaims struct {
	Email string `json:"email"`
	Auth  struct {
		ChatgptAccountID string `json:"chatgpt_account_id"`
		ChatgptPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

type CodexOAuthService struct {
	mu         sync.Mutex
	sessions   map[string]codexSession
	ttl        time.Duration
	httpClient *http.Client

	serverOnce sync.Once
	serverErr  error
	server     *http.Server
}

var codexOAuthService = NewCodexOAuthService()

func NewCodexOAuthService() *CodexOAuthService {
	return &CodexOAuthService{
		sessions: make(map[string]codexSession),
		ttl:      codexSessionTTL,
		httpClient: &http.Client{
			Timeout: codexExchangeTimeout,
		},
	}
}

func GetCodexOAuthService() *CodexOAuthService {
	return codexOAuthService
}

func (s *CodexOAuthService) EnsureCallbackServer() error {
	s.serverOnce.Do(func() {
		s.serverErr = s.startCallbackServer()
	})
	return s.serverErr
}

func (s *CodexOAuthService) Start(accountID uint) (string, string, error) {
	if accountID == 0 {
		return "", "", fmt.Errorf("account_id is required")
	}
	if err := s.EnsureCallbackServer(); err != nil {
		return "", "", err
	}

	pkce, err := generatePKCECodes()
	if err != nil {
		return "", "", err
	}

	state, err := generateState()
	if err != nil {
		return "", "", err
	}

	authURL, err := buildAuthURL(state, pkce)
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	s.mu.Lock()
	s.purgeExpiredLocked(now)
	s.sessions[state] = codexSession{
		accountID: accountID,
		pkce:      *pkce,
		createdAt: now,
		expiresAt: now.Add(s.ttl),
		status:    "pending",
	}
	s.mu.Unlock()

	return authURL, state, nil
}

func (s *CodexOAuthService) startCallbackServer() error {
	listener, err := net.Listen("tcp", codexCallbackAddr)
	if err != nil {
		return fmt.Errorf("codex callback server listen failed: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", s.handleCallback)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if errServe := s.server.Serve(listener); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
			log.Printf("codex callback server error: %v", errServe)
		}
	}()

	return nil
}

func (s *CodexOAuthService) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	code := strings.TrimSpace(query.Get("code"))
	state := strings.TrimSpace(query.Get("state"))
	errorParam := strings.TrimSpace(query.Get("error"))

	ctx, cancel := context.WithTimeout(r.Context(), codexExchangeTimeout)
	defer cancel()

	if _, err := s.completeOAuth(ctx, state, code, errorParam); err != nil {
		writeCallbackHTML(w, false, err.Error())
		return
	}

	writeCallbackHTML(w, true, "")
}

func (s *CodexOAuthService) completeOAuth(ctx context.Context, state, code, errorParam string) (*codexTokenResponse, error) {
	if errorParam != "" {
		s.updateSessionStatus(state, "failed", "oauth error: "+errorParam)
		return nil, fmt.Errorf("oauth error: %s", errorParam)
	}
	if state == "" || code == "" {
		s.updateSessionStatus(state, "failed", "missing code or state")
		return nil, errors.New("missing code or state")
	}

	session, ok := s.getSession(state)
	if !ok {
		return nil, errors.New("invalid or expired state")
	}

	tokenResp, err := s.exchangeCode(ctx, code, session.pkce.CodeVerifier)
	if err != nil {
		s.updateSessionStatus(state, "failed", err.Error())
		return nil, err
	}

	email, accountID := parseIDToken(tokenResp.IDToken)
	if err := s.updateAccount(session.accountID, tokenResp, email, accountID); err != nil {
		s.updateSessionStatus(state, "failed", err.Error())
		return nil, err
	}

	s.updateSessionStatus(state, "success", "")
	return tokenResp, nil
}

func (s *CodexOAuthService) exchangeCode(ctx context.Context, code, verifier string) (*codexTokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {codexClientID},
		"code":          {code},
		"redirect_uri":  {codexRedirectURI},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp codexTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("token response missing access_token")
	}

	return &tokenResp, nil
}

func (s *CodexOAuthService) updateAccount(accountID uint, token *codexTokenResponse, tokenEmail, tokenAccountID string) error {
	var account models.Account
	if err := models.GetDB().First(&account, accountID).Error; err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	planType := NormalizeSubscriptionStatus(parsePlanFromIDToken(token.IDToken))

	if tokenEmail != "" && !strings.EqualFold(account.Email, tokenEmail) {
		return fmt.Errorf("email mismatch: token=%s account=%s", tokenEmail, account.Email)
	}

	updates := map[string]interface{}{
		"access_token": token.AccessToken,
		"status":       "active",
	}

	if token.RefreshToken != "" {
		updates["refresh_token"] = token.RefreshToken
		account.RefreshToken = token.RefreshToken
	}
	if tokenAccountID != "" {
		updates["account_id"] = tokenAccountID
		account.AccountID = tokenAccountID
	}
	if planType != "" {
		updates["subscription_status"] = planType
		account.SubscriptionStatus = planType
	}
	if token.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		updates["token_expired"] = &expiresAt
		account.TokenExpired = &expiresAt
	}

	if err := models.GetDB().Model(&models.Account{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
		return err
	}

	account.AccessToken = token.AccessToken
	account.Status = "active"
	// 手动同步
	return nil
}

func (s *CodexOAuthService) getSession(state string) (codexSession, bool) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	return session, ok
}

func (s *CodexOAuthService) deleteSession(state string) {
	if state == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, state)
}

func (s *CodexOAuthService) updateSessionStatus(state, status, errorMsg string) {
	if state == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[state]; ok {
		session.status = status
		session.errorMsg = errorMsg
		s.sessions[state] = session
	}
}

// OAuthStatusResult OAuth 状态查询结果
type OAuthStatusResult struct {
	Status    string `json:"status"` // pending, success, failed, expired
	Message   string `json:"message,omitempty"`
	AccountID uint   `json:"account_id,omitempty"`
}

// GetStatus 获取 OAuth 授权状态
func (s *CodexOAuthService) GetStatus(state string) *OAuthStatusResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[state]
	if !ok {
		return &OAuthStatusResult{Status: "expired", Message: "会话已过期或不存在"}
	}

	if time.Now().After(session.expiresAt) {
		delete(s.sessions, state)
		return &OAuthStatusResult{Status: "expired", Message: "会话已过期"}
	}

	return &OAuthStatusResult{
		Status:    session.status,
		Message:   session.errorMsg,
		AccountID: session.accountID,
	}
}

// ProcessCallbackURL 处理手动提交的回调 URL
func (s *CodexOAuthService) ProcessCallbackURL(state, callbackURL string) (*OAuthStatusResult, error) {
	// 解析回调 URL
	code, parsedState, errMsg, err := parseCallbackURL(callbackURL)
	if err != nil {
		return nil, err
	}

	// 如果 URL 中包含 state，优先使用 URL 中的 state
	if parsedState != "" {
		state = parsedState
	}

	// 验证 state
	if state == "" {
		return nil, fmt.Errorf("state is required")
	}

	// 检查是否有错误
	if errMsg != "" {
		return &OAuthStatusResult{
			Status:  "error",
			Message: errMsg,
		}, nil
	}

	// 完成 OAuth 流程
	ctx, cancel := context.WithTimeout(context.Background(), codexExchangeTimeout)
	defer cancel()

	_, err = s.completeOAuth(ctx, state, code, "")
	if err != nil {
		return nil, err
	}

	status := s.GetStatus(state)
	if status == nil {
		return &OAuthStatusResult{Status: "success"}, nil
	}
	return status, nil
}

// parseCallbackURL 解析回调 URL 提取 OAuth 参数
func parseCallbackURL(input string) (code, state, errMsg string, err error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("callback URL is required")
	}

	candidate := trimmed
	// 处理没有 scheme 的 URL
	if !strings.Contains(candidate, "://") {
		if strings.HasPrefix(candidate, "?") {
			candidate = "http://localhost" + candidate
		} else if strings.ContainsAny(candidate, "/?#") || strings.Contains(candidate, ":") {
			candidate = "http://" + candidate
		} else if strings.Contains(candidate, "=") {
			candidate = "http://localhost/?" + candidate
		} else {
			return "", "", "", fmt.Errorf("invalid callback URL")
		}
	}

	parsedURL, err := url.Parse(candidate)
	if err != nil {
		return "", "", "", err
	}

	query := parsedURL.Query()
	code = strings.TrimSpace(query.Get("code"))
	state = strings.TrimSpace(query.Get("state"))
	errMsg = strings.TrimSpace(query.Get("error"))
	if errMsg == "" {
		errMsg = strings.TrimSpace(query.Get("error_description"))
	}

	// 处理 fragment 中的参数
	if parsedURL.Fragment != "" {
		if fragQuery, errFrag := url.ParseQuery(parsedURL.Fragment); errFrag == nil {
			if code == "" {
				code = strings.TrimSpace(fragQuery.Get("code"))
			}
			if state == "" {
				state = strings.TrimSpace(fragQuery.Get("state"))
			}
			if errMsg == "" {
				errMsg = strings.TrimSpace(fragQuery.Get("error"))
				if errMsg == "" {
					errMsg = strings.TrimSpace(fragQuery.Get("error_description"))
				}
			}
		}
	}

	// 处理 code 中包含 # 分隔符的情况
	if code != "" && state == "" && strings.Contains(code, "#") {
		parts := strings.SplitN(code, "#", 2)
		code = parts[0]
		state = parts[1]
	}

	if code == "" && errMsg == "" {
		return "", "", "", fmt.Errorf("callback URL missing authorization code")
	}

	return code, state, errMsg, nil
}

func (s *CodexOAuthService) purgeExpiredLocked(now time.Time) {
	for state, session := range s.sessions {
		if !session.expiresAt.IsZero() && now.After(session.expiresAt) {
			delete(s.sessions, state)
		}
	}
}

func generatePKCECodes() (*PKCECodes, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}
	challenge := generateCodeChallenge(verifier)
	return &PKCECodes{
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
	}, nil
}

func generateCodeVerifier() (string, error) {
	bytes := make([]byte, 96)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func generateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func buildAuthURL(state string, pkce *PKCECodes) (string, error) {
	if pkce == nil {
		return "", errors.New("pkce codes are required")
	}
	params := url.Values{
		"client_id":                  {codexClientID},
		"response_type":              {"code"},
		"redirect_uri":               {codexRedirectURI},
		"scope":                      {codexScope},
		"state":                      {state},
		"code_challenge":             {pkce.CodeChallenge},
		"code_challenge_method":      {"S256"},
		"prompt":                     {"login"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
	}
	return fmt.Sprintf("%s?%s", codexAuthURL, params.Encode()), nil
}

func parseIDToken(token string) (string, string) {
	if token == "" {
		return "", ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ""
	}
	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return "", ""
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ""
	}
	return claims.Email, claims.Auth.ChatgptAccountID
}

func parsePlanFromIDToken(token string) string {
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return ""
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Auth.ChatgptPlanType
}

func base64URLDecode(data string) ([]byte, error) {
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}
	return base64.URLEncoding.DecodeString(data)
}

func writeCallbackHTML(w http.ResponseWriter, ok bool, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if ok {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(callbackSuccessHTML))
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	escaped := htmlEscape(errMsg)
	// 错误模板有两个 %s：一个用于 postMessage，一个用于显示
	_, _ = w.Write([]byte(fmt.Sprintf(callbackErrorHTML, escaped, escaped)))
}

func htmlEscape(input string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(input)
}

const callbackSuccessHTML = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>授权成功</title>
    <style>
      body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: linear-gradient(135deg, #10b981 0%, #059669 100%); }
      .container { text-align: center; background: white; padding: 2.5rem; border-radius: 12px; box-shadow: 0 10px 25px rgba(0,0,0,0.1); max-width: 400px; }
      .icon { width: 64px; height: 64px; margin: 0 auto 1.5rem; background: #10b981; border-radius: 50%; display: flex; align-items: center; justify-content: center; color: white; font-size: 2rem; }
      h1 { color: #1f2937; margin-bottom: 1rem; font-size: 1.5rem; }
      p { color: #6b7280; margin-bottom: 1rem; }
      .countdown { color: #9ca3af; font-size: 0.875rem; }
    </style>
    <script>
      // 通知父窗口授权成功
      if (window.opener) {
        window.opener.postMessage({ type: 'oauth_success', status: 'success' }, '*');
      }
      let countdown = 5;
      const timer = setInterval(() => {
        countdown--;
        document.getElementById('countdown').textContent = countdown;
        if (countdown <= 0) { clearInterval(timer); window.close(); }
      }, 1000);
    </script>
  </head>
  <body>
    <div class="container">
      <div class="icon">✓</div>
      <h1>授权成功！</h1>
      <p>您已成功完成 OAuth 授权，账号已激活。</p>
      <p class="countdown">窗口将在 <span id="countdown">5</span> 秒后自动关闭</p>
    </div>
  </body>
</html>`

const callbackErrorHTML = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>授权失败</title>
    <style>
      body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%); }
      .container { text-align: center; background: white; padding: 2.5rem; border-radius: 12px; box-shadow: 0 10px 25px rgba(0,0,0,0.1); max-width: 400px; }
      .icon { width: 64px; height: 64px; margin: 0 auto 1.5rem; background: #ef4444; border-radius: 50%; display: flex; align-items: center; justify-content: center; color: white; font-size: 2rem; }
      h1 { color: #1f2937; margin-bottom: 1rem; font-size: 1.5rem; }
      p { color: #6b7280; margin-bottom: 1rem; }
      .error { background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; padding: 1rem; color: #dc2626; font-size: 0.875rem; word-break: break-all; }
      button { margin-top: 1rem; padding: 0.75rem 1.5rem; background: #3b82f6; color: white; border: none; border-radius: 8px; cursor: pointer; }
      button:hover { background: #2563eb; }
    </style>
    <script>
      // 通知父窗口授权失败
      if (window.opener) {
        window.opener.postMessage({ type: 'oauth_error', status: 'failed', error: '%s' }, '*');
      }
    </script>
  </head>
  <body>
    <div class="container">
      <div class="icon">✗</div>
      <h1>授权失败</h1>
      <p>OAuth 授权过程中发生错误：</p>
      <div class="error">%s</div>
      <button onclick="window.close()">关闭窗口</button>
    </div>
  </body>
</html>`

func ioReadAll(r io.Reader) ([]byte, error) {
	const maxSize = 2 << 20
	var buf strings.Builder
	if _, err := io.CopyN(&buf, r, maxSize); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return []byte(buf.String()), nil
}
