package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
)

// TestRoleMatrix checks the declared role of every account-scoped operation.
// Requests use invalid bodies / unknown IDs, so an allowed role gets a 4xx
// from validation or lookup (never 403) and nothing changes between roles —
// the role check runs in the account middleware before input parsing.
func TestRoleMatrix(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	clients := map[string]*client{
		"owner":      owner,
		"admin":      ts.memberOf(owner, "admin@example.cz", "admin"),
		"accountant": ts.memberOf(owner, "acc@example.cz", "accountant"),
		"member":     ts.memberOf(owner, "member@example.cz", "member"),
	}
	all := auth.RolesAll
	editors := auth.RolesEditors
	managers := auth.RolesManagers
	const missing = "/999999"

	for _, op := range []struct {
		method, path string
		body         any
		allowed      []string
	}{
		{"GET", "", nil, all},
		{"PATCH", "", map[string]any{"name": ""}, managers},

		{"GET", "/bank-accounts", nil, all},
		{"POST", "/bank-accounts", map[string]any{}, managers},
		{"PATCH", "/bank-accounts" + missing, map[string]any{}, managers},
		{"DELETE", "/bank-accounts" + missing, nil, managers},

		{"GET", "/number-formats", nil, all},
		{"POST", "/number-formats", map[string]any{}, managers},
		{"PATCH", "/number-formats" + missing, map[string]any{}, managers},
		{"DELETE", "/number-formats" + missing, nil, managers},

		{"GET", "/subjects", nil, all},
		{"POST", "/subjects", map[string]any{}, editors},
		{"PATCH", "/subjects" + missing, map[string]any{}, editors},
		{"DELETE", "/subjects" + missing, nil, editors},

		{"GET", "/invoices", nil, all},
		{"GET", "/dashboard", nil, all},
		{"POST", "/invoices", map[string]any{}, editors},
		{"PATCH", "/invoices" + missing, map[string]any{}, editors},
		{"DELETE", "/invoices" + missing, nil, editors},
		{"POST", "/invoices" + missing + "/actions/mark_as_sent", nil, editors},
		{"POST", "/invoices" + missing + "/correction", map[string]any{}, editors},
		{"POST", "/invoices" + missing + "/duplicate", nil, editors},
		{"POST", "/invoices" + missing + "/regenerate-public-token", nil, editors},
		{"POST", "/invoices" + missing + "/payments", map[string]any{}, editors},
		{"DELETE", "/invoices" + missing + "/payments/1", nil, editors},

		{"GET", "/members", nil, all},
		{"PATCH", "/members" + missing, map[string]any{"role": "member"}, managers},
		{"POST", "/members/invite", map[string]any{}, managers},
		{"GET", "/invitations", nil, managers},
		{"DELETE", "/invitations" + missing, nil, managers},

		{"GET", "/price-items", nil, all},
		{"GET", "/price-items" + missing, nil, all},
		{"POST", "/price-items", map[string]any{}, editors},
		{"PATCH", "/price-items" + missing, map[string]any{}, editors},
		{"DELETE", "/price-items" + missing, nil, editors},
		{"GET", "/price-items" + missing + "/stock-moves", nil, all},
		{"POST", "/price-items" + missing + "/stock-moves", map[string]any{}, editors},
		{"DELETE", "/price-items" + missing + "/stock-moves/1", nil, editors},

		{"GET", "/expenses", nil, all},
		{"GET", "/expenses/categories", nil, all},
		{"GET", "/expenses" + missing, nil, all},
		{"POST", "/expenses", map[string]any{}, editors},
		{"PATCH", "/expenses" + missing, map[string]any{}, editors},
		{"DELETE", "/expenses" + missing, nil, editors},
		{"POST", "/expenses" + missing + "/actions/lock", nil, editors},
		{"POST", "/expenses" + missing + "/payments", map[string]any{}, editors},
		{"DELETE", "/expenses" + missing + "/payments/1", nil, editors},

		{"GET", "/attachments", nil, all},
		{"POST", "/attachments", map[string]any{}, editors},
		{"DELETE", "/attachments" + missing, nil, editors},
	} {
		for role, c := range clients {
			res, body := c.do(op.method, c.acct(op.path), op.body)
			if want := slices.Contains(op.allowed, role); want && res.StatusCode == http.StatusForbidden {
				t.Errorf("%s %s as %s: 403, want allowed; body %s", op.method, op.path, role, body)
			} else if !want && res.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403; body %s", op.method, op.path, role, res.StatusCode, body)
			} else if !want && !strings.Contains(string(body), "your role ("+role+") cannot do this") {
				t.Errorf("%s %s as %s: 403 body %s", op.method, op.path, role, body)
			}
		}
	}
}

// TestEveryAccountMutationDeclaresRoles: account-scoped POST/PUT/PATCH/DELETE
// operations must declare their roles (auth.ForEditors / ForManagers / Allow),
// otherwise read-only roles (accountant) could call them. Exceptions are
// listed explicitly and check the role in the handler.
func TestEveryAccountMutationDeclaresRoles(t *testing.T) {
	exceptions := map[string]bool{
		"DELETE /api/accounts/{slug}/members/{user_id}": true, // anyone may leave; removing others checked in handler
	}
	_, humaAPI := api.New(nil, config.Config{}, api.Deps{})
	checked := 0
	for path, item := range humaAPI.OpenAPI().Paths {
		if !strings.HasPrefix(path, "/api/accounts/{slug}") {
			continue
		}
		for method, op := range map[string]*huma.Operation{
			"POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete,
		} {
			if op == nil || exceptions[method+" "+path] {
				continue
			}
			checked++
			if _, ok := op.Extensions[auth.ExtRoles]; !ok {
				t.Errorf("%s %s declares no roles", method, path)
			}
		}
	}
	if checked < 20 {
		t.Fatalf("only %d operations checked", checked)
	}
}

// TestInvoiceRoles: members work with invoices, accountants only read them.
func TestInvoiceRoles(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	member := ts.memberOf(owner, "member@example.cz", "member")
	accountant := ts.memberOf(owner, "acc@example.cz", "accountant")

	subj := newSubject(member, api.SubjectCreate{Name: "ACME"})
	inv := createInv(member, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	action(member, inv.ID, "mark_as_sent")
	pay(member, inv.ID, api.PaymentCreate{})

	if got := getInv(accountant, inv.ID); got.ID != inv.ID {
		t.Fatalf("accountant get: %+v", got)
	}
	if l := listInv(accountant, ""); l.Total != 1 {
		t.Fatalf("accountant list: %+v", l)
	}
	accountant.mustDo(http.StatusOK, "GET", accountant.acct("/dashboard"), nil)
	res, body := accountant.do("POST", invURL(accountant, inv.ID, "/actions/cancel"), nil)
	assertError(t, res, body, http.StatusForbidden, "your role (accountant)")
	res, body = accountant.do("POST", accountant.acct("/invoices"), api.InvoiceCreate{SubjectID: subj.ID})
	assertError(t, res, body, http.StatusForbidden, "your role (accountant)")
}
