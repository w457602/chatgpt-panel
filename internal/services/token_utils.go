package services

import "strings"

// ExtractAccountIDFromToken 从 access_token 里提取 chatgpt_account_id
func ExtractAccountIDFromToken(token string) string {
	claims := parseJWTClaims(token)
	if claims == nil {
		return ""
	}
	if v, ok := claims["chatgpt_account_id"].(string); ok && v != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := claims["account_id"].(string); ok && v != "" {
		return strings.TrimSpace(v)
	}
	if authClaim, ok := claims["https://api.openai.com/auth"]; ok {
		if accountID := extractAccountIDFromAuthClaim(authClaim); accountID != "" {
			return accountID
		}
	}
	return ""
}
