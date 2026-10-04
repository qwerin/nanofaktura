package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
)

// TestTenantIsolationAllIDRoutes walks EVERY account-scoped route with an ID
// in its path (from the OpenAPI document, so new routes are covered
// automatically) and calls it as the owner of account B with the IDs of
// account A's records — alone and nested under B's own parent records.
// Reads and deletes must answer 404, mutations 404 (or 422 when the body is
// rejected before the handler runs), and A's data must be unchanged.
func TestTenantIsolationAllIDRoutes(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	richFixture(t, ts, a)
	a.mustDo(http.StatusCreated, "POST", a.acct("/members/invite"), api.InvitationCreate{Email: "novy@example.cz", Role: "member"})
	aSubj := newSubject(a, api.SubjectCreate{Name: "Pozdější klient"})
	createInv(a, api.InvoiceCreate{SubjectID: new(aSubj.ID), Lines: []api.InvoiceLineInput{line("Po webhooku", "", 100, nil)}}) // → a webhook delivery
	srcAcc := accountOf(t, ts, a.slug)
	before := dumpAccount(t, ts, srcAcc.ID)

	// B's own parent records (for nested routes: B's parent + A's child)
	bSubj := newSubject(b, api.SubjectCreate{Name: "B klient"})
	bInv := createInv(b, api.InvoiceCreate{SubjectID: new(bSubj.ID), Lines: []api.InvoiceLineInput{line("B", "", 100, nil)}})
	bExp := createExp(b, api.ExpenseCreate{SubjectID: &bSubj.ID, Lines: []api.InvoiceLineInput{line("B", "", 100, nil)}})
	bItem := doJSON[api.PriceItem](b, http.StatusCreated, "POST", b.acct("/price-items"), api.PriceItemCreate{Name: "B", UnitPrice: 1})
	bHook := doJSON[api.Webhook](b, http.StatusCreated, "POST", b.acct("/webhooks"), api.WebhookCreate{URL: "http://127.0.0.1:9/b", Events: []string{"*"}})

	idsOf := func(m any, where string, args ...any) []uint {
		var ids []uint
		if err := ts.db.Model(m).Where(where, args...).Order("id").Pluck("id", &ids).Error; err != nil {
			t.Fatal(err)
		}
		return ids
	}
	byAcc := func(m any) []uint { return idsOf(m, "account_id = ?", srcAcc.ID) }
	var userIDs []uint
	ts.db.Model(&model.Membership{}).Where("account_id = ?", srcAcc.ID).Pluck("user_id", &userIDs)

	// resource segment → IDs of A's records of that resource
	ids := map[string][]uint{
		"attachments":       byAcc(&model.Attachment{}),
		"bank-accounts":     byAcc(&model.BankAccount{}),
		"bank-transactions": byAcc(&model.BankTransaction{}),
		"expenses":          byAcc(&model.Expense{}),
		"invitations":       byAcc(&model.Invitation{}),
		"invoices":          byAcc(&model.Invoice{}),
		"members":           userIDs,
		"number-formats":    byAcc(&model.NumberFormat{}),
		"price-items":       byAcc(&model.PriceItem{}),
		"recurring":         byAcc(&model.Recurring{}),
		"subjects":          byAcc(&model.Subject{}),
		"templates":         byAcc(&model.InvoiceTemplate{}),
		"todos":             byAcc(&model.Todo{}),
		"webhooks":          byAcc(&model.Webhook{}),
	}
	children := map[string][]uint{
		"invoices/payments":       byAcc(&model.Payment{}),
		"expenses/payments":       byAcc(&model.ExpensePayment{}),
		"price-items/stock-moves": byAcc(&model.StockMove{}),
		"webhooks/deliveries":     byAcc(&model.WebhookDelivery{}),
	}
	ownParent := map[string]uint{"invoices": bInv.ID, "expenses": bExp.ID, "price-items": bItem.ID, "webhooks": bHook.ID}
	for k, v := range ids {
		if len(v) == 0 {
			t.Fatalf("fixture has no %s", k)
		}
	}
	for k, v := range children {
		if len(v) == 0 {
			t.Fatalf("fixture has no %s", k)
		}
	}

	// valid bodies, so the handler (not input validation) answers
	bodies := map[string]any{
		"PATCH /api/accounts/{slug}/members/{user_id}":           map[string]any{"role": "member"},
		"POST /api/accounts/{slug}/bank-transactions/{id}/match": api.BankTransactionMatch{InvoiceID: &bInv.ID},
		"POST /api/accounts/{slug}/price-items/{id}/stock-moves": api.StockMoveCreate{Direction: "in", Quantity: "1"},
	}
	_, humaAPI := api.New(nil, config.Config{}, api.Deps{})
	const prefix = "/api/accounts/{slug}/"
	calls := 0
	statuses := map[int]int{}
	rejected := map[string]bool{} // operations answered by validation, not by the handler
	for path, item := range humaAPI.OpenAPI().Paths {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok || !strings.Contains(rest, "{") {
			continue
		}
		seg := strings.Split(rest, "/")
		resource := seg[0]
		parentIDs, known := ids[resource]
		if !known {
			t.Errorf("%s: no fixture IDs for resource %q — add it to this test", path, resource)
			continue
		}
		var childKey string
		if len(seg) >= 4 && strings.HasPrefix(seg[3], "{") && seg[3] != "{action}" {
			childKey = resource + "/" + seg[2]
			if _, ok := children[childKey]; !ok {
				t.Errorf("%s: no fixture IDs for %q — add it to this test", path, childKey)
				continue
			}
		}
		for method, op := range map[string]*huma.Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete,
		} {
			if op == nil {
				continue
			}
			// URLs to try: A's record; nested: A's parent + A's child and B's parent + A's child
			var urls []string
			fill := func(parent, child uint) string {
				out := make([]string, len(seg))
				for i, s := range seg {
					switch {
					case i == 1 && s == "{id}", i == 1 && s == "{user_id}":
						out[i] = fmt.Sprint(parent)
					case s == "{action}" && resource == "expenses":
						out[i] = "lock"
					case s == "{action}":
						out[i] = "cancel"
					case i == 3 && strings.HasPrefix(s, "{"):
						out[i] = fmt.Sprint(child)
					default:
						out[i] = s
					}
				}
				return b.acct("/" + strings.Join(out, "/"))
			}
			for _, pid := range parentIDs {
				if childKey == "" {
					urls = append(urls, fill(pid, 0))
					continue
				}
				for _, cid := range children[childKey] {
					urls = append(urls, fill(pid, cid))
				}
			}
			if childKey != "" {
				for _, cid := range children[childKey] {
					urls = append(urls, fill(ownParent[resource], cid))
				}
			}
			for _, u := range urls {
				var body any
				if method == "POST" || method == "PATCH" || method == "PUT" {
					body = map[string]any{}
					if bb, ok := bodies[method+" "+path]; ok {
						body = bb
					}
				}
				res, rb := b.do(method, u, body)
				calls++
				statuses[res.StatusCode]++
				if res.StatusCode != http.StatusNotFound {
					rejected[method+" "+path] = true
				}
				ok := res.StatusCode == http.StatusNotFound
				if body != nil && (res.StatusCode == http.StatusUnprocessableEntity || res.StatusCode == http.StatusUnsupportedMediaType) {
					ok = true // body rejected before the handler ran
				}
				if !ok {
					t.Errorf("%s %s: %d %s — want 404", method, u, res.StatusCode, rb)
				}
				if strings.Contains(string(rb), "ACME") || strings.Contains(string(rb), "Firma A") {
					t.Errorf("%s %s leaks A's data: %s", method, u, rb)
				}
			}
		}
	}
	t.Logf("%d calls, statuses %v; rejected before the handler: %v", calls, statuses, rejected)
	if calls < 100 {
		t.Fatalf("only %d calls made", calls)
	}
	after := dumpAccount(t, ts, srcAcc.ID)
	for table := range before {
		if !reflect.DeepEqual(before[table], after[table]) {
			t.Errorf("A's %s changed by B's requests:\nbefore: %v\nafter:  %v", table, before[table], after[table])
		}
	}
}
