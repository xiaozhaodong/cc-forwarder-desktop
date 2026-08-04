package timezone

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/thlib/go-timezone-local/tzlocal"
)

// FallbackName 是系统时区探测失败时的兜底时区，
// 同时也是迁移解释无 offset 历史数据的钉死口径（见 migration.LegacyTrackingTimezone）。
const FallbackName = "Asia/Shanghai"

// localSentinel 允许配置显式声明“跟随系统时区”。
const localSentinel = "local"

// DetectSystem 探测本机 IANA 时区名称；探测失败或名称非法时返回错误，由调用方决定兜底策略。
func DetectSystem() (string, error) {
	name, err := tzlocal.RuntimeTZ()
	if err != nil {
		return "", fmt.Errorf("detect system timezone: %w", err)
	}
	if _, err := Load(name); err != nil {
		return "", fmt.Errorf("system timezone %q is not loadable: %w", name, err)
	}
	return name, nil
}

// SystemDefault 探测本机 IANA 时区名称；探测失败时回退 FallbackName。
func SystemDefault() string {
	name, err := DetectSystem()
	if err != nil {
		slog.Warn("探测系统时区失败，回退默认时区", "fallback", FallbackName, "error", err)
		return FallbackName
	}
	return name
}

// ResolveConfigured 把配置值解析为具体 IANA 时区名称：
// 留空或 "local" 表示跟随系统时区，其余原样返回（合法性由调用方校验）。
func ResolveConfigured(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || strings.EqualFold(trimmed, localSentinel) {
		return SystemDefault()
	}
	return trimmed
}
