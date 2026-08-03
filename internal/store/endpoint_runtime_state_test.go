// endpoint_runtime_state_test.go - 端点运行态 revision 乱序保护测试（§14.4）
// 2026-08-02 19:08:47

package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func createRuntimeStateTestStore(t *testing.T) (*SQLiteEndpointRuntimeStateStore, *sql.DB, func()) {
	t.Helper()
	db, cleanup := createTestDB(t)
	schema := `
		CREATE TABLE IF NOT EXISTS endpoint_runtime_states (
			endpoint_id INTEGER NOT NULL,
			scope TEXT NOT NULL CHECK (scope IN ('global', 'messages', 'count_tokens')),
			state TEXT NOT NULL DEFAULT 'active',
			cooldown_until DATETIME,
			cooldown_reason TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (endpoint_id, scope)
		);
	`
	if _, err := db.Exec(schema); err != nil {
		cleanup()
		t.Fatalf("创建 endpoint_runtime_states 表失败: %v", err)
	}
	return NewSQLiteEndpointRuntimeStateStore(db), db, cleanup
}

func runtimeStateRow(t *testing.T, db *sql.DB, endpointID int64, scope string) (state string, cooldownUntil sql.NullString, revision int64) {
	t.Helper()
	err := db.QueryRow(
		`SELECT state, cooldown_until, revision FROM endpoint_runtime_states WHERE endpoint_id = ? AND scope = ?`,
		endpointID, scope,
	).Scan(&state, &cooldownUntil, &revision)
	if err != nil {
		t.Fatalf("查询运行态失败: %v", err)
	}
	return state, cooldownUntil, revision
}

func cooldownRecord(endpointID int64, scope string, until time.Time, reason string, revision int64) *EndpointRuntimeStateRecord {
	return &EndpointRuntimeStateRecord{
		EndpointID:     endpointID,
		Scope:          scope,
		State:          "cooldown",
		CooldownUntil:  &until,
		CooldownReason: reason,
		Revision:       revision,
	}
}

// TestUpsertStaleCooldownDoesNotResurrectTombstone
// 场景：clear tombstone(rev2) 先落库，延迟的旧 persist(rev1) 后执行——冷却不得复活
func TestUpsertStaleCooldownDoesNotResurrectTombstone(t *testing.T) {
	s, db, cleanup := createRuntimeStateTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// tombstone：state=active、until=NULL、rev2
	if err := s.Upsert(ctx, &EndpointRuntimeStateRecord{
		EndpointID: 1, Scope: EndpointRuntimeScopeMessages, State: "active", Revision: 2,
	}); err != nil {
		t.Fatalf("写入 tombstone 失败: %v", err)
	}

	// 晚执行的旧 persist：rev1，必须被丢弃
	until := time.Now().Add(10 * time.Minute)
	if err := s.Upsert(ctx, cooldownRecord(1, EndpointRuntimeScopeMessages, until, "soft_failure_server_error", 1)); err != nil {
		t.Fatalf("旧 persist Upsert 报错: %v", err)
	}

	state, cooldownUntil, revision := runtimeStateRow(t, db, 1, EndpointRuntimeScopeMessages)
	if state != "active" || cooldownUntil.Valid || revision != 2 {
		t.Fatalf("tombstone 被旧 persist 覆盖: state=%s until=%v revision=%d", state, cooldownUntil, revision)
	}

	// 启动恢复口径：tombstone 不应出现在活跃冷却里
	records, err := s.ListActiveCooldowns(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListActiveCooldowns 失败: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("tombstone 不应被列为活跃冷却，实际返回 %d 条", len(records))
	}
}

// TestUpsertStaleTombstoneDoesNotClearNewCooldown
// 场景：新 persist(rev2) 先落库，延迟的旧 clear tombstone(rev1) 后执行——新冷却必须保留
func TestUpsertStaleTombstoneDoesNotClearNewCooldown(t *testing.T) {
	s, db, cleanup := createRuntimeStateTestStore(t)
	defer cleanup()
	ctx := context.Background()

	until := time.Now().Add(30 * time.Minute)
	if err := s.Upsert(ctx, cooldownRecord(1, EndpointRuntimeScopeGlobal, until, "auth_rejected", 2)); err != nil {
		t.Fatalf("写入新 cooldown 失败: %v", err)
	}

	// 晚执行的旧 clear：rev1 tombstone，必须被丢弃
	if err := s.Upsert(ctx, &EndpointRuntimeStateRecord{
		EndpointID: 1, Scope: EndpointRuntimeScopeGlobal, State: "active", Revision: 1,
	}); err != nil {
		t.Fatalf("旧 tombstone Upsert 报错: %v", err)
	}

	state, cooldownUntil, revision := runtimeStateRow(t, db, 1, EndpointRuntimeScopeGlobal)
	if state != "cooldown" || !cooldownUntil.Valid || revision != 2 {
		t.Fatalf("新 cooldown 被旧 tombstone 清除: state=%s until=%v revision=%d", state, cooldownUntil, revision)
	}

	records, err := s.ListActiveCooldowns(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListActiveCooldowns 失败: %v", err)
	}
	if len(records) != 1 || records[0].CooldownReason != "auth_rejected" {
		t.Fatalf("新 cooldown 应保留为活跃冷却，实际: %+v", records)
	}
}

// TestMaxRevision 播种口径：空表为 0，有数据取最大值
func TestMaxRevision(t *testing.T) {
	s, _, cleanup := createRuntimeStateTestStore(t)
	defer cleanup()
	ctx := context.Background()

	max, err := s.MaxRevision(ctx)
	if err != nil || max != 0 {
		t.Fatalf("空表 MaxRevision 应为 0: max=%d err=%v", max, err)
	}

	until := time.Now().Add(time.Minute)
	if err := s.Upsert(ctx, cooldownRecord(1, EndpointRuntimeScopeMessages, until, "soft_failure_connection", 7)); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}
	if err := s.Upsert(ctx, cooldownRecord(2, EndpointRuntimeScopeGlobal, until, "auth_rejected", 42)); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}

	max, err = s.MaxRevision(ctx)
	if err != nil || max != 42 {
		t.Fatalf("MaxRevision 应为 42: max=%d err=%v", max, err)
	}
}
