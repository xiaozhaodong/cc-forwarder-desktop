package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cc-forwarder/internal/service"
	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

func TestGetUpstreamAccounts_UsesAccountDisplayTimeZone(t *testing.T) {
	app, _ := newAccountPoolAPITestApp(t)

	resetAtUTC := time.Date(2026, 3, 11, 13, 6, 0, 0, time.UTC)
	_, err := app.accountPoolService.CreateAccount(context.Background(), &store.UpstreamAccountRecord{
		ProviderType:                  "chatgpt_refresh_token",
		AccountName:                   "lixunhuan",
		CredentialRaw:                 `{"refresh_token":"rt-test"}`,
		BaseURL:                       "https://api.openai.com",
		CostMultiplier:                1.0,
		InputCostMultiplier:           1.0,
		OutputCostMultiplier:          1.0,
		CacheCreationCostMultiplier:   1.0,
		CacheCreationCostMultiplier1h: 1.0,
		CacheReadCostMultiplier:       1.0,
		Priority:                      20,
		Enabled:                       true,
		State:                         "active",
		QuotaStatus:                   "ok",
		Quota5HUsedPercent:            float64Ptr(6),
		Quota5HResetAt:                &resetAtUTC,
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	accounts, err := app.GetUpstreamAccounts()
	if err != nil {
		t.Fatalf("GetUpstreamAccounts failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if got := accounts[0].Quota5HResetAt; got != "2026-03-11 21:06:00" {
		t.Fatalf("expected quota reset in UTC+8, got %q", got)
	}
}

func TestGetUpstreamAccounts_PreservesLegacyNaiveTimestamps(t *testing.T) {
	app, db := newAccountPoolAPITestApp(t)

	created, err := app.accountPoolStore.CreateAccount(context.Background(), &store.UpstreamAccountRecord{
		ProviderType:                  "chatgpt_refresh_token",
		AccountName:                   "legacy-naive",
		CredentialRaw:                 `{"refresh_token":"rt-legacy"}`,
		BaseURL:                       "https://api.openai.com",
		CostMultiplier:                1.0,
		InputCostMultiplier:           1.0,
		OutputCostMultiplier:          1.0,
		CacheCreationCostMultiplier:   1.0,
		CacheCreationCostMultiplier1h: 1.0,
		CacheReadCostMultiplier:       1.0,
		Priority:                      30,
		Enabled:                       true,
		State:                         "active",
		QuotaStatus:                   "ok",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if _, err := db.Exec(
		`UPDATE upstream_accounts
		 SET quota_5h_reset_at = ?, updated_at = ?
		 WHERE id = ?`,
		"2026-03-11 21:06:00", "2026-03-11 21:06:00", created.ID,
	); err != nil {
		t.Fatalf("seed legacy naive timestamps failed: %v", err)
	}

	accounts, err := app.GetUpstreamAccounts()
	if err != nil {
		t.Fatalf("GetUpstreamAccounts failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if got := accounts[0].Quota5HResetAt; got != "2026-03-11 21:06:00" {
		t.Fatalf("expected legacy naive quota reset to stay in UTC+8 wall clock, got %q", got)
	}
	if got := accounts[0].UpdatedAt; got != "2026-03-11 21:06:00" {
		t.Fatalf("expected legacy naive updated_at to stay in UTC+8 wall clock, got %q", got)
	}
}

func TestGetUpstreamAccounts_ReportsActiveSelectionAfterSameTierManualSwitch(t *testing.T) {
	app, _ := newAccountPoolAPITestApp(t)
	ctx := context.Background()

	first, err := app.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "primary-a",
		CredentialRaw: "sk-primary-a",
		BaseURL:       "https://api.openai.com",
		Priority:      10,
		Enabled:       true,
		State:         "active",
		QuotaStatus:   "ok",
	})
	if err != nil {
		t.Fatalf("CreateAccount first failed: %v", err)
	}

	second, err := app.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "primary-b",
		CredentialRaw: "sk-primary-b",
		BaseURL:       "https://api.openai.com",
		Priority:      10,
		Enabled:       true,
		State:         "active",
		QuotaStatus:   "ok",
	})
	if err != nil {
		t.Fatalf("CreateAccount second failed: %v", err)
	}

	result, err := app.MoveUpstreamAccountToTier(second.ID, "primary")
	if err != nil {
		t.Fatalf("MoveUpstreamAccountToTier failed: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected same-tier manual switch to report changed, got %+v", result)
	}

	accounts, err := app.GetUpstreamAccounts()
	if err != nil {
		t.Fatalf("GetUpstreamAccounts failed: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	var firstInfo *UpstreamAccountInfo
	var secondInfo *UpstreamAccountInfo
	for idx := range accounts {
		account := accounts[idx]
		switch account.ID {
		case first.ID:
			firstInfo = &account
		case second.ID:
			secondInfo = &account
		}
	}
	if firstInfo == nil || secondInfo == nil {
		t.Fatalf("expected both accounts in response, got %+v", accounts)
	}
	if firstInfo.IsActiveSelection {
		t.Fatalf("expected first account to be inactive after same-tier switch, got %+v", firstInfo)
	}
	if !secondInfo.IsActiveSelection {
		t.Fatalf("expected second account to be marked active after same-tier switch, got %+v", secondInfo)
	}
}

func TestGetLatestAccountScheduleSnapshot_UsesAccountDisplayTimeZoneForCandidateTimes(t *testing.T) {
	app, _ := newAccountPoolAPITestApp(t)

	lastSuccessUTC := time.Date(2026, 3, 11, 13, 6, 0, 0, time.UTC)
	_, err := app.accountPoolService.CreateAccount(context.Background(), &store.UpstreamAccountRecord{
		ProviderType:                  "chatgpt_refresh_token",
		AccountName:                   "snapshot-candidate",
		CredentialRaw:                 `{"refresh_token":"rt-snapshot"}`,
		BaseURL:                       "https://api.openai.com",
		CostMultiplier:                1.0,
		InputCostMultiplier:           1.0,
		OutputCostMultiplier:          1.0,
		CacheCreationCostMultiplier:   1.0,
		CacheCreationCostMultiplier1h: 1.0,
		CacheReadCostMultiplier:       1.0,
		Priority:                      20,
		Enabled:                       true,
		State:                         "active",
		LastSuccessAt:                 &lastSuccessUTC,
		QuotaStatus:                   "ok",
		Quota5HUsedPercent:            float64Ptr(12),
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if _, err := app.accountPoolService.PrepareSchedulableAccounts(context.Background(), "req-1", "/v1/responses"); err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}

	snapshot, err := app.GetLatestAccountScheduleSnapshot()
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot failed: %v", err)
	}
	if len(snapshot.Candidates) == 0 {
		t.Fatal("expected snapshot candidates")
	}
	if got := snapshot.Candidates[0].LastSuccessAt; got != "2026-03-11 21:06:00" {
		t.Fatalf("expected candidate last_success_at in UTC+8, got %q", got)
	}
}

func TestMoveUpstreamAccountToTier_ReturnsChangedWhenManualSwitchOverridesStickySelection(t *testing.T) {
	app, _ := newAccountPoolAPITestApp(t)
	ctx := context.Background()
	now := time.Now()

	primary, err := app.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "manual-primary",
		CredentialRaw:          `{"refresh_token":"rt-manual-primary"}`,
		BaseURL:                "https://api.openai.com",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "exhausted",
		Quota5HUsedPercent:     float64Ptr(100),
		Quota5HResetAt:         timePtr(now.Add(90 * time.Minute)),
		QuotaWeeklyUsedPercent: float64Ptr(100),
	})
	if err != nil {
		t.Fatalf("CreateAccount primary failed: %v", err)
	}

	_, err = app.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:                  "chatgpt_refresh_token",
		AccountName:                   "sticky-backup",
		CredentialRaw:                 `{"refresh_token":"rt-sticky-backup"}`,
		BaseURL:                       "https://api.openai.com",
		Priority:                      20,
		Enabled:                       true,
		State:                         "active",
		QuotaStatus:                   "ok",
		Quota5HUsedPercent:            float64Ptr(20),
		Quota5HResetAt:                timePtr(now.Add(45 * time.Minute)),
		QuotaWeeklyUsedPercent:        float64Ptr(20),
		QuotaWeeklyResetAt:            timePtr(now.Add(6 * 24 * time.Hour)),
		CostMultiplier:                1,
		InputCostMultiplier:           1,
		OutputCostMultiplier:          1,
		CacheCreationCostMultiplier:   1,
		CacheCreationCostMultiplier1h: 1,
		CacheReadCostMultiplier:       1,
	})
	if err != nil {
		t.Fatalf("CreateAccount backup failed: %v", err)
	}

	if _, err := app.accountPoolService.PrepareSchedulableAccounts(ctx, "req-before-manual-switch", "/v1/responses"); err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}

	recoveredPrimary, err := app.accountPoolService.GetAccount(ctx, primary.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if recoveredPrimary == nil {
		t.Fatal("expected recovered primary account")
	}
	recoveredPrimary.QuotaStatus = "ok"
	recoveredPrimary.Quota5HUsedPercent = nil
	recoveredPrimary.Quota5HResetAt = nil
	recoveredPrimary.QuotaWeeklyUsedPercent = nil
	recoveredPrimary.QuotaWeeklyResetAt = nil
	if err := app.accountPoolService.UpdateAccount(ctx, recoveredPrimary); err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}

	result, err := app.MoveUpstreamAccountToTier(primary.ID, "primary")
	if err != nil {
		t.Fatalf("MoveUpstreamAccountToTier failed: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected manual switch override to report changed, got %+v", result)
	}
}

func TestMoveUpstreamAccountToTier_MessageReflectsSchedulableStatusAndCooldownPinnedSelection(t *testing.T) {
	app, _ := newAccountPoolAPITestApp(t)
	ctx := context.Background()

	primary, err := app.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "primary-cooldown",
		CredentialRaw: "sk-primary-cooldown",
		BaseURL:       "https://api.openai.com",
		Priority:      10,
		Enabled:       true,
		State:         "active",
		QuotaStatus:   "ok",
	})
	if err != nil {
		t.Fatalf("CreateAccount primary failed: %v", err)
	}
	backup, err := app.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "backup-active",
		CredentialRaw: "sk-backup-active",
		BaseURL:       "https://api.openai.com",
		Priority:      20,
		Enabled:       true,
		State:         "active",
		QuotaStatus:   "ok",
	})
	if err != nil {
		t.Fatalf("CreateAccount backup failed: %v", err)
	}

	if err := app.accountPoolService.MarkAccountTransientFailure(ctx, primary.ID, "temporary failure", 2*time.Minute); err != nil {
		t.Fatalf("MarkAccountTransientFailure primary failed: %v", err)
	}

	result, err := app.MoveUpstreamAccountToTier(primary.ID, "primary")
	if err != nil {
		t.Fatalf("MoveUpstreamAccountToTier failed: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected manual switch to report changed, got %+v", result)
	}
	if !strings.Contains(result.Message, "当前可调度状态优先使用") || strings.Contains(result.Message, "立即生效") {
		t.Fatalf("expected API message to describe deferred schedulable semantics, got %q", result.Message)
	}

	accounts, err := app.GetUpstreamAccounts()
	if err != nil {
		t.Fatalf("GetUpstreamAccounts failed: %v", err)
	}
	var primaryInfo *UpstreamAccountInfo
	var backupInfo *UpstreamAccountInfo
	for idx := range accounts {
		account := accounts[idx]
		switch account.ID {
		case primary.ID:
			primaryInfo = &account
		case backup.ID:
			backupInfo = &account
		}
	}
	if primaryInfo == nil || backupInfo == nil {
		t.Fatalf("expected both accounts in response, got %+v", accounts)
	}
	if !primaryInfo.IsActiveSelection {
		t.Fatalf("expected cooldown primary to remain manual active selection, got %+v", primaryInfo)
	}
	if backupInfo.IsActiveSelection {
		t.Fatalf("expected backup not to become manual active selection, got %+v", backupInfo)
	}
}

func TestFormatTime_PreservesInputLocationWallClock(t *testing.T) {
	loc := time.FixedZone("UTC-7", -7*60*60)
	timestamp := time.Date(2026, 3, 11, 6, 30, 0, 0, loc)

	if got := formatTime(timestamp); got != "2026-03-11 06:30:00" {
		t.Fatalf("expected wall clock to stay in source location, got %q", got)
	}
}

func newAccountPoolAPITestApp(t *testing.T) (*App, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schemaPath := filepath.Join("internal", "tracking", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema failed: %v", err)
	}

	app := NewApp()
	app.accountPoolStore = store.NewSQLiteAccountPoolStore(db)
	app.accountPoolService = service.NewAccountPoolService(app.accountPoolStore, nil)
	t.Cleanup(func() { _ = app.accountPoolService.Close() })
	return app, db
}

func float64Ptr(value float64) *float64 {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
