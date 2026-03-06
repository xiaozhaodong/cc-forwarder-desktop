package accountauth

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestNormalizeProviderType(t *testing.T) {
	cases := map[string]string{
		"chatgpt_rt":      ProviderChatGPTRefreshToken,
		"refresh_token":   ProviderChatGPTRefreshToken,
		"oauth":           ProviderChatGPTRefreshToken,
		"api_key":         ProviderAPIKey,
		"bearer":          ProviderAPIKey,
		"session_cookie":  ProviderSessionCookie,
		"cookie":          ProviderSessionCookie,
		"custom-provider": "custom-provider",
	}

	for input, expected := range cases {
		if got := NormalizeProviderType(input); got != expected {
			t.Fatalf("NormalizeProviderType(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestInferProviderType(t *testing.T) {
	if got := InferProviderType(`{"refresh_token":"rt-abc"}`); got != ProviderChatGPTRefreshToken {
		t.Fatalf("expected refresh token provider, got %s", got)
	}
	if got := InferProviderType(`session=abc; __Secure-next=def`); got != ProviderSessionCookie {
		t.Fatalf("expected session cookie provider, got %s", got)
	}
	if got := InferProviderType(`sk-test`); got != ProviderAPIKey {
		t.Fatalf("expected api key provider, got %s", got)
	}
}

func TestApplyAccountAuth_SetsHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "https://example.com", nil)
	if err := ApplyAccountAuth(context.Background(), req, "bearer", "sk-test", nil); err != nil {
		t.Fatalf("ApplyAccountAuth(api_key) failed: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("unexpected authorization header: %s", got)
	}

	req2 := httptest.NewRequest("POST", "https://example.com", nil)
	if err := ApplyAccountAuth(context.Background(), req2, "cookie", "session=value", nil); err != nil {
		t.Fatalf("ApplyAccountAuth(cookie) failed: %v", err)
	}
	if got := req2.Header.Get("Cookie"); got != "session=value" {
		t.Fatalf("unexpected cookie header: %s", got)
	}
}
