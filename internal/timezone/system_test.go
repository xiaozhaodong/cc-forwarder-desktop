package timezone

import (
	"strings"
	"testing"
)

func TestSystemDefaultReturnsLoadableName(t *testing.T) {
	name := SystemDefault()
	if strings.TrimSpace(name) == "" {
		t.Fatal("system default timezone is empty")
	}
	if _, err := Load(name); err != nil {
		t.Fatalf("system default timezone %q is not loadable: %v", name, err)
	}
}

func TestResolveConfigured(t *testing.T) {
	system := SystemDefault()
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: system},
		{input: "  ", want: system},
		{input: "local", want: system},
		{input: "LOCAL", want: system},
		{input: "America/New_York", want: "America/New_York"},
		{input: " UTC ", want: "UTC"},
	}
	for _, tc := range tests {
		if got := ResolveConfigured(tc.input); got != tc.want {
			t.Fatalf("ResolveConfigured(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
