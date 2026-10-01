package api_test

import (
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// deleting a document referenced by another one is refused; deleting an
// owner record removes its attachments (rows and files).
func TestDeleteReferencedAndAttachments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("A", "1", 100, nil)}})
	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})

	res, body := a.do("DELETE", invURL(a, inv.ID, ""), nil)
	assertCode(t, res, body, http.StatusConflict, "referenced")
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, corr.ID, ""), nil)

	att := uploadOK(a, "invoice", inv.ID, "a.pdf", []byte("%PDF-1.4 test"))
	uploadOK(a, "subject", subj.ID, "s.pdf", []byte("%PDF-1.4 test"))
	exp := createExp(a, api.ExpenseCreate{SubjectID: &subj.ID, Lines: []api.InvoiceLineInput{line("x", "1", 100, nil)}})
	uploadOK(a, "expense", exp.ID, "e.pdf", []byte("%PDF-1.4 test"))
	if n := countFiles(t, ts.dataDir); n != 3 {
		t.Fatalf("files %d", n)
	}

	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv.ID, ""), nil)
	res, body = a.do("GET", fmt.Sprintf("%s/%d", a.acct("/attachments"), att.ID), nil)
	assertCode(t, res, body, http.StatusNotFound, "not_found")
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", a.acct("/expenses"), exp.ID), nil)
	if n := countFiles(t, ts.dataDir); n != 1 {
		t.Fatalf("files after document deletes %d", n)
	}

	// a subject used by a template cannot be deleted
	tpl := newTemplate(a, subj.ID, "Hosting")
	res, body = a.do("DELETE", fmt.Sprintf("%s/%d", a.acct("/subjects"), subj.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "used_by_template")
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", a.acct("/templates"), tpl.ID), nil)
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", a.acct("/subjects"), subj.ID), nil)
	var left int64
	ts.db.Model(&model.Attachment{}).Count(&left)
	if left != 0 || countFiles(t, ts.dataDir) != 0 {
		t.Fatalf("attachments left %d, files %d", left, countFiles(t, ts.dataDir))
	}
}

// identified persons charge no domestic VAT (rates forced to 0), but keep
// the rates on reverse-charge supplies.
func TestInvoiceIdentifiedPerson(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_mode": "identified_person", "vat_no": "CZ12345678"})
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})

	dom := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("A", "1", 10000, i32(2100))}})
	if dom.YourVatMode != "identified_person" || dom.VatTotal != 0 || dom.Total != 10000 || dom.Lines[0].VatRateBps != 0 ||
		len(dom.VatRecap) != 1 || dom.VatRecap[0].VatRateBps != 0 {
		t.Fatalf("domestic: %+v %+v", dom.InvoiceSummary, dom.VatRecap)
	}
	rc := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, ReverseCharge: true,
		Lines: []api.InvoiceLineInput{line("A", "1", 10000, i32(2100))}})
	if rc.VatTotal != 0 || rc.Total != 10000 || rc.Lines[0].VatRateBps != 2100 {
		t.Fatalf("reverse charge: %+v %+v", rc.InvoiceSummary, rc.Lines)
	}
	// switching off reverse charge on PATCH forces the rate to 0 again
	p := doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, rc.ID, ""), map[string]any{"reverse_charge": false})
	if p.Lines[0].VatRateBps != 0 || p.VatTotal != 0 {
		t.Fatalf("patched: %+v", p.Lines)
	}
}

// an invoice in a currency without a bank account has no payment details and
// warns about it.
func TestInvoiceNoBankAccountWarning(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	a.mustDo(http.StatusCreated, "POST", a.acct("/bank-accounts"), api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})

	czk := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("A", "1", 100, nil)}})
	if len(czk.Warnings) != 0 || czk.BankAccount == "" {
		t.Fatalf("czk: %+v %+v", czk.Warnings, czk.InvoiceSummary)
	}
	eur := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", ExchangeRate: "25",
		Lines: []api.InvoiceLineInput{line("A", "1", 100, nil)}})
	if len(eur.Warnings) != 1 || eur.Warnings[0].Code != "no_bank_account" || eur.BankAccount != "" || eur.IBAN != "" {
		t.Fatalf("eur: %+v %+v", eur.Warnings, eur.InvoiceSummary)
	}
	if got := getInv(a, eur.ID); len(got.Warnings) != 1 {
		t.Fatalf("detail warnings: %+v", got.Warnings)
	}
	cash := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", ExchangeRate: "25", PaymentMethod: "cash",
		Lines: []api.InvoiceLineInput{line("A", "1", 100, nil)}})
	if len(cash.Warnings) != 0 {
		t.Fatalf("cash: %+v", cash.Warnings)
	}
}
