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
