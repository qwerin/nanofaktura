package reports

// FlatRate is one flat-rate expense option of a sole trader (§ 7 odst. 7
// zákona o daních z příjmů) with its statutory cap.
type FlatRate struct {
	Percent  int   `json:"percent"`
	Cap      int64 `json:"cap" doc:"Statutory maximum of the expenses (haléře)"`
	Expenses int64 `json:"expenses" doc:"min(income × percent, cap)"`
	TaxBase  int64 `json:"tax_base" doc:"income − expenses"`
}

// FlatRateCaps are the caps valid since 2021 (income up to 2 000 000 Kč):
// 80 % → 1 600 000 Kč, 60 % → 1 200 000 Kč, 40 % → 800 000 Kč, 30 % → 600 000 Kč.
var FlatRateCaps = []struct {
	Percent int
	Cap     int64
}{
	{80, 1_600_000_00},
	{60, 1_200_000_00},
	{40, 800_000_00},
	{30, 600_000_00},
}

// FlatRates computes every flat-rate option for income (haléře, without VAT).
// Expenses are rounded to whole haléře; negative income gives zero expenses.
func FlatRates(income int64) []FlatRate {
	out := make([]FlatRate, 0, len(FlatRateCaps))
	for _, c := range FlatRateCaps {
		exp := int64(0)
		if income > 0 {
			exp = min(income*int64(c.Percent)/100, c.Cap)
		}
		out = append(out, FlatRate{Percent: c.Percent, Cap: c.Cap, Expenses: exp, TaxBase: income - exp})
	}
	return out
}
