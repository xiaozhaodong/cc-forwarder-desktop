package tracking

import (
	"testing"
)

// TestHotPoolUpdateHitsArchivingMap CompleteAndArchive 后 Update 必须命中归档副本，
// 供终态快照覆盖写随批量 INSERT 落库，而不是退化为异步 DB UPDATE。
func TestHotPoolUpdateHitsArchivingMap(t *testing.T) {
	pool := NewHotPool(DefaultHotPoolConfig())
	t.Cleanup(func() { _ = pool.Close() })

	if err := pool.Add(&ActiveRequest{RequestID: "req-archiving-update"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := pool.CompleteAndArchive("req-archiving-update", nil); err != nil {
		t.Fatalf("CompleteAndArchive failed: %v", err)
	}

	status := "completed"
	if err := pool.Update("req-archiving-update", func(req *ActiveRequest) { req.Status = status }); err != nil {
		t.Fatalf("expected update to hit archiving map, got %v", err)
	}

	snapshot, ok := pool.SnapshotRequest("req-archiving-update")
	if !ok {
		t.Fatal("expected request readable from archiving map")
	}
	if snapshot.Status != "completed" {
		t.Fatalf("expected completed status in archiving copy, got %q", snapshot.Status)
	}
}

// TestHotPoolUpdateFallsBackAfterArchiveCommit 覆盖归档事务已经提交、
// ConfirmArchived 尚未清理 archiving map 的提交边界窗口。
func TestHotPoolUpdateFallsBackAfterArchiveCommit(t *testing.T) {
	pool := NewHotPool(DefaultHotPoolConfig())
	t.Cleanup(func() { _ = pool.Close() })

	const requestID = "req-archive-persisted"
	if err := pool.Add(&ActiveRequest{RequestID: requestID}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := pool.CompleteAndArchive(requestID, nil); err != nil {
		t.Fatalf("CompleteAndArchive failed: %v", err)
	}

	pool.mu.RLock()
	req := pool.archiving[requestID]
	pool.mu.RUnlock()
	if req == nil {
		t.Fatal("expected request in archiving map")
	}
	req.mu.Lock()
	req.archivePersisted = true
	req.mu.Unlock()

	updaterCalled := false
	if err := pool.Update(requestID, func(req *ActiveRequest) {
		updaterCalled = true
		req.Status = "completed"
	}); err == nil {
		t.Fatal("expected persisted archive update to fall back to database")
	}
	if updaterCalled {
		t.Fatal("updater must not mutate an already persisted archive copy")
	}
	if snapshot, ok := pool.SnapshotRequest(requestID); ok || snapshot != nil {
		t.Fatalf("expected persisted archiving copy hidden behind database source, got %+v", snapshot)
	}
}
