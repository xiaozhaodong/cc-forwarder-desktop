package service

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

func TestTestUpstreamAccount_UsesResponsesEndpointAndTreats400AsReachable(t *testing.T) {
	var receivedMethod string
	var receivedPath string
	var receivedAuth string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		receivedBody = string(raw)

		http.Error(w, `{"error":"missing required parameter: model"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("expected nil error for 400 reachability, got %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/v1/responses" {
		t.Fatalf("expected /v1/responses, got %s", receivedPath)
	}
	if receivedAuth != "Bearer sk-test" {
		t.Fatalf("expected bearer auth, got %s", receivedAuth)
	}
	if receivedBody == "" {
		t.Fatal("expected non-empty request body")
	}
}

func TestTestUpstreamAccount_ReturnsPermissionErrorOn403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"insufficient scope"}`, http.StatusForbidden)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "鉴权或权限不足") || !strings.Contains(got, "403") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestTestUpstreamAccount_MissingResponsesWriteScopeHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Missing scopes: api.responses.write"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err == nil {
		t.Fatal("expected error for missing responses scope")
	}
	msg := err.Error()
	if !strings.Contains(msg, "缺少 api.responses.write") || !strings.Contains(msg, "重新走 OAuth 授权") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestTestUpstreamAccount_Treats429AsReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	svc, accountID := newTestAccountPoolServiceForConnectivity(t, server.URL, "api_key", "sk-test")

	err := svc.TestUpstreamAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("expected nil error for 429 reachability, got %v", err)
	}
}

func newTestAccountPoolServiceForConnectivity(t *testing.T, baseURL, providerType, credential string) (*AccountPoolService, int64) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schemaPath := filepath.Join("..", "tracking", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema failed: %v", err)
	}

	st := store.NewSQLiteAccountPoolStore(db)
	rec, err := st.CreateAccount(context.Background(), &store.UpstreamAccountRecord{
		ProviderType:  providerType,
		AccountName:   "acc-test",
		CredentialRaw: credential,
		BaseURL:       baseURL,
		Priority:      1,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	svc := NewAccountPoolService(st, nil)
	return svc, rec.ID
}
