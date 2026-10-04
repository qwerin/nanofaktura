package api_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/reports"
)

func TestVatReport(t *testing.T) {
	ts := newTestServer(t) // today 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	res, body := a.do("GET", a.acct("/reports/vat?period=2026-03"), nil)
	assertCode(t, res, body, http.StatusConflict, "not_vat_payer")

	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME", VatNo: "CZ27074358"})
	muller := newSubject(a, api.SubjectCreate{Name: "Müller GmbH", VatNo: "DE811907980", Country: "DE"})
	inv1 := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("Vývoj", "1", 10_000_000, i32(2100))}})
	createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("Kniha", "1", 100_000, i32(1200))}})
	createInv(a, api.InvoiceCreate{SubjectID: new(muller.ID), ReverseCharge: true, Lines: []api.InvoiceLineInput{line("Služby", "1", 5_000_000, i32(2100))}})
	createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Currency: "EUR", ExchangeRate: "25", Lines: []api.InvoiceLineInput{line("Licence", "1", 100_000, i32(2100))}})
	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv1.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	a.mustDo(http.StatusOK, "PATCH", invURL(a, corr.ID, ""), map[string]any{
		"lines": []api.InvoiceLineInput{line("Sleva", "-0.1", 10_000_000, i32(2100))}})
	// excluded: cancelled, proforma, other period, other account
	cancelled := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 999_900, i32(2100))}})
	action(a, cancelled.ID, "cancel")
	createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("Z", "1", 999_900, i32(2100))}})
	createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), IssuedOn: "2026-04-02", Lines: []api.InvoiceLineInput{line("Duben", "1", 999_900, i32(2100))}})
	setVatPayer(b)
	createInv(b, api.InvoiceCreate{SubjectID: new(newSubject(b, api.SubjectCreate{Name: "B"}).ID), Lines: []api.InvoiceLineInput{line("B", "1", 999_900, i32(2100))}})
	// purchases: big → B.2, small → B.3, not deductible → none
	supplier := api.ExpenseSupplierFields{SupplierName: strPtr("Dodavatel s.r.o."), SupplierVatNo: strPtr("CZ25596641")}
	createExp(a, api.ExpenseCreate{OriginalNumber: "FV-2026-77", ExpenseSupplierFields: supplier, Lines: []api.InvoiceLineInput{line("Notebook", "1", 2_000_000, i32(2100))}})
	createExp(a, api.ExpenseCreate{OriginalNumber: "UC-1", ExpenseSupplierFields: supplier, Lines: []api.InvoiceLineInput{line("Papír", "1", 100_000, i32(2100))}})
	createExp(a, api.ExpenseCreate{OriginalNumber: "UC-2", ExpenseSupplierFields: supplier, TaxDeductible: new(bool),
		Lines: []api.InvoiceLineInput{line("Oběd", "1", 100_000, i32(1200))}})

	r := doJSON[api.VatReport](a, http.StatusOK, "GET", a.acct("/reports/vat?period=2026-03"), nil)
	v := r.Return
	if r.Period != "2026-03" || r.From != "2026-03-01" || r.To != "2026-03-31" || r.VatPeriod != "month" {
		t.Errorf("period %+v", r)
	}
	// ř. 1: 100 000 − 10 000 + 1 000 EUR × 25; ř. 2: 1 000
	if v.R1 != (reports.Pair{Base: 11_500_000, Vat: 2_415_000}) || v.R2 != (reports.Pair{Base: 100_000, Vat: 12_000}) {
		t.Errorf("r1 %+v r2 %+v", v.R1, v.R2)
	}
	if v.R21 != 5_000_000 || v.R25 != 0 || v.R26 != 0 {
		t.Errorf("r21 %d r25 %d r26 %d", v.R21, v.R25, v.R26)
	}
	// UC-2 is not tax-deductible for income tax, but its VAT is deducted (vat_deductible defaults to true)
	if v.R40 != (reports.Pair{Base: 2_100_000, Vat: 441_000}) || v.R41 != (reports.Pair{Base: 100_000, Vat: 12_000}) || v.R46 != 453_000 {
		t.Errorf("r40 %+v r41 %+v", v.R40, v.R41)
	}
	if v.R62 != 2_427_000 || v.R63 != 453_000 || v.R64 != 1_974_000 || v.R65 != 0 {
		t.Errorf("r62–65 %+v", v)
	}
	c := r.Control
	numbers := []string{}
	for _, a4 := range c.A4 {
		numbers = append(numbers, a4.Number)
		if a4.VatNo != "CZ27074358" {
			t.Errorf("A.4 DIČ %s", a4.VatNo)
		}
	}
	if strings.Join(numbers, ",") != "2026-0001,2026-0003,2026-0004,D2026-0001" && len(numbers) != 3 {
		t.Errorf("A.4 %v", numbers)
	}
	if len(c.A4) != 3 || c.A5.Reduced != (reports.Pair{Base: 100_000, Vat: 12_000}) || c.A5.Basic != (reports.Pair{}) {
		t.Errorf("A.4 %+v A.5 %+v", c.A4, c.A5)
	}
	if len(c.B2) != 1 || c.B2[0].Number != "FV-2026-77" || c.B2[0].VatNo != "CZ25596641" ||
		c.B3.Basic != (reports.Pair{Base: 100_000, Vat: 21_000}) || len(c.A1) != 0 {
		t.Errorf("B %+v %+v", c.B2, c.B3)
	}

	// quarter and default period (previous month of 2026-03-15)
	q := doJSON[api.VatReport](a, http.StatusOK, "GET", a.acct("/reports/vat?period=2026-Q1"), nil)
	if q.Return.R1 != v.R1 || q.From != "2026-01-01" {
		t.Errorf("quarter %+v", q.Return.R1)
	}
	if d := doJSON[api.VatReport](a, http.StatusOK, "GET", a.acct("/reports/vat"), nil); d.Period != "2026-02" || d.Return.R1.Base != 0 {
		t.Errorf("default period %+v", d)
	}
	res, body = a.do("GET", a.acct("/reports/vat?period=2026-13"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "period")

	// EPO XML needs the tax office
	res, body = a.do("GET", a.acct("/reports/vat/dphdp3.xml?period=2026-03"), nil)
	assertCode(t, res, body, http.StatusConflict, "missing_tax_office")
	acc := doJSON[api.Account](a, http.StatusOK, "PATCH", a.acct(""), map[string]any{"c_ufo": "451", "c_pracufo": "2001", "vat_period": "quarter"})
	if acc.TaxOffice != "451" || acc.TaxOfficeBranch != "2001" || acc.VatPeriod != "quarter" {
		t.Fatalf("account %+v", acc)
	}
	res, body = a.do("PATCH", a.acct(""), map[string]any{"c_ufo": "abc"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "c_ufo")
	if d := doJSON[api.VatReport](a, http.StatusOK, "GET", a.acct("/reports/vat"), nil); d.Period != "2025-Q4" {
		t.Errorf("default quarter %s", d.Period)
	}

	res, body = a.do("GET", a.acct("/reports/vat/dphdp3.xml?period=2026-03"), nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/xml" ||
		res.Header.Get("Content-Disposition") != `attachment; filename="dphdp3-2026-03.xml"` {
		t.Fatalf("dphdp3: %d %v %s", res.StatusCode, res.Header, body)
	}
	for _, want := range []string{`<DPHDP3 verzePis="03.01.03">`, `obrat23="115000"`, `dan23="24150"`, `pln_sluzby="50000"`,
		`c_ufo="451"`, `dic="12345678"`, `dano_da="19740"`, `mesic="3"`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("dphdp3 lacks %s:\n%s", want, body)
		}
	}
	res, body = a.do("GET", a.acct("/reports/vat/dphkh1.xml?period=2026-03"), nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dphkh1: %d %s", res.StatusCode, body)
	}
	for _, want := range []string{`<DPHKH1 verzePis="03.01.14">`, `c_evid_dd="FV-2026-77"`, `<VetaA5 zakl_dane2="1000.00" dan2="120.00">`,
		`dic_dod="25596641"`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("dphkh1 lacks %s:\n%s", want, body)
		}
	}

	// roles: accountants read reports, members do not
	accountant := ts.memberOf(a, "acc@example.cz", "accountant")
	member := ts.memberOf(a, "member@example.cz", "member")
	accountant.mustDo(http.StatusOK, "GET", accountant.acct("/reports/vat?period=2026-03"), nil)
	res, body = member.do("GET", member.acct("/reports/vat?period=2026-03"), nil)
	assertError(t, res, body, http.StatusForbidden, "your role (member)")

	// tenant isolation: b sees only its own invoice
	rb := doJSON[api.VatReport](b, http.StatusOK, "GET", b.acct("/reports/vat?period=2026-03"), nil)
	if rb.Return.R1.Base != 999_900 || len(rb.Control.B2) != 0 {
		t.Errorf("b report %+v", rb.Return)
	}
}

func TestOverview(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	alfa := newSubject(a, api.SubjectCreate{Name: "Alfa"})
	beta := newSubject(a, api.SubjectCreate{Name: "Beta"})
	i1 := createInv(a, api.InvoiceCreate{SubjectID: new(alfa.ID), IssuedOn: "2026-01-10", Lines: []api.InvoiceLineInput{line("A", "1", 100_000, nil)}})
	i2 := createInv(a, api.InvoiceCreate{SubjectID: new(beta.ID), IssuedOn: "2026-03-01", Lines: []api.InvoiceLineInput{line("B", "1", 300_000, nil)}})
	createInv(a, api.InvoiceCreate{SubjectID: new(beta.ID), IssuedOn: "2026-03-02", Currency: "EUR", ExchangeRate: "25",
		Lines: []api.InvoiceLineInput{line("C", "1", 10_000, nil)}}) // 100 EUR = 2 500 Kč, unpaid
	c := createInv(a, api.InvoiceCreate{SubjectID: new(alfa.ID), IssuedOn: "2026-03-03", Lines: []api.InvoiceLineInput{line("X", "1", 900_000, nil)}})
	action(a, c.ID, "cancel")
	createInv(a, api.InvoiceCreate{SubjectID: new(alfa.ID), IssuedOn: "2025-12-31", Lines: []api.InvoiceLineInput{line("Loni", "1", 50_000, nil)}})
	pay(a, i1.ID, api.PaymentCreate{PaidOn: "2026-01-20"})
	pay(a, i2.ID, api.PaymentCreate{PaidOn: "2026-03-15"})
	e := createExp(a, api.ExpenseCreate{IssuedOn: "2026-02-05", ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: strPtr("D")},
		Lines: []api.InvoiceLineInput{line("Materiál", "1", 50_000, i32(0))}})
	payExp(a, e.ID, api.ExpensePaymentCreate{PaidOn: "2026-02-06"})
	createExp(a, api.ExpenseCreate{IssuedOn: "2026-02-07", TaxDeductible: new(bool), ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: strPtr("D")},
		Lines: []api.InvoiceLineInput{line("Soukromé", "1", 20_000, i32(0))}})

	o := doJSON[api.Overview](a, http.StatusOK, "GET", a.acct("/reports/overview?year=2026"), nil)
	if o.Year != 2026 || o.RevenueByMonth[0] != 100_000 || o.RevenueByMonth[2] != 550_000 || o.RevenueTotal != 650_000 {
		t.Errorf("revenue %v %d", o.RevenueByMonth, o.RevenueTotal)
	}
	if o.ExpensesByMonth[1] != 70_000 || o.ExpensesTotal != 70_000 || o.ProfitTotal != 580_000 || o.ProfitByMonth[1] != -70_000 {
		t.Errorf("expenses %v profit %d", o.ExpensesByMonth, o.ProfitTotal)
	}
	if len(o.TopCustomers) != 2 || o.TopCustomers[0].Name != "Beta" || o.TopCustomers[0].Total != 550_000 || o.TopCustomers[0].Count != 2 ||
		o.TopCustomers[1].Total != 100_000 {
		t.Errorf("top customers %+v", o.TopCustomers)
	}
	if o.AverageDaysToPay == nil || *o.AverageDaysToPay != 12 || o.PaidCount != 2 {
		t.Errorf("average days %v %d", o.AverageDaysToPay, o.PaidCount)
	}
	it := o.IncomeTax
	if it.Income != 400_000 || it.RealExpenses != 50_000 || it.RealTaxBase != 350_000 || len(it.FlatRates) != 4 ||
		it.FlatRates[1].Percent != 60 || it.FlatRates[1].Expenses != 240_000 || it.FlatRates[1].TaxBase != 160_000 {
		t.Errorf("income tax %+v", it)
	}
	if p := doJSON[api.Overview](a, http.StatusOK, "GET", a.acct("/reports/overview"), nil); p.Year != 2026 {
		t.Errorf("default year %d", p.Year)
	}
	if ob := doJSON[api.Overview](b, http.StatusOK, "GET", b.acct("/reports/overview?year=2026"), nil); ob.RevenueTotal != 0 || len(ob.TopCustomers) != 0 || ob.AverageDaysToPay != nil {
		t.Errorf("b overview %+v", ob)
	}
}
