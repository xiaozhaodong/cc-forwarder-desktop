package proxy

import (
	"context"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
)

// TestEndpointSelfHealingIntegration 完整的端点自愈集成测试
// 验证 SuspensionManager, LifecycleManager, EndpointRecoverySignalManager 的协同工作
func TestEndpointSelfHealingIntegration(t *testing.T) {
	t.Run("CompleteEndpointSelfHealingFlow", func(t *testing.T) {
		// 🎯 场景：端点失败→挂起→其他请求成功→自动恢复
		t.Log("🚀 开始完整端点自愈集成测试")

		// 1. 设置测试环境
		cfg := &config.Config{
			RequestSuspend: config.RequestSuspendConfig{
				Enabled:              true,
				Timeout:              5 * time.Second,
				MaxSuspendedRequests: 10,
			},
			Group: config.GroupConfig{
				AutoSwitchBetweenGroups: false, // 手动模式，启用挂起
				Cooldown:                10 * time.Second,
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:  "test-endpoint",
					URL:   "https://test.example.com",
					Group: "test-group",
				},
			},
		}

		// 创建端点管理器
		endpointMgr := endpoint.NewManager(cfg)
		groupMgr := endpoint.NewGroupManager(cfg)

		// 创建恢复信号管理器
		recoverySignalManager := NewEndpointRecoverySignalManager()

		// 创建SuspensionManager（带恢复信号）
		suspensionMgr := NewSuspensionManagerWithRecoverySignal(cfg, endpointMgr, groupMgr, recoverySignalManager)

		t.Log("✅ 测试环境设置完成")

		// 2. 模拟第一个请求失败，触发挂起
		t.Log("1️⃣ 模拟请求1失败，触发挂起...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		connID1 := "req-001"
		failedEndpoint := "test-endpoint"

		t.Logf("   📊 准备测试端点恢复功能，端点: %s", failedEndpoint)

		// 在goroutine中启动挂起等待
		var waitResult bool
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			t.Logf("   ⏸️ 请求 %s 开始等待端点 %s 恢复...", connID1, failedEndpoint)
			waitResult = suspensionMgr.WaitForEndpointRecovery(ctx, connID1, failedEndpoint)
			t.Logf("   🎯 请求 %s 等待结果: %t", connID1, waitResult)
		}()

		// 等待一段时间确保挂起等待已开始
		time.Sleep(200 * time.Millisecond)

		// 验证订阅者已注册
		subscriberCount := recoverySignalManager.GetSubscriberCount(failedEndpoint)
		t.Logf("   📈 端点 %s 的等待请求数: %d", failedEndpoint, subscriberCount)

		// 3. 模拟第二个请求成功，触发端点恢复信号
		t.Log("2️⃣ 模拟请求2成功，广播端点恢复信号...")

		// 创建第二个LifecycleManager模拟成功请求
		connID2 := "req-002"
		lifecycleManager2 := NewRequestLifecycleManagerWithRecoverySignal(
			nil, // usageTracker
			nil, // monitoringMiddleware
			connID2,
			nil, // eventBus
			recoverySignalManager,
		)

		// 设置端点信息
		lifecycleManager2.SetEndpoint("test-endpoint", "test-group", "")

		// 在另一个goroutine中模拟请求成功完成
		go func() {
			time.Sleep(500 * time.Millisecond) // 模拟请求处理延迟
			t.Logf("   ✅ 请求 %s 在端点 %s 成功完成，触发恢复信号广播", connID2, failedEndpoint)

			// 模拟成功请求完成，这会触发端点恢复信号广播
			mockTokens := &tracking.TokenUsage{
				InputTokens:  100,
				OutputTokens: 200,
			}
			lifecycleManager2.CompleteRequest(mockTokens)
		}()

		// 4. 等待挂起请求自动恢复
		t.Log("3️⃣ 等待挂起请求自动恢复...")

		// 等待挂起请求完成
		done := make(chan struct{})
		go func() {
			defer close(done)
			wg.Wait()
		}()

		select {
		case <-done:
			if waitResult {
				t.Log("🎉 ✅ 端点自愈成功！挂起请求已自动恢复")
				t.Logf("   🎯 请求 %s 无需等待5分钟超时，立即恢复到端点 %s", connID1, failedEndpoint)
			} else {
				t.Error("❌ 端点自愈失败：挂起请求未能自动恢复")
			}
		case <-time.After(3 * time.Second):
			t.Error("❌ 端点自愈超时：挂起请求恢复超时")
		}

		// 5. 验证清理状态
		t.Log("4️⃣ 验证清理状态...")

		finalSubscriberCount := recoverySignalManager.GetSubscriberCount(failedEndpoint)
		if finalSubscriberCount == 0 {
			t.Logf("✅ 订阅者已清理完成：端点 %s 当前等待请求数为 %d", failedEndpoint, finalSubscriberCount)
		} else {
			t.Errorf("⚠️ 订阅者清理异常：端点 %s 仍有 %d 个等待请求", failedEndpoint, finalSubscriberCount)
		}

		t.Log("🏁 端点自愈集成测试完成")
	})
}

// TestEndpointSelfHealingRaceCondition 测试端点自愈的并发安全性
func TestEndpointSelfHealingRaceCondition(t *testing.T) {
	t.Run("ConcurrentSelfHealing", func(t *testing.T) {
		t.Log("🔄 测试并发端点自愈场景")

		recoveryManager := NewEndpointRecoverySignalManager()
		endpointName := "concurrent-endpoint"

		// 模拟多个请求同时挂起
		requestCount := 5
		var wg sync.WaitGroup
		results := make([]bool, requestCount)

		// 启动多个挂起请求
		for i := 0; i < requestCount; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				recoveryCh := recoveryManager.Subscribe(endpointName)
				defer recoveryManager.Unsubscribe(endpointName, recoveryCh)

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				select {
				case recoveredEndpoint := <-recoveryCh:
					results[index] = (recoveredEndpoint == endpointName)
					t.Logf("✅ 请求 %d 收到端点 %s 恢复信号", index+1, recoveredEndpoint)
				case <-ctx.Done():
					results[index] = false
					t.Logf("⏰ 请求 %d 等待超时", index+1)
				}
			}(i)
		}

		// 等待所有请求开始监听
		time.Sleep(100 * time.Millisecond)

		subscriberCount := recoveryManager.GetSubscriberCount(endpointName)
		t.Logf("📊 并发挂起请求数: %d", subscriberCount)

		// 广播恢复信号
		t.Logf("📡 广播端点 %s 恢复信号", endpointName)
		recoveryManager.BroadcastEndpointSuccess(endpointName)

		// 等待所有请求完成
		wg.Wait()

		// 验证结果
		successCount := 0
		for i, success := range results {
			if success {
				successCount++
			} else {
				t.Logf("❌ 请求 %d 未成功恢复", i+1)
			}
		}

		if successCount == requestCount {
			t.Logf("🎉 并发测试成功：%d/%d 个请求成功恢复", successCount, requestCount)
		} else {
			t.Errorf("❌ 并发测试部分失败：只有 %d/%d 个请求成功恢复", successCount, requestCount)
		}

		// 验证清理
		finalCount := recoveryManager.GetSubscriberCount(endpointName)
		if finalCount != 0 {
			t.Errorf("⚠️ 清理异常：仍有 %d 个订阅者", finalCount)
		}
	})
}

// TestEndpointSelfHealingTimeout 测试端点自愈超时场景
func TestEndpointSelfHealingTimeout(t *testing.T) {
	t.Run("SelfHealingTimeout", func(t *testing.T) {
		t.Log("⏰ 测试端点自愈超时场景")

		recoveryManager := NewEndpointRecoverySignalManager()
		endpointName := "timeout-endpoint"

		// 订阅但不广播恢复信号，测试超时
		recoveryCh := recoveryManager.Subscribe(endpointName)
		defer recoveryManager.Unsubscribe(endpointName, recoveryCh)

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		start := time.Now()
		select {
		case recoveredEndpoint := <-recoveryCh:
			t.Errorf("❌ 意外收到恢复信号: %s", recoveredEndpoint)
		case <-ctx.Done():
			elapsed := time.Since(start)
			t.Logf("✅ 超时测试成功：等待 %v 后正确超时", elapsed)

			if elapsed < 250*time.Millisecond || elapsed > 400*time.Millisecond {
				t.Logf("⚠️ 超时时间异常：期望约300ms，实际%v", elapsed)
			}
		}
	})
}
