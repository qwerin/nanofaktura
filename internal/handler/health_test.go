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
