package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

// TestInvoiceEndCustomer: an invoice without a contact (end customer) keeps
// the client only as client_* text.
func TestInvoiceEndCustomer(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	lines := []api.InvoiceLineInput{line("Oprava kola", "1", 150000, nil)}

	// neither a contact nor a name → 422 client_name
	res, body := a.do("POST", a.acct("/invoices"), api.InvoiceCreate{Lines: lines})
	assertError(t, res, body, http.StatusUnprocessableEntity, "client_name")
	blank := "  "
	res, body = a.do("POST", a.acct("/invoices"), api.InvoiceCreate{InvoiceSnapshotFields: api.InvoiceSnapshotFields{ClientName: &blank}, Lines: lines})
	assertError(t, res, body, http.StatusUnprocessableEntity, "client_name")

	name, city, email := "Jan Novák", "Brno", "jan@example.cz"
	inv := createInv(a, api.InvoiceCreate{
		InvoiceSnapshotFields: api.InvoiceSnapshotFields{ClientName: &name, ClientCity: &city, ClientEmail: &email}, Lines: lines})
	if inv.SubjectID != nil || inv.ClientName != name || inv.ClientCity != city || inv.Number == "" {
		t.Fatalf("end customer invoice: %+v", inv.InvoiceSummary)
	}
	invPath := fmt.Sprintf("%s/%d", a.acct("/invoices"), inv.ID)

	// the e-mail dialog is prefilled from the invoice
	p := doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?invoice_id="+uintStr(inv.ID)), nil)
	if len(p.To) != 1 || p.To[0] != email || len(p.Cc) != 0 {
		t.Fatalf("preview recipients: %+v %+v", p.To, p.Cc)
	}

	// editing keeps it without a contact; the name cannot be emptied
	note := "Díky"
	got := doJSON[api.Invoice](a, http.StatusOK, "PATCH", invPath, api.InvoicePatch{Note: &note})
	if got.SubjectID != nil || got.ClientName != name {
		t.Fatalf("patched: %+v", got.InvoiceSummary)
	}
	empty := ""
	res, body = a.do("PATCH", invPath, api.InvoicePatch{InvoiceSnapshotFields: api.InvoiceSnapshotFields{ClientName: &empty}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "client_name")

	// duplicate keeps the end customer
	dup := doJSON[api.Invoice](a, http.StatusCreated, "POST", invPath+"/duplicate", nil)
	if dup.SubjectID != nil || dup.ClientName != name || dup.ClientEmail != email {
		t.Fatalf("duplicate: %+v", dup.InvoiceSummary)
	}

	// no template without a contact
	res, body = a.do("POST", invPath+"/save-as-template", nil)
	assertCode(t, res, body, http.StatusConflict, "template_needs_subject")

	// a contact can be assigned later (re-snapshot) and unlinked again
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", City: "Praha"})
	got = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invPath, api.InvoicePatch{SubjectID: &subj.ID})
	if got.SubjectID == nil || *got.SubjectID != subj.ID || got.ClientName != "ACME" || got.ClientCity != "Praha" {
		t.Fatalf("assigned: %+v", got.InvoiceSummary)
	}
	res, body = a.do("PATCH", invPath, api.InvoicePatch{SubjectID: &subj.ID, ClearSubject: true})
	assertError(t, res, body, http.StatusUnprocessableEntity, "clear_subject")
	got = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invPath, api.InvoicePatch{ClearSubject: true})
	if got.SubjectID != nil || got.ClientName != "ACME" {
		t.Fatalf("unlinked: %+v", got.InvoiceSummary)
	}

	// top customers group end customers by name
	ov := doJSON[api.Overview](a, http.StatusOK, "GET", a.acct("/reports/overview?year=2026"), nil)
	if len(ov.TopCustomers) != 2 {
		t.Fatalf("top customers: %+v", ov.TopCustomers)
	}
	for _, c := range ov.TopCustomers {
		if c.SubjectID != nil || (c.Name != name && c.Name != "ACME") {
			t.Fatalf("top customer: %+v", c)
		}
	}
}
