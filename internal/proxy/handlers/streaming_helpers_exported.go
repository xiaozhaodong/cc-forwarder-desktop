package handlers

import "net/http"

// WriteStreamingTerminalError 导出：向流式客户端写终止错误（管线复用）
func WriteStreamingTerminalError(w http.ResponseWriter, flusher http.Flusher, statusCode int, message string) {
	writeStreamingTerminalError(w, flusher, statusCode, message)
}

// SendStreamInterruptedMessage 导出：发送流中断消息（EOFRetryHint，触发客户端自动重试）
func SendStreamInterruptedMessage(w http.ResponseWriter, flusher http.Flusher, message string, modelName string) {
	sendStreamInterruptedMessage(w, flusher, message, modelName)
}

// NewStreamingResponseWriter 导出：包装 ResponseWriter 为流式安全形态（管线复用）。
// 返回包装后的 writer、永远可用的 flusher（底层不支持时退化 no-op）与
// hasCommitted 探针（是否已向客户端写出任何内容）。
func NewStreamingResponseWriter(w http.ResponseWriter) (http.ResponseWriter, http.Flusher, func() bool) {
	tracked := newTrackedStreamingResponseWriter(w)
	return tracked, tracked, tracked.HasCommitted
}
