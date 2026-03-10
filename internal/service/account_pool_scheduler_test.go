package service

import (
	"context"
	"reflect"
	"strings"
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
	if got, want := collectAccountIDs(accounts), []int64{mainOAuth.ID, mainAPI.ID}; !reflect.DeepEqual(got, want) {
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
		Quota5HUsedPercent:     testFloat64Ptr(100),
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

func TestPrepareSchedulableAccounts_RanksByUtilizationThenHealthWithinTier(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	now := time.Now()

	expiringSoon, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "expiring-soon",
		CredentialRaw:          "rt-expiring-soon",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(40),
		Quota5HResetAt:         testTimePtr(now.Add(20 * time.Minute)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(20),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(5 * 24 * time.Hour)),
		FailCount:              0,
		LastSuccessAt:          testTimePtr(now.Add(-2 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("create expiring soon account failed: %v", err)
	}

	stableHigh, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "stable-high",
		CredentialRaw:          "rt-stable-high",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(20),
		Quota5HResetAt:         testTimePtr(now.Add(4 * time.Hour)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(20),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(5 * 24 * time.Hour)),
		FailCount:              0,
		LastSuccessAt:          testTimePtr(now.Add(-1 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("create stable high account failed: %v", err)
	}

	weeklyGuarded, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "weekly-guarded",
		CredentialRaw:          "rt-weekly-guarded",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(10),
		Quota5HResetAt:         testTimePtr(now.Add(1 * time.Hour)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(92),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(5 * 24 * time.Hour)),
		FailCount:              0,
		LastSuccessAt:          testTimePtr(now.Add(-30 * time.Second)),
	})
	if err != nil {
		t.Fatalf("create weekly guarded account failed: %v", err)
	}

	smallUrgent, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "small-urgent",
		CredentialRaw:          "rt-small-urgent",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(93),
		Quota5HResetAt:         testTimePtr(now.Add(5 * time.Minute)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(0),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(5 * 24 * time.Hour)),
		FailCount:              0,
		LastSuccessAt:          testTimePtr(now.Add(-45 * time.Second)),
	})
	if err != nil {
		t.Fatalf("create small urgent account failed: %v", err)
	}

	unknown, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "unknown-no-reset",
		CredentialRaw:          "rt-unknown-no-reset",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(10),
	})
	if err != nil {
		t.Fatalf("create unknown account failed: %v", err)
	}

	apiFallback, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "api-fallback",
		CredentialRaw: "sk-api-fallback",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create api fallback account failed: %v", err)
	}

	accounts, err := svc.PrepareSchedulableAccounts(ctx, "req-rank", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{expiringSoon.ID, smallUrgent.ID, weeklyGuarded.ID, stableHigh.ID, unknown.ID, apiFallback.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ranked order: got %v want %v", got, want)
	}
}

func TestPrepareSchedulableAccounts_UsesSingleCompleteQuotaWindowWhenScoring(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	now := time.Now()

	single5H, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:       "chatgpt_refresh_token",
		AccountName:        "single-5h",
		CredentialRaw:      "rt-single-5h",
		Priority:           10,
		Enabled:            true,
		State:              "active",
		QuotaStatus:        "ok",
		Quota5HUsedPercent: testFloat64Ptr(45),
		Quota5HResetAt:     testTimePtr(now.Add(1 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("create single 5h account failed: %v", err)
	}

	singleWeekly, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "single-weekly",
		CredentialRaw:          "rt-single-weekly",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(30),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(12 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("create single weekly account failed: %v", err)
	}

	noReset, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "known-percent-no-reset",
		CredentialRaw:          "rt-known-percent-no-reset",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(10),
	})
	if err != nil {
		t.Fatalf("create no reset account failed: %v", err)
	}

	apiFallback, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "api-fallback",
		CredentialRaw: "sk-api-fallback",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create api fallback account failed: %v", err)
	}

	accounts, err := svc.PrepareSchedulableAccounts(ctx, "req-single-window", "/v1/responses")
	if err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}
	if got, want := collectAccountIDs(accounts), []int64{single5H.ID, singleWeekly.ID, noReset.ID, apiFallback.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected single-window ranking: got %v want %v", got, want)
	}
}

func TestPrepareSchedulableAccounts_SnapshotReasonDetailsExplainUtilizationPriority(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	now := time.Now()

	expiringSoon, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "expiring-soon",
		CredentialRaw:          "rt-expiring-soon",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(40),
		Quota5HResetAt:         testTimePtr(now.Add(20 * time.Minute)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(20),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(5 * 24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("create expiring soon account failed: %v", err)
	}

	weeklyGuarded, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "weekly-guarded",
		CredentialRaw:          "rt-weekly-guarded",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		Quota5HUsedPercent:     testFloat64Ptr(10),
		Quota5HResetAt:         testTimePtr(now.Add(1 * time.Hour)),
		QuotaWeeklyUsedPercent: testFloat64Ptr(92),
		QuotaWeeklyResetAt:     testTimePtr(now.Add(5 * 24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("create weekly guarded account failed: %v", err)
	}

	unknown, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:           "chatgpt_refresh_token",
		AccountName:            "unknown-no-reset",
		CredentialRaw:          "rt-unknown-no-reset",
		Priority:               10,
		Enabled:                true,
		State:                  "active",
		QuotaStatus:            "ok",
		QuotaWeeklyUsedPercent: testFloat64Ptr(10),
	})
	if err != nil {
		t.Fatalf("create unknown account failed: %v", err)
	}

	apiFallback, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "api-fallback",
		CredentialRaw: "sk-api-fallback",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create api fallback account failed: %v", err)
	}

	if _, err := svc.PrepareSchedulableAccounts(ctx, "req-snapshot-reason", "/v1/responses"); err != nil {
		t.Fatalf("PrepareSchedulableAccounts failed: %v", err)
	}

	snapshot, err := svc.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetLatestAccountScheduleSnapshot failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected latest snapshot")
	}

	selected := mustFindCandidateDecision(t, snapshot, expiringSoon.ID)
	assertStringContainsAll(t, selected.ReasonDetail, []string{"5h", "重置"})
	if selected.EffectiveQuotaRemaining == nil {
		t.Fatal("expected selected candidate effective quota remaining to stay populated")
	}

	guarded := mustFindCandidateDecision(t, snapshot, weeklyGuarded.ID)
	assertStringContainsAll(t, guarded.ReasonDetail, []string{"周额度", "护栏"})
	if guarded.EffectiveQuotaRemaining == nil {
		t.Fatal("expected guarded candidate effective quota remaining to stay populated")
	}

	unknownCandidate := mustFindCandidateDecision(t, snapshot, unknown.ID)
	assertStringContainsAll(t, unknownCandidate.ReasonDetail, []string{"quota", "未知", "劣后"})
	if unknownCandidate.EffectiveQuotaRemaining == nil {
		t.Fatal("expected unknown candidate effective quota remaining to stay populated when usage percent exists")
	}

	apiCandidate := mustFindCandidateDecision(t, snapshot, apiFallback.ID)
	assertStringContainsAll(t, apiCandidate.ReasonDetail, []string{"api_key", "兜底", "OAuth"})
	if apiCandidate.EffectiveQuotaRemaining != nil {
		t.Fatal("expected api_key candidate effective quota remaining to remain nil")
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

func assertStringContainsAll(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}
}
