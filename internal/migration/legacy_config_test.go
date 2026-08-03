package migration

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadLegacyConfigFixture(t *testing.T) {
	config, err := LoadLegacyConfig(filepath.Join("testdata", "legacy_config.yml"))
	if err != nil {
		t.Fatalf("LoadLegacyConfig() error = %v", err)
	}
	if config.SourceMode != SourceModeYAML {
		t.Fatalf("SourceMode = %q, want yaml", config.SourceMode)
	}
	if len(config.Endpoints) != 5 {
		t.Fatalf("endpoint count = %d, want 5", len(config.Endpoints))
	}
	if got := config.Endpoints[0].Timeout.Duration; got != 45*time.Second {
		t.Fatalf("first endpoint timeout = %v, want 45s", got)
	}
	if got := config.ResolveDatabasePath("fallback.db"); got != "/tmp/ai-switchboard-migration-fixture/usage.db" {
		t.Fatalf("ResolveDatabasePath() = %q", got)
	}
}
