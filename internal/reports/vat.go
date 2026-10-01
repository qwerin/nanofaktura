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

// Sale is an issued tax document (invoice, correction, tax document for a
// received payment) converted to CZK.
type Sale struct {
	Number          string
	TaxPointDate    string // DUZP
	CustomerVatNo   string
	CustomerCountry string
	// CustomerLocalVatNo is the customer's local VAT number (Slovak IČ DPH);
	// for EU supplies it is the VAT ID when set.
	CustomerLocalVatNo string
	ReverseCharge      bool
	SupplyType         string // EU reverse charge: "goods" (§ 64, ř. 20) or services (ř. 21)
	Recap              []RateAmount
	Total              int64 // incl. VAT
	// ControlTotal decides between A.4 and A.5; for a correction it is the
	// total of the corrected invoice (a correction follows its original).
	ControlTotal int64
}

// Purchase is a received tax document (expense) converted to CZK.
type Purchase struct {
	Number          string // the supplier's document number (evidence number)
	TaxPointDate    string
	SupplierVatNo   string
	SupplierCountry string
	Recap           []RateAmount
	Total           int64
	// Deductible: the VAT deduction is claimed (ř. 40/41, 43/44).
	Deductible bool
	// ReverseCharge: the recipient self-assesses the VAT (Recap carries the
	// recipient's rates with zero VAT): EU goods (ř. 3/4) or services (ř. 5/6)
	// and A.2, domestic § 92a (ř. 10/11, B.1), services from outside the EU
	// (ř. 12/13, A.2); the deduction goes to ř. 43/44.
	ReverseCharge bool
	SupplyType    string // "goods" or services
}

// VatReturn are the rows of the VAT return (přiznání k DPH) this
// application can derive, in haléře; the EPO export rounds to whole Kč.
type VatReturn struct {
	R1  Pair  `json:"r1" doc:"ř. 1: taxable supplies at the basic rate"`
	R2  Pair  `json:"r2" doc:"ř. 2: taxable supplies at the reduced rate"`
	R3  Pair  `json:"r3" doc:"ř. 3: goods acquired from another EU member state, basic rate"`
	R4  Pair  `json:"r4" doc:"ř. 4: goods acquired from another EU member state, reduced rate"`
	R5  Pair  `json:"r5" doc:"ř. 5: services received from a person registered in another EU member state (§ 9/1), basic rate"`
	R6  Pair  `json:"r6" doc:"ř. 6: the same, reduced rate"`
	R10 Pair  `json:"r10" doc:"ř. 10: domestic reverse charge (§ 92a), recipient, basic rate"`
	R11 Pair  `json:"r11" doc:"ř. 11: domestic reverse charge (§ 92a), recipient, reduced rate"`
	R12 Pair  `json:"r12" doc:"ř. 12: other supplies taxed by the recipient (services from outside the EU), basic rate"`
	R13 Pair  `json:"r13" doc:"ř. 13: the same, reduced rate"`
	R20 int64 `json:"r20" doc:"ř. 20: exempt supply of goods to another EU member state (§ 64)"`
	R21 int64 `json:"r21" doc:"ř. 21: services with place of supply in another EU member state (§ 102)"`
	R25 int64 `json:"r25" doc:"ř. 25: domestic reverse-charge supplies (§ 92a), supplier"`
	R26 int64 `json:"r26" doc:"ř. 26: other supplies with right to deduction (e.g. services outside the EU)"`
	R40 Pair  `json:"r40" doc:"ř. 40: received taxable supplies at the basic rate (full deduction)"`
	R41 Pair  `json:"r41" doc:"ř. 41: received taxable supplies at the reduced rate (full deduction)"`
	R43 Pair  `json:"r43" doc:"ř. 43: deduction of the VAT self-assessed in ř. 3–13, basic rate"`
	R44 Pair  `json:"r44" doc:"ř. 44: deduction of the VAT self-assessed in ř. 3–13, reduced rate"`
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

// A2Row: received supply taxed by the recipient from a foreign person (EU
// goods/services, services from outside the EU).
type A2Row struct {
	Country      string `json:"country" doc:"k_stat: the supplier's country code"`
	VatID        string `json:"vat_id" doc:"vatid_dod: the supplier's VAT ID without the country prefix"`
	Number       string `json:"number"`
	TaxPointDate string `json:"taxable_fulfillment_due"`
	RateSums
}

// B1Row: received domestic reverse-charge supply (§ 92a), recipient.
type B1Row struct {
	SupplierVatNo string `json:"supplier_vat_no"`
	Number        string `json:"number"`
	TaxPointDate  string `json:"taxable_fulfillment_due"`
	RateSums
}

// ControlStatement is the VAT control statement (kontrolní hlášení).
type ControlStatement struct {
	A1 []A1Row       `json:"a1" nullable:"false"`
	A2 []A2Row       `json:"a2" nullable:"false"`
	A4 []DocumentRow `json:"a4" nullable:"false"`
	A5 RateSums      `json:"a5"`
	B1 []B1Row       `json:"b1" nullable:"false"`
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
	WarnUnsupportedRate      = "unsupported_rate"            // params: rate (percent)
	WarnReverseChargeNoDIC   = "reverse_charge_no_dic"       // domestic reverse charge, customer without CZ DIČ
	WarnEUReverseChargeNoVat = "eu_reverse_charge_no_vat"    // EU reverse charge, customer without VAT number
	WarnZeroRateNotReported  = "zero_rate_not_reported"      // params: amount (Kč)
	WarnSupplierNoDIC        = "supplier_no_dic"             // deduction skipped
	WarnCalculation          = "calculation_error"           // the document could not be recalculated
	WarnMissingTaxPointDate  = "missing_taxable_date"        // tax document without DUZP, reported by its issue date
	WarnPossibleRC           = "possible_reverse_charge"     // purchase without VAT that may have to be self-assessed
	WarnRCImportGoods        = "reverse_charge_import"       // goods from outside the EU: import VAT is not supported
	WarnRCSubjectCode        = "reverse_charge_subject_code" // § 92a: the subject code (kod_pred_pl) must be completed
	WarnECSalesList          = "ec_sales_list"               // EU supplies: file the EC Sales List (souhrnné hlášení)
	WarnCorrectionCancelled  = "correction_of_cancelled"     // a correction of a cancelled invoice
	WarnMonthlyControl       = "control_statement_monthly"   // legal person: control statement per month
)

// Warning is a document the report could not classify fully.
type Warning struct {
	Code     string            `json:"code" enum:"unsupported_rate,reverse_charge_no_dic,eu_reverse_charge_no_vat,zero_rate_not_reported,supplier_no_dic,calculation_error,missing_taxable_date,possible_reverse_charge,reverse_charge_import,reverse_charge_subject_code,ec_sales_list,correction_of_cancelled,control_statement_monthly"`
	Document string            `json:"document" doc:"Document number"`
	Message  string            `json:"message" doc:"English description"`
	Params   map[string]string `json:"params,omitempty" doc:"Values for the translated text (rate, amount)"`
}

// NewVatReport starts an empty report of p.
func NewVatReport(p Period) *VatReport {
	return &VatReport{Period: p, Control: ControlStatement{A1: []A1Row{}, A2: []A2Row{}, A4: []DocumentRow{}, B1: []B1Row{},
		B2: []DocumentRow{}}, Warnings: []Warning{}}
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
//   - reverse charge, customer in another EU state → ř. 20 (goods, § 64) or
//     ř. 21 (services); the customer's VAT ID must carry its country prefix
//   - no VAT charged, customer outside the EU → ř. 26
//   - otherwise VAT by rate → ř. 1 / ř. 2 and A.4 (customer with a CZ DIČ and
//     ControlTotal > 10 000 Kč) or A.5
func (r *VatReport) AddSale(s Sale) {
	country := strings.ToUpper(strings.TrimSpace(s.CustomerCountry))
	if country == "" {
		country = "CZ"
	}
	vatNo := normVatNo(s.CustomerVatNo)
	if l := normVatNo(s.CustomerLocalVatNo); l != "" && country != "CZ" {
		vatNo = l // e.g. the Slovak IČ DPH "SK…" — the VAT ID for EU supplies
	}
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
		if !hasCountryPrefix(vatNo) {
			r.Warn(WarnEUReverseChargeNoVat, s.Number, nil, "EU reverse charge without the customer's VAT ID (with the country prefix)")
		}
		if s.SupplyType == "goods" {
			r.Return.R20 += base
		} else {
			r.Return.R21 += base
		}
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

// AddPurchase adds a received document whose DUZP is in the period.
//
//   - reverse charge: see Purchase.ReverseCharge (AddSelfAssessed);
//   - deductible with VAT from a supplier with a CZ DIČ: ř. 40 / 41 and B.2
//     (total > 10 000 Kč) or B.3;
//   - without VAT from a foreign supplier, or from a CZ supplier: a warning
//     that the VAT may have to be self-assessed;
//   - VAT from a supplier without a CZ DIČ: skipped with a warning.
func (r *VatReport) AddPurchase(p Purchase) {
	if p.ReverseCharge {
		r.addSelfAssessed(p)
		return
	}
	if !p.Deductible {
		return
	}
	vatNo := normVatNo(p.SupplierVatNo)
	country := supplierCountry(p.SupplierCountry, vatNo)
	if vatOf(p.Recap) == 0 {
		var base int64
		for _, a := range p.Recap {
			base += a.Base
		}
		if base != 0 && (country != "CZ" || hasCountryPrefix(vatNo)) {
			r.Warn(WarnPossibleRC, p.Number, nil,
				"no VAT charged by the supplier (%s): if the VAT has to be self-assessed (services or goods from abroad, § 92a), mark the expense as reverse charge", country)
		}
		return // no VAT charged — nothing to deduct
	}
	dic, ok := czDIC(vatNo)
	if !ok {
		r.Warn(WarnSupplierNoDIC, p.Number, nil, "deduction skipped, the supplier has no CZ DIČ")
		return
	}
	sums, _ := r.splitRates(p.Number, p.Recap)
	if sums.Basic.Vat == 0 && sums.Reduced.Vat == 0 {
		return
	}
	r.Return.R40.add(sums.Basic)
	r.Return.R41.add(sums.Reduced)
	if abs(p.Total) > ControlThreshold {
		r.Control.B2 = append(r.Control.B2, DocumentRow{VatNo: dic, Number: p.Number, TaxPointDate: p.TaxPointDate, RateSums: sums})
	} else {
		r.Control.B3.add(sums)
	}
}

// addSelfAssessed adds a received supply on which the recipient pays the
// VAT: the VAT at the recipient's rates (the Recap carries bases), output
// rows 3–13 + A.2 / B.1 and, when deductible, the deduction ř. 43/44.
func (r *VatReport) addSelfAssessed(p Purchase) {
	vatNo := normVatNo(p.SupplierVatNo)
	country := supplierCountry(p.SupplierCountry, vatNo)
	recap := make([]RateAmount, len(p.Recap))
	for i, a := range p.Recap {
		recap[i] = RateAmount{RateBps: a.RateBps, Base: a.Base, Vat: divRound(a.Base*int64(a.RateBps), 10000)}
	}
	sums, zero := r.splitRates(p.Number, recap)
	if zero != 0 {
		r.Warn(WarnZeroRateNotReported, p.Number, map[string]string{"amount": kc(zero)}, "amount without VAT (%s Kč) is not reported", kc(zero))
	}
	goods := p.SupplyType == "goods"
	switch {
	case country == "CZ":
		r.Return.R10.add(sums.Basic)
		r.Return.R11.add(sums.Reduced)
		dic, ok := czDIC(vatNo)
		if !ok {
			r.Warn(WarnReverseChargeNoDIC, p.Number, nil, "domestic reverse charge without the supplier's CZ DIČ")
		}
		r.Warn(WarnRCSubjectCode, p.Number, nil, "complete the subject code of the § 92a supply (kod_pred_pl) in section B.1 of the control statement")
		r.Control.B1 = append(r.Control.B1, B1Row{SupplierVatNo: dic, Number: p.Number, TaxPointDate: p.TaxPointDate, RateSums: sums})
	case slices.Contains(EUCountries, country):
		if goods {
			r.Return.R3.add(sums.Basic)
			r.Return.R4.add(sums.Reduced)
		} else {
			r.Return.R5.add(sums.Basic)
			r.Return.R6.add(sums.Reduced)
		}
		r.Control.A2 = append(r.Control.A2, a2Row(country, vatNo, p, sums))
	default:
		if goods {
			r.Warn(WarnRCImportGoods, p.Number, nil, "goods from outside the EU are an import (VAT assessed by customs) — not reported")
			return
		}
		r.Return.R12.add(sums.Basic)
		r.Return.R13.add(sums.Reduced)
		r.Control.A2 = append(r.Control.A2, a2Row(country, vatNo, p, sums))
	}
	if p.Deductible {
		r.Return.R43.add(sums.Basic)
		r.Return.R44.add(sums.Reduced)
	}
}

func a2Row(country, vatNo string, p Purchase, sums RateSums) A2Row {
	id := vatNo
	if hasCountryPrefix(id) {
		id = id[2:]
	}
	return A2Row{Country: country, VatID: id, Number: p.Number, TaxPointDate: p.TaxPointDate, RateSums: sums}
}

// Finish computes the summary rows (46, 62–65).
func (r *VatReport) Finish() {
	v := &r.Return
	v.R46 = v.R40.Vat + v.R41.Vat + v.R43.Vat + v.R44.Vat
	v.R62 = v.R1.Vat + v.R2.Vat + v.R3.Vat + v.R4.Vat + v.R5.Vat + v.R6.Vat + v.R10.Vat + v.R11.Vat + v.R12.Vat + v.R13.Vat
	v.R63 = v.R46
	v.R64, v.R65 = 0, 0
	if d := v.R62 - v.R63; d >= 0 {
		v.R64 = d
	} else {
		v.R65 = -d
	}
}

// ---- helpers ----

// hasCountryPrefix: a VAT ID with a two-letter country prefix ("SK2020317068").
func hasCountryPrefix(v string) bool {
	return len(v) > 2 && v[0] >= 'A' && v[0] <= 'Z' && v[1] >= 'A' && v[1] <= 'Z'
}

// supplierCountry is the supplier's country: the stored country, else the
// VAT ID prefix ("EL" → Greece), else CZ.
func supplierCountry(country, vatNo string) string {
	if c := strings.ToUpper(strings.TrimSpace(country)); c != "" {
		return c
	}
	if hasCountryPrefix(vatNo) {
		return vatNo[:2]
	}
	return "CZ"
}

// divRound divides half away from zero.
func divRound(a, b int64) int64 {
	if (a < 0) != (b < 0) {
		return -((abs(a) + abs(b)/2) / abs(b))
	}
	return (abs(a) + abs(b)/2) / abs(b)
}

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
