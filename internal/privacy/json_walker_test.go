package privacy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func mustCollect(t *testing.T, body string, requestPath string) []textSegment {
	t.Helper()
	matcher := walkerForPath(requestPath)
	if matcher == nil {
		t.Fatalf("no walker for path %s", requestPath)
	}
	segments, err := collectJSONTextSegments([]byte(body), matcher)
	if err != nil {
		t.Fatalf("collect segments failed: %v", err)
	}
	return segments
}

func segmentValues(segments []textSegment) []string {
	values := make([]string, 0, len(segments))
	for _, seg := range segments {
		values = append(values, seg.Value)
	}
	return values
}

func TestClaudeWalkerCollectsExpectedFields(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4",
		"system": "system text",
		"messages": [
			{"role": "user", "content": "plain content"},
			{"role": "user", "content": [
				{"type": "text", "text": "block text"},
				{"type": "tool_result", "tool_use_id": "tu_1", "content": "tool result string"},
				{"type": "tool_result", "tool_use_id": "tu_2", "content": [
					{"type": "text", "text": "nested tool text"}
				]}
			]}
		],
		"tools": [{"name": "secret_tool", "description": "tool desc"}],
		"metadata": {"user_id": "meta-user"},
		"stream": true
	}`

	segments := mustCollect(t, body, "/v1/messages")
	got := segmentValues(segments)
	want := []string{"system text", "plain content", "block text", "tool result string", "nested tool text"}
	if len(got) != len(want) {
		t.Fatalf("segment count = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestClaudeWalkerSystemArray(t *testing.T) {
	body := `{"system": [{"type": "text", "text": "sys block"}], "messages": []}`
	segments := mustCollect(t, body, "/v1/messages")
	got := segmentValues(segments)
	if len(got) != 1 || got[0] != "sys block" {
		t.Fatalf("got %v, want [sys block]", got)
	}
}

func TestClaudeWalkerDoesNotScanToolsOrMetadata(t *testing.T) {
	body := `{
		"model": "m",
		"tools": [{"name": "t", "description": "secret-in-tools", "input_schema": {"type": "object"}}],
		"tool_choice": {"type": "tool", "name": "secret-choice"},
		"metadata": {"user_id": "secret-meta"},
		"messages": [{"role": "user", "content": [
			{"type": "tool_use", "id": "x", "name": "n", "input": {"text": "secret-in-tool-use-input"}}
		]}]
	}`
	segments := mustCollect(t, body, "/v1/messages")
	if len(segments) != 0 {
		t.Fatalf("expected no segments, got %v", segmentValues(segments))
	}
}

func TestCodexWalkerCollectsExpectedFields(t *testing.T) {
	body := `{
		"model": "gpt-5.4",
		"instructions": "instr text",
		"input": [
			{"type": "message", "role": "user", "content": [
				{"type": "input_text", "text": "input text block"}
			]},
			{"type": "message", "role": "user", "content": "string content"},
			{"type": "function_call_output", "call_id": "c1", "output": "tool output text"}
		],
		"tools": [{"type": "function", "name": "fn", "description": "tool-desc"}],
		"reasoning": {"effort": "medium"},
		"stream": true
	}`
	segments := mustCollect(t, body, "/v1/responses")
	got := segmentValues(segments)
	want := []string{"instr text", "input text block", "string content", "tool output text"}
	if len(got) != len(want) {
		t.Fatalf("segment count = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodexWalkerInputString(t *testing.T) {
	body := `{"model": "gpt-5.4", "input": "raw input string"}`
	segments := mustCollect(t, body, "/v1/responses/compact")
	got := segmentValues(segments)
	if len(got) != 1 || got[0] != "raw input string" {
		t.Fatalf("got %v, want [raw input string]", got)
	}
}

func TestReplaceSegmentsKeepsUntouchedBytesIdentical(t *testing.T) {
	body := `{"model":"m-1.5",  "big": 9007199254740993, "messages":[{"role":"user","content":"secret"}], "stream":false, "temp": 0.30}`
	segments := mustCollect(t, body, "/v1/messages")
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	out := replaceSegments([]byte(body), segments, map[int]string{0: "[已脱敏]"})

	want := strings.Replace(body, `"secret"`, `"[已脱敏]"`, 1)
	if string(out) != want {
		t.Fatalf("replaced body mismatch:\n got: %s\nwant: %s", out, want)
	}
	// 大整数与数字格式不被改写
	if !bytes.Contains(out, []byte("9007199254740993")) {
		t.Error("big integer was rewritten")
	}
	if !bytes.Contains(out, []byte("0.30")) {
		t.Error("number format was rewritten")
	}
}

func TestReplaceSegmentsNoHTMLEscape(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"keep <script> & a&b secret-token here"}]}`
	segments := mustCollect(t, body, "/v1/messages")
	out := replaceSegments([]byte(body), segments, map[int]string{0: "keep <script> & a&b [已脱敏] here"})
	if !bytes.Contains(out, []byte("<script>")) {
		t.Errorf("'<script>' was HTML-escaped: %s", out)
	}
	if !bytes.Contains(out, []byte("a&b")) {
		t.Errorf("'a&b' was HTML-escaped: %s", out)
	}
	if bytes.Contains(out, []byte("\\u003c")) || bytes.Contains(out, []byte("\\u0026")) {
		t.Errorf("unexpected HTML escaping in output: %s", out)
	}
}

func TestReplaceSegmentsEscapesSpecialCharacters(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"x"}]}`
	segments := mustCollect(t, body, "/v1/messages")
	out := replaceSegments([]byte(body), segments, map[int]string{0: "line1\nline2\t\"quoted\" \\slash 中文"})

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("replaced body is invalid json: %v\n%s", err, out)
	}
	messages := decoded["messages"].([]any)
	content := messages[0].(map[string]any)["content"].(string)
	if content != "line1\nline2\t\"quoted\" \\slash 中文" {
		t.Errorf("roundtrip mismatch: %q", content)
	}
}

func TestWalkerHandlesEscapedStringsOffsets(t *testing.T) {
	// 命中段自身含转义字符时，span 定位必须覆盖完整 token
	body := `{"system": "pre \"mid\" post", "messages":[{"role":"user","content":"a\\b secret"}]}`
	segments := mustCollect(t, body, "/v1/messages")
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if segments[0].Value != `pre "mid" post` {
		t.Errorf("decoded value mismatch: %q", segments[0].Value)
	}
	out := replaceSegments([]byte(body), segments, map[int]string{1: `a\b [已脱敏]`})
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("replaced body is invalid json: %v\n%s", err, out)
	}
	content := decoded["messages"].([]any)[0].(map[string]any)["content"].(string)
	if content != `a\b [已脱敏]` {
		t.Errorf("content = %q", content)
	}
	if system := decoded["system"].(string); system != `pre "mid" post` {
		t.Errorf("system mutated: %q", system)
	}
}

func TestWalkerInvalidJSONReturnsError(t *testing.T) {
	matcher := walkerForPath("/v1/messages")
	if _, err := collectJSONTextSegments([]byte(`{"messages": [`), matcher); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestWalkerForUnsupportedPath(t *testing.T) {
	if walkerForPath("/v1/models") != nil {
		t.Fatal("unexpected walker for unsupported path")
	}
}

func TestCountTokensReusesClaudeWalker(t *testing.T) {
	body := `{"model":"m","messages":[{"role":"user","content":"count me"}]}`
	segments := mustCollect(t, body, "/v1/messages/count_tokens")
	got := segmentValues(segments)
	if len(got) != 1 || got[0] != "count me" {
		t.Fatalf("got %v, want [count me]", got)
	}
}
