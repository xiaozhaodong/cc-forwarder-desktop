package service

import (
	"context"
	"testing"
	"time"

	"cc-forwarder/internal/store"
)

func TestAccountPoolService_RecordAccountSoftFailure_TriggersCooldownAfterThreshold(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	svc.softFailureTracker.now = func() time.Time { return base }

	acc, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "acc-soft-server",
		CredentialRaw: "sk-soft-server",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := svc.RecordAccountSoftFailure(ctx, acc.ID, "server busy", accountSoftFailureCategoryServerError, 0); err != nil {
			t.Fatalf("RecordAccountSoftFailure attempt %d failed: %v", attempt+1, err)
		}
		current, err := st.GetAccount(ctx, acc.ID)
		if err != nil {
			t.Fatalf("GetAccount failed: %v", err)
		}
		if current.State != "active" || current.CooldownUntil != nil || current.FailCount != 0 {
			t.Fatalf("expected no cooldown before threshold, got %+v", current)
		}
	}

	if err := svc.RecordAccountSoftFailure(ctx, acc.ID, "server busy", accountSoftFailureCategoryServerError, 0); err != nil {
		t.Fatalf("RecordAccountSoftFailure third attempt failed: %v", err)
	}
	current, err := st.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if current.State != "cooldown" {
		t.Fatalf("expected cooldown after threshold, got state=%s", current.State)
	}
	if current.FailCount != 1 {
		t.Fatalf("expected fail_count=1 after cooldown trigger, got %d", current.FailCount)
	}
	if current.CooldownUntil == nil {
		t.Fatal("expected cooldown_until to be set")
	}
	if got, want := current.CooldownUntil.Sub(base), defaultAccountServerErrorCooldown; got != want {
		t.Fatalf("unexpected cooldown duration: got %v want %v", got, want)
	}
}

func TestAccountPoolService_RecordAccountSoftFailure_UsesRetryAfterForRateLimit(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 8, 13, 0, 0, 0, time.FixedZone("CST", 8*3600))
	svc.softFailureTracker.now = func() time.Time { return base }

	acc, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "acc-soft-rate",
		CredentialRaw: "sk-soft-rate",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := svc.RecordAccountSoftFailure(ctx, acc.ID, "rate limited", accountSoftFailureCategoryRateLimit, 45*time.Second); err != nil {
			t.Fatalf("RecordAccountSoftFailure attempt %d failed: %v", attempt+1, err)
		}
	}
	if err := svc.RecordAccountSoftFailure(ctx, acc.ID, "rate limited", accountSoftFailureCategoryRateLimit, 45*time.Second); err != nil {
		t.Fatalf("RecordAccountSoftFailure third attempt failed: %v", err)
	}

	current, err := st.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if current.CooldownUntil == nil {
		t.Fatal("expected cooldown_until to be set")
	}
	if got := current.CooldownUntil.Sub(base); got != 45*time.Second {
		t.Fatalf("unexpected rate limit cooldown: got %v want %v", got, 45*time.Second)
	}
}

func TestAccountPoolService_MarkAccountSuccess_ClearsSoftFailureWindow(t *testing.T) {
	svc, st := newTestAccountPoolServiceWithStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 8, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))
	svc.softFailureTracker.now = func() time.Time { return base }

	acc, err := st.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "acc-soft-clear",
		CredentialRaw: "sk-soft-clear",
		Priority:      10,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := svc.RecordAccountSoftFailure(ctx, acc.ID, "server busy", accountSoftFailureCategoryServerError, 0); err != nil {
			t.Fatalf("RecordAccountSoftFailure attempt %d failed: %v", attempt+1, err)
		}
	}

	if err := svc.MarkAccountSuccess(ctx, acc.ID); err != nil {
		t.Fatalf("MarkAccountSuccess failed: %v", err)
	}
	if err := svc.RecordAccountSoftFailure(ctx, acc.ID, "server busy", accountSoftFailureCategoryServerError, 0); err != nil {
		t.Fatalf("RecordAccountSoftFailure after success failed: %v", err)
	}

	current, err := st.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if current.State != "active" {
		t.Fatalf("expected account to remain active after reset, got state=%s", current.State)
	}
	if current.FailCount != 0 {
		t.Fatalf("expected fail_count to remain 0 after reset, got %d", current.FailCount)
	}
}
