package service

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

func TestTestUpstreamAccount_UsesResponsesEndpointAndTreats400AsReachable(t *testing.T) {
	var receivedMethod string
	var receivedPath string
	var receivedAuth string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		receivedBody = string(raw)

		http.Error(w, `{"error":"missing required parameter: model"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("expected nil error for 400 reachability, got %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/v1/responses" {
		t.Fatalf("expected /v1/responses, got %s", receivedPath)
	}
	if receivedAuth != "Bearer sk-test" {
		t.Fatalf("expected bearer auth, got %s", receivedAuth)
	}
	if receivedBody == "" {
		t.Fatal("expected non-empty request body")
	}

	account, err := svc.GetAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if account == nil || account.LastSuccessAt == nil || account.LastSuccessAt.IsZero() {
		t.Fatalf("expected last_success_at to be updated after connectivity test, got %+v", account)
	}
}

func TestTestUpstreamAccount_ReturnsPermissionErrorOn403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"insufficient scope"}`, http.StatusForbidden)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "鉴权或权限不足") || !strings.Contains(got, "403") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestTestUpstreamAccount_MissingResponsesWriteScopeHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Missing scopes: api.responses.write"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err == nil {
		t.Fatal("expected error for missing responses scope")
	}
	msg := err.Error()
	if !strings.Contains(msg, "缺少 api.responses.write") || !strings.Contains(msg, "重新走 OAuth 授权") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestTestUpstreamAccount_Treats429AsReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("expected nil error for 429 reachability, got %v", err)
	}
}

func TestTestUpstreamAccount_Treats503NoAvailableProvidersAsReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"no_available_providers","message":"no_available_providers"}}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("expected nil error for 503 no_available_providers, got %v", err)
	}
}

func TestListSchedulableAccounts_V1ReturnsHighestAvailablePriorityTier(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	_, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "tier-20-a",
		CredentialRaw:          "rt-a",
		Priority:               20,
		Enabled:                true,
		State:                  "active",
		FailCount:              9,
		QuotaStatus:            "exhausted",
		QuotaWeeklyUsedPercent: testFloat64Ptr(95),
	})
	if err != nil {
		t.Fatalf("create first account failed: %v", err)
	}

	second, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "api_key",
		AccountName:            "tier-10-main",
		CredentialRaw:          "sk-main",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(5),
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}

	_, err = st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "tier-20-b",
		CredentialRaw:          "rt-b",
		Priority:               20,
		Enabled:                true,
		State:                  "active",
		FailCount:              0,
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(10),
	})
	if err != nil {
		t.Fatalf("create third account failed: %v", err)
	}

	accounts, err := svc.ListSchedulableAccounts(ctx)
	if err != nil {
		t.Fatalf("ListSchedulableAccounts failed: %v", err)
	}

	gotIDs := collectAccountIDs(accounts)
	wantIDs := []int64{second.ID}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("unexpected manual failover order: got %v want %v", gotIDs, wantIDs)
	}
}

func TestListSchedulableAccounts_V1FiltersDisabledAuthAndFutureCooldown(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	now := time.Now()

	main, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "main",
		CredentialRaw: "sk-main",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create main account failed: %v", err)
	}

	_, err = st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "disabled-auth",
		CredentialRaw: "sk-disabled",
		Priority:      5,
		Enabled:       true,
		State:         "disabled_auth",
	})
	if err != nil {
		t.Fatalf("create disabled-auth account failed: %v", err)
	}

	_, err = st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "cooldown-future",
		CredentialRaw: "sk-future",
		Priority:      1,
		Enabled:       true,
		State:         "cooldown",
		CooldownUntil: testTimePtr(now.Add(5 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("create future cooldown account failed: %v", err)
	}

	_, err = st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "cooldown-past",
		CredentialRaw: "sk-past",
		Priority:      20,
		Enabled:       true,
		State:         "cooldown",
		CooldownUntil: testTimePtr(now.Add(-5 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("create past cooldown account failed: %v", err)
	}

	_, err = st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "disabled",
		CredentialRaw: "sk-disabled-manual",
		Priority:      2,
		Enabled:       false,
		State:         "disabled",
	})
	if err != nil {
		t.Fatalf("create disabled account failed: %v", err)
	}

	accounts, err := svc.ListSchedulableAccounts(ctx)
	if err != nil {
		t.Fatalf("ListSchedulableAccounts failed: %v", err)
	}

	gotIDs := collectAccountIDs(accounts)
	wantIDs := []int64{main.ID}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("unexpected schedulable accounts: got %v want %v", gotIDs, wantIDs)
	}
}

func TestMoveAccountToTier_ReordersPrioritiesAtomically(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	first, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "first",
		CredentialRaw: "sk-first",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create first account failed: %v", err)
	}
	second, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "second",
		CredentialRaw: "sk-second",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}
	third, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "third",
		CredentialRaw: "sk-third",
		Priority:      30,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create third account failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, third.ID, 0)
	if err != nil {
		t.Fatalf("MoveAccountToTier failed: %v", err)
	}
	if !changed {
		t.Fatal("expected priorities to change")
	}

	accounts, err := st.ListAccounts(ctx, true)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}

	got := collectAccountIDs(accounts)
	want := []int64{third.ID, first.ID, second.ID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected reordered accounts: got %v want %v", got, want)
	}
	if accounts[0].Priority != 10 || accounts[1].Priority != 20 || accounts[2].Priority != 30 {
		t.Fatalf("unexpected priorities after reorder: %+v", accounts)
	}
}

func TestMoveAccountToTier_NoChangeWhenSelectionAlreadyApplied(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	first, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "first",
		CredentialRaw: "sk-first",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create first account failed: %v", err)
	}
	if _, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "second",
		CredentialRaw: "sk-second",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	}); err != nil {
		t.Fatalf("create second account failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, first.ID, 0)
	if err != nil {
		t.Fatalf("MoveAccountToTier failed: %v", err)
	}
	if !changed {
		t.Fatal("expected first manual switch to apply runtime selection")
	}

	changed, err = svc.MoveAccountToTier(ctx, first.ID, 0)
	if err != nil {
		t.Fatalf("second MoveAccountToTier failed: %v", err)
	}
	if changed {
		t.Fatal("expected no change after the same manual selection is already applied")
	}
}

func TestMoveAccountToTier_MovingToBackupDoesNotPinRuntimeSelection(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	first, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "first",
		CredentialRaw: "sk-first",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create first account failed: %v", err)
	}
	second, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "second",
		CredentialRaw: "sk-second",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}
	third, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "third",
		CredentialRaw: "sk-third",
		Priority:      30,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create third account failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, third.ID, 1)
	if err != nil {
		t.Fatalf("MoveAccountToTier failed: %v", err)
	}
	if !changed {
		t.Fatal("expected moving third account into backup tier to change priorities")
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-after-backup-move", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{first.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected primary tier selection to stay on first account, got %v want %v", got, want)
	}

	accounts, err := st.ListAccounts(ctx, true)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{first.ID, third.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected reordered accounts after moving to backup: got %v want %v", got, want)
	}
}

func collectAccountIDs(accounts []*store.UpstreamAccountRecord) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		ids = append(ids, account.ID)
	}
	return ids
}

func testFloat64Ptr(value float64) *float64 {
	return &value
}

func testTimePtr(value time.Time) *time.Time {
	return &value
}

func newTestAccountPoolServiceForConnectivity(t *testing.T, baseURL, providerType, credential string) (*AccountPoolService, int64) {
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
		ProviderType:  providerType,
		AccountName:   "acc-test",
		CredentialRaw: credential,
		BaseURL:       baseURL,
		Priority:      1,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	svc := NewAccountPoolService(st, nil)
	t.Cleanup(func() { _ = svc.Close() })
	return svc, rec.ID
}

func newTestAccountPoolServiceWithStore(t *testing.T) (*AccountPoolService, *store.SQLiteAccountPoolStore) {
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
	svc := NewAccountPoolService(st, nil)
	t.Cleanup(func() { _ = svc.Close() })
	return svc, st
}
