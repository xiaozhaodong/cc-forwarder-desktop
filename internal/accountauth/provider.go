package accountauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const (
	ProviderChatGPTRefreshToken = "chatgpt_refresh_token"
	ProviderAPIKey              = "api_key"
	ProviderSessionCookie       = "session_cookie"
)

// NormalizeProviderType 统一 provider_type 的别名写法。
func NormalizeProviderType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "chatgpt_refresh_token", "chatgpt_rt", "refresh_token", "rt", "oauth", "openai_oauth":
		return ProviderChatGPTRefreshToken
	case "api_key", "apikey", "token", "bearer":
		return ProviderAPIKey
	case "session", "session_cookie", "cookie":
		return ProviderSessionCookie
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// InferProviderType 根据凭据内容推断 provider_type。
func InferProviderType(credential string) string {
	lower := strings.ToLower(strings.TrimSpace(credential))
	if strings.HasPrefix(lower, "rt-") || strings.HasPrefix(lower, "rt_") ||
		strings.Contains(lower, "refresh_token=") || strings.Contains(lower, "\"refresh_token\"") {
		return ProviderChatGPTRefreshToken
	}
	if strings.Contains(lower, "=") && (strings.Contains(lower, "session") || strings.Contains(lower, "__secure") || strings.Contains(lower, ";")) {
		return ProviderSessionCookie
	}
	return ProviderAPIKey
}

// IsChatGPTOAuthProvider 判断 provider 是否走 ChatGPT OAuth/refresh token 链路。
func IsChatGPTOAuthProvider(providerType string) bool {
	return NormalizeProviderType(providerType) == ProviderChatGPTRefreshToken
}

// ApplyAccountAuth 根据 provider 类型设置账号鉴权头。
func ApplyAccountAuth(ctx context.Context, req *http.Request, providerType, credentialRaw string, manager *OpenAIRefreshTokenManager) error {
	cleanCredential := strings.TrimSpace(credentialRaw)
	switch NormalizeProviderType(providerType) {
	case ProviderChatGPTRefreshToken:
		if manager == nil {
			return fmt.Errorf("refresh token manager is not initialized")
		}
		resolved, err := manager.ResolveAccessTokenDetails(ctx, cleanCredential)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+resolved.AccessToken)
		if resolved.ChatGPTAccountID != "" {
			req.Header.Set("chatgpt-account-id", resolved.ChatGPTAccountID)
		}
		return nil
	case ProviderSessionCookie:
		req.Header.Set("Cookie", cleanCredential)
		return nil
	default:
		if strings.HasPrefix(strings.ToLower(cleanCredential), "bearer ") {
			req.Header.Set("Authorization", cleanCredential)
		} else {
			req.Header.Set("Authorization", "Bearer "+cleanCredential)
		}
		return nil
	}
}
