package proxy

import (
	"cc-forwarder/internal/monitor"
	"testing"
)

func TestTokenParser(t *testing.T) {
	parser := NewTokenParser()

	// Test parsing Claude API message_delta event with usage
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":5,\"cache_creation_input_tokens\":494,\"cache_read_input_tokens\":110689,\"output_tokens\":582}}",
		"",
	}

	var result *monitor.TokenUsage
	for _, line := range lines {
		if tokens := parser.ParseSSELine(line); tokens != nil {
			result = tokens
		}
	}

	if result == nil {
		t.Fatal("Expected to parse token usage, got nil")
	}

	// Check the values
	if result.InputTokens != 5 {
		t.Errorf("Expected InputTokens=5, got %d", result.InputTokens)
	}
	if result.OutputTokens != 582 {
		t.Errorf("Expected OutputTokens=582, got %d", result.OutputTokens)
	}
	if result.CacheCreationTokens != 494 {
		t.Errorf("Expected CacheCreationTokens=494, got %d", result.CacheCreationTokens)
	}
	if result.CacheReadTokens != 110689 {
		t.Errorf("Expected CacheReadTokens=110689, got %d", result.CacheReadTokens)
	}
}

func TestTokenParserNonUsageEvent(t *testing.T) {
	parser := NewTokenParser()

	// Test parsing non-usage message_delta event
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"Hello world\"}}",
		"",
	}

	var result *monitor.TokenUsage
	for _, line := range lines {
		if tokens := parser.ParseSSELine(line); tokens != nil {
			result = tokens
		}
	}

	if result != nil {
		t.Error("Expected nil for message_delta without usage, got result")
	}
}

func TestTokenParserOtherEvents(t *testing.T) {
	parser := NewTokenParser()

	// Test parsing non-message_delta events
	lines := []string{
		"event: ping",
		"data: {\"type\":\"ping\"}",
		"",
	}

	var result *monitor.TokenUsage
	for _, line := range lines {
		if tokens := parser.ParseSSELine(line); tokens != nil {
			result = tokens
		}
	}

	if result != nil {
		t.Error("Expected nil for non-message_delta events, got result")
	}
}

// ===== V2 职责纯化测试 =====

func TestTokenParserV2_MessageDeltaWithUsage(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-123")

	// Test parsing Claude API message_delta event with usage using V2 method
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":5,\"cache_creation_input_tokens\":494,\"cache_read_input_tokens\":110689,\"output_tokens\":582}}",
		"",
	}

	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}

	if result == nil {
		t.Fatal("Expected to parse token usage with V2 method, got nil")
	}

	// Check the ParseResult structure
	if result.TokenUsage == nil {
		t.Fatal("Expected TokenUsage in ParseResult, got nil")
	}

	if result.TokenUsage.InputTokens != 5 {
		t.Errorf("Expected InputTokens=5, got %d", result.TokenUsage.InputTokens)
	}
	if result.TokenUsage.OutputTokens != 582 {
		t.Errorf("Expected OutputTokens=582, got %d", result.TokenUsage.OutputTokens)
	}
	if result.TokenUsage.CacheCreationTokens != 494 {
		t.Errorf("Expected CacheCreationTokens=494, got %d", result.TokenUsage.CacheCreationTokens)
	}
	if result.TokenUsage.CacheReadTokens != 110689 {
		t.Errorf("Expected CacheReadTokens=110689, got %d", result.TokenUsage.CacheReadTokens)
	}

	if !result.IsCompleted {
		t.Error("Expected IsCompleted=true for message_delta with usage")
	}

	if result.Status != "completed" {
		t.Errorf("Expected Status=completed, got %s", result.Status)
	}
}

func TestTokenParserV2_MessageDeltaWithoutUsage(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-456")

	// Test parsing non-usage message_delta event using V2 method
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"Hello world\"}}",
		"",
	}

	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}

	if result == nil {
		t.Fatal("Expected to get ParseResult for non-usage message_delta, got nil")
	}

	// Check the ParseResult for non-token response
	if result.TokenUsage == nil {
		t.Fatal("Expected empty TokenUsage in ParseResult, got nil")
	}

	// Should have empty token usage
	if result.TokenUsage.InputTokens != 0 {
		t.Errorf("Expected InputTokens=0 for non-usage, got %d", result.TokenUsage.InputTokens)
	}

	if !result.IsCompleted {
		t.Error("Expected IsCompleted=true for non-usage message_delta")
	}

	if result.Status != "non_token_response" {
		t.Errorf("Expected Status=non_token_response, got %s", result.Status)
	}

	if result.ModelName != "default" {
		t.Errorf("Expected ModelName=default, got %s", result.ModelName)
	}
}

func TestTokenParserV2_MessageDeltaWithoutUsage_ReportedOnce(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-456-repeat")

	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"Hello\"}}",
		"",
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"World\"}}",
		"",
	}

	resultCount := 0
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			resultCount++
		}
	}

	if resultCount != 1 {
		t.Errorf("Expected non-usage ParseResult to be reported once, got %d", resultCount)
	}
}

func TestTokenParserV2_MessageDeltaWithoutUsage_AfterMessageStartIgnored(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-message-start")

	lines := []string{
		"event: message_start",
		"data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-6\",\"usage\":{\"input_tokens\":1,\"output_tokens\":8,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0}}}",
		"",
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"hello\"}}",
		"",
	}

	resultCount := 0
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			resultCount++
		}
	}

	if resultCount != 0 {
		t.Errorf("Expected no ParseResult for non-usage delta after message_start usage, got %d", resultCount)
	}

	finalUsage := parser.GetFinalUsage()
	if finalUsage == nil {
		t.Fatal("Expected fallback final usage from message_start, got nil")
	}
	if finalUsage.InputTokens != 1 || finalUsage.OutputTokens != 8 {
		t.Errorf("Expected fallback usage input=1 output=8, got input=%d output=%d", finalUsage.InputTokens, finalUsage.OutputTokens)
	}
}

func TestTokenParserV2_ErrorEvent(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-error")

	// Test parsing error event using V2 method
	lines := []string{
		"event: error",
		"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server is overloaded\"}}",
		"",
	}

	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}

	if result == nil {
		t.Fatal("Expected to get ParseResult for error event, got nil")
	}

	// Check error handling
	if result.ErrorInfo == nil {
		t.Fatal("Expected ErrorInfo in ParseResult, got nil")
	}

	if result.ErrorInfo.Type != "overloaded_error" {
		t.Errorf("Expected ErrorInfo.Type=overloaded_error, got %s", result.ErrorInfo.Type)
	}

	if result.ErrorInfo.Message != "Server is overloaded" {
		t.Errorf("Expected ErrorInfo.Message=Server is overloaded, got %s", result.ErrorInfo.Message)
	}

	if !result.IsCompleted {
		t.Error("Expected IsCompleted=true for error event")
	}

	if result.Status != StatusErrorAPI {
		t.Errorf("Expected Status=%s, got %s", StatusErrorAPI, result.Status)
	}

	if result.ModelName != "error:overloaded_error" {
		t.Errorf("Expected ModelName=error:overloaded_error, got %s", result.ModelName)
	}
}

// ===== v5.0.1+: Cache Creation 5m/1h 分开解析测试 =====

// TestTokenParserV2_CacheCreation1hTokens 测试 1 小时缓存 token 解析
func TestTokenParserV2_CacheCreation1hTokens(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-cache-1h")

	// 模拟真实 Claude API 响应，包含 cache_creation 嵌套对象
	lines := []string{
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5000,"output_tokens":800,"cache_creation_input_tokens":2000,"cache_read_input_tokens":10000,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":2000}}}`,
		"",
	}

	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}

	if result == nil {
		t.Fatal("Expected to parse token usage with cache_creation, got nil")
	}

	if result.TokenUsage == nil {
		t.Fatal("Expected TokenUsage in ParseResult, got nil")
	}

	// 验证基础 token 计数
	if result.TokenUsage.InputTokens != 5000 {
		t.Errorf("Expected InputTokens=5000, got %d", result.TokenUsage.InputTokens)
	}
	if result.TokenUsage.OutputTokens != 800 {
		t.Errorf("Expected OutputTokens=800, got %d", result.TokenUsage.OutputTokens)
	}

	// 验证缓存 token 计数
	if result.TokenUsage.CacheCreationTokens != 2000 {
		t.Errorf("Expected CacheCreationTokens=2000, got %d", result.TokenUsage.CacheCreationTokens)
	}
	if result.TokenUsage.CacheReadTokens != 10000 {
		t.Errorf("Expected CacheReadTokens=10000, got %d", result.TokenUsage.CacheReadTokens)
	}

	// v5.0.1+: 验证分开的 5m/1h 缓存 tokens
	if result.TokenUsage.CacheCreation5mTokens != 0 {
		t.Errorf("Expected CacheCreation5mTokens=0, got %d", result.TokenUsage.CacheCreation5mTokens)
	}
	if result.TokenUsage.CacheCreation1hTokens != 2000 {
		t.Errorf("Expected CacheCreation1hTokens=2000, got %d", result.TokenUsage.CacheCreation1hTokens)
	}

	t.Logf("✅ 1h cache parsing successful: 5m=%d, 1h=%d, total=%d",
		result.TokenUsage.CacheCreation5mTokens,
		result.TokenUsage.CacheCreation1hTokens,
		result.TokenUsage.CacheCreationTokens)
}

// TestTokenParserV2_CacheCreationMixed 测试混合 5m 和 1h 缓存
func TestTokenParserV2_CacheCreationMixed(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-cache-mixed")

	// 模拟同时包含 5m 和 1h 缓存的响应
	lines := []string{
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10000,"output_tokens":1500,"cache_creation_input_tokens":8000,"cache_read_input_tokens":50000,"cache_creation":{"ephemeral_5m_input_tokens":3000,"ephemeral_1h_input_tokens":5000}}}`,
		"",
	}

	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}

	if result == nil || result.TokenUsage == nil {
		t.Fatal("Expected to parse token usage, got nil")
	}

	// 验证分开的缓存 tokens
	if result.TokenUsage.CacheCreation5mTokens != 3000 {
		t.Errorf("Expected CacheCreation5mTokens=3000, got %d", result.TokenUsage.CacheCreation5mTokens)
	}
	if result.TokenUsage.CacheCreation1hTokens != 5000 {
		t.Errorf("Expected CacheCreation1hTokens=5000, got %d", result.TokenUsage.CacheCreation1hTokens)
	}

	// 验证总数（向后兼容）
	if result.TokenUsage.CacheCreationTokens != 8000 {
		t.Errorf("Expected CacheCreationTokens=8000, got %d", result.TokenUsage.CacheCreationTokens)
	}

	t.Logf("✅ Mixed cache parsing successful: 5m=%d, 1h=%d, total=%d",
		result.TokenUsage.CacheCreation5mTokens,
		result.TokenUsage.CacheCreation1hTokens,
		result.TokenUsage.CacheCreationTokens)
}

// TestTokenParserV2_BackwardCompatibleNoCacheCreationObject 测试向后兼容（无 cache_creation 对象）
func TestTokenParserV2_BackwardCompatibleNoCacheCreationObject(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-cache-compat")

	// 模拟旧版 API 响应，没有 cache_creation 嵌套对象
	lines := []string{
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":200,"cache_read_input_tokens":5000}}`,
		"",
	}

	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}

	if result == nil || result.TokenUsage == nil {
		t.Fatal("Expected to parse token usage, got nil")
	}

	// 验证基础 token 计数仍然正确
	if result.TokenUsage.CacheCreationTokens != 200 {
		t.Errorf("Expected CacheCreationTokens=200, got %d", result.TokenUsage.CacheCreationTokens)
	}

	// 无 cache_creation 对象时，5m/1h 应为 0
	if result.TokenUsage.CacheCreation5mTokens != 0 {
		t.Errorf("Expected CacheCreation5mTokens=0 for backward compatible, got %d", result.TokenUsage.CacheCreation5mTokens)
	}
	if result.TokenUsage.CacheCreation1hTokens != 0 {
		t.Errorf("Expected CacheCreation1hTokens=0 for backward compatible, got %d", result.TokenUsage.CacheCreation1hTokens)
	}

	t.Log("✅ Backward compatibility test passed: old API format still works")
}

// TestTokenParserV2_GetFinalUsageWithCacheCreation 测试 GetFinalUsage 包含 5m/1h 缓存
func TestTokenParserV2_GetFinalUsageWithCacheCreation(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-final-usage")

	// 先解析 message_start 获取初始 usage
	startLines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg-123","type":"message","role":"assistant","model":"claude-opus-4-5-20251101","usage":{"input_tokens":5000,"output_tokens":0,"cache_creation_input_tokens":2000,"cache_read_input_tokens":10000,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":2000}}}}`,
		"",
	}

	for _, line := range startLines {
		parser.ParseSSELineV2(line)
	}

	// 再解析 message_delta 获取最终 usage
	deltaLines := []string{
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5000,"output_tokens":800,"cache_creation_input_tokens":2000,"cache_read_input_tokens":10000,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":2000}}}`,
		"",
	}

	for _, line := range deltaLines {
		parser.ParseSSELineV2(line)
	}

	// 获取最终 usage
	finalUsage := parser.GetFinalUsage()
	if finalUsage == nil {
		t.Fatal("Expected GetFinalUsage to return non-nil")
	}

	// 验证最终 usage 包含正确的 5m/1h 缓存信息
	if finalUsage.CacheCreation1hTokens != 2000 {
		t.Errorf("GetFinalUsage: Expected CacheCreation1hTokens=2000, got %d", finalUsage.CacheCreation1hTokens)
	}
	if finalUsage.CacheCreation5mTokens != 0 {
		t.Errorf("GetFinalUsage: Expected CacheCreation5mTokens=0, got %d", finalUsage.CacheCreation5mTokens)
	}

	t.Logf("✅ GetFinalUsage with cache_creation: input=%d, output=%d, cache_5m=%d, cache_1h=%d, cache_read=%d",
		finalUsage.InputTokens, finalUsage.OutputTokens,
		finalUsage.CacheCreation5mTokens, finalUsage.CacheCreation1hTokens,
		finalUsage.CacheReadTokens)
}
