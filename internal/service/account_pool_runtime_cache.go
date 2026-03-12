package service

import (
	"sort"
	"sync"
	"time"

	"cc-forwarder/internal/store"
)

type accountRuntimeCache struct {
	mu            sync.RWMutex
	byID          map[int64]*store.UpstreamAccountRecord
	stateVersions map[int64]uint64
	ordered       []*store.UpstreamAccountRecord
	activeTier    int
	activeAccount int64
}

type accountRuntimeStateSnapshot struct {
	enabled       bool
	state         string
	cooldownUntil *time.Time
	failCount     int
	lastSuccessAt *time.Time
	lastError     string
	updatedAt     time.Time
	stateVersion  uint64
}

func newAccountRuntimeCache() *accountRuntimeCache {
	return &accountRuntimeCache{
		byID:          make(map[int64]*store.UpstreamAccountRecord),
		stateVersions: make(map[int64]uint64),
	}
}

func (c *accountRuntimeCache) replaceAll(records []*store.UpstreamAccountRecord) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	existingVersions := c.stateVersions
	c.byID = make(map[int64]*store.UpstreamAccountRecord, len(records))
	c.stateVersions = make(map[int64]uint64, len(records))
	for _, record := range records {
		if record == nil || record.ID <= 0 {
			continue
		}
		c.byID[record.ID] = cloneUpstreamAccountRecord(record)
		if existing, ok := existingVersions[record.ID]; ok && existing > 0 {
			c.stateVersions[record.ID] = existing
		} else {
			c.stateVersions[record.ID] = 1
		}
	}
	if c.activeAccount > 0 {
		if _, ok := c.byID[c.activeAccount]; !ok {
			c.activeAccount = 0
		}
	}
	if c.activeTier > 0 && !c.hasPriorityLocked(c.activeTier) {
		c.activeTier = 0
	}
	c.rebuildOrderedLocked()
}

func (c *accountRuntimeCache) upsert(record *store.UpstreamAccountRecord) {
	if c == nil || record == nil || record.ID <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID[record.ID] = cloneUpstreamAccountRecord(record)
	if _, ok := c.stateVersions[record.ID]; !ok {
		c.stateVersions[record.ID] = 1
	}
	c.rebuildOrderedLocked()
}

func (c *accountRuntimeCache) mergeRecordPreservingRuntimeState(record *store.UpstreamAccountRecord) bool {
	if c == nil || record == nil || record.ID <= 0 {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	current := c.byID[record.ID]
	if current == nil {
		return false
	}

	runtimeEnabled := current.Enabled
	runtimeState := current.State
	runtimeCooldownUntil := cloneTimePtr(current.CooldownUntil)
	runtimeFailCount := current.FailCount
	runtimeLastSuccessAt := cloneTimePtr(current.LastSuccessAt)
	runtimeLastError := current.LastError
	runtimeUpdatedAt := current.UpdatedAt

	merged := cloneUpstreamAccountRecord(record)
	merged.Enabled = runtimeEnabled
	merged.State = runtimeState
	merged.CooldownUntil = runtimeCooldownUntil
	merged.FailCount = runtimeFailCount
	merged.LastSuccessAt = runtimeLastSuccessAt
	merged.LastError = runtimeLastError
	merged.UpdatedAt = runtimeUpdatedAt

	c.byID[record.ID] = merged
	c.rebuildOrderedLocked()
	return true
}

func (c *accountRuntimeCache) remove(id int64) {
	if c == nil || id <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byID, id)
	delete(c.stateVersions, id)
	if c.activeAccount == id {
		c.activeAccount = 0
	}
	if c.activeTier > 0 && !c.hasPriorityLocked(c.activeTier) {
		c.activeTier = 0
	}
	c.rebuildOrderedLocked()
}

func (c *accountRuntimeCache) applyPriorityUpdates(updates map[int64]int) {
	if c == nil || len(updates) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for id, priority := range updates {
		record := c.byID[id]
		if record == nil {
			continue
		}
		record.Priority = priority
	}
	c.activeTier = 0
	c.activeAccount = 0
	c.rebuildOrderedLocked()
}

func (c *accountRuntimeCache) selectAccount(id int64) bool {
	if c == nil || id <= 0 {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.byID[id]
	if record == nil {
		return false
	}

	changed := c.activeTier != record.Priority || c.activeAccount != id
	c.activeTier = record.Priority
	c.activeAccount = id
	return changed
}

func (c *accountRuntimeCache) resolveActiveSelection(prepared []*rankedPriorityTier) (*rankedPriorityTier, int) {
	if c == nil {
		return selectFirstEligibleTier(prepared), 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	selectedTier, selectedIndex := c.resolveActiveSelectionLocked(prepared)
	if selectedTier == nil {
		c.activeAccount = 0
		return nil, -1
	}

	if selectedIndex < 0 || selectedIndex >= len(selectedTier.eligible) {
		selectedIndex = 0
	}
	c.activeTier = selectedTier.priority
	if len(selectedTier.eligible) > 0 && selectedTier.eligible[selectedIndex] != nil && selectedTier.eligible[selectedIndex].account != nil {
		c.activeAccount = selectedTier.eligible[selectedIndex].account.ID
	}
	return selectedTier, selectedIndex
}

func (c *accountRuntimeCache) resolveActiveSelectionLocked(prepared []*rankedPriorityTier) (*rankedPriorityTier, int) {
	if len(prepared) == 0 {
		return nil, -1
	}

	if c.activeTier > 0 {
		for _, tier := range prepared {
			if tier == nil || tier.priority != c.activeTier {
				continue
			}
			if len(tier.eligible) > 0 {
				if c.activeAccount > 0 {
					for idx, candidate := range tier.eligible {
						if candidate != nil && candidate.account != nil && candidate.account.ID == c.activeAccount {
							return tier, idx
						}
					}
				}
				return tier, 0
			}
			break
		}

		for _, tier := range prepared {
			if tier == nil || len(tier.eligible) == 0 || tier.priority < c.activeTier {
				continue
			}
			return tier, 0
		}
		return nil, -1
	}

	selectedTier := selectFirstEligibleTier(prepared)
	if selectedTier == nil {
		return nil, -1
	}
	return selectedTier, 0
}

func selectFirstEligibleTier(prepared []*rankedPriorityTier) *rankedPriorityTier {
	for _, tier := range prepared {
		if tier != nil && len(tier.eligible) > 0 {
			return tier
		}
	}
	return nil
}

func (c *accountRuntimeCache) hasPriorityLocked(priority int) bool {
	for _, record := range c.byID {
		if record != nil && record.Priority == priority {
			return true
		}
	}
	return false
}

func (c *accountRuntimeCache) matchesStateVersion(id int64, version uint64) bool {
	if c == nil || id <= 0 || version == 0 {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	current, ok := c.stateVersions[id]
	return ok && current == version
}

func (c *accountRuntimeCache) get(id int64) (*store.UpstreamAccountRecord, bool) {
	if c == nil || id <= 0 {
		return nil, false
	}

	now := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	record, ok := c.byID[id]
	if !ok || record == nil {
		return nil, false
	}
	return normalizeAccountRuntimeView(cloneUpstreamAccountRecord(record), now), true
}

func (c *accountRuntimeCache) activeSelectionAccountID() (int64, bool) {
	if c == nil {
		return 0, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.activeAccount <= 0 {
		return 0, false
	}
	record, ok := c.byID[c.activeAccount]
	if !ok || record == nil {
		return 0, false
	}
	return c.activeAccount, true
}

func (c *accountRuntimeCache) list(includeDisabled bool) []*store.UpstreamAccountRecord {
	if c == nil {
		return nil
	}

	now := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	records := make([]*store.UpstreamAccountRecord, 0, len(c.ordered))
	for _, record := range c.ordered {
		if record == nil {
			continue
		}
		if !includeDisabled && !record.Enabled {
			continue
		}
		records = append(records, normalizeAccountRuntimeView(cloneUpstreamAccountRecord(record), now))
	}
	return records
}

func (c *accountRuntimeCache) listSchedulable(now time.Time) []*store.UpstreamAccountRecord {
	if c == nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	records := make([]*store.UpstreamAccountRecord, 0, len(c.ordered))
	for _, record := range c.ordered {
		if record == nil || !record.Enabled || record.State == "disabled_auth" {
			continue
		}
		if record.CooldownUntil != nil && record.CooldownUntil.After(now) {
			continue
		}
		records = append(records, normalizeAccountRuntimeView(cloneUpstreamAccountRecord(record), now))
	}
	return records
}

func (c *accountRuntimeCache) markSuccessIfNoNewerFailure(id int64, successAt, attemptStartedAt time.Time) (bool, uint64) {
	if c == nil || id <= 0 {
		return false, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.byID[id]
	if record == nil {
		return false, 0
	}
	if record.UpdatedAt.After(attemptStartedAt) && (record.State == "cooldown" || record.State == "disabled_auth") {
		return false, 0
	}

	record.FailCount = 0
	record.State = "active"
	record.CooldownUntil = nil
	record.LastSuccessAt = cloneTimePtr(&successAt)
	record.LastError = ""
	record.UpdatedAt = successAt
	c.activeTier = record.Priority
	c.activeAccount = id
	return true, c.bumpStateVersionLocked(id)
}

func (c *accountRuntimeCache) markSuccess(id int64, successAt time.Time) (bool, uint64) {
	if c == nil || id <= 0 {
		return false, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.byID[id]
	if record == nil {
		return false, 0
	}

	record.FailCount = 0
	record.State = "active"
	record.CooldownUntil = nil
	record.LastSuccessAt = cloneTimePtr(&successAt)
	record.LastError = ""
	record.UpdatedAt = successAt
	c.activeTier = record.Priority
	c.activeAccount = id
	return true, c.bumpStateVersionLocked(id)
}

func (c *accountRuntimeCache) markAuthFailed(id int64, reason string, failedAt time.Time) (bool, uint64) {
	if c == nil || id <= 0 {
		return false, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.byID[id]
	if record == nil {
		return false, 0
	}

	record.Enabled = false
	record.State = "disabled_auth"
	record.CooldownUntil = nil
	record.LastError = reason
	record.UpdatedAt = failedAt
	if c.activeAccount == id {
		c.activeAccount = 0
	}
	return true, c.bumpStateVersionLocked(id)
}

func (c *accountRuntimeCache) markTransientFailure(id int64, reason string, cooldownUntil, failedAt time.Time) (bool, uint64) {
	if c == nil || id <= 0 {
		return false, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.byID[id]
	if record == nil {
		return false, 0
	}

	record.FailCount++
	record.State = "cooldown"
	record.CooldownUntil = cloneTimePtr(&cooldownUntil)
	record.LastError = reason
	record.UpdatedAt = failedAt
	if c.activeAccount == id {
		c.activeAccount = 0
	}
	return true, c.bumpStateVersionLocked(id)
}

func (c *accountRuntimeCache) applyToggle(id int64, enabled bool, changedAt time.Time) (bool, uint64, accountRuntimeStateSnapshot) {
	if c == nil || id <= 0 {
		return false, 0, accountRuntimeStateSnapshot{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := c.byID[id]
	if record == nil {
		return false, 0, accountRuntimeStateSnapshot{}
	}

	snapshot := accountRuntimeStateSnapshot{
		enabled:       record.Enabled,
		state:         record.State,
		cooldownUntil: cloneTimePtr(record.CooldownUntil),
		failCount:     record.FailCount,
		lastSuccessAt: cloneTimePtr(record.LastSuccessAt),
		lastError:     record.LastError,
		updatedAt:     record.UpdatedAt,
		stateVersion:  c.stateVersions[id],
	}

	record.Enabled = enabled
	if enabled {
		record.State = "active"
		record.FailCount = 0
		record.CooldownUntil = nil
		record.LastError = ""
	} else if c.activeAccount == id {
		c.activeAccount = 0
	}
	record.UpdatedAt = changedAt
	if c.activeTier > 0 && !c.hasPriorityLocked(c.activeTier) {
		c.activeTier = 0
	}
	return true, c.bumpStateVersionLocked(id), snapshot
}

func (c *accountRuntimeCache) restoreRuntimeStateIfVersion(id int64, expectedVersion uint64, snapshot accountRuntimeStateSnapshot) bool {
	if c == nil || id <= 0 || expectedVersion == 0 {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	current := c.byID[id]
	if current == nil {
		return false
	}
	if c.stateVersions[id] != expectedVersion {
		return false
	}

	current.Enabled = snapshot.enabled
	current.State = snapshot.state
	current.CooldownUntil = cloneTimePtr(snapshot.cooldownUntil)
	current.FailCount = snapshot.failCount
	current.LastSuccessAt = cloneTimePtr(snapshot.lastSuccessAt)
	current.LastError = snapshot.lastError
	current.UpdatedAt = snapshot.updatedAt
	if snapshot.stateVersion > 0 {
		c.stateVersions[id] = snapshot.stateVersion
	}
	return true
}

func (c *accountRuntimeCache) bumpStateVersionLocked(id int64) uint64 {
	current := c.stateVersions[id]
	if current == 0 {
		current = 1
	} else {
		current++
	}
	c.stateVersions[id] = current
	return current
}

func (c *accountRuntimeCache) rebuildOrderedLocked() {
	c.ordered = c.ordered[:0]
	if len(c.byID) == 0 {
		return
	}

	c.ordered = make([]*store.UpstreamAccountRecord, 0, len(c.byID))
	for _, record := range c.byID {
		c.ordered = append(c.ordered, record)
	}
	sort.SliceStable(c.ordered, func(i, j int) bool {
		left := c.ordered[i]
		right := c.ordered[j]
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
}

func cloneUpstreamAccountRecord(record *store.UpstreamAccountRecord) *store.UpstreamAccountRecord {
	if record == nil {
		return nil
	}

	cloned := *record
	cloned.CooldownUntil = cloneTimePtr(record.CooldownUntil)
	cloned.LastSuccessAt = cloneTimePtr(record.LastSuccessAt)
	cloned.Quota5HUsedPercent = cloneFloat64Ptr(record.Quota5HUsedPercent)
	cloned.Quota5HResetAt = cloneTimePtr(record.Quota5HResetAt)
	cloned.QuotaWeeklyUsedPercent = cloneFloat64Ptr(record.QuotaWeeklyUsedPercent)
	cloned.QuotaWeeklyResetAt = cloneTimePtr(record.QuotaWeeklyResetAt)
	cloned.QuotaRefreshedAt = cloneTimePtr(record.QuotaRefreshedAt)
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func normalizeAccountRuntimeView(record *store.UpstreamAccountRecord, now time.Time) *store.UpstreamAccountRecord {
	if record == nil {
		return nil
	}
	if record.State == "cooldown" && record.CooldownUntil != nil && !record.CooldownUntil.After(now) {
		record.State = "active"
		record.CooldownUntil = nil
	}
	return record
}
