package handler_test

import (
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/auth"
)

func TestUsers_List_Empty(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	w := do(t, api, "GET", "/users", "")
	// single-user mód → IsSuperAdmin=true, vrátí prázdný seznam
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var users []auth.User
	decodeBody(t, w, &users)
	if len(users) != 0 {
		t.Errorf("len = %d, want 0", len(users))
	}
}

func TestUsers_Create(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	w := do(t, api, "POST", "/users", `{"username":"novak","password":"heslo1234","role":"user"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var u auth.User
	decodeBody(t, w, &u)
	if u.Username != "novak" {
		t.Errorf("username = %q, want novak", u.Username)
	}
	if u.Role != auth.RoleUser {
		t.Errorf("role = %q, want user", u.Role)
	}
	if u.PasswordHash != "" {
		t.Error("password_hash must not be exposed in response")
	}
}

func TestUsers_Create_MissingFields(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	// password je required field → Huma vrátí 422 před handlerem
	w := do(t, api, "POST", "/users", `{"username":"novak"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
}

func TestUsers_Create_DuplicateUsername(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	do(t, api, "POST", "/users", `{"username":"novak","password":"heslo1234"}`)
	w := do(t, api, "POST", "/users", `{"username":"novak","password":"jine1234"}`)
	if w.Code == http.StatusCreated {
		t.Error("expected error for duplicate username, got 201")
	}
}

func TestUsers_Get(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	wc := do(t, api, "POST", "/users", `{"username":"novak","password":"heslo1234"}`)
	var created auth.User
	decodeBody(t, wc, &created)

	w := do(t, api, "GET", "/users/"+idStr(created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var u auth.User
	decodeBody(t, w, &u)
	if u.Username != "novak" {
		t.Errorf("username = %q, want novak", u.Username)
	}
}

func TestUsers_Get_NotFound(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	w := do(t, api, "GET", "/users/9999", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestUsers_Update_Deactivate(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	wc := do(t, api, "POST", "/users", `{"username":"novak","password":"heslo1234"}`)
	var created auth.User
	decodeBody(t, wc, &created)

	isActive := false
	_ = isActive
	w := do(t, api, "PUT", "/users/"+idStr(created.ID), `{"is_active":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var u auth.User
	decodeBody(t, w, &u)
	if u.IsActive {
		t.Error("is_active should be false after deactivation")
	}
}

func TestUsers_ResetPassword(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	wc := do(t, api, "POST", "/users", `{"username":"novak","password":"heslo1234"}`)
	var created auth.User
	decodeBody(t, wc, &created)

	w := do(t, api, "POST", "/users/"+idStr(created.ID)+"/reset-password", `{"password":"noveHeslo99"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUsers_Delete(t *testing.T) {
	api, _ := newTestAPIWithDB(t)
	wc := do(t, api, "POST", "/users", `{"username":"novak","password":"heslo1234"}`)
	var created auth.User
	decodeBody(t, wc, &created)

	w := do(t, api, "DELETE", "/users/"+idStr(created.ID), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	w2 := do(t, api, "GET", "/users/"+idStr(created.ID), "")
	if w2.Code != http.StatusNotFound {
		t.Errorf("after delete status = %d, want 404", w2.Code)
	}
}
