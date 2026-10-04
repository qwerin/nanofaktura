package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestExpenseDueDaysClearSubjectAttachments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	sup := newSubject(a, api.SubjectCreate{Name: "Dodavatel s.r.o.", Type: "supplier"})
	e := createExp(a, api.ExpenseCreate{SubjectID: &sup.ID, DueDays: intPtr(30),
		Lines: []api.InvoiceLineInput{line("x", "1", 100, nil)}})
	if e.DueOn != "2026-04-14" || len(e.Attachments) != 0 {
		t.Fatalf("due_on %s attachments %v", e.DueOn, e.Attachments)
	}
	att := uploadOK(a, "expense", e.ID, "e.pdf", []byte("%PDF-1.4 test"))
	if got := getExp(a, e.ID); len(got.Attachments) != 1 || got.Attachments[0].ID != att.ID {
		t.Fatalf("attachments %+v", got.Attachments)
	}

	res, body := a.do("PATCH", expURL(a, e.ID, ""), map[string]any{"clear_subject": true, "subject_id": sup.ID})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "validation_failed")
	p := doJSON[api.Expense](a, http.StatusOK, "PATCH", expURL(a, e.ID, ""), map[string]any{"clear_subject": true})
	if p.SubjectID != nil || p.SupplierName != "Dodavatel s.r.o." || len(p.Attachments) != 1 {
		t.Fatalf("cleared %+v", p.ExpenseSummary)
	}

	// invoice detail lists its attachments too
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("A", "1", 100, nil)}})
	uploadOK(a, "invoice", inv.ID, "i.pdf", []byte("%PDF-1.4 test"))
	if got := getInv(a, inv.ID); len(got.Attachments) != 1 {
		t.Fatalf("invoice attachments %+v", got.Attachments)
	}
}

func TestTemplateRecurringDisplayFields(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := newTemplate(a, subj.ID, "Hosting") // 2 × 500 Kč, non VAT payer
	if tpl.SubjectName != "ACME" || tpl.Total != 100000 || tpl.TotalCurrency != "CZK" {
		t.Fatalf("template %+v", tpl)
	}
	rec := newRecurring(a, api.RecurringCreate{Name: "R", TemplateID: tpl.ID})
	if rec.TemplateName != "Hosting" || rec.SubjectID != subj.ID || rec.SubjectName != "ACME" || rec.Total != 100000 {
		t.Fatalf("recurring %+v", rec)
	}
	l := doJSON[api.ListResponse[api.Recurring]](a, http.StatusOK, "GET", a.acct("/recurring"), nil)
	tl := doJSON[api.ListResponse[api.Template]](a, http.StatusOK, "GET", a.acct("/templates"), nil)
	if l.Items[0].SubjectName != "ACME" || tl.Items[0].Total != 100000 {
		t.Fatalf("lists %+v %+v", l.Items, tl.Items)
	}
	// VAT payer: default rate applies to lines without a rate
	setVatPayer(a)
	if got := doJSON[api.Template](a, http.StatusOK, "GET", fmt.Sprintf("%s/%d", a.acct("/templates"), tpl.ID), nil); got.Total != 121000 {
		t.Fatalf("payer total %d", got.Total)
	}
}

func TestSubjectClearDueDays(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	s := newSubject(a, api.SubjectCreate{Name: "ACME", DueDays: intPtr(30)})
	url := fmt.Sprintf("%s/%d", a.acct("/subjects"), s.ID)
	if got := doJSON[api.Subject](a, http.StatusOK, "PATCH", url, map[string]any{"name": "ACME 2"}); got.DueDays == nil || *got.DueDays != 30 {
		t.Fatalf("kept %+v", got.DueDays)
	}
	if got := doJSON[api.Subject](a, http.StatusOK, "PATCH", url, map[string]any{"clear_due_days": true}); got.DueDays != nil {
		t.Fatalf("cleared %+v", *got.DueDays)
	}
}
