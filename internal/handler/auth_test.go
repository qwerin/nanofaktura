package handler_test

import (
	"net/http"
	"testing"
)

const setupSingleUser = `{
	"company_name": "Test OSVČ",
	"multi_user": false
}`

const setupMultiUser = `{
	"company_name": "Test Firma",
	"multi_user": true,
	"username": "admin",
	"password": "heslo1234"
}`

func TestSetup_Init_SingleUser(t *testing.T) {
	api := newTestAPISetup(t)

	w := do(t, api, "POST", "/setup/init", setupSingleUser)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

func TestSetup_Init_MultiUser(t *testing.T) {
	api := newTestAPISetup(t)

	w := do(t, api, "POST", "/setup/init", setupMultiUser)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	decodeBody(t, w, &result)
	if result["username"] != "admin" {
		t.Errorf("username = %v, want admin", result["username"])
	}
}

func TestSetup_Init_AlreadyDone(t *testing.T) {
	api := newTestAPISetup(t)

	do(t, api, "POST", "/setup/init", setupSingleUser)
	w := do(t, api, "POST", "/setup/init", setupSingleUser)
	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want 410 (already initialized)", w.Code)
	}
}

func TestSetup_Init_MissingCompanyName(t *testing.T) {
	api := newTestAPISetup(t)

	w := do(t, api, "POST", "/setup/init", `{"multi_user": false}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSetup_Init_MultiUser_MissingPassword(t *testing.T) {
	api := newTestAPISetup(t)

	w := do(t, api, "POST", "/setup/init", `{"company_name":"Test","multi_user":true,"username":"admin"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAuth_Login_InvalidCredentials(t *testing.T) {
	api, _ := newTestAPIWithDB(t)

	// V single-user módu je login zakázaný
	w := do(t, api, "POST", "/auth/login", `{"username":"admin","password":"wrong"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (single-user mode)", w.Code)
	}
}

func TestAuth_Me_Unauthenticated(t *testing.T) {
	api, _ := newTestAPIWithDB(t)

	// V single-user módu /auth/me vrátí 401 (žádný user v kontextu)
	w := do(t, api, "GET", "/auth/me", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
