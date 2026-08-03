package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCreateMigrationBackupCreatesVerifiedPermanentGroup(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "usage.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE fixture (id INTEGER PRIMARY KEY, value TEXT); INSERT INTO fixture VALUES (1, 'ok')`); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("endpoints_storage:\n  type: sqlite\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CreateMigrationBackup(context.Background(), BackupOptions{
		DB: db, DatabasePath: databasePath, ConfigPath: configPath, DataDir: dir,
		SourceMode: SourceModeSQLite, DatabaseExisted: true,
		Now: func() time.Time { return time.Date(2026, 8, 3, 15, 0, 0, 123, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IntegrityCheck != "ok" || result.ManifestSHA256 == "" {
		t.Fatalf("backup result = %+v", result)
	}
	for _, path := range []string{result.DatabasePath, result.ConfigPath, result.ManifestPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(result.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("backup directory mode = %o", info.Mode().Perm())
	}
}
