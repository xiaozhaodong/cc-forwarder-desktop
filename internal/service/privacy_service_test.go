package service

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"cc-forwarder/internal/privacy"
	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

func newTestPrivacyService(t *testing.T) *PrivacyService {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := NewPrivacyService(store.NewSQLitePrivacyStore(db))
	if err := svc.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	return svc
}

func enableRedactMode(t *testing.T, svc *PrivacyService) {
	t.Helper()
	ctx := context.Background()
	settings, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("get settings failed: %v", err)
	}
	settings.Mode = privacy.ModeRedact
	if _, err := svc.UpdateSettings(ctx, settings); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}
}

func newOpenAIKeyRuleRecord() *store.PrivacyRuleRecord {
	return &store.PrivacyRuleRecord{
		Enabled:     true,
		Name:        "OpenAI Key",
		Priority:    100,
		MatchType:   "regex",
		Pattern:     `sk-(?:proj-)?[A-Za-z0-9_-]{20,}`,
		Placeholder: "[OpenAI密钥]",
		Action:      "redact",
		ScopeJSON:   "{}",
	}
}

func TestPrivacyServiceDefaultDisabledPassthrough(t *testing.T) {
	svc := newTestPrivacyService(t)
	body := []byte(`{"messages":[{"role":"user","content":"sk-proj-abcdefghijklmnopqrstuvwxyz123456"}]}`)
	result, err := svc.Apply(context.Background(), privacy.Request{Path: "/v1/messages"}, body)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !bytes.Equal(result.Body, body) || result.Action != privacy.ModeDisabled {
		t.Errorf("default disabled must passthrough: %+v", result)
	}
}

func TestPrivacyServiceRuleSaveHotReload(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	enableRedactMode(t, svc)

	versionBefore := svc.SnapshotVersion()
	created, err := svc.CreateRule(ctx, newOpenAIKeyRuleRecord())
	if err != nil {
		t.Fatalf("create rule failed: %v", err)
	}
	if svc.SnapshotVersion() <= versionBefore {
		t.Error("snapshot version must increase after rule save")
	}

	body := []byte(`{"messages":[{"role":"user","content":"key sk-proj-abcdefghijklmnopqrstuvwxyz123456"}]}`)
	result, err := svc.Apply(ctx, privacy.Request{Path: "/v1/messages", UpstreamType: "endpoint"}, body)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !result.Changed || !bytes.Contains(result.Body, []byte("[OpenAI密钥]")) {
		t.Errorf("rule not effective after save: %s", result.Body)
	}

	// 修改 pattern 后无需重启立即生效
	created.Pattern = `nomatch-[0-9]{10}`
	if _, err := svc.UpdateRule(ctx, created); err != nil {
		t.Fatalf("update rule failed: %v", err)
	}
	result, err = svc.Apply(ctx, privacy.Request{Path: "/v1/messages", UpstreamType: "endpoint"}, body)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if result.Changed {
		t.Error("updated pattern must take effect immediately")
	}

	// 禁用规则后恢复透传
	created.Pattern = `sk-(?:proj-)?[A-Za-z0-9_-]{20,}`
	created.Enabled = false
	if _, err := svc.UpdateRule(ctx, created); err != nil {
		t.Fatalf("disable rule failed: %v", err)
	}
	result, err = svc.Apply(ctx, privacy.Request{Path: "/v1/messages", UpstreamType: "endpoint"}, body)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if result.Changed || !bytes.Equal(result.Body, body) {
		t.Error("disabled rule must restore passthrough")
	}
}

func TestPrivacyServiceInvalidRegexRejectedWithoutSnapshotSwap(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	enableRedactMode(t, svc)
	if _, err := svc.CreateRule(ctx, newOpenAIKeyRuleRecord()); err != nil {
		t.Fatalf("create rule failed: %v", err)
	}
	versionBefore := svc.SnapshotVersion()

	bad := newOpenAIKeyRuleRecord()
	bad.Name = "Broken"
	bad.Pattern = `sk-(unclosed`
	if _, err := svc.CreateRule(ctx, bad); err == nil {
		t.Fatal("invalid regex must be rejected")
	}
	if svc.SnapshotVersion() != versionBefore {
		t.Error("failed save must not swap snapshot")
	}

	rules, err := svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("list rules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("invalid rule must not be persisted, got %d rules", len(rules))
	}

	// 现有规则仍然生效
	body := []byte(`{"messages":[{"role":"user","content":"sk-proj-abcdefghijklmnopqrstuvwxyz123456"}]}`)
	result, err := svc.Apply(ctx, privacy.Request{Path: "/v1/messages"}, body)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !result.Changed {
		t.Error("existing snapshot must keep working after rejected save")
	}
}

func TestPrivacyServiceDisabledRuleAllowsInvalidPatternButEnableRejects(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()

	disabled := newOpenAIKeyRuleRecord()
	disabled.Enabled = false
	disabled.Pattern = `sk-(unclosed`
	created, err := svc.CreateRule(ctx, disabled)
	if err != nil {
		t.Fatalf("disabled rule with invalid pattern must be savable: %v", err)
	}

	created.Enabled = true
	if _, err := svc.UpdateRule(ctx, created); err == nil {
		t.Fatal("enabling invalid pattern must be rejected")
	}
}

func TestPrivacyServiceStartupDegradedOnInvalidEnabledRule(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.NewSQLitePrivacyStore(db)
	ctx := context.Background()

	// 直接落一条非法启用规则模拟历史脏数据
	if _, err := st.CreateRule(ctx, &store.PrivacyRuleRecord{
		Enabled: true, Name: "Legacy Broken", Priority: 100,
		MatchType: "regex", Pattern: `sk-(unclosed`, Placeholder: "[x]",
		Action: "redact", ScopeJSON: "{}", Source: "custom",
	}); err != nil {
		t.Fatalf("seed broken rule failed: %v", err)
	}
	if _, err := st.CreateRule(ctx, &store.PrivacyRuleRecord{
		Enabled: true, Name: "Valid", Priority: 110,
		MatchType: "literal", Pattern: "secret", Placeholder: "[x]",
		Action: "redact", ScopeJSON: "{}", Source: "custom",
	}); err != nil {
		t.Fatalf("seed valid rule failed: %v", err)
	}

	svc := NewPrivacyService(st)
	if err := svc.Initialize(ctx); err != nil {
		t.Fatalf("initialize must not fail on degraded rules: %v", err)
	}
	if svc.Status() != PrivacyStatusDegraded {
		t.Errorf("status = %q, want degraded", svc.Status())
	}
	if !strings.Contains(svc.CurrentSnapshot().CompileError, "Legacy Broken") {
		t.Errorf("compile error summary missing: %q", svc.CurrentSnapshot().CompileError)
	}

	// 失败信息写回 compile_error，有效规则仍激活
	rules, err := st.ListRules(ctx)
	if err != nil {
		t.Fatalf("list rules failed: %v", err)
	}
	for _, rule := range rules {
		if rule.Name == "Legacy Broken" && rule.CompileError == "" {
			t.Error("compile_error must be written back for broken enabled rule")
		}
	}
	if len(svc.CurrentSnapshot().Rules) != 1 {
		t.Errorf("active rules = %d, want 1", len(svc.CurrentSnapshot().Rules))
	}
}

func TestPrivacyServiceImportPresetSkipsDuplicates(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()

	created, err := svc.ImportPreset(ctx, privacy.PresetAIAPIKeys)
	if err != nil {
		t.Fatalf("import preset failed: %v", err)
	}
	if len(created) == 0 {
		t.Fatal("preset must create rules")
	}

	again, err := svc.ImportPreset(ctx, privacy.PresetAIAPIKeys)
	if err != nil {
		t.Fatalf("re-import preset failed: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("duplicate names must be skipped, got %d", len(again))
	}

	if _, err := svc.ImportPreset(ctx, "no-such-preset"); err == nil {
		t.Fatal("unknown preset must fail")
	}
}

func TestPrivacyServiceImportPresetSyncsExistingPresetRule(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	enableRedactMode(t, svc)

	if _, err := svc.ImportPreset(ctx, privacy.PresetAIAPIKeys); err != nil {
		t.Fatalf("import preset failed: %v", err)
	}
	rules, err := svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("list rules failed: %v", err)
	}
	var passwordRule *store.PrivacyRuleRecord
	for _, rule := range rules {
		if rule.Name == "密码字段" {
			passwordRule = rule
			break
		}
	}
	if passwordRule == nil {
		t.Fatal("password preset rule not found")
	}

	passwordRule.Pattern = `(?i)(?:\b(?:password|passwd|pwd|passphrase)\b|密码|口令)\s*[:=：]\s*['"]?[^'"\s,;]{6,}['"]?`
	if _, err := svc.UpdateRule(ctx, passwordRule); err != nil {
		t.Fatalf("seed legacy password pattern failed: %v", err)
	}

	changed, err := svc.ImportPreset(ctx, privacy.PresetAIAPIKeys)
	if err != nil {
		t.Fatalf("re-import preset failed: %v", err)
	}
	if len(changed) != 1 || changed[0].Name != "密码字段" {
		t.Fatalf("changed rules = %+v, want password rule only", changed)
	}
	if !strings.Contains(changed[0].Pattern, "?P<redact>") {
		t.Fatalf("password rule pattern was not synced: %s", changed[0].Pattern)
	}

	result := svc.TestText(privacy.Request{Path: "/v1/messages"}, `password = "SuperSecret123"`)
	if string(result.Body) != `password = "[密码]"` {
		t.Fatalf("body = %q", result.Body)
	}
}

func TestPrivacyServiceTestTextIgnoresDisabledMode(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	if _, err := svc.CreateRule(ctx, newOpenAIKeyRuleRecord()); err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	result := svc.TestText(privacy.Request{Path: "/v1/messages"}, "key sk-proj-abcdefghijklmnopqrstuvwxyz123456")
	if result.HitCount != 1 || !strings.Contains(string(result.Body), "[OpenAI密钥]") {
		t.Errorf("test panel must work with mode=disabled: %+v", result)
	}
}

func TestPrivacyServiceRuntimeStats(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	enableRedactMode(t, svc)
	if _, err := svc.CreateRule(ctx, newOpenAIKeyRuleRecord()); err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	body := []byte(`{"messages":[{"role":"user","content":"sk-proj-abcdefghijklmnopqrstuvwxyz123456"}]}`)
	if _, err := svc.Apply(ctx, privacy.Request{Path: "/v1/messages"}, body); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	stats := svc.RuntimeStats()
	if stats.ScanCount != 1 || stats.HitCount != 1 || len(stats.RuleStats) != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestPrivacyServiceRuntimeStatsDeduplicatesByRequestID(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	enableRedactMode(t, svc)
	if _, err := svc.CreateRule(ctx, newOpenAIKeyRuleRecord()); err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	body := []byte(`{"messages":[{"role":"user","content":"sk-proj-abcdefghijklmnopqrstuvwxyz123456"}]}`)
	req := privacy.Request{RequestID: "req-privacy-dedupe", Path: "/v1/messages"}
	for i := 0; i < 2; i++ {
		if _, err := svc.Apply(ctx, req, body); err != nil {
			t.Fatalf("apply %d failed: %v", i, err)
		}
	}

	stats := svc.RuntimeStats()
	if stats.ScanCount != 1 || stats.HitCount != 1 {
		t.Fatalf("duplicate request stats not deduped: %+v", stats)
	}
	if len(stats.RuleStats) != 1 || stats.RuleStats[0].HitCount != 1 {
		t.Fatalf("duplicate rule stats not deduped: %+v", stats.RuleStats)
	}
}

func TestPrivacyServiceExportRules(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	if _, err := svc.CreateRule(ctx, newOpenAIKeyRuleRecord()); err != nil {
		t.Fatalf("create rule failed: %v", err)
	}
	export, err := svc.ExportRules(ctx)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if export.Settings == nil || len(export.Rules) != 1 {
		t.Errorf("unexpected export: %+v", export)
	}
}

func TestPrivacyServiceUpdateRulePreservesPresetSourceWhenInputOmitsSource(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()

	created, err := svc.ImportPreset(ctx, privacy.PresetAIAPIKeys)
	if err != nil {
		t.Fatalf("import preset failed: %v", err)
	}
	if len(created) == 0 {
		t.Fatal("preset must create rules")
	}
	rule := created[0]
	if rule.Source != privacy.SourcePreset {
		t.Fatalf("imported source = %q, want preset", rule.Source)
	}

	rule.Enabled = false
	rule.Source = "" // 模拟 Wails API 更新输入不携带 source
	updated, err := svc.UpdateRule(ctx, rule)
	if err != nil {
		t.Fatalf("update rule failed: %v", err)
	}
	if updated.Source != privacy.SourcePreset {
		t.Fatalf("source after update = %q, want preset", updated.Source)
	}
}

func TestPrivacyServiceUpdateSettingsValidation(t *testing.T) {
	svc := newTestPrivacyService(t)
	ctx := context.Background()
	settings, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("get settings failed: %v", err)
	}
	settings.Mode = "invalid-mode"
	if _, err := svc.UpdateSettings(ctx, settings); err == nil {
		t.Fatal("invalid mode must be rejected")
	}
}
