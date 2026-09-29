// Package isdoc generates ISDOC 6.0.2 XML (the Czech national e-invoice
// standard, https://isdoc.cz) for invoices, corrections and proformas.
//
// Mapping of document types: invoice → 1 (faktura – daňový doklad),
// correction with a negative (or zero) total → 2 (dobropis), correction with
// a positive total → 3 (vrubopis), proforma → 4 (zálohová faktura).
// Per ISDOC rule A.6 a credit note is written in positive amounts, so all
// quantities and amounts of a dobropis are negated.
//
// Amounts without the Curr suffix are in the local currency (CZK); for a
// foreign-currency document they are converted with the invoice exchange
// rate and the *Curr elements carry the original amounts. The VAT summary is
// converted per rate and the document totals are sums of the converted
// summary, so the equalities required by ISDOC (A.10, A.11) always hold.
package isdoc

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Namespace and Version of the generated documents.
const (
	Namespace = "http://isdoc.cz/namespace/2013"
	Version   = "6.0.2"
)

// LocalCurrency is the currency of the non-Curr amounts.
const LocalCurrency = "CZK"

// ISDOC document types.
const (
	TypeInvoice    = 1
	TypeCreditNote = 2
	TypeDebitNote  = 3
	TypeProforma   = 4
)

// uuidNamespace is the UUID v5 namespace of NanoFaktura documents.
var uuidNamespace = uuid.MustParse("6f1c1d5e-3f0b-4b55-9d0e-6a1f9f1b2c11")

// DocumentUUID is the stable UUID of an invoice: UUID v5 of the account and
// invoice ids, so repeated exports of one document carry the same UUID
// (ISDOC A.4: importers treat an equal UUID as the same document).
func DocumentUUID(accountID, invoiceID uint) string {
	return uuid.NewSHA1(uuidNamespace, fmt.Appendf(nil, "invoice:%d:%d", accountID, invoiceID)).String()
}

// Related describes the document an invoice refers to: the corrected
// invoice of a correction, or the paid proforma of a final invoice.
type Related struct {
	DocumentType   string // model.DocInvoice / DocProforma …
	Number         string
	IssuedOn       string
	UUID           string
	VariableSymbol string
	PaidAmount     int64 // proforma: amount paid (deducted as a non-tax deposit)
}

// Options complete the data of Generate.
type Options struct {
	UUID          string   // document UUID; empty = DocumentUUID(inv.AccountID, inv.ID)
	Related       *Related // optional
	IssuingSystem string   // default "NanoFaktura"
}

// DocumentType returns the ISDOC document type of inv.
func DocumentType(inv *model.Invoice) int {
	switch inv.DocumentType {
	case model.DocProforma:
		return TypeProforma
	case model.DocCorrection:
		if inv.Total > 0 {
			return TypeDebitNote
		}
		return TypeCreditNote
	default:
		return TypeInvoice
	}
}

// Generate returns the ISDOC XML of inv (loaded with Lines and Payments).
// acc (optional) adds the supplier's e-mail and phone.
func Generate(inv *model.Invoice, acc *model.Account, opt Options) ([]byte, error) {
	rate, err := billing.ParseRate(inv.ExchangeRate)
	if err != nil {
		return nil, fmt.Errorf("isdoc: %w", err)
	}
	foreign := inv.Currency != "" && !strings.EqualFold(inv.Currency, LocalCurrency)
	if !foreign {
		rate = billing.RateScale
	}
	g := gen{inv: inv, rate: rate, foreign: foreign, sign: 1, docType: DocumentType(inv)}
	if g.docType == TypeCreditNote {
		g.sign = -1
	}
	g.payer = inv.YourVatMode == model.VatModePayer
	doc, err := g.build(acc, opt)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("isdoc: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

type gen struct {
	inv     *model.Invoice
	rate    int64 // billing.ParseRate scale
	foreign bool
	payer   bool
	sign    int64 // -1 for credit notes (written in positive amounts)
	docType int
}

// local converts a document-currency amount to CZK (signed).
func (g *gen) local(v int64) int64 { return billing.ToLocal(g.sign*v, g.rate) }

// curr returns the signed foreign amount, or "" for CZK documents.
func (g *gen) curr(v int64) string {
	if !g.foreign {
		return ""
	}
	return money(g.sign * v)
}

// percent is the VAT percentage shown for a line/summary rate: 0 when no
// VAT is charged (non payer, reverse charge).
func (g *gen) percent(rateBps int32) int32 {
	if !g.payer || g.inv.ReverseCharge {
		return 0
	}
	return rateBps
}

func (g *gen) build(acc *model.Account, opt Options) (*Invoice, error) {
	inv := g.inv
	id := opt.UUID
	if id == "" {
		id = DocumentUUID(inv.AccountID, inv.ID)
	}
	sys := opt.IssuingSystem
	if sys == "" {
		sys = "NanoFaktura"
	}
	d := &Invoice{
		Xmlns:         Namespace,
		Version:       Version,
		DocumentType:  g.docType,
		ID:            inv.Number,
		UUID:          id,
		IssuingSystem: sys,
		IssueDate:     inv.IssuedOn,
		VATApplicable: g.payer,
		Note:          g.note(),
		LocalCurrency: LocalCurrency,
		CurrRate:      "1",
		RefCurrRate:   "1",
		Supplier: party{Party: Party{
			ID:           inv.YourRegistrationNo,
			Name:         inv.YourName,
			Address:      address(inv.YourStreet, inv.YourCity, inv.YourZip, inv.YourCountry),
			TaxSchemes:   taxSchemes(inv.YourVatNo),
			Registration: preformatted(inv.YourRegisteredBy),
		}},
		Customer: party{Party: Party{
			ID:         inv.ClientRegistrationNo,
			Name:       inv.ClientName,
			Address:    address(inv.ClientStreet, inv.ClientCity, inv.ClientZip, inv.ClientCountry),
			TaxSchemes: taxSchemes(inv.ClientVatNo),
			Contact:    contact(inv.ClientFullName, "", inv.ClientEmail),
		}},
	}
	if acc != nil {
		d.Supplier.Party.Contact = contact("", acc.Phone, acc.Email)
	}
	// TaxPointDate only on tax documents; for corrections the tax point is
	// the delivery of the document (ISDOC A.5).
	if g.docType == TypeInvoice && g.payer && inv.TaxableFulfillmentDue != "" {
		d.TaxPointDate = inv.TaxableFulfillmentDue
	}
	if g.foreign {
		d.ForeignCurrency = strings.ToUpper(inv.Currency)
		d.CurrRate = rateString(g.rate)
	}
	if n := strings.TrimSpace(inv.OrderNumber); n != "" {
		d.OrderReferences = &orderReferences{Refs: []OrderReference{{IDAttr: "1", SalesOrderID: n, ExternalOrderID: n}}}
	}
	rel := opt.Related
	if rel != nil && (g.docType == TypeCreditNote || g.docType == TypeDebitNote) {
		ref := OriginalDocumentReference{IDAttr: "1", ID: rel.Number, IssueDate: rel.IssuedOn, UUID: rel.UUID}
		d.OriginalDocumentReferences = &originalDocumentReferences{Refs: []OriginalDocumentReference{ref}}
	}

	lines, err := g.lines()
	if err != nil {
		return nil, err
	}
	d.Lines.Lines = lines
	subs, err := g.taxSubTotals()
	if err != nil {
		return nil, err
	}

	var taxable, tax, incl int64
	var taxableCurr, taxCurr, inclCurr int64
	for _, s := range subs {
		taxable += s.taxable
		tax += s.tax
		incl += s.taxable + s.tax
		taxableCurr += s.taxableCurr
		taxCurr += s.taxCurr
		inclCurr += s.taxableCurr + s.taxCurr
		d.TaxTotal.SubTotals = append(d.TaxTotal.SubTotals, s.xml(g))
	}
	d.TaxTotal.TaxAmount = money(tax)
	d.TaxTotal.TaxAmountCurr = g.currRaw(taxCurr)

	rounding := g.local(inv.Rounding)
	var deposits, depositsCurr int64
	if rel != nil && rel.DocumentType == model.DocProforma && g.docType == TypeInvoice && rel.PaidAmount > 0 {
		depositsCurr = min(rel.PaidAmount, inv.Total)
		deposits = g.local(depositsCurr)
		d.NonTaxedDeposits = &nonTaxedDeposits{Deposits: []NonTaxedDeposit{{
			ID: rel.Number, VariableSymbol: rel.VariableSymbol, DepositAmountCurr: g.curr(depositsCurr), DepositAmount: money(deposits),
		}}}
	}
	payable := incl + rounding - deposits
	d.Totals = LegalMonetaryTotal{
		TaxExclusiveAmount:                   money(taxable),
		TaxExclusiveAmountCurr:               g.currRaw(taxableCurr),
		TaxInclusiveAmount:                   money(incl),
		TaxInclusiveAmountCurr:               g.currRaw(inclCurr),
		AlreadyClaimedTaxExclusiveAmount:     "0",
		AlreadyClaimedTaxInclusiveAmount:     "0",
		DifferenceTaxExclusiveAmount:         money(taxable),
		DifferenceTaxExclusiveAmountCurr:     g.currRaw(taxableCurr),
		DifferenceTaxInclusiveAmount:         money(incl),
		DifferenceTaxInclusiveAmountCurr:     g.currRaw(inclCurr),
		PayableRoundingAmount:                money(rounding),
		PaidDepositsAmount:                   money(deposits),
		PayableAmount:                        money(payable),
		PayableAmountCurr:                    g.currRaw(inclCurr + g.sign*inv.Rounding - g.sign*depositsCurr),
		AlreadyClaimedTaxExclusiveAmountCurr: g.currRaw(0),
		AlreadyClaimedTaxInclusiveAmountCurr: g.currRaw(0),
		PayableRoundingAmountCurr:            g.currRaw(g.sign * inv.Rounding),
		PaidDepositsAmountCurr:               g.currRaw(g.sign * depositsCurr),
	}
	if pm := g.paymentMeans(payable); pm != nil {
		d.PaymentMeans = pm
	}
	return d, nil
}

// currRaw formats an already signed foreign amount ("" for CZK documents).
func (g *gen) currRaw(v int64) string {
	if !g.foreign {
		return ""
	}
	return money(v)
}

func (g *gen) note() *Note {
	parts := []string{}
	for _, p := range []string{g.inv.Note, g.inv.FooterNote} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if g.inv.ReverseCharge {
		parts = append(parts, "Daň odvede zákazník (přenesená daňová povinnost).")
	}
	if g.inv.DocumentType == model.DocProforma {
		parts = append(parts, "Nejedná se o daňový doklad.")
	}
	if len(parts) == 0 {
		return nil
	}
	return &Note{Text: strings.Join(parts, "\n\n")}
}

func (g *gen) lines() ([]InvoiceLine, error) {
	inv := g.inv
	out := make([]InvoiceLine, 0, len(inv.Lines))
	for i, l := range inv.Lines {
		pct := g.percent(l.VatRateBps)
		// Unit prices in ten-thousandths of the currency unit (4 decimals).
		var net, gross int64
		if inv.PricesIncludeVat {
			gross = l.UnitPrice * 100
			net = billing.DivRound(l.UnitPrice*1_000_000, int64(10000+pct))
		} else {
			net = l.UnitPrice * 100
			gross = billing.DivRound(l.UnitPrice*int64(10000+pct), 100)
		}
		vat := l.Vat
		if pct == 0 {
			vat = 0
		}
		method := 0
		if inv.PricesIncludeVat {
			method = 1
		}
		line := InvoiceLine{
			ID:                                  strconv.Itoa(i + 1),
			Quantity:                            &Quantity{UnitCode: l.UnitName, Value: billing.FormatQuantity(g.sign * l.QuantityMilli)},
			LineExtensionAmountCurr:             g.curr(l.Base),
			LineExtensionAmount:                 money(g.local(l.Base)),
			LineExtensionAmountTaxInclusiveCurr: g.curr(l.Base + vat),
			LineExtensionAmountTaxInclusive:     money(g.local(l.Base + vat)),
			LineExtensionTaxAmount:              money(g.local(vat)),
			UnitPrice:                           price4(billing.ToLocal(net, g.rate)),
			UnitPriceTaxInclusive:               price4(billing.ToLocal(gross, g.rate)),
			TaxCategory: ClassifiedTaxCategory{
				Percent:              bpsPercent(pct),
				VATCalculationMethod: method,
				VATApplicable:        g.payer,
			},
			Item: &Item{Description: l.Name},
		}
		if g.payer && inv.ReverseCharge {
			line.VATNote = &Note{Text: "Daň odvede zákazník"}
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("isdoc: document %s has no lines", inv.Number)
	}
	return out, nil
}

type subTotal struct {
	pct                  int32
	taxable, tax         int64 // local, signed
	taxableCurr, taxCurr int64 // foreign, signed
}

func (s subTotal) xml(g *gen) TaxSubTotal {
	return TaxSubTotal{
		TaxableAmountCurr:                g.currRaw(s.taxableCurr),
		TaxableAmount:                    money(s.taxable),
		TaxAmountCurr:                    g.currRaw(s.taxCurr),
		TaxAmount:                        money(s.tax),
		TaxInclusiveAmountCurr:           g.currRaw(s.taxableCurr + s.taxCurr),
		TaxInclusiveAmount:               money(s.taxable + s.tax),
		AlreadyClaimedTaxableAmountCurr:  g.currRaw(0),
		AlreadyClaimedTaxableAmount:      "0",
		AlreadyClaimedTaxAmountCurr:      g.currRaw(0),
		AlreadyClaimedTaxAmount:          "0",
		AlreadyClaimedTaxInclusiveCurr:   g.currRaw(0),
		AlreadyClaimedTaxInclusiveAmount: "0",
		DifferenceTaxableAmountCurr:      g.currRaw(s.taxableCurr),
		DifferenceTaxableAmount:          money(s.taxable),
		DifferenceTaxAmountCurr:          g.currRaw(s.taxCurr),
		DifferenceTaxAmount:              money(s.tax),
		DifferenceTaxInclusiveCurr:       g.currRaw(s.taxableCurr + s.taxCurr),
		DifferenceTaxInclusiveAmount:     money(s.taxable + s.tax),
		TaxCategory:                      TaxCategory{Percent: bpsPercent(s.pct), VATApplicable: g.payer},
	}
}

// taxSubTotals is the VAT recap (billing rules) merged by shown percentage.
func (g *gen) taxSubTotals() ([]subTotal, error) {
	inv := g.inv
	lines := make([]billing.Line, len(inv.Lines))
	for i, l := range inv.Lines {
		lines[i] = billing.Line{QuantityMilli: l.QuantityMilli, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps}
	}
	t, err := billing.Calculate(lines, billing.Options{
		PricesIncludeVAT: inv.PricesIncludeVat, ReverseCharge: inv.ReverseCharge,
		NonVATPayer: billing.ChargesNoVAT(inv.YourVatMode, inv.ReverseCharge),
	})
	if err != nil {
		return nil, fmt.Errorf("isdoc: %w", err)
	}
	byPct := map[int32]*subTotal{}
	for _, r := range t.VatRecap {
		pct := g.percent(r.VatRateBps)
		s := byPct[pct]
		if s == nil {
			s = &subTotal{pct: pct}
			byPct[pct] = s
		}
		vat := r.Vat
		if pct == 0 {
			vat = 0
		}
		s.taxable += g.local(r.Base)
		s.tax += g.local(vat)
		s.taxableCurr += g.sign * r.Base
		s.taxCurr += g.sign * vat
	}
	out := make([]subTotal, 0, len(byPct))
	for _, s := range byPct {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pct > out[j].pct })
	return out, nil
}

// paymentMeans: bank transfer details when something is to be paid by bank.
func (g *gen) paymentMeans(payable int64) *PaymentMeans {
	inv := g.inv
	if payable <= 0 || g.docType == TypeCreditNote || (inv.PaymentMethod != "" && inv.PaymentMethod != "bank") {
		return nil
	}
	if inv.IBAN == "" && inv.BankAccount == "" {
		return nil
	}
	number, bankCode, _ := strings.Cut(inv.BankAccount, "/")
	return &PaymentMeans{Payments: []Payment{{
		PaidAmount: money(payable),
		Code:       42, // převod na účet
		Details: &PaymentDetails{
			PaymentDueDate: cmp.Or(inv.DueOn, inv.IssuedOn),
			ID:             number,
			BankCode:       bankCode,
			Name:           "",
			IBAN:           inv.IBAN,
			BIC:            inv.SwiftBIC,
			VariableSymbol: inv.VariableSymbol,
		},
	}}}
}

// ---- helpers ----

// money formats minor units as a decimal with two places ("-12.30").
func money(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// price4 formats ten-thousandths of a unit ("12.3456").
func price4(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%04d", sign, v/10000, v%10000)
}

func bpsPercent(bps int32) string {
	s := fmt.Sprintf("%d.%02d", bps/100, bps%100)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

func rateString(rate int64) string {
	s := fmt.Sprintf("%d.%06d", rate/billing.RateScale, rate%billing.RateScale)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

// buildingRe splits "Vinohradská 1597/174" into street and building number.
var buildingRe = regexp.MustCompile(`^(.*?)\s+(\d[\w/\-]*)$`)

func address(street, city, zip, country string) PostalAddress {
	street = strings.TrimSpace(street)
	a := PostalAddress{StreetName: street, CityName: city, PostalZone: strings.ReplaceAll(zip, " ", "")}
	if m := buildingRe.FindStringSubmatch(street); m != nil {
		a.StreetName, a.BuildingNumber = m[1], m[2]
	}
	code := strings.ToUpper(strings.TrimSpace(country))
	if code == "" {
		code = "CZ"
	}
	a.Country = Country{Code: code, Name: countryName(code)}
	return a
}

var countryNames = map[string]string{
	"CZ": "Česká republika", "SK": "Slovensko", "DE": "Německo", "AT": "Rakousko", "PL": "Polsko",
	"HU": "Maďarsko", "GB": "Spojené království", "US": "Spojené státy americké", "FR": "Francie",
	"IT": "Itálie", "ES": "Španělsko", "NL": "Nizozemsko", "BE": "Belgie", "IE": "Irsko",
}

func countryName(code string) string {
	if n, ok := countryNames[code]; ok {
		return n
	}
	return code
}

func taxSchemes(vatNo string) []PartyTaxScheme {
	vatNo = strings.TrimSpace(vatNo)
	if vatNo == "" {
		return nil
	}
	return []PartyTaxScheme{{CompanyID: vatNo, TaxScheme: "VAT"}}
}

func preformatted(s string) *RegisterIdentification {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &RegisterIdentification{Preformatted: s}
}

func contact(name, phone, email string) *Contact {
	if name == "" && phone == "" && email == "" {
		return nil
	}
	return &Contact{Name: name, Telephone: phone, ElectronicMail: email}
}
