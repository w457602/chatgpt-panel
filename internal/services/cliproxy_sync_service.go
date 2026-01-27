package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/webauto/chatgpt-panel/internal/models"
)

const (
	cliproxyBaseURLEnv      = "CLIPROXY_BASE_URL"
	cliproxyKeyEnv          = "CLIPROXY_MANAGEMENT_KEY"
	cliproxyPrefixEnv       = "CLIPROXY_AUTH_FILE_PREFIX"
	cliproxyTimeoutEnv      = "CLIPROXY_TIMEOUT_SECONDS"
	defaultCliproxyPrefix   = "codex"
	defaultCliproxyTimeout  = 15 * time.Second
	cliproxyAuthFilePattern = "%s-%s.json"
	cliproxyRemoteCacheTTL  = 30 * time.Second
)

type CliproxySyncService struct {
	baseURL       string
	managementKey string
	filePrefix    string
	timeout       time.Duration
	client        *http.Client
	enabled       bool
	remoteCacheMu sync.Mutex
	remoteCacheAt time.Time
	remoteCache   map[string]struct{}
}

var (
	cliproxyOnce    sync.Once
	cliproxyService *CliproxySyncService
)

func GetCliproxySyncService() *CliproxySyncService {
	cliproxyOnce.Do(func() {
		cliproxyService = NewCliproxySyncService()
	})
	return cliproxyService
}

func NewCliproxySyncService() *CliproxySyncService {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(cliproxyBaseURLEnv)), "/")
	key := strings.TrimSpace(os.Getenv(cliproxyKeyEnv))
	prefix := strings.TrimSpace(os.Getenv(cliproxyPrefixEnv))
	if prefix == "" {
		prefix = defaultCliproxyPrefix
	}

	timeout := defaultCliproxyTimeout
	if raw := strings.TrimSpace(os.Getenv(cliproxyTimeoutEnv)); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	return &CliproxySyncService{
		baseURL:       baseURL,
		managementKey: key,
		filePrefix:    prefix,
		timeout:       timeout,
		client:        &http.Client{Timeout: timeout},
		enabled:       baseURL != "" && key != "",
	}
}

func (s *CliproxySyncService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *CliproxySyncService) Enqueue(account *models.Account) {
	if s == nil || account == nil {
		return
	}
	accountCopy := *account
	go func() {
		if err := s.SyncAccount(context.Background(), &accountCopy); err != nil {
			log.Printf("cliproxy sync failed: %v", err)
		}
	}()
}

func (s *CliproxySyncService) SyncAccount(ctx context.Context, account *models.Account) error {
	if s == nil || !s.enabled || account == nil {
		return nil
	}
	if !s.Eligible(account) {
		return nil
	}
	if strings.TrimSpace(account.RefreshToken) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	payload, err := s.buildPayload(account)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cliproxy payload marshal failed: %w", err)
	}

	reqURL := fmt.Sprintf("%s/v0/management/auth-files?name=%s", s.baseURL, url.QueryEscape(s.buildFileName(account)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cliproxy request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.managementKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("cliproxy request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("cliproxy import failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	s.markSynced(account)
	s.cleanupLegacyFiles(ctx, account)
	s.invalidateRemoteCache()
	return nil
}

func (s *CliproxySyncService) Eligible(account *models.Account) bool {
	if account == nil {
		return false
	}
	status := ""
	if account.AccessToken != "" {
		status = ExtractSubscriptionStatusFromToken(account.AccessToken)
	}
	if status == "" {
		status = NormalizeSubscriptionStatus(account.SubscriptionStatus)
	}
	return IsCliproxyEligibleSubscription(status)
}

func (s *CliproxySyncService) markSynced(account *models.Account) {
	if account == nil || account.ID == 0 {
		return
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"cliproxy_synced":    true,
		"cliproxy_synced_at": &now,
	}
	if err := models.GetDB().Model(&models.Account{}).Where("id = ?", account.ID).Updates(updates).Error; err != nil {
		log.Printf("cliproxy sync update failed: %v", err)
		return
	}
	account.CliproxySynced = true
	account.CliproxySyncedAt = &now
}

func (s *CliproxySyncService) RemoteAuthFileNames(ctx context.Context) (map[string]struct{}, error) {
	if s == nil || !s.enabled {
		return nil, errors.New("cliproxy sync is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := time.Now()
	s.remoteCacheMu.Lock()
	if s.remoteCache != nil && now.Sub(s.remoteCacheAt) < cliproxyRemoteCacheTTL {
		cached := make(map[string]struct{}, len(s.remoteCache))
		for k := range s.remoteCache {
			cached[k] = struct{}{}
		}
		s.remoteCacheMu.Unlock()
		return cached, nil
	}
	s.remoteCacheMu.Unlock()

	reqURL := fmt.Sprintf("%s/v0/management/auth-files", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.managementKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("cliproxy list failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Files []struct {
			Name     string `json:"name"`
			ID       string `json:"id"`
			FileName string `json:"file_name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	names := make(map[string]struct{}, len(payload.Files))
	for _, f := range payload.Files {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			name = strings.TrimSpace(f.ID)
		}
		if name == "" {
			name = strings.TrimSpace(f.FileName)
		}
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}

	s.remoteCacheMu.Lock()
	s.remoteCache = names
	s.remoteCacheAt = time.Now()
	s.remoteCacheMu.Unlock()

	copied := make(map[string]struct{}, len(names))
	for k := range names {
		copied[k] = struct{}{}
	}
	return copied, nil
}

func (s *CliproxySyncService) RemoteHasAccount(remote map[string]struct{}, account *models.Account) bool {
	if len(remote) == 0 || account == nil {
		return false
	}
	for _, name := range s.expectedFileNames(account) {
		if name == "" {
			continue
		}
		if _, ok := remote[name]; ok {
			return true
		}
	}
	return false
}

func (s *CliproxySyncService) buildPayload(account *models.Account) (map[string]interface{}, error) {
	if account.Email == "" {
		return nil, errors.New("cliproxy payload missing email")
	}
	if account.AccountID == "" && account.AccessToken != "" {
		account.AccountID = extractAccountIDFromAccessToken(account.AccessToken)
	}
	payload := map[string]interface{}{
		"type":          "codex",
		"email":         account.Email,
		"refresh_token": account.RefreshToken,
	}
	if account.AccessToken != "" {
		payload["access_token"] = account.AccessToken
		// CLIProxyAPI UI parses id_token claims; reuse access_token JWT as a fallback.
		payload["id_token"] = account.AccessToken
	}
	if account.AccountID != "" {
		payload["account_id"] = account.AccountID
	} else {
		return nil, errors.New("cliproxy payload missing account_id")
	}
	if account.TokenExpired != nil {
		payload["expired"] = account.TokenExpired.UTC().Format(time.RFC3339)
	}

	lastRefresh := account.UpdatedAt
	if lastRefresh.IsZero() {
		lastRefresh = time.Now()
	}
	payload["last_refresh"] = lastRefresh.UTC().Format(time.RFC3339)

	return payload, nil
}

func (s *CliproxySyncService) expectedFileNames(account *models.Account) []string {
	if account == nil {
		return nil
	}
	current := s.buildFileName(account)
	names := []string{current}
	legacyID := s.buildLegacyAccountIDFileName(account)
	if legacyID != "" && legacyID != current {
		names = append(names, legacyID)
	}
	legacyEmail := s.buildLegacyEmailFileName(account)
	if legacyEmail != "" && legacyEmail != current {
		names = append(names, legacyEmail)
	}
	return names
}

func (s *CliproxySyncService) buildFileName(account *models.Account) string {
	base := strings.TrimSpace(account.Email)
	if base == "" {
		base = strings.TrimSpace(account.AccountID)
	}
	if base == "" {
		base = fmt.Sprintf("account-%d", account.ID)
	}
	base = sanitizeFileName(base)
	return fmt.Sprintf(cliproxyAuthFilePattern, s.filePrefix, base)
}

func (s *CliproxySyncService) buildLegacyAccountIDFileName(account *models.Account) string {
	if account == nil {
		return ""
	}
	base := strings.TrimSpace(account.AccountID)
	if base == "" {
		return ""
	}
	base = sanitizeFileName(base)
	return fmt.Sprintf(cliproxyAuthFilePattern, s.filePrefix, base)
}

func (s *CliproxySyncService) buildLegacyEmailFileName(account *models.Account) string {
	if account == nil {
		return ""
	}
	base := strings.TrimSpace(account.Email)
	if base == "" {
		return ""
	}
	base = sanitizeFileNameLegacy(base)
	return fmt.Sprintf(cliproxyAuthFilePattern, s.filePrefix, base)
}

func sanitizeFileName(input string) string {
	if input == "" {
		return "account"
	}
	var b strings.Builder
	for _, r := range input {
		if r > unicode.MaxASCII {
			b.WriteByte('_')
			continue
		}
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '@' || r == '+' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "account"
	}
	return out
}

func sanitizeFileNameLegacy(input string) string {
	if input == "" {
		return "account"
	}
	var b strings.Builder
	for _, r := range input {
		if r > unicode.MaxASCII {
			b.WriteByte('_')
			continue
		}
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "account"
	}
	return out
}

func (s *CliproxySyncService) cleanupLegacyFiles(ctx context.Context, account *models.Account) {
	if account == nil || s == nil || !s.enabled {
		return
	}
	current := s.buildFileName(account)
	legacyNames := []string{
		s.buildLegacyAccountIDFileName(account),
		s.buildLegacyEmailFileName(account),
	}
	for _, name := range legacyNames {
		if name == "" || name == current {
			continue
		}
		if err := s.deleteRemoteAuthFile(ctx, name); err != nil {
			log.Printf("cliproxy cleanup failed for %s: %v", name, err)
		}
	}
}

func (s *CliproxySyncService) deleteRemoteAuthFile(ctx context.Context, name string) error {
	if s == nil || !s.enabled {
		return errors.New("cliproxy sync is not configured")
	}
	if name == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqURL := fmt.Sprintf("%s/v0/management/auth-files?name=%s", s.baseURL, url.QueryEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.managementKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("cliproxy delete failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *CliproxySyncService) invalidateRemoteCache() {
	if s == nil {
		return
	}
	s.remoteCacheMu.Lock()
	s.remoteCache = nil
	s.remoteCacheAt = time.Time{}
	s.remoteCacheMu.Unlock()
}

func extractAccountIDFromAccessToken(token string) string {
	claims := parseJWTClaims(token)
	if claims == nil {
		return ""
	}
	if v, ok := claims["chatgpt_account_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := claims["account_id"].(string); ok && v != "" {
		return v
	}
	if authClaim, ok := claims["https://api.openai.com/auth"]; ok {
		if accountID := extractAccountIDFromAuthClaim(authClaim); accountID != "" {
			return accountID
		}
	}
	return ""
}

func extractAccountIDFromAuthClaim(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}:
		return stringValue(v["chatgpt_account_id"])
	case string:
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return stringValue(parsed["chatgpt_account_id"])
		}
	}
	return ""
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func parseJWTClaims(token string) map[string]interface{} {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return claims
}

func decodeJWTPart(part string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(part)
	if err == nil {
		return decoded, nil
	}
	padding := (4 - len(part)%4) % 4
	if padding > 0 {
		part += strings.Repeat("=", padding)
	}
	return base64.URLEncoding.DecodeString(part)
}
