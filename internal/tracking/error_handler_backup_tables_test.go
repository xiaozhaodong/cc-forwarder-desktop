package tracking

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateBackupAndRestore_PreservesAccountPoolTables(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "backup-restore.db")

	tracker, err := NewUsageTracker(&Config{
		Enabled:         true,
		DatabasePath:    dbPath,
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	})
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	now := time.Now().Format(time.RFC3339)
	ctx := context.Background()

	if _, err := tracker.db.ExecContext(ctx, `
		INSERT INTO request_logs (request_id, start_time, status, endpoint_name, group_name)
		VALUES (?, ?, ?, ?, ?)
	`, "req-backup-1", now, "completed", "claude-endpoint", "claude-group"); err != nil {
		t.Fatalf("failed to insert request_logs seed: %v", err)
	}

	if _, err := tracker.db.ExecContext(ctx, `
		INSERT INTO usage_summary (date, model_name, endpoint_name, group_name, request_count, success_count, error_count, total_input_tokens, total_output_tokens, total_cache_creation_tokens, total_cache_read_tokens, total_cost_usd, avg_duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "2026-03-06", "gpt-5-codex", "claude-endpoint", "claude-group", 1, 1, 0, 10, 5, 0, 0, 1.25, 120); err != nil {
		t.Fatalf("failed to insert usage_summary seed: %v", err)
	}

	if _, err := tracker.db.ExecContext(ctx, `
		INSERT INTO subscription_sources (id, name, url, enabled, sync_mode, last_status, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, 1, "source-1", "https://example.com/subscription", 1, "manual", "ok", ""); err != nil {
		t.Fatalf("failed to insert subscription_sources seed: %v", err)
	}

	if _, err := tracker.db.ExecContext(ctx, `
		INSERT INTO upstream_accounts (id, source_id, provider_type, account_name, credential_raw, base_url, priority, enabled, state, fail_count, fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 10, 1, "api_key", "account-1", "sk-test", "https://chatgpt.com", 1, 1, "active", 0, "fp-1"); err != nil {
		t.Fatalf("failed to insert upstream_accounts seed: %v", err)
	}

	if _, err := tracker.db.ExecContext(ctx, `
		INSERT INTO account_request_logs (request_id, account_id, source_name, account_name, start_time, status, model_name, input_tokens, output_tokens, total_cost_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "req-account-1", 10, "source-1", "account-1", now, "completed", "gpt-5-codex", 12, 4, 0.55); err != nil {
		t.Fatalf("failed to insert account_request_logs seed: %v", err)
	}

	if _, err := tracker.db.ExecContext(ctx, `
		INSERT INTO sync_logs (source_id, started_at, finished_at, result, added_count, updated_count, disabled_count, error_summary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, 1, now, now, "success", 1, 0, 0, ""); err != nil {
		t.Fatalf("failed to insert sync_logs seed: %v", err)
	}

	if err := tracker.errorHandler.CreateBackup(); err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}

	backupPath := dbPath + ".backup"
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	for _, table := range []string{
		"request_logs",
		"usage_summary",
		"account_request_logs",
		"subscription_sources",
		"upstream_accounts",
		"sync_logs",
	} {
		if _, err := tracker.db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("failed to clear %s before restore: %v", table, err)
		}
	}

	if err := tracker.errorHandler.RestoreFromBackup(); err != nil {
		t.Fatalf("failed to restore backup: %v", err)
	}

	counts := map[string]int{
		"request_logs":         1,
		"usage_summary":        1,
		"account_request_logs": 1,
		"subscription_sources": 1,
		"upstream_accounts":    1,
		"sync_logs":            1,
	}
	for table, expected := range counts {
		var actual int
		if err := tracker.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&actual); err != nil {
			t.Fatalf("failed to query %s count: %v", table, err)
		}
		if actual != expected {
			t.Fatalf("expected %s count=%d after restore, got %d", table, expected, actual)
		}
	}
}
