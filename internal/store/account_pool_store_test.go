package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestFindAccountByFingerprint_ReturnsAccount(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	record, err := st.CreateAccount(ctx, &UpstreamAccountRecord{
		ProviderType:                  "api_key",
		AccountName:                   "alpha",
		CredentialRaw:                 "sk-alpha",
		BaseURL:                       "https://api.openai.com",
		CostMultiplier:                1.6,
		InputCostMultiplier:           1.2,
		OutputCostMultiplier:          1.3,
		CacheCreationCostMultiplier:   1.4,
		CacheCreationCostMultiplier1h: 1.5,
		CacheReadCostMultiplier:       1.1,
		Priority:                      10,
		Enabled:                       true,
		State:                         "active",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	got, err := st.FindAccountByFingerprint(ctx, record.Fingerprint)
	if err != nil {
		t.Fatalf("FindAccountByFingerprint failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected account, got nil")
	}
	if got.ID != record.ID {
		t.Fatalf("expected account id %d, got %d", record.ID, got.ID)
	}
	if got.CostMultiplier != 1.6 || got.InputCostMultiplier != 1.2 || got.OutputCostMultiplier != 1.3 {
		t.Fatalf("unexpected multipliers: %+v", got)
	}
}

func TestUpdateAccountPriorities_UpdatesAllRequestedAccounts(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	first := mustCreateTestAccount(t, st, "first", 10)
	second := mustCreateTestAccount(t, st, "second", 20)
	third := mustCreateTestAccount(t, st, "third", 30)

	if err := st.UpdateAccountPriorities(ctx, map[int64]int{
		first.ID:  30,
		second.ID: 10,
		third.ID:  20,
	}); err != nil {
		t.Fatalf("UpdateAccountPriorities failed: %v", err)
	}

	accounts, err := st.ListAccounts(ctx, true)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(accounts))
	}

	if accounts[0].ID != second.ID || accounts[0].Priority != 10 {
		t.Fatalf("expected second account to become priority 10, got %+v", accounts[0])
	}
	if accounts[1].ID != third.ID || accounts[1].Priority != 20 {
		t.Fatalf("expected third account to become priority 20, got %+v", accounts[1])
	}
	if accounts[2].ID != first.ID || accounts[2].Priority != 30 {
		t.Fatalf("expected first account to become priority 30, got %+v", accounts[2])
	}
}

func TestMarkAccountSuccessIfNoNewerFailure_PreservesNewerCooldownState(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	record := mustCreateTestAccount(t, st, "guarded", 10)
	failureTime := time.Now()
	if err := st.MarkAccountTransientFailure(ctx, record.ID, "stream failed", failureTime.Add(60*time.Second)); err != nil {
		t.Fatalf("MarkAccountTransientFailure failed: %v", err)
	}

	updated, err := st.MarkAccountSuccessIfNoNewerFailure(ctx, record.ID, failureTime.Add(2*time.Second), failureTime.Add(-2*time.Second))
	if err != nil {
		t.Fatalf("MarkAccountSuccessIfNoNewerFailure failed: %v", err)
	}
	if updated {
		t.Fatal("expected stale success not to clear newer cooldown state")
	}

	current, err := st.GetAccount(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if current.State != "cooldown" {
		t.Fatalf("expected account to remain in cooldown, got %s", current.State)
	}
	if current.CooldownUntil == nil {
		t.Fatal("expected cooldown_until to remain set")
	}
	if current.FailCount != 1 {
		t.Fatalf("expected fail_count to remain 1, got %d", current.FailCount)
	}

	updated, err = st.MarkAccountSuccessIfNoNewerFailure(ctx, record.ID, failureTime.Add(3*time.Second), failureTime.Add(3*time.Second))
	if err != nil {
		t.Fatalf("MarkAccountSuccessIfNoNewerFailure second call failed: %v", err)
	}
	if !updated {
		t.Fatal("expected newer success attempt to clear cooldown state")
	}

	current, err = st.GetAccount(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetAccount after success failed: %v", err)
	}
	if current.State != "active" {
		t.Fatalf("expected account to become active, got %s", current.State)
	}
	if current.CooldownUntil != nil {
		t.Fatalf("expected cooldown_until cleared, got %v", current.CooldownUntil)
	}
	if current.FailCount != 0 {
		t.Fatalf("expected fail_count reset to 0, got %d", current.FailCount)
	}
}

func TestMarkAccountSuccessIfNoNewerFailure_PreservesSameMillisecondNewerCooldownState(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	record := mustCreateTestAccount(t, st, "same-ms-guarded", 10)
	attemptStartedAt := time.Date(2026, 3, 9, 12, 34, 56, 789123000, accountDBTimeZone)
	newerFailureAt := time.Date(2026, 3, 9, 12, 34, 56, 789456000, accountDBTimeZone)
	cooldownUntil := newerFailureAt.Add(60 * time.Second)

	if _, err := st.db.ExecContext(ctx,
		`UPDATE upstream_accounts
		 SET fail_count = 1, state = 'cooldown', cooldown_until = ?, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		formatDBTime(cooldownUntil), "same millisecond failure", formatDBTime(newerFailureAt), record.ID); err != nil {
		t.Fatalf("seed newer cooldown state failed: %v", err)
	}

	updated, err := st.MarkAccountSuccessIfNoNewerFailure(ctx, record.ID, newerFailureAt.Add(2*time.Second), attemptStartedAt)
	if err != nil {
		t.Fatalf("MarkAccountSuccessIfNoNewerFailure failed: %v", err)
	}
	if updated {
		t.Fatal("expected stale success not to clear newer cooldown state in the same millisecond window")
	}

	current, err := st.GetAccount(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if current.State != "cooldown" {
		t.Fatalf("expected account to remain in cooldown, got %s", current.State)
	}
	if current.FailCount != 1 {
		t.Fatalf("expected fail_count to remain 1, got %d", current.FailCount)
	}
}

func TestMarkAccountTransientFailure_StoresMicrosecondUpdatedAt(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	record := mustCreateTestAccount(t, st, "precision-check", 10)
	if err := st.MarkAccountTransientFailure(ctx, record.ID, "stream failed", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("MarkAccountTransientFailure failed: %v", err)
	}

	var updatedAt string
	if err := st.db.QueryRowContext(ctx, `SELECT updated_at FROM upstream_accounts WHERE id = ?`, record.ID).Scan(&updatedAt); err != nil {
		t.Fatalf("query updated_at failed: %v", err)
	}

	pattern := regexp.MustCompile(`\.\d{6}[+-]\d{2}:\d{2}$`)
	if !pattern.MatchString(updatedAt) {
		t.Fatalf("expected microsecond precision updated_at, got %q", updatedAt)
	}
}

func TestMarkAccountAuthFailedWithProfile_ReturnsErrorWhenAccountMissing(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	err := st.MarkAccountAuthFailedWithProfile(ctx, &UpstreamAccountRecord{
		ID:            999,
		ProviderType:  "chatgpt_refresh_token",
		AccountName:   "missing",
		CredentialRaw: "rt-missing",
		BaseURL:       "https://chatgpt.com",
		PlanType:      "free",
		QuotaStatus:   "auth_invalid",
	}, "oauth invalid")
	if err == nil {
		t.Fatal("expected missing account error")
	}
	if !strings.Contains(err.Error(), "账号不存在") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTestSQLiteAccountPoolStore(t *testing.T) *SQLiteAccountPoolStore {
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

	return NewSQLiteAccountPoolStore(db)
}

func mustCreateTestAccount(t *testing.T, st *SQLiteAccountPoolStore, name string, priority int) *UpstreamAccountRecord {
	t.Helper()

	record, err := st.CreateAccount(context.Background(), &UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   name,
		CredentialRaw: "sk-" + name,
		BaseURL:       "https://api.openai.com",
		Priority:      priority,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("CreateAccount(%s) failed: %v", name, err)
	}
	return record
}
