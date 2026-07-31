package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

func TestIsCountTokensUnsupportedRequiresCountTokensContextForTextMatches(t *testing.T) {
	if !isCountTokensUnsupported(http.StatusNotFound, `{"error":"missing route"}`) {
		t.Fatal("expected 404 to mark count_tokens as unsupported")
	}
	if !isCountTokensUnsupported(http.StatusBadRequest, `{"error":"count_tokens endpoint is not supported"}`) {
		t.Fatal("expected explicit count_tokens not supported message to be marked unsupported")
	}
	if isCountTokensUnsupported(http.StatusBadRequest, `{"error":"model unsupported"}`) {
		t.Fatal("model unsupported should not mark count_tokens as unsupported")
	}
	if isCountTokensUnsupported(http.StatusUnprocessableEntity, `{"error":"parameter temperature is not supported"}`) {
		t.Fatal("generic parameter errors should not mark count_tokens as unsupported")
	}
}

func TestCountTokensTryForwardRewritesEndpointModel(t *testing.T) {
	var receivedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream body failed: %v", err)
		}
		receivedModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":12}`))
	}))
	defer server.Close()

	cfg := &config.Config{}
	manager := endpoint.NewManager(cfg)
	defer manager.Stop()
	forwarder := NewForwarder(cfg, manager)
	handler := NewCountTokensHandler(cfg, manager, forwarder)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{
		Name:                "count-rewrite",
		URL:                 server.URL,
		Timeout:             2 * time.Second,
		SupportsCountTokens: true,
		ModelRewriteRules:   `[{"paths":["/v1/messages/count_tokens"],"match":"exact","from":"claude-sonnet-4-5","to":"provider-sonnet"}]`,
	}}
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))

	result, ok, err := handler.tryForward(context.Background(), req, body, []*endpoint.Endpoint{ep}, "req-count-rewrite")
	if err != nil || !ok {
		t.Fatalf("count_tokens forward failed: ok=%v err=%v body=%s", ok, err, string(result))
	}
	if receivedModel != "provider-sonnet" {
		t.Fatalf("expected rewritten count_tokens model, got %q", receivedModel)
	}
}
