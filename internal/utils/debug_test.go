package utils

import "testing"

func TestExtractUsageFromDebugContent_ClaudeMessageStopFormat(t *testing.T) {
	content := `
=== 流式TOKEN解析失败调试信息 ===
[行1] event: message_stop
[行2] data: {"type":"message_stop","usage":{"input_tokens":123,"cache_creation_input_tokens":45,"cache_read_input_tokens":67,"output_tokens":89}}
`

	usage, err := extractUsageFromDebugContent(content)
	if err != nil {
		t.Fatalf("extractUsageFromDebugContent returned error: %v", err)
	}
	if usage.InputTokens != 123 || usage.OutputTokens != 89 || usage.CacheCreationTokens != 45 || usage.CacheReadTokens != 67 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestExtractUsageFromDebugContent_CodexResponsesFormat(t *testing.T) {
	content := `
=== 流式TOKEN解析失败调试信息 ===
[行1] event: response.completed
[行2] data: {"type":"response.completed","response":{"model":"gpt-5-codex"},"usage":{"input_tokens":85388,"input_tokens_details":{"cached_tokens":85248},"output_tokens":1929,"output_tokens_details":{"reasoning_tokens":892},"total_tokens":87317}}
`

	usage, err := extractUsageFromDebugContent(content)
	if err != nil {
		t.Fatalf("extractUsageFromDebugContent returned error: %v", err)
	}
	if usage.InputTokens != 85388 || usage.OutputTokens != 1929 || usage.CacheCreationTokens != 0 || usage.CacheReadTokens != 85248 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}
