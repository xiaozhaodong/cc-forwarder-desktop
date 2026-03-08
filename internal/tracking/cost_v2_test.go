package tracking

import (
	"math"
	"testing"
)

// TestCalculateCostV2_Basic 测试基础成本计算
func TestCalculateCostV2_Basic(t *testing.T) {
	pricing := &ModelPricing{
		Input:           3.0,  // $3/1M tokens
		Output:          15.0, // $15/1M tokens
		CacheCreation:   3.75, // $3.75/1M tokens (5m cache = 1.25x input)
		CacheCreation1h: 6.0,  // $6/1M tokens (1h cache = 2x input)
		CacheRead:       0.30, // $0.30/1M tokens
	}

	usage := &TokenUsage{
		InputTokens:  1000000,
		OutputTokens: 100000,
	}

	result := CalculateCostV2(usage, pricing, nil)

	// Input: 1M tokens * $3/1M = $3.00
	if math.Abs(result.InputCost-3.0) > 0.001 {
		t.Errorf("Expected InputCost $3.00, got $%f", result.InputCost)
	}

	// Output: 0.1M tokens * $15/1M = $1.50
	if math.Abs(result.OutputCost-1.5) > 0.001 {
		t.Errorf("Expected OutputCost $1.50, got $%f", result.OutputCost)
	}

	// Total: $3.00 + $1.50 = $4.50
	if math.Abs(result.TotalCost-4.5) > 0.001 {
		t.Errorf("Expected TotalCost $4.50, got $%f", result.TotalCost)
	}
}

// TestCalculateCostV2_Separate5mAnd1hCache 测试分开的 5m/1h 缓存定价
func TestCalculateCostV2_Separate5mAnd1hCache(t *testing.T) {
	// Claude Opus 4.5 定价
	pricing := &ModelPricing{
		Input:           15.0,  // $15/1M tokens
		Output:          75.0,  // $75/1M tokens
		CacheCreation:   18.75, // $18.75/1M tokens (5m = 1.25x input)
		CacheCreation1h: 30.0,  // $30/1M tokens (1h = 2x input)
		CacheRead:       1.50,  // $1.50/1M tokens
	}

	usage := &TokenUsage{
		InputTokens:           10000,
		OutputTokens:          5000,
		CacheCreation5mTokens: 0,     // 无 5 分钟缓存
		CacheCreation1hTokens: 1000,  // 1000 个 1 小时缓存 tokens
		CacheCreationTokens:   1000,  // 总数（向后兼容）
		CacheReadTokens:       50000, // 50000 个缓存读取 tokens
	}

	result := CalculateCostV2(usage, pricing, nil)

	// Input: 10000 tokens * $15/1M = $0.15
	expectedInput := 10000.0 * 15.0 / 1_000_000
	if math.Abs(result.InputCost-expectedInput) > 0.0001 {
		t.Errorf("Expected InputCost $%f, got $%f", expectedInput, result.InputCost)
	}

	// Output: 5000 tokens * $75/1M = $0.375
	expectedOutput := 5000.0 * 75.0 / 1_000_000
	if math.Abs(result.OutputCost-expectedOutput) > 0.0001 {
		t.Errorf("Expected OutputCost $%f, got $%f", expectedOutput, result.OutputCost)
	}

	// 5m Cache: 0 tokens = $0.00
	if math.Abs(result.CacheCreation5mCost-0) > 0.0001 {
		t.Errorf("Expected CacheCreation5mCost $0.00, got $%f", result.CacheCreation5mCost)
	}

	// 1h Cache: 1000 tokens * $30/1M = $0.03
	expectedCache1h := 1000.0 * 30.0 / 1_000_000
	if math.Abs(result.CacheCreation1hCost-expectedCache1h) > 0.0001 {
		t.Errorf("Expected CacheCreation1hCost $%f, got $%f", expectedCache1h, result.CacheCreation1hCost)
	}

	// Total Cache: $0.03
	if math.Abs(result.CacheCreationCost-expectedCache1h) > 0.0001 {
		t.Errorf("Expected CacheCreationCost $%f, got $%f", expectedCache1h, result.CacheCreationCost)
	}

	// Cache Read: 50000 tokens * $1.50/1M = $0.075
	expectedCacheRead := 50000.0 * 1.50 / 1_000_000
	if math.Abs(result.CacheReadCost-expectedCacheRead) > 0.0001 {
		t.Errorf("Expected CacheReadCost $%f, got $%f", expectedCacheRead, result.CacheReadCost)
	}

	// Total: $0.15 + $0.375 + $0.03 + $0.075 = $0.63
	expectedTotal := expectedInput + expectedOutput + expectedCache1h + expectedCacheRead
	if math.Abs(result.TotalCost-expectedTotal) > 0.0001 {
		t.Errorf("Expected TotalCost $%f, got $%f", expectedTotal, result.TotalCost)
	}
}

// TestCalculateCostV2_BackwardCompatibility 测试向后兼容（只有总缓存数）
func TestCalculateCostV2_BackwardCompatibility(t *testing.T) {
	pricing := &ModelPricing{
		Input:           3.0,
		Output:          15.0,
		CacheCreation:   3.75, // 5m 价格
		CacheCreation1h: 6.0,  // 1h 价格
		CacheRead:       0.30,
	}

	// 只有 CacheCreationTokens 总数，没有分开的 5m/1h
	usage := &TokenUsage{
		InputTokens:           100000,
		OutputTokens:          50000,
		CacheCreationTokens:   10000, // 总数
		CacheCreation5mTokens: 0,     // 无分开数据
		CacheCreation1hTokens: 0,     // 无分开数据
		CacheReadTokens:       20000,
	}

	result := CalculateCostV2(usage, pricing, nil)

	// 向后兼容：使用 5m 定价
	expectedCache5m := 10000.0 * 3.75 / 1_000_000
	if math.Abs(result.CacheCreation5mCost-expectedCache5m) > 0.0001 {
		t.Errorf("Backward compatible: Expected CacheCreation5mCost $%f, got $%f", expectedCache5m, result.CacheCreation5mCost)
	}

	if math.Abs(result.CacheCreation1hCost-0) > 0.0001 {
		t.Errorf("Backward compatible: Expected CacheCreation1hCost $0, got $%f", result.CacheCreation1hCost)
	}
}

// TestCalculateCostV2_Mixed5mAnd1hCache 测试混合 5m 和 1h 缓存
func TestCalculateCostV2_Mixed5mAnd1hCache(t *testing.T) {
	pricing := &ModelPricing{
		Input:           3.0,
		Output:          15.0,
		CacheCreation:   3.75, // 5m = 1.25x
		CacheCreation1h: 6.0,  // 1h = 2x
		CacheRead:       0.30,
	}

	usage := &TokenUsage{
		InputTokens:           100000,
		OutputTokens:          50000,
		CacheCreation5mTokens: 5000, // 5000 个 5 分钟缓存
		CacheCreation1hTokens: 3000, // 3000 个 1 小时缓存
		CacheCreationTokens:   8000, // 总数
		CacheReadTokens:       20000,
	}

	result := CalculateCostV2(usage, pricing, nil)

	// 5m Cache: 5000 * $3.75/1M = $0.01875
	expectedCache5m := 5000.0 * 3.75 / 1_000_000
	if math.Abs(result.CacheCreation5mCost-expectedCache5m) > 0.0001 {
		t.Errorf("Expected CacheCreation5mCost $%f, got $%f", expectedCache5m, result.CacheCreation5mCost)
	}

	// 1h Cache: 3000 * $6/1M = $0.018
	expectedCache1h := 3000.0 * 6.0 / 1_000_000
	if math.Abs(result.CacheCreation1hCost-expectedCache1h) > 0.0001 {
		t.Errorf("Expected CacheCreation1hCost $%f, got $%f", expectedCache1h, result.CacheCreation1hCost)
	}

	// Total Cache: $0.01875 + $0.018 = $0.03675
	expectedTotalCache := expectedCache5m + expectedCache1h
	if math.Abs(result.CacheCreationCost-expectedTotalCache) > 0.0001 {
		t.Errorf("Expected CacheCreationCost $%f, got $%f", expectedTotalCache, result.CacheCreationCost)
	}
}

// TestCalculateCostV2_WithEndpointMultiplier 测试端点倍率
func TestCalculateCostV2_WithEndpointMultiplier(t *testing.T) {
	pricing := &ModelPricing{
		Input:           3.0,
		Output:          15.0,
		CacheCreation:   3.75,
		CacheCreation1h: 6.0,
		CacheRead:       0.30,
	}

	usage := &TokenUsage{
		InputTokens:           100000,
		OutputTokens:          50000,
		CacheCreation5mTokens: 5000,
		CacheCreation1hTokens: 3000,
		CacheReadTokens:       20000,
	}

	// 分项倍率模式
	multiplier := &EndpointMultiplier{
		CostMultiplier:                0,   // 不使用总体倍率
		InputCostMultiplier:           1.5, // 输入 1.5x
		OutputCostMultiplier:          1.2, // 输出 1.2x
		CacheCreationCostMultiplier:   1.0, // 5m 缓存 1.0x
		CacheCreationCostMultiplier1h: 2.0, // 1h 缓存 2.0x
		CacheReadCostMultiplier:       1.0, // 读取 1.0x
	}

	result := CalculateCostV2(usage, pricing, multiplier)

	// Input: 100000 * $3/1M * 1.5 = $0.45
	expectedInput := 100000.0 * 3.0 / 1_000_000 * 1.5
	if math.Abs(result.InputCost-expectedInput) > 0.0001 {
		t.Errorf("With multiplier: Expected InputCost $%f, got $%f", expectedInput, result.InputCost)
	}

	// 1h Cache: 3000 * $6/1M * 2.0 = $0.036
	expectedCache1h := 3000.0 * 6.0 / 1_000_000 * 2.0
	if math.Abs(result.CacheCreation1hCost-expectedCache1h) > 0.0001 {
		t.Errorf("With multiplier: Expected CacheCreation1hCost $%f, got $%f", expectedCache1h, result.CacheCreation1hCost)
	}
}

// TestCalculateCostV2_TotalMultiplier 测试总体倍率模式
func TestCalculateCostV2_TotalMultiplier(t *testing.T) {
	pricing := &ModelPricing{
		Input:           3.0,
		Output:          15.0,
		CacheCreation:   3.75,
		CacheCreation1h: 6.0,
		CacheRead:       0.30,
	}

	usage := &TokenUsage{
		InputTokens:           100000,
		OutputTokens:          50000,
		CacheCreation5mTokens: 5000,
		CacheCreation1hTokens: 3000,
		CacheReadTokens:       20000,
	}

	// 总体倍率模式
	multiplier := &EndpointMultiplier{
		CostMultiplier: 1.5, // 所有成本 1.5x
	}

	result := CalculateCostV2(usage, pricing, multiplier)

	// 先计算无倍率的成本
	baseResult := CalculateCostV2(usage, pricing, nil)

	// 验证所有成本都乘以 1.5
	if math.Abs(result.InputCost-baseResult.InputCost*1.5) > 0.0001 {
		t.Errorf("Total multiplier: InputCost mismatch")
	}
	if math.Abs(result.OutputCost-baseResult.OutputCost*1.5) > 0.0001 {
		t.Errorf("Total multiplier: OutputCost mismatch")
	}
	if math.Abs(result.CacheCreation5mCost-baseResult.CacheCreation5mCost*1.5) > 0.0001 {
		t.Errorf("Total multiplier: CacheCreation5mCost mismatch")
	}
	if math.Abs(result.CacheCreation1hCost-baseResult.CacheCreation1hCost*1.5) > 0.0001 {
		t.Errorf("Total multiplier: CacheCreation1hCost mismatch")
	}
	if math.Abs(result.TotalCost-baseResult.TotalCost*1.5) > 0.0001 {
		t.Errorf("Total multiplier: TotalCost mismatch, expected $%f, got $%f", baseResult.TotalCost*1.5, result.TotalCost)
	}
}

// TestCalculateCostV2_NilInputs 测试空输入
func TestCalculateCostV2_NilInputs(t *testing.T) {
	pricing := &ModelPricing{
		Input:  3.0,
		Output: 15.0,
	}

	// nil usage
	result := CalculateCostV2(nil, pricing, nil)
	if result.TotalCost != 0 {
		t.Errorf("nil usage: Expected TotalCost $0, got $%f", result.TotalCost)
	}

	// nil pricing
	usage := &TokenUsage{InputTokens: 1000}
	result = CalculateCostV2(usage, nil, nil)
	if result.TotalCost != 0 {
		t.Errorf("nil pricing: Expected TotalCost $0, got $%f", result.TotalCost)
	}
}

// TestCalculateCostV2_Default1hPrice 测试 1h 定价默认值
func TestCalculateCostV2_Default1hPrice(t *testing.T) {
	// 不设置 CacheCreation1h，应默认使用 2x input
	pricing := &ModelPricing{
		Input:           10.0,
		Output:          50.0,
		CacheCreation:   12.5, // 1.25x input
		CacheCreation1h: 0,    // 未设置，应默认 2x input = 20.0
		CacheRead:       1.0,
	}

	usage := &TokenUsage{
		CacheCreation1hTokens: 10000,
	}

	result := CalculateCostV2(usage, pricing, nil)

	// 1h Cache: 10000 * $20/1M = $0.20 (因为默认 2x input = 2 * 10 = 20)
	expectedCache1h := 10000.0 * 20.0 / 1_000_000
	if math.Abs(result.CacheCreation1hCost-expectedCache1h) > 0.0001 {
		t.Errorf("Default 1h price: Expected CacheCreation1hCost $%f, got $%f", expectedCache1h, result.CacheCreation1hCost)
	}
}

// TestCalculateCostV2_RealWorldScenario 测试真实场景
// 模拟请求 req-c2a06ef3: claude-opus-4-5-20251101
func TestCalculateCostV2_RealWorldScenario(t *testing.T) {
	// Claude Opus 4.5 官方定价
	pricing := &ModelPricing{
		Input:           15.0,  // $15/1M tokens
		Output:          75.0,  // $75/1M tokens
		CacheCreation:   18.75, // $18.75/1M (5m = 1.25x input)
		CacheCreation1h: 30.0,  // $30/1M (1h = 2x input)
		CacheRead:       1.50,  // $1.50/1M
	}

	// 真实请求数据（从之前的调查）
	usage := &TokenUsage{
		InputTokens:           5000,
		OutputTokens:          800,
		CacheCreation1hTokens: 2000,  // 使用 1 小时缓存
		CacheCreation5mTokens: 0,     // 无 5 分钟缓存
		CacheCreationTokens:   2000,  // 总数
		CacheReadTokens:       10000, // 缓存读取
	}

	result := CalculateCostV2(usage, pricing, nil)

	// 手动计算预期成本
	// Input: 5000 * $15/1M = $0.075
	expectedInput := 5000.0 * 15.0 / 1_000_000
	// Output: 800 * $75/1M = $0.06
	expectedOutput := 800.0 * 75.0 / 1_000_000
	// 1h Cache: 2000 * $30/1M = $0.06
	expectedCache1h := 2000.0 * 30.0 / 1_000_000
	// Cache Read: 10000 * $1.50/1M = $0.015
	expectedCacheRead := 10000.0 * 1.50 / 1_000_000

	expectedTotal := expectedInput + expectedOutput + expectedCache1h + expectedCacheRead

	t.Logf("Real-world scenario cost breakdown:")
	t.Logf("  Input:       $%.6f (expected $%.6f)", result.InputCost, expectedInput)
	t.Logf("  Output:      $%.6f (expected $%.6f)", result.OutputCost, expectedOutput)
	t.Logf("  Cache 5m:    $%.6f (expected $0.000000)", result.CacheCreation5mCost)
	t.Logf("  Cache 1h:    $%.6f (expected $%.6f)", result.CacheCreation1hCost, expectedCache1h)
	t.Logf("  Cache Read:  $%.6f (expected $%.6f)", result.CacheReadCost, expectedCacheRead)
	t.Logf("  Total:       $%.6f (expected $%.6f)", result.TotalCost, expectedTotal)

	if math.Abs(result.TotalCost-expectedTotal) > 0.0001 {
		t.Errorf("Real-world: Expected TotalCost $%f, got $%f", expectedTotal, result.TotalCost)
	}
}

// TestCalculateCost_BackwardCompatible 测试旧版 CalculateCost 向后兼容
func TestCalculateCost_BackwardCompatible(t *testing.T) {
	pricing := &ModelPricing{
		Input:           3.0,
		Output:          15.0,
		CacheCreation:   3.75,
		CacheCreation1h: 6.0,
		CacheRead:       0.30,
	}

	// 使用旧版 CalculateCost，use1hCache = false
	result5m := CalculateCost(100000, 50000, 10000, 20000, pricing, nil, false)

	// 缓存应该使用 5m 定价
	expectedCache5m := 10000.0 * 3.75 / 1_000_000
	if math.Abs(result5m.CacheCreation5mCost-expectedCache5m) > 0.0001 {
		t.Errorf("CalculateCost(use1hCache=false): Expected CacheCreation5mCost $%f, got $%f", expectedCache5m, result5m.CacheCreation5mCost)
	}
	if math.Abs(result5m.CacheCreation1hCost-0) > 0.0001 {
		t.Errorf("CalculateCost(use1hCache=false): Expected CacheCreation1hCost $0, got $%f", result5m.CacheCreation1hCost)
	}

	// 使用旧版 CalculateCost，use1hCache = true
	result1h := CalculateCost(100000, 50000, 10000, 20000, pricing, nil, true)

	// 缓存应该使用 1h 定价
	expectedCache1h := 10000.0 * 6.0 / 1_000_000
	if math.Abs(result1h.CacheCreation1hCost-expectedCache1h) > 0.0001 {
		t.Errorf("CalculateCost(use1hCache=true): Expected CacheCreation1hCost $%f, got $%f", expectedCache1h, result1h.CacheCreation1hCost)
	}
	if math.Abs(result1h.CacheCreation5mCost-0) > 0.0001 {
		t.Errorf("CalculateCost(use1hCache=true): Expected CacheCreation5mCost $0, got $%f", result1h.CacheCreation5mCost)
	}
}

func TestCalculateCostV2_ResponsesInputIncludesCachedTokens(t *testing.T) {
	pricing := &ModelPricing{
		Input:         2.5,
		Output:        15.0,
		CacheCreation: 3.125,
		CacheRead:     0.25,
	}

	usage := &TokenUsage{
		InputTokens:     21135,
		OutputTokens:    62,
		CacheReadTokens: 5760,
	}
	multiplier := &EndpointMultiplier{CostMultiplier: 0.6}

	result := CalculateCostV2(usage, pricing, multiplier, "/v1/responses")

	expectedInput := float64(21135-5760) * 2.5 / 1_000_000 * 0.6
	expectedOutput := float64(62) * 15.0 / 1_000_000 * 0.6
	expectedCacheRead := float64(5760) * 0.25 / 1_000_000 * 0.6
	expectedTotal := expectedInput + expectedOutput + expectedCacheRead

	if math.Abs(result.InputCost-expectedInput) > 0.0000001 {
		t.Fatalf("expected InputCost %f, got %f", expectedInput, result.InputCost)
	}
	if math.Abs(result.OutputCost-expectedOutput) > 0.0000001 {
		t.Fatalf("expected OutputCost %f, got %f", expectedOutput, result.OutputCost)
	}
	if math.Abs(result.CacheReadCost-expectedCacheRead) > 0.0000001 {
		t.Fatalf("expected CacheReadCost %f, got %f", expectedCacheRead, result.CacheReadCost)
	}
	if math.Abs(result.TotalCost-expectedTotal) > 0.0000001 {
		t.Fatalf("expected TotalCost %f, got %f", expectedTotal, result.TotalCost)
	}
}
