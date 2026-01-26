package endpoint

import (
	"testing"
	"time"

	"cc-forwarder/config"
)

// TestForceActivation tests the force activation feature
// v4.0: 适配一端点一组架构，组名 = 端点名
func TestForceActivation(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Minute,
			AutoSwitchBetweenGroups: false,
		},
	}

	// Create group manager
	gm := NewGroupManager(cfg)

	// v4.0: 创建测试端点，每个端点自动成为一个独立的组
	// 组名就是端点名
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:     "test-endpoint",
				URL:      "https://api.example.com",
				Priority: 1,
			},
			Status: EndpointStatus{
				Healthy: true,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	// v4.0: 组名 = 端点名
	groupName := "test-endpoint"

	t.Run("Normal activation with healthy endpoints", func(t *testing.T) {
		// Ensure endpoint is healthy
		endpoints[0].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroup(groupName)
		if err != nil {
			t.Errorf("Normal activation failed: %v", err)
		}

		groups := gm.GetAllGroups()
		var testGroup *GroupInfo
		for _, g := range groups {
			if g.Name == groupName {
				testGroup = g
				break
			}
		}
		if testGroup == nil || !testGroup.IsActive {
			t.Error("Group should be active after normal activation")
		}
		if testGroup != nil && testGroup.ForcedActivation {
			t.Error("Group should not be marked as force activated")
		}
	})

	t.Run("Normal activation succeeds regardless of health probe", func(t *testing.T) {
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// Reset group state
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ManuallyPaused = false
		}

		err := gm.ManualActivateGroup(groupName)
		if err != nil {
			t.Errorf("Normal activation should succeed regardless of health probe: %v", err)
		}
	})

	t.Run("Force activation succeeds with no healthy endpoints", func(t *testing.T) {
		// Ensure all endpoints are unhealthy
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// Reset group state
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ForcedActivationTime = time.Time{}
			group.ManuallyPaused = false
		}

		err := gm.ManualActivateGroupWithForce(groupName, true)
		if err != nil {
			t.Errorf("Force activation failed: %v", err)
		}

		groups := gm.GetAllGroups()
		var testGroup *GroupInfo
		for _, g := range groups {
			if g.Name == groupName {
				testGroup = g
				break
			}
		}
		if testGroup == nil || !testGroup.IsActive {
			t.Error("Group should be active after force activation")
		}
		if testGroup != nil && !testGroup.ForcedActivation {
			t.Error("Group should be marked as force activated")
		}
		if testGroup != nil && testGroup.ForcedActivationTime.IsZero() {
			t.Error("Force activation time should be set")
		}
	})

	t.Run("Force activation always succeeds", func(t *testing.T) {
		endpoints[0].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		// Reset group state
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ManuallyPaused = false
		}

		err := gm.ManualActivateGroupWithForce(groupName, true)
		if err != nil {
			t.Errorf("Force activation should succeed: %v", err)
		}
	})

	t.Run("Group details include force activation info", func(t *testing.T) {
		// Force activate with no healthy endpoints
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// Reset group state
		for _, group := range gm.groups {
			group.IsActive = false
			group.ForcedActivation = false
			group.ForcedActivationTime = time.Time{}
			group.ManuallyPaused = false
		}

		err := gm.ManualActivateGroupWithForce(groupName, true)
		if err != nil {
			t.Errorf("Force activation failed: %v", err)
		}

		details := gm.GetGroupDetails()
		groups, ok := details["groups"].([]map[string]interface{})
		if !ok || len(groups) == 0 {
			t.Fatal("Expected group details")
		}

		// Find the test group
		var testGroupData map[string]interface{}
		for _, g := range groups {
			if g["name"] == groupName {
				testGroupData = g
				break
			}
		}
		if testGroupData == nil {
			t.Fatal("Expected to find test group in details")
		}

		if testGroupData["forced_activation"] != true {
			t.Error("Group should be marked as force activated in details")
		}
		if testGroupData["activation_type"] != "forced" {
			t.Error("Activation type should be 'forced'")
		}
		if testGroupData["can_force_activate"] != false {
			t.Error("Can force activate should be false for active group")
		}
		if forcedTime, ok := testGroupData["forced_activation_time"].(string); !ok || forcedTime == "" {
			t.Error("Forced activation time should be set")
		}
	})

	t.Run("Normal activation clears force activation flags", func(t *testing.T) {
		// Make endpoints healthy and do normal activation
		endpoints[0].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroup(groupName)
		if err != nil {
			t.Errorf("Normal activation failed: %v", err)
		}

		groups := gm.GetAllGroups()
		var testGroup *GroupInfo
		for _, g := range groups {
			if g.Name == groupName {
				testGroup = g
				break
			}
		}
		if testGroup == nil || !testGroup.IsActive {
			t.Error("Group should be active after normal activation")
		}
		if testGroup != nil && testGroup.ForcedActivation {
			t.Error("Force activation flag should be cleared")
		}
		if testGroup != nil && !testGroup.ForcedActivationTime.IsZero() {
			t.Error("Force activation time should be cleared")
		}
	})
}
