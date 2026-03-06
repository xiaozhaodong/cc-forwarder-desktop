package accountauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExtractRefreshToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "json refresh token",
			in:   `{"refresh_token":"rt-json"}`,
			want: "rt-json",
		},
		{
			name: "kv refresh token",
			in:   "refresh_token=rt-kv",
			want: "rt-kv",
		},
		{
			name: "rt shorthand",
			in:   "rt=rt-short",
			want: "rt-short",
		},
		{
			name: "raw token",
			in:   "rt-raw",
			want: "rt-raw",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := extractRefreshToken(tc.in); got != tc.want {
				t.Fatalf("extractRefreshToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractChatGPTAccountID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "json chatgpt account id",
			in:   `{"refresh_token":"rt-json","chatgpt_account_id":"acc-123"}`,
			want: "acc-123",
		},
		{
			name: "json nested account id",
			in:   `{"credentials":{"account_id":"acc-nested"}}`,
			want: "acc-nested",
		},
		{
			name: "kv account id",
			in:   "refresh_token=rt-kv;chatgpt_account_id=acc-kv",
			want: "acc-kv",
		},
		{
			name: "id token account id",
			in:   `{"refresh_token":"rt-json","id_token":"` + testIDTokenForAccount("acc-from-id-token") + `"}`,
			want: "acc-from-id-token",
		},
		{
			name: "no account id",
			in:   "rt-only",
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractChatGPTAccountID(tc.in); got != tc.want {
				t.Fatalf("ExtractChatGPTAccountID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractChatGPTAccountIDFromIDToken(t *testing.T) {
	idToken := testIDTokenForAccount("acc-from-id-token")
	if got := ExtractChatGPTAccountIDFromIDToken(idToken); got != "acc-from-id-token" {
		t.Fatalf("ExtractChatGPTAccountIDFromIDToken() = %q, want %q", got, "acc-from-id-token")
	}
}

func TestResolveAccessToken_UsesStoredAccessTokenWhenFresh(t *testing.T) {
	manager := NewOpenAIRefreshTokenManager(nil)
	expiresAt := time.Now().Add(30 * time.Minute).Format(time.RFC3339)

	token, err := manager.ResolveAccessToken(context.Background(), `{"refresh_token":"rt-stored","access_token":"at-stored","expires_at":"`+expiresAt+`"}`)
	if err != nil {
		t.Fatalf("ResolveAccessToken failed: %v", err)
	}
	if token != "at-stored" {
		t.Fatalf("unexpected access token: %s", token)
	}
}

func TestResolveAccessToken_Cache(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)

		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form failed: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Fatalf("unexpected grant_type: %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "rt-cache" {
			t.Fatalf("unexpected refresh_token: %s", r.Form.Get("refresh_token"))
		}
		if r.Form.Get("client_id") == "" {
			t.Fatal("client_id should not be empty")
		}
		if r.Form.Get("scope") != "" {
			t.Fatalf("scope should be omitted on refresh, got: %s", r.Form.Get("scope"))
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-cached",
			"refresh_token": "rt-cache",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	oldURL := openAIRefreshTokenURL
	openAIRefreshTokenURL = server.URL
	defer func() {
		openAIRefreshTokenURL = oldURL
	}()

	manager := NewOpenAIRefreshTokenManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := manager.ResolveAccessToken(ctx, "rt-cache")
	if err != nil {
		t.Fatalf("ResolveAccessToken first call failed: %v", err)
	}
	if first != "at-cached" {
		t.Fatalf("unexpected access token: %s", first)
	}

	second, err := manager.ResolveAccessToken(ctx, "refresh_token=rt-cache")
	if err != nil {
		t.Fatalf("ResolveAccessToken second call failed: %v", err)
	}
	if second != "at-cached" {
		t.Fatalf("unexpected cached access token: %s", second)
	}

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected one refresh request, got %d", got)
	}
}

func TestResolveAccessToken_LargeResponseBody(t *testing.T) {
	largeIDToken := strings.Repeat("x", 12*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-large",
			"refresh_token": "rt-large",
			"expires_in":    3600,
			"id_token":      largeIDToken,
		})
	}))
	defer server.Close()

	oldURL := openAIRefreshTokenURL
	openAIRefreshTokenURL = server.URL
	defer func() {
		openAIRefreshTokenURL = oldURL
	}()

	manager := NewOpenAIRefreshTokenManager(nil)
	token, err := manager.ResolveAccessToken(context.Background(), "rt-large")
	if err != nil {
		t.Fatalf("ResolveAccessToken failed: %v", err)
	}
	if token != "at-large" {
		t.Fatalf("unexpected access token: %s", token)
	}
}

func TestResolveAccessToken_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	oldURL := openAIRefreshTokenURL
	openAIRefreshTokenURL = server.URL
	defer func() {
		openAIRefreshTokenURL = oldURL
	}()

	manager := NewOpenAIRefreshTokenManager(nil)
	_, err := manager.ResolveAccessToken(context.Background(), "rt-invalid")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveAccessTokenDetails_ExtractsAccountIDFromIDToken(t *testing.T) {
	idToken := testIDTokenForAccount("acc-detail")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-detail",
			"refresh_token": "rt-detail-rotated",
			"expires_in":    3600,
			"id_token":      idToken,
		})
	}))
	defer server.Close()

	oldURL := openAIRefreshTokenURL
	openAIRefreshTokenURL = server.URL
	defer func() {
		openAIRefreshTokenURL = oldURL
	}()

	manager := NewOpenAIRefreshTokenManager(nil)
	details, err := manager.ResolveAccessTokenDetails(context.Background(), "rt-detail")
	if err != nil {
		t.Fatalf("ResolveAccessTokenDetails failed: %v", err)
	}
	if details.AccessToken != "at-detail" {
		t.Fatalf("unexpected access token: %s", details.AccessToken)
	}
	if details.RefreshToken != "rt-detail-rotated" {
		t.Fatalf("unexpected refresh token: %s", details.RefreshToken)
	}
	if details.ChatGPTAccountID != "acc-detail" {
		t.Fatalf("unexpected account id: %s", details.ChatGPTAccountID)
	}
	if details.IDToken != idToken {
		t.Fatalf("unexpected id token: %s", details.IDToken)
	}
}

func testIDTokenForAccount(accountID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"` + accountID + `"}}`))
	return header + "." + payload + ".signature"
}
