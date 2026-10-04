package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

func listTodos(c *client, query string) api.ListResponse[api.Todo] {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.Todo]](c, http.StatusOK, "GET", c.acct("/todos")+query, nil)
}

func todoURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/todos"), id, suffix)
}

func TestTodoOverdueInvoice(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), IssuedOn: "2026-02-01", DueDays: intPtr(14),
		Lines: []api.InvoiceLineInput{line("Práce", "1", 1000_00, nil)}})

	mustRunJob(ts, "todos")
	mustRunJob(ts, "todos") // idempotent
	l := listTodos(a, "?completed=false")
	if l.Total != 1 || l.Items[0].Name != "invoice.overdue" || !l.Items[0].Automatic || l.Items[0].RelatedID == nil ||
		*l.Items[0].RelatedID != inv.ID || l.Items[0].DueOn != "2026-02-15" {
		t.Fatalf("todos %+v", l.Items)
	}
	if n := listEvents(a, "?name=invoice.overdue").Total; n != 1 {
		t.Fatalf("invoice.overdue events %d, want exactly 1", n)
	}
	todo := l.Items[0]

	// automatic todos cannot be edited or deleted
	res, body := a.do("PATCH", todoURL(a, todo.ID, ""), map[string]any{"text": "x"})
	assertError(t, res, body, http.StatusConflict, "automatic")
	res, body = a.do("DELETE", todoURL(a, todo.ID, ""), nil)
	assertError(t, res, body, http.StatusConflict, "automatic")

	// paid → auto-completed; payment deleted → reopened
	p := pay(a, inv.ID, api.PaymentCreate{})
	if l := listTodos(a, "?completed=true"); l.Total != 1 || !l.Items[0].Completed {
		t.Fatalf("after payment: %+v", l.Items)
	}
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv.ID, fmt.Sprintf("/payments/%d", p.Payment.ID)), nil)
	if l := listTodos(a, "?completed=false"); l.Total != 1 || l.Items[0].ID != todo.ID {
		t.Fatalf("after payment deleted: %+v", l.Items)
	}
	// completed by a person: stays completed while the condition persists
	a.mustDo(http.StatusOK, "POST", todoURL(a, todo.ID, "/toggle"), nil)
	mustRunJob(ts, "todos")
	if l := listTodos(a, "?completed=false"); l.Total != 0 {
		t.Fatalf("manually completed todo reopened: %+v", l.Items)
	}
	if n := listEvents(a, "?name=invoice.overdue").Total; n != 1 {
		t.Fatalf("invoice.overdue events %d", n)
	}
	// deleting the invoice removes its automatic todo
	a.mustDo(http.StatusOK, "POST", todoURL(a, todo.ID, "/toggle"), nil)
	action(a, inv.ID, "cancel")
	if l := listTodos(a, "?completed=false"); l.Total != 0 {
		t.Fatalf("cancelled invoice still has an open todo: %+v", l.Items)
	}
}

func TestTodoLowStock(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	item := newPriceItem(a, api.PriceItemCreate{Name: "Šroub", UnitName: "ks", UnitPrice: 10_00, TrackStock: true, StockQuantity: "10", MinStock: "5"})
	if l := listTodos(a, ""); l.Total != 0 {
		t.Fatalf("todo before: %+v", l.Items)
	}
	ln := line("Šroub", "6", 10_00, nil)
	ln.PriceItemID = &item.ID
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{ln}})
	l := listTodos(a, "?completed=false&related_type=price_item")
	if l.Total != 1 || l.Items[0].Name != "stock.low" {
		t.Fatalf("low stock todo: %+v", l.Items)
	}
	// rewriting the invoice's stock moves does not repeat stock.low
	a.mustDo(http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"note": "x"})
	if n := listEvents(a, "?name=stock.low").Total; n != 1 {
		t.Fatalf("stock.low events %d", n)
	}
	// restock → completed
	a.mustDo(http.StatusCreated, "POST", fmt.Sprintf("%s/%d/stock-moves", a.acct("/price-items"), item.ID),
		api.StockMoveCreate{Direction: "in", Quantity: "5"})
	if l := listTodos(a, "?completed=false"); l.Total != 0 {
		t.Fatalf("after restock: %+v", l.Items)
	}
	if n := listEvents(a, "?name=stock.moved").Total; n != 1 {
		t.Fatalf("stock.moved events %d", n)
	}
}

func TestTodoBankSuggestion(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	ba := model.BankAccount{AccountID: acc.ID, Name: "Hlavní", Currency: "CZK"}
	if err := ts.db.Create(&ba).Error; err != nil {
		t.Fatal(err)
	}
	id := uint(1)
	btx := model.BankTransaction{AccountID: acc.ID, BankAccountID: ba.ID, ExternalID: "x1", BookedOn: "2026-03-10", Amount: 500_00,
		Currency: "CZK", CounterpartyName: "ACME", SuggestionCount: 1,
		Suggestions: []model.MatchSuggestion{{InvoiceID: &id, Number: "2026-0001", Score: 60}}}
	if err := ts.db.Create(&btx).Error; err != nil {
		t.Fatal(err)
	}
	mustRunJob(ts, "todos")
	l := listTodos(a, "?completed=false")
	if l.Total != 1 || l.Items[0].Name != "bank.suggested" || l.Items[0].RelatedType != "bank_transaction" {
		t.Fatalf("todos %+v", l.Items)
	}
	a.mustDo(http.StatusOK, "POST", fmt.Sprintf("%s/%d/ignore", a.acct("/bank-transactions"), btx.ID), nil)
	if l := listTodos(a, "?completed=false"); l.Total != 0 {
		t.Fatalf("after ignore: %+v", l.Items)
	}
}

func TestTodoRecurringFailed(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	r := model.Recurring{AccountID: acc.ID, Name: "Hosting", TemplateID: 999, StartOn: "2026-03-01", NextOccurrenceOn: "2026-03-01",
		MonthsPeriod: 1, IssueAs: "invoice", Active: true}
	if err := ts.db.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	_ = runJob(ts, "recurring") // fails: template missing
	if n := listEvents(a, "?name=recurring.failed").Total; n != 1 {
		t.Fatalf("recurring.failed events %d", n)
	}
	if l := listTodos(a, "?completed=false"); l.Total != 1 || l.Items[0].Name != "recurring.failed" {
		t.Fatalf("todos %+v", l.Items)
	}
}

func TestManualTodos(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})

	td := doJSON[api.Todo](a, http.StatusCreated, "POST", a.acct("/todos"),
		api.TodoCreate{Text: "Zavolat klientovi", DueOn: "2026-03-20", RelatedType: "subject", RelatedID: &subj.ID})
	if td.Automatic || td.Completed || td.Name != "manual" || td.UserID == nil {
		t.Fatalf("created %+v", td)
	}
	res, body := a.do("POST", a.acct("/todos"), api.TodoCreate{Text: "x", RelatedType: "invoice", RelatedID: &subj.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "invoice not found")
	res, body = a.do("POST", a.acct("/todos"), api.TodoCreate{Text: "x", RelatedType: "subject"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "related_id")

	td = doJSON[api.Todo](a, http.StatusOK, "PATCH", todoURL(a, td.ID, ""), map[string]any{"text": "Zavolat", "due_on": ""})
	if td.Text != "Zavolat" || td.DueOn != "" {
		t.Fatalf("patched %+v", td)
	}
	td = doJSON[api.Todo](a, http.StatusOK, "POST", todoURL(a, td.ID, "/toggle"), nil)
	if !td.Completed || td.CompletedAt == nil {
		t.Fatalf("toggled %+v", td)
	}
	if l := listTodos(a, fmt.Sprintf("?completed=true&related_type=subject&related_id=%d", subj.ID)); l.Total != 1 {
		t.Fatalf("filter %+v", l)
	}
	td = doJSON[api.Todo](a, http.StatusOK, "PATCH", todoURL(a, td.ID, ""), map[string]any{"completed": false})
	if td.Completed {
		t.Fatalf("reopened %+v", td)
	}

	// tenant isolation
	res, body = b.do("PATCH", todoURL(b, td.ID, ""), map[string]any{"text": "x"})
	assertError(t, res, body, http.StatusNotFound, "todo not found")
	if l := listTodos(b, ""); l.Total != 0 {
		t.Fatalf("b sees %+v", l.Items)
	}
	a.mustDo(http.StatusNoContent, "DELETE", todoURL(a, td.ID, ""), nil)
	if l := listTodos(a, ""); l.Total != 0 {
		t.Fatalf("after delete %+v", l.Items)
	}
}
