package services

import (
	"encoding/json"
	"strings"
)

const (
	subscriptionFree     = "free"
	subscriptionPlus     = "plus"
	subscriptionTeam     = "team"
	subscriptionBusiness = "business"
)

func NormalizeSubscriptionStatus(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "":
		return ""
	case "chatgptteamplan":
		return subscriptionTeam
	}
	return value
}

func ExtractSubscriptionStatusFromToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	claims := parseJWTClaims(token)
	if claims == nil {
		return ""
	}
	if v, ok := claims["chatgpt_plan_type"].(string); ok {
		if plan := NormalizeSubscriptionStatus(v); plan != "" {
			return plan
		}
	}
	if authClaim, ok := claims["https://api.openai.com/auth"]; ok {
		if plan := extractPlanTypeFromAuthClaim(authClaim); plan != "" {
			return NormalizeSubscriptionStatus(plan)
		}
	}
	return ""
}

func extractPlanTypeFromAuthClaim(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}:
		if plan := stringValue(v["chatgpt_plan_type"]); plan != "" {
			return plan
		}
		return stringValue(v["plan_type"])
	case string:
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			if plan := stringValue(parsed["chatgpt_plan_type"]); plan != "" {
				return plan
			}
			return stringValue(parsed["plan_type"])
		}
	}
	return ""
}

func IsCliproxyEligibleSubscription(status string) bool {
	switch NormalizeSubscriptionStatus(status) {
	case subscriptionPlus, subscriptionTeam, subscriptionBusiness:
		return true
	default:
		return false
	}
}
