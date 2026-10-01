package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/reports"
)

// validateXSD checks an XML document against a schema of the repo with
// xmllint (skipped when xmllint is not installed).
func validateXSD(t *testing.T, doc []byte, schema string) {
	t.Helper()
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Log("xmllint not installed; XSD validation skipped")
		return
	}
	f := filepath.Join(t.TempDir(), "doc.xml")
	if err := os.WriteFile(f, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("xmllint", "--noout", "--schema", schema, f).CombinedOutput(); err != nil {
		t.Fatalf("XSD validation failed: %v\n%s\n%s", err, out, doc)
	}
}

const (
	isdocXSD  = "../isdoc/testdata/isdoc-invoice-6.0.2.xsd"
	dphdp3XSD = "../reports/testdata/dphdp3_epo2.xsd"
	dphkh1XSD = "../reports/testdata/dphkh1_epo2.xsd"
)

func overview(c *client) api.Overview {
	c.ts.t.Helper()
	return doJSON[api.Overview](c, http.StatusOK, "GET", c.acct("/reports/overview?year=2026"), nil)
}

func vatReport(c *client, period string) api.VatReport {
	c.ts.t.Helper()
	return doJSON[api.VatReport](c, http.StatusOK, "GET", c.acct("/reports/vat?period="+period), nil)
}

func hasWarning(r api.VatReport, code, doc string) bool {
	for _, w := range r.Warnings {
		if w.Code == code && (doc == "" || w.Document == doc) {
			return true
		}
	}
	return false
}

// Tax audit H-03 + money C-02/M-11: a VAT payer's proforma payment gets a
// tax document (taxed at the payment date), the final invoice issued at the
// delivery takes the payments over, deducts the taxed advance and settles
// the proforma, so nothing is double counted.
func TestProformaTaxDocumentsAndFinalInvoice(t *testing.T) {
	ts := newTestServer(t) // today 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME", VatNo: "CZ27074358"})
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: acme.ID, IssuedOn: "2026-03-01",
		Lines: []api.InvoiceLineInput{line("Dílo", "1", 100_000, i32(2100))}}) // 1 210,00

	r := pay(a, pro.ID, api.PaymentCreate{PaidOn: "2026-03-10", Amount: i64(40_000)})
	if r.TaxDocumentID == nil || r.Payment.TaxDocumentID == nil || *r.Payment.TaxDocumentID != *r.TaxDocumentID {
		t.Fatalf("no tax document: %+v", r)
	}
	td := getInv(a, *r.TaxDocumentID)
	if td.DocumentType != "tax_document" || td.Number != "ZD2026-0001" || td.IssuedOn != "2026-03-10" ||
		td.TaxableFulfillmentDue != "2026-03-10" || td.Total != 40_000 || td.Subtotal != 33_058 || td.VatTotal != 6_942 ||
		td.Status != "paid" || td.RelatedID == nil || *td.RelatedID != pro.ID || len(td.Payments) != 1 ||
		td.Payments[0].SourcePaymentID == nil || *td.Payments[0].SourcePaymentID != r.Payment.ID {
		t.Fatalf("tax document: %+v payments %+v", td.InvoiceSummary, td.Payments)
	}
	if p := getInv(a, pro.ID); p.Status != "open" || p.RemainingAmount != 81_000 || len(p.RelatedDocuments) != 1 {
		t.Fatalf("proforma after partial payment: %+v %+v", p.InvoiceSummary, p.RelatedDocuments)
	}
	if d := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard"), nil); d.UnpaidTotal != 81_000 {
		t.Fatalf("unpaid %d", d.UnpaidTotal)
	}
	v := vatReport(a, "2026-03")
	if v.Return.R1 != (reports.Pair{Base: 33_058, Vat: 6_942}) || len(v.Control.A5.Basic.String()) == 0 {
		t.Fatalf("advance VAT: %+v", v.Return.R1)
	}
	if o := overview(a); o.IncomeTax.Income != 33_058 || o.RevenueTotal != 0 {
		t.Fatalf("income of the advance: %+v revenue %d", o.IncomeTax, o.RevenueTotal)
	}

	// the tax document follows its payment
	res, body := a.do("DELETE", fmt.Sprintf("%s/%d", invURL(a, td.ID, "/payments"), td.Payments[0].ID), nil)
	assertCode(t, res, body, http.StatusConflict, "advance_payment")
	res, body = a.do("DELETE", invURL(a, td.ID, ""), nil)
	assertCode(t, res, body, http.StatusConflict, "tax_document_fixed")
	res, body = a.do("PATCH", invURL(a, td.ID, ""), map[string]any{"lines": []map[string]any{{"name": "x", "unit_price": 1}}})
	assertCode(t, res, body, http.StatusConflict, "tax_document_fixed")
	a.mustDo(http.StatusOK, "PATCH", invURL(a, td.ID, ""), map[string]any{"private_note": "ok"})
	for _, op := range []string{"/duplicate", "/save-as-template"} {
		res, body = a.do("POST", invURL(a, td.ID, op), nil)
		assertCode(t, res, body, http.StatusConflict, "tax_document_fixed")
	}

	// final invoice at the delivery (April): full lines, the advance deducted
	ts.now = day(2026, 4, 2)
	fin := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, pro.ID, "/final-invoice"),
		api.FinalInvoiceCreate{IssuedOn: "2026-04-02", TaxableFulfillmentDue: "2026-03-31"})
	if fin.DocumentType != "invoice" || fin.Total != 121_000 || fin.PaidAmount != 40_000 || fin.RemainingAmount != 81_000 ||
		fin.Status != "open" || len(fin.Payments) != 1 || fin.Payments[0].SourcePaymentID == nil ||
		len(fin.Deposits) != 1 || fin.Deposits[0].Number != "ZD2026-0001" || fin.Deposits[0].VatRecap[0].Vat != 6_942 {
		t.Fatalf("final invoice: %+v payments %+v deposits %+v", fin.InvoiceSummary, fin.Payments, fin.Deposits)
	}
	if p := getInv(a, pro.ID); p.Status != "paid" {
		t.Fatalf("proforma not settled: %+v", p.InvoiceSummary)
	}
	if d := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard"), nil); d.UnpaidTotal != 81_000 || d.UnpaidCount != 1 {
		t.Fatalf("receivable double counted: %+v", d)
	}
	res, body = a.do("POST", invURL(a, pro.ID, "/payments"), api.PaymentCreate{Amount: i64(81_000)})
	assertCode(t, res, body, http.StatusConflict, "proforma_settled")
	res, body = a.do("PATCH", invURL(a, pro.ID, ""), map[string]any{"note": "x"})
	assertCode(t, res, body, http.StatusConflict, "proforma_settled")
	res, body = a.do("POST", invURL(a, pro.ID, "/final-invoice"), nil)
	assertCode(t, res, body, http.StatusConflict, "final_exists")

	// VAT: March has the final invoice (DUZP 31. 3.) net of the advance → 1 000,00 / 210,00 in total
	v = vatReport(a, "2026-03")
	if v.Return.R1 != (reports.Pair{Base: 100_000, Vat: 21_000}) {
		t.Fatalf("VAT with the deposit deducted: %+v", v.Return.R1)
	}
	for _, f := range []string{"dphdp3", "dphkh1"} {
		a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"c_ufo": "451"})
		xmlDoc := a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/"+f+".xml?period=2026-03"), nil)
		validateXSD(t, xmlDoc, map[string]string{"dphdp3": dphdp3XSD, "dphkh1": dphkh1XSD}[f])
	}
	isd := a.mustDo(http.StatusOK, "GET", invURL(a, fin.ID, "/isdoc"), nil)
	for _, want := range []string{"<TaxedDeposits>", "<ID>ZD2026-0001</ID>", "<AlreadyClaimedTaxAmount>69.42</AlreadyClaimedTaxAmount>",
		"<PayableAmount>810.00</PayableAmount>"} {
		if !bytes.Contains(isd, []byte(want)) {
			t.Errorf("final ISDOC lacks %s", want)
		}
	}
	validateXSD(t, isd, isdocXSD)
	isd = a.mustDo(http.StatusOK, "GET", invURL(a, td.ID, "/isdoc"), nil)
	if !bytes.Contains(isd, []byte("<DocumentType>5</DocumentType>")) || !bytes.Contains(isd, []byte("<TaxPointDate>2026-03-10</TaxPointDate>")) {
		t.Errorf("tax document ISDOC:\n%s", isd)
	}
	validateXSD(t, isd, isdocXSD)
	for _, id := range []uint{fin.ID, td.ID} {
		if res, _ := a.do("GET", invURL(a, id, "/pdf"), nil); res.StatusCode != http.StatusOK {
			t.Fatalf("pdf %d: %d", id, res.StatusCode)
		}
	}

	// the rest is paid on the final invoice; income = the whole price without VAT
	rest := pay(a, fin.ID, api.PaymentCreate{PaidOn: "2026-04-02"})
	if rest.Invoice.Status != "paid" || rest.Payment.Amount != 81_000 {
		t.Fatalf("rest: %+v", rest.Invoice.InvoiceSummary)
	}
	if o := overview(a); o.IncomeTax.Income != 100_000 {
		t.Fatalf("income: %+v", o.IncomeTax)
	}

	// deleting: the final invoice needs its own payments gone first, then it reopens the proforma
	res, body = a.do("DELETE", invURL(a, fin.ID, ""), nil)
	assertCode(t, res, body, http.StatusConflict, "has_payments")
	res, body = a.do("DELETE", fmt.Sprintf("%s/%d", invURL(a, pro.ID, "/payments"), r.Payment.ID), nil)
	assertCode(t, res, body, http.StatusConflict, "proforma_settled")
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", invURL(a, fin.ID, "/payments"), rest.Payment.ID), nil)
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, fin.ID, ""), nil)
	if p := getInv(a, pro.ID); p.Status != "overdue" || p.RemainingAmount != 81_000 { // reopened (due 15. 3.)
		t.Fatalf("proforma after deleting the final invoice: %+v", p.InvoiceSummary)
	}
	// deleting the proforma payment deletes its tax document (money M-11)
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", invURL(a, pro.ID, "/payments"), r.Payment.ID), nil)
	res, body = a.do("GET", invURL(a, td.ID, ""), nil)
	assertCode(t, res, body, http.StatusNotFound, "not_found")
	if o := overview(a); o.IncomeTax.Income != 0 {
		t.Fatalf("income after deleting everything: %+v", o.IncomeTax)
	}
}

// Money C-02: a partial payment with create_final_invoice settles the
// proforma; the rest is due once (on the final invoice) and the income is
// complete once it is paid.
func TestProformaPartialPaymentWithFinalInvoice(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "1", 121_000, nil)}})
	r := pay(a, pro.ID, api.PaymentCreate{Amount: i64(40_000), CreateFinalInvoice: true})
	if r.TaxDocumentID != nil { // a non-payer issues no tax documents
		t.Fatalf("tax document of a non-payer: %+v", r)
	}
	fin := getInv(a, *r.FinalInvoiceID)
	if fin.Total != 121_000 || fin.RemainingAmount != 81_000 || fin.Status != "open" || r.Invoice.Status != "paid" {
		t.Fatalf("final %+v proforma %+v", fin.InvoiceSummary, r.Invoice.InvoiceSummary)
	}
	if d := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard"), nil); d.UnpaidTotal != 81_000 {
		t.Fatalf("unpaid %d, want 81000 (not 2 × 81000)", d.UnpaidTotal)
	}
	res, body := a.do("POST", invURL(a, pro.ID, "/payments"), api.PaymentCreate{})
	assertCode(t, res, body, http.StatusConflict, "proforma_settled")
	pay(a, fin.ID, api.PaymentCreate{})
	if o := overview(a); o.IncomeTax.Income != 121_000 {
		t.Fatalf("income %+v", o.IncomeTax)
	}
	if l := listInv(a, "?status=unpaid"); l.Total != 0 {
		t.Fatalf("unpaid list: %+v", l)
	}
}

// Money M-13: a proforma paid from the bank (auto-match) takes the same
// path: tax document for a VAT payer, income counted.
func TestBankMatchedProformaPayment(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	bank := fioBank(a)
	vs := "55"
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: acme.ID, VariableSymbol: &vs,
		Lines: []api.InvoiceLineInput{line("X", "1", 100_000, i32(2100))}})
	res := importOK(a, bank.ID, fioJSON(fioTx{id: "1", date: "2026-03-12", vs: "55", amount: "1210.00", name: "ACME"}))
	if res.Matched != 1 {
		t.Fatalf("import: %+v", res)
	}
	p := getInv(a, pro.ID)
	if p.Status != "paid" || len(p.Payments) != 1 || p.Payments[0].TaxDocumentID == nil {
		t.Fatalf("proforma: %+v %+v", p.InvoiceSummary, p.Payments)
	}
	if td := getInv(a, *p.Payments[0].TaxDocumentID); td.TaxableFulfillmentDue != "2026-03-12" || td.VatTotal != 21_000 {
		t.Fatalf("tax document: %+v", td.InvoiceSummary)
	}
	if o := overview(a); o.IncomeTax.Income != 100_000 {
		t.Fatalf("income %+v", o.IncomeTax)
	}
	if v := vatReport(a, "2026-03"); v.Return.R1.Vat != 21_000 {
		t.Fatalf("VAT %+v", v.Return.R1)
	}
	// unmatching removes the payment and its tax document
	var txs api.ListResponse[api.BankTransaction]
	txs = doJSON[api.ListResponse[api.BankTransaction]](a, http.StatusOK, "GET", a.acct("/bank-transactions"), nil)
	a.mustDo(http.StatusOK, "POST", fmt.Sprintf("%s/%d/unmatch", a.acct("/bank-transactions"), txs.Items[0].ID), nil)
	var n int64
	ts.db.Model(&model.Invoice{}).Where("document_type = ?", model.DocTaxDocument).Count(&n)
	if n != 0 {
		t.Fatalf("tax documents after unmatch: %d", n)
	}
}

// Money M-02/M-03/M-06: currency change with payments, non-positive rates
// and payments of the wrong sign are rejected.
func TestPaymentAndRateGuards(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "1", 121_000, nil)}})
	pay(a, inv.ID, api.PaymentCreate{})
	res, body := a.do("PATCH", invURL(a, inv.ID, ""), map[string]any{"currency": "EUR", "exchange_rate": "25"})
	assertCode(t, res, body, http.StatusConflict, "currency_has_payments")
	exp := createExp(a, api.ExpenseCreate{SubjectID: &acme.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	a.mustDo(http.StatusCreated, "POST", fmt.Sprintf("%s/%d/payments", a.acct("/expenses"), exp.ID), map[string]any{})
	res, body = a.do("PATCH", fmt.Sprintf("%s/%d", a.acct("/expenses"), exp.ID), map[string]any{"currency": "EUR"})
	assertCode(t, res, body, http.StatusConflict, "currency_has_payments")

	for _, rate := range []string{"0", "0.000000"} {
		res, body = a.do("POST", a.acct("/invoices"), api.InvoiceCreate{SubjectID: acme.ID, Currency: "EUR", ExchangeRate: rate,
			Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
		assertError(t, res, body, http.StatusUnprocessableEntity, "exchange_rate")
		res, body = a.do("POST", a.acct("/expenses"), api.ExpenseCreate{SubjectID: &acme.ID, Currency: "EUR", ExchangeRate: rate,
			Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
		assertError(t, res, body, http.StatusUnprocessableEntity, "exchange_rate")
	}
	eur := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Currency: "EUR", ExchangeRate: "25", Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	res, body = a.do("PATCH", invURL(a, eur.ID, ""), map[string]any{"exchange_rate": "0"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "exchange_rate")

	// a credit note is refunded with a negative amount
	cr := createInv(a, api.InvoiceCreate{DocumentType: "correction", RelatedID: &inv.ID, SubjectID: acme.ID,
		Lines: []api.InvoiceLineInput{line("X", "-1", 121_000, nil)}})
	res, body = a.do("POST", invURL(a, cr.ID, "/payments"), api.PaymentCreate{Amount: i64(121_000)})
	assertError(t, res, body, http.StatusUnprocessableEntity, "same sign")
	if p := pay(a, cr.ID, api.PaymentCreate{Amount: i64(-121_000)}); p.Invoice.Status != "paid" {
		t.Fatalf("refund: %+v", p.Invoice.InvoiceSummary)
	}
}

// Money M-05/M-08: overpayments do not net out unpaid sums; credit notes are
// not receivables on the dashboard.
func TestUnpaidSumsIgnoreOverpaymentsAndCreditNotes(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	i1 := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100_000, nil)}})
	createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("Y", "1", 50_000, nil)}})
	pay(a, i1.ID, api.PaymentCreate{Amount: i64(150_000)})
	l := doJSON[api.DocumentList[api.InvoiceSummary]](a, http.StatusOK, "GET", a.acct("/invoices"), nil)
	if len(l.Sums) != 1 || l.Sums[0].SumRemaining != 50_000 {
		t.Fatalf("sums %+v", l.Sums)
	}
	createInv(a, api.InvoiceCreate{DocumentType: "correction", RelatedID: &i1.ID, SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "-1", 10_000, nil)}})
	if d := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard"), nil); d.UnpaidTotal != 50_000 || d.UnpaidCount != 1 {
		t.Fatalf("dashboard %+v", d)
	}
}

// Tax audit C-02/M-06: invoices from a template, a recurring invoice and a
// duplicate in a foreign currency get the ČNB rate of their own date.
func TestForeignCurrencyRateOnGeneratedInvoices(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := doJSON[api.Template](a, http.StatusCreated, "POST", a.acct("/templates"), map[string]any{
		"name": "EUR měsíčně", "subject_id": acme.ID, "currency": "EUR",
		"lines": []map[string]any{{"name": "Služba", "quantity": "1", "unit_price": 100_000}}})
	inv := doJSON[api.Invoice](a, http.StatusCreated, "POST", fmt.Sprintf("%s/%d/create-invoice", a.acct("/templates"), tpl.ID), nil)
	if inv.Currency != "EUR" || inv.ExchangeRate != "24.350" {
		t.Fatalf("template invoice rate %q", inv.ExchangeRate)
	}
	rec := doJSON[api.Recurring](a, http.StatusCreated, "POST", a.acct("/recurring"), api.RecurringCreate{Name: "R", TemplateID: tpl.ID})
	run := doJSON[api.Invoice](a, http.StatusCreated, "POST", fmt.Sprintf("%s/%d/run-now", a.acct("/recurring"), rec.ID), nil)
	if run.ExchangeRate != "24.350" {
		t.Fatalf("recurring invoice rate %q", run.ExchangeRate)
	}
	old := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Currency: "EUR", ExchangeRate: "25.1", IssuedOn: "2026-01-10",
		Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	dup := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, old.ID, "/duplicate"), nil)
	if dup.ExchangeRate != "24.350" {
		t.Fatalf("duplicate rate %q", dup.ExchangeRate)
	}
	// a fixed template rate is used as is
	fixed := doJSON[api.Template](a, http.StatusCreated, "POST", a.acct("/templates"), map[string]any{
		"name": "Pevný kurz", "subject_id": acme.ID, "currency": "EUR", "exchange_rate": "25",
		"lines": []map[string]any{{"name": "Služba", "unit_price": 100}}})
	if inv := doJSON[api.Invoice](a, http.StatusCreated, "POST", fmt.Sprintf("%s/%d/create-invoice", a.acct("/templates"), fixed.ID), nil); inv.ExchangeRate != "25" {
		t.Fatalf("fixed rate %q", inv.ExchangeRate)
	}
}
