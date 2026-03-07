package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"cc-forwarder/internal/store"
)

func TestPrepareSchedulableAccounts_SelectsHighestPriorityTierOnly(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	mainAPI, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "main-api",
		CredentialRaw: "sk-main-api",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create main api account failed: %v", err)
	}

	mainOAuth, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "main-oauth",
		CredentialRaw:          "rt-main-oauth",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(40),
	})
	if err != nil {
		t.Fatalf("create main oauth account failed: %v", err)
	}

	backup, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "backup-api",
		CredentialRaw: "sk-backup-api",
		Priority:      20,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create backup account failed: %v", err)
	}

	accounts, err := svc.PrepareSchedulableAccounts(ctx, "req-select-main", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{mainAPI.ID, mainOAuth.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected selected tier accounts: got %v want %v", got, want)
	}

	snapshot, err := svc.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected latest snapshot")
	}
	if snapshot.SelectedPriority != 10 || snapshot.SelectedTierIndex != 1 || snapshot.SelectedTierLabel != "主组" {
		t.Fatalf("unexpected selected tier info: %+v", snapshot)
	}
	if snapshot.DegradedToLowerPriority {
		t.Fatalf("expected no degrade, got %+v", snapshot)
	}
	candidate := mustFindCandidateDecision(t, snapshot, backup.ID)
	if candidate.Decision != accountScheduleDecisionSkipped || candidate.Reason != "higher_priority_tier_selected" {
		t.Fatalf("expected lower tier skipped by higher priority, got %+v", candidate)
	}
}

func TestPrepareSchedulableAccounts_DegradesWhenHigherTierTemporarilyExhausted(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	now := time.Now()

	exhausted, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "main-exhausted",
		CredentialRaw:          "rt-main-exhausted",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "exhausted",
		Quota5HResetAt:         testTimePtr(now.Add(2 * time.Hour)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(100),
	})
	if err != nil {
		t.Fatalf("create exhausted account failed: %v", err)
	}

	backup, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "backup-ok",
		CredentialRaw:          "rt-backup-ok",
		Priority:               20,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(20),
	})
	if err != nil {
		t.Fatalf("create backup account failed: %v", err)
	}

	accounts, err := svc.PrepareSchedulableAccounts(ctx, "req-degrade", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{backup.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected degraded selection: got %v want %v", got, want)
	}

	snapshot, err := svc.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected latest snapshot")
	}
	if !snapshot.DegradedToLowerPriority || snapshot.SelectedTierIndex != 2 || snapshot.SelectedTierLabel != "备组" {
		t.Fatalf("expected degrade to backup tier, got %+v", snapshot)
	}
	candidate := mustFindCandidateDecision(t, snapshot, exhausted.ID)
	if candidate.Reason != "quota_exhausted_until_reset" {
		t.Fatalf("expected exhausted account skipped by reset reason, got %+v", candidate)
	}
}

func TestPrepareSchedulableAccounts_RanksByQuotaThenHealthWithinTier(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	now := time.Now()

	best, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "best",
		CredentialRaw:          "rt-best",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(10),
		QuotaWeeklyUsedPercent: testFloat64Ptr(30),
		FailCount:              0,
		LastSuccessAt:          testTimePtr(now.Add(-2 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("create best account failed: %v", err)
	}

	second, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "second",
		CredentialRaw:          "rt-second",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(10),
		QuotaWeeklyUsedPercent: testFloat64Ptr(30),
		FailCount:              1,
		LastSuccessAt:          testTimePtr(now.Add(-1 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("create second account failed: %v", err)
	}

	third, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "third",
		CredentialRaw:          "rt-third",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(50),
		QuotaWeeklyUsedPercent: testFloat64Ptr(60),
		FailCount:              0,
		LastSuccessAt:          testTimePtr(now.Add(-30 * time.Second)),
	})
	if err != nil {
		t.Fatalf("create third account failed: %v", err)
	}

	unknown, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "chatgpt_refresh_token",
		AccountName:   "unknown",
		CredentialRaw: "rt-unknown",
		Priority:      10,
		Enabled:       true,
		State:         "active",
		QuotaStatus:   "unavailable",
	})
	if err != nil {
		t.Fatalf("create unknown account failed: %v", err)
	}

	accounts, err := svc.PrepareSchedulableAccounts(ctx, "req-rank", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{best.ID, second.ID, third.ID, unknown.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ranked order: got %v want %v", got, want)
	}
}

func TestCompleteLatestScheduleSnapshot_UpdatesOutcomeAndRuntimeCandidate(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()

	selected, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "selected",
		CredentialRaw: "sk-selected",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create selected account failed: %v", err)
	}

	if _, err := svc.PrepareSchedulableAccounts(ctx, "req-complete", "/v1/responses"); err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if err := svc.CompleteLatestScheduleSnapshot(ctx, "req-complete", selected.ID, selected.AccountName, accountScheduleOutcomeSuccess, ""); err != nil {
		t.Fatalf("CompleteLatestScheduleSnapshot failed: %v", err)
	}

	snapshot, err := svc.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected latest snapshot")
	}
	if snapshot.FinalOutcome != accountScheduleOutcomeSuccess {
		t.Fatalf("expected success outcome, got %+v", snapshot)
	}
	candidate := mustFindCandidateDecision(t, snapshot, selected.ID)
	if candidate.RuntimeOutcome != accountScheduleOutcomeSuccess {
		t.Fatalf("expected runtime outcome to be recorded, got %+v", candidate)
	}
}

func TestLatestAccountScheduleSnapshotStore_SaveDraftCleansExpiredPending(t *testing.T) {
	store := newLatestAccountScheduleSnapshotStore()
	now := time.Now()
	store.pending["expired"] = &LatestAccountScheduleSnapshot{
		RequestID:   "expired",
		CapturedAt:  now.Add(-latestAccountScheduleSnapshotPendingTTL - time.Minute),
		UpdatedAt:   now.Add(-latestAccountScheduleSnapshotPendingTTL - time.Minute),
		RequestPath: "/v1/responses",
	}
	store.pending["fresh"] = &LatestAccountScheduleSnapshot{
		RequestID:   "fresh",
		CapturedAt:  now,
		UpdatedAt:   now,
		RequestPath: "/v1/responses",
	}

	store.saveDraft(&LatestAccountScheduleSnapshot{
		RequestID:   "new-request",
		CapturedAt:  now,
		UpdatedAt:   now,
		RequestPath: "/v1/responses",
	})

	store.mu.RLock()
	defer store.mu.RUnlock()

	if _, ok := store.pending["expired"]; ok {
		t.Fatal("expected expired pending snapshot to be cleaned up")
	}
	if _, ok := store.pending["fresh"]; !ok {
		t.Fatal("expected fresh pending snapshot to be retained")
	}
	if _, ok := store.pending["new-request"]; !ok {
		t.Fatal("expected new draft snapshot to be stored")
	}
}

func mustFindCandidateDecision(t *testing.T, snapshot *LatestAccountScheduleSnapshot, accountID int64) AccountScheduleCandidateDecision {
	t.Helper()
	for _, candidate := range snapshot.Candidates {
		if candidate.AccountID == accountID {
			return candidate
		}
	}
	t.Fatalf("candidate %d not found in snapshot %+v", accountID, snapshot)
	return AccountScheduleCandidateDecision{}
}
