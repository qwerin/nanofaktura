package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

// ---- helpers ----

func invURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/invoices"), id, suffix)
}

func i32(v int32) *int32 { return &v }
func intPtr(v int) *int  { return &v }

func newSubject(c *client, body api.SubjectCreate) api.Subject {
	c.ts.t.Helper()
	return doJSON[api.Subject](c, http.StatusCreated, "POST", c.acct("/subjects"), body)
}

func line(name, qty string, price int64, rate *int32) api.InvoiceLineInput {
	return api.InvoiceLineInput{Name: name, Quantity: qty, UnitPrice: price, VatRateBps: rate}
}

func createInv(c *client, body api.InvoiceCreate) api.Invoice {
	c.ts.t.Helper()
	return doJSON[api.Invoice](c, http.StatusCreated, "POST", c.acct("/invoices"), body)
}

func getInv(c *client, id uint) api.Invoice {
	c.ts.t.Helper()
	return doJSON[api.Invoice](c, http.StatusOK, "GET", invURL(c, id, ""), nil)
}

func action(c *client, id uint, act string) api.Invoice {
	c.ts.t.Helper()
	return doJSON[api.Invoice](c, http.StatusOK, "POST", invURL(c, id, "/actions/"+act), nil)
}

func pay(c *client, id uint, body api.PaymentCreate) api.PaymentResult {
	c.ts.t.Helper()
	return doJSON[api.PaymentResult](c, http.StatusCreated, "POST", invURL(c, id, "/payments"), body)
}

func listInv(c *client, query string) api.ListResponse[api.InvoiceSummary] {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.InvoiceSummary]](c, http.StatusOK, "GET", c.acct("/invoices")+query, nil)
}

func numbers(l api.ListResponse[api.InvoiceSummary]) []string {
	out := []string{}
	for _, i := range l.Items {
		out = append(out, i.Number)
	}
	return out
}

func assertNumbers(t *testing.T, l api.ListResponse[api.InvoiceSummary], want ...string) {
	t.Helper()
	if got := numbers(l); fmt.Sprint(got) != fmt.Sprint(want) || l.Total != int64(len(want)) {
		t.Fatalf("numbers %v (total %d), want %v", got, l.Total, want)
	}
}

func i64(v int64) *int64 { return &v }

func setVatPayer(c *client) {
	c.ts.t.Helper()
	c.mustDo(http.StatusOK, "PATCH", c.acct(""), map[string]any{"vat_mode": "vat_payer", "vat_no": "CZ12345678"})
}

// ---- create ----

func TestInvoiceCreateDefaults(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{
		"registration_no": "12345678", "street": "Hlavní 1", "city": "Brno", "zip": "60200",
		"registered_by": "Zapsán v ŽR", "default_note": "Fakturujeme Vám", "default_footer_note": "Děkujeme",
	})
	fio := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	acme := newSubject(a, api.SubjectCreate{Name: "ACME s.r.o.", FullName: "Jan Novák", Street: "Dlouhá 5",
		City: "Praha", Zip: "11000", Email: "acme@example.cz"})

	inv := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{
		line("Práce", "1,5", 100000, i32(2100)), // non VAT payer → rate forced to 0
		{Name: " Materiál ", UnitPrice: 5050, UnitName: "ks"},
	}})
	s := inv.InvoiceSummary
	if s.DocumentType != "invoice" || s.Number != "2026-0001" || s.VariableSymbol != "20260001" || s.Status != "open" ||
		s.SubjectID != acme.ID || s.RelatedID != nil || s.IssuedOn != "2026-03-15" || s.TaxableFulfillmentDue != "" ||
		s.DueDays != 14 || s.DueOn != "2026-03-29" || s.Currency != "CZK" || s.ExchangeRate != "1" ||
		s.Language != "cs" || s.PaymentMethod != "bank" || s.SentAt != nil || s.PaidOn != "" || s.LockedAt != nil {
		t.Fatalf("defaults: %+v", s)
	}
	if s.BankAccountID == nil || *s.BankAccountID != fio.ID || s.BankAccount != "2000145399/2010" ||
		s.IBAN != "CZ9320100000002000145399" || s.SwiftBIC != "FIOBCZPP" {
		t.Fatalf("bank snapshot: %+v", s)
	}
	if s.ClientName != "ACME s.r.o." || s.ClientFullName != "Jan Novák" || s.ClientStreet != "Dlouhá 5" ||
		s.ClientCity != "Praha" || s.ClientZip != "11000" || s.ClientCountry != "CZ" || s.ClientEmail != "acme@example.cz" {
		t.Fatalf("client snapshot: %+v", s)
	}
	if s.YourName != "Firma A" || s.YourRegistrationNo != "12345678" || s.YourStreet != "Hlavní 1" || s.YourCity != "Brno" ||
		s.YourZip != "60200" || s.YourCountry != "CZ" || s.YourRegisteredBy != "Zapsán v ŽR" || s.YourVatMode != "non_vat_payer" {
		t.Fatalf("your snapshot: %+v", s)
	}
	if s.Note != "Fakturujeme Vám" || s.FooterNote != "Děkujeme" || s.Tags == nil || len(s.Tags) != 0 || len(s.PublicToken) < 24 {
		t.Fatalf("texts/token: %+v", s)
	}
	wantLines := []api.InvoiceLine{
		{ID: inv.Lines[0].ID, Position: 1, Name: "Práce", Quantity: "1.5", UnitPrice: 100000, Base: 150000, Total: 150000},
		{ID: inv.Lines[1].ID, Position: 2, Name: "Materiál", Quantity: "1", UnitName: "ks", UnitPrice: 5050, Base: 5050, Total: 5050},
	}
	if !reflect.DeepEqual(inv.Lines, wantLines) {
		t.Fatalf("lines %+v", inv.Lines)
	}
	if s.Subtotal != 155050 || s.VatTotal != 0 || s.Rounding != 0 || s.Total != 155050 || s.PaidAmount != 0 || s.RemainingAmount != 155050 ||
		!reflect.DeepEqual(inv.VatRecap, []api.VatRecapItem{{VatRateBps: 0, Base: 155050, Total: 155050}}) || len(inv.Payments) != 0 {
		t.Fatalf("totals: %+v recap %+v", s, inv.VatRecap)
	}

	// subject due_days wins over the account default; numbers continue
	beta := newSubject(a, api.SubjectCreate{Name: "Beta", DueDays: intPtr(30)})
	inv2 := createInv(a, api.InvoiceCreate{SubjectID: beta.ID, Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	if inv2.Number != "2026-0002" || inv2.DueDays != 30 || inv2.DueOn != "2026-04-14" {
		t.Fatalf("inv2: %+v", inv2.InvoiceSummary)
	}
	// proforma has its own series
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	if pro.Number != "Z2026-0001" || pro.VariableSymbol != "20260001" || pro.DocumentType != "proforma" {
		t.Fatalf("proforma: %+v", pro.InvoiceSummary)
	}
	// custom number does not advance the counter; VS from its digits
	custom := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Number: " X-15 ", Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	if custom.Number != "X-15" || custom.VariableSymbol != "15" {
		t.Fatalf("custom: %+v", custom.InvoiceSummary)
	}
	next := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, VariableSymbol: strPtr("777"), Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	if next.Number != "2026-0003" || next.VariableSymbol != "777" {
		t.Fatalf("next: %+v", next.InvoiceSummary)
	}
	res, body := a.do("POST", a.acct("/invoices"), api.InvoiceCreate{SubjectID: acme.ID, Number: "X-15",
		Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	assertError(t, res, body, http.StatusConflict, "X-15 already exists")
	// the same number in another document type is fine
	createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: acme.ID, Number: "X-15", Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	// a failed create does not consume a number
	if n := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}}); n.Number != "2026-0004" {
		t.Fatalf("after conflict: %s", n.Number)
	}
}

func TestInvoiceCreateVatPayer(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"round_total": true})
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})

	// mixed rates 0/12/21, default rate 21 %, VAT per rate, round_total from the account
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{
		line("A", "1.5", 12345, i32(2100)),
		line("B", "3", 999, i32(1200)),
		line("C", "1", 5000, i32(0)),
		line("D", "2", 333, nil),
	}})
	if inv.TaxableFulfillmentDue != "2026-03-15" || inv.YourVatMode != "vat_payer" || inv.YourVatNo != "CZ12345678" || !inv.RoundTotal {
		t.Fatalf("payer defaults: %+v", inv.InvoiceSummary)
	}
	wantRecap := []api.VatRecapItem{
		{VatRateBps: 2100, Base: 19184, Vat: 4029, Total: 23213},
		{VatRateBps: 1200, Base: 2997, Vat: 360, Total: 3357},
		{VatRateBps: 0, Base: 5000, Vat: 0, Total: 5000},
	}
	if !reflect.DeepEqual(inv.VatRecap, wantRecap) || inv.Subtotal != 27181 || inv.VatTotal != 4389 ||
		inv.Rounding != 30 || inv.Total != 31600 || inv.Lines[3].VatRateBps != 2100 || inv.Lines[0].Vat != 3889 {
		t.Fatalf("mixed: %+v recap %+v lines %+v", inv.InvoiceSummary, inv.VatRecap, inv.Lines)
	}

	// prices including VAT, rounding switched off explicitly
	gross := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, PricesIncludeVat: true, RoundTotal: new(bool),
		Lines: []api.InvoiceLineInput{line("A", "1", 12100, i32(2100)), line("B", "1", 1000, i32(1200))}})
	if gross.Subtotal != 10893 || gross.VatTotal != 2207 || gross.Total != 13100 || gross.Rounding != 0 || gross.RoundTotal {
		t.Fatalf("gross: %+v", gross.InvoiceSummary)
	}

	// reverse charge: no VAT, rates kept
	rc := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, ReverseCharge: true, RoundTotal: new(bool),
		Lines: []api.InvoiceLineInput{line("A", "2", 10000, i32(2100))}})
	if rc.VatTotal != 0 || rc.Total != 20000 || rc.Lines[0].VatRateBps != 2100 ||
		!reflect.DeepEqual(rc.VatRecap, []api.VatRecapItem{{VatRateBps: 2100, Base: 20000, Total: 20000}}) {
		t.Fatalf("reverse charge: %+v %+v", rc.InvoiceSummary, rc.VatRecap)
	}

	// explicit values and snapshot overrides
	ov := createInv(a, api.InvoiceCreate{
		SubjectID: subj.ID, IssuedOn: "2026-02-01", TaxableFulfillmentDue: strPtr("2026-01-31"), DueDays: intPtr(0),
		Currency: "EUR", ExchangeRate: "24.355", Language: "en", PaymentMethod: "custom", CustomPaymentMethod: "Barter",
		OrderNumber: "PO-1", Note: strPtr(""), PrivateNote: "tajné", Tags: []string{" a ", "b", "a", ""},
		InvoiceSnapshotFields: api.InvoiceSnapshotFields{ClientName: strPtr("Jiný název"), YourName: strPtr("Firma A s.r.o.")},
		Lines:                 []api.InvoiceLineInput{line("A", "-1", 100, nil)},
	})
	if ov.IssuedOn != "2026-02-01" || ov.TaxableFulfillmentDue != "2026-01-31" || ov.DueOn != "2026-02-01" ||
		ov.Currency != "EUR" || ov.ExchangeRate != "24.355" || ov.Language != "en" || ov.CustomPaymentMethod != "Barter" ||
		ov.OrderNumber != "PO-1" || ov.PrivateNote != "tajné" || !reflect.DeepEqual(ov.Tags, []string{"a", "b"}) ||
		ov.ClientName != "Jiný název" || ov.YourName != "Firma A s.r.o." || ov.Number != "2026-0004" ||
		ov.BankAccountID != nil || ov.IBAN != "" || ov.Total != -100 {
		t.Fatalf("overrides: %+v", ov.InvoiceSummary)
	}

	// non VAT payer snapshot (overridden) forces rates to 0
	np := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, TaxableFulfillmentDue: strPtr(""),
		InvoiceSnapshotFields: api.InvoiceSnapshotFields{YourVatMode: strPtr("non_vat_payer")},
		Lines:                 []api.InvoiceLineInput{line("A", "1", 1000, i32(2100))}})
	if np.VatTotal != 0 || np.Lines[0].VatRateBps != 0 || np.TaxableFulfillmentDue != "" {
		t.Fatalf("non payer: %+v", np.InvoiceSummary)
	}

	// the default bank account of the invoice currency is used
	eur := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "EUR", Currency: "EUR", IBAN: "DE89370400440532013000"})
	e := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", Lines: []api.InvoiceLineInput{line("A", "1", 1, nil)}})
	if e.BankAccountID == nil || *e.BankAccountID != eur.ID || e.IBAN != "DE89370400440532013000" {
		t.Fatalf("eur bank: %+v", e.InvoiceSummary)
	}
	// explicit bank account (other currency) + explicit IBAN override
	e2 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, BankAccountID: &eur.ID,
		InvoiceSnapshotFields: api.InvoiceSnapshotFields{IBAN: strPtr("X")}, Lines: []api.InvoiceLineInput{line("A", "1", 1, nil)}})
	if *e2.BankAccountID != eur.ID || e2.IBAN != "X" || e2.Currency != "CZK" {
		t.Fatalf("explicit bank: %+v", e2.InvoiceSummary)
	}
}

func TestInvoiceCreateValidation(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	bSubj := newSubject(b, api.SubjectCreate{Name: "B"})
	bBank := doJSON[api.BankAccount](b, http.StatusCreated, "POST", b.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	bInv := createInv(b, api.InvoiceCreate{SubjectID: bSubj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}})
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}})
	ok := []api.InvoiceLineInput{line("X", "1", 100, nil)}

	tests := []struct {
		name string
		body any
		want string
	}{
		{"no lines", map[string]any{"subject_id": subj.ID}, "lines"},
		{"empty lines", map[string]any{"subject_id": subj.ID, "lines": []any{}}, "lines"},
		{"no subject", map[string]any{"lines": ok}, "subject_id"},
		{"blank line name", api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line(" ", "1", 1, nil)}}, "lines[0].name"},
		{"bad quantity", api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil), line("Y", "1.2345", 1, nil)}}, "lines[1].quantity"},
		{"rate too high", api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, i32(20000))}}, "vat_rate_bps"},
		{"overflow", api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "999999999999", 100000000000000, nil)}}, "out of range"},
		{"unknown subject", api.InvoiceCreate{SubjectID: 9999, Lines: ok}, "subject not found"},
		{"foreign subject", api.InvoiceCreate{SubjectID: bSubj.ID, Lines: ok}, "subject not found"},
		{"correction without related", api.InvoiceCreate{DocumentType: "correction", SubjectID: subj.ID, Lines: ok}, "requires related_id"},
		{"correction of foreign invoice", api.InvoiceCreate{DocumentType: "correction", SubjectID: subj.ID, RelatedID: &bInv.ID, Lines: ok}, "related document not found"},
		{"correction of proforma", api.InvoiceCreate{DocumentType: "correction", SubjectID: subj.ID, RelatedID: &pro.ID, Lines: ok}, "must relate to an invoice"},
		{"bad issued_on", api.InvoiceCreate{SubjectID: subj.ID, IssuedOn: "2026-02-30", Lines: ok}, "issued_on"},
		{"bad duzp", api.InvoiceCreate{SubjectID: subj.ID, TaxableFulfillmentDue: strPtr("brzy"), Lines: ok}, "taxable_fulfillment_due"},
		{"foreign bank account", api.InvoiceCreate{SubjectID: subj.ID, BankAccountID: &bBank.ID, Lines: ok}, "bank account not found"},
		{"bad currency", api.InvoiceCreate{SubjectID: subj.ID, Currency: "czk", Lines: ok}, "currency"},
		{"bad variable symbol", api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("12345678901"), Lines: ok}, "variable_symbol"},
		{"bad document type", map[string]any{"subject_id": subj.ID, "document_type": "receipt", "lines": ok}, "document_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, body := a.do("POST", a.acct("/invoices"), tt.body)
			assertError(t, res, body, http.StatusUnprocessableEntity, tt.want)
		})
	}

	// a correction of an own invoice is fine
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: ok})
	corr := createInv(a, api.InvoiceCreate{DocumentType: "correction", SubjectID: subj.ID, RelatedID: &inv.ID, Lines: ok})
	if corr.Number != "D2026-0001" || corr.RelatedID == nil || *corr.RelatedID != inv.ID {
		t.Fatalf("correction: %+v", corr.InvoiceSummary)
	}

	// without a default number format → 409
	ts.db.Where("document_type = ?", "invoice").Delete(&model.NumberFormat{})
	res, body := a.do("POST", a.acct("/invoices"), api.InvoiceCreate{SubjectID: subj.ID, Lines: ok})
	assertError(t, res, body, http.StatusConflict, "number format")
	createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Number: "RUČNĚ-1", Lines: ok}) // custom number needs no format
}

// ---- list ----

func TestInvoiceList(t *testing.T) {
	ts := newTestServer(t) // today = 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	beta := newSubject(a, api.SubjectCreate{Name: "Beta a.s."})
	mk := func(subj uint, issued string, price int64) api.Invoice {
		return createInv(a, api.InvoiceCreate{SubjectID: subj, IssuedOn: issued, Lines: []api.InvoiceLineInput{line("X", "1", price, nil)}})
	}
	i1 := mk(acme.ID, "2026-01-10", 500) // due 01-24 → overdue
	i2 := mk(acme.ID, "2026-03-10", 300) // due 03-24 → open
	i3 := mk(acme.ID, "2026-02-01", 100) // due 02-15, sent → overdue
	action(a, i3.ID, "mark_as_sent")
	i4 := mk(beta.ID, "2026-03-01", 900) // paid
	pay(a, i4.ID, api.PaymentCreate{})
	i5 := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: acme.ID, IssuedOn: "2026-03-12",
		Lines: []api.InvoiceLineInput{line("X", "1", 200, nil)}})
	i6 := mk(acme.ID, "2026-03-05", 400) // cancelled
	action(a, i6.ID, "cancel")
	_ = i2

	all := listInv(a, "")
	assertNumbers(t, all, "Z2026-0001", "2026-0002", "2026-0005", "2026-0004", "2026-0003", "2026-0001")
	if strings.Contains(string(a.mustDo(http.StatusOK, "GET", a.acct("/invoices"), nil)), `"lines"`) {
		t.Fatal("list items must not contain lines")
	}
	if all.Items[5].Status != "overdue" || all.Items[4].Status != "overdue" || all.Items[3].Status != "paid" || all.Items[2].Status != "cancelled" {
		t.Fatalf("statuses: %+v", all.Items)
	}

	assertNumbers(t, listInv(a, "?status=overdue"), "2026-0003", "2026-0001")
	assertNumbers(t, listInv(a, "?status=open"), "Z2026-0001", "2026-0002")
	assertNumbers(t, listInv(a, "?status=sent"))
	assertNumbers(t, listInv(a, "?status=paid"), "2026-0004")
	assertNumbers(t, listInv(a, "?status=cancelled"), "2026-0005")
	assertNumbers(t, listInv(a, "?document_type=proforma"), "Z2026-0001")
	assertNumbers(t, listInv(a, fmt.Sprintf("?subject_id=%d", beta.ID)), "2026-0004")
	assertNumbers(t, listInv(a, "?since=2026-03-01&until=2026-03-10"), "2026-0002", "2026-0005", "2026-0004")
	assertNumbers(t, listInv(a, "?query=z2026"), "Z2026-0001")
	assertNumbers(t, listInv(a, "?query=BETA"), "2026-0004")
	assertNumbers(t, listInv(a, "?query=20260003"), "2026-0003")
	assertNumbers(t, listInv(a, "?query=%25"))   // % is literal
	assertNumbers(t, listInv(a, "?query=2026_")) // _ is literal
	assertNumbers(t, listInv(a, "?sort=issued_on"), "2026-0001", "2026-0003", "2026-0004", "2026-0005", "2026-0002", "Z2026-0001")
	assertNumbers(t, listInv(a, "?sort=-total"), "2026-0004", "2026-0001", "2026-0005", "2026-0002", "Z2026-0001", "2026-0003")
	assertNumbers(t, listInv(a, "?sort=due_on"), "2026-0001", "2026-0003", "2026-0004", "2026-0005", "2026-0002", "Z2026-0001")
	assertNumbers(t, listInv(a, "?sort=-number"), "Z2026-0001", "2026-0005", "2026-0004", "2026-0003", "2026-0002", "2026-0001")
	assertNumbers(t, listInv(a, "?status=open&document_type=invoice&subject_id="+fmt.Sprint(acme.ID)), "2026-0002")

	page := listInv(a, "?per_page=2&page=3")
	if page.Total != 6 || len(page.Items) != 2 || page.Items[0].ID != i3.ID || page.Items[1].ID != i1.ID {
		t.Fatalf("page 3: %+v", page)
	}
	for _, q := range []string{"?status=late", "?sort=name", "?since=03-2026", "?document_type=x"} {
		res, body := a.do("GET", a.acct("/invoices")+q, nil)
		assertError(t, res, body, http.StatusUnprocessableEntity, "validation")
	}

	// moving the clock makes the open invoice overdue too
	ts.now = time.Date(2026, 3, 27, 8, 0, 0, 0, time.UTC)
	assertNumbers(t, listInv(a, "?status=overdue"), "Z2026-0001", "2026-0002", "2026-0003", "2026-0001")
	if getInv(a, i5.ID).Status != "overdue" || getInv(a, i4.ID).Status != "paid" {
		t.Fatal("effective status on detail")
	}
}

// ---- get / patch / delete ----

func TestInvoicePatch(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME", City: "Praha"})
	beta := newSubject(a, api.SubjectCreate{Name: "Beta a.s.", City: "Brno"})
	doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"), api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{
		line("L1", "1", 100, nil), line("L2", "1", 200, nil), line("L3", "1", 300, nil),
	}})
	other := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("O", "1", 1, nil)}})
	if got := getInv(a, inv.ID); !reflect.DeepEqual(got, inv) {
		t.Fatalf("get differs from create:\n%+v\n%+v", got, inv)
	}
	l1, l2, l3 := inv.Lines[0].ID, inv.Lines[1].ID, inv.Lines[2].ID

	// full replacement of lines: reorder, update, insert, delete
	p := doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"lines": []any{
		map[string]any{"id": l2, "name": "L2b", "quantity": "2", "unit_price": 250},
		map[string]any{"name": "New", "unit_price": 50},
		map[string]any{"id": l1, "name": "L1", "unit_price": 100},
	}})
	if len(p.Lines) != 3 || p.Lines[0].ID != l2 || p.Lines[0].Name != "L2b" || p.Lines[0].Total != 500 || p.Lines[0].Position != 1 ||
		p.Lines[1].Name != "New" || p.Lines[1].Position != 2 || p.Lines[2].ID != l1 || p.Lines[2].Position != 3 || p.Total != 650 {
		t.Fatalf("lines patch: %+v total %d", p.Lines, p.Total)
	}
	var n int64
	ts.db.Model(&model.InvoiceLine{}).Where("id = ?", l3).Count(&n)
	if n != 0 {
		t.Fatal("removed line still stored")
	}
	if p.Number != inv.Number || p.ClientName != "ACME" || p.Note != inv.Note {
		t.Fatalf("untouched fields changed: %+v", p.InvoiceSummary)
	}

	// dates: issued_on and due_days recompute due_on
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"issued_on": "2026-03-20"})
	if p.DueOn != "2026-04-03" {
		t.Fatalf("due_on: %s", p.DueOn)
	}
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"due_days": 30, "round_total": true})
	if p.DueOn != "2026-04-19" || p.Total != 700 || p.Rounding != 50 {
		t.Fatalf("due_days/round: %+v", p.InvoiceSummary)
	}

	// subject change re-snapshots client_*, explicit client fields win
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"subject_id": beta.ID})
	if p.SubjectID != beta.ID || p.ClientName != "Beta a.s." || p.ClientCity != "Brno" {
		t.Fatalf("resnapshot: %+v", p.InvoiceSummary)
	}
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"subject_id": acme.ID, "client_name": "Vlastní"})
	if p.ClientName != "Vlastní" || p.ClientCity != "Praha" {
		t.Fatalf("resnapshot with override: %+v", p.InvoiceSummary)
	}
	// tags, other fields; currency change re-snapshots the bank account (none in EUR)
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{
		"tags": []string{"a", " a ", "b"}, "currency": "EUR", "note": "", "variable_symbol": "42", "number": "2026-0099",
	})
	if !reflect.DeepEqual(p.Tags, []string{"a", "b"}) || p.Currency != "EUR" || p.BankAccountID != nil || p.IBAN != "" ||
		p.Note != "" || p.VariableSymbol != "42" || p.Number != "2026-0099" {
		t.Fatalf("misc patch: %+v", p.InvoiceSummary)
	}
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"tags": []string{}})
	if p.Tags == nil || len(p.Tags) != 0 {
		t.Fatalf("clear tags: %+v", p.Tags)
	}

	// errors
	for _, tt := range []struct {
		body   map[string]any
		status int
		want   string
	}{
		{map[string]any{"number": other.Number}, http.StatusConflict, "already exists"},
		{map[string]any{"number": " "}, http.StatusUnprocessableEntity, "number"},
		{map[string]any{"lines": []any{map[string]any{"id": other.Lines[0].ID, "name": "X", "unit_price": 1}}}, http.StatusUnprocessableEntity, "lines[0].id"},
		{map[string]any{"lines": []any{map[string]any{"id": l1, "name": "X", "unit_price": 1}, map[string]any{"id": l1, "name": "Y", "unit_price": 1}}}, http.StatusUnprocessableEntity, "lines[1].id"},
		{map[string]any{"lines": []any{}}, http.StatusUnprocessableEntity, "lines"},
		{map[string]any{"subject_id": 9999}, http.StatusUnprocessableEntity, "subject not found"},
		{map[string]any{"related_id": inv.ID}, http.StatusUnprocessableEntity, "itself"},
		{map[string]any{"issued_on": "2026-13-01"}, http.StatusUnprocessableEntity, "issued_on"},
	} {
		res, body := a.do("PATCH", invURL(a, inv.ID, ""), tt.body)
		assertError(t, res, body, tt.status, tt.want)
	}
	if got := getInv(a, inv.ID); got.Number != "2026-0099" || len(got.Lines) != 3 {
		t.Fatalf("failed patches must not change anything: %+v", got.InvoiceSummary)
	}

	// changing the total of a paid invoice reopens it
	paid := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	pay(a, paid.ID, api.PaymentCreate{})
	p = doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, paid.ID, ""), map[string]any{"lines": []any{map[string]any{"name": "X", "unit_price": 150}}})
	if p.Status != "open" || p.PaidOn != "" || p.RemainingAmount != 50 {
		t.Fatalf("reopened: %+v", p.InvoiceSummary)
	}

	// locked / cancelled / uncollectible cannot be edited
	action(a, inv.ID, "lock")
	res, body := a.do("PATCH", invURL(a, inv.ID, ""), map[string]any{"note": "x"})
	assertError(t, res, body, http.StatusConflict, "locked")
	action(a, inv.ID, "unlock")
	action(a, inv.ID, "cancel")
	res, body = a.do("PATCH", invURL(a, inv.ID, ""), map[string]any{"note": "x"})
	assertError(t, res, body, http.StatusConflict, "cancelled invoice cannot be edited")
	action(a, other.ID, "mark_as_uncollectible")
	res, body = a.do("PATCH", invURL(a, other.ID, ""), map[string]any{"note": "x"})
	assertError(t, res, body, http.StatusConflict, "uncollectible")

	res, body = a.do("GET", invURL(a, 9999, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

func TestInvoiceDelete(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	mk := func() api.Invoice {
		return createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	}
	inv := mk()
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv.ID, ""), nil)
	res, body := a.do("GET", invURL(a, inv.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
	var n int64
	ts.db.Model(&model.InvoiceLine{}).Where("invoice_id = ?", inv.ID).Count(&n)
	if n != 0 {
		t.Fatal("lines not deleted")
	}
	res, body = a.do("DELETE", invURL(a, inv.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")

	locked := mk()
	action(a, locked.ID, "lock")
	res, body = a.do("DELETE", invURL(a, locked.ID, ""), nil)
	assertError(t, res, body, http.StatusConflict, "locked")

	paid := mk()
	pay(a, paid.ID, api.PaymentCreate{Amount: i64(10)})
	res, body = a.do("DELETE", invURL(a, paid.ID, ""), nil)
	assertError(t, res, body, http.StatusConflict, "payments")

	// cancelled invoices without payments can be deleted
	cancelled := mk()
	action(a, cancelled.ID, "cancel")
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, cancelled.ID, ""), nil)
}

// ---- actions ----

func TestInvoiceActions(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})

	s := action(a, inv.ID, "mark_as_sent")
	if s.Status != "sent" || s.SentAt == nil || !s.SentAt.Equal(ts.now) {
		t.Fatalf("sent: %+v", s.InvoiceSummary)
	}
	res, body := a.do("POST", invURL(a, inv.ID, "/actions/mark_as_sent"), nil)
	assertError(t, res, body, http.StatusConflict, "only an open document can be marked as sent")

	if s = action(a, inv.ID, "cancel"); s.Status != "cancelled" || s.CancelledAt == nil {
		t.Fatalf("cancel: %+v", s.InvoiceSummary)
	}
	res, body = a.do("POST", invURL(a, inv.ID, "/actions/mark_as_uncollectible"), nil)
	assertError(t, res, body, http.StatusConflict, "current status: cancelled")
	if s = action(a, inv.ID, "undo_cancel"); s.Status != "sent" || s.CancelledAt != nil {
		t.Fatalf("undo cancel: %+v", s.InvoiceSummary)
	}
	if s = action(a, inv.ID, "mark_as_uncollectible"); s.Status != "uncollectible" || s.UncollectibleAt == nil {
		t.Fatalf("uncollectible: %+v", s.InvoiceSummary)
	}
	res, body = a.do("POST", invURL(a, inv.ID, "/actions/undo_cancel"), nil)
	assertError(t, res, body, http.StatusConflict, "not cancelled")
	if s = action(a, inv.ID, "undo_uncollectible"); s.Status != "sent" || s.UncollectibleAt != nil {
		t.Fatalf("undo uncollectible: %+v", s.InvoiceSummary)
	}
	if s = action(a, inv.ID, "lock"); s.LockedAt == nil || s.Status != "sent" {
		t.Fatalf("lock: %+v", s.InvoiceSummary)
	}
	if s = action(a, inv.ID, "unlock"); s.LockedAt != nil {
		t.Fatalf("unlock: %+v", s.InvoiceSummary)
	}

	// open invoice: undo_cancel goes back to open
	inv2 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	action(a, inv2.ID, "cancel")
	if s = action(a, inv2.ID, "undo_cancel"); s.Status != "open" {
		t.Fatalf("undo cancel open: %+v", s.InvoiceSummary)
	}
	// with payments it cannot be cancelled; paid cannot be sent
	pay(a, inv2.ID, api.PaymentCreate{Amount: i64(10)})
	res, body = a.do("POST", invURL(a, inv2.ID, "/actions/cancel"), nil)
	assertError(t, res, body, http.StatusConflict, "payments")
	pay(a, inv2.ID, api.PaymentCreate{})
	res, body = a.do("POST", invURL(a, inv2.ID, "/actions/mark_as_sent"), nil)
	assertError(t, res, body, http.StatusConflict, "current status: paid")

	res, body = a.do("POST", invURL(a, inv.ID, "/actions/explode"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "action")
	res, body = a.do("POST", invURL(a, 9999, "/actions/lock"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

// ---- payments ----

func TestPayments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 10000, nil)}})

	r := pay(a, inv.ID, api.PaymentCreate{Amount: i64(4000), PaidOn: "2026-03-16", Note: "záloha"})
	if r.Payment.Amount != 4000 || r.Payment.PaidOn != "2026-03-16" || r.Payment.Note != "záloha" || r.Payment.InvoiceID != inv.ID ||
		r.Invoice.Status != "open" || r.Invoice.PaidAmount != 4000 || r.Invoice.RemainingAmount != 6000 || r.FinalInvoiceID != nil {
		t.Fatalf("partial: %+v", r)
	}
	first := r.Payment.ID
	r = pay(a, inv.ID, api.PaymentCreate{}) // default: remaining amount, today
	if r.Payment.Amount != 6000 || r.Payment.PaidOn != "2026-03-15" || r.Invoice.Status != "paid" ||
		r.Invoice.PaidOn != "2026-03-16" || r.Invoice.RemainingAmount != 0 || len(r.Invoice.Payments) != 2 ||
		r.Invoice.Payments[0].PaidOn != "2026-03-15" {
		t.Fatalf("full: %+v", r)
	}
	res, body := a.do("POST", invURL(a, inv.ID, "/payments"), api.PaymentCreate{})
	assertError(t, res, body, http.StatusConflict, "nothing to pay")
	res, body = a.do("POST", invURL(a, inv.ID, "/payments"), api.PaymentCreate{Amount: i64(0)})
	assertError(t, res, body, http.StatusUnprocessableEntity, "amount")
	res, body = a.do("POST", invURL(a, inv.ID, "/payments"), map[string]any{"amount": 1, "paid_on": "2026-02-31"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "paid_on")
	res, body = a.do("POST", invURL(a, inv.ID, "/payments"), api.PaymentCreate{CreateFinalInvoice: true, Amount: i64(1)})
	assertError(t, res, body, http.StatusUnprocessableEntity, "only be created for a proforma")

	// deleting a payment reopens the invoice
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv.ID, fmt.Sprintf("/payments/%d", first)), nil)
	g := getInv(a, inv.ID)
	if g.Status != "open" || g.PaidAmount != 6000 || g.PaidOn != "" || len(g.Payments) != 1 {
		t.Fatalf("after delete: %+v", g.InvoiceSummary)
	}
	res, body = a.do("DELETE", invURL(a, inv.ID, fmt.Sprintf("/payments/%d", first)), nil)
	assertError(t, res, body, http.StatusNotFound, "payment not found")
	// a payment of another invoice cannot be deleted through this one
	inv2 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 500, nil)}})
	p2 := pay(a, inv2.ID, api.PaymentCreate{}).Payment
	res, body = a.do("DELETE", invURL(a, inv.ID, fmt.Sprintf("/payments/%d", p2.ID)), nil)
	assertError(t, res, body, http.StatusNotFound, "payment not found")
	// sent invoice returns to sent
	action(a, inv.ID, "mark_as_sent")
	last := pay(a, inv.ID, api.PaymentCreate{})
	if last.Invoice.Status != "paid" {
		t.Fatalf("paid again: %+v", last.Invoice.InvoiceSummary)
	}
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv.ID, fmt.Sprintf("/payments/%d", last.Payment.ID)), nil)
	if g = getInv(a, inv.ID); g.Status != "sent" {
		t.Fatalf("back to sent: %s", g.Status)
	}

	// cancelled / uncollectible invoices take no payments
	c := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 500, nil)}})
	action(a, c.ID, "cancel")
	res, body = a.do("POST", invURL(a, c.ID, "/payments"), api.PaymentCreate{})
	assertError(t, res, body, http.StatusConflict, "cancelled")
	action(a, c.ID, "undo_cancel")
	action(a, c.ID, "mark_as_uncollectible")
	res, body = a.do("POST", invURL(a, c.ID, "/payments"), api.PaymentCreate{})
	assertError(t, res, body, http.StatusConflict, "uncollectible")

	// refund on a correction: negative total is paid by a negative payment
	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv2.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	r = pay(a, corr.ID, api.PaymentCreate{})
	if r.Payment.Amount != -500 || r.Invoice.Status != "paid" {
		t.Fatalf("refund: %+v", r)
	}

	res, body = a.do("POST", invURL(a, 9999, "/payments"), api.PaymentCreate{})
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

func TestPaymentCreateFinalInvoice(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}}) // 2026-0001
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: subj.ID, Note: strPtr("Záloha"),
		Lines: []api.InvoiceLineInput{line("Práce", "2", 5000, i32(2100)), line("Kniha", "1", 1000, i32(1200))}})

	r := pay(a, pro.ID, api.PaymentCreate{PaidOn: "2026-03-20", CreateFinalInvoice: true})
	if r.FinalInvoiceID == nil || r.Invoice.ID != pro.ID || r.Invoice.Status != "paid" || r.Payment.Amount != pro.Total {
		t.Fatalf("result: %+v", r)
	}
	fin := getInv(a, *r.FinalInvoiceID)
	if fin.DocumentType != "invoice" || fin.Number != "2026-0002" || fin.RelatedID == nil || *fin.RelatedID != pro.ID ||
		fin.Status != "paid" || fin.PaidOn != "2026-03-20" || fin.IssuedOn != "2026-03-20" || fin.TaxableFulfillmentDue != "2026-03-20" ||
		fin.Total != pro.Total || fin.VatTotal != pro.VatTotal || len(fin.Lines) != 2 || fin.Lines[1].Name != "Kniha" ||
		len(fin.Payments) != 1 || fin.Payments[0].Amount != pro.Total || fin.Note != "Záloha" || fin.ClientName != "ACME" {
		t.Fatalf("final invoice: %+v lines %+v", fin.InvoiceSummary, fin.Lines)
	}

	// only once: the settled proforma takes no more payments; the rejected request leaves no payment behind
	res, body := a.do("POST", invURL(a, pro.ID, "/payments"), api.PaymentCreate{Amount: i64(1), CreateFinalInvoice: true})
	assertCode(t, res, body, http.StatusConflict, "proforma_settled")
	if g := getInv(a, pro.ID); len(g.Payments) != 1 {
		t.Fatalf("payments after conflict: %+v", g.Payments)
	}
}

// ---- correction / duplicate / token ----

func TestCorrectionAndDuplicate(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Tags: []string{"x"}, Lines: []api.InvoiceLineInput{
		line("A", "2", 1000, i32(2100)), line("B", "1.5", 500, i32(1200)),
	}})
	action(a, inv.ID, "mark_as_sent")
	// the subject changes later; the correction keeps the invoice's snapshot
	a.mustDo(http.StatusOK, "PATCH", subjectURL(a, subj.ID), map[string]any{"name": "ACME Nová"})

	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	if corr.DocumentType != "correction" || corr.Number != "D2026-0001" || corr.RelatedID == nil || *corr.RelatedID != inv.ID ||
		corr.Status != "open" || corr.SentAt != nil || corr.ClientName != "ACME" || corr.Total != -inv.Total ||
		corr.Lines[0].Quantity != "-2" || corr.Lines[1].Quantity != "-1.5" || corr.Lines[1].VatRateBps != 1200 ||
		!reflect.DeepEqual(corr.Tags, []string{"x"}) || corr.ID == inv.ID {
		t.Fatalf("correction: %+v lines %+v", corr.InvoiceSummary, corr.Lines)
	}
	// the client edits it afterwards
	p := doJSON[api.Invoice](a, http.StatusOK, "PATCH", invURL(a, corr.ID, ""), map[string]any{
		"lines": []any{map[string]any{"id": corr.Lines[0].ID, "name": "A", "quantity": "-1", "unit_price": 1000, "vat_rate_bps": 2100}},
	})
	if p.Total != -1210 {
		t.Fatalf("patched correction: %+v", p.InvoiceSummary)
	}
	res, body := a.do("POST", invURL(a, corr.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	assertError(t, res, body, http.StatusConflict, "only be issued for an invoice")
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}})
	res, body = a.do("POST", invURL(a, pro.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	assertError(t, res, body, http.StatusConflict, "proforma")

	// duplicate: new number, today, open, fresh snapshot, same lines
	ts.now = time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	dup := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/duplicate"), nil)
	if dup.DocumentType != "invoice" || dup.Number != "2026-0002" || dup.IssuedOn != "2026-04-02" || dup.DueOn != "2026-04-16" ||
		dup.Status != "open" || dup.SentAt != nil || dup.ClientName != "ACME Nová" || dup.Total != inv.Total ||
		dup.PublicToken == inv.PublicToken || len(dup.Lines) != 2 || dup.Lines[1].Quantity != "1.5" || dup.RelatedID != nil {
		t.Fatalf("duplicate: %+v", dup.InvoiceSummary)
	}
	dupPro := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, pro.ID, "/duplicate"), nil)
	if dupPro.DocumentType != "proforma" || dupPro.Number != "Z2026-0002" {
		t.Fatalf("duplicate proforma: %+v", dupPro.InvoiceSummary)
	}
	dupCorr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, corr.ID, "/duplicate"), nil)
	if dupCorr.DocumentType != "correction" || dupCorr.RelatedID == nil || *dupCorr.RelatedID != inv.ID || dupCorr.Total != -1210 {
		t.Fatalf("duplicate correction: %+v", dupCorr.InvoiceSummary)
	}
	res, body = a.do("POST", invURL(a, 9999, "/duplicate"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

func TestRegeneratePublicToken(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}})
	action(a, inv.ID, "lock") // allowed on locked invoices too
	r := doJSON[api.Invoice](a, http.StatusOK, "POST", invURL(a, inv.ID, "/regenerate-public-token"), nil)
	if len(r.PublicToken) < 24 || r.PublicToken == inv.PublicToken || getInv(a, inv.ID).PublicToken != r.PublicToken {
		t.Fatalf("token: %q → %q", inv.PublicToken, r.PublicToken)
	}
}

// ---- dashboard ----

func TestDashboard(t *testing.T) {
	ts := newTestServer(t) // today 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	mk := func(docType, issued, currency string, price int64, related *uint) api.Invoice {
		return createInv(a, api.InvoiceCreate{DocumentType: docType, SubjectID: subj.ID, IssuedOn: issued, Currency: currency,
			RelatedID: related, Lines: []api.InvoiceLineInput{line("X", "1", price, nil)}})
	}
	jan := mk("", "2026-01-05", "", 1000, nil)
	pay(a, jan.ID, api.PaymentCreate{})
	mar := mk("", "2026-03-10", "", 2000, nil)        // open, due 03-24
	mk("correction", "2026-02-10", "", -300, &jan.ID) // open, due 02-24: a refund owed, not a receivable
	mk("proforma", "2026-03-11", "", 700, nil)        // not revenue, unpaid
	mk("", "2026-03-12", "EUR", 9999, nil)            // other currency
	mk("", "2025-12-20", "", 400, nil)                // other year, overdue
	cancelled := mk("", "2026-03-13", "", 500, nil)   // cancelled → ignored
	action(a, cancelled.ID, "cancel")
	unc := mk("", "2026-03-14", "", 50, nil) // uncollectible: revenue, not unpaid
	action(a, unc.ID, "mark_as_uncollectible")

	d := doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard"), nil)
	want := api.Dashboard{
		Year: 2026, Currency: "CZK", RevenueByMonth: []int64{1000, -300, 2050, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		RevenueTotal: 2750, UnpaidTotal: 3100, UnpaidCount: 3, OverdueTotal: 400, OverdueCount: 1,
		ExpensesByMonth: make([]int64, 12), ProfitTotal: 2750,
	}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("dashboard\n got %+v\nwant %+v", d, want)
	}
	pay(a, mar.ID, api.PaymentCreate{Amount: i64(500)})
	d = doJSON[api.Dashboard](a, http.StatusOK, "GET", a.acct("/dashboard?year=2025"), nil)
	if d.Year != 2025 || d.RevenueByMonth[11] != 400 || d.RevenueTotal != 400 || d.UnpaidTotal != 2600 {
		t.Fatalf("2025: %+v", d)
	}
	res, body := a.do("GET", a.acct("/dashboard?year=12"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "year")
}

// ---- tenant isolation ----

func TestInvoicesTenantIsolation(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	p := pay(a, inv.ID, api.PaymentCreate{Amount: i64(10)}).Payment

	for _, r := range []struct {
		method, suffix string
		body           any
	}{
		{"GET", "", nil},
		{"PATCH", "", map[string]any{"note": "x"}},
		{"DELETE", "", nil},
		{"POST", "/actions/lock", nil},
		{"POST", "/payments", api.PaymentCreate{}},
		{"DELETE", fmt.Sprintf("/payments/%d", p.ID), nil},
		{"POST", "/correction", nil},
		{"POST", "/duplicate", nil},
		{"POST", "/regenerate-public-token", nil},
	} {
		res, body := b.do(r.method, invURL(b, inv.ID, r.suffix), r.body)
		assertError(t, res, body, http.StatusNotFound, "invoice not found")
		// and not through a's slug either
		res, body = b.do(r.method, invURL(a, inv.ID, r.suffix), r.body)
		assertError(t, res, body, http.StatusNotFound, "not found")
	}
	if l := listInv(b, ""); l.Total != 0 || len(l.Items) != 0 {
		t.Fatalf("b sees invoices: %+v", l)
	}
	if d := doJSON[api.Dashboard](b, http.StatusOK, "GET", b.acct("/dashboard"), nil); d.UnpaidCount != 0 || d.RevenueTotal != 0 {
		t.Fatalf("b dashboard: %+v", d)
	}
	if g := getInv(a, inv.ID); g.Note != "" || g.LockedAt != nil || len(g.Payments) != 1 {
		t.Fatalf("a's invoice changed: %+v", g.InvoiceSummary)
	}
	// b's own numbering is independent
	bs := newSubject(b, api.SubjectCreate{Name: "B"})
	if n := createInv(b, api.InvoiceCreate{SubjectID: bs.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1, nil)}}).Number; n != "2026-0001" {
		t.Fatalf("b number: %s", n)
	}
}
