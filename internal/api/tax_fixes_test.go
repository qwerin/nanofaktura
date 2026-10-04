package api_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/reports"
)

// Tax audit H-04: a VAT payer's tax document needs a DUZP; old documents
// without one are reported by their issue date with a warning.
func TestVatPayerRequiresTaxPointDate(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	res, body := a.do("POST", a.acct("/invoices"), api.InvoiceCreate{SubjectID: new(acme.ID), TaxableFulfillmentDue: strPtr(""),
		Lines: []api.InvoiceLineInput{line("X", "1", 100_000, i32(2100))}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "taxable_fulfillment_due")
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100_000, i32(2100))}})
	res, body = a.do("PATCH", invURL(a, inv.ID, ""), map[string]any{"taxable_fulfillment_due": ""})
	assertError(t, res, body, http.StatusUnprocessableEntity, "taxable_fulfillment_due")
	// a proforma is no tax document
	createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: new(acme.ID), TaxableFulfillmentDue: strPtr(""),
		Lines: []api.InvoiceLineInput{line("X", "1", 100, i32(2100))}})

	ts.db.Model(&model.Invoice{}).Where("id = ?", inv.ID).Update("taxable_fulfillment_due", "") // legacy data
	v := vatReport(a, "2026-03")
	if v.Return.R1.Vat != 21_000 || !hasWarning(v, reports.WarnMissingTaxPointDate, inv.Number) {
		t.Fatalf("legacy document without DUZP: %+v %+v", v.Return.R1, v.Warnings)
	}
}

// Tax audit H-02: a VAT payer's correction states its reason (API, PDF, ISDOC).
func TestCorrectionReasonRequired(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100_000, i32(2100))}})
	res, body := a.do("POST", invURL(a, inv.ID, "/correction"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "correction_reason")
	cr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/correction"), api.CorrectionCreate{CorrectionReason: "Vrácení zboží"})
	if cr.CorrectionReason != "Vrácení zboží" {
		t.Fatalf("reason %q", cr.CorrectionReason)
	}
	res, body = a.do("PATCH", invURL(a, cr.ID, ""), map[string]any{"correction_reason": " "})
	assertError(t, res, body, http.StatusUnprocessableEntity, "correction_reason")
	isd := a.mustDo(http.StatusOK, "GET", invURL(a, cr.ID, "/isdoc"), nil)
	if !bytes.Contains(isd, []byte("Důvod opravy: Vrácení zboží")) {
		t.Fatalf("ISDOC note:\n%s", isd)
	}
	validateXSD(t, isd, isdocXSD)

	// non-payers issue a plain "opravná faktura"; the reason is optional
	b := ts.signup("b@example.cz", "Firma B")
	bs := newSubject(b, api.SubjectCreate{Name: "ACME"})
	bi := createInv(b, api.InvoiceCreate{SubjectID: new(bs.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	b.mustDo(http.StatusCreated, "POST", invURL(b, bi.ID, "/correction"), nil)
}

// Tax audit M-01: a sent tax document of a VAT payer is corrected, not cancelled.
func TestCancelSentTaxDocument(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	mk := func() api.Invoice {
		return createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, i32(2100))}})
	}
	sent := mk()
	action(a, sent.ID, "mark_as_sent")
	res, body := a.do("POST", invURL(a, sent.ID, "/actions/cancel"), nil)
	assertCode(t, res, body, http.StatusConflict, "correction_required")
	action(a, mk().ID, "cancel") // not delivered yet: may be cancelled

	b := ts.signup("b@example.cz", "Firma B") // non-payer
	bs := newSubject(b, api.SubjectCreate{Name: "ACME"})
	bi := createInv(b, api.InvoiceCreate{SubjectID: new(bs.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	action(b, bi.ID, "mark_as_sent")
	action(b, bi.ID, "cancel")
}

// Tax audit H-01: an identified person's EU service is a tax document with
// DUZP and the reverse-charge wording in the ISDOC.
func TestIdentifiedPersonReverseChargeISDOC(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_mode": "identified_person", "vat_no": "CZ8001011234"})
	de := newSubject(a, api.SubjectCreate{Name: "GmbH", Country: "DE", VatNo: "DE811907980"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(de.ID), ReverseCharge: true, IssuedOn: "2026-03-20",
		TaxableFulfillmentDue: strPtr("2026-03-18"), Lines: []api.InvoiceLineInput{line("Consulting", "1", 200_000, i32(2100))}})
	if inv.VatTotal != 0 || inv.TaxableFulfillmentDue != "2026-03-18" {
		t.Fatalf("invoice %+v", inv.InvoiceSummary)
	}
	isd := a.mustDo(http.StatusOK, "GET", invURL(a, inv.ID, "/isdoc"), nil)
	for _, want := range []string{"<TaxPointDate>2026-03-18</TaxPointDate>", "Daň odvede zákazník"} {
		if !bytes.Contains(isd, []byte(want)) {
			t.Errorf("ISDOC lacks %s", want)
		}
	}
	validateXSD(t, isd, isdocXSD)
	res, body := a.do("PATCH", invURL(a, inv.ID, ""), map[string]any{"taxable_fulfillment_due": ""})
	assertError(t, res, body, http.StatusUnprocessableEntity, "taxable_fulfillment_due")
}

// Tax audit M-02/M-03: EU goods go to ř. 20 with the § 64 wording, the
// Slovak IČ DPH is snapshotted and printed, the EC Sales List is reminded.
func TestEUSuppliesGoodsAndSlovakVatID(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	de := newSubject(a, api.SubjectCreate{Name: "GmbH", Country: "DE", VatNo: "DE811907980"})
	sk := newSubject(a, api.SubjectCreate{Name: "s.r.o.", Country: "SK", VatNo: "2020317068", LocalVatNo: "SK2020317068"})
	skNoID := newSubject(a, api.SubjectCreate{Name: "Bez IČ DPH", Country: "SK", VatNo: "2020317068"})
	goods := createInv(a, api.InvoiceCreate{SubjectID: new(de.ID), ReverseCharge: true, SupplyType: "goods",
		Lines: []api.InvoiceLineInput{line("Zboží", "1", 300_000, i32(2100))}})
	svc := createInv(a, api.InvoiceCreate{SubjectID: new(sk.ID), ReverseCharge: true, Lines: []api.InvoiceLineInput{line("Služba", "1", 100_000, i32(2100))}})
	bad := createInv(a, api.InvoiceCreate{SubjectID: new(skNoID.ID), ReverseCharge: true, Lines: []api.InvoiceLineInput{line("Služba", "1", 50_000, i32(2100))}})
	if svc.ClientLocalVatNo != "SK2020317068" || svc.SupplyType != "services" || goods.SupplyType != "goods" {
		t.Fatalf("snapshot %+v / %+v", svc.InvoiceSummary, goods.InvoiceSummary)
	}
	v := vatReport(a, "2026-03")
	if v.Return.R20 != 300_000 || v.Return.R21 != 150_000 {
		t.Fatalf("r20 %d r21 %d", v.Return.R20, v.Return.R21)
	}
	if !hasWarning(v, reports.WarnECSalesList, "") || hasWarning(v, reports.WarnEUReverseChargeNoVat, svc.Number) ||
		!hasWarning(v, reports.WarnEUReverseChargeNoVat, bad.Number) {
		t.Fatalf("warnings %+v", v.Warnings)
	}
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"c_ufo": "451"})
	dp3 := a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/dphdp3.xml?period=2026-03"), nil)
	if !bytes.Contains(dp3, []byte(`dod_zb="3000"`)) {
		t.Fatalf("dphdp3:\n%s", dp3)
	}
	validateXSD(t, dp3, dphdp3XSD)
	isd := a.mustDo(http.StatusOK, "GET", invURL(a, svc.ID, "/isdoc"), nil)
	if !bytes.Contains(isd, []byte("<CompanyID>SK2020317068</CompanyID>")) {
		t.Fatalf("ISDOC without IČ DPH:\n%s", isd)
	}
	validateXSD(t, isd, isdocXSD)
	isd = a.mustDo(http.StatusOK, "GET", invURL(a, goods.ID, "/isdoc"), nil)
	if !bytes.Contains(isd, []byte("§ 64")) || bytes.Contains(isd, []byte("Daň odvede zákazník")) {
		t.Fatalf("ISDOC of EU goods:\n%s", isd)
	}
}

// Tax audit H-05/M-04: received EU services and domestic reverse charge are
// self-assessed (ř. 5/10, 43, KH A.2/B.1); the VAT deduction is independent
// of the income-tax deductibility; zero-VAT purchases from abroad warn.
func TestReverseChargePurchases(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	ie := newSubject(a, api.SubjectCreate{Name: "Ads Ltd", Type: "supplier", Country: "IE", VatNo: "IE6388047V"})
	cz := newSubject(a, api.SubjectCreate{Name: "Stavby s.r.o.", Type: "supplier", VatNo: "CZ27074358"})
	us := newSubject(a, api.SubjectCreate{Name: "Cloud Inc", Type: "supplier", Country: "US"})
	no := false
	createExp(a, api.ExpenseCreate{SubjectID: &ie.ID, OriginalNumber: "IE-1", ReverseCharge: true,
		Lines: []api.InvoiceLineInput{line("Reklama", "1", 1_000_000, i32(2100))}})
	createExp(a, api.ExpenseCreate{SubjectID: &cz.ID, OriginalNumber: "CZ-RC", ReverseCharge: true, TaxDeductible: &no,
		Lines: []api.InvoiceLineInput{line("Stavební práce", "1", 2_000_000, i32(2100))}})
	createExp(a, api.ExpenseCreate{SubjectID: &us.ID, OriginalNumber: "US-1", ReverseCharge: true, VatDeductible: &no,
		Lines: []api.InvoiceLineInput{line("Hosting", "1", 100_000, i32(2100))}})
	createExp(a, api.ExpenseCreate{SubjectID: &ie.ID, OriginalNumber: "IE-2", Lines: []api.InvoiceLineInput{line("Bez DPH", "1", 50_000, i32(0))}})
	createExp(a, api.ExpenseCreate{SubjectID: &cz.ID, OriginalNumber: "NOVAT", VatDeductible: &no,
		Lines: []api.InvoiceLineInput{line("Notebook", "1", 100_000, i32(2100))}})

	v := vatReport(a, "2026-03")
	r := v.Return
	if r.R5 != (reports.Pair{Base: 1_000_000, Vat: 210_000}) || r.R10 != (reports.Pair{Base: 2_000_000, Vat: 420_000}) ||
		r.R12 != (reports.Pair{Base: 100_000, Vat: 21_000}) {
		t.Fatalf("self-assessed output: r5 %v r10 %v r12 %v", r.R5, r.R10, r.R12)
	}
	// the US hosting is not deductible; NOVAT neither (vat_deductible=false)
	if r.R43 != (reports.Pair{Base: 3_000_000, Vat: 630_000}) || r.R40 != (reports.Pair{}) || r.R46 != 630_000 {
		t.Fatalf("deduction: r43 %v r40 %v r46 %d", r.R43, r.R40, r.R46)
	}
	if r.R62 != 651_000 || r.R64 != 21_000 {
		t.Fatalf("r62 %d r64 %d", r.R62, r.R64)
	}
	c := v.Control
	if len(c.A2) != 2 || c.A2[0].Country != "IE" || c.A2[0].VatID != "6388047V" || len(c.B1) != 1 || c.B1[0].SupplierVatNo != "27074358" {
		t.Fatalf("control A.2 %+v B.1 %+v", c.A2, c.B1)
	}
	if !hasWarning(v, reports.WarnPossibleRC, "IE-2") || !hasWarning(v, reports.WarnRCSubjectCode, "CZ-RC") {
		t.Fatalf("warnings %+v", v.Warnings)
	}
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"c_ufo": "451"})
	dp3 := a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/dphdp3.xml?period=2026-03"), nil)
	for _, want := range []string{`p_sl23_e="10000"`, `dan_psl23_e="2100"`, `rez_pren23="20000"`, `p_sl23_z="1000"`,
		`nar_zdp23="30000"`, `od_zdp23="6300"`, `odp_sum_nar="6300"`} {
		if !bytes.Contains(dp3, []byte(want)) {
			t.Errorf("dphdp3 lacks %s", want)
		}
	}
	validateXSD(t, dp3, dphdp3XSD)
	kh := a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/dphkh1.xml?period=2026-03"), nil)
	for _, want := range []string{`<VetaA2 c_radku="1" k_stat="IE" vatid_dod="6388047V"`, `<VetaB1 c_radku="1" dic_dod="27074358"`, `rez_pren23="20000.00"`} {
		if !bytes.Contains(kh, []byte(want)) {
			t.Errorf("dphkh1 lacks %s", want)
		}
	}
	validateXSD(t, kh, dphkh1XSD)
}

// Tax audit M-05: a legal person files the control statement monthly.
func TestControlStatementMonthlyForLegalPerson(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_mode": "vat_payer", "vat_no": "CZ12345678",
		"vat_period": "quarter", "c_ufo": "451"})
	res, body := a.do("GET", a.acct("/reports/vat/dphkh1.xml?period=2026-Q1"), nil)
	assertCode(t, res, body, http.StatusConflict, "monthly_control_statement")
	if v := vatReport(a, "2026-Q1"); !hasWarning(v, reports.WarnMonthlyControl, "") {
		t.Fatalf("warnings %+v", v.Warnings)
	}
	a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/dphkh1.xml?period=2026-03"), nil)
	a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/dphdp3.xml?period=2026-Q1"), nil)
	// a natural person (birth number DIČ) may file quarterly
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_no": "CZ8001011234"})
	a.mustDo(http.StatusOK, "GET", a.acct("/reports/vat/dphkh1.xml?period=2026-Q1"), nil)
}

// Tax audit L-02/L-04/L-08: payer rates since 2024, one number space for tax
// documents, a non-payer's correction is no "opravný daňový doklad" in e-mails.
func TestTaxLowFixes(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	res, body := a.do("POST", a.acct("/invoices"), api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, i32(1000))}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "lines[0].vat_rate_bps")
	createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), IssuedOn: "2023-12-20", Lines: []api.InvoiceLineInput{line("X", "1", 100, i32(1000))}})

	inv := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Number: "X-1", Lines: []api.InvoiceLineInput{line("X", "1", 100, i32(2100))}})
	res, body = a.do("POST", a.acct("/invoices"), api.InvoiceCreate{DocumentType: "correction", RelatedID: &inv.ID, Number: "X-1",
		CorrectionReason: "Sleva", SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "-1", 100, i32(2100))}})
	assertCode(t, res, body, http.StatusConflict, "already_exists")

	b := ts.signup("b@example.cz", "Firma B")
	bs := newSubject(b, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	bi := createInv(b, api.InvoiceCreate{SubjectID: new(bs.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	cr := doJSON[api.Invoice](b, http.StatusCreated, "POST", invURL(b, bi.ID, "/correction"), nil)
	b.mustDo(http.StatusOK, "POST", invURL(b, cr.ID, "/send"), api.InvoiceSend{})
	if m, _ := ts.mail.Last(); strings.Contains(m.Subject+m.Text, "daňový doklad") || !strings.Contains(m.Subject+m.Text, "pravná faktura") {
		t.Fatalf("e-mail of a non-payer's correction: %s\n%s", m.Subject, m.Text)
	}
}
