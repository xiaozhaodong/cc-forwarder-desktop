package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cc-forwarder/internal/accountauth"
	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

func TestRefreshAccountProfile_FreeAccountUsesWeeklyQuotaOnly(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-free",
			"refresh_token": "rt-free-rotated",
			"expires_in":    3600,
			"id_token":      testServiceIDTokenWithProfile("free", "acc-free", "user-free", "org-free"),
		})
	}))
	defer authServer.Close()

	var receivedAccountHeader string
	quotaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccountHeader = r.Header.Get("chatgpt-account-id")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type": "free",
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent":        12.5,
					"reset_after_seconds": 600,
				},
			},
		})
	}))
	defer quotaServer.Close()

	svc, st, accountID := newOAuthProfileTestService(t, accountauth.BuildStoredOpenAICredential(
		"rt-free",
		"",
		"",
		accountauth.OpenAIAccountProfile{},
		time.Time{},
	))

	oldRefreshURL := accountauthOpenAIRefreshURL()
	oldQuotaURL := chatGPTWhamUsageURL
	t.Cleanup(func() {
		setAccountauthOpenAIRefreshURL(oldRefreshURL)
		chatGPTWhamUsageURL = oldQuotaURL
	})
	setAccountauthOpenAIRefreshURL(authServer.URL)
	chatGPTWhamUsageURL = quotaServer.URL

	result, err := svc.RefreshAccountProfile(context.Background(), accountID)
	if err != nil {
		t.Fatalf("RefreshAccountProfile failed: %v", err)
	}
	if !result.Success || result.QuotaStatus != quotaStatusOK {
		t.Fatalf("unexpected refresh result: %+v", result)
	}

	rec, err := st.GetAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if rec.PlanType != "free" {
		t.Fatalf("unexpected plan type: %s", rec.PlanType)
	}
	if rec.ChatGPTAccountID != "acc-free" || rec.ChatGPTUserID != "user-free" || rec.OrganizationID != "org-free" {
		t.Fatalf("unexpected profile: %+v", rec)
	}
	if rec.Quota5HUsedPercent != nil || rec.Quota5HResetAt != nil {
		t.Fatalf("free account should not have 5h quota: %+v", rec)
	}
	if rec.QuotaWeeklyUsedPercent == nil || *rec.QuotaWeeklyUsedPercent != 12.5 {
		t.Fatalf("unexpected weekly quota: %+v", rec.QuotaWeeklyUsedPercent)
	}
	if rec.QuotaStatus != quotaStatusOK || rec.QuotaRefreshedAt == nil {
		t.Fatalf("unexpected quota status: %+v", rec)
	}
	if !strings.Contains(rec.CredentialRaw, `"refresh_token":"rt-free-rotated"`) {
		t.Fatalf("credential should persist rotated refresh token: %s", rec.CredentialRaw)
	}
	if receivedAccountHeader != "" {
		t.Fatalf("free account should not send chatgpt-account-id header, got: %s", receivedAccountHeader)
	}
}

func TestRefreshAccountProfile_FreeAccountFallsBackToStoredPlanTypeWhenQuotaPayloadOmitsPlanType(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-free",
			"refresh_token": "rt-free-rotated",
			"expires_in":    3600,
			"id_token":      testServiceIDTokenWithProfile("free", "acc-free", "user-free", "org-free"),
		})
	}))
	defer authServer.Close()

	quotaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent":        9,
					"reset_after_seconds": 600,
				},
			},
		})
	}))
	defer quotaServer.Close()

	svc, st, accountID := newOAuthProfileTestService(t, accountauth.BuildStoredOpenAICredential(
		"rt-free",
		"",
		"",
		accountauth.OpenAIAccountProfile{
			PlanType:         "free",
			ChatGPTAccountID: "acc-free",
		},
		time.Time{},
	))

	oldRefreshURL := accountauthOpenAIRefreshURL()
	oldQuotaURL := chatGPTWhamUsageURL
	t.Cleanup(func() {
		setAccountauthOpenAIRefreshURL(oldRefreshURL)
		chatGPTWhamUsageURL = oldQuotaURL
	})
	setAccountauthOpenAIRefreshURL(authServer.URL)
	chatGPTWhamUsageURL = quotaServer.URL

	result, err := svc.RefreshAccountProfile(context.Background(), accountID)
	if err != nil {
		t.Fatalf("RefreshAccountProfile failed: %v", err)
	}
	if !result.Success || result.QuotaStatus != quotaStatusOK {
		t.Fatalf("unexpected refresh result: %+v", result)
	}

	rec, err := st.GetAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if rec.Quota5HUsedPercent != nil || rec.Quota5HResetAt != nil {
		t.Fatalf("free account should not have 5h quota after fallback normalization: %+v", rec)
	}
	if rec.QuotaWeeklyUsedPercent == nil || *rec.QuotaWeeklyUsedPercent != 9 {
		t.Fatalf("weekly quota should be recovered from primary window: %+v", rec.QuotaWeeklyUsedPercent)
	}
	if rec.QuotaWeeklyResetAt == nil {
		t.Fatalf("weekly reset time should be recovered from primary window")
	}
}

func TestRefreshAccountProfile_BodyPriorityAndHeaderFallback(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-team",
			"refresh_token": "rt-team",
			"expires_in":    3600,
			"id_token":      testServiceIDTokenWithProfile("team", "acc-team", "user-team", "org-team"),
		})
	}))
	defer authServer.Close()

	var receivedAccountHeader string
	quotaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccountHeader = r.Header.Get("chatgpt-account-id")
		w.Header().Set("x-codex-plan-type", "team")
		w.Header().Set("x-codex-primary-used-percent", "99")
		w.Header().Set("x-codex-primary-reset-after-seconds", "600")
		w.Header().Set("x-codex-secondary-used-percent", "88")
		w.Header().Set("x-codex-secondary-reset-after-seconds", "3600")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type": "team",
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 40,
				},
				"secondary_window": map[string]any{
					"used_percent": 70,
				},
			},
		})
	}))
	defer quotaServer.Close()

	svc, st, accountID := newOAuthProfileTestService(t, accountauth.BuildStoredOpenAICredential(
		"rt-team",
		"",
		"",
		accountauth.OpenAIAccountProfile{
			PlanType:         "team",
			ChatGPTAccountID: "acc-team",
		},
		time.Time{},
	))

	oldRefreshURL := accountauthOpenAIRefreshURL()
	oldQuotaURL := chatGPTWhamUsageURL
	t.Cleanup(func() {
		setAccountauthOpenAIRefreshURL(oldRefreshURL)
		chatGPTWhamUsageURL = oldQuotaURL
	})
	setAccountauthOpenAIRefreshURL(authServer.URL)
	chatGPTWhamUsageURL = quotaServer.URL

	result, err := svc.RefreshAccountProfile(context.Background(), accountID)
	if err != nil {
		t.Fatalf("RefreshAccountProfile failed: %v", err)
	}
	if !result.Success || result.QuotaStatus != quotaStatusOK {
		t.Fatalf("unexpected refresh result: %+v", result)
	}

	rec, err := st.GetAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if rec.Quota5HUsedPercent == nil || *rec.Quota5HUsedPercent != 40 {
		t.Fatalf("body value should win for 5h quota, got %+v", rec.Quota5HUsedPercent)
	}
	if rec.QuotaWeeklyUsedPercent == nil || *rec.QuotaWeeklyUsedPercent != 70 {
		t.Fatalf("body value should win for weekly quota, got %+v", rec.QuotaWeeklyUsedPercent)
	}
	if rec.Quota5HResetAt == nil || rec.QuotaWeeklyResetAt == nil {
		t.Fatalf("header fallback should populate reset_at values: %+v", rec)
	}
	if receivedAccountHeader != "acc-team" {
		t.Fatalf("expected chatgpt-account-id header, got %s", receivedAccountHeader)
	}
}

func TestTestUpstreamAccount_ReachableStillTriggersProfileRefresh(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-oauth",
			"refresh_token": "rt-oauth",
			"expires_in":    3600,
			"id_token":      testServiceIDTokenWithProfile("plus", "acc-oauth", "user-oauth", "org-oauth"),
		})
	}))
	defer authServer.Close()

	responsesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"missing required parameter: model"}`, http.StatusBadRequest)
	}))
	defer responsesServer.Close()

	quotaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"upstream unavailable"}`, http.StatusInternalServerError)
	}))
	defer quotaServer.Close()

	svc, st, accountID := newOAuthProfileTestService(t, accountauth.BuildStoredOpenAICredential(
		"rt-oauth",
		"",
		"",
		accountauth.OpenAIAccountProfile{
			PlanType:         "plus",
			ChatGPTAccountID: "acc-oauth",
		},
		time.Time{},
	))

	oldRefreshURL := accountauthOpenAIRefreshURL()
	oldQuotaURL := chatGPTWhamUsageURL
	oldTestURL := chatGPTCodexTestURL
	t.Cleanup(func() {
		setAccountauthOpenAIRefreshURL(oldRefreshURL)
		chatGPTWhamUsageURL = oldQuotaURL
		chatGPTCodexTestURL = oldTestURL
	})
	setAccountauthOpenAIRefreshURL(authServer.URL)
	chatGPTWhamUsageURL = quotaServer.URL
	chatGPTCodexTestURL = responsesServer.URL

	if err := svc.TestUpstreamAccount(context.Background(), accountID); err != nil {
		t.Fatalf("TestUpstreamAccount should treat reachable OAuth response as success, got %v", err)
	}

	rec, err := st.GetAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if rec.QuotaStatus != quotaStatusUnavailable {
		t.Fatalf("auto refresh should mark quota unavailable, got %s", rec.QuotaStatus)
	}
	if rec.QuotaRefreshedAt == nil {
		t.Fatalf("auto refresh should write quota_refreshed_at")
	}
}

func newOAuthProfileTestService(t *testing.T, credentialRaw string) (*AccountPoolService, store.AccountPoolStore, int64) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schemaPath := filepath.Join("..", "tracking", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema failed: %v", err)
	}

	st := store.NewSQLiteAccountPoolStore(db)
	rec, err := st.CreateAccount(context.Background(), &store.UpstreamAccountRecord{
		ProviderType:  accountauth.ProviderChatGPTRefreshToken,
		AccountName:   "oauth-test",
		CredentialRaw: credentialRaw,
		BaseURL:       "https://api.openai.com",
		Priority:      1,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	return NewAccountPoolService(st, nil), st, rec.ID
}

func testServiceIDTokenWithProfile(planType, accountID, userID, organizationID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"` + accountID + `","chatgpt_plan_type":"` + planType + `","chatgpt_user_id":"` + userID + `","organization_id":"` + organizationID + `"}}`))
	return header + "." + payload + ".signature"
}

func accountauthOpenAIRefreshURL() string {
	return accountauth.CurrentOpenAIRefreshTokenURLForTest()
}

func setAccountauthOpenAIRefreshURL(value string) {
	accountauth.SetOpenAIRefreshTokenURLForTest(value)
}
