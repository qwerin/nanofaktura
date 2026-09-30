package billing

import (
	"math/rand/v2"
	"testing"
)

var propRates = []int32{0, 1000, 1200, 1500, 2100}

func randomLines(rng *rand.Rand) []Line {
	n := 1 + rng.IntN(12)
	ls := make([]Line, n)
	for i := range ls {
		ls[i] = Line{
			QuantityMilli: int64(1 + rng.IntN(100_000)),    // 0.001 … 100
			UnitPrice:     int64(rng.IntN(10_000_000)) - 5, // up to 100 000 Kč, occasionally negative
			VatRateBps:    propRates[rng.IntN(len(propRates))],
		}
	}
	return ls
}

func randomOptions(rng *rand.Rand) Options {
	return Options{
		PricesIncludeVAT: rng.IntN(2) == 0,
		ReverseCharge:    rng.IntN(6) == 0,
		RoundTotal:       rng.IntN(2) == 0,
		NonVATPayer:      rng.IntN(6) == 0,
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestCalculateInvariants checks, for many random documents, the invariants
// every document must satisfy regardless of options.
func TestCalculateInvariants(t *testing.T) {
	rng := rand.New(rand.NewPCG(2026, 3))
	for iter := range 5000 {
		lines, o := randomLines(rng), randomOptions(rng)
		tot, err := Calculate(lines, o)
		if err != nil {
			t.Fatalf("#%d: %v", iter, err)
		}
		fail := func(msg string, args ...any) {
			t.Helper()
			t.Fatalf("#%d %+v %+v: "+msg+"\ntotals %+v", append(append([]any{iter, o, lines}, args...), tot)...)
		}
		if len(tot.Lines) != len(lines) {
			fail("line count")
		}

		// recap sums are the totals; rates unique, highest first
		var base, vat, total int64
		for i, r := range tot.VatRecap {
			base += r.Base
			vat += r.Vat
			total += r.Total
			if r.Base+r.Vat != r.Total {
				fail("recap %d: base+vat != total", i)
			}
			if i > 0 && tot.VatRecap[i-1].VatRateBps <= r.VatRateBps {
				fail("recap not strictly descending")
			}
		}
		if base != tot.Subtotal || vat != tot.VatTotal || total != tot.Subtotal+tot.VatTotal {
			fail("recap sums %d/%d/%d", base, vat, total)
		}
		if tot.Total != tot.Subtotal+tot.VatTotal+tot.Rounding {
			fail("total != subtotal + vat + rounding")
		}

		// rounding of the total
		if o.RoundTotal {
			if tot.Total%100 != 0 || abs(tot.Rounding) > 50 {
				fail("round_total: total %d rounding %d", tot.Total, tot.Rounding)
			}
		} else if tot.Rounding != 0 {
			fail("rounding without round_total")
		}

		// no VAT charged
		if (o.ReverseCharge || o.NonVATPayer) && tot.VatTotal != 0 {
			fail("VAT charged with reverse charge / non-payer")
		}
		if o.NonVATPayer && (len(tot.VatRecap) > 1 || (len(tot.VatRecap) == 1 && tot.VatRecap[0].VatRateBps != 0)) {
			fail("non-payer recap must have only the 0 %% rate")
		}

		// per rate: recap is the exact sum of lines in the price basis, and its
		// VAT is the correctly rounded VAT of that sum (error ≤ half a unit)
		perRate := map[int32]struct{ amount, lineVat, n int64 }{}
		for i, l := range lines {
			r := EffectiveRate(l.VatRateBps, o)
			la := tot.Lines[i]
			if la.Base+la.Vat != la.Total {
				fail("line %d: base+vat != total", i)
			}
			s := perRate[r]
			if o.PricesIncludeVAT {
				s.amount += la.Total
			} else {
				s.amount += la.Base
			}
			s.lineVat += la.Vat
			s.n++
			perRate[r] = s
		}
		for _, rc := range tot.VatRecap {
			s := perRate[rc.VatRateBps]
			got := rc.Base
			if o.PricesIncludeVAT {
				got = rc.Total
			}
			if got != s.amount {
				fail("rate %d: recap amount %d != Σ lines %d", rc.VatRateBps, got, s.amount)
			}
			if o.ReverseCharge || o.NonVATPayer || rc.VatRateBps == 0 {
				continue
			}
			rate := int64(rc.VatRateBps)
			den := int64(10000)
			if o.PricesIncludeVAT {
				den += rate
			}
			// |vat·den − amount·rate| ≤ den/2
			if 2*abs(rc.Vat*den-s.amount*rate) > den {
				fail("rate %d: VAT %d not the rounded VAT of %d", rate, rc.Vat, s.amount)
			}
			// Σ of per-line VATs differs from the recap VAT by at most half a unit per line
			if 2*abs(s.lineVat-rc.Vat) > s.n+1 {
				fail("rate %d: Σ line VAT %d vs recap %d (%d lines)", rate, s.lineVat, rc.Vat, s.n)
			}
		}

		// line order does not matter
		shuffled := append([]Line(nil), lines...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		tot2, _ := Calculate(shuffled, o)
		if tot2.Total != tot.Total || tot2.VatTotal != tot.VatTotal || tot2.Subtotal != tot.Subtotal {
			fail("order dependent: %+v", tot2)
		}

		// negating every price negates every amount (half away from zero is symmetric)
		neg := append([]Line(nil), lines...)
		for i := range neg {
			neg[i].UnitPrice = -neg[i].UnitPrice
		}
		tn, _ := Calculate(neg, o)
		if tn.Total != -tot.Total || tn.VatTotal != -tot.VatTotal || tn.Subtotal != -tot.Subtotal || tn.Rounding != -tot.Rounding {
			fail("not symmetric: %+v", tn)
		}
	}
}

// TestGrossNetConsistency: a gross price split into base + VAT, priced again
// as net, gives back the gross price within one minor unit.
func TestGrossNetConsistency(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for range 20000 {
		rate := propRates[rng.IntN(len(propRates))]
		gross := int64(rng.IntN(100_000_000))
		g, err := Calculate([]Line{{QuantityMilli: 1000, UnitPrice: gross, VatRateBps: rate}}, Options{PricesIncludeVAT: true})
		if err != nil {
			t.Fatal(err)
		}
		if g.Total != gross || g.Subtotal+g.VatTotal != gross {
			t.Fatalf("gross %d @%d: %+v", gross, rate, g)
		}
		n, _ := Calculate([]Line{{QuantityMilli: 1000, UnitPrice: g.Subtotal, VatRateBps: rate}}, Options{})
		if abs(n.Total-gross) > 1 {
			t.Fatalf("gross %d @%d → base %d → net total %d (diff > 1)", gross, rate, g.Subtotal, n.Total)
		}
	}
}

func TestCalculateOverflowCases(t *testing.T) {
	const big = int64(1) << 62
	for name, lines := range map[string][]Line{
		"line":  {{QuantityMilli: 1_000_000, UnitPrice: big}},
		"sum":   {{QuantityMilli: 1000, UnitPrice: big}, {QuantityMilli: 1000, UnitPrice: big}},
		"vat":   {{QuantityMilli: 1000, UnitPrice: 8_000_000_000_000_000_000, VatRateBps: 2100}},
		"total": {{QuantityMilli: 1000, UnitPrice: big, VatRateBps: 2100}, {QuantityMilli: 1000, UnitPrice: big, VatRateBps: 1200}},
	} {
		if _, err := Calculate(lines, Options{}); err != ErrOverflow {
			t.Errorf("%s: err = %v, want ErrOverflow", name, err)
		}
	}
	if tot, err := Calculate(nil, Options{RoundTotal: true}); err != nil || tot.Total != 0 || tot.VatRecap == nil || len(tot.VatRecap) != 0 {
		t.Fatalf("empty document: %+v %v", tot, err)
	}
}
