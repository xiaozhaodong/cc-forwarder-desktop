package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/service"
	"cc-forwarder/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

func TestLoadClaudeRoutingOverrideDoesNotDeadlockDuringStartupLock(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			value_type TEXT DEFAULT 'string',
			label TEXT,
			description TEXT,
			display_order INTEGER DEFAULT 0,
			requires_restart INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
			updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
			UNIQUE(category, key)
		);
	`); err != nil {
		t.Fatalf("create settings table failed: %v", err)
	}

	settingsStore := store.NewSQLiteSettingsStore(db)
	settingsService := service.NewSettingsService(settingsStore)
	ctx := context.Background()
	if err := settingsService.InitDefaults(ctx); err != nil {
		t.Fatalf("init default settings failed: %v", err)
	}
	for _, item := range []*store.SettingRecord{
		{Category: service.CategoryClaudeRouting, Key: "mode", Value: endpoint.RouteModeManualPreferred},
		{Category: service.CategoryClaudeRouting, Key: "endpoint_name", Value: "mywechat"},
		{Category: service.CategoryClaudeRouting, Key: "fallback_enabled", Value: "true"},
		{Category: service.CategoryClaudeRouting, Key: "set_by", Value: endpoint.RouteCallerUser},
		{Category: service.CategoryClaudeRouting, Key: "set_at", Value: "2026-06-02T17:27:05+08:00"},
	} {
		if err := settingsStore.BatchUpdateValues(ctx, []*store.SettingRecord{item}); err != nil {
			t.Fatalf("seed %s failed: %v", item.Key, err)
		}
	}

	app := NewApp()
	app.settingsService = settingsService
	app.settingsStore = settingsStore
	app.endpointManager = endpoint.NewManager(&config.Config{
		Endpoints: []config.EndpointConfig{
			{Name: "mywechat", URL: "https://example.com", Group: "mywechat"},
		},
	})

	app.mu.Lock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.loadClaudeRoutingOverride(ctx)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		app.mu.Unlock()
		t.Fatal("loadClaudeRoutingOverride deadlocked while App startup lock was held")
	}
	app.mu.Unlock()

	state := app.endpointManager.GetClaudeRoutingOverride()
	if state.Mode != endpoint.RouteModeManualPreferred || state.EndpointName != "mywechat" {
		t.Fatalf("unexpected restored state: mode=%s endpoint=%s", state.Mode, state.EndpointName)
	}
}
