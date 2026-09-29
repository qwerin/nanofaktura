package reports

import (
	"fmt"
	"slices"
	"strings"
)

// Statutory VAT rates in basis points (since 2024).
const (
	RateBasic   int32 = 2100
	RateReduced int32 = 1200
)

// ControlThreshold: documents above 10 000 Kč including VAT are reported
// one by one in the control statement (A.4 / B.2), the rest in aggregate.
const ControlThreshold int64 = 10_000_00

// Pair is a tax base with its VAT.
type Pair struct {
	Base int64 `json:"base"`
	Vat  int64 `json:"vat"`
}

func (p *Pair) add(o Pair) { p.Base += o.Base; p.Vat += o.Vat }

// RateSums are amounts split by the basic and the reduced rate.
type RateSums struct {
	Basic   Pair `json:"basic" doc:"21 %"`
	Reduced Pair `json:"reduced" doc:"12 %"`
}

func (r *RateSums) add(o RateSums) { r.Basic.add(o.Basic); r.Reduced.add(o.Reduced) }

// RateAmount is one rate of a document's VAT recapitulation (CZK).
type RateAmount struct {
	RateBps int32
	Base    int64
	Vat     int64
}

// Sale is an issued tax document (invoice or correction) converted to CZK.
type Sale struct {
	Number          string
	TaxPointDate    string // DUZP
	CustomerVatNo   string
	CustomerCountry string
	ReverseCharge   bool
	Recap           []RateAmount
	Total           int64 // incl. VAT
	// ControlTotal decides between A.4 and A.5; for a correction it is the
	// total of the corrected invoice (a correction follows its original).
	ControlTotal int64
}

// Purchase is a received tax document (expense) converted to CZK.
type Purchase struct {
	Number        string // the supplier's document number (evidence number)
	TaxPointDate  string
	SupplierVatNo string
	Recap         []RateAmount
	Total         int64
}

// VatReturn are the rows of the VAT return (přiznání k DPH) this
// application can derive, in haléře; the EPO export rounds to whole Kč.
type VatReturn struct {
	R1  Pair  `json:"r1" doc:"ř. 1: taxable supplies at the basic rate"`
	R2  Pair  `json:"r2" doc:"ř. 2: taxable supplies at the reduced rate"`
	R21 int64 `json:"r21" doc:"ř. 21: services with place of supply in another EU member state (§ 102)"`
	R25 int64 `json:"r25" doc:"ř. 25: domestic reverse-charge supplies (§ 92a), supplier"`
	R26 int64 `json:"r26" doc:"ř. 26: other supplies with right to deduction (e.g. services outside the EU)"`
	R40 Pair  `json:"r40" doc:"ř. 40: received taxable supplies at the basic rate (full deduction)"`
	R41 Pair  `json:"r41" doc:"ř. 41: received taxable supplies at the reduced rate (full deduction)"`
	R46 int64 `json:"r46" doc:"ř. 46: total deduction in full"`
	R62 int64 `json:"r62" doc:"ř. 62: output tax"`
	R63 int64 `json:"r63" doc:"ř. 63: deduction"`
	R64 int64 `json:"r64" doc:"ř. 64: own tax (62 − 63 when positive)"`
	R65 int64 `json:"r65" doc:"ř. 65: excess deduction (63 − 62 when positive)"`
}

// A1Row: domestic reverse-charge supply (§ 92a) to a VAT payer.
type A1Row struct {
	CustomerVatNo string `json:"customer_vat_no"`
	Number        string `json:"number"`
	TaxPointDate  string `json:"taxable_fulfillment_due"`
	Base          int64  `json:"base"`
}

// DocumentRow is one document of section A.4 (issued) or B.2 (received).
type DocumentRow struct {
	VatNo        string `json:"vat_no" doc:"Customer (A.4) or supplier (B.2) DIČ"`
	Number       string `json:"number"`
	TaxPointDate string `json:"taxable_fulfillment_due"`
	RateSums
}

// ControlStatement is the VAT control statement (kontrolní hlášení).
type ControlStatement struct {
	A1 []A1Row       `json:"a1" nullable:"false"`
	A4 []DocumentRow `json:"a4" nullable:"false"`
	A5 RateSums      `json:"a5"`
	B2 []DocumentRow `json:"b2" nullable:"false"`
	B3 RateSums      `json:"b3"`
}

// VatReport is the VAT return and control statement of a period.
type VatReport struct {
	Period   Period
	Return   VatReturn
	Control  ControlStatement
	Warnings []Warning
}

// Warning codes of the VAT report (clients translate them; Message is English).
const (
	WarnUnsupportedRate      = "unsupported_rate"         // params: rate (percent)
	WarnReverseChargeNoDIC   = "reverse_charge_no_dic"    // domestic reverse charge, customer without CZ DIČ
	WarnEUReverseChargeNoVat = "eu_reverse_charge_no_vat" // EU reverse charge, customer without VAT number
	WarnZeroRateNotReported  = "zero_rate_not_reported"   // params: amount (Kč)
	WarnSupplierNoDIC        = "supplier_no_dic"          // deduction skipped
	WarnCalculation          = "calculation_error"        // the document could not be recalculated
)

// Warning is a document the report could not classify fully.
type Warning struct {
	Code     string            `json:"code" enum:"unsupported_rate,reverse_charge_no_dic,eu_reverse_charge_no_vat,zero_rate_not_reported,supplier_no_dic,calculation_error"`
	Document string            `json:"document" doc:"Document number"`
	Message  string            `json:"message" doc:"English description"`
	Params   map[string]string `json:"params,omitempty" doc:"Values for the translated text (rate, amount)"`
}

// NewVatReport starts an empty report of p.
func NewVatReport(p Period) *VatReport {
	return &VatReport{Period: p, Control: ControlStatement{A1: []A1Row{}, A4: []DocumentRow{}, B2: []DocumentRow{}}, Warnings: []Warning{}}
}

// EUCountries are the EU member states (ISO codes; Greece also as EL).
var EUCountries = []string{
	"AT", "BE", "BG", "CY", "CZ", "DE", "DK", "EE", "EL", "GR", "ES", "FI", "FR", "HR", "HU", "IE",
	"IT", "LT", "LU", "LV", "MT", "NL", "PL", "PT", "RO", "SE", "SI", "SK",
}

// splitRates sums a recap into basic/reduced; other non-zero rates are
// reported as a warning and zero-rate amounts are returned separately.
func (r *VatReport) splitRates(doc string, recap []RateAmount) (sums RateSums, zero int64) {
	for _, a := range recap {
		switch a.RateBps {
		case RateBasic:
			sums.Basic.add(Pair{a.Base, a.Vat})
		case RateReduced:
			sums.Reduced.add(Pair{a.Base, a.Vat})
		case 0:
			zero += a.Base
		default:
			r.Warn(WarnUnsupportedRate, doc, map[string]string{"rate": pct(a.RateBps)},
				"VAT rate %s %% is not reported (only 21 %% and 12 %%)", pct(a.RateBps))
		}
	}
	return sums, zero
}

// Warn records a warning about document doc.
func (r *VatReport) Warn(code, doc string, params map[string]string, format string, args ...any) {
	r.Warnings = append(r.Warnings, Warning{Code: code, Document: doc, Params: params,
		Message: doc + ": " + fmt.Sprintf(format, args...)})
}

// AddSale adds an issued tax document whose DUZP is in the period.
//
//   - reverse charge, customer in CZ → ř. 25 + A.1
//   - reverse charge, customer in another EU state with a VAT number → ř. 21
//     (services; goods deliveries ř. 20 are not distinguished)
//   - no VAT charged, customer outside the EU → ř. 26
//   - otherwise VAT by rate → ř. 1 / ř. 2 and A.4 (customer with a CZ DIČ and
//     ControlTotal > 10 000 Kč) or A.5
func (r *VatReport) AddSale(s Sale) {
	country := strings.ToUpper(strings.TrimSpace(s.CustomerCountry))
	if country == "" {
		country = "CZ"
	}
	vatNo := normVatNo(s.CustomerVatNo)
	var base int64
	for _, a := range s.Recap {
		base += a.Base
	}
	switch {
	case s.ReverseCharge && country == "CZ":
		r.Return.R25 += base
		dic, ok := czDIC(vatNo)
		if !ok {
			r.Warn(WarnReverseChargeNoDIC, s.Number, nil, "reverse charge without the customer's CZ DIČ")
		}
		r.Control.A1 = append(r.Control.A1, A1Row{CustomerVatNo: dic, Number: s.Number, TaxPointDate: s.TaxPointDate, Base: base})
		return
	case s.ReverseCharge && slices.Contains(EUCountries, country):
		if vatNo == "" {
			r.Warn(WarnEUReverseChargeNoVat, s.Number, nil, "EU reverse charge without the customer's VAT number")
		}
		r.Return.R21 += base
		return
	case !slices.Contains(EUCountries, country) && vatOf(s.Recap) == 0:
		r.Return.R26 += base
		return
	}

	sums, zero := r.splitRates(s.Number, s.Recap)
	if zero != 0 {
		r.Warn(WarnZeroRateNotReported, s.Number, map[string]string{"amount": kc(zero)}, "amount without VAT (%s Kč) is not reported", kc(zero))
	}
	r.Return.R1.add(sums.Basic)
	r.Return.R2.add(sums.Reduced)
	control := s.ControlTotal
	if control == 0 {
		control = s.Total
	}
	if dic, ok := czDIC(vatNo); ok && abs(control) > ControlThreshold {
		r.Control.A4 = append(r.Control.A4, DocumentRow{VatNo: dic, Number: s.Number, TaxPointDate: s.TaxPointDate, RateSums: sums})
	} else {
		r.Control.A5.add(sums)
	}
}

// AddPurchase adds a received tax document with a right to deduction whose
// DUZP is in the period: ř. 40 / 41 and B.2 (supplier with a CZ DIČ and
// total > 10 000 Kč) or B.3. Purchases from suppliers without a CZ DIČ are
// skipped with a warning (acquisitions from abroad are not supported).
func (r *VatReport) AddPurchase(p Purchase) {
	dic, ok := czDIC(normVatNo(p.SupplierVatNo))
	if !ok {
		if vatOf(p.Recap) != 0 {
			r.Warn(WarnSupplierNoDIC, p.Number, nil, "deduction skipped, the supplier has no CZ DIČ")
		}
		return
	}
	sums, _ := r.splitRates(p.Number, p.Recap)
	if sums.Basic.Vat == 0 && sums.Reduced.Vat == 0 {
		return // no VAT charged — nothing to deduct
	}
	r.Return.R40.add(sums.Basic)
	r.Return.R41.add(sums.Reduced)
	if abs(p.Total) > ControlThreshold {
		r.Control.B2 = append(r.Control.B2, DocumentRow{VatNo: dic, Number: p.Number, TaxPointDate: p.TaxPointDate, RateSums: sums})
	} else {
		r.Control.B3.add(sums)
	}
}

// Finish computes the summary rows (46, 62–65).
func (r *VatReport) Finish() {
	v := &r.Return
	v.R46 = v.R40.Vat + v.R41.Vat
	v.R62 = v.R1.Vat + v.R2.Vat
	v.R63 = v.R46
	v.R64, v.R65 = 0, 0
	if d := v.R62 - v.R63; d >= 0 {
		v.R64 = d
	} else {
		v.R65 = -d
	}
}

// ---- helpers ----

func normVatNo(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// czDIC returns the numeric part of a Czech DIČ ("CZ12345678" → "12345678").
func czDIC(vatNo string) (string, bool) {
	d, ok := strings.CutPrefix(vatNo, "CZ")
	if !ok || len(d) < 8 || len(d) > 10 {
		return "", false
	}
	for _, c := range d {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return d, true
}

func vatOf(recap []RateAmount) int64 {
	var v int64
	for _, a := range recap {
		v += a.Vat
	}
	return v
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func pct(bps int32) string {
	s := fmt.Sprintf("%d.%02d", bps/100, bps%100)
	return strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
}

// kc formats haléře as "1234.50".
func kc(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}
