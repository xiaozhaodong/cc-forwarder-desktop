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

	endpointCalled := make(chan struct{}, 1)
	accountCalled := make(chan struct{}, 1)
	app.startupEndpointCheckRunner = func() {
		endpointCalled <- struct{}{}
	}
	app.startupAccountCheckRunner = func() {
		accountCalled <- struct{}{}
	}

	app.scheduleStartupConnectivityChecks()

	select {
	case <-endpointCalled:
		t.Fatal("expected endpoint startup check runner to be skipped")
	default:
	}

	select {
	case <-accountCalled:
		t.Fatal("expected account startup check runner to be skipped")
	default:
	}
}
