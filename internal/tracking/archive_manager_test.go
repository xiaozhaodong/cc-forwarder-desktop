package tracking

import "testing"

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
