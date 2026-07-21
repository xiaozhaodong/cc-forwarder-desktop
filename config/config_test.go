package config

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestDynamicTokenResolution(t *testing.T) {
	configContent := `
server:
  host: "localhost"
  port: 8080

strategy:
  type: "priority"

endpoints:
  # Group 1: main - first endpoint defines the group token
  - name: "main-primary"
    url: "https://api1.example.com"
    group: "main"
    group-priority: 1
    priority: 1
    token: "sk-main-token"
    timeout: "30s"

  - name: "main-backup"
    url: "https://api2.example.com"
    priority: 2
    # No token defined - should dynamically resolve to "sk-main-token" at runtime

  # Group 2: backup - first endpoint defines the group token  
  - name: "backup-primary"
    url: "https://api3.example.com"
    group: "backup"
    group-priority: 2
    priority: 1
    token: "sk-backup-token"
    timeout: "45s"

  - name: "backup-secondary"
    url: "https://api4.example.com"
    priority: 2
    # No token defined - should dynamically resolve to "sk-backup-token" at runtime

  # Group 3: with explicit token override
  - name: "override-primary"
    url: "https://api5.example.com"
    group: "override"
    group-priority: 3
    priority: 1
    token: "sk-group-token"

  - name: "override-custom"
    url: "https://api6.example.com"
    priority: 2
    token: "sk-custom-token"
    # Has its own token - should use its own, not inherit from group
`

	tmpFile, err := os.CreateTemp("", "test-dynamic-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify that tokens are NOT inherited during config parsing (dynamic resolution)
	// Group 1: main
	mainPrimary := config.Endpoints[0]
	if mainPrimary.Token != "sk-main-token" {
		t.Errorf("main-primary: expected own token 'sk-main-token', got '%s'", mainPrimary.Token)
	}

	mainBackup := config.Endpoints[1]
	if mainBackup.Token != "" {
		t.Errorf("main-backup: expected no static token (for dynamic resolution), got '%s'", mainBackup.Token)
	}

	// Group 2: backup
	backupPrimary := config.Endpoints[2]
	if backupPrimary.Token != "sk-backup-token" {
		t.Errorf("backup-primary: expected own token 'sk-backup-token', got '%s'", backupPrimary.Token)
	}

	backupSecondary := config.Endpoints[3]
	if backupSecondary.Token != "" {
		t.Errorf("backup-secondary: expected no static token (for dynamic resolution), got '%s'", backupSecondary.Token)
	}

	// Group 3: override scenarios
	overridePrimary := config.Endpoints[4]
	if overridePrimary.Token != "sk-group-token" {
		t.Errorf("override-primary: expected own token 'sk-group-token', got '%s'", overridePrimary.Token)
	}

	overrideCustom := config.Endpoints[5]
	if overrideCustom.Token != "sk-custom-token" {
		t.Errorf("override-custom: expected own token 'sk-custom-token', got '%s'", overrideCustom.Token)
	}

	// Verify group assignments are still working
	if mainBackup.Group != "main" {
		t.Errorf("main-backup: expected group 'main', got '%s'", mainBackup.Group)
	}
	if backupSecondary.Group != "backup" {
		t.Errorf("backup-secondary: expected group 'backup', got '%s'", backupSecondary.Group)
	}
	if overrideCustom.Group != "override" {
		t.Errorf("override-custom: expected group 'override', got '%s'", overrideCustom.Group)
	}
}

func TestFullParameterInheritance(t *testing.T) {
	configContent := `
server:
  host: "localhost"
  port: 8080

strategy:
  type: "priority"

endpoints:
  - name: "primary"
    url: "https://api1.example.com"
    priority: 1
    timeout: "45s"
    token: "primary-token"
    headers:
      X-API-Version: "v1"
      User-Agent: "Claude-Forwarder/1.0"
      Authorization-Fallback: "Bearer fallback"

  - name: "inherits-all"
    url: "https://api2.example.com"
    priority: 2
    # Should inherit timeout and all headers, but NOT token (dynamic resolution)

  - name: "partial-override" 
    url: "https://api3.example.com"
    priority: 3
    timeout: "60s"
    headers:
      X-API-Version: "v2"
      X-Custom: "custom-value"

  - name: "full-custom"
    url: "https://api4.example.com"
    priority: 4
    timeout: "30s"
    token: "custom-token"
    headers:
      X-Different: "different-value"
`

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test full inheritance endpoint (timeout and headers, but not token)
	inheritsAll := config.Endpoints[1]
	if inheritsAll.Timeout != 45*time.Second {
		t.Errorf("Expected inherited timeout 45s, got %v", inheritsAll.Timeout)
	}
	// Token should NOT be inherited in the new dynamic system
	if inheritsAll.Token != "" {
		t.Errorf("Expected no static token (dynamic resolution), got '%s'", inheritsAll.Token)
	}
	if len(inheritsAll.Headers) != 3 {
		t.Errorf("Expected 3 inherited headers, got %d", len(inheritsAll.Headers))
	}
	if inheritsAll.Headers["X-API-Version"] != "v1" {
		t.Errorf("Expected inherited header X-API-Version=v1, got '%s'", inheritsAll.Headers["X-API-Version"])
	}

	// Test partial override endpoint (timeout and headers, but not token)
	partialOverride := config.Endpoints[2]
	if partialOverride.Timeout != 60*time.Second {
		t.Errorf("Expected override timeout 60s, got %v", partialOverride.Timeout)
	}
	// Token should NOT be inherited in the new dynamic system
	if partialOverride.Token != "" {
		t.Errorf("Expected no static token (dynamic resolution), got '%s'", partialOverride.Token)
	}
	if partialOverride.Headers["X-API-Version"] != "v2" {
		t.Errorf("Expected override header X-API-Version=v2, got '%s'", partialOverride.Headers["X-API-Version"])
	}
	if partialOverride.Headers["User-Agent"] != "Claude-Forwarder/1.0" {
		t.Errorf("Expected inherited User-Agent, got '%s'", partialOverride.Headers["User-Agent"])
	}
	if partialOverride.Headers["X-Custom"] != "custom-value" {
		t.Errorf("Expected new header X-Custom=custom-value, got '%s'", partialOverride.Headers["X-Custom"])
	}

	// Test full custom with merging
	fullCustom := config.Endpoints[3]
	if fullCustom.Timeout != 30*time.Second {
		t.Errorf("Expected custom timeout 30s, got %v", fullCustom.Timeout)
	}
	if fullCustom.Token != "custom-token" {
		t.Errorf("Expected custom token 'custom-token', got '%s'", fullCustom.Token)
	}
	if fullCustom.Headers["X-Different"] != "different-value" {
		t.Errorf("Expected custom header X-Different=different-value, got '%s'", fullCustom.Headers["X-Different"])
	}
	if fullCustom.Headers["User-Agent"] != "Claude-Forwarder/1.0" {
		t.Errorf("Expected inherited User-Agent, got '%s'", fullCustom.Headers["User-Agent"])
	}
}

func TestNoTokenInheritance(t *testing.T) {
	// Test case where first endpoint has no token
	configContent := `
server:
  host: "localhost"
  port: 8080

strategy:
  type: "priority"

endpoints:
  - name: "first"
    url: "https://api1.example.com"
    priority: 1
    # No token specified

  - name: "second"
    url: "https://api2.example.com"
    priority: 2
    # No token specified

  - name: "third"
    url: "https://api3.example.com"
    priority: 3
    token: "third-token"
`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load configuration
	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify no token inheritance when first endpoint has no token
	if config.Endpoints[0].Token != "" {
		t.Errorf("First endpoint token: expected empty, got '%s'", config.Endpoints[0].Token)
	}

	if config.Endpoints[1].Token != "" {
		t.Errorf("Second endpoint token: expected empty, got '%s'", config.Endpoints[1].Token)
	}

	if config.Endpoints[2].Token != "third-token" {
		t.Errorf("Third endpoint token: expected 'third-token', got '%s'", config.Endpoints[2].Token)
	}
}

func TestApplyPrimaryEndpoint(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		primaryEndpoint  string
		expectError      bool
		expectedPriority int
		expectedAdjusted int
	}{
		{
			name: "Valid endpoint name",
			config: &Config{
				Endpoints: []EndpointConfig{
					{Name: "endpoint1", Priority: 5},
					{Name: "endpoint2", Priority: 1},
					{Name: "endpoint3", Priority: 3},
				},
			},
			primaryEndpoint:  "endpoint3",
			expectError:      false,
			expectedPriority: 1,
			expectedAdjusted: 1, // only endpoint2 has priority <= 1
		},
		{
			name: "Invalid endpoint name",
			config: &Config{
				Endpoints: []EndpointConfig{
					{Name: "endpoint1", Priority: 1},
					{Name: "endpoint2", Priority: 2},
				},
			},
			primaryEndpoint: "nonexistent",
			expectError:     true,
		},
		{
			name: "Empty primary endpoint",
			config: &Config{
				Endpoints: []EndpointConfig{
					{Name: "endpoint1", Priority: 1},
				},
			},
			primaryEndpoint: "",
			expectError:     false,
		},
		{
			name: "Multiple endpoints need adjustment",
			config: &Config{
				Endpoints: []EndpointConfig{
					{Name: "endpoint1", Priority: 1},
					{Name: "endpoint2", Priority: 0},
					{Name: "endpoint3", Priority: -1},
					{Name: "endpoint4", Priority: 5},
				},
			},
			primaryEndpoint:  "endpoint4",
			expectError:      false,
			expectedPriority: 1,
			expectedAdjusted: 3, // endpoints 1, 2, 3 all have priority <= 1
		},
		{
			name: "No adjustments needed",
			config: &Config{
				Endpoints: []EndpointConfig{
					{Name: "endpoint1", Priority: 5},
					{Name: "endpoint2", Priority: 3},
					{Name: "endpoint3", Priority: 2},
				},
			},
			primaryEndpoint:  "endpoint2",
			expectError:      false,
			expectedPriority: 1,
			expectedAdjusted: 0, // no endpoints have priority <= 1 except the primary
		},
	}

	// Create a null logger for testing
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the primary endpoint
			tt.config.PrimaryEndpoint = tt.primaryEndpoint

			// Apply the primary endpoint
			err := tt.config.ApplyPrimaryEndpoint(logger)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected an error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}

			// If no error expected, check the results
			if !tt.expectError && tt.primaryEndpoint != "" {
				// Find the primary endpoint and check its priority
				primaryIndex := tt.config.findEndpointIndex(tt.primaryEndpoint)
				if primaryIndex != -1 {
					if tt.config.Endpoints[primaryIndex].Priority != tt.expectedPriority {
						t.Errorf("Expected primary endpoint priority %d, got %d",
							tt.expectedPriority, tt.config.Endpoints[primaryIndex].Priority)
					}
				}

				// Count endpoints that were adjusted (now have priority > original priority)
				adjustedCount := 0
				for i := range tt.config.Endpoints {
					if i != primaryIndex {
						// Check if this endpoint was adjusted (has priority > 1)
						switch tt.name {
						case "Valid endpoint name":
							if i == 1 && tt.config.Endpoints[i].Priority == 3 { // endpoint2: 1 + 2 = 3
								adjustedCount++
							}
						case "Multiple endpoints need adjustment":
							if (i == 0 && tt.config.Endpoints[i].Priority == 3) || // endpoint1: 1 + 2 = 3
								(i == 1 && tt.config.Endpoints[i].Priority == 2) || // endpoint2: 0 + 2 = 2
								(i == 2 && tt.config.Endpoints[i].Priority == 1) { // endpoint3: -1 + 2 = 1
								adjustedCount++
							}
						}
					}
				}

				if adjustedCount != tt.expectedAdjusted {
					t.Errorf("Expected %d endpoints to be adjusted, got %d",
						tt.expectedAdjusted, adjustedCount)
				}
			}
		})
	}
}

func TestFindEndpointIndex(t *testing.T) {
	config := &Config{
		Endpoints: []EndpointConfig{
			{Name: "endpoint1"},
			{Name: "endpoint2"},
			{Name: "endpoint3"},
		},
	}

	tests := []struct {
		name     string
		endpoint string
		expected int
	}{
		{"First endpoint", "endpoint1", 0},
		{"Middle endpoint", "endpoint2", 1},
		{"Last endpoint", "endpoint3", 2},
		{"Non-existent endpoint", "nonexistent", -1},
		{"Empty string", "", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.findEndpointIndex(tt.endpoint)
			if result != tt.expected {
				t.Errorf("Expected index %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestLoadConfig_AccountPoolEnabledDefaultWhenMissing(t *testing.T) {
	configContent := `
endpoints_storage:
  type: "sqlite"
`

	tmpFile, err := os.CreateTemp("", "test-account-pool-default-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.AccountPool.Enabled {
		t.Fatalf("expected account_pool.enabled=false when missing explicit config")
	}
}

func TestLoadConfig_EOFRetryHintMigration(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{
			name: "missing both defaults false",
			yaml: "endpoints_storage:\n  type: sqlite\n",
			want: false,
		},
		{
			name: "legacy true is preserved",
			yaml: "endpoints_storage:\n  type: sqlite\nrequest_suspend:\n  eof_retry_hint: true\n",
			want: true,
		},
		{
			name: "new explicit false overrides legacy true",
			yaml: "endpoints_storage:\n  type: sqlite\nstreaming:\n  eof_retry_hint: false\nrequest_suspend:\n  eof_retry_hint: true\n",
			want: false,
		},
		{
			name: "new true overrides legacy false",
			yaml: "endpoints_storage:\n  type: sqlite\nstreaming:\n  eof_retry_hint: true\nrequest_suspend:\n  eof_retry_hint: false\n",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "eof-retry-hint-*.yaml")
			if err != nil {
				t.Fatalf("create config: %v", err)
			}
			if _, err := file.WriteString(tt.yaml); err != nil {
				t.Fatalf("write config: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close config: %v", err)
			}

			cfg, err := LoadConfig(file.Name())
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if cfg.Streaming.EOFRetryHint != tt.want {
				t.Fatalf("streaming.eof_retry_hint = %v, want %v", cfg.Streaming.EOFRetryHint, tt.want)
			}
		})
	}
}

func TestLoadConfig_AccountPoolEnabledExplicitFalse(t *testing.T) {
	configContent := `
endpoints_storage:
  type: "sqlite"
account_pool:
  enabled: false
`

	tmpFile, err := os.CreateTemp("", "test-account-pool-false-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.AccountPool.Enabled {
		t.Fatalf("expected account_pool.enabled=false when explicitly configured false")
	}
}

func TestLoadConfig_AccountPoolEnabledExplicitTrue(t *testing.T) {
	configContent := `
endpoints_storage:
  type: "sqlite"
account_pool:
  enabled: true
`

	tmpFile, err := os.CreateTemp("", "test-account-pool-true-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if !cfg.AccountPool.Enabled {
		t.Fatalf("expected account_pool.enabled=true when explicitly configured true")
	}
}

func TestLoadConfig_AccountPoolFailurePolicyDefaults(t *testing.T) {
	configContent := `
endpoints_storage:
  type: "sqlite"
`

	tmpFile, err := os.CreateTemp("", "test-account-pool-failure-defaults-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if got := cfg.AccountPool.FailurePolicy.SoftFailureWindow; got != 5*time.Minute {
		t.Fatalf("expected soft_failure_window=5m, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.SoftFailureThreshold; got != 3 {
		t.Fatalf("expected soft_failure_threshold=3, got %d", got)
	}
	if cfg.AccountPool.FailurePolicy.RespectRetryAfter == nil || !*cfg.AccountPool.FailurePolicy.RespectRetryAfter {
		t.Fatalf("expected respect_retry_after=true by default, got %+v", cfg.AccountPool.FailurePolicy.RespectRetryAfter)
	}
	if got := cfg.AccountPool.FailurePolicy.LocalNoAvailableProvidersMarker; got != "no_available_providers::ccf_local" {
		t.Fatalf("unexpected local marker: %q", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.ConnectionFailure; got != 90*time.Second {
		t.Fatalf("expected connection_failure=90s, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.ProcessingFailure; got != 60*time.Second {
		t.Fatalf("expected processing_failure=60s, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.RateLimitDefault; got != 180*time.Second {
		t.Fatalf("expected rate_limit_default=180s, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.ServerError; got != 120*time.Second {
		t.Fatalf("expected server_error=120s, got %v", got)
	}
}

func TestLoadConfig_AccountPoolFailurePolicyExplicitValues(t *testing.T) {
	configContent := `
endpoints_storage:
  type: "sqlite"
account_pool:
  enabled: true
  failure_policy:
    soft_failure_window: "2m"
    soft_failure_threshold: 5
    respect_retry_after: false
    local_no_available_providers_marker: "no_available_providers::custom_marker"
    cooldowns:
      connection_failure: "30s"
      processing_failure: "45s"
      rate_limit_default: "75s"
      server_error: "95s"
`

	tmpFile, err := os.CreateTemp("", "test-account-pool-failure-explicit-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if got := cfg.AccountPool.FailurePolicy.SoftFailureWindow; got != 2*time.Minute {
		t.Fatalf("expected soft_failure_window=2m, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.SoftFailureThreshold; got != 5 {
		t.Fatalf("expected soft_failure_threshold=5, got %d", got)
	}
	if cfg.AccountPool.FailurePolicy.RespectRetryAfter == nil || *cfg.AccountPool.FailurePolicy.RespectRetryAfter {
		t.Fatalf("expected respect_retry_after=false, got %+v", cfg.AccountPool.FailurePolicy.RespectRetryAfter)
	}
	if got := cfg.AccountPool.FailurePolicy.LocalNoAvailableProvidersMarker; got != "no_available_providers::custom_marker" {
		t.Fatalf("unexpected local marker: %q", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.ConnectionFailure; got != 30*time.Second {
		t.Fatalf("expected connection_failure=30s, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.ProcessingFailure; got != 45*time.Second {
		t.Fatalf("expected processing_failure=45s, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.RateLimitDefault; got != 75*time.Second {
		t.Fatalf("expected rate_limit_default=75s, got %v", got)
	}
	if got := cfg.AccountPool.FailurePolicy.Cooldowns.ServerError; got != 95*time.Second {
		t.Fatalf("expected server_error=95s, got %v", got)
	}
}
