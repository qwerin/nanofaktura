package models

import (
	"fmt"
	"testing"
)

func TestRecalculate_NonVatPayer(t *testing.T) {
	inv := Invoice{
		VatExempt: true,
		Lines: []InvoiceLine{
			// 10 hodin × 1 500 Kč = 15 000 Kč
			{Quantity: "10", UnitName: "hod", UnitPriceHal: 150_000, VatRateBps: 0},
			// 1 ks × 500 Kč = 500 Kč
			{Quantity: "1", UnitName: "ks", UnitPriceHal: 50_000, VatRateBps: 0},
		},
	}

	inv.Recalculate()

	want := int64(1_550_000) // 15 000 + 500 Kč v haléřích
	if inv.Total != want {
		t.Errorf("Total = %d, want %d", inv.Total, want)
	}
	if inv.TotalVatHal != 0 {
		t.Errorf("TotalVatHal = %d, want 0 (neplátce DPH)", inv.TotalVatHal)
	}
}

func TestRecalculate_VatPayer21(t *testing.T) {
	inv := Invoice{
		VatExempt: false,
		Lines: []InvoiceLine{
			// 1 ks × 1 000 Kč základ, DPH 21 % → celkem 1 210 Kč
			{Quantity: "1", UnitName: "ks", UnitPriceHal: 100_000, VatRateBps: 2100},
		},
	}

	inv.Recalculate()

	line := inv.Lines[0]
	if line.TotalPriceWithoutVatHal != 100_000 {
		t.Errorf("TotalPriceWithoutVatHal = %d, want 100_000", line.TotalPriceWithoutVatHal)
	}
	if line.TotalVatHal != 21_000 {
		t.Errorf("TotalVatHal = %d, want 21_000", line.TotalVatHal)
	}
	if line.TotalHal != 121_000 {
		t.Errorf("line.TotalHal = %d, want 121_000", line.TotalHal)
	}
	if inv.Total != 121_000 {
		t.Errorf("inv.Total = %d, want 121_000", inv.Total)
	}
}

func TestRecalculate_FractionalQuantity(t *testing.T) {
	inv := Invoice{
		VatExempt: true,
		Lines: []InvoiceLine{
			// 1.5 hod × 2 000 Kč = 3 000 Kč
			{Quantity: "1.5", UnitName: "hod", UnitPriceHal: 200_000, VatRateBps: 0},
		},
	}

	inv.Recalculate()

	want := int64(300_000)
	if inv.Total != want {
		t.Errorf("Total = %d, want %d", inv.Total, want)
	}
}

func TestFormatHal(t *testing.T) {
	cases := []struct {
		input int64
		want  string
	}{
		{150_000, "1500,00"},
		{150_050, "1500,50"},
		{100, "1,00"},
		{0, "0,00"},
	}

	for _, tc := range cases {
		got := formatHalStr(tc.input)
		if got != tc.want {
			t.Errorf("formatHal(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// formatHalStr je kopie pdf.formatHal – testujeme logiku bez importu PDF balíčku.
func formatHalStr(hal int64) string {
	kc := hal / 100
	haler := hal % 100
	if haler < 0 {
		haler = -haler
	}
	return fmt.Sprintf("%d,%02d", kc, haler)
}
