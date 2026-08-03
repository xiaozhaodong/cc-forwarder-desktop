package tracking

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCoreDatabaseOpensWithoutUsageTracker(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.db")
	core, err := OpenCoreDatabase(DatabaseConfig{Type: "sqlite", DatabasePath: dbPath, Timezone: "Asia/Shanghai"})
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
