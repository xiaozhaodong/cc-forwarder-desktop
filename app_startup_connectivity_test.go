package main

import (
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
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
