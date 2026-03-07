package service

import (
	"strings"
	"sync"
	"time"
)

const (
	accountSoftFailureCategoryRateLimit   = "rate_limit"
	accountSoftFailureCategoryServerError = "server_error"

	defaultAccountSoftFailureWindow    = 5 * time.Minute
	defaultAccountSoftFailureThreshold = 3
	defaultAccountRateLimitCooldown    = 180 * time.Second
	defaultAccountServerErrorCooldown  = 120 * time.Second
)

type accountSoftFailureTracker struct {
	mu        sync.Mutex
	events    map[int64][]time.Time
	now       func() time.Time
	window    time.Duration
	threshold int
}

func newAccountSoftFailureTracker() *accountSoftFailureTracker {
	return &accountSoftFailureTracker{
		events:    make(map[int64][]time.Time),
		now:       time.Now,
		window:    defaultAccountSoftFailureWindow,
		threshold: defaultAccountSoftFailureThreshold,
	}
}

func (t *accountSoftFailureTracker) Now() time.Time {
	if t == nil || t.now == nil {
		return time.Now()
	}
	return t.now()
}

func (t *accountSoftFailureTracker) Record(accountID int64) int {
	if t == nil || accountID <= 0 {
		return 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.Now()
	windowStart := now.Add(-t.window)
	valid := make([]time.Time, 0, len(t.events[accountID])+1)
	for _, item := range t.events[accountID] {
		if item.After(windowStart) {
			valid = append(valid, item)
		}
	}
	valid = append(valid, now)
	t.events[accountID] = valid
	return len(valid)
}

func (t *accountSoftFailureTracker) Clear(accountID int64) {
	if t == nil || accountID <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.events, accountID)
}

func resolveAccountSoftFailureCooldown(category string, retryAfter time.Duration) time.Duration {
	switch strings.TrimSpace(strings.ToLower(category)) {
	case accountSoftFailureCategoryRateLimit:
		if retryAfter > 0 {
			return retryAfter
		}
		return defaultAccountRateLimitCooldown
	default:
		return defaultAccountServerErrorCooldown
	}
}
