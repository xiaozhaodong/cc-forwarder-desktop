package store

import "testing"

func TestNormalizeAccountRecord_ApiKeyPreservesCustomMultipliers(t *testing.T) {
	record := &UpstreamAccountRecord{
		ProviderType:                  "api_key",
		AccountName:                   "api-key-account",
		CredentialRaw:                 "sk-test",
		BaseURL:                       "https://api.openai.com",
		CostMultiplier:                1.8,
		InputCostMultiplier:           1.2,
		OutputCostMultiplier:          1.3,
		CacheCreationCostMultiplier:   1.4,
		CacheCreationCostMultiplier1h: 1.5,
		CacheReadCostMultiplier:       1.1,
	}

	normalizeAccountRecord(record)

	if record.CostMultiplier != 1.8 || record.InputCostMultiplier != 1.2 || record.OutputCostMultiplier != 1.3 {
		t.Fatalf("expected custom api_key multipliers to be preserved, got %+v", record)
	}
	if record.CacheCreationCostMultiplier != 1.4 || record.CacheCreationCostMultiplier1h != 1.5 || record.CacheReadCostMultiplier != 1.1 {
		t.Fatalf("expected custom cache multipliers to be preserved, got %+v", record)
	}
}

func TestNormalizeAccountRecord_NonAPIKeyResetsMultipliersToDefault(t *testing.T) {
	record := &UpstreamAccountRecord{
		ProviderType:                  "chatgpt_refresh_token",
		AccountName:                   "oauth-account",
		CredentialRaw:                 "rt-test",
		BaseURL:                       "https://chatgpt.com",
		CostMultiplier:                9.9,
		InputCostMultiplier:           8.8,
		OutputCostMultiplier:          7.7,
		CacheCreationCostMultiplier:   6.6,
		CacheCreationCostMultiplier1h: 5.5,
		CacheReadCostMultiplier:       4.4,
	}

	normalizeAccountRecord(record)

	if record.CostMultiplier != 1.0 || record.InputCostMultiplier != 1.0 || record.OutputCostMultiplier != 1.0 {
		t.Fatalf("expected non-api_key multipliers reset to 1.0, got %+v", record)
	}
	if record.CacheCreationCostMultiplier != 1.0 || record.CacheCreationCostMultiplier1h != 1.0 || record.CacheReadCostMultiplier != 1.0 {
		t.Fatalf("expected non-api_key cache multipliers reset to 1.0, got %+v", record)
	}
}
