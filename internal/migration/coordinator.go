package migration

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const MigrationID = "20260803_claude_endpoint_flatten_v1"

type Phase string

const (
	PhasePrepared        Phase = "prepared"
	PhaseDBCommitted     Phase = "db_committed"
	PhaseConfigCommitted Phase = "config_committed"
	PhaseCompleted       Phase = "completed"
	PhaseFailed          Phase = "failed"
)

type StartupState string

const (
	StartupInitializing      StartupState = "initializing"
	StartupMigrationRequired StartupState = "migration_required"
	StartupMigrating         StartupState = "migrating"
	StartupMigrationFailed   StartupState = "migration_failed"
	StartupReady             StartupState = "ready"
)

type Status struct {
	State               StartupState `json:"state"`
	MigrationID         string       `json:"migration_id"`
	Phase               Phase        `json:"phase"`
	Error               string       `json:"error"`
	DatabasePath        string       `json:"database_path"`
	ConfigPath          string       `json:"config_path"`
	BackupDir           string       `json:"backup_dir"`
	DatabaseIntegrity   string       `json:"database_integrity"`
	BackupIntegrity     string       `json:"backup_integrity"`
	RetryAllowed        bool         `json:"retry_allowed"`
	DatabaseExisted     bool         `json:"database_existed"`
	SourceMode          string       `json:"source_mode"`
	EndpointCountBefore int          `json:"endpoint_count_before"`
	EndpointCountAfter  int          `json:"endpoint_count_after"`
	SplitEndpointCount  int          `json:"split_endpoint_count"`
	DerivedRecordCount  int          `json:"derived_record_count"`
	RequestLogCount     int64        `json:"request_log_count"`
	ManifestSHA256      string       `json:"backup_manifest_sha256"`
}

type Coordinator struct {
	DB              *sql.DB
	DatabasePath    string
	ConfigPath      string
	DataDir         string
	DatabaseExisted bool
	Logger          *slog.Logger
	Now             func() time.Time
	RewriteConfig   func(string) error

	mu     sync.RWMutex
	status Status
}

func (c *Coordinator) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Coordinator) setStatus(status Status) {
	c.mu.Lock()
	c.status = status
	c.mu.Unlock()
}

func (c *Coordinator) Run(ctx context.Context) (Status, error) {
	if c.DB == nil {
		return c.fail(Status{}, fmt.Errorf("migration database is nil"))
	}
	legacy, err := LoadLegacyConfig(c.ConfigPath)
	if err != nil {
		return c.fail(Status{}, err)
	}
	status := Status{
		State:           StartupInitializing,
		MigrationID:     MigrationID,
		DatabasePath:    c.DatabasePath,
		ConfigPath:      c.ConfigPath,
		DatabaseExisted: c.DatabaseExisted,
		SourceMode:      string(legacy.SourceMode),
	}
	c.setStatus(status)

	ledger, err := loadLedger(ctx, c.DB)
	if err != nil {
		return c.fail(status, err)
	}
	needs, err := needsMigration(ctx, c.DB, legacy)
	if err != nil {
		return c.fail(status, err)
	}
	if ledger != nil && ledger.Phase == PhaseCompleted && !needs {
		status.State = StartupReady
		status.Phase = PhaseCompleted
		status.BackupDir = ledger.BackupDir
		status.ManifestSHA256 = ledger.ManifestSHA256
		c.setStatus(status)
		return status, nil
	}
	if ledger != nil {
		status.BackupDir = ledger.BackupDir
		status.ManifestSHA256 = ledger.ManifestSHA256
		status.Phase = ledger.Phase
	}
	if ledger != nil && ledger.Phase == PhaseCompleted && needs {
		return c.fail(status, fmt.Errorf("migration ledger is completed but legacy schema or config is still active"))
	}
	if !needs && ledger == nil {
		status.State = StartupReady
		c.setStatus(status)
		return status, nil
	}

	status.State = StartupMigrating
	status.Phase = PhasePrepared
	c.setStatus(status)

	if ledger == nil || ledger.Phase == PhasePrepared || ledger.Phase == PhaseFailed {
		backup, err := CreateMigrationBackup(ctx, BackupOptions{
			DB: c.DB, DatabasePath: c.DatabasePath, ConfigPath: c.ConfigPath,
			DataDir: c.DataDir, SourceMode: legacy.SourceMode,
			DatabaseExisted: c.DatabaseExisted, Now: c.Now,
		})
		if err != nil {
			return c.fail(status, err)
		}
		status.BackupDir = backup.Directory
		status.BackupIntegrity = backup.IntegrityCheck
		status.ManifestSHA256 = backup.ManifestSHA256
		c.setStatus(status)

		migrationResult, err := migrateDatabase(ctx, c.DB, legacy, backup)
		if err != nil {
			return c.fail(status, err)
		}
		status.Phase = PhaseDBCommitted
		status.EndpointCountBefore = migrationResult.EndpointCountBefore
		status.EndpointCountAfter = migrationResult.EndpointCountAfter
		status.SplitEndpointCount = migrationResult.SplitEndpointCount
		status.DerivedRecordCount = migrationResult.DerivedRecordCount
		status.RequestLogCount = migrationResult.RequestLogCount
		ledger = &migrationLedger{Phase: PhaseDBCommitted, BackupDir: backup.Directory, ManifestSHA256: backup.ManifestSHA256}
	}

	if ledger.Phase == PhaseDBCommitted {
		rewriteConfig := c.RewriteConfig
		if rewriteConfig == nil {
			rewriteConfig = RewriteConfigToSQLite
		}
		if err := rewriteConfig(c.ConfigPath); err != nil {
			return c.fail(status, err)
		}
		if err := updateLedgerPhase(ctx, c.DB, PhaseConfigCommitted, ""); err != nil {
			return c.fail(status, err)
		}
		ledger.Phase = PhaseConfigCommitted
		status.Phase = PhaseConfigCommitted
		c.setStatus(status)
	}

	if ledger.Phase == PhaseConfigCommitted {
		integrity, err := checkDatabaseIntegrity(ctx, c.DB)
		if err != nil {
			return c.fail(status, err)
		}
		if integrity != "ok" {
			return c.fail(status, fmt.Errorf("migrated database integrity_check returned %q", integrity))
		}
		if err := validateTargetSchema(ctx, c.DB); err != nil {
			return c.fail(status, err)
		}
		if err := updateLedgerPhase(ctx, c.DB, PhaseCompleted, ""); err != nil {
			return c.fail(status, err)
		}
		status.DatabaseIntegrity = integrity
		status.Phase = PhaseCompleted
	}
	status.State = StartupReady
	status.RetryAllowed = false
	c.setStatus(status)
	if c.Logger != nil {
		c.Logger.Info("Claude 端点扁平化迁移完成",
			"migration_id", MigrationID, "source_mode", status.SourceMode,
			"backup_dir", status.BackupDir, "endpoint_count_before", status.EndpointCountBefore,
			"endpoint_count_after", status.EndpointCountAfter, "derived_record_count", status.DerivedRecordCount,
			"request_log_count", status.RequestLogCount)
	}
	return status, nil
}

func (c *Coordinator) fail(status Status, err error) (Status, error) {
	status.State = StartupMigrationFailed
	status.Error = sanitizeError(err)
	status.RetryAllowed = true
	if status.MigrationID == "" {
		status.MigrationID = MigrationID
	}
	c.setStatus(status)
	if c.Logger != nil {
		c.Logger.Error("Claude 端点扁平化迁移失败", "migration_id", MigrationID, "phase", status.Phase, "error", status.Error, "backup_dir", status.BackupDir)
	}
	return status, err
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 800 {
		message = message[:800]
	}
	return message
}

type migrationLedger struct {
	Phase          Phase
	BackupDir      string
	ManifestSHA256 string
}

func loadLedger(ctx context.Context, db *sql.DB) (*migrationLedger, error) {
	exists, err := tableExists(ctx, db, "app_schema_migrations")
	if err != nil || !exists {
		return nil, err
	}
	var phase, backupDir, manifest string
	err = db.QueryRowContext(ctx, `SELECT phase, backup_dir, backup_manifest_sha256 FROM app_schema_migrations WHERE migration_id = ?`, MigrationID).Scan(&phase, &backupDir, &manifest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	return &migrationLedger{Phase: Phase(phase), BackupDir: backupDir, ManifestSHA256: manifest}, nil
}

func updateLedgerPhase(ctx context.Context, db *sql.DB, phase Phase, errorMessage string) error {
	column := ""
	switch phase {
	case PhaseDBCommitted:
		column = ", db_committed_at = CURRENT_TIMESTAMP"
	case PhaseConfigCommitted:
		column = ", config_committed_at = CURRENT_TIMESTAMP"
	case PhaseCompleted:
		column = ", completed_at = CURRENT_TIMESTAMP"
	}
	_, err := db.ExecContext(ctx, `UPDATE app_schema_migrations SET phase = ?, error_message = ?`+column+` WHERE migration_id = ?`, string(phase), errorMessage, MigrationID)
	if err != nil {
		return fmt.Errorf("update migration ledger phase: %w", err)
	}
	return nil
}

func needsMigration(ctx context.Context, db *sql.DB, legacy *LegacyConfig) (bool, error) {
	if legacy.SourceMode != SourceModeSQLite || len(legacy.Endpoints) > 0 {
		return true, nil
	}
	for table, columns := range map[string][]string{
		"endpoints":    {"channel", "enabled"},
		"request_logs": {"channel", "group_name"},
	} {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		for _, column := range columns {
			has, err := columnExists(ctx, db, table, column)
			if err != nil {
				return false, err
			}
			if has {
				return true, nil
			}
		}
	}
	return false, nil
}

func tableExists(ctx context.Context, db queryRower, table string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quoteIdentifier(table)+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func checkDatabaseIntegrity(ctx context.Context, db *sql.DB) (string, error) {
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return "", fmt.Errorf("check migrated database integrity: %w", err)
	}
	return result, nil
}
