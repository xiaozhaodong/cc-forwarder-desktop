package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type SourceMode string

const (
	SourceModeYAML   SourceMode = "yaml"
	SourceModeSQLite SourceMode = "sqlite"
)

type legacyDuration struct{ time.Duration }

func (d *legacyDuration) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || strings.TrimSpace(node.Value) == "" {
		return nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(node.Value))
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

type LegacyCredential struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type LegacyEndpoint struct {
	Name                          string             `yaml:"name"`
	URL                           string             `yaml:"url"`
	Channel                       string             `yaml:"channel"`
	Group                         string             `yaml:"group"`
	GroupPriority                 int                `yaml:"group-priority"`
	Priority                      int                `yaml:"priority"`
	Token                         string             `yaml:"token"`
	APIKey                        string             `yaml:"api-key"`
	Tokens                        []LegacyCredential `yaml:"tokens"`
	APIKeys                       []LegacyCredential `yaml:"api-keys"`
	Headers                       map[string]string  `yaml:"headers"`
	Timeout                       legacyDuration     `yaml:"timeout"`
	Cooldown                      *legacyDuration    `yaml:"cooldown"`
	SupportsCountTokens           bool               `yaml:"supports_count_tokens"`
	ModelRewriteRules             string             `yaml:"model_rewrite_rules"`
	Enabled                       *bool              `yaml:"enabled"`
	AvailabilityEnabled           *bool              `yaml:"availability_enabled"`
	FailoverEnabled               *bool              `yaml:"failover_enabled"`
	CostMultiplier                float64            `yaml:"cost_multiplier"`
	InputCostMultiplier           float64            `yaml:"input_cost_multiplier"`
	OutputCostMultiplier          float64            `yaml:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64            `yaml:"cache_creation_cost_multiplier"`
	CacheCreationCostMultiplier1h float64            `yaml:"cache_creation_cost_multiplier_1h"`
	CacheReadCostMultiplier       float64            `yaml:"cache_read_cost_multiplier"`
}

type legacyConfigDocument struct {
	EndpointsStorage struct {
		Type string `yaml:"type"`
	} `yaml:"endpoints_storage"`
	UsageTracking struct {
		DatabasePath string `yaml:"database_path"`
		Database     struct {
			Path     string `yaml:"path"`
			Timezone string `yaml:"timezone"`
		} `yaml:"database"`
	} `yaml:"usage_tracking"`
	GlobalTimeout legacyDuration   `yaml:"global_timeout"`
	Endpoints     []LegacyEndpoint `yaml:"endpoints"`
}

type LegacyConfig struct {
	Path             string
	Raw              []byte
	Root             yaml.Node
	SourceMode       SourceMode
	DatabasePath     string
	DatabaseTimezone string
	GlobalTimeout    time.Duration
	Endpoints        []LegacyEndpoint
}

func LoadLegacyConfig(path string) (*LegacyConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy config: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse legacy config node: %w", err)
	}
	var document legacyConfigDocument
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse legacy config values: %w", err)
	}
	mode := SourceModeYAML
	if strings.EqualFold(strings.TrimSpace(document.EndpointsStorage.Type), string(SourceModeSQLite)) {
		mode = SourceModeSQLite
	}
	databasePath := strings.TrimSpace(document.UsageTracking.Database.Path)
	if databasePath == "" {
		databasePath = strings.TrimSpace(document.UsageTracking.DatabasePath)
	}
	return &LegacyConfig{
		Path:             path,
		Raw:              raw,
		Root:             root,
		SourceMode:       mode,
		DatabasePath:     databasePath,
		DatabaseTimezone: strings.TrimSpace(document.UsageTracking.Database.Timezone),
		GlobalTimeout:    document.GlobalTimeout.Duration,
		Endpoints:        document.Endpoints,
	}, nil
}

func (c *LegacyConfig) ResolveDatabasePath(defaultPath string) string {
	if c == nil || strings.TrimSpace(c.DatabasePath) == "" {
		return defaultPath
	}
	if filepath.IsAbs(c.DatabasePath) {
		return filepath.Clean(c.DatabasePath)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(c.Path), c.DatabasePath))
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func positiveOrDefault(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}
