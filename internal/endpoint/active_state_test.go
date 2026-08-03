package endpoint

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/store"
)

// fakeEndpointStore 可注入失败的 EndpointStore 测试替身
type fakeEndpointStore struct {
	mu            sync.Mutex
	enabledByName map[string]bool
	activateErr   error
	setEnabledErr error
	deleteErr     error
	onActivate    func(name string)
	activateLog   []string
	deleteLog     []string
}

func newFakeEndpointStore(names ...string) *fakeEndpointStore {
	enabled := make(map[string]bool, len(names))
	for _, name := range names {
		enabled[name] = false
	}
	return &fakeEndpointStore{enabledByName: enabled}
}

func (f *fakeEndpointStore) setActivateErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activateErr = err
}

func (f *fakeEndpointStore) setDeleteErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteErr = err
}

func (f *fakeEndpointStore) enabledSnapshot() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	snapshot := make(map[string]bool, len(f.enabledByName))
	for name, enabled := range f.enabledByName {
		snapshot[name] = enabled
	}
	return snapshot
}

func (f *fakeEndpointStore) activateHistory() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.activateLog...)
}

func (f *fakeEndpointStore) ActivateExclusive(_ context.Context, name string) error {
	f.mu.Lock()
	hook := f.onActivate
	f.mu.Unlock()
	if hook != nil {
		hook(name)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.activateErr != nil {
		return f.activateErr
	}
	if _, ok := f.enabledByName[name]; !ok {
		return fmt.Errorf("端点不存在: %s", name)
	}
	for key := range f.enabledByName {
		f.enabledByName[key] = key == name
	}
	f.activateLog = append(f.activateLog, name)
	return nil
}

func (f *fakeEndpointStore) SetAvailabilityEnabled(_ context.Context, name string, enabled bool) error {
	return nil
}

func (f *fakeEndpointStore) SetFailoverEnabled(_ context.Context, name string, enabled bool) error {
	return nil
}

func (f *fakeEndpointStore) SetEnabled(_ context.Context, name string, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setEnabledErr != nil {
		return f.setEnabledErr
	}
	if _, ok := f.enabledByName[name]; !ok {
		return fmt.Errorf("端点不存在: %s", name)
	}
	f.enabledByName[name] = enabled
	return nil
}

func (f *fakeEndpointStore) Delete(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.enabledByName[name]; !ok {
		return fmt.Errorf("端点不存在: %s", name)
	}
	delete(f.enabledByName, name)
	f.deleteLog = append(f.deleteLog, name)
	return nil
}

func (f *fakeEndpointStore) Create(context.Context, *store.EndpointRecord) (*store.EndpointRecord, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeEndpointStore) Get(context.Context, string) (*store.EndpointRecord, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeEndpointStore) GetByID(context.Context, int64) (*store.EndpointRecord, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeEndpointStore) List(context.Context) ([]*store.EndpointRecord, error) { return nil, nil }
func (f *fakeEndpointStore) Update(context.Context, *store.EndpointRecord) error {
	return errors.New("not implemented")
}
func (f *fakeEndpointStore) BatchCreate(context.Context, []*store.EndpointRecord) error { return nil }
func (f *fakeEndpointStore) BatchDelete(context.Context, []string) error                { return nil }
func (f *fakeEndpointStore) ListByChannel(context.Context, string) ([]*store.EndpointRecord, error) {
	return nil, nil
}
func (f *fakeEndpointStore) ListEnabled(context.Context) ([]*store.EndpointRecord, error) {
	return nil, nil
}
func (f *fakeEndpointStore) Count(context.Context) (int, error) { return 0, nil }
func (f *fakeEndpointStore) WithTx(*sql.Tx) store.EndpointStore { return f }

func newActiveStateTestManager(t *testing.T, st store.EndpointStore, endpointNames ...string) *Manager {
	t.Helper()
	cfg := &config.Config{
		Failover: config.FailoverConfig{Enabled: true},
	}
	for _, name := range endpointNames {
		cfg.Endpoints = append(cfg.Endpoints, config.EndpointConfig{
			Name: name, URL: "http://" + name + ".test", Priority: len(cfg.Endpoints) + 1,
		})
	}
	manager := NewManager(cfg)
	t.Cleanup(manager.Stop)

	if st != nil {
		writer := NewRuntimeWriter(st, manager.IsCurrentActiveRevision)
		manager.SetRuntimeWriter(writer)
		t.Cleanup(func() { _ = writer.Close() })
	}
	return manager
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waitUntil timeout: %s", message)
}

func TestRuntimeWriter_StaleAutoTaskSilentlyDropped(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	current := int64(3)
	writer := NewRuntimeWriter(st, func(rev int64) bool { return rev == current })
	defer writer.Close()

	// 乱序：高 revision 任务先到，旧 revision 任务后到
	writer.EnqueueAuto(endpointTaskActivate, "b", 3)
	writer.EnqueueAuto(endpointTaskActivate, "a", 2)

	waitUntil(t, 2*time.Second, func() bool {
		return len(st.activateHistory()) >= 1
	}, "activate task executed")
	time.Sleep(50 * time.Millisecond)

	history := st.activateHistory()
	if len(history) != 1 || history[0] != "b" {
		t.Fatalf("expected only current-revision task executed, got %v", history)
	}
	if enabled := st.enabledSnapshot(); !enabled["b"] || enabled["a"] {
		t.Fatalf("expected DB to reflect highest revision, got %v", enabled)
	}
}

func TestRuntimeWriter_ManualSupersededGetsAck(t *testing.T) {
	st := newFakeEndpointStore("a")
	writer := NewRuntimeWriter(st, func(int64) bool { return false })
	defer writer.Close()

	err := writer.EnqueueManual(endpointTaskActivate, "a", 1)
	if !errors.Is(err, ErrSuperseded) {
		t.Fatalf("expected ErrSuperseded for stale manual task, got %v", err)
	}
	if len(st.activateHistory()) != 0 {
		t.Fatal("stale manual task must not touch DB")
	}
}

func TestRuntimeWriter_CloseFlushesPending(t *testing.T) {
	st := newFakeEndpointStore("a")
	writer := NewRuntimeWriter(st, func(int64) bool { return true })

	writer.EnqueueAuto(endpointTaskActivate, "a", 1)
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if enabled := st.enabledSnapshot(); !enabled["a"] {
		t.Fatal("expected pending task flushed on close")
	}
}

func TestRuntimeWriter_CloseWaitsForInFlightSubmitBeforeDrain(t *testing.T) {
	st := newFakeEndpointStore("a")
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var blockFirst sync.Once
	st.onActivate = func(string) {
		blockFirst.Do(func() {
			close(firstStarted)
			<-releaseFirst
		})
	}

	writer := NewRuntimeWriter(st, func(int64) bool { return true })
	writer.EnqueueAuto(endpointTaskActivate, "a", 1)
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("writer did not start the blocking task")
	}

	// 消费者阻塞时填满队列，让下一次手动提交卡在 channel send，
	// 同时持有 lifecycleMu.RLock，确保 Close 不能越过它先取消消费者。
	for i := 0; i < cap(writer.tasks); i++ {
		writer.EnqueueAuto(endpointTaskActivate, "a", int64(i+2))
	}
	manualDone := make(chan error, 1)
	go func() {
		manualDone <- writer.EnqueueManual(endpointTaskActivate, "a", 1000)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if writer.lifecycleMu.TryLock() {
			writer.lifecycleMu.Unlock()
			if time.Now().After(deadline) {
				t.Fatal("manual submit did not enter the lifecycle gate")
			}
			time.Sleep(time.Millisecond)
			continue
		}
		break
	}

	closeStarted := make(chan struct{})
	closeDone := make(chan error, 1)
	go func() {
		close(closeStarted)
		closeDone <- writer.Close()
	}()
	<-closeStarted
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before the in-flight submit could enqueue: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	select {
	case err := <-manualDone:
		if err != nil {
			t.Fatalf("in-flight manual submit must receive its ACK during drain: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual submit remained blocked waiting for ACK")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish after draining the in-flight submit")
	}
}

func TestActiveState_ManualActivatePersistsAndMatchesMemory(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	manager := newActiveStateTestManager(t, st, "a", "b")

	if err := manager.ActivateEndpointManually("b"); err != nil {
		t.Fatalf("manual activate failed: %v", err)
	}

	active, _ := manager.GetActiveEndpointSelection()
	if active != "b" {
		t.Fatalf("expected memory active=b, got %q", active)
	}
	if enabled := st.enabledSnapshot(); !enabled["b"] || enabled["a"] {
		t.Fatalf("expected DB active=b after ACK, got %v", enabled)
	}
	name, _, established := manager.runtimeWriter.LastPersisted()
	if !established || name != "b" {
		t.Fatalf("expected lastPersisted=b established, got %q established=%v", name, established)
	}
}

// A/B/C 反例（§4.3）：DB=A，自动迁移内存=B（任务写库失败未落库），
// 手动 C 使 B 任务过期，C 写库失败 → 必须回滚到 lastPersisted=A，而非操作前内存值 B。
func TestActiveState_ManualFailureRollsBackToLastPersistedNotPreviousMemory(t *testing.T) {
	st := newFakeEndpointStore("a", "b", "c")
	manager := newActiveStateTestManager(t, st, "a", "b", "c")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate a failed: %v", err)
	}

	st.setActivateErr(errors.New("disk io error"))

	_, revBefore := manager.GetActiveEndpointSelection()
	if !manager.TryMigrateActiveEndpoint("b", revBefore, 0) {
		t.Fatal("auto migrate to b should pass CAS")
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "b" {
		t.Fatalf("expected memory active=b after migrate, got %q", active)
	}

	err := manager.ActivateEndpointManually("c")
	if err == nil {
		t.Fatal("expected manual activate c to fail while store is failing")
	}

	active, revAfter := manager.GetActiveEndpointSelection()
	if active != "a" {
		t.Fatalf("expected rollback to lastPersisted=a, got %q", active)
	}
	if revAfter <= revBefore {
		t.Fatalf("expected rollback to produce a newer revision, before=%d after=%d", revBefore, revAfter)
	}
	if enabled := st.enabledSnapshot(); !enabled["a"] {
		t.Fatalf("DB should still hold a, got %v", enabled)
	}
}

func TestActiveState_RollbackRevisionInvalidatesOldScheduleSnapshot(t *testing.T) {
	st := newFakeEndpointStore("a", "b", "c")
	manager := newActiveStateTestManager(t, st, "a", "b", "c")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}
	_, staleRevision := manager.GetActiveEndpointSelection()

	st.setActivateErr(errors.New("disk io error"))
	if err := manager.ActivateEndpointManually("c"); err == nil {
		t.Fatal("expected manual activate to fail")
	}
	st.setActivateErr(nil)

	// 失败回滚后 revision 已推进：临时状态期间取得的旧快照不得通过 CAS
	if manager.TryMigrateActiveEndpoint("b", staleRevision, 0) {
		t.Fatal("stale snapshot revision must fail CAS after rollback")
	}
}

func TestActiveState_NoRollbackWhenNewerRevisionExists(t *testing.T) {
	st := newFakeEndpointStore("a", "b", "c")
	manager := newActiveStateTestManager(t, st, "a", "b", "c")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}

	// hook：手动任务执行（将失败）期间产生更新的 revision（模拟并发变更）
	st.mu.Lock()
	st.onActivate = func(name string) {
		if name == "b" {
			manager.RestoreActiveEndpoint("c")
		}
	}
	st.mu.Unlock()
	st.setActivateErr(errors.New("disk io error"))

	if err := manager.ActivateEndpointManually("b"); err == nil {
		t.Fatal("expected manual activate b to fail")
	}

	// 已有更新 revision（active=c）：不得被回滚覆盖
	if active, _ := manager.GetActiveEndpointSelection(); active != "c" {
		t.Fatalf("expected newer state c preserved (no rollback), got %q", active)
	}
}

func TestActiveState_AutoTaskRetriesUntilPersisted(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	manager := newActiveStateTestManager(t, st, "a", "b")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}

	st.setActivateErr(errors.New("transient db error"))
	_, rev := manager.GetActiveEndpointSelection()
	if !manager.TryMigrateActiveEndpoint("b", rev, 0) {
		t.Fatal("auto migrate should pass CAS")
	}

	// 无后续迁移事件：仅靠重试机制最终落库
	time.Sleep(300 * time.Millisecond)
	st.setActivateErr(nil)

	waitUntil(t, 3*time.Second, func() bool {
		enabled := st.enabledSnapshot()
		return enabled["b"] && !enabled["a"]
	}, "auto task retried until persisted")

	name, _, established := manager.runtimeWriter.LastPersisted()
	if !established || name != "b" {
		t.Fatalf("expected lastPersisted=b after retry, got %q established=%v", name, established)
	}
}

func TestActiveState_DeactivateActiveClearsMemoryAndDisablesDB(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	manager := newActiveStateTestManager(t, st, "a", "b")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}
	if err := manager.DeactivateActiveEndpointManually("a"); err != nil {
		t.Fatalf("deactivate failed: %v", err)
	}

	if active, _ := manager.GetActiveEndpointSelection(); active != "" {
		t.Fatalf("expected active cleared, got %q", active)
	}
	if enabled := st.enabledSnapshot(); enabled["a"] {
		t.Fatal("expected endpoint a disabled in DB")
	}
	name, _, established := manager.runtimeWriter.LastPersisted()
	if !established || name != "" {
		t.Fatalf("expected lastPersisted empty established, got %q", name)
	}
}

func TestActiveState_DeactivateConcurrentWithAutoMigrateKeepsRevisionOrder(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	manager := newActiveStateTestManager(t, st, "a", "b")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}
	_, staleRev := manager.GetActiveEndpointSelection()

	if err := manager.DeactivateActiveEndpointManually("a"); err != nil {
		t.Fatalf("deactivate failed: %v", err)
	}

	// 停用后，携带停用前 revision 的自动迁移必须 CAS 失败
	if manager.TryMigrateActiveEndpoint("b", staleRev, 0) {
		t.Fatal("auto migrate with pre-deactivate revision must fail CAS")
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "" {
		t.Fatalf("expected active stays cleared, got %q", active)
	}
}

func TestActiveState_DeleteActiveEndpointClearsActive(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	manager := newActiveStateTestManager(t, st, "a", "b")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}
	if err := manager.DeleteEndpointCoordinated("a"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if active, _ := manager.GetActiveEndpointSelection(); active != "" {
		t.Fatalf("expected active cleared after deleting active endpoint, got %q", active)
	}
	if manager.GetEndpointByNameAny("a") != nil {
		t.Fatal("expected endpoint a removed from memory")
	}
	if _, ok := st.enabledSnapshot()["a"]; ok {
		t.Fatal("expected endpoint a deleted from DB")
	}
}

// v7 场景：DB/lastPersisted=B，自动迁移内存=C 未落库，删除非 active 的 B。
// 删除必须经 writer 串行更新 lastPersisted，随后手动写库失败不得回滚到已删除的 B。
func TestActiveState_DeleteLastPersistedEndpointWhileMigrationUnflushed(t *testing.T) {
	st := newFakeEndpointStore("b", "c", "d")
	manager := newActiveStateTestManager(t, st, "b", "c", "d")

	if err := manager.ActivateEndpointManually("b"); err != nil {
		t.Fatalf("seed activate b failed: %v", err)
	}

	st.setActivateErr(errors.New("db busy"))
	_, rev := manager.GetActiveEndpointSelection()
	if !manager.TryMigrateActiveEndpoint("c", rev, 0) {
		t.Fatal("auto migrate to c should pass CAS")
	}

	// 删除非 active 的 lastPersisted 端点 B（Delete 不受 activateErr 影响）
	if err := manager.DeleteEndpointCoordinated("b"); err != nil {
		t.Fatalf("delete b failed: %v", err)
	}
	name, _, established := manager.runtimeWriter.LastPersisted()
	if !established || name != "" {
		t.Fatalf("expected lastPersisted cleared after deleting persisted endpoint, got %q", name)
	}

	// 手动激活 d 失败：回滚基准应为空（清空 active），绝不能是已删除的 b
	if err := manager.ActivateEndpointManually("d"); err == nil {
		t.Fatal("expected manual activate d to fail")
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "" {
		t.Fatalf("expected rollback to cleared active, got %q", active)
	}
}

func TestActiveState_DeleteDBFailureLeavesMemoryUntouched(t *testing.T) {
	st := newFakeEndpointStore("a", "b")
	manager := newActiveStateTestManager(t, st, "a", "b")

	if err := manager.ActivateEndpointManually("a"); err != nil {
		t.Fatalf("seed activate failed: %v", err)
	}
	st.setDeleteErr(errors.New("db locked"))

	if err := manager.DeleteEndpointCoordinated("a"); err == nil {
		t.Fatal("expected delete to fail")
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "a" {
		t.Fatalf("expected active unchanged after delete failure, got %q", active)
	}
	if manager.GetEndpointByNameAny("a") == nil {
		t.Fatal("expected endpoint a still present in memory after delete failure")
	}
}

func TestActiveState_RestoreFromEnabledThreeStates(t *testing.T) {
	t.Run("zero enabled", func(t *testing.T) {
		st := newFakeEndpointStore("a")
		manager := newActiveStateTestManager(t, st, "a")

		active, degraded := manager.RestoreActiveFromEnabled(nil)
		if active != "" || degraded {
			t.Fatalf("expected empty active without degraded, got %q %v", active, degraded)
		}
		name, _, established := manager.runtimeWriter.LastPersisted()
		if !established || name != "" {
			t.Fatalf("expected empty lastPersisted established, got %q", name)
		}
	})

	t.Run("single enabled", func(t *testing.T) {
		st := newFakeEndpointStore("a", "b")
		manager := newActiveStateTestManager(t, st, "a", "b")

		active, degraded := manager.RestoreActiveFromEnabled([]EnabledEndpointRecord{{Name: "b", Priority: 5, ID: 2}})
		if active != "b" || degraded {
			t.Fatalf("expected active=b without degraded, got %q %v", active, degraded)
		}
		if got, _ := manager.GetActiveEndpointSelection(); got != "b" {
			t.Fatalf("expected memory restore b, got %q", got)
		}
		// 启动恢复仅内存：不得写回刚读取的 DB
		if len(st.activateHistory()) != 0 {
			t.Fatalf("startup restore must not write DB, got %v", st.activateHistory())
		}
		name, _, established := manager.runtimeWriter.LastPersisted()
		if !established || name != "b" {
			t.Fatalf("expected lastPersisted=b, got %q", name)
		}
	})

	t.Run("multiple enabled repaired to deterministic winner", func(t *testing.T) {
		st := newFakeEndpointStore("a", "b", "c")
		manager := newActiveStateTestManager(t, st, "a", "b", "c")

		active, degraded := manager.RestoreActiveFromEnabled([]EnabledEndpointRecord{
			{Name: "a", Priority: 20, ID: 1},
			{Name: "b", Priority: 10, ID: 3},
			{Name: "c", Priority: 10, ID: 2},
		})
		if degraded {
			t.Fatal("repair should succeed")
		}
		if active != "c" {
			t.Fatalf("expected winner=c (min priority, tie by min id), got %q", active)
		}
		if enabled := st.enabledSnapshot(); !enabled["c"] || enabled["a"] || enabled["b"] {
			t.Fatalf("expected repair transaction to leave only winner enabled, got %v", enabled)
		}
		name, _, established := manager.runtimeWriter.LastPersisted()
		if !established || name != "c" {
			t.Fatalf("expected lastPersisted=winner, got %q established=%v", name, established)
		}
	})

	t.Run("repair failure enters degraded and manual failure rolls back to empty", func(t *testing.T) {
		st := newFakeEndpointStore("a", "b")
		manager := newActiveStateTestManager(t, st, "a", "b")
		st.setActivateErr(errors.New("db locked"))

		active, degraded := manager.RestoreActiveFromEnabled([]EnabledEndpointRecord{
			{Name: "a", Priority: 10, ID: 1},
			{Name: "b", Priority: 20, ID: 2},
		})
		if !degraded || active != "a" {
			t.Fatalf("expected degraded with winner=a, got %q %v", active, degraded)
		}
		if _, _, established := manager.runtimeWriter.LastPersisted(); established {
			t.Fatal("lastPersisted must stay unestablished while repair fails")
		}

		// degraded 期间手动激活失败：基准未确立 → 回滚为清空 active（保守基准）
		if err := manager.ActivateEndpointManually("b"); err == nil {
			t.Fatal("expected manual activate to fail while store failing")
		}
		if got, _ := manager.GetActiveEndpointSelection(); got != "" {
			t.Fatalf("expected conservative rollback to empty active, got %q", got)
		}
	})

	t.Run("repair failure converges via retry when store recovers", func(t *testing.T) {
		st := newFakeEndpointStore("a", "b")
		manager := newActiveStateTestManager(t, st, "a", "b")
		st.setActivateErr(errors.New("db locked"))

		active, degraded := manager.RestoreActiveFromEnabled([]EnabledEndpointRecord{
			{Name: "a", Priority: 10, ID: 1},
			{Name: "b", Priority: 20, ID: 2},
		})
		if !degraded || active != "a" {
			t.Fatalf("expected degraded with winner=a, got %q %v", active, degraded)
		}

		// 无其他变更（revision 不变），故障恢复后重试队列内的修复任务最终收敛
		st.setActivateErr(nil)
		waitUntil(t, 3*time.Second, func() bool {
			name, _, established := manager.runtimeWriter.LastPersisted()
			return established && name == "a"
		}, "repair retry converges")
		if enabled := st.enabledSnapshot(); !enabled["a"] || enabled["b"] {
			t.Fatalf("expected repair to leave only winner enabled, got %v", enabled)
		}
	})
}
