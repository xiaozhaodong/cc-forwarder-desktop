package tracking

import (
	"sync"
	"testing"
	"time"
)

func TestNewHotPool(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)
	defer pool.Close()

	if pool == nil {
		t.Fatal("NewHotPool returned nil")
	}

	if pool.GetActiveCount() != 0 {
		t.Errorf("Expected 0 active requests, got %d", pool.GetActiveCount())
	}
}

func TestHotPoolAdd(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)
	defer pool.Close()

	req := NewActiveRequest("req-001", "127.0.0.1", "test-agent", "POST", "/v1/messages", false)
	err := pool.Add(req)
	if err != nil {
		t.Fatalf("Failed to add request: %v", err)
	}

	if pool.GetActiveCount() != 1 {
		t.Errorf("Expected 1 active request, got %d", pool.GetActiveCount())
	}

	// Test duplicate add
	err = pool.Add(req)
	if err == nil {
		t.Error("Expected error when adding duplicate request")
	}
}

func TestHotPoolUpdate(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)
	defer pool.Close()

	req := NewActiveRequest("req-002", "127.0.0.1", "test-agent", "POST", "/v1/messages", true)
	pool.Add(req)

	// Update the request
	err := pool.Update("req-002", func(r *ActiveRequest) {
		r.Status = "forwarding"
		r.EndpointName = "primary"
		r.RetryCount = 1
	})
	if err != nil {
		t.Fatalf("Failed to update request: %v", err)
	}

	// Verify update
	updated, exists := pool.Get("req-002")
	if !exists {
		t.Fatal("Request not found after update")
	}
	if updated.Status != "forwarding" {
		t.Errorf("Expected status 'forwarding', got '%s'", updated.Status)
	}
	if updated.EndpointName != "primary" {
		t.Errorf("Expected endpoint 'primary', got '%s'", updated.EndpointName)
	}
	if updated.RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", updated.RetryCount)
	}

	// Test update non-existent request
	err = pool.Update("non-existent", func(r *ActiveRequest) {})
	if err == nil {
		t.Error("Expected error when updating non-existent request")
	}
}

func TestHotPoolRemove(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)
	defer pool.Close()

	req := NewActiveRequest("req-003", "127.0.0.1", "test-agent", "POST", "/v1/messages", false)
	pool.Add(req)

	removed := pool.Remove("req-003")
	if removed == nil {
		t.Fatal("Remove returned nil")
	}
	if removed.RequestID != "req-003" {
		t.Errorf("Expected request ID 'req-003', got '%s'", removed.RequestID)
	}

	if pool.GetActiveCount() != 0 {
		t.Errorf("Expected 0 active requests after remove, got %d", pool.GetActiveCount())
	}

	// Test remove non-existent
	removed = pool.Remove("non-existent")
	if removed != nil {
		t.Error("Expected nil when removing non-existent request")
	}
}

func TestHotPoolCompleteAndArchive(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)
	defer pool.Close()

	var archivedReq *ActiveRequest
	pool.SetArchiveCallback(func(req *ActiveRequest) {
		archivedReq = req
	})

	req := NewActiveRequest("req-004", "127.0.0.1", "test-agent", "POST", "/v1/messages", true)
	pool.Add(req)

	err := pool.CompleteAndArchive("req-004", func(r *ActiveRequest) {
		r.Status = "completed"
		r.ModelName = "claude-3-sonnet"
		r.InputTokens = 100
		r.OutputTokens = 200
	})
	if err != nil {
		t.Fatalf("Failed to complete and archive: %v", err)
	}

	// Verify archived
	if archivedReq == nil {
		t.Fatal("Archive callback was not called")
	}
	if archivedReq.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", archivedReq.Status)
	}
	if archivedReq.ModelName != "claude-3-sonnet" {
		t.Errorf("Expected model 'claude-3-sonnet', got '%s'", archivedReq.ModelName)
	}
	if archivedReq.EndTime == nil {
		t.Error("Expected end time to be set")
	}

	// 请求现在在归档缓存中（等待数据库写入确认）
	// GetActiveCount() 返回 requests + archiving 的总数
	if pool.GetActiveCount() != 1 {
		t.Errorf("Expected 1 request in archiving, got %d", pool.GetActiveCount())
	}
	if pool.GetArchivingCount() != 1 {
		t.Errorf("Expected 1 request in archiving cache, got %d", pool.GetArchivingCount())
	}

	// 模拟数据库写入成功后的确认
	pool.ConfirmArchived([]string{"req-004"})

	// 确认后应该没有请求了
	if pool.GetActiveCount() != 0 {
		t.Errorf("Expected 0 active requests after confirm, got %d", pool.GetActiveCount())
	}
}

func TestHotPoolConcurrentAccess(t *testing.T) {
	config := DefaultHotPoolConfig()
	config.MaxSize = 1000
	pool := NewHotPool(config)
	defer pool.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOpsPerGoroutine := 10

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				reqID := time.Now().UnixNano()
				req := NewActiveRequest(
					string(rune(reqID)),
					"127.0.0.1",
					"test-agent",
					"POST",
					"/v1/messages",
					false,
				)
				pool.Add(req)
			}
		}(i)
	}

	wg.Wait()

	// Should have some requests (exact count depends on timing)
	count := pool.GetActiveCount()
	t.Logf("Concurrent test: %d requests in pool", count)
}

func TestHotPoolMaxSize(t *testing.T) {
	config := HotPoolConfig{
		MaxAge:          30 * time.Minute,
		MaxSize:         5,
		CleanupInterval: 1 * time.Hour, // Long interval to prevent cleanup
	}
	pool := NewHotPool(config)
	defer pool.Close()

	// Add up to max size
	for i := 0; i < 5; i++ {
		req := NewActiveRequest(string(rune('a'+i)), "127.0.0.1", "test", "POST", "/", false)
		err := pool.Add(req)
		if err != nil {
			t.Fatalf("Failed to add request %d: %v", i, err)
		}
	}

	// Next add should fail
	req := NewActiveRequest("overflow", "127.0.0.1", "test", "POST", "/", false)
	err := pool.Add(req)
	if err == nil {
		t.Error("Expected error when exceeding max size")
	}

	stats := pool.GetStats()
	if stats.TotalOverflow != 1 {
		t.Errorf("Expected 1 overflow, got %d", stats.TotalOverflow)
	}
}

func TestHotPoolStats(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)
	defer pool.Close()

	pool.SetArchiveCallback(func(req *ActiveRequest) {})

	// Add some requests
	for i := 0; i < 3; i++ {
		req := NewActiveRequest(string(rune('a'+i)), "127.0.0.1", "test", "POST", "/", false)
		pool.Add(req)
	}

	// Archive one
	pool.CompleteAndArchive("a", func(r *ActiveRequest) {
		r.Status = "completed"
	})

	stats := pool.GetStats()

	if stats.TotalAdded != 3 {
		t.Errorf("Expected TotalAdded=3, got %d", stats.TotalAdded)
	}
	if stats.TotalRemoved != 1 {
		t.Errorf("Expected TotalRemoved=1, got %d", stats.TotalRemoved)
	}
	if stats.TotalArchived != 1 {
		t.Errorf("Expected TotalArchived=1, got %d", stats.TotalArchived)
	}
	if stats.CurrentSize != 2 {
		t.Errorf("Expected CurrentSize=2, got %d", stats.CurrentSize)
	}
	if stats.PeakSize != 3 {
		t.Errorf("Expected PeakSize=3, got %d", stats.PeakSize)
	}
}

func TestHotPoolClose(t *testing.T) {
	config := DefaultHotPoolConfig()
	pool := NewHotPool(config)

	var archived []*ActiveRequest
	pool.SetArchiveCallback(func(req *ActiveRequest) {
		archived = append(archived, req)
	})

	// Add some requests
	for i := 0; i < 3; i++ {
		req := NewActiveRequest(string(rune('a'+i)), "127.0.0.1", "test", "POST", "/", false)
		pool.Add(req)
	}

	// Close should archive remaining
	pool.Close()

	if len(archived) != 3 {
		t.Errorf("Expected 3 archived on close, got %d", len(archived))
	}

	// Add after close should fail
	req := NewActiveRequest("new", "127.0.0.1", "test", "POST", "/", false)
	err := pool.Add(req)
	if err == nil {
		t.Error("Expected error when adding to closed pool")
	}
}

func TestHotPoolCleanup_ArchiveCallbackRunsOutsideLock(t *testing.T) {
	config := DefaultHotPoolConfig()
	config.MaxAge = time.Millisecond
	config.ArchiveOnCleanup = true
	pool := NewHotPool(config)
	defer pool.Close()

	req := NewActiveRequest("req-cleanup", "127.0.0.1", "test", "POST", "/", false)
	req.StartTime = time.Now().Add(-2 * time.Minute)
	if err := pool.Add(req); err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	done := make(chan struct{})
	pool.SetArchiveCallback(func(req *ActiveRequest) {
		_ = pool.GetActiveCount()
		close(done)
	})

	go pool.cleanup()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("archive callback did not finish; cleanup may still be holding the pool lock")
	}
}

func TestNewActiveRequest(t *testing.T) {
	req := NewActiveRequest("req-test", "192.168.1.1", "Mozilla/5.0", "POST", "/v1/messages", true)

	if req.RequestID != "req-test" {
		t.Errorf("Expected RequestID 'req-test', got '%s'", req.RequestID)
	}
	if req.ClientIP != "192.168.1.1" {
		t.Errorf("Expected ClientIP '192.168.1.1', got '%s'", req.ClientIP)
	}
	if req.UserAgent != "Mozilla/5.0" {
		t.Errorf("Expected UserAgent 'Mozilla/5.0', got '%s'", req.UserAgent)
	}
	if req.Method != "POST" {
		t.Errorf("Expected Method 'POST', got '%s'", req.Method)
	}
	if req.Path != "/v1/messages" {
		t.Errorf("Expected Path '/v1/messages', got '%s'", req.Path)
	}
	if !req.IsStreaming {
		t.Error("Expected IsStreaming to be true")
	}
	if req.Status != "pending" {
		t.Errorf("Expected Status 'pending', got '%s'", req.Status)
	}
	if req.StartTime.IsZero() {
		t.Error("Expected StartTime to be set")
	}
}
