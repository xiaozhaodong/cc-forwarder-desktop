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

func TestPrepareSchedulableAccounts_DoesNotExposeManualSelectionForAutomaticChoice(t *testing.T) {
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
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-auto-selection", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{first.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected automatic ranking: got %v want %v", got, want)
	}

	accountID, ok, err := svc.GetActiveSelectionAccountID(ctx)
	if err != nil {
		t.Fatalf("GetActiveSelectionAccountID failed: %v", err)
	}
	if ok {
		t.Fatalf("expected automatic scheduling not to set manual active selection, got account %d", accountID)
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

func TestMoveAccountToTier_ChangingPrimaryAccountWithinSameTierCountsAsChange(t *testing.T) {
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
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, first.ID, 0)
	if err != nil {
		t.Fatalf("MoveAccountToTier first failed: %v", err)
	}
	if !changed {
		t.Fatal("expected first manual primary selection to change runtime state")
	}

	changed, err = svc.MoveAccountToTier(ctx, second.ID, 0)
	if err != nil {
		t.Fatalf("MoveAccountToTier second failed: %v", err)
	}
	if !changed {
		t.Fatal("expected switching to another account in the same primary tier to count as change")
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-same-tier-manual-switch", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{second.ID, first.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected second account to become active selection, got %v want %v", got, want)
	}
}

func TestUpdateAccount_PreservesPinnedSelectionWhenPinnedAccountPriorityChanges(t *testing.T) {
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
		t.Fatal("expected manual primary selection to change runtime state")
	}

	updatedFirst, err := svc.GetAccount(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetAccount first failed: %v", err)
	}
	if updatedFirst == nil {
		t.Fatal("expected first account")
	}
	updatedFirst.Priority = 30
	if err := svc.UpdateAccount(ctx, updatedFirst); err != nil {
		t.Fatalf("UpdateAccount first failed: %v", err)
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-pinned-priority-edit", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{first.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected pinned account to remain selected after priority edit, got %v want %v", got, want)
	}

	accountID, ok, err := svc.GetActiveSelectionAccountID(ctx)
	if err != nil {
		t.Fatalf("GetActiveSelectionAccountID failed: %v", err)
	}
	if !ok || accountID != first.ID {
		t.Fatalf("expected pinned account to remain active selection, got ok=%v accountID=%d", ok, accountID)
	}
}

func TestMoveAccountToTier_ManualSelectionPreservesPinnedTargetAcrossTransientCooldown(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	primary, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "primary",
		CredentialRaw: "sk-primary",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create primary account failed: %v", err)
	}
	backup, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "backup",
		CredentialRaw: "sk-backup",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create backup account failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, primary.ID, 0)
	if err != nil {
		t.Fatalf("MoveAccountToTier primary failed: %v", err)
	}
	if !changed {
		t.Fatal("expected manual primary selection to change runtime state")
	}

	if err := svc.MarkAccountTransientFailure(ctx, primary.ID, "temporary upstream failure", 2*time.Minute); err != nil {
		t.Fatalf("MarkAccountTransientFailure primary failed: %v", err)
	}

	accountID, ok, err := svc.GetActiveSelectionAccountID(ctx)
	if err != nil {
		t.Fatalf("GetActiveSelectionAccountID failed: %v", err)
	}
	if !ok || accountID != primary.ID {
		t.Fatalf("expected manual selection to stay pinned on primary during cooldown, got ok=%v accountID=%d", ok, accountID)
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-manual-cooldown-degrade", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{backup.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected cooled-down primary to be skipped until cooldown ends, got %v want %v", got, want)
	}

	currentPrimary, err := svc.GetAccount(ctx, primary.ID)
	if err != nil {
		t.Fatalf("GetAccount primary failed: %v", err)
	}
	if currentPrimary == nil {
		t.Fatal("expected primary account")
	}
	if currentPrimary.State != "cooldown" || currentPrimary.CooldownUntil == nil || currentPrimary.FailCount != 1 {
		t.Fatalf("expected manual selection not to clear transient cooldown, got %+v", currentPrimary)
	}

	persistedPrimary, err := st.GetAccount(ctx, primary.ID)
	if err != nil {
		t.Fatalf("store GetAccount primary failed: %v", err)
	}
	if persistedPrimary == nil {
		t.Fatal("expected persisted primary account")
	}
	persistedPrimary.State = "cooldown"
	expiredAt := time.Now().Add(-time.Minute)
	persistedPrimary.CooldownUntil = &expiredAt
	persistedPrimary.FailCount = 1
	if err := st.UpdateAccount(ctx, persistedPrimary); err != nil {
		t.Fatalf("UpdateAccount primary cooldown expiry failed: %v", err)
	}
	if err := svc.reloadAccountIntoCache(ctx, primary.ID); err != nil {
		t.Fatalf("reloadAccountIntoCache primary failed: %v", err)
	}

	ordered, err = svc.PrepareSchedulableAccounts(ctx, "req-manual-cooldown-recovered", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts after cooldown failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{primary.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected manual pinned primary to take effect after cooldown ends, got %v want %v", got, want)
	}

	currentBackup, err := svc.GetAccount(ctx, backup.ID)
	if err != nil {
		t.Fatalf("GetAccount backup failed: %v", err)
	}
	if currentBackup == nil {
		t.Fatal("expected backup account")
	}
}

func TestManualPinnedSelection_IsNotOverwrittenByLaterSuccessFromAnotherAccount(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	primary, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "primary",
		CredentialRaw: "sk-primary",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create primary account failed: %v", err)
	}
	backup, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "backup",
		CredentialRaw: "sk-backup",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create backup account failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, primary.ID, 0)
	if err != nil {
		t.Fatalf("MoveAccountToTier primary failed: %v", err)
	}
	if !changed {
		t.Fatal("expected manual primary selection to change runtime state")
	}

	if err := svc.MarkAccountSuccess(ctx, backup.ID); err != nil {
		t.Fatalf("MarkAccountSuccess backup failed: %v", err)
	}

	accountID, ok, err := svc.GetActiveSelectionAccountID(ctx)
	if err != nil {
		t.Fatalf("GetActiveSelectionAccountID failed: %v", err)
	}
	if !ok || accountID != primary.ID {
		t.Fatalf("expected manual pinned selection to remain on primary, got ok=%v accountID=%d", ok, accountID)
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-manual-pin-not-overwritten", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{primary.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected manual pinned selection to remain effective after backup success, got %v want %v", got, want)
	}
}

func TestMoveAccountToTier_BackupNoOpWithSingleTierPreservesSnapshotAndSelection(t *testing.T) {
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
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-before-single-tier-backup", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts before backup no-op failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{first.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected baseline selected accounts: got %v want %v", got, want)
	}

	snapshotBefore, err := svc.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot before backup no-op failed: %v", err)
	}
	if snapshotBefore == nil {
		t.Fatal("expected existing snapshot before backup no-op")
	}

	changed, err := svc.MoveAccountToTier(ctx, second.ID, 1)
	if err != nil {
		t.Fatalf("MoveAccountToTier backup no-op failed: %v", err)
	}
	if changed {
		t.Fatal("expected moving a single tier into backup to be a no-op")
	}

	snapshotAfter, err := svc.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot after backup no-op failed: %v", err)
	}
	if snapshotAfter == nil || snapshotAfter.RequestID != snapshotBefore.RequestID {
		t.Fatalf("expected latest snapshot to be preserved after backup no-op, got before=%+v after=%+v", snapshotBefore, snapshotAfter)
	}

	ordered, err = svc.PrepareSchedulableAccounts(ctx, "req-after-single-tier-backup", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts after backup no-op failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{first.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected backup no-op to preserve active selection, got %v want %v", got, want)
	}
}

func TestMoveAccountToTier_MovingToBackupMovesWholeTierAndSelectsClickedAccount(t *testing.T) {
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
	thirdPeer, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "third-peer",
		CredentialRaw: "sk-third-peer",
		Priority:      30,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create third peer account failed: %v", err)
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
	if got, want := collectAccountIDs(ordered), []int64{third.ID, thirdPeer.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected clicked backup tier account to become active selection, got %v want %v", got, want)
	}

	for accountID, wantPriority := range map[int64]int{
		first.ID:     10,
		third.ID:     20,
		thirdPeer.ID: 20,
		second.ID:    30,
	} {
		record, getErr := st.GetAccount(ctx, accountID)
		if getErr != nil {
			t.Fatalf("GetAccount %d failed: %v", accountID, getErr)
		}
		if record == nil || record.Priority != wantPriority {
			t.Fatalf("unexpected priority for account %d: got %+v want priority %d", accountID, record, wantPriority)
		}
	}

	accounts, err := st.ListAccounts(ctx, true)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{first.ID, third.ID, thirdPeer.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected reordered accounts after moving to backup: got %v want %v", got, want)
	}
}

func TestMoveAccountToTier_ChangingBackupAccountWithinSameTierCountsAsChange(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	if _, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "primary",
		CredentialRaw: "sk-primary",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	}); err != nil {
		t.Fatalf("create primary account failed: %v", err)
	}
	backupA, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "backup-a",
		CredentialRaw: "sk-backup-a",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create backup-a failed: %v", err)
	}
	backupB, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "backup-b",
		CredentialRaw: "sk-backup-b",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create backup-b failed: %v", err)
	}

	changed, err := svc.MoveAccountToTier(ctx, backupA.ID, 1)
	if err != nil {
		t.Fatalf("MoveAccountToTier backup-a failed: %v", err)
	}
	if !changed {
		t.Fatal("expected first backup selection to change runtime state")
	}

	changed, err = svc.MoveAccountToTier(ctx, backupB.ID, 1)
	if err != nil {
		t.Fatalf("MoveAccountToTier backup-b failed: %v", err)
	}
	if !changed {
		t.Fatal("expected switching to another account in the same backup tier to count as change")
	}

	ordered, err := svc.PrepareSchedulableAccounts(ctx, "req-same-tier-backup-switch", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(ordered), []int64{backupB.ID, backupA.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected second backup account to become active selection, got %v want %v", got, want)
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
