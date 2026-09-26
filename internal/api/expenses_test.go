package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

// ---- helpers ----

func expURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/expenses"), id, suffix)
}

func createExp(c *client, body api.ExpenseCreate) api.Expense {
	c.ts.t.Helper()
	return doJSON[api.Expense](c, http.StatusCreated, "POST", c.acct("/expenses"), body)
}

func getExp(c *client, id uint) api.Expense {
	c.ts.t.Helper()
	return doJSON[api.Expense](c, http.StatusOK, "GET", expURL(c, id, ""), nil)
}

func listExp(c *client, query string) api.ListResponse[api.ExpenseSummary] {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.ExpenseSummary]](c, http.StatusOK, "GET", c.acct("/expenses")+query, nil)
}

func assertExpNumbers(t *testing.T, l api.ListResponse[api.ExpenseSummary], want ...string) {
	t.Helper()
	got := []string{}
	for _, e := range l.Items {
		got = append(got, e.Number)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) || l.Total != int64(len(want)) {
		t.Fatalf("numbers %v (total %d), want %v", got, l.Total, want)
	}
}

func payExp(c *client, id uint, body api.ExpensePaymentCreate) api.ExpensePaymentResult {
	c.ts.t.Helper()
	return doJSON[api.ExpensePaymentResult](c, http.StatusCreated, "POST", expURL(c, id, "/payments"), body)
}

// ---- tests ----

func TestExpenseCreate(t *testing.T) {
	ts := newTestServer(t) // today 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	sup := newSubject(a, api.SubjectCreate{Name: "Alza", FullName: "Alza.cz a.s.", RegistrationNo: "27082440",
		VatNo: "CZ27082440", Street: "Jankovcova 1522/53", City: "Praha", Zip: "17000",
		BankAccount: "2000145399/2010", IBAN: "CZ9320100000002000145399"})

	e := createExp(a, api.ExpenseCreate{SubjectID: &sup.ID, OriginalNumber: "FV-2026/0042", Category: " Hardware ",
		Description: "Notebook", Tags: []string{"it", "it", " "}, Lines: []api.InvoiceLineInput{
			line("Notebook", "1", 2000000, i32(2100)),
			line("Myš", "2", 50000, nil), // default rate 21 %
		}})
	s := e.ExpenseSummary
	if s.Number != "N2026-0001" || s.OriginalNumber != "FV-2026/0042" || s.VariableSymbol != "20260042" || s.Status != "open" ||
		s.SubjectID == nil || *s.SubjectID != sup.ID || s.IssuedOn != "2026-03-15" || s.TaxableFulfillmentDue != "2026-03-15" ||
		s.DueOn != "2026-03-29" || s.PaidOn != "" || s.LockedAt != nil || s.Currency != "CZK" || s.ExchangeRate != "1" ||
		s.PaymentMethod != "bank" || s.Category != "Hardware" || s.Description != "Notebook" || !s.TaxDeductible ||
		!reflect.DeepEqual(s.Tags, []string{"it"}) {
		t.Fatalf("defaults: %+v", s)
	}
	if s.SupplierName != "Alza" || s.SupplierFullName != "Alza.cz a.s." || s.SupplierRegistrationNo != "27082440" ||
		s.SupplierVatNo != "CZ27082440" || s.SupplierStreet != "Jankovcova 1522/53" || s.SupplierCity != "Praha" ||
		s.SupplierZip != "17000" || s.SupplierCountry != "CZ" || s.SupplierBankAccount != "2000145399/2010" ||
		s.SupplierIBAN != "CZ9320100000002000145399" {
		t.Fatalf("supplier snapshot: %+v", s)
	}
	// VAT is computed even though the account is not a VAT payer (supplier's VAT)
	if s.Subtotal != 2100000 || s.VatTotal != 441000 || s.Total != 2541000 || s.RemainingAmount != 2541000 ||
		len(e.Lines) != 2 || e.Lines[1].VatRateBps != 2100 || e.Lines[1].Base != 100000 || e.Lines[1].Position != 2 ||
		!reflect.DeepEqual(e.VatRecap, []api.VatRecapItem{{VatRateBps: 2100, Base: 2100000, Vat: 441000, Total: 2541000}}) ||
		len(e.Payments) != 0 {
		t.Fatalf("totals: %+v lines %+v recap %+v", s, e.Lines, e.VatRecap)
	}

	// without a subject: supplier_name required; explicit values win
	name := "Trafika"
	e2 := createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, IssuedOn: "2026-02-01", DueOn: "2026-02-05",
		TaxableFulfillmentDue: strPtr(""), VariableSymbol: strPtr("123"), TaxDeductible: new(bool), PaymentMethod: "cash",
		PricesIncludeVat: true, RoundTotal: true, Currency: "EUR", ExchangeRate: "24.5",
		Lines: []api.InvoiceLineInput{line("Noviny", "1", 12150, i32(1200))}})
	if e2.Number != "N2026-0002" || e2.SubjectID != nil || e2.SupplierName != "Trafika" || e2.DueOn != "2026-02-05" ||
		e2.TaxableFulfillmentDue != "" || e2.VariableSymbol != "123" || e2.TaxDeductible || e2.PaymentMethod != "cash" ||
		e2.Currency != "EUR" || e2.Total != 12200 || e2.Rounding != 50 || e2.VatTotal != 1302 || e2.Status != "overdue" {
		t.Fatalf("explicit: %+v", e2.ExpenseSummary)
	}
	// custom number; duplicates → 409
	e3 := createExp(a, api.ExpenseCreate{Number: "VLASTNI-1", ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Lines: []api.InvoiceLineInput{line("X", "", 1, nil)}})
	if e3.Number != "VLASTNI-1" || e3.VariableSymbol != "" {
		t.Fatalf("custom: %+v", e3.ExpenseSummary)
	}
	res, body := a.do("POST", a.acct("/expenses"), api.ExpenseCreate{Number: "VLASTNI-1", ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name},
		Lines: []api.InvoiceLineInput{line("X", "", 1, nil)}})
	assertError(t, res, body, http.StatusConflict, "VLASTNI-1 already exists")

	// validation
	other := ts.signup("b@example.cz", "Firma B")
	foreign := newSubject(other, api.SubjectCreate{Name: "Cizí"})
	for _, c := range []struct {
		body  api.ExpenseCreate
		field string
	}{
		{api.ExpenseCreate{Lines: []api.InvoiceLineInput{line("X", "", 1, nil)}}, "body.supplier_name"},
		{api.ExpenseCreate{SubjectID: &foreign.ID, Lines: []api.InvoiceLineInput{line("X", "", 1, nil)}}, "body.subject_id"},
		{api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}}, "body.lines"},
		{api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Lines: []api.InvoiceLineInput{line("X", "abc", 1, nil)}}, "body.lines[0].quantity"},
		{api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, TaxableFulfillmentDue: strPtr("x"), Lines: []api.InvoiceLineInput{line("X", "", 1, nil)}}, "body.taxable_fulfillment_due"},
		{api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, IssuedOn: "2026-02-31", Lines: []api.InvoiceLineInput{line("X", "", 1, nil)}}, "issued_on"},
	} {
		res, body := a.do("POST", a.acct("/expenses"), c.body)
		assertError(t, res, body, http.StatusUnprocessableEntity, c.field)
	}
}

func TestExpenseListAndCategories(t *testing.T) {
	ts := newTestServer(t) // today 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	alza := newSubject(a, api.SubjectCreate{Name: "Alza"})
	o2 := newSubject(a, api.SubjectCreate{Name: "O2 Czech"})
	mk := func(subj uint, issued, due, category, orig string, price int64) api.Expense {
		return createExp(a, api.ExpenseCreate{SubjectID: &subj, IssuedOn: issued, DueOn: due, Category: category,
			OriginalNumber: orig, Lines: []api.InvoiceLineInput{line("X", "1", price, i32(0))}})
	}
	e1 := mk(alza.ID, "2026-01-10", "2026-01-20", "Hardware", "A-1", 1000) // overdue
	e2 := mk(o2.ID, "2026-02-10", "2026-03-31", "Telefon", "T_1", 3000)    // open
	e3 := mk(alza.ID, "2026-03-01", "2026-03-10", "Hardware", "A-2", 2000) // paid
	payExp(a, e3.ID, api.ExpensePaymentCreate{})

	assertExpNumbers(t, listExp(a, ""), e3.Number, e2.Number, e1.Number)
	assertExpNumbers(t, listExp(a, "?status=overdue"), e1.Number)
	assertExpNumbers(t, listExp(a, "?status=open"), e2.Number)
	assertExpNumbers(t, listExp(a, "?status=paid"), e3.Number)
	assertExpNumbers(t, listExp(a, "?category=Hardware&sort=issued_on"), e1.Number, e3.Number)
	assertExpNumbers(t, listExp(a, fmt.Sprintf("?subject_id=%d", o2.ID)), e2.Number)
	assertExpNumbers(t, listExp(a, "?since=2026-02-01&until=2026-02-28"), e2.Number)
	assertExpNumbers(t, listExp(a, "?query=o2"), e2.Number)
	assertExpNumbers(t, listExp(a, "?query=t_"), e2.Number) // _ is literal
	assertExpNumbers(t, listExp(a, "?sort=-total"), e2.Number, e3.Number, e1.Number)
	assertExpNumbers(t, listExp(a, "?sort=due_on"), e1.Number, e3.Number, e2.Number)
	if l := listExp(a, "?status=overdue"); l.Items[0].Status != "overdue" {
		t.Fatalf("effective status: %+v", l.Items[0])
	}
	res, body := a.do("GET", a.acct("/expenses?status=cancelled"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "status")

	cats := doJSON[api.ExpenseCategories](a, http.StatusOK, "GET", a.acct("/expenses/categories"), nil)
	if !reflect.DeepEqual(cats.Items, []string{"Hardware", "Telefon"}) {
		t.Fatalf("categories: %+v", cats)
	}
	cats = doJSON[api.ExpenseCategories](a, http.StatusOK, "GET", a.acct("/expenses/categories?query=tel"), nil)
	if !reflect.DeepEqual(cats.Items, []string{"Telefon"}) {
		t.Fatalf("categories query: %+v", cats)
	}
}

func TestExpensePatchDelete(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	alza := newSubject(a, api.SubjectCreate{Name: "Alza"})
	o2 := newSubject(a, api.SubjectCreate{Name: "O2", Street: "Za Brumlovkou"})
	e := createExp(a, api.ExpenseCreate{SubjectID: &alza.ID, Lines: []api.InvoiceLineInput{
		line("A", "1", 1000, i32(0)), line("B", "2", 500, i32(0)),
	}})
	createExp(a, api.ExpenseCreate{Number: "OTHER", SubjectID: &alza.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}})

	keep := line("A2", "3", 1000, i32(2100))
	keep.ID = &e.Lines[0].ID
	p := doJSON[api.Expense](a, http.StatusOK, "PATCH", expURL(a, e.ID, ""), map[string]any{
		"subject_id": o2.ID, "category": "Služby", "issued_on": "2026-03-01", "due_on": "2026-03-20",
		"description": "d", "tags": []string{"x"}, "tax_deductible": false, "lines": []api.InvoiceLineInput{keep, line("C", "1", 100, i32(0))},
	})
	if p.SubjectID == nil || *p.SubjectID != o2.ID || p.SupplierName != "O2" || p.SupplierStreet != "Za Brumlovkou" ||
		p.Category != "Služby" || p.IssuedOn != "2026-03-01" || p.DueOn != "2026-03-20" || p.TaxDeductible ||
		len(p.Lines) != 2 || p.Lines[0].ID != e.Lines[0].ID || p.Lines[0].Name != "A2" || p.Lines[1].Name != "C" ||
		p.Subtotal != 3100 || p.VatTotal != 630 || p.Total != 3730 {
		t.Fatalf("patch: %+v lines %+v", p.ExpenseSummary, p.Lines)
	}
	// explicit supplier override wins over re-snapshot
	p = doJSON[api.Expense](a, http.StatusOK, "PATCH", expURL(a, e.ID, ""), map[string]any{"subject_id": alza.ID, "supplier_name": "Alza CZ"})
	if p.SupplierName != "Alza CZ" || *p.SubjectID != alza.ID {
		t.Fatalf("override: %+v", p.ExpenseSummary)
	}

	for _, c := range []struct {
		body  any
		field string
		code  int
	}{
		{map[string]any{"lines": []map[string]any{{"id": 99999, "name": "X", "unit_price": 1}}}, "body.lines[0].id", 422},
		{map[string]any{"lines": []map[string]any{{"id": e.Lines[0].ID, "name": "X", "unit_price": 1}, {"id": e.Lines[0].ID, "name": "Y", "unit_price": 1}}}, "body.lines[1].id", 422},
		{map[string]any{"due_on": "2026-13-01"}, "due_on", 422},
		{map[string]any{"number": "OTHER"}, "OTHER already exists", 409},
		{map[string]any{"subject_id": 99999}, "body.subject_id", 422},
	} {
		res, body := a.do("PATCH", expURL(a, e.ID, ""), c.body)
		assertError(t, res, body, c.code, c.field)
	}

	// lock blocks PATCH and DELETE
	l := doJSON[api.Expense](a, http.StatusOK, "POST", expURL(a, e.ID, "/actions/lock"), nil)
	if l.LockedAt == nil {
		t.Fatalf("lock: %+v", l.ExpenseSummary)
	}
	res, body := a.do("PATCH", expURL(a, e.ID, ""), map[string]any{"description": "x"})
	assertError(t, res, body, http.StatusConflict, "locked")
	res, body = a.do("DELETE", expURL(a, e.ID, ""), nil)
	assertError(t, res, body, http.StatusConflict, "locked")
	res, body = a.do("POST", expURL(a, e.ID, "/actions/cancel"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "action")
	if u := doJSON[api.Expense](a, http.StatusOK, "POST", expURL(a, e.ID, "/actions/unlock"), nil); u.LockedAt != nil {
		t.Fatalf("unlock: %+v", u.ExpenseSummary)
	}

	// payments block DELETE
	pr := payExp(a, e.ID, api.ExpensePaymentCreate{Amount: i64(100)})
	res, body = a.do("DELETE", expURL(a, e.ID, ""), nil)
	assertError(t, res, body, http.StatusConflict, "payments")
	a.mustDo(http.StatusNoContent, "DELETE", expURL(a, e.ID, fmt.Sprintf("/payments/%d", pr.Payment.ID)), nil)
	a.mustDo(http.StatusNoContent, "DELETE", expURL(a, e.ID, ""), nil)
	res, body = a.do("GET", expURL(a, e.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "expense not found")
}

func TestExpensePayments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	name := "Dodavatel"
	e := createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Lines: []api.InvoiceLineInput{line("X", "1", 10000, i32(0))}})

	r := payExp(a, e.ID, api.ExpensePaymentCreate{Amount: i64(4000), PaidOn: "2026-03-10", Note: "záloha"})
	if r.Payment.Amount != 4000 || r.Payment.ExpenseID != e.ID || r.Payment.PaidOn != "2026-03-10" || r.Payment.Note != "záloha" ||
		r.Expense.Status != "open" || r.Expense.PaidAmount != 4000 || r.Expense.RemainingAmount != 6000 || len(r.Expense.Payments) != 1 {
		t.Fatalf("partial: %+v", r)
	}
	r2 := payExp(a, e.ID, api.ExpensePaymentCreate{}) // default: the remaining amount, today
	if r2.Payment.Amount != 6000 || r2.Expense.Status != "paid" || r2.Expense.PaidOn != "2026-03-15" || r2.Expense.RemainingAmount != 0 {
		t.Fatalf("full: %+v", r2.Expense.ExpenseSummary)
	}
	res, body := a.do("POST", expURL(a, e.ID, "/payments"), api.ExpensePaymentCreate{})
	assertError(t, res, body, http.StatusConflict, "nothing to pay")
	res, body = a.do("POST", expURL(a, e.ID, "/payments"), api.ExpensePaymentCreate{Amount: i64(0)})
	assertError(t, res, body, http.StatusUnprocessableEntity, "body.amount")
	res, body = a.do("POST", expURL(a, e.ID, "/payments"), api.ExpensePaymentCreate{Amount: i64(1), PaidOn: "2026-02-30"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "paid_on")

	// a PATCH that raises the total reopens the expense
	g := doJSON[api.Expense](a, http.StatusOK, "PATCH", expURL(a, e.ID, ""), api.ExpensePatch{
		Lines: []api.InvoiceLineInput{line("X", "2", 10000, i32(0))}})
	if g.Status != "open" || g.PaidOn != "" || g.RemainingAmount != 10000 {
		t.Fatalf("reopened: %+v", g.ExpenseSummary)
	}
	a.mustDo(http.StatusNoContent, "DELETE", expURL(a, e.ID, fmt.Sprintf("/payments/%d", r2.Payment.ID)), nil)
	if g := getExp(a, e.ID); g.PaidAmount != 4000 || len(g.Payments) != 1 || g.Status != "open" {
		t.Fatalf("after delete: %+v", g.ExpenseSummary)
	}
	res, body = a.do("DELETE", expURL(a, e.ID, fmt.Sprintf("/payments/%d", r2.Payment.ID)), nil)
	assertError(t, res, body, http.StatusNotFound, "payment not found")
}

func TestExpensesTenantIsolation(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	name := "Dodavatel"
	e := createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Category: "Tajné", Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	p := payExp(a, e.ID, api.ExpensePaymentCreate{Amount: i64(10)}).Payment

	for _, r := range []struct {
		method, suffix string
		body           any
	}{
		{"GET", "", nil},
		{"PATCH", "", map[string]any{"description": "x"}},
		{"DELETE", "", nil},
		{"POST", "/actions/lock", nil},
		{"POST", "/payments", api.ExpensePaymentCreate{}},
		{"DELETE", fmt.Sprintf("/payments/%d", p.ID), nil},
	} {
		res, body := b.do(r.method, expURL(b, e.ID, r.suffix), r.body)
		assertError(t, res, body, http.StatusNotFound, "expense not found")
		res, body = b.do(r.method, expURL(a, e.ID, r.suffix), r.body)
		assertError(t, res, body, http.StatusNotFound, "not found")
	}
	if l := listExp(b, ""); l.Total != 0 {
		t.Fatalf("b sees expenses: %+v", l)
	}
	if c := doJSON[api.ExpenseCategories](b, http.StatusOK, "GET", b.acct("/expenses/categories"), nil); len(c.Items) != 0 {
		t.Fatalf("b sees categories: %+v", c)
	}
	if d := doJSON[api.Dashboard](b, http.StatusOK, "GET", b.acct("/dashboard"), nil); d.ExpensesTotal != 0 {
		t.Fatalf("b dashboard: %+v", d)
	}
	if g := getExp(a, e.ID); g.Description != "" || g.LockedAt != nil || len(g.Payments) != 1 {
		t.Fatalf("a's expense changed: %+v", g.ExpenseSummary)
	}
	// b's numbering is independent
	if n := createExp(b, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}}).Number; n != "N2026-0001" {
		t.Fatalf("b number: %s", n)
	}
}

func TestDashboardExpenses(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	createInv(a, api.InvoiceCreate{SubjectID: subj.ID, IssuedOn: "2026-02-05", Lines: []api.InvoiceLineInput{line("X", "1", 10000, nil)}})
	name := "Dodavatel"
	mk := func(issued, currency string, price int64) {
		createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, IssuedOn: issued, Currency: currency,
			Lines: []api.InvoiceLineInput{line("X", "1", price, i32(0))}})
	}
	mk("2026-01-10", "", 1500)
	mk("2026-01-20", "", 500)
	mk("2026-03-01", "", 3000)
	mk("2026-03-02", "EUR", 999) // other currency
	mk("2025-12-31", "", 700)    // other year

	d := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard"), nil)
	if !reflect.DeepEqual(d.ExpensesByMonth, []int64{2000, 0, 3000, 0, 0, 0, 0, 0, 0, 0, 0, 0}) ||
		d.ExpensesTotal != 5000 || d.RevenueTotal != 10000 || d.ProfitTotal != 5000 {
		t.Fatalf("dashboard: %+v", d)
	}
	d = doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard?year=2025"), nil)
	if d.ExpensesByMonth[11] != 700 || d.ExpensesTotal != 700 || d.ProfitTotal != -700 {
		t.Fatalf("2025: %+v", d)
	}
}

func TestExpenseAttachments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	name := "Dodavatel"
	e := createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name},
		Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	att := uploadOK(a, "expense", e.ID, "uctenka.pdf", []byte("%PDF-1.4 test"))
	if att.OwnerType != "expense" || att.OwnerID != e.ID {
		t.Fatalf("attachment: %+v", att)
	}
	res, body := b.upload(map[string]string{"owner_type": "expense", "owner_id": fmt.Sprint(e.ID)}, "x.pdf", []byte("%PDF-1.4"))
	assertError(t, res, body, http.StatusNotFound, "owner not found")
}
