package isdoc

import (
	"bytes"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

// validate checks doc against the official XSD with xmllint when installed.
func validate(t *testing.T, doc []byte) {
	t.Helper()
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Log("xmllint not installed; XSD validation skipped")
		return
	}
	f := filepath.Join(t.TempDir(), "doc.isdoc")
	if err := os.WriteFile(f, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("xmllint", "--noout", "--schema", "testdata/isdoc-invoice-6.0.2.xsd", f).CombinedOutput()
	if err != nil {
		t.Fatalf("XSD validation failed: %v\n%s\n%s", err, out, doc)
	}
}

// parsed is a subset of the document for assertions.
type parsed struct {
	DocumentType int    `xml:"DocumentType"`
	ID           string `xml:"ID"`
	UUID         string `xml:"UUID"`
	TaxPointDate string `xml:"TaxPointDate"`
	Foreign      string `xml:"ForeignCurrencyCode"`
	CurrRate     string `xml:"CurrRate"`
	Lines        []struct {
		Qty  string `xml:"InvoicedQuantity"`
		Base string `xml:"LineExtensionAmount"`
		Pct  string `xml:"ClassifiedTaxCategory>Percent"`
	} `xml:"InvoiceLines>InvoiceLine"`
	Subs []struct {
		Taxable string `xml:"TaxableAmount"`
		Tax     string `xml:"TaxAmount"`
		Pct     string `xml:"TaxCategory>Percent"`
	} `xml:"TaxTotal>TaxSubTotal"`
	TaxAmount string `xml:"TaxTotal>TaxAmount"`
	Totals    struct {
		Exclusive string `xml:"TaxExclusiveAmount"`
		Inclusive string `xml:"TaxInclusiveAmount"`
		Deposits  string `xml:"PaidDepositsAmount"`
		Payable   string `xml:"PayableAmount"`
		PayCurr   string `xml:"PayableAmountCurr"`
	} `xml:"LegalMonetaryTotal"`
	OrigID   string `xml:"OriginalDocumentReferences>OriginalDocumentReference>ID"`
	PayCode  string `xml:"PaymentMeans>Payment>PaymentMeansCode"`
	PayIBAN  string `xml:"PaymentMeans>Payment>Details>IBAN"`
	PayVS    string `xml:"PaymentMeans>Payment>Details>VariableSymbol"`
	Supplier string `xml:"AccountingSupplierParty>Party>PartyName>Name"`
	Building string `xml:"AccountingSupplierParty>Party>PostalAddress>BuildingNumber"`
}

func render(t *testing.T, inv *model.Invoice, acc *model.Account, opt Options) (parsed, []byte) {
	t.Helper()
	b, err := Generate(inv, acc, opt)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, b)
	var p parsed
	if err := xml.Unmarshal(b, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return p, b
}

func TestInvoiceVatPayer(t *testing.T) {
	inv, acc := pdf.Sample(pdf.SampleSpec{VatPayer: true})
	p, b := render(t, inv, acc, Options{})
	if p.DocumentType != TypeInvoice || p.ID != inv.Number || p.TaxPointDate != "2026-03-15" {
		t.Fatalf("header: %+v", p)
	}
	if p.UUID != DocumentUUID(acc.ID, inv.ID) || p.UUID == DocumentUUID(acc.ID, inv.ID+1) {
		t.Fatalf("uuid %s", p.UUID)
	}
	if p.Totals.Exclusive != money(inv.Subtotal) || p.TaxAmount != money(inv.VatTotal) ||
		p.Totals.Payable != money(inv.Total) {
		t.Fatalf("totals %+v, want %d/%d/%d", p.Totals, inv.Subtotal, inv.VatTotal, inv.Total)
	}
	if len(p.Subs) != 2 || p.Subs[0].Pct != "21" || p.Subs[1].Pct != "12" {
		t.Fatalf("subtotals %+v", p.Subs)
	}
	if p.PayCode != "42" || p.PayIBAN != inv.IBAN || p.PayVS != inv.VariableSymbol {
		t.Fatalf("payment %s %s %s", p.PayCode, p.PayIBAN, p.PayVS)
	}
	if p.Supplier != acc.Name || p.Building != "1597/174" {
		t.Fatalf("supplier %q %q", p.Supplier, p.Building)
	}
	if !bytes.Contains(b, []byte(`xmlns="http://isdoc.cz/namespace/2013" version="6.0.2"`)) {
		t.Fatalf("root: %s", b[:200])
	}
}

func TestNonPayerProformaReverseCharge(t *testing.T) {
	inv, acc := pdf.Sample(pdf.SampleSpec{})
	p, _ := render(t, inv, acc, Options{})
	if len(p.Subs) != 1 || p.Subs[0].Pct != "0" || p.TaxPointDate != "" || p.Totals.Payable != money(inv.Total) {
		t.Fatalf("non payer: %+v", p)
	}

	inv, acc = pdf.Sample(pdf.SampleSpec{DocumentType: model.DocProforma, VatPayer: true})
	p, _ = render(t, inv, acc, Options{})
	if p.DocumentType != TypeProforma || p.TaxPointDate != "" {
		t.Fatalf("proforma: %+v", p)
	}

	inv, acc = pdf.Sample(pdf.SampleSpec{VatPayer: true, ReverseCharge: true})
	p, _ = render(t, inv, acc, Options{})
	if p.TaxAmount != "0.00" || len(p.Subs) != 1 || p.Subs[0].Pct != "0" || p.Totals.Exclusive != money(inv.Subtotal) {
		t.Fatalf("reverse charge: %+v", p)
	}
}

func TestCreditNotePositive(t *testing.T) {
	inv, acc := pdf.Sample(pdf.SampleSpec{DocumentType: model.DocCorrection, VatPayer: true})
	if inv.Total >= 0 {
		t.Fatal("sample correction should be negative")
	}
	rel := &Related{DocumentType: model.DocInvoice, Number: "2026-0041", IssuedOn: "2026-03-01", UUID: DocumentUUID(1, 41)}
	p, _ := render(t, inv, acc, Options{Related: rel})
	if p.DocumentType != TypeCreditNote || p.OrigID != "2026-0041" {
		t.Fatalf("credit note: %+v", p)
	}
	if p.Totals.Inclusive != money(-inv.Total) || strings.HasPrefix(p.Lines[0].Qty, "-") || strings.HasPrefix(p.Lines[0].Base, "-") {
		t.Fatalf("credit note must be positive: %+v", p)
	}
	if p.PayCode != "" {
		t.Fatal("credit note has payment means")
	}

	// a correction increasing the amount is a debit note (vrubopis)
	for i := range inv.Lines {
		inv.Lines[i].QuantityMilli = -inv.Lines[i].QuantityMilli
	}
	inv.Total = -inv.Total
	p, _ = render(t, inv, acc, Options{Related: rel})
	if p.DocumentType != TypeDebitNote {
		t.Fatalf("debit note: %d", p.DocumentType)
	}
}

func TestForeignCurrencyAndDeposit(t *testing.T) {
	inv, acc := pdf.Sample(pdf.SampleSpec{VatPayer: true, Lines: 3})
	inv.Currency, inv.ExchangeRate = "EUR", "24.5"
	inv.IBAN, inv.BankAccount = "", ""
	p, b := render(t, inv, acc, Options{})
	if p.Foreign != "EUR" || p.CurrRate != "24.5" || p.Totals.PayCurr != money(inv.Total) {
		t.Fatalf("foreign: %+v", p)
	}
	// local sums are consistent (Σ subtotals) and ≈ total × rate
	var sum int64
	for _, s := range p.Subs {
		sum += parseMoney(t, s.Taxable) + parseMoney(t, s.Tax)
	}
	if got := parseMoney(t, p.Totals.Inclusive); got != sum || abs(got-inv.Total*245/10) > 10 {
		t.Fatalf("local inclusive %d, Σ %d, total %d", got, sum, inv.Total)
	}
	if bytes.Contains(b, []byte("PaymentMeans")) {
		t.Fatal("no bank account → no payment means")
	}

	// final invoice of a paid proforma deducts the deposit
	inv, acc = pdf.Sample(pdf.SampleSpec{VatPayer: true})
	rel := &Related{DocumentType: model.DocProforma, Number: "Z2026-0012", VariableSymbol: "20260012", PaidAmount: inv.Total}
	p, _ = render(t, inv, acc, Options{Related: rel})
	if p.Totals.Deposits != money(inv.Total) || p.Totals.Payable != "0.00" || p.PayCode != "" {
		t.Fatalf("deposit: %+v", p.Totals)
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[int64]string{0: "0.00", 5: "0.05", -1234: "-12.34", 100: "1.00"} {
		if got := money(in); got != want {
			t.Errorf("money(%d) = %s", in, got)
		}
	}
	if price4(123456) != "12.3456" || bpsPercent(2100) != "21" || bpsPercent(1250) != "12.5" || rateString(24_355_000) != "24.355" {
		t.Error("formatting")
	}
	a := address("Na Příkopě", "Praha", "110 00", "")
	if a.StreetName != "Na Příkopě" || a.BuildingNumber != "" || a.PostalZone != "11000" || a.Country.Name != "Česká republika" {
		t.Errorf("address %+v", a)
	}
	if _, err := Generate(&model.Invoice{Number: "1", ExchangeRate: "x"}, nil, Options{}); err == nil {
		t.Error("invalid rate accepted")
	}
	if _, err := Generate(&model.Invoice{Number: "1", ExchangeRate: "1"}, nil, Options{}); err == nil {
		t.Error("document without lines accepted")
	}
}

func parseMoney(t *testing.T, s string) int64 {
	t.Helper()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	var v int64
	for _, r := range whole + frac {
		v = v*10 + int64(r-'0')
	}
	if neg {
		v = -v
	}
	return v
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
