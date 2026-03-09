package proxy

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestEndpointRecoverySignalManager 测试端点恢复信号管理器
func TestEndpointRecoverySignalManager(t *testing.T) {
	manager := NewEndpointRecoverySignalManager()

	// 测试订阅和广播
	t.Run("SubscribeAndBroadcast", func(t *testing.T) {
		endpointName := "test-endpoint"

		// 订阅端点恢复信号
		recoveryCh := manager.Subscribe(endpointName)

		// 检查订阅者数量
		count := manager.GetSubscriberCount(endpointName)
		if count != 1 {
			t.Errorf("Expected 1 subscriber, got %d", count)
		}

		// 在另一个goroutine中广播成功信号
		go func() {
			time.Sleep(100 * time.Millisecond)
			manager.BroadcastEndpointSuccess(endpointName)
		}()

		// 等待恢复信号
		select {
		case recoveredEndpoint := <-recoveryCh:
			if recoveredEndpoint != endpointName {
				t.Errorf("Expected endpoint %s, got %s", endpointName, recoveredEndpoint)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for recovery signal")
		}

		// 广播后订阅者应该被清空
		count = manager.GetSubscriberCount(endpointName)
		if count != 0 {
			t.Errorf("Expected 0 subscribers after broadcast, got %d", count)
		}
	})

	// 测试多个订阅者
	t.Run("MultipleSubscribers", func(t *testing.T) {
		endpointName := "multi-endpoint"
		subscriberCount := 3

		// 创建多个订阅者
		channels := make([]chan string, subscriberCount)
		for i := 0; i < subscriberCount; i++ {
			channels[i] = manager.Subscribe(endpointName)
		}

		// 检查订阅者数量
		count := manager.GetSubscriberCount(endpointName)
		if count != subscriberCount {
			t.Errorf("Expected %d subscribers, got %d", subscriberCount, count)
		}

		// 广播成功信号
		go func() {
			time.Sleep(50 * time.Millisecond)
			manager.BroadcastEndpointSuccess(endpointName)
		}()

		// 等待所有订阅者收到信号
		var wg sync.WaitGroup
		wg.Add(subscriberCount)

		for i, ch := range channels {
			go func(index int, recoveryCh chan string) {
				defer wg.Done()
				select {
				case recoveredEndpoint := <-recoveryCh:
					if recoveredEndpoint != endpointName {
						t.Errorf("Subscriber %d: Expected endpoint %s, got %s", index, endpointName, recoveredEndpoint)
					}
				case <-time.After(1 * time.Second):
					t.Errorf("Subscriber %d: Timeout waiting for recovery signal", index)
				}
			}(i, ch)
		}

		wg.Wait()

		// 广播后订阅者应该被清空
		count = manager.GetSubscriberCount(endpointName)
		if count != 0 {
			t.Errorf("Expected 0 subscribers after broadcast, got %d", count)
		}
	})

	// 测试取消订阅
	t.Run("Unsubscribe", func(t *testing.T) {
		endpointName := "unsubscribe-endpoint"

		// 订阅端点恢复信号
		recoveryCh := manager.Subscribe(endpointName)

		// 检查订阅者数量
		count := manager.GetSubscriberCount(endpointName)
		if count != 1 {
			t.Errorf("Expected 1 subscriber, got %d", count)
		}

		// 取消订阅
		manager.Unsubscribe(endpointName, recoveryCh)

		// 检查订阅者数量
		count = manager.GetSubscriberCount(endpointName)
		if count != 0 {
			t.Errorf("Expected 0 subscribers after unsubscribe, got %d", count)
		}
	})
}

// TestSuspensionManagerEndpointRecovery 测试SuspensionManager的端点恢复功能
func TestSuspensionManagerEndpointRecovery(t *testing.T) {
	// 这是一个概念测试，展示端点自愈机制如何工作
	t.Run("EndpointRecoveryFlow", func(t *testing.T) {
		// 创建恢复信号管理器
		recoveryManager := NewEndpointRecoverySignalManager()

		endpointName := "test-endpoint"

		// 模拟挂起请求等待端点恢复
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// 订阅端点恢复信号
		recoveryCh := recoveryManager.Subscribe(endpointName)
		defer recoveryManager.Unsubscribe(endpointName, recoveryCh)

		// 模拟另一个请求成功，触发端点恢复信号
		go func() {
			time.Sleep(500 * time.Millisecond) // 模拟处理延迟
			t.Logf("🚀 模拟端点 %s 另一个请求成功，广播恢复信号", endpointName)
			recoveryManager.BroadcastEndpointSuccess(endpointName)
		}()

		// 等待端点恢复信号
		select {
		case recoveredEndpoint := <-recoveryCh:
			t.Logf("✅ 端点自愈成功！端点 %s 已恢复，可以重试挂起的请求", recoveredEndpoint)
			if recoveredEndpoint != endpointName {
				t.Errorf("Expected endpoint %s, got %s", endpointName, recoveredEndpoint)
			}
		case <-ctx.Done():
			t.Error("❌ 端点自愈超时，未收到恢复信号")
		}
	})
}

// TestEndpointSelfHealingScenario 测试完整的端点自愈场景
func TestEndpointSelfHealingScenario(t *testing.T) {
	t.Run("CompleteScenario", func(t *testing.T) {
		// 🎯 场景：端点A的请求失败 → 挂起 → 端点A其他请求成功 → 挂起请求自动恢复

		recoveryManager := NewEndpointRecoverySignalManager()
		endpointA := "endpoint-a"

		t.Logf("📝 场景开始：端点 %s 自愈测试", endpointA)

		// Step 1: 模拟请求失败，触发挂起
		t.Log("1️⃣ 请求A失败，触发挂起...")
		recoveryCh := recoveryManager.Subscribe(endpointA)
		defer recoveryManager.Unsubscribe(endpointA, recoveryCh)

		subscriberCount := recoveryManager.GetSubscriberCount(endpointA)
		t.Logf("   📊 当前等待端点 %s 的挂起请求数: %d", endpointA, subscriberCount)

		// Step 2: 模拟同端点其他请求成功
		go func() {
			time.Sleep(300 * time.Millisecond)
			t.Logf("2️⃣ 端点 %s 其他请求成功，广播恢复信号...", endpointA)
			recoveryManager.BroadcastEndpointSuccess(endpointA)
		}()

		// Step 3: 挂起请求收到恢复信号，自动重试
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		select {
		case recoveredEndpoint := <-recoveryCh:
			t.Logf("3️⃣ ✅ 端点自愈成功！端点 %s 已恢复", recoveredEndpoint)
			t.Log("   🎯 挂起的请求现在可以立即重试原端点，无需等待5分钟超时")
			t.Log("   🚀 5分钟内自愈达成！")

			if recoveredEndpoint != endpointA {
				t.Errorf("Expected endpoint %s, got %s", endpointA, recoveredEndpoint)
			}

		case <-ctx.Done():
			t.Error("❌ 端点自愈失败，超时未收到恢复信号")
		}

		// Step 4: 验证订阅者已清理
		subscriberCount = recoveryManager.GetSubscriberCount(endpointA)
		if subscriberCount != 0 {
			t.Errorf("Expected 0 waiting requests after recovery, got %d", subscriberCount)
		}

		t.Log("🎉 端点自愈场景测试完成！")
	})
}
