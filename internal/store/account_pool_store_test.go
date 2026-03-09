package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFindAccountByFingerprint_ReturnsAccount(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	record, err := st.CreateAccount(ctx, &UpstreamAccountRecord{
		ProviderType:                  "api_key",
		AccountName:                   "alpha",
		CredentialRaw:                 "sk-alpha",
		BaseURL:                       "https://api.openai.com",
		CostMultiplier:                1.6,
		InputCostMultiplier:           1.2,
		OutputCostMultiplier:          1.3,
		CacheCreationCostMultiplier:   1.4,
		CacheCreationCostMultiplier1h: 1.5,
		CacheReadCostMultiplier:       1.1,
		Priority:                      10,
		Enabled:                       true,
		State:                         "active",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	got, err := st.FindAccountByFingerprint(ctx, record.Fingerprint)
	if err != nil {
		t.Fatalf("FindAccountByFingerprint failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected account, got nil")
	}
	if got.ID != record.ID {
		t.Fatalf("expected account id %d, got %d", record.ID, got.ID)
	}
	if got.CostMultiplier != 1.6 || got.InputCostMultiplier != 1.2 || got.OutputCostMultiplier != 1.3 {
		t.Fatalf("unexpected multipliers: %+v", got)
	}
}

func TestUpdateAccountPriorities_UpdatesAllRequestedAccounts(t *testing.T) {
	st := newTestSQLiteAccountPoolStore(t)
	ctx := context.Background()

	first := mustCreateTestAccount(t, st, "first", 10)
	second := mustCreateTestAccount(t, st, "second", 20)
	third := mustCreateTestAccount(t, st, "third", 30)

	if err := st.UpdateAccountPriorities(ctx, map[int64]int{
		first.ID:  30,
		second.ID: 10,
		third.ID:  20,
	}); err != nil {
		t.Fatalf("UpdateAccountPriorities failed: %v", err)
	}

	accounts, err := st.ListAccounts(ctx, true)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(accounts))
	}

	if accounts[0].ID != second.ID || accounts[0].Priority != 10 {
		t.Fatalf("expected second account to become priority 10, got %+v", accounts[0])
	}
	if accounts[1].ID != third.ID || accounts[1].Priority != 20 {
		t.Fatalf("expected third account to become priority 20, got %+v", accounts[1])
	}
	if accounts[2].ID != first.ID || accounts[2].Priority != 30 {
		t.Fatalf("expected first account to become priority 30, got %+v", accounts[2])
	}
}

func newTestSQLiteAccountPoolStore(t *testing.T) *SQLiteAccountPoolStore {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schemaPath := filepath.Join("..", "tracking", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema failed: %v", err)
	}

	return NewSQLiteAccountPoolStore(db)
}

func mustCreateTestAccount(t *testing.T, st *SQLiteAccountPoolStore, name string, priority int) *UpstreamAccountRecord {
	t.Helper()

	record, err := st.CreateAccount(context.Background(), &UpstreamAccountRecord{
		ProviderType:  "api_key",
		AccountName:   name,
		CredentialRaw: "sk-" + name,
		BaseURL:       "https://api.openai.com",
		Priority:      priority,
		Enabled:       true,
		State:         "active",
	})
	if err != nil {
		t.Fatalf("CreateAccount(%s) failed: %v", name, err)
	}
	return record
}
