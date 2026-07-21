package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// 流式写回辅助（自旧 StreamingHandler 循环壳保留；循环壳已随 v7 管线重构退役）

// trackedStreamingResponseWriter 追踪是否已向客户端写出内容的流式 writer，
// 底层不支持 Flusher 时自动退化为 no-op flush。
type trackedStreamingResponseWriter struct {
	http.ResponseWriter
	committed bool
}

func newTrackedStreamingResponseWriter(w http.ResponseWriter) *trackedStreamingResponseWriter {
	return &trackedStreamingResponseWriter{ResponseWriter: w}
}

func (w *trackedStreamingResponseWriter) WriteHeader(statusCode int) {
	w.committed = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *trackedStreamingResponseWriter) Write(data []byte) (int, error) {
	if len(data) > 0 {
		w.committed = true
	}
	return w.ResponseWriter.Write(data)
}

func (w *trackedStreamingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *trackedStreamingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *trackedStreamingResponseWriter) HasCommitted() bool {
	return w != nil && w.committed
}

func writeStreamingTerminalError(w http.ResponseWriter, flusher http.Flusher, statusCode int, message string) {
	if tracked, ok := w.(*trackedStreamingResponseWriter); !ok || !tracked.HasCommitted() {
		w.WriteHeader(statusCode)
	}
	fmt.Fprintf(w, "data: error: %s\n\n", message)
	flusher.Flush()
}

// SendAnthropicRetryableError 发送 Anthropic API 标准格式的可重试错误事件
// 使用 overloaded_error 类型，Claude Code 等客户端会识别并自动重试
func SendAnthropicRetryableError(w http.ResponseWriter, flusher http.Flusher, message string) {
	fmt.Fprintf(w, "event: error\n")
	fmt.Fprintf(w, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"%s\"}}\n\n",
		escapeJSONString(message))
	flusher.Flush()
}

// sendStreamInterruptedMessage 发送流中断消息，触发客户端自动重试
// 模拟上游代理的行为：当 EOF 发生时，发送一个完整的新消息（第二个 message_start）
// Claude Code 会识别这种模式并自动重试请求
// 参考: req-fd3a8dd9.debug 中观察到的上游代理行为
func sendStreamInterruptedMessage(w http.ResponseWriter, flusher http.Flusher, message string, modelName string) {
	// 使用 UUID 作为 message id（模拟上游代理的行为，与正常的 msg_ 前缀不同）
	msgID := uuid.New().String()

	// 1. message_start - 新消息开始（使用 Anthropic 格式的 usage 字段）
	fmt.Fprintf(w, "event: message_start\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"%s\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"%s\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n",
		msgID, escapeJSONString(modelName))
	flusher.Flush()

	// 2. content_block_start - 开始 text 类型的内容块
	fmt.Fprintf(w, "event: content_block_start\n")
	fmt.Fprintf(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	flusher.Flush()

	// 3. content_block_delta - 发送错误信息内容
	fmt.Fprintf(w, "event: content_block_delta\n")
	fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"%s\"}}\n\n",
		escapeJSONString(message))
	flusher.Flush()

	// 4. content_block_stop - 内容块结束
	fmt.Fprintf(w, "event: content_block_stop\n")
	fmt.Fprintf(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	flusher.Flush()

	// 5. message_delta - 消息增量更新（包含 stop_reason 和 usage，让客户端认为流完整）
	fmt.Fprintf(w, "event: message_delta\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":0}}\n\n")
	flusher.Flush()

	// 6. message_stop - 消息结束（关键！让客户端识别为完整消息从而触发重试）
	fmt.Fprintf(w, "event: message_stop\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
	flusher.Flush()
}

// escapeJSONString 对字符串进行 JSON 转义
func escapeJSONString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
