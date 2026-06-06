package handlers

import (
	"net/http"
	"testing"
)

func TestIsCountTokensUnsupportedRequiresCountTokensContextForTextMatches(t *testing.T) {
	if !isCountTokensUnsupported(http.StatusNotFound, `{"error":"missing route"}`) {
		t.Fatal("expected 404 to mark count_tokens as unsupported")
	}
	if !isCountTokensUnsupported(http.StatusBadRequest, `{"error":"count_tokens endpoint is not supported"}`) {
		t.Fatal("expected explicit count_tokens not supported message to be marked unsupported")
	}
	if isCountTokensUnsupported(http.StatusBadRequest, `{"error":"model unsupported"}`) {
		t.Fatal("model unsupported should not mark count_tokens as unsupported")
	}
	if isCountTokensUnsupported(http.StatusUnprocessableEntity, `{"error":"parameter temperature is not supported"}`) {
		t.Fatal("generic parameter errors should not mark count_tokens as unsupported")
	}
}
