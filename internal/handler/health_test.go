package handler_test

import (
	"net/http"
	"testing"
)

func TestHealth_OK(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "GET", "/health", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var result struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	decodeBody(t, w, &result)
	if result.Status != "ok" {
		t.Errorf("status = %q, want ok", result.Status)
	}
}

// TestHealth_UnauthenticatedMultiUser ověří, že /api/health je dostupné bez session tokenu
// i v multi-user módu (ConditionalAuth nesmí blokovat health endpoint).
func TestHealth_UnauthenticatedMultiUser(t *testing.T) {
	r, _ := newTestAPIMultiUserChi(t)
	w := do(t, r, "GET", "/api/health", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (health musí být veřejný); body: %s", w.Code, w.Body.String())
	}
}
