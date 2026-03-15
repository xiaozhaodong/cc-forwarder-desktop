package utils

import "testing"

func TestMaskToken_EmptyStringReturnsEmpty(t *testing.T) {
	if got := MaskToken(""); got != "" {
		t.Fatalf("expected empty string for empty token, got %q", got)
	}
}

func TestMaskToken_ShortStringReturnsMasked(t *testing.T) {
	if got := MaskToken("short"); got != "****" {
		t.Fatalf("expected short token to be fully masked, got %q", got)
	}
}
