package service

import (
	"context"
	"fmt"
	"sort"

	"cc-forwarder/internal/store"
)

type groupedManualFailoverTier struct {
	priority int
	accounts []*store.UpstreamAccountRecord
}

// MoveAccountToTier 将账号移动到指定手动主备层。
func (s *AccountPoolService) MoveAccountToTier(ctx context.Context, accountID int64, targetTierIndex int) (bool, error) {
	if err := s.ensureRuntimeCache(ctx); err != nil {
		return false, err
	}
	if accountID <= 0 {
		return false, fmt.Errorf("无效的账号 ID: %d", accountID)
	}

	accounts, err := s.ListAccounts(ctx, true)
	if err != nil {
		return false, fmt.Errorf("加载账号列表失败: %w", err)
	}

	updates, err := buildManualFailoverPriorityUpdates(accounts, accountID, targetTierIndex)
	if err != nil {
		return false, err
	}
	if len(updates) == 0 {
		return false, nil
	}

	if err := s.store.UpdateAccountPriorities(ctx, updates); err != nil {
		return false, err
	}
	s.runtimeCache.applyPriorityUpdates(updates)
	return true, nil
}

func buildManualFailoverPriorityUpdates(accounts []*store.UpstreamAccountRecord, targetAccountID int64, targetTierIndex int) (map[int64]int, error) {
	tiers := buildManualFailoverTierGroups(accounts)
	if len(tiers) == 0 {
		return nil, fmt.Errorf("当前没有可调整顺序的账号")
	}

	var targetAccount *store.UpstreamAccountRecord
	remainingTiers := make([]groupedManualFailoverTier, 0, len(tiers))
	for _, tier := range tiers {
		nextAccounts := make([]*store.UpstreamAccountRecord, 0, len(tier.accounts))
		for _, account := range tier.accounts {
			if account == nil {
				continue
			}
			if targetAccount == nil && account.ID == targetAccountID {
				targetAccount = account
				continue
			}
			nextAccounts = append(nextAccounts, account)
		}
		if len(nextAccounts) > 0 {
			remainingTiers = append(remainingTiers, groupedManualFailoverTier{
				priority: tier.priority,
				accounts: nextAccounts,
			})
		}
	}
	if targetAccount == nil {
		return nil, fmt.Errorf("账号不存在: %d", targetAccountID)
	}

	if targetTierIndex < 0 {
		targetTierIndex = 0
	}
	if targetTierIndex > len(remainingTiers) {
		targetTierIndex = len(remainingTiers)
	}

	insertedTier := groupedManualFailoverTier{
		priority: 0,
		accounts: []*store.UpstreamAccountRecord{targetAccount},
	}
	remainingTiers = append(remainingTiers[:targetTierIndex], append([]groupedManualFailoverTier{insertedTier}, remainingTiers[targetTierIndex:]...)...)

	updates := make(map[int64]int)
	for index, tier := range remainingTiers {
		nextPriority := (index + 1) * 10
		for _, account := range tier.accounts {
			if account == nil || account.ID <= 0 {
				continue
			}
			if account.Priority == nextPriority {
				continue
			}
			updates[account.ID] = nextPriority
		}
	}

	return updates, nil
}

func buildManualFailoverTierGroups(accounts []*store.UpstreamAccountRecord) []groupedManualFailoverTier {
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

	groups := make([]groupedManualFailoverTier, 0)
	for _, account := range sorted {
		if account == nil {
			continue
		}
		if len(groups) == 0 || groups[len(groups)-1].priority != account.Priority {
			groups = append(groups, groupedManualFailoverTier{
				priority: account.Priority,
				accounts: []*store.UpstreamAccountRecord{account},
			})
			continue
		}
		groups[len(groups)-1].accounts = append(groups[len(groups)-1].accounts, account)
	}
	return groups
}
