package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFlattenLegacyEndpointsFixture(t *testing.T) {
	legacy, err := LoadLegacyConfig(filepath.Join("testdata", "legacy_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := FlattenLegacyEndpoints(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceEndpointCount != 5 || len(result.Endpoints) != 10 || result.SplitEndpointCount != 1 || result.DerivedRecordCount != 5 {
		t.Fatalf("unexpected flatten summary: %+v", result)
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "expected_endpoints.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected struct {
		Endpoints []struct {
			Name                string `json:"name"`
			AvailabilityEnabled bool   `json:"availability_enabled"`
			FailoverEnabled     bool   `json:"failover_enabled"`
		}
	}
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	for i, want := range expected.Endpoints {
		got := result.Endpoints[i]
		if got.Name != want.Name || got.AvailabilityEnabled != want.AvailabilityEnabled || got.FailoverEnabled != want.FailoverEnabled {
			t.Fatalf("endpoint[%d] = (%q,%v,%v), want (%q,%v,%v)", i, got.Name, got.AvailabilityEnabled, got.FailoverEnabled, want.Name, want.AvailabilityEnabled, want.FailoverEnabled)
		}
	}

	child := findSnapshot(t, result.Endpoints, "inherited-auth-child")
	if child.Token != "synthetic-inherited-token" || child.APIKey != "synthetic-inherited-api" {
		t.Fatalf("child credentials did not resolve legacy group inheritance: token=%q api_key=%q", child.Token, child.APIKey)
	}
	if child.Headers["X-Legacy-Header"] != "inherited-value" || child.Headers["X-Override-Me"] != "child" {
		t.Fatalf("child headers = %#v", child.Headers)
	}
	if child.TimeoutSeconds != 45 {
		t.Fatalf("child timeout = %d, want inherited first endpoint timeout 45", child.TimeoutSeconds)
	}
}

func TestFlattenLegacyEndpointsStableFallbackAndCollision(t *testing.T) {
	legacy := &LegacyConfig{Endpoints: []LegacyEndpoint{
		{Name: "dirty\n name", URL: "https://example.test", Tokens: []LegacyCredential{{Value: "a"}, {Value: "b"}}},
		{Name: "dirty name · token-2", URL: "https://collision.test"},
	}}
	result, err := FlattenLegacyEndpoints(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Endpoints[0].Name; got != "dirty name" {
		t.Fatalf("clean base name = %q", got)
	}
	if got := result.Endpoints[1].Name; got != "dirty name · token-2-2" {
		t.Fatalf("derived fallback name = %q", got)
	}
	if got := result.Endpoints[2].Name; got != "dirty name · token-2" {
		t.Fatalf("collision name = %q", got)
	}
}

func findSnapshot(t *testing.T, endpoints []EndpointSnapshot, name string) EndpointSnapshot {
	t.Helper()
	for _, endpoint := range endpoints {
		if endpoint.Name == name {
			return endpoint
		}
	}
	t.Fatalf("endpoint %q not found", name)
	return EndpointSnapshot{}
}
