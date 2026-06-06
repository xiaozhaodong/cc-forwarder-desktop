package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/service"
	"cc-forwarder/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const EventClaudeRoutingUpdate = "claude-routing:update"

type ClaudeRoutingState struct {
	Mode                  string   `json:"mode"`
	EndpointName          string   `json:"endpoint_name"`
	SetBy                 string   `json:"set_by"`
	SetAt                 string   `json:"set_at"`
	FallbackEnabled       bool     `json:"fallback_enabled"`
	Revision              int64    `json:"revision"`
	FallbackReason        string   `json:"fallback_reason"`
	LastEffectiveEndpoint string   `json:"last_effective_endpoint"`
	LastDecisionAt        string   `json:"last_decision_at"`
	CurrentActiveEndpoint string   `json:"current_active_endpoint"`
	AvailableEndpoints    []string `json:"available_endpoints"`
}

type SetClaudeRoutingOverrideInput struct {
	Mode            string `json:"mode"`
	EndpointName    string `json:"endpoint_name"`
	FallbackEnabled bool   `json:"fallback_enabled"`
}

func (a *App) GetClaudeRoutingState() (ClaudeRoutingState, error) {
	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()

	if manager == nil {
		return ClaudeRoutingState{Mode: endpoint.RouteModeAuto, FallbackEnabled: true}, nil
	}
	return a.buildClaudeRoutingState(manager.GetClaudeRoutingOverride()), nil
}

func (a *App) SetClaudeRoutingOverride(input SetClaudeRoutingOverrideInput) (ClaudeRoutingState, error) {
	mode := endpoint.NormalizeRouteMode(input.Mode)
	if mode == endpoint.RouteModeAuto {
		return a.ClearClaudeRoutingOverride()
	}
	if input.EndpointName == "" {
		return ClaudeRoutingState{}, fmt.Errorf("手动路由需要指定端点")
	}

	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()
	if manager == nil {
		return ClaudeRoutingState{}, fmt.Errorf("端点管理器未初始化")
	}
	if manager.GetEndpointByNameAny(input.EndpointName) == nil {
		return ClaudeRoutingState{}, fmt.Errorf("端点 '%s' 不存在", input.EndpointName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.activateClaudeEndpoint(ctx, input.EndpointName); err != nil {
		return ClaudeRoutingState{}, err
	}

	fallbackEnabled := input.FallbackEnabled
	if mode == endpoint.RouteModeManualPreferred {
		fallbackEnabled = true
	}
	state := manager.SetClaudeRoutingOverride(endpoint.RouteOverrideState{
		Mode:            mode,
		EndpointName:    input.EndpointName,
		SetBy:           endpoint.RouteCallerUser,
		SetAt:           time.Now(),
		FallbackEnabled: fallbackEnabled,
	})

	if err := a.persistClaudeRoutingState(ctx, state); err != nil {
		return ClaudeRoutingState{}, err
	}

	result := a.buildClaudeRoutingState(state)
	a.emitClaudeRoutingUpdate(result)
	a.emitEndpointUpdate()
	return result, nil
}

func (a *App) ClearClaudeRoutingOverride() (ClaudeRoutingState, error) {
	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()
	if manager == nil {
		return ClaudeRoutingState{Mode: endpoint.RouteModeAuto, FallbackEnabled: true}, nil
	}

	state := manager.ClearClaudeRoutingOverride(endpoint.RouteCallerUser)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.persistClaudeRoutingState(ctx, state); err != nil {
		return ClaudeRoutingState{}, err
	}

	result := a.buildClaudeRoutingState(state)
	a.emitClaudeRoutingUpdate(result)
	return result, nil
}

func (a *App) ClearNegativeHitCache(endpointName string) error {
	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()
	if manager == nil {
		return fmt.Errorf("端点管理器未初始化")
	}

	manager.ClearNegativeHitCache(endpointName)
	return nil
}

func (a *App) loadClaudeRoutingOverride(ctx context.Context) {
	manager := a.endpointManager
	endpointService := a.endpointService
	if a.settingsService == nil || manager == nil {
		return
	}

	mode, _ := a.settingsService.GetValue(ctx, service.CategoryClaudeRouting, "mode")
	mode = endpoint.NormalizeRouteMode(mode)
	if mode == endpoint.RouteModeAuto {
		manager.ClearClaudeRoutingOverride(endpoint.RouteCallerStartupRecovery)
		return
	}

	endpointName, _ := a.settingsService.GetValue(ctx, service.CategoryClaudeRouting, "endpoint_name")
	if endpointName == "" || manager.GetEndpointByNameAny(endpointName) == nil {
		slog.Warn("⚠️ [Claude路由] 持久化手动端点不存在，已恢复自动路由", "endpoint", endpointName)
		state := manager.ClearClaudeRoutingOverride(endpoint.RouteCallerStartupRecovery)
		if err := a.persistClaudeRoutingState(ctx, state); err != nil {
			slog.Warn("⚠️ [Claude路由] 恢复自动路由持久化失败", "error", err)
		}
		return
	}

	fallbackEnabled := a.settingsService.GetBool(ctx, service.CategoryClaudeRouting, "fallback_enabled", mode == endpoint.RouteModeManualPreferred)
	setBy, _ := a.settingsService.GetValue(ctx, service.CategoryClaudeRouting, "set_by")
	setAtValue, _ := a.settingsService.GetValue(ctx, service.CategoryClaudeRouting, "set_at")
	setAt := time.Now()
	if parsed, err := time.Parse(time.RFC3339, setAtValue); err == nil {
		setAt = parsed
	}

	if err := a.activateClaudeEndpointWith(ctx, manager, endpointService, endpointName); err != nil {
		slog.Warn("⚠️ [Claude路由] 恢复手动端点激活失败，已恢复自动路由", "endpoint", endpointName, "error", err)
		state := manager.ClearClaudeRoutingOverride(endpoint.RouteCallerStartupRecovery)
		if err := a.persistClaudeRoutingState(ctx, state); err != nil {
			slog.Warn("⚠️ [Claude路由] 恢复自动路由持久化失败", "error", err)
		}
		return
	}

	if setBy == "" {
		setBy = endpoint.RouteCallerStartupRecovery
	}
	state := manager.SetClaudeRoutingOverride(endpoint.RouteOverrideState{
		Mode:            mode,
		EndpointName:    endpointName,
		SetBy:           setBy,
		SetAt:           setAt,
		FallbackEnabled: fallbackEnabled,
	})
	slog.Info("✅ [Claude路由] 已恢复持久化路由状态", "mode", state.Mode, "endpoint", state.EndpointName)
}

func (a *App) activateClaudeEndpoint(ctx context.Context, endpointName string) error {
	a.mu.RLock()
	manager := a.endpointManager
	endpointService := a.endpointService
	a.mu.RUnlock()

	return a.activateClaudeEndpointWith(ctx, manager, endpointService, endpointName)
}

func (a *App) activateClaudeEndpointWith(ctx context.Context, manager *endpoint.Manager, endpointService *service.EndpointService, endpointName string) error {
	if manager == nil {
		return fmt.Errorf("端点管理器未初始化")
	}
	if manager.GetEndpointByNameAny(endpointName) == nil {
		return fmt.Errorf("端点 '%s' 不存在", endpointName)
	}

	if endpointService != nil {
		if err := endpointService.DisableAllEndpoints(ctx); err != nil {
			if a.logger != nil {
				a.logger.Warn("禁用所有端点失败", "error", err)
			}
		}
		if err := endpointService.ToggleEndpoint(ctx, endpointName, true); err != nil {
			if a.logger != nil {
				a.logger.Warn("启用端点失败", "endpoint", endpointName, "error", err)
			}
		} else if a.logger != nil {
			a.logger.Info("✅ 端点已同步到数据库", "endpoint", endpointName, "enabled", true)
		}
	}

	return manager.ManualActivateGroup(endpointName)
}

func (a *App) persistClaudeRoutingState(ctx context.Context, state endpoint.RouteOverrideState) error {
	if a.settingsService == nil || a.settingsStore == nil {
		return nil
	}

	values := map[string]string{
		"mode":             endpoint.NormalizeRouteMode(state.Mode),
		"endpoint_name":    state.EndpointName,
		"set_by":           state.SetBy,
		"set_at":           formatRouteTime(state.SetAt),
		"fallback_enabled": fmt.Sprintf("%t", state.FallbackEnabled),
	}

	records := make([]*store.SettingRecord, 0, len(values))
	for _, defaultRecord := range a.settingsService.GetAllDefaults() {
		if defaultRecord.Category != service.CategoryClaudeRouting {
			continue
		}
		record := *defaultRecord
		if value, ok := values[record.Key]; ok {
			record.Value = value
		}
		records = append(records, &record)
	}
	return a.settingsStore.BatchSet(ctx, records)
}

func (a *App) buildClaudeRoutingState(state endpoint.RouteOverrideState) ClaudeRoutingState {
	if state.Mode == "" {
		state.Mode = endpoint.RouteModeAuto
	}
	return ClaudeRoutingState{
		Mode:                  state.Mode,
		EndpointName:          state.EndpointName,
		SetBy:                 state.SetBy,
		SetAt:                 formatRouteTime(state.SetAt),
		FallbackEnabled:       state.FallbackEnabled,
		Revision:              state.Revision,
		FallbackReason:        state.FallbackReason,
		LastEffectiveEndpoint: state.LastEffectiveEndpoint,
		LastDecisionAt:        formatRouteTime(state.LastDecisionAt),
		CurrentActiveEndpoint: a.currentActiveClaudeEndpoint(),
		AvailableEndpoints:    a.availableClaudeEndpoints(),
	}
}

func (a *App) currentActiveClaudeEndpoint() string {
	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()
	if manager == nil || manager.GetGroupManager() == nil {
		return ""
	}
	for _, group := range manager.GetGroupManager().GetAllGroups() {
		if group.IsActive {
			return group.Name
		}
	}
	return ""
}

func (a *App) availableClaudeEndpoints() []string {
	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()
	if manager == nil {
		return []string{}
	}
	endpoints := manager.GetAllEndpoints()
	names := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep == nil {
			continue
		}
		names = append(names, ep.Config.Name)
	}
	return names
}

func (a *App) emitClaudeRoutingUpdate(state ClaudeRoutingState) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, EventClaudeRoutingUpdate, state)
}

func formatRouteTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
