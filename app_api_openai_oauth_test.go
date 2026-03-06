package main

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"cc-forwarder/internal/accountauth"
)

func TestBuildOpenAIOAuthURL(t *testing.T) {
	state := "state-123"
	verifier := "verifier-abc"
	redirectURI := "http://localhost:1455/auth/callback"

	authURL := buildOpenAIOAuthURL(state, verifier, redirectURI)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse authURL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "auth.openai.com" {
		t.Fatalf("unexpected auth host: %s", parsed.Host)
	}

	query := parsed.Query()
	if query.Get("client_id") != openAIOAuthClientID {
		t.Fatalf("unexpected client_id: %s", query.Get("client_id"))
	}
	if query.Get("state") != state {
		t.Fatalf("unexpected state: %s", query.Get("state"))
	}
	if query.Get("redirect_uri") != redirectURI {
		t.Fatalf("unexpected redirect_uri: %s", query.Get("redirect_uri"))
	}
	scope := query.Get("scope")
	if !strings.Contains(scope, "offline_access") {
		t.Fatalf("scope should include offline_access, got: %s", scope)
	}
	if strings.Contains(scope, "api.responses.write") {
		t.Fatalf("scope should not include api.responses.write for current oauth client, got: %s", scope)
	}
	if query.Get("code_challenge") == "" {
		t.Fatal("code_challenge should not be empty")
	}
}

func TestExchangeChatGPTOAuthCallback_OAuthErrorFromQuery(t *testing.T) {
	app := NewApp()
	app.oauthSessions = map[string]openAIOAuthSession{
		"sess-1": {
			State:        "state-1",
			CodeVerifier: "verifier-1",
			RedirectURI:  openAIOAuthDefaultRedirectURI,
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		},
	}

	_, err := app.ExchangeChatGPTOAuthCallback(ExchangeChatGPTOAuthCallbackInput{
		SessionID:   "sess-1",
		CallbackURL: "http://localhost:1455/auth/callback?error=invalid_scope&error_description=scope+not+allowed&state=state-1",
	})
	if err == nil {
		t.Fatal("expected oauth error")
	}
	errText := err.Error()
	if !strings.Contains(errText, "OAuth 授权失败: invalid_scope") {
		t.Fatalf("unexpected oauth error: %s", errText)
	}
	if !strings.Contains(errText, "不允许该 scope") {
		t.Fatalf("expected invalid_scope hint, got: %s", errText)
	}
}

func TestSha256CodeChallenge(t *testing.T) {
	challenge := sha256CodeChallenge("abc")
	if challenge == "" {
		t.Fatal("challenge should not be empty")
	}
	if strings.Contains(challenge, "=") {
		t.Fatalf("challenge should not include base64 padding: %s", challenge)
	}
}

func TestBuildOAuthCredentialResult_IncludesAccountIDAndCredentialRaw(t *testing.T) {
	idToken := testOAuthIDToken("acc-test-1")
	result := buildOAuthCredentialResult(&openAIOAuthTokenResponse{
		AccessToken:  "at-1",
		IDToken:      idToken,
		RefreshToken: "rt-1",
		ExpiresIn:    3600,
	}, "ok")

	if !result.Success {
		t.Fatal("expected success=true")
	}
	if result.ChatGPTAccountID != "acc-test-1" {
		t.Fatalf("unexpected account id: %s", result.ChatGPTAccountID)
	}
	if !strings.Contains(result.CredentialRaw, `"refresh_token":"rt-1"`) {
		t.Fatalf("credential_raw should contain refresh token, got: %s", result.CredentialRaw)
	}
	if accountauth.ExtractChatGPTAccountID(result.CredentialRaw) != "acc-test-1" {
		t.Fatalf("credential_raw should expose chatgpt_account_id, got: %s", result.CredentialRaw)
	}
}

func TestBuildOAuthCredentialResultFromValues_ParsesDirectTokenPayload(t *testing.T) {
	values := url.Values{}
	values.Set("refresh_token", "rt-direct")
	values.Set("access_token", "at-direct")
	values.Set("id_token", testOAuthIDToken("acc-direct"))
	values.Set("expires_in", "1800")

	result, ok := buildOAuthCredentialResultFromValues(values, "direct")
	if !ok {
		t.Fatal("expected result to be built from direct token payload")
	}
	if result.ChatGPTAccountID != "acc-direct" {
		t.Fatalf("unexpected account id: %s", result.ChatGPTAccountID)
	}
	if !strings.Contains(result.CredentialRaw, `"access_token":"at-direct"`) {
		t.Fatalf("credential_raw should contain access token, got: %s", result.CredentialRaw)
	}
}

func testOAuthIDToken(accountID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"` + accountID + `"}}`))
	return header + "." + payload + ".signature"
}
