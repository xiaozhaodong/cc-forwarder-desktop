package endpoint

import (
	"sync"
	"testing"
	"time"
)

func TestKeyManager_InitEndpoint(t *testing.T) {
	km := NewKeyManager()

	// 初始化端点
	km.InitEndpoint("test-endpoint", 3, 2)

	// 验证初始状态
	if idx := km.GetActiveTokenIndex("test-endpoint"); idx != 0 {
		t.Errorf("期望初始 Token 索引为 0，实际为 %d", idx)
	}

	if idx := km.GetActiveApiKeyIndex("test-endpoint"); idx != 0 {
		t.Errorf("期望初始 API Key 索引为 0，实际为 %d", idx)
	}

	// 验证不存在的端点返回 0
	if idx := km.GetActiveTokenIndex("non-existent"); idx != 0 {
		t.Errorf("期望不存在端点返回 0，实际为 %d", idx)
	}
}

func TestKeyManager_SwitchToken(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 测试正常切换
	err := km.SwitchToken("test-endpoint", 1)
	if err != nil {
		t.Errorf("切换 Token 失败: %v", err)
	}

	if idx := km.GetActiveTokenIndex("test-endpoint"); idx != 1 {
		t.Errorf("期望 Token 索引为 1，实际为 %d", idx)
	}

	// 测试切换到最后一个
	err = km.SwitchToken("test-endpoint", 2)
	if err != nil {
		t.Errorf("切换 Token 失败: %v", err)
	}

	if idx := km.GetActiveTokenIndex("test-endpoint"); idx != 2 {
		t.Errorf("期望 Token 索引为 2，实际为 %d", idx)
	}

	// 测试切换回第一个
	err = km.SwitchToken("test-endpoint", 0)
	if err != nil {
		t.Errorf("切换 Token 失败: %v", err)
	}

	if idx := km.GetActiveTokenIndex("test-endpoint"); idx != 0 {
		t.Errorf("期望 Token 索引为 0，实际为 %d", idx)
	}
}

func TestKeyManager_SwitchToken_OutOfRange(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 测试越界切换
	err := km.SwitchToken("test-endpoint", 5)
	if err == nil {
		t.Error("期望越界切换返回错误")
	}

	// 测试负数索引
	err = km.SwitchToken("test-endpoint", -1)
	if err == nil {
		t.Error("期望负数索引返回错误")
	}

	// 测试不存在的端点
	err = km.SwitchToken("non-existent", 0)
	if err == nil {
		t.Error("期望不存在端点返回错误")
	}
}

func TestKeyManager_SwitchApiKey(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 测试正常切换
	err := km.SwitchApiKey("test-endpoint", 1)
	if err != nil {
		t.Errorf("切换 API Key 失败: %v", err)
	}

	if idx := km.GetActiveApiKeyIndex("test-endpoint"); idx != 1 {
		t.Errorf("期望 API Key 索引为 1，实际为 %d", idx)
	}
}

func TestKeyManager_SwitchApiKey_OutOfRange(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 测试越界切换
	err := km.SwitchApiKey("test-endpoint", 3)
	if err == nil {
		t.Error("期望越界切换返回错误")
	}
}

func TestKeyManager_GetEndpointKeyState(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 切换 Token
	km.SwitchToken("test-endpoint", 1)

	state := km.GetEndpointKeyState("test-endpoint")
	if state == nil {
		t.Fatal("期望返回非空状态")
	}

	if state.EndpointName != "test-endpoint" {
		t.Errorf("期望端点名称为 test-endpoint，实际为 %s", state.EndpointName)
	}

	if state.ActiveTokenIndex != 1 {
		t.Errorf("期望 Token 索引为 1，实际为 %d", state.ActiveTokenIndex)
	}

	if state.TokenCount != 3 {
		t.Errorf("期望 Token 总数为 3，实际为 %d", state.TokenCount)
	}

	if state.ApiKeyCount != 2 {
		t.Errorf("期望 API Key 总数为 2，实际为 %d", state.ApiKeyCount)
	}

	// 测试不存在的端点
	nilState := km.GetEndpointKeyState("non-existent")
	if nilState != nil {
		t.Error("期望不存在端点返回 nil")
	}
}

func TestKeyManager_GetAllStates(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("endpoint1", 2, 1)
	km.InitEndpoint("endpoint2", 3, 2)

	states := km.GetAllStates()

	if len(states) != 2 {
		t.Errorf("期望返回 2 个状态，实际为 %d", len(states))
	}

	if _, exists := states["endpoint1"]; !exists {
		t.Error("期望包含 endpoint1")
	}

	if _, exists := states["endpoint2"]; !exists {
		t.Error("期望包含 endpoint2")
	}
}

func TestKeyManager_HasMultipleTokens(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("single", 1, 1)
	km.InitEndpoint("multi", 3, 1)

	if km.HasMultipleTokens("single") {
		t.Error("期望 single 端点返回 false")
	}

	if !km.HasMultipleTokens("multi") {
		t.Error("期望 multi 端点返回 true")
	}

	if km.HasMultipleTokens("non-existent") {
		t.Error("期望不存在端点返回 false")
	}
}

func TestKeyManager_UpdateEndpointKeyCount(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 切换到索引 2
	km.SwitchToken("test-endpoint", 2)

	// 更新 Token 数量为 2（当前索引会超出范围）
	km.UpdateEndpointKeyCount("test-endpoint", 2, 2)

	// 验证索引被重置为 0
	if idx := km.GetActiveTokenIndex("test-endpoint"); idx != 0 {
		t.Errorf("期望索引被重置为 0，实际为 %d", idx)
	}

	// 测试更新不存在的端点（应创建新状态）
	km.UpdateEndpointKeyCount("new-endpoint", 5, 3)
	if idx := km.GetActiveTokenIndex("new-endpoint"); idx != 0 {
		t.Errorf("期望新端点初始索引为 0，实际为 %d", idx)
	}
}

func TestKeyManager_RemoveEndpoint(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 移除端点
	km.RemoveEndpoint("test-endpoint")

	// 验证端点已移除
	state := km.GetEndpointKeyState("test-endpoint")
	if state != nil {
		t.Error("期望移除后返回 nil")
	}
}

func TestKeyManager_LastSwitchTime(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 3, 2)

	// 记录切换前时间
	beforeSwitch := time.Now()

	// 等待一点点时间
	time.Sleep(10 * time.Millisecond)

	// 切换 Token
	km.SwitchToken("test-endpoint", 1)

	state := km.GetEndpointKeyState("test-endpoint")
	if state == nil {
		t.Fatal("期望返回非空状态")
	}

	if state.LastSwitchTime.Before(beforeSwitch) {
		t.Error("期望切换时间在切换操作之后")
	}
}

func TestKeyManager_Concurrency(t *testing.T) {
	km := NewKeyManager()
	km.InitEndpoint("test-endpoint", 10, 10)

	var wg sync.WaitGroup
	concurrency := 100

	// 并发切换 Token
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			km.SwitchToken("test-endpoint", idx%10)
		}(i)
	}

	// 并发切换 API Key
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			km.SwitchApiKey("test-endpoint", idx%10)
		}(i)
	}

	// 并发读取
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			km.GetActiveTokenIndex("test-endpoint")
			km.GetActiveApiKeyIndex("test-endpoint")
			km.GetEndpointKeyState("test-endpoint")
		}()
	}

	wg.Wait()

	// 验证状态仍然有效
	state := km.GetEndpointKeyState("test-endpoint")
	if state == nil {
		t.Fatal("并发操作后状态不应为 nil")
	}

	// 验证索引在有效范围内
	if state.ActiveTokenIndex < 0 || state.ActiveTokenIndex >= 10 {
		t.Errorf("Token 索引超出范围: %d", state.ActiveTokenIndex)
	}

	if state.ActiveApiKeyIndex < 0 || state.ActiveApiKeyIndex >= 10 {
		t.Errorf("API Key 索引超出范围: %d", state.ActiveApiKeyIndex)
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "****"},             // 长度 <= 8
		{"12345678", "****"},          // 长度 == 8
		{"123456789", "1234****6789"}, // 长度 > 8
		{"sk-ant-api03-xxx", "sk-a****-xxx"},
		{"sk-very-long-api-key-value", "sk-v****alue"},
	}

	for _, test := range tests {
		result := maskKey(test.input)
		if result != test.expected {
			t.Errorf("maskKey(%q) = %q，期望 %q", test.input, result, test.expected)
		}
	}
}
