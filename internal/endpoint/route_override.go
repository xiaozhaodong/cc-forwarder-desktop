package endpoint

import (
	"fmt"
	"sync"
	"time"
)

const (
	RouteModeAuto            = "auto"
	RouteModeManualPreferred = "manual_preferred"
	RouteModeManualFixed     = "manual_fixed"
)

const (
	RouteCallerUser                    = "user"
	RouteCallerSystemFailoverRequest   = "system_failover_request"
	RouteCallerSystemFailoverSelection = "system_failover_selection"
	RouteCallerStartupRecovery         = "startup_recovery"
)

type RouteOverrideState struct {
	Mode                  string    `json:"mode"`
	EndpointName          string    `json:"endpoint_name"`
	SetBy                 string    `json:"set_by"`
	SetAt                 time.Time `json:"set_at"`
	ExpiresAt             time.Time `json:"expires_at,omitempty"`
	FallbackEnabled       bool      `json:"fallback_enabled"`
	Revision              int64     `json:"revision"`
	FallbackReason        string    `json:"fallback_reason,omitempty"`
	LastEffectiveEndpoint string    `json:"last_effective_endpoint,omitempty"`
	LastDecisionAt        time.Time `json:"last_decision_at,omitempty"`
}

type RouteOverride struct {
	mu    sync.RWMutex
	state RouteOverrideState
}

func NewRouteOverride() *RouteOverride {
	return &RouteOverride{
		state: RouteOverrideState{
			Mode:            RouteModeAuto,
			FallbackEnabled: true,
		},
	}
}

func NormalizeRouteMode(mode string) string {
	switch mode {
	case RouteModeManualPreferred, RouteModeManualFixed:
		return mode
	default:
		return RouteModeAuto
	}
}

func (ro *RouteOverride) Snapshot() RouteOverrideState {
	if ro == nil {
		return RouteOverrideState{Mode: RouteModeAuto, FallbackEnabled: true}
	}

	ro.mu.RLock()
	defer ro.mu.RUnlock()
	state := ro.state
	if state.Mode == "" {
		state.Mode = RouteModeAuto
	}
	return state
}

func (ro *RouteOverride) Set(state RouteOverrideState) RouteOverrideState {
	if ro == nil {
		return RouteOverrideState{Mode: RouteModeAuto, FallbackEnabled: true}
	}

	ro.mu.Lock()
	defer ro.mu.Unlock()

	state.Mode = NormalizeRouteMode(state.Mode)
	if state.Mode == RouteModeAuto {
		state.EndpointName = ""
		state.FallbackReason = ""
		state.LastEffectiveEndpoint = ""
		state.LastDecisionAt = time.Time{}
		state.FallbackEnabled = true
	} else if state.FallbackEnabled == false && state.Mode == RouteModeManualPreferred {
		state.FallbackEnabled = true
	}
	if state.SetAt.IsZero() {
		state.SetAt = time.Now()
	}
	if state.SetBy == "" {
		state.SetBy = "user"
	}
	state.Revision = ro.state.Revision + 1
	ro.state = state
	return ro.state
}

// ApplyPersisted 原子发布已持久化的完整路由状态（§6.4：persist-then-publish，
// revision 以数据库为准，不再自增）。较旧 revision 不得覆盖较新状态。
func (ro *RouteOverride) ApplyPersisted(state RouteOverrideState) RouteOverrideState {
	if ro == nil {
		return RouteOverrideState{Mode: RouteModeAuto, FallbackEnabled: true}
	}

	ro.mu.Lock()
	defer ro.mu.Unlock()

	if state.Revision != 0 && state.Revision < ro.state.Revision {
		return ro.state // 并发保护：旧提交不覆盖新状态
	}
	state.Mode = NormalizeRouteMode(state.Mode)
	if state.Mode == RouteModeAuto {
		state.EndpointName = ""
		state.FallbackEnabled = true
	} else if !state.FallbackEnabled && state.Mode == RouteModeManualPreferred {
		state.FallbackEnabled = true
	}
	ro.state = state
	return ro.state
}

func (ro *RouteOverride) Clear(setBy string) RouteOverrideState {
	return ro.Set(RouteOverrideState{
		Mode:            RouteModeAuto,
		SetBy:           setBy,
		FallbackEnabled: true,
		SetAt:           time.Now(),
	})
}

func (ro *RouteOverride) NoteDecision(effectiveEndpoint, fallbackReason string) RouteOverrideState {
	if ro == nil {
		return RouteOverrideState{Mode: RouteModeAuto, FallbackEnabled: true}
	}

	ro.mu.Lock()
	defer ro.mu.Unlock()
	ro.state.LastEffectiveEndpoint = effectiveEndpoint
	ro.state.FallbackReason = fallbackReason
	ro.state.LastDecisionAt = time.Now()
	return ro.state
}

func (ro *RouteOverride) AllowSystemSwitch(fromEndpoint, toEndpoint, callerKind string) (bool, string) {
	state := ro.Snapshot()
	if state.Mode != RouteModeManualFixed {
		return true, ""
	}
	if callerKind == RouteCallerUser || callerKind == RouteCallerStartupRecovery {
		return true, ""
	}
	return false, fmt.Sprintf("manual_fixed endpoint %q forbids system switch from %q to %q", state.EndpointName, fromEndpoint, toEndpoint)
}

func (m *Manager) GetClaudeRoutingOverride() RouteOverrideState {
	return m.routeOverride.Snapshot()
}

func (m *Manager) SetClaudeRoutingOverride(state RouteOverrideState) RouteOverrideState {
	return m.routeOverride.Set(state)
}

// ApplyPersistedClaudeRoutingState 发布已持久化路由状态（§6.4）
func (m *Manager) ApplyPersistedClaudeRoutingState(state RouteOverrideState) RouteOverrideState {
	return m.routeOverride.ApplyPersisted(state)
}

func (m *Manager) ClearClaudeRoutingOverride(setBy string) RouteOverrideState {
	return m.routeOverride.Clear(setBy)
}

func (m *Manager) NoteRouteDecision(effectiveEndpoint, fallbackReason string) RouteOverrideState {
	prev := m.routeOverride.Snapshot().LastEffectiveEndpoint
	state := m.routeOverride.NoteDecision(effectiveEndpoint, fallbackReason)
	if state.LastEffectiveEndpoint != prev {
		m.routeDecisionMu.RLock()
		notify := m.routeDecisionNotify
		m.routeDecisionMu.RUnlock()
		if notify != nil {
			go notify(state)
		}
	}
	return state
}

// SetRouteDecisionNotifier 注入 last_effective_endpoint 变更回调（App 层桥接前端事件）
func (m *Manager) SetRouteDecisionNotifier(notify func(RouteOverrideState)) {
	m.routeDecisionMu.Lock()
	m.routeDecisionNotify = notify
	m.routeDecisionMu.Unlock()
}

func (m *Manager) AllowSystemRouteSwitch(fromEndpoint, toEndpoint, callerKind string) (bool, string) {
	return m.routeOverride.AllowSystemSwitch(fromEndpoint, toEndpoint, callerKind)
}
