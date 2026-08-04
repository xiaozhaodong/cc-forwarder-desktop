package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	EndpointFlattenMigrationID = "20260803_claude_endpoint_flatten_v1"
	TimezoneUTCMigrationID     = "20260804_timezone_utc_v1"
	MigrationID                = EndpointFlattenMigrationID
)

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
	status, err := c.runEndpointFlatten(ctx)
	if err != nil {
		return status, err
	}
	legacy, err := LoadLegacyConfig(c.ConfigPath)
	if err != nil {
		return c.failForMigration(status, TimezoneUTCMigrationID, err)
	}
	return c.runTimezoneUTCMigration(ctx, legacy, status)
}

func (c *Coordinator) runEndpointFlatten(ctx context.Context) (Status, error) {
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
		if ledger.SourceMode != "" {
			status.SourceMode = string(ledger.SourceMode)
		}
		if ledger.Phase == PhaseCompleted {
			status.DatabaseIntegrity = "ok"
		} else {
			c.restoreStatusMetadata(ctx, &status, ledger)
		}
		c.setStatus(status)
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
		ledger = &migrationLedger{
			Phase:          PhaseDBCommitted,
			SourceMode:     legacy.SourceMode,
			BackupDir:      backup.Directory,
			ManifestSHA256: backup.ManifestSHA256,
		}
		c.setStatus(status)
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
	return c.failForMigration(status, EndpointFlattenMigrationID, err)
}

func (c *Coordinator) failForMigration(status Status, migrationID string, err error) (Status, error) {
	status.State = StartupMigrationFailed
	status.Error = sanitizeError(err)
	status.RetryAllowed = true
	if status.MigrationID == "" {
		status.MigrationID = migrationID
	}
	c.setStatus(status)
	if c.Logger != nil {
		c.Logger.Error("数据库迁移失败", "migration_id", migrationID, "phase", status.Phase, "error", status.Error, "backup_dir", status.BackupDir)
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
	SourceMode     SourceMode
	BackupDir      string
	ManifestSHA256 string
}

func loadLedger(ctx context.Context, db *sql.DB) (*migrationLedger, error) {
	exists, err := tableExists(ctx, db, "app_schema_migrations")
	if err != nil || !exists {
		return nil, err
	}
	var phase, sourceMode, backupDir, manifest string
	err = db.QueryRowContext(ctx, `SELECT phase, source_mode, backup_dir, backup_manifest_sha256 FROM app_schema_migrations WHERE migration_id = ?`, MigrationID).Scan(&phase, &sourceMode, &backupDir, &manifest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	return &migrationLedger{Phase: Phase(phase), SourceMode: SourceMode(sourceMode), BackupDir: backupDir, ManifestSHA256: manifest}, nil
}

func (c *Coordinator) restoreStatusMetadata(ctx context.Context, status *Status, ledger *migrationLedger) {
	if status == nil || ledger == nil {
		return
	}

	manifest, err := readVerifiedBackupManifest(ledger.BackupDir, ledger.ManifestSHA256, EndpointFlattenMigrationID)
	if err != nil {
		c.warnStatusRestore("读取迁移备份清单失败", err)
	} else {
		status.BackupIntegrity = manifest.IntegrityCheck
	}

	if count, ok, err := countSQLiteTableRows(ctx, c.DB, "endpoints"); err != nil {
		c.warnStatusRestore("恢复迁移后端点计数失败", err)
	} else if ok {
		status.EndpointCountAfter = int(count)
	}
	if count, ok, err := countSQLiteTableRows(ctx, c.DB, "request_logs"); err != nil {
		c.warnStatusRestore("恢复请求历史计数失败", err)
	} else if ok {
		status.RequestLogCount = count
	}

	backupDBPath := filepath.Join(ledger.BackupDir, "usage.db")
	if _, err := os.Stat(backupDBPath); err == nil {
		backupDB, openErr := sql.Open("sqlite", "file:"+backupDBPath+"?mode=ro")
		if openErr != nil {
			c.warnStatusRestore("打开迁移备份数据库失败", openErr)
		} else {
			if count, ok, countErr := countSQLiteTableRows(ctx, backupDB, "endpoints"); countErr != nil {
				c.warnStatusRestore("恢复迁移前端点计数失败", countErr)
			} else if ok {
				status.EndpointCountBefore = int(count)
			}
			_ = backupDB.Close()
		}
	} else if !os.IsNotExist(err) {
		c.warnStatusRestore("检查迁移备份数据库失败", err)
	}

	if ledger.SourceMode == SourceModeYAML {
		legacy, loadErr := LoadLegacyConfig(filepath.Join(ledger.BackupDir, "config.yaml"))
		if loadErr != nil {
			c.warnStatusRestore("读取迁移前配置失败", loadErr)
			return
		}
		flattened, flattenErr := FlattenLegacyEndpoints(legacy)
		if flattenErr != nil {
			c.warnStatusRestore("恢复端点拆分计数失败", flattenErr)
			return
		}
		status.SplitEndpointCount = flattened.SplitEndpointCount
		status.DerivedRecordCount = flattened.DerivedRecordCount
	}
}

func readVerifiedBackupManifest(directory, expectedSHA256, expectedMigrationID string) (BackupManifest, error) {
	var manifest BackupManifest
	if strings.TrimSpace(directory) == "" {
		return manifest, fmt.Errorf("backup directory is empty")
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	_, actualSHA256, err := fileInfoAndSHA256(manifestPath)
	if err != nil {
		return manifest, err
	}
	if expectedSHA256 != "" && !strings.EqualFold(actualSHA256, expectedSHA256) {
		return manifest, fmt.Errorf("backup manifest hash mismatch")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	if manifest.MigrationID != expectedMigrationID {
		return manifest, fmt.Errorf("unexpected backup migration id %q", manifest.MigrationID)
	}
	return manifest, nil
}

func countSQLiteTableRows(ctx context.Context, db *sql.DB, table string) (int64, bool, error) {
	if db == nil {
		return 0, false, nil
	}
	switch table {
	case "endpoints", "request_logs":
	default:
		return 0, false, fmt.Errorf("unsupported count table %q", table)
	}
	exists, err := tableExists(ctx, db, table)
	if err != nil || !exists {
		return 0, false, err
	}
	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+quoteIdentifier(table)).Scan(&count); err != nil {
		return 0, false, err
	}
	return count, true, nil
}

func (c *Coordinator) warnStatusRestore(message string, err error) {
	if c.Logger != nil && err != nil {
		c.Logger.Warn(message, "migration_id", MigrationID, "error", err)
	}
}

func updateLedgerPhase(ctx context.Context, db *sql.DB, phase Phase, errorMessage string) error {
	column := ""
	switch phase {
	case PhaseDBCommitted:
		column = ", db_committed_at = strftime('%Y-%m-%dT%H:%M:%f000Z', 'now')"
	case PhaseConfigCommitted:
		column = ", config_committed_at = strftime('%Y-%m-%dT%H:%M:%f000Z', 'now')"
	case PhaseCompleted:
		column = ", completed_at = strftime('%Y-%m-%dT%H:%M:%f000Z', 'now')"
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

func columnExists(ctx context.Context, db queryContexter, table, column string) (bool, error) {
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

type queryContexter interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
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
