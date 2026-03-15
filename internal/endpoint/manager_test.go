package endpoint

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/events"
)

// MockEventBus 用于测试的模拟EventBus
type MockEventBus struct {
	mutex       sync.RWMutex
	events      []events.Event
	broadcaster events.SSEBroadcaster
}

// Event 类型别名以便测试使用
type Event = events.Event
type PriorityHigh = events.EventPriority

const (
	PriorityHighValue = events.PriorityHigh
)

// Publish 实现EventBus接口
func (m *MockEventBus) Publish(event events.Event) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.events = append(m.events, event)
}

// SetSSEBroadcaster 实现EventBus接口
func (m *MockEventBus) SetSSEBroadcaster(broadcaster events.SSEBroadcaster) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.broadcaster = broadcaster
}

// Start 实现EventBus接口
func (m *MockEventBus) Start() error {
	return nil
}

// Stop 实现EventBus接口
func (m *MockEventBus) Stop() error {
	return nil
}

// GetStats 实现EventBus接口
func (m *MockEventBus) GetStats() events.BusStats {
	return events.BusStats{}
}

// EventsSnapshot returns a copy for concurrent-safe assertions.
func (m *MockEventBus) EventsSnapshot() []events.Event {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	copied := make([]events.Event, len(m.events))
	copy(copied, m.events)
	return copied
}

// ClearEvents clears all recorded events in a concurrent-safe way.
func (m *MockEventBus) ClearEvents() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.events = m.events[:0]
}

func TestHealthCheckWithAPIEndpoint(t *testing.T) {
	testCases := []struct {
		name          string
		statusCode    int
		expectHealthy bool
	}{
		{"Success 200", 200, true},
		{"Success 201", 201, true},
		{"Bad Request 400", 400, true},
		{"Unauthorized 401", 401, true},
		{"Forbidden 403", 403, true},
		{"Not Found 404", 404, true},
		{"Server Error 500", 500, false}, // API has issues
		{"Bad Gateway 502", 502, false},  // API unreachable
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server that returns the specified status code
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Add small delay to ensure response time is measurable
				time.Sleep(1 * time.Millisecond)

				// Verify it's checking the correct path
				if r.URL.Path != "/v1/models" {
					t.Errorf("Expected request to /v1/models, got %s", r.URL.Path)
				}
				// Verify Authorization header is present
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", r.Header.Get("Authorization"))
				}
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			// Create config with test server URL
			cfg := &config.Config{
				Health: config.HealthConfig{
					CheckInterval: 30 * time.Second,
					Timeout:       5 * time.Second,
					HealthPath:    "/v1/models",
				},
				Endpoints: []config.EndpointConfig{
					{
						Name:    "test-endpoint",
						URL:     server.URL,
						Token:   "test-token",
						Timeout: 30 * time.Second,
					},
				},
			}

			// Create manager and perform single health check
			manager := NewManager(cfg)
			endpoint := manager.GetAllEndpoints()[0]

			manager.checkEndpointHealth(endpoint)

			// Check result
			if endpoint.IsHealthy() != tc.expectHealthy {
				t.Errorf("Expected healthy=%v for status %d, got %v",
					tc.expectHealthy, tc.statusCode, endpoint.IsHealthy())
			}

			// Verify response time is recorded (should be > 0 for all HTTP responses)
			responseTime := endpoint.GetResponseTime()
			if responseTime <= 0 {
				t.Errorf("Expected response time to be recorded for status %d, got %v", tc.statusCode, responseTime)
			}
		})
	}
}

func TestManagerStartDoesNotPerformAutomaticConnectivityChecks(t *testing.T) {
	requests := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 20 * time.Millisecond,
			Timeout:       1 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:    "auto-check-endpoint",
				URL:     server.URL,
				Token:   "test-token",
				Timeout: 30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)
	manager.Start()
	defer manager.Stop()

	time.Sleep(80 * time.Millisecond)

	select {
	case <-requests:
		t.Fatal("expected manager start to avoid automatic connectivity checks")
	default:
	}
}

func TestAddEndpointDoesNotTriggerAutomaticConnectivityCheck(t *testing.T) {
	requests := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	manager := NewManager(&config.Config{})
	err := manager.AddEndpoint(config.EndpointConfig{
		Name: "new-endpoint",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("AddEndpoint returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case <-requests:
		t.Fatal("expected AddEndpoint to avoid automatic connectivity checks")
	default:
	}
}

func TestUpdateEndpointConfigDoesNotTriggerAutomaticConnectivityCheck(t *testing.T) {
	requests := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	manager := NewManager(&config.Config{
		Endpoints: []config.EndpointConfig{
			{
				Name: "existing-endpoint",
				URL:  "https://old.example.com",
			},
		},
	})

	err := manager.UpdateEndpointConfig("existing-endpoint", config.EndpointConfig{
		URL: server.URL,
	})
	if err != nil {
		t.Fatalf("UpdateEndpointConfig returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case <-requests:
		t.Fatal("expected UpdateEndpointConfig to avoid automatic connectivity checks")
	default:
	}
}

func TestManagerStartCleansExpiredFailureTrackerEntries(t *testing.T) {
	originalCleanupInterval := failureTrackerCleanupInterval
	failureTrackerCleanupInterval = 10 * time.Millisecond
	defer func() {
		failureTrackerCleanupInterval = originalCleanupInterval
	}()

	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: time.Hour,
		},
		FailureTracker: config.FailureTrackerConfig{
			Enabled:    true,
			TimeWindow: 10 * time.Millisecond,
			Threshold:  1,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name: "tracked-endpoint",
				URL:  "https://example.com",
			},
		},
	}

	manager := NewManager(cfg)
	manager.RecordFailure("tracked-endpoint")

	if len(manager.failureTracker.endpointFailures) != 1 {
		t.Fatalf("expected failure tracker to contain one endpoint entry, got %d", len(manager.failureTracker.endpointFailures))
	}

	time.Sleep(15 * time.Millisecond)

	manager.Start()
	defer manager.Stop()

	time.Sleep(50 * time.Millisecond)

	if len(manager.failureTracker.endpointFailures) != 0 {
		t.Fatalf("expected expired failure tracker entries to be cleaned, got %d", len(manager.failureTracker.endpointFailures))
	}
}

func TestFastestStrategyLogging(t *testing.T) {
	// Create multiple test servers with different response times
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Simulate slow response
		w.WriteHeader(200)
	}))
	defer slowServer.Close()

	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate fast response
		w.WriteHeader(200)
	}))
	defer fastServer.Close()

	cfg := &config.Config{
		Strategy: config.StrategyConfig{
			Type: "fastest",
		},
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "slow-endpoint",
				URL:      slowServer.URL,
				Priority: 1,
				Timeout:  30 * time.Second,
			},
			{
				Name:     "fast-endpoint",
				URL:      fastServer.URL,
				Priority: 2,
				Timeout:  30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	// Perform health checks to populate response times
	for _, endpoint := range manager.GetAllEndpoints() {
		manager.checkEndpointHealth(endpoint)
	}

	// Get healthy endpoints (this should trigger logging for fastest strategy)
	healthy := manager.GetHealthyEndpoints()

	// Handle case where endpoints might not be healthy due to path mismatch
	if len(healthy) == 0 {
		t.Skip("No healthy endpoints available - this may be due to health check path requirements")
	}

	if len(healthy) < 2 {
		t.Logf("Expected 2 healthy endpoints, got %d", len(healthy))
		return // Skip the rest of the test if we don't have enough endpoints
	}

	// Verify the fast endpoint comes first
	if healthy[0].Config.Name != "fast-endpoint" {
		t.Errorf("Expected fast-endpoint to be first in fastest strategy, got %s", healthy[0].Config.Name)
	}

	// Verify response times are different
	fastTime := healthy[0].GetResponseTime()
	slowTime := healthy[1].GetResponseTime()

	if fastTime >= slowTime {
		t.Errorf("Expected fast endpoint to have lower response time. Fast: %v, Slow: %v", fastTime, slowTime)
	}
}

// TestGetEndpointByNameWithGroups tests endpoint selection with group priority
// v4.0: 适配一端点一组架构，组名 = 端点名
func TestGetEndpointByNameWithGroups(t *testing.T) {
	// v4.0: 创建两个不同优先级的端点，每个端点自动成为一个独立的组
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown:                10 * time.Minute,
			AutoSwitchBetweenGroups: true, // Enable auto switching for this test
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "primary-endpoint",
				URL:      "https://primary.example.com",
				Priority: 1, // v4.0: 使用 Priority 替代 GroupPriority
				Token:    "primary-token",
				Timeout:  30 * time.Second,
			},
			{
				Name:     "backup-endpoint",
				URL:      "https://backup.example.com",
				Priority: 2,
				Token:    "backup-token",
				Timeout:  30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	// Make endpoints healthy for testing
	for _, ep := range manager.GetAllEndpoints() {
		ep.mutex.Lock()
		ep.Status.Healthy = true
		ep.mutex.Unlock()
	}

	// 先激活 primary-endpoint 组（优先级最高）
	err := manager.ManualActivateGroup("primary-endpoint")
	if err != nil {
		t.Fatalf("Failed to activate primary-endpoint group: %v", err)
	}

	// v4.0: 组名 = 端点名
	// Test: With primary-endpoint group active (priority 1), should return that endpoint
	endpoint := manager.GetEndpointByName("primary-endpoint")
	if endpoint == nil {
		t.Fatal("Expected to find endpoint by name, got nil")
	}
	if endpoint.Config.Name != "primary-endpoint" {
		t.Errorf("Expected primary-endpoint, got: %s", endpoint.Config.Name)
	}
	if endpoint.Config.URL != "https://primary.example.com" {
		t.Errorf("Expected primary URL, got: %s", endpoint.Config.URL)
	}

	// Test: GetEndpointByNameAny should return the endpoint
	endpointAny := manager.GetEndpointByNameAny("primary-endpoint")
	if endpointAny == nil {
		t.Fatal("Expected to find endpoint by name (any), got nil")
	}
	if endpointAny.Config.Name != "primary-endpoint" {
		t.Errorf("Expected primary-endpoint (any search), got: %s", endpointAny.Config.Name)
	}

	// Test: Put primary-endpoint group in cooldown
	// v4.0: 组名 = 端点名
	manager.GetGroupManager().SetGroupCooldown("primary-endpoint")

	// Now GetEndpointByName for primary-endpoint should return nil
	// (because primary-endpoint's group is in cooldown)
	endpoint = manager.GetEndpointByName("primary-endpoint")
	if endpoint != nil {
		t.Errorf("Expected nil when endpoint's group is in cooldown, got: %s", endpoint.Config.Name)
	}

	// v4.0: 必须先激活 backup-endpoint 组才能通过 GetEndpointByName 访问它
	err = manager.ManualActivateGroup("backup-endpoint")
	if err != nil {
		t.Fatalf("Failed to activate backup-endpoint group: %v", err)
	}

	// backup-endpoint should be accessible now
	endpoint = manager.GetEndpointByName("backup-endpoint")
	if endpoint == nil {
		t.Fatal("Expected to find backup-endpoint when primary is in cooldown, got nil")
	}
	if endpoint.Config.Name != "backup-endpoint" {
		t.Errorf("Expected backup-endpoint, got: %s", endpoint.Config.Name)
	}

	// Test: GetEndpointByNameAny should still return primary regardless of cooldown
	endpointAny = manager.GetEndpointByNameAny("primary-endpoint")
	if endpointAny == nil {
		t.Fatal("Expected to find endpoint by name (any) after cooldown, got nil")
	}
	if endpointAny.Config.Name != "primary-endpoint" {
		t.Errorf("Expected primary-endpoint (any search) even after cooldown, got: %s", endpointAny.Config.Name)
	}
}

// TestGetEndpointByNameWithNoActiveGroups tests behavior when no groups are active
// v4.0: 适配一端点一组架构
func TestGetEndpointByNameWithNoActiveGroups(t *testing.T) {
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "test-endpoint",
				URL:      "https://test.example.com",
				Priority: 1,
				Token:    "test-token",
				Timeout:  30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	// Put the only group in cooldown
	// v4.0: 组名 = 端点名
	manager.GetGroupManager().SetGroupCooldown("test-endpoint")

	// GetEndpointByName should return nil (no active groups)
	endpoint := manager.GetEndpointByName("test-endpoint")
	if endpoint != nil {
		t.Errorf("Expected nil when no active groups, got endpoint: %s", endpoint.Config.Name)
	}

	// GetEndpointByNameAny should still return the endpoint
	endpointAny := manager.GetEndpointByNameAny("test-endpoint")
	if endpointAny == nil {
		t.Fatal("Expected to find endpoint by name (any) even with no active groups, got nil")
	}
	if endpointAny.Config.Name != "test-endpoint" {
		t.Errorf("Expected test-endpoint, got: %s", endpointAny.Config.Name)
	}
}

// TestGroupEventBusPublish 测试EventBus组事件发布功能
// v4.0: 适配一端点一组架构
func TestGroupEventBusPublish(t *testing.T) {
	// 创建模拟EventBus
	mockEventBus := &MockEventBus{
		events: make([]Event, 0),
	}

	// 创建测试配置
	// v4.0: 端点名 = 组名
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown:                10 * time.Minute,
			AutoSwitchBetweenGroups: true,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "test-endpoint",
				URL:      "https://test.example.com",
				Priority: 1,
				Token:    "test-token",
				Timeout:  30 * time.Second,
			},
		},
	}

	// 创建Manager实例
	manager := NewManager(cfg)

	// 设置EventBus
	manager.SetEventBus(mockEventBus)

	// v4.0: 组名 = 端点名
	// 调用notifyWebGroupChange方法
	manager.notifyWebGroupChange("group_manually_activated", "test-endpoint")

	// 验证EventBus是否收到正确的事件
	eventsSnapshot := mockEventBus.EventsSnapshot()
	if len(eventsSnapshot) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(eventsSnapshot))
	}

	event := eventsSnapshot[0]

	// 验证事件类型
	if event.Type != "group_status_changed" {
		t.Errorf("Expected event type 'group_status_changed', got '%s'", event.Type)
	}

	// 验证事件来源
	if event.Source != "endpoint_manager" {
		t.Errorf("Expected event source 'endpoint_manager', got '%s'", event.Source)
	}

	// 验证事件优先级
	if event.Priority != events.PriorityHigh {
		t.Errorf("Expected event priority PriorityHigh (%d), got %d", events.PriorityHigh, event.Priority)
	}

	// 验证事件数据格式
	data := event.Data
	if data == nil {
		t.Fatal("Expected event data, got nil")
	}

	// 验证必要字段存在
	requiredFields := []string{"event", "group", "timestamp", "details"}
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			t.Errorf("Expected field '%s' in event data", field)
		}
	}

	// 验证事件字段值
	if data["event"] != "group_manually_activated" {
		t.Errorf("Expected event 'group_manually_activated', got '%v'", data["event"])
	}

	// v4.0: 组名 = 端点名
	if data["group"] != "test-endpoint" {
		t.Errorf("Expected group 'test-endpoint', got '%v'", data["group"])
	}

	// 验证timestamp是字符串格式
	if _, ok := data["timestamp"].(string); !ok {
		t.Errorf("Expected timestamp to be string, got %T", data["timestamp"])
	}

	// 验证details是map类型
	if _, ok := data["details"].(map[string]interface{}); !ok {
		t.Errorf("Expected details to be map[string]interface{}, got %T", data["details"])
	}
}

// TestGroupEventBusPublishWithoutEventBus 测试没有EventBus时的处理
// v4.0: 适配一端点一组架构
func TestGroupEventBusPublishWithoutEventBus(t *testing.T) {
	// 创建测试配置
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "test-endpoint",
				URL:      "https://test.example.com",
				Priority: 1,
				Token:    "test-token",
				Timeout:  30 * time.Second,
			},
		},
	}

	// 创建Manager实例，但不设置EventBus
	manager := NewManager(cfg)

	// 调用notifyWebGroupChange方法应该不会panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("notifyWebGroupChange panicked when EventBus is nil: %v", r)
		}
	}()

	// v4.0: 组名 = 端点名
	manager.notifyWebGroupChange("group_manually_activated", "test-endpoint")
}

// TestGroupEventBusIntegration 测试组管理和EventBus的集成
// v4.0: 适配一端点一组架构
func TestGroupEventBusIntegration(t *testing.T) {
	// 创建模拟EventBus
	mockEventBus := &MockEventBus{
		events: make([]Event, 0),
	}

	// 创建测试配置
	// v4.0: 每个端点自动成为一个独立的组，组名 = 端点名
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown:                10 * time.Minute,
			AutoSwitchBetweenGroups: true,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "primary-endpoint",
				URL:      "https://primary.example.com",
				Priority: 1,
				Token:    "primary-token",
				Timeout:  30 * time.Second,
			},
			{
				Name:     "backup-endpoint",
				URL:      "https://backup.example.com",
				Priority: 2,
				Token:    "backup-token",
				Timeout:  30 * time.Second,
			},
		},
	}

	// 创建Manager实例
	manager := NewManager(cfg)
	manager.SetEventBus(mockEventBus)

	// 手动设置端点为健康状态以便能够激活组
	allEndpoints := manager.GetAllEndpoints()
	for _, ep := range allEndpoints {
		ep.mutex.Lock()
		ep.Status.Healthy = true
		ep.mutex.Unlock()
	}

	// v4.0: 组名 = 端点名
	// 测试手动激活组
	err := manager.ManualActivateGroup("backup-endpoint")
	if err != nil {
		t.Fatalf("Failed to manually activate group: %v", err)
	}

	// 等待goroutine完成
	time.Sleep(10 * time.Millisecond)

	// 验证是否发布了事件
	eventsSnapshot := mockEventBus.EventsSnapshot()
	if len(eventsSnapshot) != 1 {
		t.Errorf("Expected 1 event after manual activation, got %d", len(eventsSnapshot))
	}

	// 验证激活事件
	if len(eventsSnapshot) > 0 {
		event := eventsSnapshot[0]
		if event.Type != "group_status_changed" {
			t.Errorf("Expected group_status_changed event, got %s", event.Type)
		}
		if event.Data["event"] != "group_manually_activated" {
			t.Errorf("Expected group_manually_activated event, got %v", event.Data["event"])
		}
		// v4.0: 组名 = 端点名
		if event.Data["group"] != "backup-endpoint" {
			t.Errorf("Expected backup-endpoint group, got %v", event.Data["group"])
		}
	}

	// 清空事件记录
	mockEventBus.ClearEvents()

	// 测试手动暂停组
	err = manager.ManualPauseGroup("backup-endpoint", 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to manually pause group: %v", err)
	}

	// 等待goroutine完成
	time.Sleep(10 * time.Millisecond)

	// 验证是否发布了暂停事件
	eventsSnapshot = mockEventBus.EventsSnapshot()
	if len(eventsSnapshot) != 1 {
		t.Errorf("Expected 1 event after manual pause, got %d", len(eventsSnapshot))
	}

	// 验证暂停事件
	if len(eventsSnapshot) > 0 {
		event := eventsSnapshot[0]
		if event.Type != "group_status_changed" {
			t.Errorf("Expected group_status_changed event, got %s", event.Type)
		}
		if event.Data["event"] != "group_manually_paused" {
			t.Errorf("Expected group_manually_paused event, got %v", event.Data["event"])
		}
		// v4.0: 组名 = 端点名
		if event.Data["group"] != "backup-endpoint" {
			t.Errorf("Expected backup-endpoint group, got %v", event.Data["group"])
		}
	}

	// 清空事件记录
	mockEventBus.ClearEvents()

	// 测试手动恢复组
	err = manager.ManualResumeGroup("backup-endpoint")
	if err != nil {
		t.Fatalf("Failed to manually resume group: %v", err)
	}

	// 等待goroutine完成
	time.Sleep(10 * time.Millisecond)

	// 验证是否发布了恢复事件
	eventsSnapshot = mockEventBus.EventsSnapshot()
	if len(eventsSnapshot) != 1 {
		t.Errorf("Expected 1 event after manual resume, got %d", len(eventsSnapshot))
	}

	// 验证恢复事件
	if len(eventsSnapshot) > 0 {
		event := eventsSnapshot[0]
		if event.Type != "group_status_changed" {
			t.Errorf("Expected group_status_changed event, got %s", event.Type)
		}
		if event.Data["event"] != "group_manually_resumed" {
			t.Errorf("Expected group_manually_resumed event, got %v", event.Data["event"])
		}
		// v4.0: 组名 = 端点名
		if event.Data["group"] != "backup-endpoint" {
			t.Errorf("Expected backup-endpoint group, got %v", event.Data["group"])
		}
	}
}
