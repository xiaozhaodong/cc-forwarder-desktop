package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/store"
)

func TestScheduleStartupConnectivityChecks_DoesNotBlockStartup(t *testing.T) {
	app := NewApp()
	app.endpointManager = endpoint.NewManager(&config.Config{
		Endpoints: []config.EndpointConfig{
			{
				Name: "startup-endpoint",
				URL:  "https://example.com",
			},
		},
	})

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	app.startupEndpointCheckRunner = func() {
		started <- struct{}{}
		<-release
	}

	begin := time.Now()
	app.scheduleStartupConnectivityChecks()
	elapsed := time.Since(begin)

	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected startup checks to be async, took %v", elapsed)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected endpoint startup check runner to be scheduled")
	}

	close(release)
}

func TestScheduleStartupConnectivityChecks_DoesNotBlockStartupWhenAccountChecksScheduled(t *testing.T) {
	app := NewApp()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	app.startupAccountCheckRunner = func() {
		started <- struct{}{}
		<-release
	}

	begin := time.Now()
	app.scheduleStartupConnectivityChecks()
	elapsed := time.Since(begin)

	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected startup account checks to be async, took %v", elapsed)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected account startup check runner to be scheduled")
	}

	close(release)
}

func TestScheduleStartupConnectivityChecks_SkipsWhenNothingConfigured(t *testing.T) {
	app := NewApp()

	if app.shouldRunStartupEndpointChecks() {
		t.Fatal("expected endpoint startup checks to be skipped when no endpoints are configured")
	}
	if app.shouldRunStartupAccountChecks() {
		t.Fatal("expected account startup checks to be skipped when no account service is configured")
	}

	app.scheduleStartupConnectivityChecks()
}

func TestScheduleStartupConnectivityChecks_TriggersAccountChecksWhenEligibleAccountsExist(t *testing.T) {
	app, _ := newAccountPoolAPITestApp(t)

	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("expected request to /v1/responses, got %s", r.URL.Path)
		}
		requests <- struct{}{}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := app.accountPoolService.CreateAccount(context.Background(), &store.UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   "startup-account",
		CredentialRaw: "sk-startup-account",
		BaseURL:       server.URL,
		Priority:      10,
		Enabled:       true,
		State:         "active",
		QuotaStatus:   "ok",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	app.scheduleStartupConnectivityChecks()

	select {
	case <-requests:
	case <-time.After(time.Second):
		t.Fatal("expected startup account checks to trigger upstream connectivity test")
	}
}
