package backup

import (
	"testing"

	"github.com/qwerin/nanofaktura/internal/billing"
)

// Money audit M-07: imported totals and paid amounts follow the lines and payments.
func TestRecomputeTotals(t *testing.T) {
	lines := []Line{{QuantityMilli: 2000, UnitPrice: 50_000, VatRateBps: 2100, Base: 1, Vat: 1, Total: 2}}
	payments := []Payment{{Amount: 60_000}, {Amount: 61_000}}
	sub, vat, round, total, paid := int64(100_000), int64(21_000), int64(0), int64(999_999), int64(5)
	if !recompute(lines, payments, billing.Options{}, &sub, &vat, &round, &total, &paid) {
		t.Fatal("tampered totals not detected")
	}
	if sub != 100_000 || vat != 21_000 || total != 121_000 || paid != 121_000 || lines[0].Total != 121_000 {
		t.Fatalf("recomputed %d %d %d %d %+v", sub, vat, total, paid, lines[0])
	}
	if recompute(lines, payments, billing.Options{}, &sub, &vat, &round, &total, &paid) {
		t.Fatal("consistent document reported as changed")
	}
}
