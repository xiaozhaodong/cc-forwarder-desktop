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

func (m *Manager) ClearClaudeRoutingOverride(setBy string) RouteOverrideState {
	return m.routeOverride.Clear(setBy)
}

func (m *Manager) NoteRouteDecision(effectiveEndpoint, fallbackReason string) RouteOverrideState {
	return m.routeOverride.NoteDecision(effectiveEndpoint, fallbackReason)
}

func (m *Manager) AllowSystemRouteSwitch(fromEndpoint, toEndpoint, callerKind string) (bool, string) {
	return m.routeOverride.AllowSystemSwitch(fromEndpoint, toEndpoint, callerKind)
}
