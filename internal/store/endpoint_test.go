package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// createTestDB 创建测试用的 SQLite 数据库
func createTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "endpoint_store_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("打开数据库失败: %v", err)
	}

	// 创建 endpoints 表
	schema := `
		CREATE TABLE IF NOT EXISTS endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel TEXT NOT NULL,
			name TEXT UNIQUE NOT NULL,
			url TEXT NOT NULL,
			token TEXT,
			api_key TEXT,
			headers TEXT,
			priority INTEGER DEFAULT 1,
			failover_enabled INTEGER DEFAULT 1,
			cooldown_seconds INTEGER,
			timeout_seconds INTEGER DEFAULT 300,
			supports_count_tokens INTEGER DEFAULT 0,
			cost_multiplier REAL DEFAULT 1.0,
			input_cost_multiplier REAL DEFAULT 1.0,
			output_cost_multiplier REAL DEFAULT 1.0,
			cache_creation_cost_multiplier REAL DEFAULT 1.0,
			cache_creation_cost_multiplier_1h REAL DEFAULT 1.0,
			cache_read_cost_multiplier REAL DEFAULT 1.0,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
			updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
		);
		CREATE INDEX IF NOT EXISTS idx_endpoints_channel ON endpoints(channel);
		CREATE INDEX IF NOT EXISTS idx_endpoints_priority ON endpoints(priority);
		CREATE INDEX IF NOT EXISTS idx_endpoints_enabled ON endpoints(enabled);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("创建表失败: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// TestCreate 测试创建端点
func TestCreate(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	record := &EndpointRecord{
		Channel:         "test-channel",
		Name:            "test-endpoint",
		URL:             "https://api.example.com",
		Token:           "sk-test-token",
		Priority:        1,
		FailoverEnabled: true,
		TimeoutSeconds:  300,
		Enabled:         true,
	}

	created, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("创建端点失败: %v", err)
	}

	if created.ID == 0 {
		t.Error("创建后 ID 应该不为 0")
	}

	if created.Name != "test-endpoint" {
		t.Errorf("Name 不匹配: got %s, want test-endpoint", created.Name)
	}

	// 验证默认成本倍率
	if created.CostMultiplier != 1.0 {
		t.Errorf("CostMultiplier 应为 1.0, got %f", created.CostMultiplier)
	}
}

// TestGet 测试获取端点
func TestGet(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 先创建
	record := &EndpointRecord{
		Channel:         "test-channel",
		Name:            "get-test",
		URL:             "https://api.example.com",
		Token:           "sk-token",
		Priority:        2,
		FailoverEnabled: true,
		TimeoutSeconds:  600,
		Enabled:         true,
		Headers:         map[string]string{"X-Custom": "value"},
	}

	_, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("创建端点失败: %v", err)
	}

	// 获取
	got, err := store.Get(ctx, "get-test")
	if err != nil {
		t.Fatalf("获取端点失败: %v", err)
	}

	if got == nil {
		t.Fatal("获取的端点不应为 nil")
	}

	if got.Name != "get-test" {
		t.Errorf("Name 不匹配: got %s, want get-test", got.Name)
	}

	if got.Priority != 2 {
		t.Errorf("Priority 不匹配: got %d, want 2", got.Priority)
	}

	if got.Headers["X-Custom"] != "value" {
		t.Errorf("Headers 不匹配: got %v", got.Headers)
	}
}

// TestGetNotFound 测试获取不存在的端点
func TestGetNotFound(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	got, err := store.Get(ctx, "non-existent")
	if err != nil {
		t.Fatalf("获取不存在的端点不应报错: %v", err)
	}

	if got != nil {
		t.Error("不存在的端点应返回 nil")
	}
}

// TestList 测试列出所有端点
func TestList(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 创建多个端点
	endpoints := []*EndpointRecord{
		{Channel: "ch1", Name: "ep1", URL: "https://api1.com", Priority: 2, FailoverEnabled: true, Enabled: true},
		{Channel: "ch1", Name: "ep2", URL: "https://api2.com", Priority: 1, FailoverEnabled: true, Enabled: true},
		{Channel: "ch2", Name: "ep3", URL: "https://api3.com", Priority: 1, FailoverEnabled: false, Enabled: false},
	}

	for _, ep := range endpoints {
		_, err := store.Create(ctx, ep)
		if err != nil {
			t.Fatalf("创建端点失败: %v", err)
		}
	}

	// 列出
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("列出端点失败: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("端点数量不匹配: got %d, want 3", len(list))
	}

	// 验证排序（按 priority ASC）
	if list[0].Name != "ep2" && list[0].Name != "ep3" {
		t.Errorf("排序错误: 第一个应该是 priority=1 的端点, got %s", list[0].Name)
	}
}

// TestUpdate 测试更新端点
func TestUpdate(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 先创建
	record := &EndpointRecord{
		Channel:         "test-channel",
		Name:            "update-test",
		URL:             "https://api.example.com",
		Priority:        1,
		FailoverEnabled: true,
		Enabled:         true,
	}

	_, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("创建端点失败: %v", err)
	}

	// 等待一点时间确保 updated_at 会变化
	time.Sleep(10 * time.Millisecond)

	// 更新
	record.URL = "https://new-api.example.com"
	record.Priority = 5
	record.CostMultiplier = 1.5

	err = store.Update(ctx, record)
	if err != nil {
		t.Fatalf("更新端点失败: %v", err)
	}

	// 验证更新
	got, err := store.Get(ctx, "update-test")
	if err != nil {
		t.Fatalf("获取更新后的端点失败: %v", err)
	}

	if got.URL != "https://new-api.example.com" {
		t.Errorf("URL 未更新: got %s", got.URL)
	}

	if got.Priority != 5 {
		t.Errorf("Priority 未更新: got %d, want 5", got.Priority)
	}

	if got.CostMultiplier != 1.5 {
		t.Errorf("CostMultiplier 未更新: got %f, want 1.5", got.CostMultiplier)
	}
}

// TestDelete 测试删除端点
func TestDelete(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 先创建
	record := &EndpointRecord{
		Channel:         "test-channel",
		Name:            "delete-test",
		URL:             "https://api.example.com",
		FailoverEnabled: true,
		Enabled:         true,
	}

	_, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("创建端点失败: %v", err)
	}

	// 删除
	err = store.Delete(ctx, "delete-test")
	if err != nil {
		t.Fatalf("删除端点失败: %v", err)
	}

	// 验证删除
	got, err := store.Get(ctx, "delete-test")
	if err != nil {
		t.Fatalf("获取已删除端点不应报错: %v", err)
	}

	if got != nil {
		t.Error("已删除的端点应返回 nil")
	}
}

// TestDeleteNotFound 测试删除不存在的端点
func TestDeleteNotFound(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	err := store.Delete(ctx, "non-existent")
	if err == nil {
		t.Error("删除不存在的端点应该报错")
	}
}

// TestBatchCreate 测试批量创建
func TestBatchCreate(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	records := []*EndpointRecord{
		{Channel: "batch", Name: "batch1", URL: "https://api1.com", FailoverEnabled: true, Enabled: true},
		{Channel: "batch", Name: "batch2", URL: "https://api2.com", FailoverEnabled: true, Enabled: true},
		{Channel: "batch", Name: "batch3", URL: "https://api3.com", FailoverEnabled: false, Enabled: true},
	}

	err := store.BatchCreate(ctx, records)
	if err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	// 验证
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("获取数量失败: %v", err)
	}

	if count != 3 {
		t.Errorf("数量不匹配: got %d, want 3", count)
	}
}

// TestBatchDelete 测试批量删除
func TestBatchDelete(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 先批量创建
	records := []*EndpointRecord{
		{Channel: "batch", Name: "del1", URL: "https://api1.com", FailoverEnabled: true, Enabled: true},
		{Channel: "batch", Name: "del2", URL: "https://api2.com", FailoverEnabled: true, Enabled: true},
		{Channel: "batch", Name: "del3", URL: "https://api3.com", FailoverEnabled: true, Enabled: true},
	}

	err := store.BatchCreate(ctx, records)
	if err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	// 批量删除前两个
	err = store.BatchDelete(ctx, []string{"del1", "del2"})
	if err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}

	// 验证
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("获取数量失败: %v", err)
	}

	if count != 1 {
		t.Errorf("删除后数量不匹配: got %d, want 1", count)
	}

	// 验证剩余的是 del3
	got, err := store.Get(ctx, "del3")
	if err != nil {
		t.Fatalf("获取剩余端点失败: %v", err)
	}

	if got == nil {
		t.Error("del3 不应该被删除")
	}
}

// TestListByChannel 测试按渠道列出
func TestListByChannel(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 创建不同渠道的端点
	records := []*EndpointRecord{
		{Channel: "channel-a", Name: "a1", URL: "https://a1.com", FailoverEnabled: true, Enabled: true},
		{Channel: "channel-a", Name: "a2", URL: "https://a2.com", FailoverEnabled: true, Enabled: true},
		{Channel: "channel-b", Name: "b1", URL: "https://b1.com", FailoverEnabled: true, Enabled: true},
	}

	err := store.BatchCreate(ctx, records)
	if err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	// 按渠道列出
	list, err := store.ListByChannel(ctx, "channel-a")
	if err != nil {
		t.Fatalf("按渠道列出失败: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("渠道 a 端点数量不匹配: got %d, want 2", len(list))
	}
}

// TestListEnabled 测试列出启用的端点
func TestListEnabled(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 创建混合状态的端点
	records := []*EndpointRecord{
		{Channel: "ch", Name: "enabled1", URL: "https://e1.com", FailoverEnabled: true, Enabled: true},
		{Channel: "ch", Name: "disabled1", URL: "https://d1.com", FailoverEnabled: true, Enabled: false},
		{Channel: "ch", Name: "enabled2", URL: "https://e2.com", FailoverEnabled: true, Enabled: true},
	}

	err := store.BatchCreate(ctx, records)
	if err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	// 列出启用的
	list, err := store.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("列出启用端点失败: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("启用端点数量不匹配: got %d, want 2", len(list))
	}

	for _, ep := range list {
		if !ep.Enabled {
			t.Errorf("列出的端点应该都是启用的: %s", ep.Name)
		}
	}
}

// TestCostMultipliers 测试成本倍率
func TestCostMultipliers(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	record := &EndpointRecord{
		Channel:                     "test",
		Name:                        "cost-test",
		URL:                         "https://api.example.com",
		FailoverEnabled:             true,
		Enabled:                     true,
		CostMultiplier:              0.8,
		InputCostMultiplier:         1.2,
		OutputCostMultiplier:        1.5,
		CacheCreationCostMultiplier: 0.5,
		CacheReadCostMultiplier:     0.1,
	}

	_, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("创建端点失败: %v", err)
	}

	got, err := store.Get(ctx, "cost-test")
	if err != nil {
		t.Fatalf("获取端点失败: %v", err)
	}

	if got.CostMultiplier != 0.8 {
		t.Errorf("CostMultiplier 不匹配: got %f, want 0.8", got.CostMultiplier)
	}

	if got.InputCostMultiplier != 1.2 {
		t.Errorf("InputCostMultiplier 不匹配: got %f, want 1.2", got.InputCostMultiplier)
	}

	if got.OutputCostMultiplier != 1.5 {
		t.Errorf("OutputCostMultiplier 不匹配: got %f, want 1.5", got.OutputCostMultiplier)
	}

	if got.CacheCreationCostMultiplier != 0.5 {
		t.Errorf("CacheCreationCostMultiplier 不匹配: got %f, want 0.5", got.CacheCreationCostMultiplier)
	}

	if got.CacheReadCostMultiplier != 0.1 {
		t.Errorf("CacheReadCostMultiplier 不匹配: got %f, want 0.1", got.CacheReadCostMultiplier)
	}
}

// TestCooldownSeconds 测试冷却时间
func TestCooldownSeconds(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	cooldown := 120
	record := &EndpointRecord{
		Channel:         "test",
		Name:            "cooldown-test",
		URL:             "https://api.example.com",
		FailoverEnabled: true,
		Enabled:         true,
		CooldownSeconds: &cooldown,
	}

	_, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("创建端点失败: %v", err)
	}

	got, err := store.Get(ctx, "cooldown-test")
	if err != nil {
		t.Fatalf("获取端点失败: %v", err)
	}

	if got.CooldownSeconds == nil {
		t.Error("CooldownSeconds 不应为 nil")
	} else if *got.CooldownSeconds != 120 {
		t.Errorf("CooldownSeconds 不匹配: got %d, want 120", *got.CooldownSeconds)
	}
}

// TestCount 测试计数
func TestCount(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	// 初始为空
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("获取数量失败: %v", err)
	}

	if count != 0 {
		t.Errorf("初始数量应为 0, got %d", count)
	}

	// 添加一些端点
	records := []*EndpointRecord{
		{Channel: "ch", Name: "c1", URL: "https://c1.com", FailoverEnabled: true, Enabled: true},
		{Channel: "ch", Name: "c2", URL: "https://c2.com", FailoverEnabled: true, Enabled: true},
	}

	err = store.BatchCreate(ctx, records)
	if err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	count, err = store.Count(ctx)
	if err != nil {
		t.Fatalf("获取数量失败: %v", err)
	}

	if count != 2 {
		t.Errorf("数量应为 2, got %d", count)
	}
}

// TestDuplicateName 测试重复名称
func TestDuplicateName(t *testing.T) {
	db, cleanup := createTestDB(t)
	defer cleanup()

	store := NewSQLiteEndpointStore(db)
	ctx := context.Background()

	record := &EndpointRecord{
		Channel:         "test",
		Name:            "duplicate",
		URL:             "https://api.example.com",
		FailoverEnabled: true,
		Enabled:         true,
	}

	_, err := store.Create(ctx, record)
	if err != nil {
		t.Fatalf("第一次创建失败: %v", err)
	}

	// 尝试创建同名端点
	_, err = store.Create(ctx, record)
	if err == nil {
		t.Error("创建重复名称的端点应该失败")
	}
}
