package endpoint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cc-forwarder/config"
)

// TestEmergencyActivationCooldownScenarios 测试应急激活在冷却期间的行为
func TestEmergencyActivationCooldownScenarios(t *testing.T) {
	// 创建测试配置
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Second * 2, // 短冷却时间用于测试
			AutoSwitchBetweenGroups: false,
		},
		Failover: config.FailoverConfig{
			Enabled:         false,
			DefaultCooldown: time.Second * 2,
		},
	}

	// 创建组管理器
	gm := NewGroupManager(cfg)

	// 创建测试端点
	// v4.0: 端点名即组名
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:          "endpoint-1",
				URL:           "https://api.example.com",
				Group:         "test-group", // v3.x兼容字段，v4.0中会被忽略
				GroupPriority: 1,
				Priority:      1,
			},
			Status: EndpointStatus{
				Healthy: false,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("冷却期间应急激活应该被拒绝", func(t *testing.T) {
		t.Log("=== 测试冷却期间应急激活被拒绝 ===")

		// v4.0: 组名是端点名
		groupName := "endpoint-1"

		// 手动设置组进入冷却状态
		groups := gm.GetAllGroups()
		testGroup := findGroupByName(groups, groupName)
		require.NotNil(t, testGroup, "应该找到"+groupName)

		// 直接设置冷却时间
		gm.mutex.Lock()
		gm.groups[groupName].CooldownUntil = time.Now().Add(time.Hour)
		gm.mutex.Unlock()

		// 尝试应急激活（应该被拒绝）
		err := gm.ManualActivateGroupWithForce(groupName, true)
		assert.Error(t, err, "冷却期间应急激活应该被拒绝")
		assert.Contains(t, err.Error(), "仍在冷却中", "错误消息应该提到冷却")
		assert.Contains(t, err.Error(), "剩余时间", "错误消息应该提到剩余时间")

		t.Logf("✅ 冷却期间应急激活被正确拒绝: %v", err.Error())
	})

	t.Run("冷却结束后应急激活应该成功", func(t *testing.T) {
		t.Log("=== 测试冷却结束后应急激活成功 ===")

		// v4.0: 组名是端点名
		groupName := "endpoint-1"

		// 清除冷却状态
		gm.mutex.Lock()
		gm.groups[groupName].CooldownUntil = time.Time{}
		gm.groups[groupName].IsActive = false
		gm.groups[groupName].ForcedActivation = false
		gm.groups[groupName].ForcedActivationTime = time.Time{}
		gm.mutex.Unlock()

		// 确保端点不健康
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// 尝试应急激活（应该成功）
		err := gm.ManualActivateGroupWithForce(groupName, true)
		assert.NoError(t, err, "冷却结束后应急激活应该成功")

		// 验证激活状态
		groups := gm.GetAllGroups()
		testGroup := findGroupByName(groups, groupName)
		require.NotNil(t, testGroup, "应该找到"+groupName)

		assert.True(t, testGroup.IsActive, "组应该已激活")
		assert.True(t, testGroup.ForcedActivation, "应该标记为强制激活")
		assert.False(t, testGroup.ForcedActivationTime.IsZero(), "应该有强制激活时间")

		t.Logf("✅ 冷却结束后应急激活成功")
	})
}

// TestEmergencyActivationEdgeCases 测试应急激活的边缘情况
func TestEmergencyActivationEdgeCases(t *testing.T) {
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Minute,
			AutoSwitchBetweenGroups: false,
		},
	}

	gm := NewGroupManager(cfg)

	t.Run("不存在的组应该返回错误", func(t *testing.T) {
		err := gm.ManualActivateGroupWithForce("nonexistent-group", true)
		assert.Error(t, err, "不存在的组应该返回错误")
		assert.Contains(t, err.Error(), "组不存在", "错误消息应该提到组不存在")
	})

	t.Run("空组名应该在Web API层面被拦截", func(t *testing.T) {
		// 这个测试验证空组名的处理逻辑
		// 实际情况下这会在Web API层面被拦截，这里测试底层行为
		err := gm.ManualActivateGroupWithForce("", true)
		assert.Error(t, err, "空组名应该返回错误")
	})

	t.Run("没有端点的组应该能正常处理", func(t *testing.T) {
		// 创建一个没有端点的组
		emptyEndpoints := []*Endpoint{}
		gm.UpdateGroups(emptyEndpoints)

		// 尝试激活不存在端点的组
		err := gm.ManualActivateGroupWithForce("empty-group", true)
		assert.Error(t, err, "空组应该返回错误")
	})
}

// TestEmergencyActivationComprehensiveAPI 测试应急激活的完整API响应
// v4.0: 一端点一组架构
func TestEmergencyActivationComprehensiveAPI(t *testing.T) {
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Minute,
			AutoSwitchBetweenGroups: false,
		},
		Failover: config.FailoverConfig{
			Enabled:         false,
			DefaultCooldown: time.Minute,
		},
	}

	gm := NewGroupManager(cfg)

	// v4.0: 创建端点用于测试，每个端点自动成为一个独立的组
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:     "healthy-endpoint",
				URL:      "https://healthy.example.com",
				Priority: 1,
			},
			Status: EndpointStatus{
				Healthy: true,
			},
		},
		{
			Config: config.EndpointConfig{
				Name:     "unhealthy-endpoint",
				URL:      "https://unhealthy.example.com",
				Priority: 2,
			},
			Status: EndpointStatus{
				Healthy: false,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("综合测试所有组的can_force_activate状态", func(t *testing.T) {
		// 确保没有组是活跃的
		for _, group := range gm.groups {
			group.IsActive = false
			group.ManuallyPaused = false
			group.CooldownUntil = time.Time{}
		}

		// v4.0: 组名就是端点名
		gm.ManualPauseGroup("healthy-endpoint", 0)
		gm.ManualPauseGroup("unhealthy-endpoint", 0)

		details := gm.GetGroupDetails()
		groupsData, ok := details["groups"].([]map[string]interface{})
		require.True(t, ok, "应该返回groups数组")

		for _, groupData := range groupsData {
			groupName := groupData["name"].(string)
			canForceActivate := groupData["can_force_activate"].(bool)
			healthyEndpoints := groupData["healthy_endpoints"].(int)
			isActive := groupData["is_active"].(bool)

			if groupName == "healthy-endpoint" {
				assert.True(t, canForceActivate, "非活跃组应该能强制激活")
				assert.Greater(t, healthyEndpoints, 0, "healthy-endpoint应该有健康探测结果")
			} else if groupName == "unhealthy-endpoint" {
				assert.True(t, canForceActivate, "非活跃组应该能强制激活")
				assert.Equal(t, 0, healthyEndpoints, "unhealthy-endpoint应该没有健康探测结果")
			}

			assert.False(t, isActive, "所有组都应该是非活跃状态")

			t.Logf("组 %s: can_force_activate=%v, healthy_endpoints=%d, is_active=%v",
				groupName, canForceActivate, healthyEndpoints, isActive)
		}
	})

	t.Run("测试强制激活后状态变化", func(t *testing.T) {
		// v4.0: 强制激活unhealthy-endpoint（组名=端点名）
		err := gm.ManualActivateGroupWithForce("unhealthy-endpoint", true)
		require.NoError(t, err, "应该成功强制激活")

		details := gm.GetGroupDetails()
		groupsData, ok := details["groups"].([]map[string]interface{})
		require.True(t, ok, "应该返回groups数组")

		var unhealthyGroupData, healthyGroupData map[string]interface{}
		for _, groupData := range groupsData {
			if groupData["name"] == "unhealthy-endpoint" {
				unhealthyGroupData = groupData
			} else if groupData["name"] == "healthy-endpoint" {
				healthyGroupData = groupData
			}
		}

		require.NotNil(t, unhealthyGroupData, "应该找到unhealthy-endpoint")
		require.NotNil(t, healthyGroupData, "应该找到healthy-endpoint")

		// 验证强制激活的组
		assert.True(t, unhealthyGroupData["is_active"].(bool), "unhealthy-endpoint应该已激活")
		assert.True(t, unhealthyGroupData["forced_activation"].(bool), "应该标记为强制激活")
		assert.Equal(t, "forced", unhealthyGroupData["activation_type"], "激活类型应该是forced")
		assert.False(t, unhealthyGroupData["can_force_activate"].(bool), "已激活组不能再强制激活")
		assert.NotEmpty(t, unhealthyGroupData["forced_activation_time"], "应该有强制激活时间")

		// 验证其他组
		assert.False(t, healthyGroupData["is_active"].(bool), "healthy-endpoint应该非活跃")
		assert.False(t, healthyGroupData["forced_activation"].(bool), "healthy-endpoint不应该标记为强制激活")
		assert.Equal(t, "normal", healthyGroupData["activation_type"], "激活类型应该是normal")

		t.Logf("✅ 强制激活后状态验证成功")
	})
}
