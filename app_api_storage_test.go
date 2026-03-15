package main

import (
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/service"
	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

func TestCreateEndpointRecord_PersistsCacheCreationCostMultiplier1h(t *testing.T) {
	app, cleanup := newEndpointStorageAPITestApp(t)
	defer cleanup()

	input := CreateEndpointInput{
		Channel:                       "openai",
		Name:                          "ep-cache-1h",
		URL:                           "https://api.example.com",
		Token:                         "sk-test-token",
		Priority:                      10,
		FailoverEnabled:               true,
		TimeoutSeconds:                120,
		SupportsCountTokens:           true,
		CostMultiplier:                1.2,
		InputCostMultiplier:           1.3,
		OutputCostMultiplier:          1.4,
		CacheCreationCostMultiplier:   1.5,
		CacheCreationCostMultiplier1h: 2.5,
		CacheReadCostMultiplier:       1.6,
	}

	if err := app.CreateEndpointRecord(input); err != nil {
		t.Fatalf("CreateEndpointRecord failed: %v", err)
	}

	records, err := app.GetEndpointRecords()
	if err != nil {
		t.Fatalf("GetEndpointRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 endpoint record, got %d", len(records))
	}
	if got := records[0].CacheCreationCostMultiplier1h; got != 2.5 {
		t.Fatalf("expected list API cache_creation_cost_multiplier_1h=2.5, got %v", got)
	}

	record, err := app.GetEndpointRecord("ep-cache-1h")
	if err != nil {
		t.Fatalf("GetEndpointRecord failed: %v", err)
	}
	if got := record.CacheCreationCostMultiplier1h; got != 2.5 {
		t.Fatalf("expected detail API cache_creation_cost_multiplier_1h=2.5, got %v", got)
	}

	update := input
	update.CacheCreationCostMultiplier1h = 3.5
	if err := app.UpdateEndpointRecord("ep-cache-1h", update); err != nil {
		t.Fatalf("UpdateEndpointRecord failed: %v", err)
	}

	updated, err := app.GetEndpointRecord("ep-cache-1h")
	if err != nil {
		t.Fatalf("GetEndpointRecord after update failed: %v", err)
	}
	if got := updated.CacheCreationCostMultiplier1h; got != 3.5 {
		t.Fatalf("expected updated detail API cache_creation_cost_multiplier_1h=3.5, got %v", got)
	}
}

func newEndpointStorageAPITestApp(t *testing.T) (*App, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "endpoint_storage_api_test_*")
	if err != nil {
		t.Fatalf("create temp dir failed: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("open sqlite failed: %v", err)
	}

	schema := `
		CREATE TABLE IF NOT EXISTS endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel TEXT NOT NULL,
			name TEXT UNIQUE NOT NULL,
			url TEXT NOT NULL,
			token TEXT,
			api_key TEXT,
			headers TEXT,
			priority INTEGER DEFAULT 1,
			failover_enabled INTEGER DEFAULT 1,
			cooldown_seconds INTEGER,
			timeout_seconds INTEGER DEFAULT 300,
			supports_count_tokens INTEGER DEFAULT 0,
			cost_multiplier REAL DEFAULT 1.0,
			input_cost_multiplier REAL DEFAULT 1.0,
			output_cost_multiplier REAL DEFAULT 1.0,
			cache_creation_cost_multiplier REAL DEFAULT 1.0,
			cache_creation_cost_multiplier_1h REAL DEFAULT 1.0,
			cache_read_cost_multiplier REAL DEFAULT 1.0,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
			updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
		);
		CREATE INDEX IF NOT EXISTS idx_endpoints_channel ON endpoints(channel);
		CREATE INDEX IF NOT EXISTS idx_endpoints_priority ON endpoints(priority);
		CREATE INDEX IF NOT EXISTS idx_endpoints_enabled ON endpoints(enabled);
	`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("create schema failed: %v", err)
	}

	cfg := &config.Config{
		Health: config.HealthConfig{
			Timeout: 5 * time.Second,
		},
		Endpoints: []config.EndpointConfig{},
	}

	manager := endpoint.NewManager(cfg)
	st := store.NewSQLiteEndpointStore(db)
	svc := service.NewEndpointService(st, manager, cfg)

	app := NewApp()
	app.config = cfg
	app.endpointManager = manager
	app.endpointStore = st
	app.endpointService = svc
	app.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	cleanup := func() {
		manager.Stop()
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	}
	return app, cleanup
}
