package pdf

import (
	"testing"

	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

func renderInv(t *testing.T, inv *model.Invoice, acc *model.Account, opt Options) extracted {
	t.Helper()
	b, err := Render(inv, acc, opt)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return extract(t, b)
}

// oneLine replaces the sample lines by one line of 1 000,00 at 21 %.
func oneLine(inv *model.Invoice) {
	inv.Lines = []model.InvoiceLine{{Name: "Služba", QuantityMilli: 1000, UnitPrice: 100_000, VatRateBps: 2100}}
	t, _ := billing.Calculate([]billing.Line{{QuantityMilli: 1000, UnitPrice: 100_000, VatRateBps: 2100}},
		billing.Options{ReverseCharge: inv.ReverseCharge, NonVATPayer: billing.ChargesNoVAT(inv.YourVatMode, inv.ReverseCharge)})
	inv.Lines[0].Base, inv.Lines[0].Vat, inv.Lines[0].Total = t.Lines[0].Base, t.Lines[0].Vat, t.Lines[0].Total
	inv.Subtotal, inv.VatTotal, inv.Total, inv.Rounding, inv.PaidAmount, inv.Payments = t.Subtotal, t.VatTotal, t.Total, 0, 0, nil
}

// Tax audit C-01: a VAT payer's document in a foreign currency states the
// VAT in CZK (§ 29 odst. 1 písm. l) with the rate used.
func TestForeignCurrencyCZKRecap(t *testing.T) {
	inv, acc := Sample(SampleSpec{VatPayer: true})
	inv.Currency, inv.ExchangeRate = "EUR", "24.355"
	oneLine(inv)
	e := renderInv(t, inv, acc, Options{})
	e.has(t, "DPH v Kč (kurz ČNB 1 EUR = 24,355 Kč)")
	e.has(t, "24 355,00 Kč") // base
	e.has(t, "5 114,55 Kč")  // VAT

	inv.Currency, inv.ExchangeRate = "CZK", "1"
	renderInv(t, inv, acc, Options{}).lacks(t, "kurz ČNB")
}

// Tax audit H-01 / L-01: an identified person's reverse-charge invoice (EU
// service) is a tax document with DUZP and "daň odvede zákazník"; payers
// show "PDP" instead of a Czech rate.
func TestIdentifiedPersonReverseCharge(t *testing.T) {
	inv, acc := Sample(SampleSpec{})
	inv.YourVatMode, inv.ReverseCharge, inv.TaxableFulfillmentDue = model.VatModeIdentifiedPerson, true, "2026-09-18"
	inv.IssuedOn = "2026-09-20"
	oneLine(inv)
	e := renderInv(t, inv, acc, Options{})
	e.has(t, "Faktura – daňový doklad")
	e.has(t, "Daň odvede zákazník")
	e.has(t, "18. 9. 2026")

	pay, pacc := Sample(SampleSpec{VatPayer: true, ReverseCharge: true})
	oneLine(pay)
	e = renderInv(t, pay, pacc, Options{})
	e.has(t, "PDP")
	e.lacks(t, "21 %")

	// EU supply of goods: exemption, not "daň odvede zákazník" (tax audit M-02)
	pay.SupplyType = model.SupplyGoods
	e = renderInv(t, pay, pacc, Options{})
	e.has(t, "§ 64 zákona o DPH")
	e.lacks(t, "Daň odvede zákazník")
}

// Tax audit H-02 / M-03: the correction reason and the customer's IČ DPH are printed.
func TestCorrectionReasonAndLocalVatNo(t *testing.T) {
	inv, acc := Sample(SampleSpec{DocumentType: model.DocCorrection, VatPayer: true})
	inv.CorrectionReason = "Vrácení zboží"
	inv.ClientCountry, inv.ClientVatNo, inv.ClientLocalVatNo = "SK", "2020317068", "SK2020317068"
	e := renderInv(t, inv, acc, Options{RelatedNumber: "2026-0041"})
	e.has(t, "Důvod opravy: Vrácení zboží")
	e.has(t, "IČ DPH: SK2020317068")
}

// Tax audit H-03: a tax document for a received payment and the deduction
// of the advance on the final invoice.
func TestTaxDocumentAndDeposits(t *testing.T) {
	td, acc := Sample(SampleSpec{VatPayer: true})
	td.DocumentType, td.Number = model.DocTaxDocument, "ZD2026-0001"
	td.Lines = []model.InvoiceLine{{Name: "Přijatá platba", QuantityMilli: 1000, UnitPrice: 40_000, VatRateBps: 2100}}
	td.PricesIncludeVat = true
	e := renderInv(t, td, acc, Options{RelatedNumber: "Z2026-0001"})
	e.has(t, "Daňový doklad k přijaté platbě")
	e.has(t, "K zálohové faktuře č. Z2026-0001")

	fin, _ := Sample(SampleSpec{VatPayer: true})
	oneLine(fin)
	e = renderInv(t, fin, acc, Options{RelatedNumber: "Z2026-0001", Deposits: []*model.Invoice{td}})
	e.has(t, "Odpočet záloh")
	e.has(t, "ZD2026-0001")
	e.has(t, "−330,58") // base of the 400,00 advance (U+2212 minus)
	e.has(t, "−69,42")
	e.has(t, "DPH po odpočtu záloh")
	e.has(t, "669,42") // 1 000,00 − 330,58
	e.has(t, "140,58") // 210,00 − 69,42
}
