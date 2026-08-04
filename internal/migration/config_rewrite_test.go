package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRewriteConfigToSQLitePreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RewriteConfigToSQLite(path); err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rewritten)
	if strings.Contains(text, "\nendpoints:") || strings.Contains(text, "\ngroup:") {
		t.Fatalf("legacy endpoint/group blocks remain:\n%s", text)
	}
	if !strings.Contains(text, "keep_me: preserved") || !strings.Contains(text, "配置改写必须保留未知字段与注释") {
		t.Fatalf("unknown field or comment lost:\n%s", text)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(rewritten, &parsed); err != nil {
		t.Fatal(err)
	}
	storage, ok := parsed["endpoints_storage"].(map[string]any)
	if !ok || storage["type"] != "sqlite" {
		t.Fatalf("endpoints_storage = %#v", parsed["endpoints_storage"])
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRewriteConfigToSQLiteRemovesLegacyCommentsWithoutDroppingUnrelatedComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`# 根配置说明必须保留。
endpoints_storage:
  type: yaml

# 旧 group 说明必须随区块删除。
group:
  cooldown: 10m

# 旧 endpoints 说明必须随区块删除。
endpoints:
  # 端点内部说明也必须删除。
  - name: legacy
    url: https://legacy.example.test

# 自定义插件说明必须保留。
custom_plugin:
  # 插件开关说明必须保留。
  enabled: true
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RewriteConfigToSQLite(path); err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rewritten)
	for _, removed := range []string{
		"旧 group 说明必须随区块删除",
		"旧 endpoints 说明必须随区块删除",
		"端点内部说明也必须删除",
	} {
		if strings.Contains(text, removed) {
			t.Fatalf("legacy comment remains after block removal: %q\n%s", removed, text)
		}
	}
	for _, preserved := range []string{
		"根配置说明必须保留",
		"自定义插件说明必须保留",
		"插件开关说明必须保留",
	} {
		if !strings.Contains(text, preserved) {
			t.Fatalf("unrelated comment was lost: %q\n%s", preserved, text)
		}
	}
}
