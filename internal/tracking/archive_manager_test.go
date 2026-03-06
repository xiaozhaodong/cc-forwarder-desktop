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
