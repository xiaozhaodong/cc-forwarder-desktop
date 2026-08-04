// Package timezone 定义数据库时间的存储与读取契约。
//
// 存储：所有真实时间点统一为固定微秒精度 UTC 文本（StorageLayout），
// 由 DBTime / NullDBTime 在写入时完成格式化。
//
// 读取：SQL 必须以 CAST(col AS TEXT) 选取时间列。schema 中时间列声明为
// DATETIME，SQLite 驱动会对该声明类型的裸查询自动把文本解析成 time.Time：
// 解析规则不受本包控制（会剥离 Z、逐个猜格式），且 mattn 驱动在解析失败时
// 会静默返回零值时间。因此 DBTime.Scan 拒收 time.Time，强制读取路径经过
// ParseStorage 显式校验——违反约定的查询会在第一行立刻报错，
// 而不是随数据静默出错。
package timezone

import (
	"database/sql/driver"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	StorageLayout = "2006-01-02T15:04:05.000000Z"
	displayLayout = "2006-01-02T15:04:05.000000-07:00"
)

type policySnapshot struct {
	name     string
	location *time.Location
}

// Policy 是配置时区的原子快照。Location 返回的 time.Location 是不可变对象，
// 因此读取方可以在一次操作中安全地保留同一个快照。
type Policy struct {
	state atomic.Pointer[policySnapshot]
}

func New(name string) (*Policy, error) {
	p := &Policy{}
	if err := p.Update(name); err != nil {
		return nil, err
	}
	return p, nil
}

func Load(name string) (*time.Location, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("timezone is required")
	}
	location, err := time.LoadLocation(trimmed)
	if err != nil {
		return nil, fmt.Errorf("load IANA timezone %q: %w", trimmed, err)
	}
	return location, nil
}

func (p *Policy) Update(name string) error {
	location, err := Load(name)
	if err != nil {
		return err
	}
	p.state.Store(&policySnapshot{name: strings.TrimSpace(name), location: location})
	return nil
}

func (p *Policy) Name() string {
	if snapshot := p.snapshot(); snapshot != nil {
		return snapshot.name
	}
	return ""
}

func (p *Policy) Location() *time.Location {
	if snapshot := p.snapshot(); snapshot != nil {
		return snapshot.location
	}
	return time.UTC
}

func (p *Policy) snapshot() *policySnapshot {
	if p == nil {
		return nil
	}
	return p.state.Load()
}

// Snapshot 固定当前活动时区，供一次查询、导出或汇总重建在热重载期间保持同一口径。
func (p *Policy) Snapshot() *Policy {
	current := &Policy{}
	if snapshot := p.snapshot(); snapshot != nil {
		current.state.Store(snapshot)
	}
	return current
}

func FormatStorage(value time.Time) string {
	return value.UTC().Format(StorageLayout)
}

func ParseStorage(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("database time is empty")
	}
	for _, layout := range []string{
		StorageLayout,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported database time %q", value)
}

func (p *Policy) ParseInput(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("time input is empty")
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}

	var wall time.Time
	var err error
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		wall, err = time.Parse(layout, value)
		if err == nil {
			return resolveWallTime(wall, p.Location())
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time input %q", value)
}

func (p *Policy) FormatDisplay(value time.Time) string {
	return value.In(p.Location()).Format(displayLayout)
}

func (p *Policy) BusinessDate(value time.Time) string {
	return value.In(p.Location()).Format(time.DateOnly)
}

func (p *Policy) DayRange(date string) (time.Time, time.Time, error) {
	wall, err := time.Parse(time.DateOnly, strings.TrimSpace(date))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse business date %q: %w", date, err)
	}
	start, err := resolveWallTime(wall, p.Location())
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("resolve start of business date %q: %w", date, err)
	}
	nextWall := wall.AddDate(0, 0, 1)
	end, err := resolveWallTime(nextWall, p.Location())
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("resolve end of business date %q: %w", date, err)
	}
	return start.UTC(), end.UTC(), nil
}

func resolveWallTime(wall time.Time, location *time.Location) (time.Time, error) {
	if location == nil {
		return time.Time{}, fmt.Errorf("timezone location is nil")
	}
	wallUTC := time.Date(
		wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.UTC,
	)
	offsets := candidateOffsets(wallUTC, location)
	candidates := make([]time.Time, 0, len(offsets))
	for offset := range offsets {
		candidate := wallUTC.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(location)
		if sameWallTime(local, wall) {
			candidates = append(candidates, candidate.UTC())
		}
	}
	if len(candidates) == 0 {
		return time.Time{}, fmt.Errorf("local time %s does not exist in %s", wall.Format("2006-01-02 15:04:05.999999999"), location)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })
	return candidates[0], nil
}

func candidateOffsets(wallUTC time.Time, location *time.Location) map[int]struct{} {
	offsets := make(map[int]struct{})
	// 时区跳变通常小于一天；扩大到前后 48 小时可覆盖日期线调整并保持实现确定。
	for hour := -48; hour <= 48; hour++ {
		_, offset := wallUTC.Add(time.Duration(hour) * time.Hour).In(location).Zone()
		offsets[offset] = struct{}{}
	}
	return offsets
}

func sameWallTime(actual, expected time.Time) bool {
	return actual.Year() == expected.Year() &&
		actual.Month() == expected.Month() &&
		actual.Day() == expected.Day() &&
		actual.Hour() == expected.Hour() &&
		actual.Minute() == expected.Minute() &&
		actual.Second() == expected.Second() &&
		actual.Nanosecond() == expected.Nanosecond()
}

type DBTime struct {
	Time time.Time
}

func (t *DBTime) Scan(value any) error {
	if t == nil {
		return fmt.Errorf("scan DBTime into nil receiver")
	}
	if value == nil {
		return fmt.Errorf("scan NULL into DBTime")
	}
	parsed, err := parseDriverTime(value)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func (t DBTime) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, fmt.Errorf("store zero DBTime")
	}
	return FormatStorage(t.Time), nil
}

type NullDBTime struct {
	Time  time.Time
	Valid bool
}

func (t *NullDBTime) Scan(value any) error {
	if t == nil {
		return fmt.Errorf("scan NullDBTime into nil receiver")
	}
	if value == nil {
		t.Time = time.Time{}
		t.Valid = false
		return nil
	}
	parsed, err := parseDriverTime(value)
	if err != nil {
		return err
	}
	t.Time = parsed
	t.Valid = true
	return nil
}

func (t NullDBTime) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	if t.Time.IsZero() {
		return nil, fmt.Errorf("store zero valid NullDBTime")
	}
	return FormatStorage(t.Time), nil
}

func parseDriverTime(value any) (time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		return time.Time{}, fmt.Errorf("database driver normalized time %q before validation; select the field as TEXT", typed.Format(time.RFC3339Nano))
	case string:
		return ParseStorage(typed)
	case []byte:
		return ParseStorage(string(typed))
	default:
		return time.Time{}, fmt.Errorf("unsupported database time type %T", value)
	}
}
