package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const inviteTimeout = 15 * time.Second

type InviteService struct {
	httpClient *http.Client
}

type InviteResult struct {
	Success    bool   `json:"success"`
	InviteID   string `json:"invite_id,omitempty"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message,omitempty"`
}

type TeamMember struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type MembersResult struct {
	Success    bool
	StatusCode int
	Message    string
	Members    []TeamMember
}

type PendingInvite struct {
	EmailAddress string `json:"email_address"`
}

type PendingInvitesResult struct {
	Success    bool
	StatusCode int
	Message    string
	Invites    []PendingInvite
}

func NewInviteService() *InviteService {
	return &InviteService{
		httpClient: &http.Client{Timeout: inviteTimeout},
	}
}

func applyCommonHeaders(req *http.Request, accessToken, accountID, referer string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("sec-ch-ua", "\"Chromium\";v=\"120\", \"Not.A/Brand\";v=\"24\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"Windows\"")
}

func (s *InviteService) SendInvite(ctx context.Context, accessToken, accountID, email string) (*InviteResult, error) {
	if accessToken == "" || accountID == "" || email == "" {
		return nil, fmt.Errorf("missing access_token/account_id/email")
	}

	payload := map[string]interface{}{
		"email_addresses": []string{email},
		"role":            "standard-user",
		"resend_emails":   false,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("https://chatgpt.com/backend-api/accounts/%s/invites", accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	applyCommonHeaders(req, accessToken, accountID, "https://chatgpt.com/admin")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		var parsed struct {
			AccountInvites []struct {
				ID string `json:"id"`
			} `json:"account_invites"`
		}
		inviteID := ""
		if json.Unmarshal(raw, &parsed) == nil && len(parsed.AccountInvites) > 0 {
			inviteID = parsed.AccountInvites[0].ID
		}
		return &InviteResult{
			Success:    true,
			InviteID:   inviteID,
			StatusCode: resp.StatusCode,
		}, nil
	}

	message := strings.TrimSpace(string(raw))
	if len(message) > 500 {
		message = message[:500]
	}

	return &InviteResult{
		Success:    false,
		StatusCode: resp.StatusCode,
		Message:    message,
	}, nil
}

func (s *InviteService) GetMembers(ctx context.Context, accessToken, accountID string) (*MembersResult, error) {
	if accessToken == "" || accountID == "" {
		return nil, fmt.Errorf("missing access_token/account_id")
	}

	url := fmt.Sprintf("https://chatgpt.com/backend-api/accounts/%s/users", accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	applyCommonHeaders(req, accessToken, accountID, "https://chatgpt.com/admin")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusOK {
		var parsed struct {
			Items []TeamMember `json:"items"`
		}
		if json.Unmarshal(raw, &parsed) != nil {
			return &MembersResult{
				Success:    false,
				StatusCode: resp.StatusCode,
				Message:    "解析成员列表失败",
			}, nil
		}
		return &MembersResult{
			Success:    true,
			StatusCode: resp.StatusCode,
			Members:    parsed.Items,
		}, nil
	}

	message := strings.TrimSpace(string(raw))
	if len(message) > 500 {
		message = message[:500]
	}

	return &MembersResult{
		Success:    false,
		StatusCode: resp.StatusCode,
		Message:    message,
	}, nil
}

func (s *InviteService) GetPendingInvites(ctx context.Context, accessToken, accountID string) (*PendingInvitesResult, error) {
	if accessToken == "" || accountID == "" {
		return nil, fmt.Errorf("missing access_token/account_id")
	}

	url := fmt.Sprintf("https://chatgpt.com/backend-api/accounts/%s/invites", accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	applyCommonHeaders(req, accessToken, accountID, "https://chatgpt.com/admin")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusOK {
		var parsed struct {
			Items []PendingInvite `json:"items"`
		}
		if json.Unmarshal(raw, &parsed) != nil {
			return &PendingInvitesResult{
				Success:    false,
				StatusCode: resp.StatusCode,
				Message:    "解析邀请列表失败",
			}, nil
		}
		return &PendingInvitesResult{
			Success:    true,
			StatusCode: resp.StatusCode,
			Invites:    parsed.Items,
		}, nil
	}

	message := strings.TrimSpace(string(raw))
	if len(message) > 500 {
		message = message[:500]
	}

	return &PendingInvitesResult{
		Success:    false,
		StatusCode: resp.StatusCode,
		Message:    message,
	}, nil
}
