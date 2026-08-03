package migration

import (
	"context"
	"testing"
)

func TestRequestHistoryMigrationUsesPathBeforeUpstreamType(t *testing.T) {
	db := openLegacyFixtureDB(t)
	defer db.Close()
	if _, err := db.Exec(`UPDATE request_logs SET upstream_type = 'endpoint' WHERE request_id = 'req-fixture-codex'`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrateRequestHistory(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertRequestFamily(t, db, "req-fixture-codex", "codex", "endpoint", "codex-fixture")
}

func TestRequestHistoryMigrationRecognizesStandaloneCodexSearch(t *testing.T) {
	db := openLegacyFixtureDB(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO request_logs (request_id, path, start_time, status, upstream_type)
		VALUES ('req-search', '/v1/alpha/search', CURRENT_TIMESTAMP, 'completed', '')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrateRequestHistory(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertRequestFamily(t, db, "req-search", "codex", "account", "")
}
