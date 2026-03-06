package tracking

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateBackupAndRestore_PreservesCoreTables(t *testing.T) {
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
		INSERT INTO upstream_accounts (id, provider_type, account_name, credential_raw, base_url, priority, enabled, state, fail_count, fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 10, "api_key", "account-1", "sk-test", "https://chatgpt.com", 1, 1, "active", 0, "fp-1"); err != nil {
		t.Fatalf("failed to insert upstream_accounts seed: %v", err)
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
		"upstream_accounts",
	} {
		if _, err := tracker.db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("failed to clear %s before restore: %v", table, err)
		}
	}

	if err := tracker.errorHandler.RestoreFromBackup(); err != nil {
		t.Fatalf("failed to restore backup: %v", err)
	}

	counts := map[string]int{
		"request_logs":      1,
		"usage_summary":     1,
		"upstream_accounts": 1,
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
