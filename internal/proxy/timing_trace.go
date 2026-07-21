package proxy

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// upstreamTraceState 记录单次上游转发的连接级事件。
// 以 WroteHeaders 是否触发作为「重放安全 / 歧义」分界：
// WroteHeaders 前失败 = 请求确定未发出（DNS / refused / dial 超时 / TLS 握手失败），
// 可安全换候选重放；触发后的任何失败均视为歧义，不得据此重放。
type upstreamTraceState struct {
	mu               sync.Mutex
	gotConn          bool
	tlsHandshakeDone bool
	wroteHeaders     bool
	wroteRequest     bool
	wroteRequestErr  error
}

func (s *upstreamTraceState) markGotConn() {
	s.mu.Lock()
	s.gotConn = true
	s.mu.Unlock()
}

func (s *upstreamTraceState) markTLSHandshakeDone() {
	s.mu.Lock()
	s.tlsHandshakeDone = true
	s.mu.Unlock()
}

func (s *upstreamTraceState) markWroteHeaders() {
	s.mu.Lock()
	s.wroteHeaders = true
	s.mu.Unlock()
}

func (s *upstreamTraceState) markWroteRequest(err error) {
	s.mu.Lock()
	s.wroteRequest = true
	s.wroteRequestErr = err
	s.mu.Unlock()
}

// WroteHeaders 返回请求头是否已写向上游。
func (s *upstreamTraceState) WroteHeaders() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wroteHeaders
}

// WroteRequestErr 返回请求体写出阶段记录的错误（未写出或成功时为 nil）。
func (s *upstreamTraceState) WroteRequestErr() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wroteRequestErr
}

// withUpstreamTrace 在 ctx 上挂载 GotConn / TLSHandshakeDone / WroteHeaders /
// WroteRequest 四事件 trace，返回记录事件的状态结构。
// onWroteRequest 保留首字计时用途（可为 nil）。
func withUpstreamTrace(ctx context.Context, onWroteRequest func(time.Time)) (context.Context, *upstreamTraceState) {
	state := &upstreamTraceState{}
	trace := &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) {
			state.markGotConn()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			state.markTLSHandshakeDone()
		},
		WroteHeaders: func() {
			state.markWroteHeaders()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			state.markWroteRequest(info.Err)
			if onWroteRequest != nil {
				onWroteRequest(time.Now())
			}
		},
	}
	return httptrace.WithClientTrace(ctx, trace), state
}
