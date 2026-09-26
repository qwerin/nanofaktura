package billing

import "sort"

// Line is the input of Calculate: one document line (invoice, expense…).
type Line struct {
	QuantityMilli int64 // 1500 = 1.5
	UnitPrice     int64 // minor units; with or without VAT per Options.PricesIncludeVAT
	VatRateBps    int32 // 2100 = 21 %
}

// Options switch the calculation mode of a document.
type Options struct {
	PricesIncludeVAT bool // unit prices are gross (VAT included)
	ReverseCharge    bool // no VAT is charged; rates are kept for display
	RoundTotal       bool // round the total to whole units (100 minor units)
	NonVATPayer      bool // every rate is treated as 0 %
}

// LineAmounts are the computed amounts of one line. For lines they are
// informative; the authoritative VAT is computed per rate in the recap.
type LineAmounts struct {
	Base  int64 // without VAT
	Vat   int64
	Total int64 // with VAT
}

// VatRecap sums one VAT rate.
type VatRecap struct {
	VatRateBps int32
	Base       int64
	Vat        int64
	Total      int64
}

// Totals is the result of Calculate.
type Totals struct {
	Lines    []LineAmounts // same order as the input lines
	VatRecap []VatRecap    // one entry per rate, highest rate first
	Subtotal int64         // Σ recap base
	VatTotal int64         // Σ recap vat
	Rounding int64         // Total − (Subtotal + VatTotal)
	Total    int64
}

// EffectiveRate is the rate actually used for a line: 0 for non-VAT payers.
func EffectiveRate(rate int32, o Options) int32 {
	if o.NonVATPayer {
		return 0
	}
	return rate
}

// Calculate computes line amounts, the VAT recapitulation and document totals
// (SPEC §4.5), using integer arithmetic with half-away-from-zero rounding:
//
//   - net prices: line base = round(unit_price × qty); per rate base = Σ base,
//     vat = round(base × rate / 10000), total = base + vat
//   - gross prices: line total = round(unit_price × qty); per rate total = Σ total,
//     vat = round(total × rate / (10000 + rate)), base = total − vat
//   - reverse charge / non-payer: vat = 0
//   - round_total: total rounded to whole units, the difference is Rounding
//
// It fails only with ErrOverflow when amounts do not fit into int64.
func Calculate(lines []Line, o Options) (Totals, error) {
	var c checked
	t := Totals{Lines: make([]LineAmounts, len(lines)), VatRecap: []VatRecap{}}
	sums := map[int32]int64{} // per rate: Σ base (net prices) or Σ total (gross prices)
	for i, l := range lines {
		rate := EffectiveRate(l.VatRateBps, o)
		amount := c.mulDiv(l.UnitPrice, l.QuantityMilli, 1000)
		t.Lines[i] = c.split(amount, rate, o)
		sums[rate] = c.add(sums[rate], amount)
	}

	rates := make([]int32, 0, len(sums))
	for r := range sums {
		rates = append(rates, r)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] > rates[j] })
	for _, r := range rates {
		la := c.split(sums[r], r, o)
		t.VatRecap = append(t.VatRecap, VatRecap{VatRateBps: r, Base: la.Base, Vat: la.Vat, Total: la.Total})
		t.Subtotal = c.add(t.Subtotal, la.Base)
		t.VatTotal = c.add(t.VatTotal, la.Vat)
	}
	before := c.add(t.Subtotal, t.VatTotal)
	if c.overflow {
		return Totals{}, ErrOverflow
	}
	t.Total = before
	if o.RoundTotal {
		t.Total = RoundTo(before, 100)
	}
	t.Rounding = t.Total - before
	return t, nil
}

// checked accumulates int64 overflow over a sequence of operations.
type checked struct{ overflow bool }

func (c *checked) add(a, b int64) int64 {
	s, ok := addChecked(a, b)
	c.overflow = c.overflow || !ok
	return s
}

func (c *checked) mulDiv(a, b, den int64) int64 {
	v, ok := MulDivRound(a, b, den)
	c.overflow = c.overflow || !ok
	return v
}

// split turns an amount (net or gross per o.PricesIncludeVAT) at rate into
// base/vat/total.
func (c *checked) split(amount int64, rate int32, o Options) LineAmounts {
	var vat int64
	if !o.ReverseCharge && !o.NonVATPayer && rate != 0 {
		den := int64(10000)
		if o.PricesIncludeVAT {
			den += int64(rate)
		}
		vat = c.mulDiv(amount, int64(rate), den)
	}
	if o.PricesIncludeVAT {
		return LineAmounts{Base: amount - vat, Vat: vat, Total: amount}
	}
	return LineAmounts{Base: amount, Vat: vat, Total: c.add(amount, vat)}
}
