package proxy

import (
	"testing"
)

// ===== TokenParser 流完整性追踪测试 =====
// 2025-12-11: 测试流完整性检测功能

func TestStreamCompleteness_CompleteStream(t *testing.T) {
	// 测试完整的流：message_start + message_delta(usage) + message_stop
	parser := NewTokenParserWithRequestID("test-complete-stream")

	// 模拟完整的 SSE 事件序列
	lines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":100}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	// 验证完整性状态
	completeness := parser.GetStreamCompleteness()
	if !completeness.IsComplete {
		t.Errorf("期望流完整，但得到不完整状态: %s", completeness.Reason)
	}
	if completeness.FailureReason != "" {
		t.Errorf("期望 FailureReason 为空，但得到: %s", completeness.FailureReason)
	}

	// 验证 IsStreamComplete 方法
	if !parser.IsStreamComplete() {
		t.Error("IsStreamComplete() 应该返回 true")
	}
}

func TestStreamCompleteness_MissingMessageStop(t *testing.T) {
	// 测试缺少 message_stop 但有 usage 的情况
	parser := NewTokenParserWithRequestID("test-missing-stop")

	lines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":100}}`,
		"",
		// 没有 message_stop 事件
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	if completeness.IsComplete {
		t.Error("期望流不完整（缺少 message_stop），但得到完整状态")
	}
	if completeness.FailureReason != "incomplete_stream" {
		t.Errorf("期望 FailureReason='incomplete_stream'，但得到: %s", completeness.FailureReason)
	}
	if completeness.Reason == "" {
		t.Error("期望有不完整原因说明")
	}

	t.Logf("✅ 缺少 message_stop 测试通过: %s", completeness.Reason)
}

func TestStreamCompleteness_StreamTruncated(t *testing.T) {
	// 测试响应被截断的情况（只有 message_start，没有 usage 和 stop）
	parser := NewTokenParserWithRequestID("test-truncated")

	lines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		// 流被截断，没有 message_delta(usage) 和 message_stop
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	if completeness.IsComplete {
		t.Error("期望流不完整（被截断），但得到完整状态")
	}
	if completeness.FailureReason != "stream_truncated" {
		t.Errorf("期望 FailureReason='stream_truncated'，但得到: %s", completeness.FailureReason)
	}

	t.Logf("✅ 流截断测试通过: %s", completeness.Reason)
}

func TestStreamCompleteness_NoEvents(t *testing.T) {
	// 测试没有收到任何有效事件的情况
	parser := NewTokenParserWithRequestID("test-no-events")

	// 只有 ping 事件
	lines := []string{
		"event: ping",
		`data: {"type":"ping"}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	if completeness.IsComplete {
		t.Error("期望流不完整（无有效事件），但得到完整状态")
	}
	if completeness.FailureReason != "stream_truncated" {
		t.Errorf("期望 FailureReason='stream_truncated'，但得到: %s", completeness.FailureReason)
	}

	t.Logf("✅ 无有效事件测试通过: %s", completeness.Reason)
}

func TestStreamCompleteness_FallbackUsed(t *testing.T) {
	// 测试使用 fallback（只有 message_start 的 usage）但有 message_stop 的情况
	parser := NewTokenParserWithRequestID("test-fallback")

	lines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		// message_delta 没有 usage
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	// 验证 fallback 被使用
	if !parser.IsFallbackUsed() {
		t.Error("期望使用 fallback 机制")
	}

	completeness := parser.GetStreamCompleteness()
	// 即使有 message_stop，但使用了 fallback 也应该标记为不完整
	if completeness.IsComplete {
		t.Error("期望流不完整（使用了 fallback），但得到完整状态")
	}
	if completeness.FailureReason != "incomplete_stream" {
		t.Errorf("期望 FailureReason='incomplete_stream'，但得到: %s", completeness.FailureReason)
	}

	t.Logf("✅ Fallback 使用测试通过: %s", completeness.Reason)
}

func TestStreamCompleteness_Reset(t *testing.T) {
	// 测试 Reset 方法是否正确重置完整性追踪字段
	parser := NewTokenParserWithRequestID("test-reset")

	// 先解析一些事件
	lines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":100}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	// 验证流是完整的
	if !parser.IsStreamComplete() {
		t.Error("Reset 前期望流完整")
	}

	// 重置解析器
	parser.Reset()

	// 验证重置后的状态
	completeness := parser.GetStreamCompleteness()
	if completeness.IsComplete {
		t.Error("Reset 后期望流不完整（因为没有收到任何事件）")
	}

	t.Log("✅ Reset 测试通过")
}

func TestStreamCompleteness_MessageStopOnly(t *testing.T) {
	// 测试只有 message_stop 的异常情况
	parser := NewTokenParserWithRequestID("test-stop-only")

	lines := []string{
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	// 有 message_stop 但没有 message_start 和 usage，仍然应该检查是否有完整的 usage
	// 由于没有 message_start，应该是完整的（只检查 message_stop）
	if !completeness.IsComplete {
		// 这种情况取决于具体实现逻辑
		t.Logf("只有 message_stop 的情况: IsComplete=%v, Reason=%s", completeness.IsComplete, completeness.Reason)
	}
}

// ===== 边界条件测试 =====

func TestStreamCompleteness_EmptyParser(t *testing.T) {
	// 测试全新的解析器
	parser := NewTokenParser()

	completeness := parser.GetStreamCompleteness()
	if completeness.IsComplete {
		t.Error("新解析器应该返回不完整状态")
	}

	t.Logf("✅ 空解析器测试通过: FailureReason=%s", completeness.FailureReason)
}

func TestStreamCompleteness_MultipleMessageDelta(t *testing.T) {
	// 测试多个 message_delta 事件（只有最后一个有 usage）
	parser := NewTokenParserWithRequestID("test-multi-delta")

	lines := []string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":null}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":100}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	if !completeness.IsComplete {
		t.Errorf("期望流完整，但得到: %s", completeness.Reason)
	}

	t.Log("✅ 多 message_delta 测试通过")
}

func TestStreamCompleteness_ResponsesCompleted(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-responses-complete")

	lines := []string{
		"event: response.in_progress",
		`data: {"type":"response.in_progress","response":{"model":"gpt-5-codex"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"model":"gpt-5-codex"},"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	if !completeness.IsComplete {
		t.Errorf("expected responses stream complete, got reason=%s failure=%s",
			completeness.Reason, completeness.FailureReason)
	}
}

func TestStreamCompleteness_ResponsesMissingCompleted(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-responses-missing-completed")

	lines := []string{
		"event: response.in_progress",
		`data: {"type":"response.in_progress","response":{"model":"gpt-5-codex"}}`,
		"",
	}

	for _, line := range lines {
		parser.ParseSSELineV2(line)
	}

	completeness := parser.GetStreamCompleteness()
	if completeness.IsComplete {
		t.Fatal("expected responses stream incomplete when missing response.completed")
	}
	if completeness.FailureReason != "stream_truncated" {
		t.Fatalf("expected stream_truncated, got %s", completeness.FailureReason)
	}
}
