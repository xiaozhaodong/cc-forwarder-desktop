package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cc-forwarder/internal/accountauth"
	"cc-forwarder/internal/store"
)

const (
	accountScheduleDecisionSelected = "selected"
	accountScheduleDecisionEligible = "eligible"
	accountScheduleDecisionSkipped  = "skipped"
)

const (
	AccountScheduleOutcomeSuccess                        = "success"
	AccountScheduleOutcomeAuthFailed                     = "auth_failed"
	AccountScheduleOutcomeTransientFailure               = "transient_failure"
	AccountScheduleOutcomePassthroughNoAvailableProvider = "passthrough_no_available_providers"
	AccountScheduleOutcomePassthroughOther4xx            = "passthrough_other_4xx"
	AccountScheduleOutcomeNoSchedulableAccounts          = "no_schedulable_accounts"
	latestAccountScheduleSnapshotPendingTTL              = 5 * time.Minute

	accountScheduleOutcomeSuccess                        = AccountScheduleOutcomeSuccess
	accountScheduleOutcomeAuthFailed                     = AccountScheduleOutcomeAuthFailed
	accountScheduleOutcomeTransientFailure               = AccountScheduleOutcomeTransientFailure
	accountScheduleOutcomePassthroughNoAvailableProvider = AccountScheduleOutcomePassthroughNoAvailableProvider
	accountScheduleOutcomePassthroughOther4xx            = AccountScheduleOutcomePassthroughOther4xx
	accountScheduleOutcomeNoSchedulableAccounts          = AccountScheduleOutcomeNoSchedulableAccounts
)

type AccountScheduleCandidateDecision struct {
	AccountID               int64      `json:"account_id"`
	AccountName             string     `json:"account_name"`
	ProviderType            string     `json:"provider_type"`
	Priority                int        `json:"priority"`
	TierIndex               int        `json:"tier_index"`
	TierLabel               string     `json:"tier_label"`
	QuotaStatus             string     `json:"quota_status"`
	EffectiveQuotaRemaining *float64   `json:"effective_quota_remaining,omitempty"`
	FailCount               int        `json:"fail_count"`
	LastSuccessAt           *time.Time `json:"last_success_at,omitempty"`
	Decision                string     `json:"decision"`
	Reason                  string     `json:"reason"`
	ReasonDetail            string     `json:"reason_detail"`
	RuntimeOutcome          string     `json:"runtime_outcome,omitempty"`
	RuntimeError            string     `json:"runtime_error,omitempty"`
}

type LatestAccountScheduleSnapshot struct {
	RequestID               string                             `json:"request_id"`
	CapturedAt              time.Time                          `json:"captured_at"`
	UpdatedAt               time.Time                          `json:"updated_at"`
	RequestPath             string                             `json:"request_path"`
	SelectedPriority        int                                `json:"selected_priority"`
	SelectedTierIndex       int                                `json:"selected_tier_index"`
	SelectedTierLabel       string                             `json:"selected_tier_label"`
	DegradedToLowerPriority bool                               `json:"degraded_to_lower_priority"`
	SelectedAccountID       int64                              `json:"selected_account_id"`
	SelectedAccountName     string                             `json:"selected_account_name"`
	FinalOutcome            string                             `json:"final_outcome"`
	FinalError              string                             `json:"final_error"`
	Summary                 string                             `json:"summary"`
	Candidates              []AccountScheduleCandidateDecision `json:"candidates"`
}

type rankedSchedulableAccount struct {
	account                 *store.UpstreamAccountRecord
	quotaBucket             int
	effectiveQuotaRemaining *float64
	skipReason              string
	skipReasonDetail        string
}

type rankedPriorityTier struct {
	priority int
	index    int
	eligible []*rankedSchedulableAccount
	skipped  []*rankedSchedulableAccount
}

type latestAccountScheduleSnapshotStore struct {
	mu      sync.RWMutex
	latest  *LatestAccountScheduleSnapshot
	pending map[string]*LatestAccountScheduleSnapshot
}

func newLatestAccountScheduleSnapshotStore() *latestAccountScheduleSnapshotStore {
	return &latestAccountScheduleSnapshotStore{
		pending: make(map[string]*LatestAccountScheduleSnapshot),
	}
}

func (s *latestAccountScheduleSnapshotStore) saveDraft(snapshot *LatestAccountScheduleSnapshot) {
	if s == nil || snapshot == nil {
		return
	}

	clone := cloneLatestAccountScheduleSnapshot(snapshot)
	now := time.Now()
	if clone.CapturedAt.IsZero() {
		clone.CapturedAt = now
	}
	clone.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-latestAccountScheduleSnapshotPendingTTL)
	for id, pending := range s.pending {
		if pending == nil || pending.CapturedAt.Before(cutoff) {
			delete(s.pending, id)
		}
	}
	if clone.RequestID != "" {
		s.pending[clone.RequestID] = cloneLatestAccountScheduleSnapshot(clone)
	}
	s.latest = clone
}

func (s *latestAccountScheduleSnapshotStore) complete(requestID string, accountID int64, accountName, outcome, finalError string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var snapshot *LatestAccountScheduleSnapshot
	if requestID != "" {
		if pending, ok := s.pending[requestID]; ok {
			snapshot = cloneLatestAccountScheduleSnapshot(pending)
			delete(s.pending, requestID)
		}
	}
	if snapshot == nil && s.latest != nil {
		snapshot = cloneLatestAccountScheduleSnapshot(s.latest)
	}
	if snapshot == nil {
		return
	}

	snapshot.UpdatedAt = time.Now()
	if outcome != "" {
		snapshot.FinalOutcome = outcome
	}
	if finalError != "" {
		snapshot.FinalError = finalError
	}
	if accountID > 0 {
		snapshot.SelectedAccountID = accountID
	}
	if strings.TrimSpace(accountName) != "" {
		snapshot.SelectedAccountName = accountName
	}

	for idx := range snapshot.Candidates {
		candidate := &snapshot.Candidates[idx]
		if candidate.AccountID != accountID {
			continue
		}
		candidate.RuntimeOutcome = outcome
		candidate.RuntimeError = finalError
		break
	}

	s.latest = snapshot
}

func (s *latestAccountScheduleSnapshotStore) getLatest() *LatestAccountScheduleSnapshot {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneLatestAccountScheduleSnapshot(s.latest)
}

func cloneLatestAccountScheduleSnapshot(snapshot *LatestAccountScheduleSnapshot) *LatestAccountScheduleSnapshot {
	if snapshot == nil {
		return nil
	}

	clone := *snapshot
	if len(snapshot.Candidates) > 0 {
		clone.Candidates = make([]AccountScheduleCandidateDecision, len(snapshot.Candidates))
		copy(clone.Candidates, snapshot.Candidates)
	}
	return &clone
}

func (s *AccountPoolService) PrepareSchedulableAccounts(ctx context.Context, requestID, requestPath string) ([]*store.UpstreamAccountRecord, error) {
	return s.prepareSchedulableAccounts(ctx, requestID, requestPath)
}

func (s *AccountPoolService) GetLatestAccountScheduleSnapshot(ctx context.Context) (*LatestAccountScheduleSnapshot, error) {
	_ = ctx
	if s.scheduleSnapshots == nil {
		return nil, nil
	}
	return s.scheduleSnapshots.getLatest(), nil
}

func (s *AccountPoolService) CompleteLatestScheduleSnapshot(ctx context.Context, requestID string, accountID int64, accountName, outcome, finalError string) error {
	_ = ctx
	if s.scheduleSnapshots == nil {
		return nil
	}
	s.scheduleSnapshots.complete(requestID, accountID, accountName, outcome, finalError)
	return nil
}

func (s *AccountPoolService) prepareSchedulableAccounts(ctx context.Context, requestID, requestPath string) ([]*store.UpstreamAccountRecord, error) {
	now := time.Now()
	accounts, err := s.store.ListSchedulableAccounts(ctx, now)
	if err != nil {
		return nil, err
	}

	ordered, snapshot := rankSchedulableAccounts(accounts, now, requestID, requestPath)
	if s.scheduleSnapshots != nil && snapshot != nil {
		s.scheduleSnapshots.saveDraft(snapshot)
	}
	return ordered, nil
}

func rankSchedulableAccounts(accounts []*store.UpstreamAccountRecord, now time.Time, requestID, requestPath string) ([]*store.UpstreamAccountRecord, *LatestAccountScheduleSnapshot) {
	preparedTiers := preparePriorityTiers(accounts, now)
	snapshot := &LatestAccountScheduleSnapshot{
		RequestID:   requestID,
		CapturedAt:  now,
		UpdatedAt:   now,
		RequestPath: requestPath,
	}

	selectedTier := (*rankedPriorityTier)(nil)
	for idx := range preparedTiers {
		if len(preparedTiers[idx].eligible) > 0 {
			selectedTier = preparedTiers[idx]
			break
		}
	}

	if selectedTier == nil {
		snapshot.FinalOutcome = accountScheduleOutcomeNoSchedulableAccounts
		snapshot.Summary = "当前没有可调度账号：所有候选均因状态或额度暂不可调度"
		for _, tier := range preparedTiers {
			appendSkippedCandidates(snapshot, tier, tier.skipped)
		}
		return nil, snapshot
	}

	snapshot.SelectedPriority = selectedTier.priority
	snapshot.SelectedTierIndex = selectedTier.index
	snapshot.SelectedTierLabel = accountPriorityTierLabel(selectedTier.index)
	snapshot.DegradedToLowerPriority = selectedTier.index > 1
	if len(selectedTier.eligible) > 0 {
		snapshot.SelectedAccountID = selectedTier.eligible[0].account.ID
		snapshot.SelectedAccountName = displayAccountName(selectedTier.eligible[0].account)
	}
	snapshot.Summary = buildScheduleSummary(selectedTier, snapshot.SelectedAccountName)

	ordered := make([]*store.UpstreamAccountRecord, 0, len(selectedTier.eligible))
	for _, tier := range preparedTiers {
		if tier.index < selectedTier.index {
			appendSkippedCandidates(snapshot, tier, tier.skipped)
			continue
		}
		if tier.index == selectedTier.index {
			for idx, candidate := range tier.eligible {
				decision := accountScheduleDecisionEligible
				reason := "same_tier_lower_rank"
				detail := fmt.Sprintf("同层排序位次 %d", idx+1)
				if idx == 0 {
					decision = accountScheduleDecisionSelected
					reason = "highest_ranked_in_selected_tier"
					detail = "当前优先级组内排序第一"
				}
				snapshot.Candidates = append(snapshot.Candidates, buildCandidateDecision(candidate, tier, decision, reason, detail))
				ordered = append(ordered, candidate.account)
			}
			appendSkippedCandidates(snapshot, tier, tier.skipped)
			continue
		}

		for _, candidate := range tier.eligible {
			snapshot.Candidates = append(snapshot.Candidates, buildCandidateDecision(candidate, tier, accountScheduleDecisionSkipped, "higher_priority_tier_selected", "更高优先级组已有可用账号，本轮未启用该层"))
		}
		appendSkippedCandidates(snapshot, tier, tier.skipped)
	}

	return ordered, snapshot
}

func preparePriorityTiers(accounts []*store.UpstreamAccountRecord, now time.Time) []*rankedPriorityTier {
	grouped := groupAccountsByPriority(accounts)
	prepared := make([]*rankedPriorityTier, 0, len(grouped))
	for index, tier := range grouped {
		eligible, skipped := rankPriorityTier(tier.accounts, now)
		prepared = append(prepared, &rankedPriorityTier{
			priority: tier.priority,
			index:    index + 1,
			eligible: eligible,
			skipped:  skipped,
		})
	}
	return prepared
}

type groupedPriorityTier struct {
	priority int
	accounts []*store.UpstreamAccountRecord
}

func groupAccountsByPriority(accounts []*store.UpstreamAccountRecord) []groupedPriorityTier {
	sorted := append([]*store.UpstreamAccountRecord(nil), accounts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.ID < right.ID
	})

	result := make([]groupedPriorityTier, 0)
	for _, account := range sorted {
		if account == nil {
			continue
		}
		if len(result) == 0 || result[len(result)-1].priority != account.Priority {
			result = append(result, groupedPriorityTier{priority: account.Priority, accounts: []*store.UpstreamAccountRecord{account}})
			continue
		}
		result[len(result)-1].accounts = append(result[len(result)-1].accounts, account)
	}
	return result
}

func rankPriorityTier(accounts []*store.UpstreamAccountRecord, now time.Time) ([]*rankedSchedulableAccount, []*rankedSchedulableAccount) {
	eligible := make([]*rankedSchedulableAccount, 0)
	skipped := make([]*rankedSchedulableAccount, 0)
	for _, account := range accounts {
		candidate := classifySchedulableAccount(account, now)
		if candidate.skipReason != "" {
			skipped = append(skipped, candidate)
			continue
		}
		eligible = append(eligible, candidate)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		left := eligible[i]
		right := eligible[j]
		if left.quotaBucket != right.quotaBucket {
			return left.quotaBucket < right.quotaBucket
		}
		if cmp := compareEffectiveQuotaRemaining(left.effectiveQuotaRemaining, right.effectiveQuotaRemaining); cmp != 0 {
			return cmp > 0
		}
		if left.account.FailCount != right.account.FailCount {
			return left.account.FailCount < right.account.FailCount
		}
		if cmp := compareOptionalTimeDesc(left.account.LastSuccessAt, right.account.LastSuccessAt); cmp != 0 {
			return cmp > 0
		}
		return left.account.ID < right.account.ID
	})

	return eligible, skipped
}

func classifySchedulableAccount(account *store.UpstreamAccountRecord, now time.Time) *rankedSchedulableAccount {
	candidate := &rankedSchedulableAccount{account: account}
	if account == nil {
		candidate.skipReason = "invalid_account"
		candidate.skipReasonDetail = "账号记录为空，无法参与调度"
		return candidate
	}

	providerType := accountauth.NormalizeProviderType(account.ProviderType)
	quotaStatus := normalizeQuotaStatus(account.QuotaStatus)
	candidate.effectiveQuotaRemaining = effectiveQuotaRemaining(account)

	if providerType == accountauth.ProviderAPIKey {
		candidate.quotaBucket = 0
		return candidate
	}

	if quotaStatus == "exhausted" && hasFutureQuotaReset(account, now) {
		candidate.skipReason = "quota_exhausted_until_reset"
		candidate.skipReasonDetail = "额度已耗尽且重置时间未到，本轮暂不参与调度"
		return candidate
	}

	switch quotaStatus {
	case "ok":
		candidate.quotaBucket = 1
	case "", "pending", "unavailable":
		candidate.quotaBucket = 2
	case "exhausted":
		candidate.quotaBucket = 3
	default:
		candidate.quotaBucket = 4
	}

	return candidate
}

func effectiveQuotaRemaining(account *store.UpstreamAccountRecord) *float64 {
	if account == nil {
		return nil
	}
	if accountauth.NormalizeProviderType(account.ProviderType) == accountauth.ProviderAPIKey {
		return nil
	}

	planType := strings.TrimSpace(strings.ToLower(account.PlanType))
	if planType == "free" {
		return usedPercentToRemaining(account.QuotaWeeklyUsedPercent)
	}

	remaining5H := usedPercentToRemaining(account.Quota5HUsedPercent)
	remainingWeekly := usedPercentToRemaining(account.QuotaWeeklyUsedPercent)
	if remaining5H != nil && remainingWeekly != nil {
		if *remaining5H <= *remainingWeekly {
			return remaining5H
		}
		return remainingWeekly
	}
	if remaining5H != nil {
		return remaining5H
	}
	return remainingWeekly
}

func usedPercentToRemaining(used *float64) *float64 {
	if used == nil {
		return nil
	}
	remaining := 100 - *used
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return &remaining
}

func hasFutureQuotaReset(account *store.UpstreamAccountRecord, now time.Time) bool {
	if account == nil {
		return false
	}
	for _, resetAt := range []*time.Time{account.Quota5HResetAt, account.QuotaWeeklyResetAt} {
		if resetAt != nil && resetAt.After(now) {
			return true
		}
	}
	return false
}

func normalizeQuotaStatus(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func compareEffectiveQuotaRemaining(left, right *float64) int {
	switch {
	case left != nil && right == nil:
		return 1
	case left == nil && right != nil:
		return -1
	case left == nil && right == nil:
		return 0
	case *left > *right:
		return 1
	case *left < *right:
		return -1
	default:
		return 0
	}
}

func compareOptionalTimeDesc(left, right *time.Time) int {
	switch {
	case left != nil && right == nil:
		return 1
	case left == nil && right != nil:
		return -1
	case left == nil && right == nil:
		return 0
	case left.After(*right):
		return 1
	case left.Before(*right):
		return -1
	default:
		return 0
	}
}

func accountPriorityTierLabel(index int) string {
	switch index {
	case 1:
		return "主组"
	case 2:
		return "备组"
	case 3:
		return "兜底组"
	default:
		if index <= 0 {
			return "未分层"
		}
		return fmt.Sprintf("第 %d 层", index)
	}
}

func displayAccountName(account *store.UpstreamAccountRecord) string {
	if account == nil {
		return ""
	}
	if name := strings.TrimSpace(account.AccountName); name != "" {
		return name
	}
	return fmt.Sprintf("account-%d", account.ID)
}

func buildCandidateDecision(candidate *rankedSchedulableAccount, tier *rankedPriorityTier, decision, reason, reasonDetail string) AccountScheduleCandidateDecision {
	item := AccountScheduleCandidateDecision{
		Decision:     decision,
		Reason:       reason,
		ReasonDetail: reasonDetail,
	}
	if candidate == nil || candidate.account == nil || tier == nil {
		return item
	}
	item.AccountID = candidate.account.ID
	item.AccountName = displayAccountName(candidate.account)
	item.ProviderType = accountauth.NormalizeProviderType(candidate.account.ProviderType)
	item.Priority = candidate.account.Priority
	item.TierIndex = tier.index
	item.TierLabel = accountPriorityTierLabel(tier.index)
	item.QuotaStatus = normalizeQuotaStatus(candidate.account.QuotaStatus)
	item.EffectiveQuotaRemaining = candidate.effectiveQuotaRemaining
	item.FailCount = candidate.account.FailCount
	item.LastSuccessAt = candidate.account.LastSuccessAt
	return item
}

func appendSkippedCandidates(snapshot *LatestAccountScheduleSnapshot, tier *rankedPriorityTier, candidates []*rankedSchedulableAccount) {
	if snapshot == nil || tier == nil {
		return
	}
	for _, candidate := range candidates {
		reason := candidate.skipReason
		detail := candidate.skipReasonDetail
		if reason == "" {
			reason = "not_selected"
			detail = "当前不参与调度"
		}
		snapshot.Candidates = append(snapshot.Candidates, buildCandidateDecision(candidate, tier, accountScheduleDecisionSkipped, reason, detail))
	}
}

func buildScheduleSummary(selectedTier *rankedPriorityTier, accountName string) string {
	if selectedTier == nil {
		return "当前没有可调度账号"
	}
	if selectedTier.index <= 1 {
		return fmt.Sprintf("%s可用，优先选择 %s", accountPriorityTierLabel(selectedTier.index), accountName)
	}
	return fmt.Sprintf("更高优先级组当前不可用，已降级到%s并优先选择 %s", accountPriorityTierLabel(selectedTier.index), accountName)
}
