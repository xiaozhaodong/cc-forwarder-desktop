package endpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cc-forwarder/config"
)

// LogEntry 用于解析结构化日志
type LogEntry struct {
	Time    string                 `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"msg"`
	Source  map[string]interface{} `json:"source,omitempty"`
}

// TestEmergencyActivationLogging 专门测试应急激活功能的日志记录
// v4.0: 适配一端点一组架构
func TestEmergencyActivationLogging(t *testing.T) {
	// 设置测试用的日志缓冲区
	var logBuffer bytes.Buffer

	// 创建一个自定义的日志处理器，输出到缓冲区
	jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// 设置全局日志器
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(jsonHandler))

	// 测试完成后恢复原始日志器
	defer slog.SetDefault(originalLogger)

	// 创建测试配置
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

	// 创建组管理器
	gm := NewGroupManager(cfg)

	// v4.0: 创建测试端点，每个端点自动成为一个独立的组
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:     "unhealthy-endpoint",
				URL:      "https://api.example.com",
				Priority: 1,
			},
			Status: EndpointStatus{
				Healthy: false,
			},
		},
		{
			Config: config.EndpointConfig{
				Name:     "healthy-endpoint",
				URL:      "https://healthy.example.com",
				Priority: 2,
			},
			Status: EndpointStatus{
				Healthy: true,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("验证正常激活日志记录", func(t *testing.T) {
		t.Log("=== 测试正常激活日志记录 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// v4.0: 组名就是端点名
		err := gm.ManualActivateGroupWithForce("healthy-endpoint", false)
		require.NoError(t, err, "正常激活应该成功")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("实际日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 查找正常激活日志
		var normalActivationLog *LogEntry
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "正常激活") && strings.Contains(entry.Message, "healthy-endpoint") {
				normalActivationLog = entry
				break
			}
		}

		require.NotNil(t, normalActivationLog, "应该找到正常激活日志")

		// 验证日志级别
		assert.Equal(t, "INFO", normalActivationLog.Level, "正常激活应该使用INFO级别")

		// 验证日志格式
		expectedPattern := "🔄 [正常激活] 手动激活组: healthy-endpoint (端点数: 1)"
		assert.Equal(t, expectedPattern, normalActivationLog.Message, "正常激活日志格式应该符合设计文档")

		assert.True(t, strings.HasPrefix(normalActivationLog.Message, "🔄"), "正常激活日志应该以🔄开头")
		assert.Contains(t, normalActivationLog.Message, "healthy-endpoint", "日志应该包含组名")
		assert.Contains(t, normalActivationLog.Message, "端点数: 1", "日志应该包含端点数量")

		t.Logf("✅ 正常激活日志验证成功:")
		t.Logf("   - 级别: %s", normalActivationLog.Level)
		t.Logf("   - 消息: %s", normalActivationLog.Message)
	})

	t.Run("验证应急激活日志记录", func(t *testing.T) {
		t.Log("=== 测试应急激活日志记录 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 确保unhealthy-endpoint不健康
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// v4.0: 执行应急激活，组名就是端点名
		err := gm.ManualActivateGroupWithForce("unhealthy-endpoint", true)
		require.NoError(t, err, "应急激活应该成功")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("实际日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 查找应急激活WARN日志
		var emergencyWarnLog *LogEntry
		var safetyErrorLog *LogEntry

		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && strings.Contains(entry.Message, "unhealthy-endpoint") && entry.Level == "WARN" {
				emergencyWarnLog = entry
			}
			if strings.Contains(entry.Message, "安全警告") && strings.Contains(entry.Message, "unhealthy-endpoint") && entry.Level == "ERROR" {
				safetyErrorLog = entry
			}
		}

		require.NotNil(t, emergencyWarnLog, "应该找到应急激活WARN日志")
		require.NotNil(t, safetyErrorLog, "应该找到安全警告ERROR日志")

		// 验证WARN级别日志
		t.Log("验证WARN级别日志...")
		assert.Equal(t, "WARN", emergencyWarnLog.Level, "应急激活应该使用WARN级别")

		// 验证WARN日志格式和内容
		assert.True(t, strings.HasPrefix(emergencyWarnLog.Message, "⚠️ [强制激活]"), "WARN日志应该以⚠️ [强制激活]开头")
		assert.Contains(t, emergencyWarnLog.Message, "用户强制激活组: unhealthy-endpoint", "应该包含组名信息")
		assert.Contains(t, emergencyWarnLog.Message, "端点数: 1", "应该包含端点数量")
		assert.Contains(t, emergencyWarnLog.Message, "操作时间:", "应该包含操作时间")
		assert.Contains(t, emergencyWarnLog.Message, "风险等级: HIGH", "应该包含风险等级")

		// 验证ERROR级别日志
		t.Log("验证ERROR级别日志...")
		assert.Equal(t, "ERROR", safetyErrorLog.Level, "安全警告应该使用ERROR级别")

		// 验证ERROR日志格式和内容
		expectedErrorPattern := "🚨 [安全警告] 强制激活可能导致请求失败! 组: unhealthy-endpoint, 建议尽快检查端点健康状态"
		assert.Equal(t, expectedErrorPattern, safetyErrorLog.Message, "ERROR日志格式应该符合设计文档")

		assert.True(t, strings.HasPrefix(safetyErrorLog.Message, "🚨 [安全警告]"), "ERROR日志应该以🚨 [安全警告]开头")
		assert.Contains(t, safetyErrorLog.Message, "可能导致请求失败", "应该包含风险警告")
		assert.Contains(t, safetyErrorLog.Message, "建议尽快检查端点健康状态", "应该包含建议")

		t.Logf("✅ 应急激活日志验证成功:")
		t.Logf("   - WARN日志级别: %s", emergencyWarnLog.Level)
		t.Logf("   - WARN日志消息: %s", emergencyWarnLog.Message)
		t.Logf("   - ERROR日志级别: %s", safetyErrorLog.Level)
		t.Logf("   - ERROR日志消息: %s", safetyErrorLog.Message)
	})

	t.Run("强制激活不再被拒绝", func(t *testing.T) {
		t.Log("=== 测试强制激活不再被拒绝 ===")

		logBuffer.Reset()

		endpoints[1].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		gm.ManualPauseGroup("healthy-endpoint", 0)

		err := gm.ManualActivateGroupWithForce("healthy-endpoint", true)
		assert.NoError(t, err, "强制激活应该成功")

		logContent := logBuffer.String()
		t.Logf("实际日志输出:\n%s", logContent)

		logEntries := parseLogEntries(t, logContent)

		hasForceWarn := false
		hasSecurityError := false
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && strings.Contains(entry.Message, "healthy-endpoint") && entry.Level == "WARN" {
				hasForceWarn = true
			}
			if strings.Contains(entry.Message, "安全警告") && strings.Contains(entry.Message, "healthy-endpoint") && entry.Level == "ERROR" {
				hasSecurityError = true
			}
		}

		assert.True(t, hasForceWarn, "应该有强制激活WARN日志")
		assert.True(t, hasSecurityError, "应该有安全警告ERROR日志")
	})

	t.Run("验证应急激活完整日志序列", func(t *testing.T) {
		t.Log("=== 测试应急激活完整日志序列 ===")

		// 清空日志缓冲区
		logBuffer.Reset()

		// 确保unhealthy-endpoint不健康
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// v4.0: 执行应急激活
		err := gm.ManualActivateGroupWithForce("unhealthy-endpoint", true)
		require.NoError(t, err, "应急激活应该成功")

		// 获取日志内容
		logContent := logBuffer.String()
		t.Logf("完整日志输出:\n%s", logContent)

		// 解析日志条目
		logEntries := parseLogEntries(t, logContent)

		// 验证日志序列：应该先有WARN日志，然后有ERROR日志
		var warnLogIndex, errorLogIndex int = -1, -1

		for i, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && entry.Level == "WARN" {
				warnLogIndex = i
			}
			if strings.Contains(entry.Message, "安全警告") && entry.Level == "ERROR" {
				errorLogIndex = i
			}
		}

		assert.NotEqual(t, -1, warnLogIndex, "应该有WARN级别的强制激活日志")
		assert.NotEqual(t, -1, errorLogIndex, "应该有ERROR级别的安全警告日志")
		assert.True(t, warnLogIndex < errorLogIndex, "WARN日志应该在ERROR日志之前")

		t.Logf("✅ 完整日志序列验证成功:")
		t.Logf("   - WARN日志位置: %d", warnLogIndex)
		t.Logf("   - ERROR日志位置: %d", errorLogIndex)
		t.Logf("   - 日志序列正确: %v", warnLogIndex < errorLogIndex)
	})
}

// parseLogEntries 解析JSON格式的日志条目
func parseLogEntries(t *testing.T, logContent string) []*LogEntry {
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	var entries []*LogEntry

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Logf("警告: 无法解析日志行: %s, 错误: %v", line, err)
			continue
		}

		entries = append(entries, &entry)
	}

	return entries
}

// TestLogFormatCompliance 测试日志格式是否符合设计文档要求
// v4.0: 适配一端点一组架构
func TestLogFormatCompliance(t *testing.T) {
	t.Log("=== 日志格式符合性测试 ===")

	// 设计文档中定义的日志格式
	expectedFormats := map[string]string{
		"normal_activation": "🔄 [正常激活] 手动激活组: %s (端点数: %d)",
		"force_activation":  "⚠️ [强制激活] 用户强制激活组: %s (端点数: %d, 操作时间: %s, 风险等级: HIGH)",
		"safety_warning":    "🚨 [安全警告] 强制激活可能导致请求失败! 组: %s, 建议尽快检查端点健康状态",
	}

	// 创建测试配置
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

	// v4.0: 创建测试端点
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:     "test-endpoint",
				URL:      "https://test.example.com",
				Priority: 1,
			},
			Status: EndpointStatus{
				Healthy: true,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("正常激活格式验证", func(t *testing.T) {
		// 设置日志缓冲区
		var logBuffer bytes.Buffer
		jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		originalLogger := slog.Default()
		slog.SetDefault(slog.New(jsonHandler))
		defer slog.SetDefault(originalLogger)

		// v4.0: 执行正常激活，组名就是端点名
		err := gm.ManualActivateGroupWithForce("test-endpoint", false)
		require.NoError(t, err)

		// 检查日志格式
		logContent := logBuffer.String()
		logEntries := parseLogEntries(t, logContent)

		var normalLog *LogEntry
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "正常激活") {
				normalLog = entry
				break
			}
		}

		require.NotNil(t, normalLog, "应该找到正常激活日志")

		// 验证格式匹配
		expectedMsg := fmt.Sprintf(expectedFormats["normal_activation"], "test-endpoint", 1)
		assert.Equal(t, expectedMsg, normalLog.Message, "正常激活日志格式应该完全匹配设计文档")

		t.Logf("✅ 正常激活格式验证通过: %s", normalLog.Message)
	})

	t.Run("应急激活格式验证", func(t *testing.T) {
		// 设置日志缓冲区
		var logBuffer bytes.Buffer
		jsonHandler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		originalLogger := slog.Default()
		slog.SetDefault(slog.New(jsonHandler))
		defer slog.SetDefault(originalLogger)

		// 让端点变为不健康
		endpoints[0].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		// v4.0: 执行应急激活
		err := gm.ManualActivateGroupWithForce("test-endpoint", true)
		require.NoError(t, err)

		// 检查日志格式
		logContent := logBuffer.String()
		logEntries := parseLogEntries(t, logContent)

		var forceLog, warningLog *LogEntry
		for _, entry := range logEntries {
			if strings.Contains(entry.Message, "强制激活") && entry.Level == "WARN" {
				forceLog = entry
			}
			if strings.Contains(entry.Message, "安全警告") && entry.Level == "ERROR" {
				warningLog = entry
			}
		}

		require.NotNil(t, forceLog, "应该找到强制激活日志")
		require.NotNil(t, warningLog, "应该找到安全警告日志")

		// 由于时间可能有细微差异，我们分别验证各个部分
		assert.Contains(t, forceLog.Message, "⚠️ [强制激活] 用户强制激活组: test-endpoint", "强制激活日志应该包含正确的前缀")
		assert.Contains(t, forceLog.Message, "端点数: 1", "应该包含正确的端点统计")
		assert.Contains(t, forceLog.Message, "操作时间:", "应该包含操作时间")
		assert.Contains(t, forceLog.Message, "风险等级: HIGH", "应该包含风险等级")

		// 验证安全警告日志格式
		expectedWarningMsg := fmt.Sprintf(expectedFormats["safety_warning"], "test-endpoint")
		assert.Equal(t, expectedWarningMsg, warningLog.Message, "安全警告日志格式应该完全匹配设计文档")

		t.Logf("✅ 应急激活格式验证通过:")
		t.Logf("   - 强制激活日志: %s", forceLog.Message)
		t.Logf("   - 安全警告日志: %s", warningLog.Message)
	})
}
