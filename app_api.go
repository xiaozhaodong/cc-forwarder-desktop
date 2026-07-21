// app_api.go - 暴露给前端的 API 方法 (Wails Bindings)
// 这些方法会被自动生成为 JavaScript 调用
//
// API 文件按功能模块拆分:
// - app_api.go         - 系统状态、配置、辅助函数 (本文件)
// - app_api_endpoint.go - 端点管理 + Key 管理
// - app_api_group.go    - 组管理
// - app_api_usage.go    - 使用统计 + 请求记录
// - app_api_chart.go    - 图表数据
// - app_api_storage.go  - v5.0+ 端点存储管理 (SQLite)

package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// 端口检测缓存（避免频繁 TCP 连接）
// 使用 map 绑定到具体的 host:port，避免配置更改后返回旧端口状态
type portCheckCacheEntry struct {
	result    bool
	timestamp time.Time
}

var (
	portCheckCacheMap = make(map[string]portCheckCacheEntry) // key: "host:port"
	portCheckCacheMu  sync.RWMutex
	portCheckCacheTTL = 500 * time.Millisecond // 缓存有效期
)

// ============================================================
// 系统状态 API
// ============================================================

// SystemStatus 系统状态结构
type SystemStatus struct {
	Version       string `json:"version"`
	Uptime        string `json:"uptime"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	StartTime     string `json:"start_time"` // ISO8601 格式的启动时间
	ProxyPort     int    `json:"proxy_port"`
	ProxyHost     string `json:"proxy_host"`
	ProxyRunning  bool   `json:"proxy_running"`
	ActiveGroup   string `json:"active_group"`
	ConfigPath    string `json:"config_path"`
	AuthEnabled   bool   `json:"auth_enabled"`
}

// GetSystemStatus 获取系统状态
func (a *App) GetSystemStatus() SystemStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	uptime := time.Since(a.startTime)

	status := SystemStatus{
		Version:       Version,
		Uptime:        formatDuration(uptime),
		UptimeSeconds: int64(uptime.Seconds()),
		StartTime:     a.startTime.Format(time.RFC3339),
		ProxyRunning:  a.isRunning && a.proxyServer != nil,
		ConfigPath:    a.configPath,
	}

	if a.config != nil {
		status.ProxyPort = a.config.Server.Port
		status.ProxyHost = a.config.Server.Host
		status.AuthEnabled = a.config.Auth.Enabled

		// 真正检测端口是否在监听
		if status.ProxyRunning {
			status.ProxyRunning = a.checkPortListening(status.ProxyHost, status.ProxyPort)
		}
	}

	if a.endpointManager != nil {
		status.ActiveGroup = a.endpointManager.GetActiveGroupName()
	}

	return status
}

// checkPortListening 检测端口是否在监听（带缓存，避免频繁 TCP 连接）
func (a *App) checkPortListening(host string, port int) bool {
	// 使用 net.JoinHostPort 正确处理 IPv6 地址
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	// 先检查缓存（绑定到具体的 host:port）
	portCheckCacheMu.RLock()
	if entry, ok := portCheckCacheMap[addr]; ok {
		if time.Since(entry.timestamp) < portCheckCacheTTL {
			portCheckCacheMu.RUnlock()
			return entry.result
		}
	}
	portCheckCacheMu.RUnlock()

	// 缓存过期或不存在，执行真实检测
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)

	result := err == nil
	if conn != nil {
		conn.Close()
	}

	// 更新缓存
	portCheckCacheMu.Lock()
	portCheckCacheMap[addr] = portCheckCacheEntry{
		result:    result,
		timestamp: time.Now(),
	}
	portCheckCacheMu.Unlock()

	return result
}

// ============================================================
// 配置 API
// ============================================================

// ConfigInfo 配置信息（脱敏）
type ConfigInfo struct {
	ServerHost      string `json:"server_host"`
	ServerPort      int    `json:"server_port"`
	AuthEnabled     bool   `json:"auth_enabled"`
	ProxyEnabled    bool   `json:"proxy_enabled"`
	TrackingEnabled bool   `json:"tracking_enabled"`
	FailoverEnabled bool   `json:"failover_enabled"`
	EndpointCount   int    `json:"endpoint_count"`
}

// GetConfig 获取当前配置（脱敏）
func (a *App) GetConfig() ConfigInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.config == nil {
		return ConfigInfo{}
	}

	return ConfigInfo{
		ServerHost:      a.config.Server.Host,
		ServerPort:      a.config.Server.Port,
		AuthEnabled:     a.config.Auth.Enabled,
		ProxyEnabled:    a.config.Proxy.Enabled,
		TrackingEnabled: a.config.UsageTracking.Enabled,
		FailoverEnabled: a.config.Failover.Enabled,
		EndpointCount:   len(a.config.Endpoints),
	}
}

// ============================================================
// 系统功能 API
// ============================================================

// GetProxyURL 获取代理 URL
func (a *App) GetProxyURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.config == nil {
		return ""
	}

	return fmt.Sprintf("http://%s:%d", a.config.Server.Host, a.config.Server.Port)
}

// IsProxyRunning 检查代理是否运行中
func (a *App) IsProxyRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.isRunning
}

// ============================================================
// 辅助函数
// ============================================================

// formatDuration 格式化时长
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// parseTimeWithLocation 解析时间字符串（支持多种格式）
// 支持格式:
//   - 2025-12-05T00:00 (不带时区，使用 loc)
//   - 2025-12-05T00:00:00 (不带时区，使用 loc)
//   - 2025-12-05T00:00:00+08:00 (带时区)
//   - 2025-12-05 00:00:00 (简单格式，使用 loc)
//   - 2025-12-05 (仅日期，当天开始)
func parseTimeWithLocation(s string, loc *time.Location) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time string")
	}

	// 尝试各种格式
	formats := []struct {
		layout string
		useLoc bool // 是否使用 loc
	}{
		{"2006-01-02T15:04:05Z07:00", false}, // RFC3339
		{"2006-01-02T15:04:05+08:00", false}, // 带固定时区
		{"2006-01-02T15:04:05", true},        // 不带时区
		{"2006-01-02T15:04", true},           // 不带秒
		{"2006-01-02 15:04:05", true},        // 简单格式
		{"2006-01-02 15:04", true},           // 简单格式不带秒
		{"2006-01-02", true},                 // 仅日期
	}

	for _, f := range formats {
		var t time.Time
		var err error

		if f.useLoc && loc != nil {
			t, err = time.ParseInLocation(f.layout, s, loc)
		} else {
			t, err = time.Parse(f.layout, s)
		}

		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}
