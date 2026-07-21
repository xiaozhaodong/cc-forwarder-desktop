package endpoint

import (
	"context"
	"testing"
	"time"

	"cc-forwarder/config"
)

func newSchedulerTestManager(t *testing.T, cfg *config.Config) *Manager {
	t.Helper()
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)
	return manager
}

func schedulerTestConfig(failoverEnabled bool, names ...string) *config.Config {
	cfg := &config.Config{
		Failover: config.FailoverConfig{Enabled: failoverEnabled},
		Strategy: config.StrategyConfig{Type: "priority"},
		FailureTracker: config.FailureTrackerConfig{
			Enabled:    true,
			TimeWindow: 5 * time.Minute,
			Threshold:  3,
			Action:     "failover",
		},
	}
	for i, name := range names {
		cfg.Endpoints = append(cfg.Endpoints, config.EndpointConfig{
			Name: name, URL: "http://" + name + ".test", Priority: i + 1,
		})
	}
	return cfg
}

func candidateNames(result EndpointScheduleResult) []string {
	names := make([]string, 0, len(result.Candidates))
	for _, ep := range result.Candidates {
		names = append(names, ep.Config.Name)
	}
	return names
}

func skippedReason(t *testing.T, result EndpointScheduleResult, name string) (string, time.Time) {
	t.Helper()
	for _, decision := range result.Snapshot.Decisions {
		if decision.Name == name && decision.Decision == endpointDecisionSkipped {
			return decision.Reason, decision.AvailableAt
		}
	}
	t.Fatalf("expected skipped decision for %s, got %+v", name, result.Snapshot.Decisions)
	return "", time.Time{}
}

func TestPrepareRouteCandidates_ActiveFirstThenPriorityOrder(t *testing.T) {
	manager := newSchedulerTestManager(t, schedulerTestConfig(true, "a", "b", "c"))
	manager.RestoreActiveEndpoint("c")

	result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if got := candidateNames(result); len(got) != 3 || got[0] != "c" || got[1] != "a" || got[2] != "b" {
		t.Fatalf("expected [c a b], got %v", got)
	}
	if result.ActiveEndpointAtSelection != "c" {
		t.Fatalf("expected active snapshot c, got %q", result.ActiveEndpointAtSelection)
	}
	if result.ActiveRevision == 0 {
		t.Fatal("expected non-zero active revision in snapshot")
	}
}

func TestPrepareRouteCandidates_FiltersWithAvailableAtSources(t *testing.T) {
	manager := newSchedulerTestManager(t, schedulerTestConfig(true, "active-ep", "cooling", "paused", "tripped", "negcached", "ok"))
	manager.RestoreActiveEndpoint("active-ep")

	now := time.Now()
	cooldownUntil := now.Add(10 * time.Minute)
	pausedUntil := now.Add(30 * time.Minute)

	cooling := manager.GetEndpointByNameAny("cooling")
	cooling.mutex.Lock()
	cooling.Status.CooldownUntil = cooldownUntil
	cooling.mutex.Unlock()

	paused := manager.GetEndpointByNameAny("paused")
	paused.mutex.Lock()
	paused.Status.PausedUntil = pausedUntil
	paused.mutex.Unlock()

	for i := 0; i < 3; i++ {
		manager.RecordFailure("tripped")
	}

	profile := RouteRequestProfile{Model: "gpt-test"}
	manager.RecordNegativeRouteHit("negcached", "model_unsupported", profile, "model not found")

	result := manager.PrepareRouteCandidates(context.Background(), profile)
	if got := candidateNames(result); len(got) != 2 || got[0] != "active-ep" || got[1] != "ok" {
		t.Fatalf("expected [active-ep ok], got %v", got)
	}

	if reason, availableAt := skippedReason(t, result, "cooling"); reason != "cooldown" || !availableAt.Equal(cooldownUntil) {
		t.Fatalf("cooling: got reason=%q availableAt=%v", reason, availableAt)
	}
	if reason, availableAt := skippedReason(t, result, "paused"); reason != "paused" || !availableAt.Equal(pausedUntil) {
		t.Fatalf("paused: got reason=%q availableAt=%v", reason, availableAt)
	}
	if reason, availableAt := skippedReason(t, result, "tripped"); reason != "failure_threshold_tripped" || availableAt.IsZero() {
		t.Fatalf("tripped: got reason=%q availableAt=%v", reason, availableAt)
	} else if availableAt.Before(now) || availableAt.After(now.Add(6*time.Minute)) {
		t.Fatalf("tripped availableAt should be ~earliest failure + window, got %v", availableAt)
	}
	if reason, availableAt := skippedReason(t, result, "negcached"); reason != "negative_cache_model_unsupported" || availableAt.IsZero() {
		t.Fatalf("negcached: got reason=%q availableAt=%v", reason, availableAt)
	}

	// 四来源中最早的是 tripped（最早失败 + 5m 窗口），早于 10m 冷却与 30m 暂停
	if earliest := result.Snapshot.EarliestAvailableAt(); earliest.IsZero() ||
		earliest.Before(now) || earliest.After(now.Add(6*time.Minute)) {
		t.Fatalf("expected earliest availableAt from tripped window (~5m), got %v", earliest)
	}
}

func TestPrepareRouteCandidates_FailoverDisabledSingleCandidate(t *testing.T) {
	manager := newSchedulerTestManager(t, schedulerTestConfig(false, "a", "b", "c"))
	manager.RestoreActiveEndpoint("b")

	result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if got := candidateNames(result); len(got) != 1 || got[0] != "b" {
		t.Fatalf("expected single candidate [b], got %v", got)
	}

	// active 不可路由时不得静默尝试备用端点
	active := manager.GetEndpointByNameAny("b")
	active.mutex.Lock()
	active.Status.CooldownUntil = time.Now().Add(time.Minute)
	active.mutex.Unlock()

	result = manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if len(result.Candidates) != 0 {
		t.Fatalf("expected empty candidates when active cooling and failover disabled, got %v", candidateNames(result))
	}
}

func TestPrepareRouteCandidates_ManualFixedOnlyTarget(t *testing.T) {
	manager := newSchedulerTestManager(t, schedulerTestConfig(true, "a", "b", "c"))
	manager.RestoreActiveEndpoint("a")
	manager.SetClaudeRoutingOverride(RouteOverrideState{Mode: RouteModeManualFixed, EndpointName: "c"})

	result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if got := candidateNames(result); len(got) != 1 || got[0] != "c" {
		t.Fatalf("expected manual_fixed single candidate [c], got %v", got)
	}
	if result.RouteOverrideRevision == 0 {
		t.Fatal("expected route override revision captured")
	}

	// 目标冷却：空候选而非静默转移
	target := manager.GetEndpointByNameAny("c")
	target.mutex.Lock()
	target.Status.CooldownUntil = time.Now().Add(time.Minute)
	target.mutex.Unlock()

	result = manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if len(result.Candidates) != 0 {
		t.Fatalf("expected empty candidates for cooling manual_fixed target, got %v", candidateNames(result))
	}
	if reason, _ := skippedReason(t, result, "c"); reason != "manual_fixed_cooldown" {
		t.Fatalf("expected manual_fixed_cooldown reason, got %q", reason)
	}
}

func TestPrepareRouteCandidates_ManualPreferredMovesTargetFirst(t *testing.T) {
	manager := newSchedulerTestManager(t, schedulerTestConfig(true, "a", "b", "c"))
	manager.RestoreActiveEndpoint("a")
	manager.SetClaudeRoutingOverride(RouteOverrideState{Mode: RouteModeManualPreferred, EndpointName: "c"})

	result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if got := candidateNames(result); len(got) != 3 || got[0] != "c" {
		t.Fatalf("expected preferred target first, got %v", got)
	}
}

func TestPrepareRouteCandidates_FailoverDisabledEndpointExcluded(t *testing.T) {
	cfg := schedulerTestConfig(true, "a", "b", "c")
	disabled := false
	cfg.Endpoints[2].FailoverEnabled = &disabled

	manager := newSchedulerTestManager(t, cfg)
	manager.RestoreActiveEndpoint("a")

	result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if got := candidateNames(result); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b] with c excluded, got %v", got)
	}
	if reason, _ := skippedReason(t, result, "c"); reason != "failover_disabled_endpoint" {
		t.Fatalf("expected failover_disabled_endpoint reason, got %q", reason)
	}
}
