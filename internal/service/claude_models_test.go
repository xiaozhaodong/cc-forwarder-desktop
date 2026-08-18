package service

import (
	"strings"
	"testing"
)

func TestNormalizeClaudeModelCatalogAllowsArbitraryIDsAndBuildsDiscoveryResponse(t *testing.T) {
	catalog, err := NormalizeClaudeModelCatalog(ClaudeModelCatalog{
		Enabled: true,
		Models: []ClaudeModelEntry{
			{ID: " glm-5.2[1m] ", DisplayName: " GLM 5.2 ", Enabled: true},
			{ID: "deepseek-v4-flash[1m]", Enabled: true},
			{ID: "kimi-k3", DisplayName: "Kimi K3", Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeClaudeModelCatalog() error = %v", err)
	}
	if len(catalog.Models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(catalog.Models))
	}
	if catalog.Models[0].ID != "deepseek-v4-flash[1m]" || catalog.Models[0].DisplayName != "deepseek-v4-flash[1m]" {
		t.Fatalf("unexpected first normalized model: %+v", catalog.Models[0])
	}
	if catalog.Models[1].ID != "glm-5.2[1m]" || catalog.Models[1].DisplayName != "GLM 5.2" {
		t.Fatalf("unexpected second normalized model: %+v", catalog.Models[1])
	}

	response := catalog.BuildListResponse()
	if len(response.Data) != 2 {
		t.Fatalf("expected only enabled models in response, got %+v", response.Data)
	}
	if response.Data[0].ID != "deepseek-v4-flash[1m]" || response.Data[1].ID != "glm-5.2[1m]" {
		t.Fatalf("unexpected discovery response order: %+v", response.Data)
	}
}

func TestClaudeModelCatalogDisabledReturnsEmptyDiscoveryResponse(t *testing.T) {
	catalog, err := NormalizeClaudeModelCatalog(ClaudeModelCatalog{
		Enabled: false,
		Models:  []ClaudeModelEntry{{ID: "deepseek-v4-flash[1m]", Enabled: true}},
	})
	if err != nil {
		t.Fatalf("NormalizeClaudeModelCatalog() error = %v", err)
	}
	if got := len(catalog.BuildListResponse().Data); got != 0 {
		t.Fatalf("expected disabled catalog response to be empty, got %d entries", got)
	}
}

func TestDecodeClaudeModelCatalogRejectsInvalidInput(t *testing.T) {
	tooMany := make([]string, MaxClaudeModelEntries+1)
	for index := range tooMany {
		tooMany[index] = `{"id":"model-` + strings.Repeat("x", index%3) + `","enabled":true}`
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "whitespace in id", raw: `{"enabled":true,"models":[{"id":"deepseek v4","enabled":true}]}`},
		{name: "control in display name", raw: "{\"enabled\":true,\"models\":[{\"id\":\"kimi-k3\",\"display_name\":\"Kimi\\u0001\",\"enabled\":true}]}"},
		{name: "duplicate id", raw: `{"enabled":true,"models":[{"id":"glm-5","enabled":true},{"id":"glm-5","enabled":false}]}`},
		{name: "unknown field", raw: `{"enabled":true,"models":[],"mode":"local"}`},
		{name: "trailing json", raw: `{"enabled":true,"models":[]} {}`},
		{name: "too many", raw: `{"enabled":true,"models":[` + strings.Join(tooMany, ",") + `]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeClaudeModelCatalog(test.raw); err == nil {
				t.Fatalf("expected invalid catalog to fail: %s", test.raw)
			}
		})
	}
}

func TestDecodeClaudeModelCatalogDefaultsMissingSetting(t *testing.T) {
	catalog, err := DecodeClaudeModelCatalog("  ")
	if err != nil {
		t.Fatalf("DecodeClaudeModelCatalog() error = %v", err)
	}
	if !catalog.Enabled || catalog.Models == nil || len(catalog.Models) != 0 {
		t.Fatalf("unexpected default catalog: %+v", catalog)
	}
}
