package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// CoreDatabase 是应用配置与业务存储使用的常驻 SQLite 连接。
// 它不受 usage_tracking.enabled 控制；UsageTracker 仅负责请求追踪生命周期。
type CoreDatabase struct {
	mu      sync.RWMutex
	adapter DatabaseAdapter
}

func OpenCoreDatabase(config DatabaseConfig) (*CoreDatabase, error) {
	adapter, err := NewDatabaseAdapter(config)
	if err != nil {
		return nil, fmt.Errorf("create core database adapter: %w", err)
	}
	if err := adapter.Open(); err != nil {
		return nil, fmt.Errorf("open core database: %w", err)
	}
	return &CoreDatabase{adapter: adapter}, nil
}

// InitSchema 必须在一次性启动迁移成功后调用。
func (c *CoreDatabase) InitSchema() error {
	if c == nil {
		return fmt.Errorf("core database is nil")
	}
	c.mu.RLock()
	adapter := c.adapter
	c.mu.RUnlock()
	if adapter == nil {
		return fmt.Errorf("core database is closed")
	}
	if err := adapter.InitSchema(); err != nil {
		return fmt.Errorf("initialize core database schema: %w", err)
	}
	return nil
}

func (c *CoreDatabase) DB() *sql.DB {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.adapter == nil {
		return nil
	}
	return c.adapter.GetDB()
}

func (c *CoreDatabase) Ping(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("core database is nil")
	}
	c.mu.RLock()
	adapter := c.adapter
	c.mu.RUnlock()
	if adapter == nil {
		return fmt.Errorf("core database is closed")
	}
	return adapter.Ping(ctx)
}

func (c *CoreDatabase) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	adapter := c.adapter
	c.adapter = nil
	c.mu.Unlock()
	if adapter == nil {
		return nil
	}
	return adapter.Close()
}
