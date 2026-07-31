package modelrewrite

import "testing"

func TestRewrite(t *testing.T) {
	rules := `[{"paths":["/v1/messages"],"match":"exact","from":"claude-sonnet-4-5","to":"provider-sonnet"},{"paths":["/v1/messages"],"match":"prefix","from":"legacy-","to":"legacy-target"}]`

	tests := []struct {
		name      string
		path      string
		model     string
		want      string
		rewritten bool
	}{
		{name: "exact", path: "/v1/messages", model: "claude-sonnet-4-5", want: "provider-sonnet", rewritten: true},
		{name: "case insensitive exact", path: "/v1/messages", model: "CLAUDE-SONNET-4-5", want: "provider-sonnet", rewritten: true},
		{name: "wrong path", path: "/v1/responses", model: "claude-sonnet-4-5"},
		{name: "exact does not match suffix", path: "/v1/messages", model: "claude-sonnet-4-5-latest"},
		{name: "legacy prefix", path: "/v1/messages", model: "legacy-model", want: "legacy-target", rewritten: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rewritten, err := Rewrite(rules, tt.path, tt.model)
			if err != nil {
				t.Fatalf("Rewrite failed: %v", err)
			}
			if got != tt.want || rewritten != tt.rewritten {
				t.Fatalf("got model=%q rewritten=%v, want model=%q rewritten=%v", got, rewritten, tt.want, tt.rewritten)
			}
		})
	}
}

func TestRewriteSupportsSingleRuleObjectAndRejectsInvalidJSON(t *testing.T) {
	got, rewritten, err := Rewrite(`{"paths":["/v1/messages"],"from":"source","to":"target"}`, "/v1/messages", "source")
	if err != nil || !rewritten || got != "target" {
		t.Fatalf("unexpected single-rule result: model=%q rewritten=%v err=%v", got, rewritten, err)
	}

	if _, _, err := Rewrite(`{"from":`, "/v1/messages", "source"); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestValidateExact(t *testing.T) {
	valid := `[{"paths":["/v1/messages","/v1/messages/count_tokens"],"match":"exact","from":"source","to":"target"}]`
	if err := ValidateExact(valid, "/v1/messages", "/v1/messages/count_tokens"); err != nil {
		t.Fatalf("valid rules rejected: %v", err)
	}

	invalidRules := []string{
		`[]`,
		`[{"paths":["/v1/messages"],"match":"prefix","from":"source","to":"target"}]`,
		`[{"paths":["/v1/messages"],"match":"exact","from":"source","to":"target"}]`,
		`[{"paths":["/v1/messages","/v1/messages"],"match":"exact","from":"source","to":"target"}]`,
		`[{"paths":["/v1/responses"],"match":"exact","from":"source","to":"target"}]`,
		`[{"paths":["/v1/messages"],"match":"exact","from":"source","to":"source"}]`,
		`[{"paths":[],"match":"exact","from":"source","to":"target"}]`,
	}
	for _, raw := range invalidRules {
		if err := ValidateExact(raw, "/v1/messages", "/v1/messages/count_tokens"); err == nil {
			t.Fatalf("expected invalid rules to fail: %s", raw)
		}
	}
}
