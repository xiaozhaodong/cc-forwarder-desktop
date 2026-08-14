package tracking

import (
	"testing"
	"time"
)

func TestArchiveManager_UpdatePricingClonesInput(t *testing.T) {
	am := &ArchiveManager{}
	input := map[string]ModelPricing{
		"model-a": {Input: 1.0, Output: 2.0},
	}

	am.UpdatePricing(input)
	input["model-a"] = ModelPricing{Input: 9.0, Output: 9.0}

	am.cacheMu.RLock()
	pricing := am.pricing["model-a"]
	am.cacheMu.RUnlock()

	if pricing.Input != 1.0 || pricing.Output != 2.0 {
		t.Fatalf("archive manager pricing should not track external map mutations, got %+v", pricing)
	}
}

func TestArchiveManager_UpdateEndpointMultipliersClonesInput(t *testing.T) {
	am := &ArchiveManager{}
	input := map[string]EndpointMultiplier{
		"ep-a": {CostMultiplier: 1.5},
	}

	am.UpdateEndpointMultipliers(input)
	input["ep-a"] = EndpointMultiplier{CostMultiplier: 9.9}

	am.cacheMu.RLock()
	multiplier := am.endpointMultipliers["ep-a"]
	am.cacheMu.RUnlock()

	if multiplier.CostMultiplier != 1.5 {
		t.Fatalf("archive manager endpoint multipliers should not track external map mutations, got %+v", multiplier)
	}
}

func TestArchiveManager_UpdateAccountMultipliersClonesInput(t *testing.T) {
	am := &ArchiveManager{}
	input := map[int64]EndpointMultiplier{
		10: {CostMultiplier: 1.7},
	}

	am.UpdateAccountMultipliers(input)
	input[10] = EndpointMultiplier{CostMultiplier: 9.9}

	am.cacheMu.RLock()
	multiplier := am.accountMultipliers[10]
	am.cacheMu.RUnlock()

	if multiplier.CostMultiplier != 1.7 {
		t.Fatalf("archive manager account multipliers should not track external map mutations, got %+v", multiplier)
	}
}

func TestArchiveManager_CalculateCostV2_UsesAccountMultiplierByUpstreamID(t *testing.T) {
	am := &ArchiveManager{
		pricing: map[string]ModelPricing{
			"gpt-5": {Input: 10, Output: 20, CacheCreation: 12, CacheCreation1h: 14, CacheRead: 5},
		},
		endpointMultipliers: map[string]EndpointMultiplier{
			"acc-1": {CostMultiplier: 9.0},
		},
		accountMultipliers: map[int64]EndpointMultiplier{
			42: {CostMultiplier: 2.0},
		},
	}

	req := &ActiveRequest{
		ModelName:    "gpt-5",
		UpstreamType: "account",
		UpstreamID:   42,
		EndpointName: "acc-1",
		InputTokens:  1000,
	}

	result := am.calculateCostV2(req)
	expectedBase := 1000.0 * 10.0 / 1_000_000
	if result.InputCost != expectedBase*2.0 {
		t.Fatalf("expected account multiplier to apply by upstream_id, got %+v", result)
	}
}

func TestArchiveManager_BatchInsertMarksArchivePersistedOnlyAfterCommit(t *testing.T) {
	adapter, err := NewSQLiteAdapter(DatabaseConfig{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("NewSQLiteAdapter failed: %v", err)
	}
	if err := adapter.Open(); err != nil {
		t.Fatalf("adapter.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.InitSchema(); err != nil {
		t.Fatalf("adapter.InitSchema failed: %v", err)
	}

	am := &ArchiveManager{adapter: adapter}
	startTime := time.Now().Add(-time.Second)
	endTime := time.Now()
	req := &ActiveRequest{
		RequestID:  "req-archive-commit-barrier",
		StartTime:  startTime,
		EndTime:    &endTime,
		Method:     "POST",
		Path:       "/v1/responses",
		Status:     "completed",
		HTTPStatus: 200,
	}

	if err := am.batchInsert([]*ArchiveEvent{{Request: req, Timestamp: endTime}}); err != nil {
		t.Fatalf("batchInsert failed: %v", err)
	}

	req.mu.RLock()
	persisted := req.archivePersisted
	req.mu.RUnlock()
	if !persisted {
		t.Fatal("expected archivePersisted after successful commit")
	}

	var status string
	if err := adapter.GetReadDB().QueryRow(
		"SELECT status FROM request_logs WHERE request_id = ?", req.RequestID,
	).Scan(&status); err != nil {
		t.Fatalf("query committed request failed: %v", err)
	}
	if status != "completed" {
		t.Fatalf("expected committed status completed, got %q", status)
	}
}

func TestArchiveManager_BatchInsertDoesNotMarkArchivePersistedOnRollback(t *testing.T) {
	adapter, err := NewSQLiteAdapter(DatabaseConfig{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("NewSQLiteAdapter failed: %v", err)
	}
	if err := adapter.Open(); err != nil {
		t.Fatalf("adapter.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.InitSchema(); err != nil {
		t.Fatalf("adapter.InitSchema failed: %v", err)
	}

	am := &ArchiveManager{adapter: adapter}
	req := &ActiveRequest{
		RequestID: "req-archive-rollback",
		StartTime: time.Now(),
		Method:    "POST",
		Path:      "/v1/responses",
		Status:    "completed",
	}
	event := &ArchiveEvent{Request: req, Timestamp: time.Now()}

	if err := am.batchInsert([]*ArchiveEvent{event, event}); err == nil {
		t.Fatal("expected duplicate request batch to roll back")
	}

	req.mu.RLock()
	persisted := req.archivePersisted
	req.mu.RUnlock()
	if persisted {
		t.Fatal("archivePersisted must remain false after rollback")
	}

	var count int
	if err := adapter.GetReadDB().QueryRow(
		"SELECT COUNT(*) FROM request_logs WHERE request_id = ?", req.RequestID,
	).Scan(&count); err != nil {
		t.Fatalf("query rollback result failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected rolled back request absent from database, got count %d", count)
	}
}
