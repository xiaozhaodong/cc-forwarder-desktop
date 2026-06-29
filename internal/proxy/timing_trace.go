package proxy

import (
	"context"
	"net/http/httptrace"
	"time"
)

func withWroteRequestTrace(ctx context.Context, onWroteRequest func(time.Time)) context.Context {
	if onWroteRequest == nil {
		return ctx
	}
	trace := &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) {
			onWroteRequest(time.Now())
		},
	}
	return httptrace.WithClientTrace(ctx, trace)
}
