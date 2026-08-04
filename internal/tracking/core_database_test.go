package tracking

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCoreDatabaseOpensWithoutUsageTracker(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.db")
	core, err := OpenCoreDatabase(DatabaseConfig{Type: "sqlite", DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("OpenCoreDatabase failed: %v", err)
	}
	defer core.Close()

	if err := core.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	if err := core.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	var count int
	if err := core.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='endpoints'`).Scan(&count); err != nil {
		t.Fatalf("query endpoints table failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected endpoints table, got count=%d", count)
	}
}

func TestCoreDatabaseSchemaUsesCanonicalUTCTimestamps(t *testing.T) {
	core, err := OpenCoreDatabase(DatabaseConfig{Type: "sqlite", DatabasePath: filepath.Join(t.TempDir(), "usage.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if err := core.InitSchema(); err != nil {
		t.Fatal(err)
	}

	rows, err := core.DB().Query(`SELECT name, sql FROM sqlite_master WHERE sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, statement string
		if err := rows.Scan(&name, &statement); err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(statement)
		if strings.Contains(lower, "localtime") || strings.Contains(lower, "current_timestamp") {
			t.Fatalf("schema object %s contains legacy timestamp expression: %s", name, statement)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if _, err := core.DB().Exec(`INSERT INTO endpoints (name, url) VALUES ('utc-default', 'https://example.test')`); err != nil {
		t.Fatal(err)
	}
	var createdAt, updatedAt string
	if err := core.DB().QueryRow(`SELECT CAST(created_at AS TEXT), CAST(updated_at AS TEXT) FROM endpoints WHERE name='utc-default'`).Scan(&createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	canonical := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$`)
	if !canonical.MatchString(createdAt) || !canonical.MatchString(updatedAt) {
		t.Fatalf("default timestamps are not canonical UTC: created=%q updated=%q", createdAt, updatedAt)
	}
	if _, err := core.DB().Exec(`UPDATE endpoints SET priority=2 WHERE name='utc-default'`); err != nil {
		t.Fatal(err)
	}
	if err := core.DB().QueryRow(`SELECT CAST(updated_at AS TEXT) FROM endpoints WHERE name='utc-default'`).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if !canonical.MatchString(updatedAt) {
		t.Fatalf("trigger timestamp is not canonical UTC: %q", updatedAt)
	}
}
