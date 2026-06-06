package endpoint

import (
	"strings"
	"testing"
	"time"

	"cc-forwarder/config"
)

func TestRouteOverrideManualFixedBlocksSystemSwitch(t *testing.T) {
	override := NewRouteOverride()
	override.Set(RouteOverrideState{
		Mode:            RouteModeManualFixed,
		EndpointName:    "primary",
		SetBy:           RouteCallerUser,
		FallbackEnabled: false,
	})

	if allowed, reason := override.AllowSystemSwitch("primary", "backup", RouteCallerSystemFailoverRequest); allowed || reason == "" {
		t.Fatalf("expected system failover to be blocked, allowed=%v reason=%q", allowed, reason)
	}
	if allowed, reason := override.AllowSystemSwitch("primary", "backup", RouteCallerUser); !allowed || reason != "" {
		t.Fatalf("expected user switch to be allowed, allowed=%v reason=%q", allowed, reason)
	}
}

func TestManualFixedFailoverBlockReasonPreservesPercentLiterals(t *testing.T) {
	cfg := &config.Config{
		Strategy: config.StrategyConfig{Type: "priority"},
		Failover: config.FailoverConfig{
			Enabled: true,
		},
		Endpoints: []config.EndpointConfig{
			{Name: "primary%s", URL: "http://primary.example", Priority: 1},
			{Name: "backup%q", URL: "http://backup.example", Priority: 2},
		},
	}
	manager := NewManager(cfg)
	manager.SetClaudeRoutingOverride(RouteOverrideState{
		Mode:            RouteModeManualFixed,
		EndpointName:    "primary%s",
		SetBy:           RouteCallerUser,
		FallbackEnabled: false,
	})

	_, err := manager.TriggerRequestFailoverWithCaller("primary%s", "unit test", RouteCallerSystemFailoverRequest)
	if err == nil {
		t.Fatal("expected manual fixed failover to be blocked")
	}
	if strings.Contains(err.Error(), "%!") {
		t.Fatalf("block reason was treated as a format string: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "primary%s") || !strings.Contains(err.Error(), "backup%q") {
		t.Fatalf("block reason lost endpoint names: %q", err.Error())
	}
}

func TestManualPreferredAllowsFallbackWithoutClearingPreference(t *testing.T) {
	cfg := &config.Config{
		Strategy: config.StrategyConfig{Type: "priority"},
		Failover: config.FailoverConfig{
			Enabled: true,
		},
		Endpoints: []config.EndpointConfig{
			{Name: "primary", URL: "http://primary.example", Priority: 1},
			{Name: "backup", URL: "http://backup.example", Priority: 2},
		},
	}
	manager := NewManager(cfg)
	manager.SetClaudeRoutingOverride(RouteOverrideState{
		Mode:            RouteModeManualPreferred,
		EndpointName:    "primary",
		SetBy:           RouteCallerUser,
		FallbackEnabled: true,
	})

	newEndpoint, err := manager.TriggerRequestFailoverWithCaller("primary", "unit test", RouteCallerSystemFailoverRequest)
	if err != nil {
		t.Fatalf("manual preferred should allow fallback: %v", err)
	}
	if newEndpoint != "backup" {
		t.Fatalf("expected fallback to backup, got %q", newEndpoint)
	}

	override := manager.GetClaudeRoutingOverride()
	if override.Mode != RouteModeManualPreferred || override.EndpointName != "primary" {
		t.Fatalf("manual preference should remain recorded, got %+v", override)
	}
}

func TestManualFixedSelectionDoesNotFailoverPastPinnedEndpoint(t *testing.T) {
	cfg := &config.Config{
		Strategy: config.StrategyConfig{Type: "priority"},
		Failover: config.FailoverConfig{
			Enabled: true,
		},
		FailureTracker: config.FailureTrackerConfig{
			Enabled:    true,
			TimeWindow: time.Hour,
			Threshold:  1,
			Action:     "failover",
		},
		Endpoints: []config.EndpointConfig{
			{Name: "primary", URL: "http://primary.example", Priority: 1},
			{Name: "backup", URL: "http://backup.example", Priority: 2},
		},
	}
	manager := NewManager(cfg)
	manager.SetClaudeRoutingOverride(RouteOverrideState{
		Mode:            RouteModeManualFixed,
		EndpointName:    "primary",
		SetBy:           RouteCallerUser,
		FallbackEnabled: false,
	})
	manager.RecordFailure("primary")

	healthy := manager.GetHealthyEndpoints()
	if len(healthy) != 0 {
		t.Fatalf("expected manual fixed route to return no fallback endpoint, got %d", len(healthy))
	}

	override := manager.GetClaudeRoutingOverride()
	if override.Mode != RouteModeManualFixed || override.EndpointName != "primary" {
		t.Fatalf("manual fixed override changed unexpectedly: %+v", override)
	}
}
