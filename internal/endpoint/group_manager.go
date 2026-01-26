package endpoint

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"cc-forwarder/config"
)

// GroupInfo represents information about an endpoint group
type GroupInfo struct {
	Name          string
	Priority      int
	IsActive      bool
	CooldownUntil time.Time
	Endpoints     []*Endpoint
	// Manual control states
	ManuallyPaused       bool
	ManualActivationTime time.Time
	// Forced activation states
	ForcedActivation     bool
	ForcedActivationTime time.Time // 强制激活时间
}

// GroupManager manages endpoint groups and their cooldown states
type GroupManager struct {
	groups           map[string]*GroupInfo
	config           *config.Config
	mutex            sync.RWMutex
	cooldownDuration time.Duration
	// Group change notification subscribers
	groupChangeSubscribers []chan string
	subscriberMutex        sync.RWMutex
}

// NewGroupManager creates a new group manager
// v4.0: Support both old Group config and new Failover config
func NewGroupManager(cfg *config.Config) *GroupManager {
	// v4.0: 优先使用 Failover 配置，如果没有则使用 Group 配置（向后兼容）
	cooldownDuration := cfg.Group.Cooldown
	if cfg.Failover.DefaultCooldown > 0 {
		cooldownDuration = cfg.Failover.DefaultCooldown
	}

	return &GroupManager{
		groups:                 make(map[string]*GroupInfo),
		config:                 cfg,
		cooldownDuration:       cooldownDuration,
		groupChangeSubscribers: make([]chan string, 0),
	}
}

// UpdateConfig updates the group manager configuration
// v4.0: Support both old Group config and new Failover config
func (gm *GroupManager) UpdateConfig(cfg *config.Config) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	gm.config = cfg

	// v4.0: 优先使用 Failover 配置，如果没有则使用 Group 配置（向后兼容）
	if cfg.Failover.DefaultCooldown > 0 {
		gm.cooldownDuration = cfg.Failover.DefaultCooldown
	} else {
		gm.cooldownDuration = cfg.Group.Cooldown
	}
}

// UpdateGroups rebuilds group information from endpoints
// v4.0: Automatically creates one group per endpoint
func (gm *GroupManager) UpdateGroups(endpoints []*Endpoint) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	// v5.0: SQLite 模式下需要保留 IsActive 状态
	isSQLiteMode := gm.config.EndpointsStorage.Type == "sqlite"

	// Clear existing groups but preserve cooldown states (and IsActive for SQLite mode)
	oldGroups := make(map[string]*GroupInfo)
	for name, group := range gm.groups {
		// v5.0: SQLite 模式下保留 IsActive 状态
		if isSQLiteMode || (!group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil)) {
			oldGroups[name] = &GroupInfo{
				Name:          group.Name,
				Priority:      group.Priority,
				IsActive:      group.IsActive, // v5.0: 保留激活状态
				CooldownUntil: group.CooldownUntil,
				Endpoints:     nil, // Will be updated
			}
		}
	}

	// Rebuild groups from current endpoints
	newGroups := make(map[string]*GroupInfo)

	for _, ep := range endpoints {
		// v4.0: 自动为每个端点创建一个独立的组
		// 使用端点名作为组名
		groupName := ep.Config.Name

		// 检查是否参与故障转移（从配置中读取，默认为true）
		failoverEnabled := true
		if ep.Config.FailoverEnabled != nil {
			failoverEnabled = *ep.Config.FailoverEnabled
		}

		// Check if this group was in cooldown or had active state
		var cooldownUntil time.Time
		var wasActive bool
		if oldGroup, existed := oldGroups[groupName]; existed {
			cooldownUntil = oldGroup.CooldownUntil
			wasActive = oldGroup.IsActive // v5.0: 恢复之前的激活状态
		}

		group := &GroupInfo{
			Name:          groupName,
			Endpoints:     []*Endpoint{ep},
			IsActive:      wasActive, // v5.0: SQLite 模式下保留之前的激活状态
			CooldownUntil: cooldownUntil,
			Priority:      ep.Config.Priority,
		}

		// v4.0: 由 failover_enabled 控制组是否处于活跃状态
		// 如果 failover_enabled=false，则不参与故障转移（类似手动暂停）
		if !failoverEnabled {
			group.ManuallyPaused = true // 使用现有的手动暂停机制来实现不参与故障转移
		}

		newGroups[groupName] = group
	}

	gm.groups = newGroups

	// Update active status based on cooldown timers
	gm.updateActiveGroups()
}

// updateActiveGroups updates which groups are currently active
func (gm *GroupManager) updateActiveGroups() {
	// v5.0: SQLite 模式下，禁用自动激活逻辑（由 enabled 字段控制）
	// 但仍需处理冷却超时清理
	isSQLiteMode := gm.config.EndpointsStorage.Type == "sqlite"

	now := time.Now()
	var newlyActivatedGroup string

	// Track previous active state to detect changes
	previousActiveGroups := make(map[string]bool)
	for _, group := range gm.groups {
		previousActiveGroups[group.Name] = group.IsActive
	}

	// First, check cooldown timers and clear expired cooldowns
	for _, group := range gm.groups {
		if !group.CooldownUntil.IsZero() && now.After(group.CooldownUntil) {
			// Cooldown expired, clear it but don't auto-activate in manual mode
			group.CooldownUntil = time.Time{}
			// 🔧 [热更新修复] 统一使用 Failover.Enabled，不再使用废弃的 Group.AutoSwitchBetweenGroups
			autoSwitchEnabled := gm.config.Failover.Enabled
			slog.Info(fmt.Sprintf("🔄 [组管理] 组冷却结束: %s (优先级: %d) - %s",
				group.Name, group.Priority,
				map[bool]string{true: "自动激活", false: "等待手动激活"}[autoSwitchEnabled]))
		} else if !group.CooldownUntil.IsZero() && now.Before(group.CooldownUntil) {
			// Still in cooldown
			group.IsActive = false
		}
	}

	// v5.0: SQLite 模式下跳过自动激活逻辑（手动控制）
	if isSQLiteMode {
		// SQLite 模式：保持手动设置的激活状态，不自动切换
		return
	}

	// Determine which groups should be active based on priority
	// Only auto-activate next group if auto switching is enabled
	// 🔧 [热更新修复] 统一使用 Failover.Enabled
	autoSwitchEnabled := gm.config.Failover.Enabled
	if autoSwitchEnabled {
		// Auto mode: automatically activate highest priority available group
		// Get all groups sorted by priority
		sortedGroups := gm.getSortedGroups()

		// Find the highest priority group that's not in cooldown and not manually paused
		activeGroupFound := false
		for _, group := range sortedGroups {
			isAvailable := group.CooldownUntil.IsZero() && !group.ManuallyPaused
			if isAvailable {
				if !activeGroupFound {
					wasActive := group.IsActive
					group.IsActive = true
					activeGroupFound = true
					// Check if this group became newly active
					if !wasActive && group.IsActive {
						newlyActivatedGroup = group.Name
					}
				} else {
					group.IsActive = false // Only one group can be active at a time
				}
			} else {
				group.IsActive = false
			}
		}
	} else {
		// Manual mode: Only activate priority 1 group at startup if no groups are active
		// Don't auto-switch between groups during runtime
		currentActiveCount := 0
		for _, group := range gm.groups {
			if group.IsActive {
				currentActiveCount++
			}
		}

		// Handle cooldown states first
		for _, group := range gm.groups {
			if !group.CooldownUntil.IsZero() && now.Before(group.CooldownUntil) {
				// Still in cooldown, keep inactive
				group.IsActive = false
			}
		}

		// If no groups are active, determine if this is startup or runtime failure
		if currentActiveCount == 0 {
			// Check if this is truly startup (no groups have ever failed) or runtime failure
			isActualStartup := true
			for _, group := range gm.groups {
				if !group.CooldownUntil.IsZero() || group.ManuallyPaused {
					isActualStartup = false
					break
				}
			}

			// Determine activation strategy based on startup vs runtime context
			var shouldAutoActivate bool
			if isActualStartup {
				// Always auto-activate priority 1 group at startup for better UX
				shouldAutoActivate = true
				slog.Debug("🚀 [组管理] 检测到系统启动 - 尝试激活优先级1组")
			} else {
				// This is runtime failure - respect manual mode + suspend settings
				// 🔧 [热更新修复] 统一使用 Failover.Enabled
				autoSwitchEnabled := gm.config.Failover.Enabled
				if !autoSwitchEnabled && gm.config.RequestSuspend.Enabled {
					shouldAutoActivate = false
					slog.Debug("⏸️ [组管理] 运行时故障且启用挂起 - 不激活其他组，等待挂起处理")
				} else {
					// Manual mode without suspend, or auto mode - allow activation
					shouldAutoActivate = true
					slog.Debug("🔄 [组管理] 运行时故障但未启用挂起 - 尝试激活可用组")
				}
			}

			if shouldAutoActivate {
				sortedGroups := gm.getSortedGroups()
				for _, group := range sortedGroups {
					// 关键修复：检查组是否被手动暂停（包括因失败而暂停的组）
					if group.Priority == 1 && group.CooldownUntil.IsZero() && !group.ManuallyPaused {
						if len(group.Endpoints) > 0 {
							wasActive := group.IsActive
							group.IsActive = true
							autoSwitchEnabled := gm.config.Failover.Enabled
							if isActualStartup {
								if autoSwitchEnabled {
									slog.Info(fmt.Sprintf("🚀 [自动模式] 启动时激活优先级1组: %s", group.Name))
								} else {
									slog.Info(fmt.Sprintf("🚀 [手动模式] 启动时激活优先级1组: %s - 后续故障将启用挂起", group.Name))
								}
							} else {
								slog.Info(fmt.Sprintf("🔄 [运行时] 激活可用组: %s (优先级: %d)", group.Name, group.Priority))
							}
							if !wasActive && group.IsActive {
								newlyActivatedGroup = group.Name
							}
							break
						}
					} else if group.ManuallyPaused {
						// 记录被暂停的组，说明为什么没有激活
						slog.Debug(fmt.Sprintf("⏸️ [手动模式] 跳过已暂停组: %s (优先级: %d) - 等待手动恢复", group.Name, group.Priority))
					}
				}
			}
		}
	}

	// Notify subscribers if a group was newly activated
	if newlyActivatedGroup != "" {
		// Check if this is truly a state change (not just the same group remaining active)
		if !previousActiveGroups[newlyActivatedGroup] {
			slog.Debug(fmt.Sprintf("📡 [组通知] 检测到组状态变化: %s 变为活跃", newlyActivatedGroup))
			gm.notifyGroupChange(newlyActivatedGroup)
		}
	}
}

// getSortedGroups returns groups sorted by priority (lower number = higher priority)
func (gm *GroupManager) getSortedGroups() []*GroupInfo {
	groups := make([]*GroupInfo, 0, len(gm.groups))
	for _, group := range gm.groups {
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Priority < groups[j].Priority
	})

	return groups
}

// GetActiveGroups returns currently active groups
func (gm *GroupManager) GetActiveGroups() []*GroupInfo {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	gm.updateActiveGroups()

	var active []*GroupInfo
	for _, group := range gm.groups {
		if group.IsActive {
			active = append(active, group)
		}
	}

	// Sort by priority
	sort.Slice(active, func(i, j int) bool {
		return active[i].Priority < active[j].Priority
	})

	return active
}

// GetAllGroups returns all groups
func (gm *GroupManager) GetAllGroups() []*GroupInfo {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	gm.updateActiveGroups()

	groups := make([]*GroupInfo, 0, len(gm.groups))
	for _, group := range gm.groups {
		groups = append(groups, group)
	}

	// Sort by priority
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Priority < groups[j].Priority
	})

	return groups
}

// SetGroupCooldown sets a group into cooldown mode (only in auto mode)
func (gm *GroupManager) SetGroupCooldown(groupName string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	group, exists := gm.groups[groupName]
	if !exists {
		return
	}

	// In manual mode, mark group as manually paused to prevent re-activation
	autoSwitchEnabled := gm.config.Failover.Enabled
	if !autoSwitchEnabled {
		group.IsActive = false
		group.ManuallyPaused = true
		slog.Warn(fmt.Sprintf("⚠️ [手动模式] 组 %s 失败已停用并标记为暂停状态，需要手动切换到其他组", groupName))
		return
	}

	// Auto mode: use cooldown mechanism
	now := time.Now()
	group.CooldownUntil = now.Add(gm.cooldownDuration)
	group.IsActive = false

	slog.Warn(fmt.Sprintf("❄️ [自动模式] 组进入冷却状态: %s (冷却时长: %v, 恢复时间: %s)",
		groupName, gm.cooldownDuration, group.CooldownUntil.Format("15:04:05")))

	gm.updateActiveGroups()

	for _, g := range gm.getSortedGroups() {
		if g.IsActive {
			slog.Info(fmt.Sprintf("🔄 [自动模式] 切换到下一优先级组: %s (优先级: %d)",
				g.Name, g.Priority))
			gm.notifyGroupChange(g.Name)
			break
		}
	}
}

// IsGroupInCooldown checks if a group is currently in cooldown
func (gm *GroupManager) IsGroupInCooldown(groupName string) bool {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	if group, exists := gm.groups[groupName]; exists {
		return !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil)
	}

	return false
}

// GetGroupCooldownRemaining returns remaining cooldown time for a group
func (gm *GroupManager) GetGroupCooldownRemaining(groupName string) time.Duration {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	if group, exists := gm.groups[groupName]; exists {
		if !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil) {
			return group.CooldownUntil.Sub(time.Now())
		}
	}

	return 0
}

// ManualActivateGroup manually activates a specific group and deactivates others (compatibility function)
func (gm *GroupManager) ManualActivateGroup(groupName string) error {
	return gm.ManualActivateGroupWithForce(groupName, false)
}

// ManualActivateGroupWithForce manually activates a specific group and deactivates others

func (gm *GroupManager) ManualActivateGroupWithForce(groupName string, force bool) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}

	// 检查冷却状态（强制激活仍需检查冷却）
	if !targetGroup.CooldownUntil.IsZero() && time.Now().Before(targetGroup.CooldownUntil) {
		remaining := targetGroup.CooldownUntil.Sub(time.Now())
		return fmt.Errorf("组 %s 仍在冷却中，剩余时间: %v", groupName, remaining.Round(time.Second))
	}

	// v5.0: SQLite 模式下跳过健康检查（因为启动时健康检查还没开始）
	isSQLiteMode := gm.config.EndpointsStorage.Type == "sqlite"

	totalCount := len(targetGroup.Endpoints)

	if isSQLiteMode {
		slog.Info(fmt.Sprintf("🔄 [SQLite模式] 激活端点: %s (端点数: %d)", groupName, totalCount))
	} else {
		if force {
			slog.Warn(fmt.Sprintf("⚠️ [强制激活] 用户强制激活组: %s (端点数: %d, 操作时间: %s, 风险等级: HIGH)",
				groupName, totalCount, time.Now().Format("2006-01-02 15:04:05")))
			slog.Error(fmt.Sprintf("🚨 [安全警告] 强制激活可能导致请求失败! 组: %s, 建议尽快检查端点健康状态", groupName))

			targetGroup.ForcedActivation = true
			targetGroup.ForcedActivationTime = time.Now()
		} else {
			targetGroup.ForcedActivation = false
			targetGroup.ForcedActivationTime = time.Time{}
			slog.Info(fmt.Sprintf("🔄 [正常激活] 手动激活组: %s (端点数: %d)", groupName, totalCount))
		}
	}

	// 停用所有组
	for _, group := range gm.groups {
		group.IsActive = false
		group.ManuallyPaused = false
	}

	// 激活目标组
	targetGroup.IsActive = true
	targetGroup.ManualActivationTime = time.Now()
	targetGroup.CooldownUntil = time.Time{}

	// 通知订阅者
	gm.notifyGroupChange(groupName)

	return nil
}

// DeactivateGroup 停用指定组（用于故障转移时停用失败的端点）
// 注意：这只是简单地设置 IsActive=false，不设置 ManuallyPaused 标志
func (gm *GroupManager) DeactivateGroup(groupName string) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}

	if targetGroup.IsActive {
		targetGroup.IsActive = false
		slog.Info(fmt.Sprintf("🔴 [组管理] 组已停用: %s", groupName))
	}

	return nil
}

// ManualPauseGroup manually pauses a group (prevents it from being auto-activated)
func (gm *GroupManager) ManualPauseGroup(groupName string, duration time.Duration) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}

	targetGroup.ManuallyPaused = true

	var switchedToGroup string
	prevActiveGroups := make(map[string]bool)
	for _, g := range gm.groups {
		prevActiveGroups[g.Name] = g.IsActive
	}

	if targetGroup.IsActive {
		targetGroup.IsActive = false
		gm.updateActiveGroups()
		for _, g := range gm.groups {
			if g.IsActive && !prevActiveGroups[g.Name] {
				switchedToGroup = g.Name
				break
			}
		}
	}

	if duration > 0 {
		go func() {
			time.Sleep(duration)
			gm.mutex.Lock()
			defer gm.mutex.Unlock()
			if targetGroup.ManuallyPaused {
				targetGroup.ManuallyPaused = false
				prevActive := make(map[string]bool)
				for _, g := range gm.groups {
					prevActive[g.Name] = g.IsActive
				}
				gm.updateActiveGroups()
				for _, g := range gm.groups {
					if g.IsActive && !prevActive[g.Name] {
						gm.notifyGroupChange(g.Name)
						break
					}
				}
				slog.Info(fmt.Sprintf("⏰ [自动恢复] 组 %s 暂停期已结束，重新可用", groupName))
			}
		}()
	}

	if switchedToGroup != "" {
		gm.notifyGroupChange(switchedToGroup)
	}

	slog.Info(fmt.Sprintf("⏸️ [手动暂停] 组 %s 已暂停，需要手动恢复", groupName))

	return nil
}

// ManualResumeGroup manually resumes a paused group
func (gm *GroupManager) ManualResumeGroup(groupName string) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	targetGroup, exists := gm.groups[groupName]
	if !exists {
		return fmt.Errorf("组不存在: %s", groupName)
	}

	if !targetGroup.ManuallyPaused {
		return fmt.Errorf("组 %s 未处于暂停状态", groupName)
	}

	targetGroup.ManuallyPaused = false

	// Store previous active groups to detect changes
	prevActiveGroups := make(map[string]bool)
	for _, g := range gm.groups {
		prevActiveGroups[g.Name] = g.IsActive
	}

	gm.updateActiveGroups() // Re-evaluate active groups

	// Check if any group became newly active
	for _, g := range gm.groups {
		if g.IsActive && !prevActiveGroups[g.Name] {
			gm.notifyGroupChange(g.Name)
			slog.Debug(fmt.Sprintf("📡 [组通知] 因恢复组 %s 而激活组 %s", groupName, g.Name))
			break
		}
	}

	slog.Info(fmt.Sprintf("▶️ [手动恢复] 组 %s 已恢复，重新参与自动选择", groupName))
	return nil
}

// GetGroupDetails returns detailed information about all groups
func (gm *GroupManager) GetGroupDetails() map[string]interface{} {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	gm.updateActiveGroups()

	result := make(map[string]interface{})
	groupsData := make([]map[string]interface{}, 0, len(gm.groups))

	for _, group := range gm.groups {
		healthyCount := 0
		unhealthyCount := 0
		totalEndpoints := len(group.Endpoints)

		for _, ep := range group.Endpoints {
			if ep.IsHealthy() {
				healthyCount++
			} else {
				unhealthyCount++
			}
		}

		var status string
		var statusColor string
		var cooldownRemaining time.Duration

		if group.IsActive {
			status = "活跃"
			statusColor = "success"
		} else if group.ManuallyPaused {
			status = "手动暂停"
			statusColor = "warning"
		} else if !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil) {
			status = "冷却中"
			statusColor = "danger"
			cooldownRemaining = group.CooldownUntil.Sub(time.Now())
		} else {
			status = "可用"
			statusColor = "secondary"
		}

		groupData := map[string]interface{}{
			"name":                   group.Name,
			"priority":               group.Priority,
			"is_active":              group.IsActive,
			"status":                 status,
			"status_color":           statusColor,
			"total_endpoints":        totalEndpoints,
			"healthy_endpoints":      healthyCount,
			"unhealthy_endpoints":    unhealthyCount,
			"manually_paused":        group.ManuallyPaused,
			"in_cooldown":            !group.CooldownUntil.IsZero() && time.Now().Before(group.CooldownUntil),
			"cooldown_remaining":     cooldownRemaining.Round(time.Second).String(),
			"can_activate":           totalEndpoints > 0 && !group.IsActive && (group.CooldownUntil.IsZero() || time.Now().After(group.CooldownUntil)),
			"can_pause":              !group.ManuallyPaused,
			"can_resume":             group.ManuallyPaused,
			"forced_activation":      group.ForcedActivation,
			"forced_activation_time": "",
			"activation_type":        "normal",
			"can_force_activate":     !group.IsActive && (group.CooldownUntil.IsZero() || time.Now().After(group.CooldownUntil)),
		}

		// 添加强制激活时间
		if !group.ForcedActivationTime.IsZero() {
			groupData["forced_activation_time"] = group.ForcedActivationTime.Format("2006-01-02 15:04:05")
		}

		// 设置激活类型
		if group.ForcedActivation {
			groupData["activation_type"] = "forced"
			groupData["_computed_health_status"] = "强制激活"
		}

		if !group.ManualActivationTime.IsZero() {
			groupData["last_manual_activation"] = group.ManualActivationTime.Format("2006-01-02 15:04:05")
		}

		groupsData = append(groupsData, groupData)
	}

	// Sort by priority
	sort.Slice(groupsData, func(i, j int) bool {
		return groupsData[i]["priority"].(int) < groupsData[j]["priority"].(int)
	})

	result["groups"] = groupsData
	result["total_groups"] = len(groupsData)
	result["active_groups"] = len(gm.GetActiveGroups())

	return result
}

// FilterEndpointsByActiveGroups filters endpoints to only include those in active groups
// v4.0: 一端点一组架构，组名 = 端点名
func (gm *GroupManager) FilterEndpointsByActiveGroups(endpoints []*Endpoint) []*Endpoint {
	activeGroups := gm.GetActiveGroups()
	if len(activeGroups) == 0 {
		return nil
	}

	// Create a map of active group names for quick lookup
	activeGroupNames := make(map[string]bool)
	for _, group := range activeGroups {
		activeGroupNames[group.Name] = true
	}

	// Filter endpoints
	// v4.0: 组名 = 端点名
	var filtered []*Endpoint
	for _, ep := range endpoints {
		// v4.0 架构：组名就是端点名
		groupName := ep.Config.Name

		if activeGroupNames[groupName] {
			filtered = append(filtered, ep)
		}
	}

	return filtered
}

// SubscribeToGroupChanges subscribes to group change notifications
// Returns a channel that will receive the name of the newly activated group
func (gm *GroupManager) SubscribeToGroupChanges() <-chan string {
	gm.subscriberMutex.Lock()
	defer gm.subscriberMutex.Unlock()

	// Create a buffered channel to avoid blocking the sender
	ch := make(chan string, 10)
	gm.groupChangeSubscribers = append(gm.groupChangeSubscribers, ch)

	slog.Debug(fmt.Sprintf("📡 [组通知] 新增订阅者，当前订阅者数: %d", len(gm.groupChangeSubscribers)))

	return ch
}

// UnsubscribeFromGroupChanges removes a subscriber from group change notifications
func (gm *GroupManager) UnsubscribeFromGroupChanges(ch <-chan string) {
	gm.subscriberMutex.Lock()
	defer gm.subscriberMutex.Unlock()

	// Find and remove the channel from subscribers
	for i, subscriber := range gm.groupChangeSubscribers {
		if subscriber == ch {
			// Remove the channel from the slice
			gm.groupChangeSubscribers = append(gm.groupChangeSubscribers[:i], gm.groupChangeSubscribers[i+1:]...)
			// Close the channel to signal unsubscription
			close(subscriber)
			slog.Debug(fmt.Sprintf("📡 [组通知] 移除订阅者，当前订阅者数: %d", len(gm.groupChangeSubscribers)))
			return
		}
	}
}

// notifyGroupChange sends a non-blocking notification to all subscribers
// This method should be called with appropriate locks already held
func (gm *GroupManager) notifyGroupChange(activatedGroupName string) {
	gm.subscriberMutex.RLock()
	defer gm.subscriberMutex.RUnlock()

	if len(gm.groupChangeSubscribers) == 0 {
		return
	}

	slog.Debug(fmt.Sprintf("📡 [组通知] 广播组切换事件: %s (订阅者数: %d)",
		activatedGroupName, len(gm.groupChangeSubscribers)))

	// Send notification to all subscribers in a non-blocking manner
	for i, subscriber := range gm.groupChangeSubscribers {
		select {
		case subscriber <- activatedGroupName:
			// Successfully sent
		default:
			// Channel is full or closed, log warning
			slog.Warn(fmt.Sprintf("📡 [组通知] 订阅者 #%d 通道已满或已关闭，跳过通知", i))
		}
	}
}
