package api_test

import (
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestMarkInvoicesPaid(t *testing.T) {
	ts := newTestServer(t) // today 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"paid_thanks_enabled": true})
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "acme@example.cz"})
	days := func(n int) *int { return &n }
	newInv := func(issued string, due int, price int64) api.Invoice {
		return createInv(a, api.InvoiceCreate{SubjectID: subj.ID, IssuedOn: issued, DueDays: days(due),
			Lines: []api.InvoiceLineInput{line("X", "1", price, nil)}})
	}
	old := newInv("2025-01-10", 14, 10000)     // due 2025-01-24
	partial := newInv("2025-02-01", 14, 20000) // due 2025-02-15, partly paid
	pay(a, partial.ID, api.PaymentCreate{Amount: i64(5000), PaidOn: "2025-02-10"})
	future := newInv("2026-03-10", 30, 3000) // due 2026-04-09 → paid today
	paid := newInv("2025-03-01", 14, 700)
	pay(a, paid.ID, api.PaymentCreate{})
	cancelled := newInv("2025-04-01", 14, 900)
	action(a, cancelled.ID, "cancel")
	bOther := createInv(b, api.InvoiceCreate{SubjectID: newSubject(b, api.SubjectCreate{Name: "B"}).ID,
		Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	mails := len(ts.mail.Messages())

	url := a.acct("/invoices/mark-paid")
	dry := doJSON[api.MarkPaidResult](a, http.StatusOK, "POST", url, api.MarkPaidRequest{DryRun: true})
	if dry.Count != 3 || len(dry.Sums) != 1 || dry.Sums[0].SumRemaining != 10000+15000+3000 {
		t.Fatalf("dry run: %+v", dry)
	}
	if g := getInv(a, old.ID); g.Status == "paid" {
		t.Fatal("dry run must not pay")
	}

	// the filter narrows the documents: only issued until 2025-12-31
	r := doJSON[api.MarkPaidResult](a, http.StatusOK, "POST", url+"?until=2025-12-31&status=unpaid", api.MarkPaidRequest{})
	if r.Count != 2 || r.Sums[0].SumRemaining != 25000 {
		t.Fatalf("filtered: %+v", r)
	}
	if g := getInv(a, old.ID); g.Status != "paid" || g.PaidOn != "2025-01-24" || len(g.Payments) != 1 || g.Payments[0].Amount != 10000 {
		t.Fatalf("old: %+v", g.InvoiceSummary)
	}
	if g := getInv(a, partial.ID); g.Status != "paid" || g.PaidOn != "2025-02-15" || g.Payments[1].Amount != 15000 {
		t.Fatalf("partial: %+v", g.InvoiceSummary)
	}
	if g := getInv(a, future.ID); g.Status == "paid" {
		t.Fatal("outside the filter")
	}
	if g := getInv(a, cancelled.ID); g.Status != "cancelled" || len(g.Payments) != 0 {
		t.Fatalf("cancelled: %+v", g.InvoiceSummary)
	}

	// explicit date; due date in the future → that date anyway
	r = doJSON[api.MarkPaidResult](a, http.StatusOK, "POST", url, api.MarkPaidRequest{PaidOn: "2026-03-01"})
	if r.Count != 1 {
		t.Fatalf("rest: %+v", r)
	}
	if g := getInv(a, future.ID); g.Status != "paid" || g.PaidOn != "2026-03-01" {
		t.Fatalf("future: %+v", g.InvoiceSummary)
	}
	if n := len(ts.mail.Messages()); n != mails {
		t.Fatalf("bulk marking must not send thank-you e-mails (%d → %d)", mails, n)
	}
	// nothing left; other accounts untouched
	if r = doJSON[api.MarkPaidResult](a, http.StatusOK, "POST", url, api.MarkPaidRequest{}); r.Count != 0 || r.Sums == nil {
		t.Fatalf("empty: %+v", r)
	}
	if g := getInv(b, bOther.ID); g.Status == "paid" {
		t.Fatal("tenant isolation")
	}

	res, body := a.do("POST", url, map[string]any{"paid_on": "2026-02-30"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "paid_on")
	res, body = a.do("POST", url+"?status=bogus", api.MarkPaidRequest{})
	assertError(t, res, body, http.StatusUnprocessableEntity, "status")
}

func TestMarkExpensesPaid(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	name := "Dodavatel"
	newExp := func(issued, due string, price int64) api.Expense {
		return createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name},
			IssuedOn: issued, DueOn: due, Lines: []api.InvoiceLineInput{line("X", "1", price, i32(0))}})
	}
	e1 := newExp("2025-05-01", "2025-05-20", 4000)
	e2 := newExp("2026-03-01", "2026-04-01", 6000)

	url := a.acct("/expenses/mark-paid")
	if dry := doJSON[api.MarkPaidResult](a, http.StatusOK, "POST", url, api.MarkPaidRequest{DryRun: true}); dry.Count != 2 {
		t.Fatalf("dry: %+v", dry)
	}
	r := doJSON[api.MarkPaidResult](a, http.StatusOK, "POST", url, api.MarkPaidRequest{})
	if r.Count != 2 || r.Sums[0].SumRemaining != 10000 {
		t.Fatalf("marked: %+v", r)
	}
	if g := getExp(a, e1.ID); g.Status != "paid" || g.PaidOn != "2025-05-20" {
		t.Fatalf("e1: %+v", g.ExpenseSummary)
	}
	if g := getExp(a, e2.ID); g.Status != "paid" || g.PaidOn != "2026-03-15" { // due in the future → today
		t.Fatalf("e2: %+v", g.ExpenseSummary)
	}
}
