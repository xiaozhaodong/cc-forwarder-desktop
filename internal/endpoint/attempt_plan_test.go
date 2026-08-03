package endpoint

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
)

func TestAcquireEndpointAttemptBuildsImmutableForwardTarget(t *testing.T) {
	availability := true
	autoSchedule := true
	cfg := &config.Config{Endpoints: []config.EndpointConfig{{
		Name:                "primary",
		URL:                 "https://old.example.com",
		Channel:             "old-channel",
		Group:               "old-group",
		Priority:            3,
		Timeout:             15 * time.Second,
		Headers:             map[string]string{"X-Snapshot": "old"},
		Tokens:              []config.TokenConfig{{Name: "first", Value: "token-1"}, {Name: "second", Value: "token-2"}},
		ApiKeys:             []config.ApiKeyConfig{{Name: "first", Value: "key-1"}, {Name: "second", Value: "key-2"}},
		SupportsCountTokens: true,
		ModelRewriteRules:   `[{"match":"exact","from":"old","to":"new"}]`,
		AvailabilityEnabled: &availability,
		FailoverEnabled:     &autoSchedule,
	}}}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)

	if err := manager.keyManager.SwitchToken("primary", 1); err != nil {
		t.Fatalf("switch token: %v", err)
	}
	if err := manager.keyManager.SwitchApiKey("primary", 1); err != nil {
		t.Fatalf("switch api key: %v", err)
	}

	plan := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{}).Plans[0]
	revision := plan.ConfigRevision
	admission, err := manager.AcquireEndpointAttempt(plan)
	if err != nil {
		t.Fatalf("acquire attempt: %v", err)
	}
	t.Cleanup(admission.Release)

	updated := cfg.Endpoints[0]
	updated.URL = "https://new.example.com"
	updated.Channel = "new-channel"
	updated.Group = "new-group"
	updated.Priority = 9
	updated.Timeout = time.Minute
	updated.Headers = map[string]string{"X-Snapshot": "new"}
	updated.Token = "new-token"
	updated.Tokens = nil
	updated.ApiKey = "new-key"
	updated.ApiKeys = nil
	updated.SupportsCountTokens = false
	updated.ModelRewriteRules = ""
	if err := manager.UpdateEndpointConfig("primary", updated); err != nil {
		t.Fatalf("update live endpoint: %v", err)
	}

	targetConfig := admission.Target.Config()
	if targetConfig.URL != "https://old.example.com" || targetConfig.Channel != "old-channel" || targetConfig.Group != "old-group" {
		t.Fatalf("target identity changed with live config: %+v", targetConfig)
	}
	if targetConfig.Priority != 3 || targetConfig.Timeout != 15*time.Second {
		t.Fatalf("target scheduling fields changed: priority=%d timeout=%s", targetConfig.Priority, targetConfig.Timeout)
	}
	if targetConfig.Headers["X-Snapshot"] != "old" {
		t.Fatalf("target headers changed: %+v", targetConfig.Headers)
	}
	if targetConfig.Token != "token-2" || targetConfig.ApiKey != "key-2" {
		t.Fatalf("expected selected credentials in snapshot, token=%q apiKey=%q", targetConfig.Token, targetConfig.ApiKey)
	}
	if len(targetConfig.Tokens) != 0 || len(targetConfig.ApiKeys) != 0 {
		t.Fatalf("credential lists must be collapsed in target: tokens=%d apiKeys=%d", len(targetConfig.Tokens), len(targetConfig.ApiKeys))
	}
	if !targetConfig.SupportsCountTokens || targetConfig.ModelRewriteRules == "" {
		t.Fatalf("target capability or rewrite rules changed: %+v", targetConfig)
	}
	if admission.Target.Revision() != revision {
		t.Fatalf("target revision mismatch: got %d want %d", admission.Target.Revision(), revision)
	}

	// Config 必须返回副本，调用方修改 headers 不能反向污染 target。
	targetConfig.Headers["X-Snapshot"] = "mutated-copy"
	if got := admission.Target.Config().Headers["X-Snapshot"]; got != "old" {
		t.Fatalf("target headers are externally mutable, got %q", got)
	}
}

func TestUpdateEndpointPriorityInvalidatesExistingPlan(t *testing.T) {
	cfg := &config.Config{Endpoints: []config.EndpointConfig{{
		Name: "priority-target", URL: "https://example.com", Priority: 1, Timeout: time.Second,
	}}}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)

	result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if len(result.Plans) != 1 {
		t.Fatalf("expected one plan, got %d", len(result.Plans))
	}
	oldPlan := result.Plans[0]
	if err := manager.UpdateEndpointPriority("priority-target", 5); err != nil {
		t.Fatalf("update priority: %v", err)
	}
	if _, err := manager.AcquireEndpointAttempt(oldPlan); err == nil || !strings.Contains(err.Error(), "config_changed_since_snapshot") {
		t.Fatalf("old plan must be rejected after priority update, got %v", err)
	}

	updated := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
	if len(updated.Plans) != 1 || updated.Plans[0].Priority != 5 || updated.Plans[0].ConfigRevision == oldPlan.ConfigRevision {
		t.Fatalf("expected new priority/revision plan, old=%+v new=%+v", oldPlan, updated.Plans)
	}
}

func TestAcquireEndpointAttemptDoesNotObservePartialCredentialPublication(t *testing.T) {
	cfg := &config.Config{Endpoints: []config.EndpointConfig{{
		Name: "credential-target", URL: "https://old.example.com", Priority: 1,
		Tokens: []config.TokenConfig{{Name: "first", Value: "old-a"}, {Name: "second", Value: "old-b"}},
	}}}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)
	if err := manager.keyManager.SwitchToken("credential-target", 1); err != nil {
		t.Fatalf("switch token: %v", err)
	}
	oldPlan := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{}).Plans[0]

	// 模拟配置发布中的半完成阶段：KeyManager 已变化，但 Config/revision 尚未发布。
	// Acquire 必须被 generation barrier 阻塞，不能用旧列表配新索引。
	manager.endpointConfigMu.Lock()
	manager.keyManager.UpdateEndpointKeyCount("credential-target", 1, 0)
	started := make(chan struct{})
	type acquireResult struct {
		admission *AttemptAdmission
		err       error
	}
	resultCh := make(chan acquireResult, 1)
	go func() {
		close(started)
		admission, err := manager.AcquireEndpointAttempt(oldPlan)
		resultCh <- acquireResult{admission: admission, err: err}
	}()
	<-started
	select {
	case result := <-resultCh:
		manager.endpointConfigMu.Unlock()
		if result.admission != nil {
			result.admission.Release()
		}
		t.Fatalf("acquire observed partial publication: %v", result.err)
	case <-time.After(30 * time.Millisecond):
	}

	ep := manager.GetEndpointByNameAny("credential-target")
	ep.mutex.Lock()
	ep.Config.URL = "https://new.example.com"
	ep.Config.Tokens = []config.TokenConfig{{Name: "new", Value: "new-token"}}
	ep.configRevision = NextEndpointConfigRevision()
	ep.mutex.Unlock()
	manager.endpointConfigMu.Unlock()

	result := <-resultCh
	if result.admission != nil {
		result.admission.Release()
		t.Fatal("stale plan must not be admitted after publication completes")
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "config_changed_since_snapshot") {
		t.Fatalf("expected stale revision rejection, got %v", result.err)
	}
}

func TestAcquireEndpointAttemptUsesCredentialsCapturedDuringPlanning(t *testing.T) {
	cfg := &config.Config{Endpoints: []config.EndpointConfig{{
		Name: "credential-target", URL: "https://example.com", Priority: 1,
		Tokens:  []config.TokenConfig{{Name: "first", Value: "token-1"}, {Name: "second", Value: "token-2"}},
		ApiKeys: []config.ApiKeyConfig{{Name: "first", Value: "key-1"}, {Name: "second", Value: "key-2"}},
	}}}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)

	plan := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{}).Plans[0]
	if err := manager.keyManager.SwitchToken("credential-target", 1); err != nil {
		t.Fatalf("switch token: %v", err)
	}
	if err := manager.keyManager.SwitchApiKey("credential-target", 1); err != nil {
		t.Fatalf("switch api key: %v", err)
	}

	admission, err := manager.AcquireEndpointAttempt(plan)
	if err != nil {
		t.Fatalf("acquire attempt: %v", err)
	}
	t.Cleanup(admission.Release)
	got := admission.Target.Config()
	if got.Token != "token-1" || got.ApiKey != "key-1" {
		t.Fatalf("attempt credentials changed after planning: token=%q apiKey=%q", got.Token, got.ApiKey)
	}
}

func TestAttemptSettlementDoesNotMutateNewRevision(t *testing.T) {
	cfg := &config.Config{
		FailureTracker: config.FailureTrackerConfig{Enabled: true, TimeWindow: time.Minute, Threshold: 3},
		Endpoints:      []config.EndpointConfig{{Name: "settlement-target", URL: "https://old.example.com", Priority: 1}},
	}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)
	profile := BuildRouteRequestProfile("/v1/messages", []byte(`{"model":"old-model"}`))
	oldPlan := manager.PrepareRouteCandidates(context.Background(), profile).Plans[0]
	oldAdmission, err := manager.AcquireEndpointAttempt(oldPlan)
	if err != nil {
		t.Fatalf("acquire old attempt: %v", err)
	}
	oldRevision := oldAdmission.Target.Revision()
	oldAdmission.Release()

	updated := cfg.Endpoints[0]
	updated.URL = "https://new.example.com"
	if err := manager.UpdateEndpointConfig("settlement-target", updated); err != nil {
		t.Fatalf("update endpoint: %v", err)
	}

	applied := manager.ApplyEndpointAttemptSettlement("settlement-target", oldRevision, func() {
		manager.RecordSoftFailure("settlement-target", SoftFailureScopeMessages, SoftFailureCategoryServerError)
		manager.SetEndpointCooldown("settlement-target", time.Minute, "auth_rejected")
		manager.RecordNegativeRouteHit("settlement-target", FailureClassModelUnsupported, profile, "old response")
	})
	if applied {
		t.Fatal("old attempt settlement must be fenced after config edit")
	}
	if counts := manager.GetSoftFailureCounts("settlement-target", SoftFailureScopeMessages); len(counts) != 0 {
		t.Fatalf("old attempt polluted soft failures: %+v", counts)
	}
	if inCooldown, _, _ := manager.GetEndpointCooldownInfo("settlement-target"); inCooldown {
		t.Fatal("old attempt polluted cooldown state")
	}
	if hit, _ := manager.routeState.HasNegativeHit("settlement-target", profile); hit {
		t.Fatal("old attempt polluted negative route cache")
	}

	manager.RecordSoftFailure("settlement-target", SoftFailureScopeMessages, SoftFailureCategoryConnection)
	manager.ApplyEndpointAttemptSettlement("settlement-target", oldRevision, func() {
		manager.RecordSuccessSince("settlement-target", time.Now())
	})
	counts := manager.GetSoftFailureCounts("settlement-target", SoftFailureScopeMessages)
	if counts[SoftFailureCategoryConnection] != 1 {
		t.Fatalf("old success must not clear new revision failures: %+v", counts)
	}
}

func TestPrepareRouteCandidatesUsesConsistentConfigSnapshotsDuringUpdates(t *testing.T) {
	auto := true
	cfgA := config.EndpointConfig{
		Name: "moving", URL: "https://a.example.com", Channel: "a", Priority: 1,
		Timeout: time.Second, SupportsCountTokens: true, FailoverEnabled: &auto,
	}
	cfgB := config.EndpointConfig{
		Name: "moving", URL: "https://b.example.com", Channel: "b", Priority: 3,
		Timeout: 3 * time.Second, SupportsCountTokens: false, FailoverEnabled: &auto,
	}
	cfg := &config.Config{
		Strategy: config.StrategyConfig{Type: "priority"},
		Failover: config.FailoverConfig{Enabled: true},
		Endpoints: []config.EndpointConfig{
			cfgA,
			{Name: "stable", URL: "https://stable.example.com", Channel: "stable", Priority: 2, Timeout: 2 * time.Second},
		},
	}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)

	const iterations = 300
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			next := cfgA
			if i%2 == 1 {
				next = cfgB
			}
			if err := manager.UpdateEndpointConfig("moving", next); err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}()

	for i := 0; i < iterations; i++ {
		result := manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{})
		if len(result.Plans) != 2 {
			t.Fatalf("expected two plans, got %+v", result.Plans)
		}
		var moving EndpointAttemptPlan
		movingIndex := -1
		for idx, plan := range result.Plans {
			if plan.EndpointName == "moving" {
				moving = plan
				movingIndex = idx
				break
			}
		}
		switch moving.URL {
		case cfgA.URL:
			if moving.Channel != cfgA.Channel || moving.Priority != cfgA.Priority || moving.Timeout != cfgA.Timeout || !moving.SupportsCountTokens || movingIndex != 0 {
				t.Fatalf("mixed A snapshot: plan=%+v index=%d", moving, movingIndex)
			}
		case cfgB.URL:
			if moving.Channel != cfgB.Channel || moving.Priority != cfgB.Priority || moving.Timeout != cfgB.Timeout || moving.SupportsCountTokens || movingIndex != 1 {
				t.Fatalf("mixed B snapshot: plan=%+v index=%d", moving, movingIndex)
			}
		default:
			t.Fatalf("unexpected moving snapshot: %+v", moving)
		}
	}
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatalf("concurrent update failed: %v", err)
	default:
	}
}

func TestAcquireEndpointAttemptSnapshotsInheritedCredentials(t *testing.T) {
	cfg := &config.Config{Failover: config.FailoverConfig{Enabled: true}, Endpoints: []config.EndpointConfig{
		{Name: "credential-source", Group: "shared", Token: "old-token", ApiKey: "old-key"},
		{Name: "target", Group: "shared", URL: "https://target.example.com", Timeout: time.Second},
	}}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)

	var plan EndpointAttemptPlan
	for _, candidate := range manager.PrepareRouteCandidates(context.Background(), RouteRequestProfile{}).Plans {
		if candidate.EndpointName == "target" {
			plan = candidate
			break
		}
	}
	if plan.EndpointName == "" {
		t.Fatal("target attempt plan not found")
	}

	donorUpdate := cfg.Endpoints[0]
	donorUpdate.Token = "new-token"
	donorUpdate.ApiKey = "new-key"
	if err := manager.UpdateEndpointConfig("credential-source", donorUpdate); err != nil {
		t.Fatalf("update credential source: %v", err)
	}
	admission, err := manager.AcquireEndpointAttempt(plan)
	if err != nil {
		t.Fatalf("acquire attempt: %v", err)
	}
	t.Cleanup(admission.Release)

	targetConfig := admission.Target.Config()
	if targetConfig.Token != "old-token" || targetConfig.ApiKey != "old-key" {
		t.Fatalf("inherited credentials were not frozen: token=%q apiKey=%q", targetConfig.Token, targetConfig.ApiKey)
	}
}

func TestRemoveEndpointClearsNameKeyedRuntimeStateBeforeRecreate(t *testing.T) {
	cfg := &config.Config{
		FailureTracker: config.FailureTrackerConfig{Enabled: true, TimeWindow: time.Minute, Threshold: 3},
		Endpoints:      []config.EndpointConfig{{Name: "recreated", URL: "https://old.example.com", Priority: 1}},
	}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)
	profile := BuildRouteRequestProfile("/v1/messages/count_tokens", []byte(`{"model":"old-model"}`))

	manager.RecordNegativeRouteHit("recreated", FailureClassCountTokensUnsupported, profile, "unsupported")
	manager.SetScopedCooldown("recreated", SoftFailureScopeCountTokens, time.Minute, "soft_failure_rate_limit")
	manager.RecordSoftFailure("recreated", SoftFailureScopeMessages, SoftFailureCategoryConnection)
	if !manager.UpdateAutoRetention("recreated", 1, "auto_priority", 0) {
		t.Fatal("expected retained setup")
	}

	if err := manager.RemoveEndpoint("recreated"); err != nil {
		t.Fatalf("remove endpoint: %v", err)
	}
	if err := manager.AddEndpoint(config.EndpointConfig{Name: "recreated", URL: "https://new.example.com", Priority: 1}); err != nil {
		t.Fatalf("recreate endpoint: %v", err)
	}

	if hit, _ := manager.routeState.HasNegativeHit("recreated", profile); hit {
		t.Fatal("negative cache survived endpoint recreation")
	}
	if active, _, _ := manager.ScopedCooldownActive("recreated", SoftFailureScopeCountTokens); active {
		t.Fatal("scoped cooldown survived endpoint recreation")
	}
	if counts := manager.GetSoftFailureCounts("recreated", SoftFailureScopeMessages); len(counts) != 0 {
		t.Fatalf("soft failures survived endpoint recreation: %+v", counts)
	}
	if got := manager.RetainedInTier(1); got != "" {
		t.Fatalf("retained state survived endpoint recreation: %q", got)
	}
}
