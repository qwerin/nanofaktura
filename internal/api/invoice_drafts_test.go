package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

// TestInvoiceDrafts: a draft has no number, counts nowhere and cannot be paid,
// sent or shared until it is issued (SPEC §4.5 "Koncepty").
func TestInvoiceDrafts(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "acme@example.cz"})
	item := doJSON[api.PriceItem](a, http.StatusCreated, "POST", a.acct("/price-items"),
		api.PriceItemCreate{Name: "Krabice", UnitPrice: 1000, TrackStock: true, StockQuantity: "10"})
	lines := []api.InvoiceLineInput{{Name: "Krabice", Quantity: "2", UnitPrice: 50000, PriceItemID: &item.ID}}

	d1 := createInv(a, api.InvoiceCreate{Draft: true, SubjectID: new(acme.ID), Lines: lines})
	d2 := createInv(a, api.InvoiceCreate{Draft: true, SubjectID: new(acme.ID), Lines: lines}) // several drafts without a number
	if d1.Status != "draft" || d1.Number != "" || d1.VariableSymbol != "" || d1.Total != 100000 || d2.Number != "" {
		t.Fatalf("draft: %+v", d1.InvoiceSummary)
	}
	stock := func() string {
		return doJSON[api.PriceItem](a, http.StatusOK, "GET", fmt.Sprintf("%s/%d", a.acct("/price-items"), item.ID), nil).StockQuantity
	}
	if q := stock(); q != "10" {
		t.Fatalf("a draft moved stock: %s", q)
	}

	// listed (and filterable), but not counted in sums, dashboard or reports
	if l := listInv(a, ""); l.Total != 2 {
		t.Fatalf("list: %+v", l)
	}
	if l := listInv(a, "?status=unpaid"); l.Total != 0 {
		t.Fatalf("unpaid contains drafts: %+v", l)
	}
	page := doJSON[api.DocumentList[api.InvoiceSummary]](a, http.StatusOK, "GET", a.acct("/invoices?status=draft"), nil)
	if page.Total != 2 || len(page.Sums) != 0 {
		t.Fatalf("draft filter: total %d sums %+v", page.Total, page.Sums)
	}
	dash := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard?year=2026"), nil)
	if dash.RevenueTotal != 0 || dash.UnpaidCount != 0 {
		t.Fatalf("dashboard counts drafts: %+v", dash)
	}

	// nothing that needs an issued document
	for _, c := range []struct{ method, path string }{
		{"POST", invURL(a, d1.ID, "/payments")},
		{"POST", invURL(a, d1.ID, "/send")},
		{"GET", invURL(a, d1.ID, "/isdoc")},
		{"POST", invURL(a, d1.ID, "/correction")},
	} {
		res, body := a.do(c.method, c.path, map[string]any{})
		assertCode(t, res, body, http.StatusConflict, "invoice_draft")
	}
	if n := len(ts.mail.Messages()); n != 0 {
		t.Fatalf("%d e-mails", n)
	}
	res, body := ts.anon().do("GET", "/api/public/invoices/"+d1.PublicToken, nil)
	assertError(t, res, body, http.StatusNotFound, "not found")
	for _, act := range []string{"mark_as_sent", "cancel", "mark_as_uncollectible"} {
		res, body := a.do("POST", invURL(a, d1.ID, "/actions/"+act), nil)
		assertCode(t, res, body, http.StatusConflict, "invalid_transition")
	}
	// the PDF is a preview marked as draft
	if res, body := a.do("GET", invURL(a, d1.ID, "/pdf"), nil); res.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "%PDF") {
		t.Fatalf("pdf: %d", res.StatusCode)
	}
	// edits keep it a draft; a copy of a draft is a draft
	patched := doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, d1.ID, ""), api.InvoicePatch{IssuedOn: new("2026-03-01")})
	if patched.Status != "draft" || patched.Number != "" || patched.DueOn != "2026-03-15" {
		t.Fatalf("patched: %+v", patched.InvoiceSummary)
	}
	dup := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, d2.ID, "/duplicate"), nil)
	if dup.Status != "draft" || dup.Number != "" {
		t.Fatalf("duplicate: %+v", dup.InvoiceSummary)
	}
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, dup.ID, ""), nil)

	// issuing: number of the series, VS from it, a past issue date moves to today
	issued := action(a, d1.ID, "issue")
	if issued.Status != "open" || issued.Number != "2026-0001" || issued.VariableSymbol != "20260001" ||
		issued.IssuedOn != "2026-03-15" || issued.DueOn != "2026-03-29" {
		t.Fatalf("issued: %+v", issued.InvoiceSummary)
	}
	if q := stock(); q != "8" {
		t.Fatalf("stock after issue: %s", q)
	}
	res, body = a.do("POST", invURL(a, d1.ID, "/actions/issue"), nil)
	assertCode(t, res, body, http.StatusConflict, "invalid_transition")
	if got := action(a, d2.ID, "issue"); got.Number != "2026-0002" {
		t.Fatalf("second: %+v", got.InvoiceSummary)
	}
	var ev model.Event
	if err := ts.db.Where("name = ? AND subject_id = ?", "invoice.issued", d1.ID).First(&ev).Error; err != nil || !strings.Contains(ev.Text, "2026-0001") {
		t.Fatalf("event: %+v %v", ev, err)
	}
	var created model.Event
	if err := ts.db.Where("name = ? AND subject_id = ?", "invoice.created", d1.ID).First(&created).Error; err != nil || !strings.Contains(created.Text, "Koncept faktury pro ACME") {
		t.Fatalf("created event: %+v %v", created, err)
	}

	// now it counts and can be paid
	if dash := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard?year=2026"), nil); dash.RevenueTotal != 200000 {
		t.Fatalf("dashboard: %+v", dash)
	}
	if p := pay(a, d1.ID, api.PaymentCreate{}); p.Invoice.Status != "paid" {
		t.Fatalf("pay: %+v", p.Invoice.InvoiceSummary)
	}

	// templates can produce drafts
	tpl := doJSON[api.Template](a, http.StatusCreated, "POST", invURL(a, d2.ID, "/save-as-template"), map[string]any{"name": "Krabice"})
	fromTpl := doJSON[api.Invoice](a, http.StatusCreated, "POST", fmt.Sprintf("%s/%d/create-invoice", a.acct("/templates"), tpl.ID), api.TemplateIssue{Draft: true})
	if fromTpl.Status != "draft" || fromTpl.Number != "" {
		t.Fatalf("from template: %+v", fromTpl.InvoiceSummary)
	}
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, fromTpl.ID, ""), nil)

	// a future issue date is kept; a custom number is kept on issue
	future := createInv(a, api.InvoiceCreate{Draft: true, SubjectID: new(acme.ID), IssuedOn: "2026-04-01", Number: "X-1",
		Lines: []api.InvoiceLineInput{line("Práce", "1", 100, nil)}})
	if future.Number != "X-1" || future.Status != "draft" {
		t.Fatalf("custom draft: %+v", future.InvoiceSummary)
	}
	if got := action(a, future.ID, "issue"); got.Number != "X-1" || got.IssuedOn != "2026-04-01" {
		t.Fatalf("future: %+v", got.InvoiceSummary)
	}
}

// TestDraftNotInVatReport: VAT reports ignore drafts of a VAT payer.
func TestDraftNotInVatReport(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_mode": "vat_payer", "vat_no": "CZ12345678"})
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	d := createInv(a, api.InvoiceCreate{Draft: true, SubjectID: new(acme.ID), IssuedOn: "2026-02-10", Lines: []api.InvoiceLineInput{line("Práce", "1", 100000, i32(2100))}})
	b := a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat?period=2026-02"), nil)
	if strings.Contains(string(b), "100000") || strings.Contains(string(b), "21000") {
		t.Fatalf("VAT report contains the draft: %s", b)
	}
	// issued (dated today, DUZP kept) it is reported in its DUZP period
	action(a, d.ID, "issue")
	if b := a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat?period=2026-02"), nil); !strings.Contains(string(b), "21000") {
		t.Fatalf("VAT report lacks the issued invoice: %s", b)
	}
}
