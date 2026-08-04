package migration

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCoordinatorRunMigratesAndCompletes(t *testing.T) {
	db := openLegacyFixtureDB(t)
	defer db.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	coordinator := &Coordinator{
		DB: db, DatabasePath: filepath.Join(dir, "usage.db"), ConfigPath: configPath,
		DataDir: dir, DatabaseExisted: true,
		Now: func() time.Time { return time.Date(2026, 8, 3, 16, 0, 0, 0, time.UTC) },
	}
	status, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StartupReady || status.Phase != PhaseCompleted || status.EndpointCountAfter != 10 || status.BackupIntegrity != "ok" {
		t.Fatalf("status = %+v", status)
	}
	legacy, err := LoadLegacyConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.SourceMode != SourceModeSQLite || len(legacy.Endpoints) != 0 {
		t.Fatalf("rewritten config = source:%q endpoints:%d", legacy.SourceMode, len(legacy.Endpoints))
	}
	second, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.State != StartupReady || second.Phase != PhaseCompleted {
		t.Fatalf("second run status = %+v", second)
	}
	var endpointCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpointCount); err != nil {
		t.Fatal(err)
	}
	if endpointCount != 10 {
		t.Fatalf("second run repeated split, endpoint count = %d", endpointCount)
	}
}

func TestCoordinatorRetryAfterConfigRewriteFailureDoesNotRepeatSplit(t *testing.T) {
	db := openLegacyFixtureDB(t)
	defer db.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	coordinator := &Coordinator{
		DB: db, DatabasePath: filepath.Join(dir, "usage.db"), ConfigPath: configPath,
		DataDir: dir, DatabaseExisted: true,
		RewriteConfig: func(string) error { return fmt.Errorf("fixture config write failure") },
	}
	failed, err := coordinator.Run(context.Background())
	if err == nil {
		t.Fatal("expected config rewrite failure")
	}
	if failed.State != StartupMigrationFailed || failed.Phase != PhaseDBCommitted || !failed.RetryAllowed {
		t.Fatalf("failed status = %+v", failed)
	}
	var endpointCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpointCount); err != nil {
		t.Fatal(err)
	}
	if endpointCount != 10 {
		t.Fatalf("database commit endpoint count = %d", endpointCount)
	}
	legacy, err := LoadLegacyConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.SourceMode != SourceModeYAML || len(legacy.Endpoints) == 0 {
		t.Fatalf("failed rewrite must leave legacy config intact: source=%q endpoints=%d", legacy.SourceMode, len(legacy.Endpoints))
	}

	coordinator.RewriteConfig = nil
	retried, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != StartupReady || retried.Phase != PhaseCompleted || retried.RetryAllowed {
		t.Fatalf("retried status = %+v", retried)
	}
	if retried.BackupIntegrity != failed.BackupIntegrity ||
		retried.EndpointCountBefore != failed.EndpointCountBefore ||
		retried.EndpointCountAfter != failed.EndpointCountAfter ||
		retried.SplitEndpointCount != failed.SplitEndpointCount ||
		retried.DerivedRecordCount != failed.DerivedRecordCount ||
		retried.RequestLogCount != failed.RequestLogCount {
		t.Fatalf("retried status metadata changed: failed=%+v retried=%+v", failed, retried)
	}
	var endpointCountAfterRetry int
	if err := db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpointCountAfterRetry); err != nil {
		t.Fatal(err)
	}
	if endpointCountAfterRetry != endpointCount {
		t.Fatalf("retry repeated endpoint split: before=%d after=%d", endpointCount, endpointCountAfterRetry)
	}
}

func TestCoordinatorPublishesDBCommittedStatusBeforeConfigRewriteCompletes(t *testing.T) {
	db := openLegacyFixtureDB(t)
	defer db.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	rewriteStarted := make(chan struct{})
	releaseRewrite := make(chan struct{})
	coordinator := &Coordinator{
		DB: db, DatabasePath: filepath.Join(dir, "usage.db"), ConfigPath: configPath,
		DataDir: dir, DatabaseExisted: true,
		RewriteConfig: func(string) error {
			close(rewriteStarted)
			<-releaseRewrite
			return fmt.Errorf("fixture rewrite stop")
		},
	}

	result := make(chan Status, 1)
	go func() {
		status, _ := coordinator.Run(context.Background())
		result <- status
	}()

	select {
	case <-rewriteStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("config rewrite did not start")
	}
	status := coordinator.Status()
	if status.State != StartupMigrating || status.Phase != PhaseDBCommitted ||
		status.EndpointCountBefore != 2 || status.EndpointCountAfter != 10 ||
		status.BackupIntegrity != "ok" || status.RequestLogCount != 4 {
		t.Fatalf("published status = %+v", status)
	}

	close(releaseRewrite)
	select {
	case <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator did not stop after rewrite release")
	}
}

// TestCoordinatorOfflineCopyDrill 只在显式传入同一隔离目录下的数据库/配置副本时运行。
// 测试会原地改写这两份副本，但不会访问或修改副本目录之外的文件。
func TestCoordinatorOfflineCopyDrill(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("MIGRATION_DRILL_ROOT"))
	databasePath := strings.TrimSpace(os.Getenv("MIGRATION_DRILL_DB"))
	configPath := strings.TrimSpace(os.Getenv("MIGRATION_DRILL_CONFIG"))
	if root == "" || databasePath == "" || configPath == "" {
		t.Skip("set MIGRATION_DRILL_ROOT, MIGRATION_DRILL_DB and MIGRATION_DRILL_CONFIG to isolated copies")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	databasePath = requirePathWithinDrillRoot(t, root, databasePath)
	configPath = requirePathWithinDrillRoot(t, root, configPath)

	db, err := sql.Open("sqlite", databasePath+"?_foreign_keys=1")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	legacyBefore, err := LoadLegacyConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := readDrillMetrics(db)
	if err != nil {
		t.Fatal(err)
	}
	var endpointsBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpointsBefore); err != nil {
		t.Fatal(err)
	}
	legacyEndpointSchemaBefore, err := columnExists(context.Background(), db, "endpoints", "channel")
	if err != nil {
		t.Fatal(err)
	}

	coordinator := &Coordinator{
		DB: db, DatabasePath: databasePath, ConfigPath: configPath,
		DataDir: root, DatabaseExisted: true,
	}
	status, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StartupReady || status.Phase != PhaseCompleted || status.DatabaseIntegrity != "ok" || status.BackupIntegrity != "ok" {
		t.Fatalf("migration status = %+v", status)
	}
	if status.EndpointCountBefore != endpointsBefore {
		t.Fatalf("endpoint count before = %d, want %d", status.EndpointCountBefore, endpointsBefore)
	}
	if status.BackupDir == "" {
		t.Fatal("offline drill expected a migration backup")
	}

	after, err := readDrillMetrics(db)
	if err != nil {
		t.Fatal(err)
	}
	if !before.equal(after) {
		t.Fatalf("request metrics changed during migration: before=%+v after=%+v", before, after)
	}
	for table, columns := range map[string][]string{
		"endpoints":    {"channel", "enabled"},
		"request_logs": {"channel", "group_name"},
	} {
		for _, column := range columns {
			has, err := columnExists(context.Background(), db, table, column)
			if err != nil {
				t.Fatal(err)
			}
			if has {
				t.Fatalf("legacy column %s.%s still exists", table, column)
			}
		}
	}
	if err := assertNoForeignKeyViolations(db); err != nil {
		t.Fatal(err)
	}
	legacy, err := LoadLegacyConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.SourceMode != SourceModeSQLite || len(legacy.Endpoints) != 0 {
		t.Fatalf("rewritten config = source:%q endpoints:%d", legacy.SourceMode, len(legacy.Endpoints))
	}

	var endpointCountAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpointCountAfter); err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.State != StartupReady || second.Phase != PhaseCompleted {
		t.Fatalf("second run status = %+v", second)
	}
	var endpointCountSecond int
	if err := db.QueryRow(`SELECT COUNT(*) FROM endpoints`).Scan(&endpointCountSecond); err != nil {
		t.Fatal(err)
	}
	if endpointCountSecond != endpointCountAfter {
		t.Fatalf("idempotent run changed endpoint count: first=%d second=%d", endpointCountAfter, endpointCountSecond)
	}
	timezoneManifest, err := readVerifiedBackupManifest(status.BackupDir, status.ManifestSHA256, TimezoneUTCMigrationID)
	if err != nil {
		t.Fatal(err)
	}
	if timezoneManifest.LegacyTimezone == "" {
		t.Fatal("timezone rollback backup did not pin the legacy timezone")
	}

	rollbackBackupDir := status.BackupDir
	if legacyBefore.SourceMode != SourceModeSQLite || len(legacyBefore.Endpoints) > 0 || legacyEndpointSchemaBefore {
		if err := db.QueryRow(`SELECT backup_dir FROM app_schema_migrations WHERE migration_id=?`, EndpointFlattenMigrationID).Scan(&rollbackBackupDir); err != nil {
			t.Fatal(err)
		}
		rollbackBackupDir = requirePathWithinDrillRoot(t, root, rollbackBackupDir)
	}

	rollbackDir := filepath.Join(root, "rollback-verification")
	if err := os.Mkdir(rollbackDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollbackDB := filepath.Join(rollbackDir, "usage.db")
	rollbackConfig := filepath.Join(rollbackDir, "config.yaml")
	if err := copyFile0600(filepath.Join(rollbackBackupDir, "usage.db"), rollbackDB); err != nil {
		t.Fatal(err)
	}
	if err := copyFile0600(filepath.Join(rollbackBackupDir, "config.yaml"), rollbackConfig); err != nil {
		t.Fatal(err)
	}
	if integrity, err := checkSQLiteIntegrity(rollbackDB); err != nil || integrity != "ok" {
		t.Fatalf("rollback database integrity = %q, err=%v", integrity, err)
	}
	rollbackLegacy, err := LoadLegacyConfig(rollbackConfig)
	if err != nil {
		t.Fatal(err)
	}
	if rollbackLegacy.SourceMode != legacyBefore.SourceMode || len(rollbackLegacy.Endpoints) != len(legacyBefore.Endpoints) {
		t.Fatalf("rollback config source/endpoints = %q/%d, want %q/%d", rollbackLegacy.SourceMode, len(rollbackLegacy.Endpoints), legacyBefore.SourceMode, len(legacyBefore.Endpoints))
	}
	rollback, err := sql.Open("sqlite", "file:"+rollbackDB+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Close()
	if legacyEndpointSchemaBefore {
		for _, column := range []string{"channel", "enabled"} {
			has, err := columnExists(context.Background(), rollback, "endpoints", column)
			if err != nil {
				t.Fatal(err)
			}
			if !has {
				t.Fatalf("rollback copy is missing legacy endpoints.%s", column)
			}
		}
	}
	t.Logf("offline migration and rollback verification passed: endpoints=%d->%d requests=%d timezone_backup=%s rollback_backup=%s",
		endpointsBefore, endpointCountAfter, after.RequestCount, status.BackupDir, rollbackBackupDir)
}

type drillMetrics struct {
	RequestCount int64
	TokenTotal   int64
	CostTotal    float64
}

func (m drillMetrics) equal(other drillMetrics) bool {
	return m.RequestCount == other.RequestCount && m.TokenTotal == other.TokenTotal && math.Abs(m.CostTotal-other.CostTotal) < 1e-9
}

func readDrillMetrics(db *sql.DB) (drillMetrics, error) {
	var metrics drillMetrics
	err := db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0),
		COALESCE(SUM(total_cost_usd), 0)
		FROM request_logs`).Scan(&metrics.RequestCount, &metrics.TokenTotal, &metrics.CostTotal)
	return metrics, err
}

func requirePathWithinDrillRoot(t *testing.T, root, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		t.Fatal(err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("drill path %q must be a file below isolated root %q", absolute, root)
	}
	return absolute
}

func assertNoForeignKeyViolations(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign_key_check returned at least one violation")
	}
	return rows.Err()
}
