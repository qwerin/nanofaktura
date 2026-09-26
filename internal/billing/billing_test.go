package billing

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/model"
)

func TestParseQuantity(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		err  bool
	}{
		{"1", 1000, false},
		{"1.5", 1500, false},
		{"1,5", 1500, false},
		{" 2.25 ", 2250, false},
		{"0.001", 1, false},
		{"0,125", 125, false},
		{"-1", -1000, false},
		{"-0.5", -500, false},
		{"+3", 3000, false},
		{"0", 0, false},
		{"-0", 0, false},
		{"007.10", 7100, false},
		{"999999999999.999", 999999999999999, false},
		{"", 0, true},
		{"-", 0, true},
		{"1.", 0, true},
		{".5", 0, true},
		{"1.2345", 0, true},
		{"1.2.3", 0, true},
		{"1,2,3", 0, true},
		{"1.5,", 0, true},
		{"abc", 0, true},
		{"1e3", 0, true},
		{"--1", 0, true},
		{"1 000", 0, true},
		{"1000000000000", 0, true}, // 13 integer digits
	}
	for _, tt := range tests {
		got, err := ParseQuantity(tt.in)
		if (err != nil) != tt.err || got != tt.want {
			t.Errorf("ParseQuantity(%q) = %d, %v; want %d, err=%v", tt.in, got, err, tt.want, tt.err)
		}
		if err != nil && !errors.Is(err, ErrInvalidQuantity) {
			t.Errorf("ParseQuantity(%q) error %v is not ErrInvalidQuantity", tt.in, err)
		}
	}
}

func TestFormatQuantity(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"}, {1000, "1"}, {1500, "1.5"}, {1250, "1.25"}, {1, "0.001"}, {10, "0.01"},
		{-1500, "-1.5"}, {-250, "-0.25"}, {-1, "-0.001"}, {123456789, "123456.789"},
		{math.MinInt64, "-9223372036854775.808"},
	}
	for _, tt := range tests {
		if got := FormatQuantity(tt.in); got != tt.want {
			t.Errorf("FormatQuantity(%d) = %q, want %q", tt.in, got, tt.want)
		}
		if tt.in != math.MinInt64 {
			if back, err := ParseQuantity(tt.want); err != nil || back != tt.in {
				t.Errorf("round trip %d → %q → %d, %v", tt.in, tt.want, back, err)
			}
		}
	}
}

func TestMulDivRound(t *testing.T) {
	tests := []struct {
		a, b, den int64
		want      int64
		ok        bool
	}{
		{5, 1, 2, 3, true},   // 2.5 → 3
		{-5, 1, 2, -3, true}, // -2.5 → -3 (away from zero)
		{4, 1, 3, 1, true},
		{5, 1, 3, 2, true},
		{-4, 1, 3, -1, true},
		{-5, 1, 3, -2, true},
		{1, 1, -2, -1, true}, // negative denominator
		{-1, 1, -2, 1, true},
		{0, 7, 3, 0, true},
		{12345, 1500, 1000, 18518, true},   // 18517.5
		{12345, -1500, 1000, -18518, true}, // -18517.5
		{1, 1, 0, 0, false},
		{math.MaxInt64, 2, 1, 0, false},
		{math.MaxInt64, 2, 2, math.MaxInt64, true}, // exact intermediate beyond int64
		{math.MinInt64, 1, 1, math.MinInt64, true},
	}
	for _, tt := range tests {
		got, ok := MulDivRound(tt.a, tt.b, tt.den)
		if got != tt.want || ok != tt.ok {
			t.Errorf("MulDivRound(%d,%d,%d) = %d,%v; want %d,%v", tt.a, tt.b, tt.den, got, ok, tt.want, tt.ok)
		}
	}
	if DivRound(7, 2) != 4 || DivRound(-7, 2) != -4 || DivRound(1, 0) != 0 {
		t.Error("DivRound")
	}
}

func TestRoundTo(t *testing.T) {
	tests := []struct{ in, want int64 }{
		{12349, 12300}, {12350, 12400}, {12351, 12400}, {12300, 12300}, {0, 0},
		{-12349, -12300}, {-12350, -12400}, {-12351, -12400}, {49, 0}, {50, 100}, {-50, -100},
	}
	for _, tt := range tests {
		if got := RoundTo(tt.in, 100); got != tt.want {
			t.Errorf("RoundTo(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestCalculate(t *testing.T) {
	tests := []struct {
		name  string
		lines []Line
		opts  Options
		want  Totals
	}{
		{
			name:  "no lines",
			lines: nil,
			want:  Totals{Lines: []LineAmounts{}, VatRecap: []VatRecap{}},
		},
		{
			name:  "net single line",
			lines: []Line{{2000, 10000, 2100}},
			want: Totals{
				Lines:    []LineAmounts{{20000, 4200, 24200}},
				VatRecap: []VatRecap{{2100, 20000, 4200, 24200}},
				Subtotal: 20000, VatTotal: 4200, Total: 24200,
			},
		},
		{
			name: "net mixed rates 0/12/21, VAT rounded per rate not per line",
			lines: []Line{
				{1500, 12345, 2100}, // 18517.5 → 18518
				{3000, 999, 1200},
				{1000, 5000, 0},
				{2000, 333, 2100},
			},
			want: Totals{
				Lines: []LineAmounts{
					{18518, 3889, 22407}, {2997, 360, 3357}, {5000, 0, 5000}, {666, 140, 806},
				},
				VatRecap: []VatRecap{
					{2100, 19184, 4029, 23213}, // 4028.64
					{1200, 2997, 360, 3357},    // 359.64
					{0, 5000, 0, 5000},
				},
				Subtotal: 27181, VatTotal: 4389, Total: 31570,
			},
		},
		{
			name: "prices include VAT",
			lines: []Line{
				{1000, 12100, 2100},
				{1000, 100, 2100}, // line VAT 17.36 → 17, but per rate sum is used
				{1000, 100, 2100},
				{1000, 1000, 1200}, // 107.14
			},
			opts: Options{PricesIncludeVAT: true},
			want: Totals{
				Lines: []LineAmounts{
					{10000, 2100, 12100}, {83, 17, 100}, {83, 17, 100}, {893, 107, 1000},
				},
				VatRecap: []VatRecap{
					{2100, 10165, 2135, 12300}, // 2134.71
					{1200, 893, 107, 1000},
				},
				Subtotal: 11058, VatTotal: 2242, Total: 13300,
			},
		},
		{
			name:  "round total down",
			lines: []Line{{1000, 12345, 2100}}, // vat 2592.45
			opts:  Options{RoundTotal: true},
			want: Totals{
				Lines:    []LineAmounts{{12345, 2592, 14937}},
				VatRecap: []VatRecap{{2100, 12345, 2592, 14937}},
				Subtotal: 12345, VatTotal: 2592, Rounding: -37, Total: 14900,
			},
		},
		{
			name:  "round total half up",
			lines: []Line{{1000, 12350, 0}},
			opts:  Options{RoundTotal: true},
			want: Totals{
				Lines:    []LineAmounts{{12350, 0, 12350}},
				VatRecap: []VatRecap{{0, 12350, 0, 12350}},
				Subtotal: 12350, Rounding: 50, Total: 12400,
			},
		},
		{
			name:  "negative correction lines, half away from zero",
			lines: []Line{{-1500, 12345, 2100}},
			opts:  Options{RoundTotal: true},
			want: Totals{
				Lines:    []LineAmounts{{-18518, -3889, -22407}},
				VatRecap: []VatRecap{{2100, -18518, -3889, -22407}},
				Subtotal: -18518, VatTotal: -3889, Rounding: 7, Total: -22400,
			},
		},
		{
			name:  "negative gross, round total -xx.50",
			lines: []Line{{-1000, 12350, 0}},
			opts:  Options{PricesIncludeVAT: true, RoundTotal: true},
			want: Totals{
				Lines:    []LineAmounts{{-12350, 0, -12350}},
				VatRecap: []VatRecap{{0, -12350, 0, -12350}},
				Subtotal: -12350, Rounding: -50, Total: -12400,
			},
		},
		{
			name:  "mixed positive and negative lines (discount)",
			lines: []Line{{1000, 10000, 2100}, {-1000, 1000, 2100}},
			want: Totals{
				Lines:    []LineAmounts{{10000, 2100, 12100}, {-1000, -210, -1210}},
				VatRecap: []VatRecap{{2100, 9000, 1890, 10890}},
				Subtotal: 9000, VatTotal: 1890, Total: 10890,
			},
		},
		{
			name:  "reverse charge keeps rates, no VAT",
			lines: []Line{{2000, 10000, 2100}, {1000, 500, 1200}},
			opts:  Options{ReverseCharge: true},
			want: Totals{
				Lines:    []LineAmounts{{20000, 0, 20000}, {500, 0, 500}},
				VatRecap: []VatRecap{{2100, 20000, 0, 20000}, {1200, 500, 0, 500}},
				Subtotal: 20500, Total: 20500,
			},
		},
		{
			name:  "reverse charge with gross prices",
			lines: []Line{{1000, 12100, 2100}},
			opts:  Options{ReverseCharge: true, PricesIncludeVAT: true},
			want: Totals{
				Lines:    []LineAmounts{{12100, 0, 12100}},
				VatRecap: []VatRecap{{2100, 12100, 0, 12100}},
				Subtotal: 12100, Total: 12100,
			},
		},
		{
			name:  "non VAT payer: every rate is 0",
			lines: []Line{{1000, 1000, 2100}, {2000, 500, 1200}},
			opts:  Options{NonVATPayer: true},
			want: Totals{
				Lines:    []LineAmounts{{1000, 0, 1000}, {1000, 0, 1000}},
				VatRecap: []VatRecap{{0, 2000, 0, 2000}},
				Subtotal: 2000, Total: 2000,
			},
		},
		{
			name:  "three decimal quantities",
			lines: []Line{{333, 100, 0}, {335, 100, 0}}, // 33.3 → 33, 33.5 → 34
			want: Totals{
				Lines:    []LineAmounts{{33, 0, 33}, {34, 0, 34}},
				VatRecap: []VatRecap{{0, 67, 0, 67}},
				Subtotal: 67, Total: 67,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(tt.lines, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
			if got.Total != got.Subtotal+got.VatTotal+got.Rounding {
				t.Fatalf("total invariant broken: %+v", got)
			}
		})
	}
}

func TestCalculateOverflow(t *testing.T) {
	cases := [][]Line{
		{{2000, math.MaxInt64, 0}},
		{{1000, math.MaxInt64, 0}, {1000, 1, 0}},
		{{1000, math.MaxInt64 / 10 * 9, 2100}},                  // VAT pushes total over
		{{1000, math.MaxInt64 / 10 * 9, 2100}, {1000, 1, 1200}}, // subtotal+vat overflow
		{{1000, math.MaxInt64, 2100}, {1000, 1, 0}},
	}
	for i, lines := range cases {
		if _, err := Calculate(lines, Options{}); !errors.Is(err, ErrOverflow) {
			t.Errorf("case %d: err = %v, want ErrOverflow", i, err)
		}
	}
}

func TestEffectiveStatus(t *testing.T) {
	today := "2026-03-15"
	tests := []struct {
		status, due, want string
	}{
		{model.StatusOpen, "2026-03-14", StatusOverdue},
		{model.StatusSent, "2026-01-01", StatusOverdue},
		{model.StatusOpen, "2026-03-15", model.StatusOpen}, // due today is not overdue
		{model.StatusSent, "2026-04-01", model.StatusSent},
		{model.StatusOpen, "", model.StatusOpen},
		{model.StatusPaid, "2026-01-01", model.StatusPaid},
		{model.StatusCancelled, "2026-01-01", model.StatusCancelled},
		{model.StatusUncollectible, "2026-01-01", model.StatusUncollectible},
	}
	for _, tt := range tests {
		if got := EffectiveStatus(tt.status, tt.due, today); got != tt.want {
			t.Errorf("EffectiveStatus(%s, %s) = %s, want %s", tt.status, tt.due, got, tt.want)
		}
	}
}

func TestPaymentStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		total, paid int64
		payments    int
		sent        bool
		want        string
	}{
		{"fully paid", model.StatusOpen, 1000, 1000, 1, false, model.StatusPaid},
		{"overpaid", model.StatusSent, 1000, 1500, 2, true, model.StatusPaid},
		{"partial, not sent", model.StatusOpen, 1000, 500, 1, false, model.StatusOpen},
		{"partial, sent", model.StatusSent, 1000, 500, 1, true, model.StatusSent},
		{"payment deleted from paid, sent", model.StatusPaid, 1000, 0, 0, true, model.StatusSent},
		{"payment deleted from paid, not sent", model.StatusPaid, 1000, 0, 0, false, model.StatusOpen},
		{"zero total without payments stays open", model.StatusOpen, 0, 0, 0, false, model.StatusOpen},
		{"zero total with payment", model.StatusOpen, 0, 0, 1, false, model.StatusPaid},
		{"refund covered", model.StatusOpen, -1000, -1000, 1, false, model.StatusPaid},
		{"refund partial", model.StatusOpen, -1000, -400, 1, false, model.StatusOpen},
		{"total raised above paid", model.StatusPaid, 2000, 1000, 1, true, model.StatusSent},
		{"cancelled untouched", model.StatusCancelled, 1000, 1000, 1, false, model.StatusCancelled},
		{"uncollectible untouched", model.StatusUncollectible, 1000, 1000, 1, false, model.StatusUncollectible},
	}
	for _, tt := range tests {
		if got := PaymentStatus(tt.status, tt.total, tt.paid, tt.payments, tt.sent); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestApplyAction(t *testing.T) {
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	earlier := now.Add(-48 * time.Hour)
	st := func(status string) State { return State{Status: status} }
	sent := func(status string) State { return State{Status: status, SentAt: &earlier} }

	type want struct {
		status        string
		ok            bool
		sentAt        *time.Time
		cancelled     bool
		uncollectible bool
		locked        bool
	}
	tests := []struct {
		name   string
		in     State
		action string
		want   want
	}{
		{"send open", st(model.StatusOpen), ActionMarkAsSent, want{status: model.StatusSent, ok: true, sentAt: &now}},
		{"send sent", sent(model.StatusSent), ActionMarkAsSent, want{}},
		{"send paid", st(model.StatusPaid), ActionMarkAsSent, want{}},
		{"send cancelled", st(model.StatusCancelled), ActionMarkAsSent, want{}},
		{"send uncollectible", st(model.StatusUncollectible), ActionMarkAsSent, want{}},

		{"cancel open", st(model.StatusOpen), ActionCancel, want{status: model.StatusCancelled, ok: true, cancelled: true}},
		{"cancel sent", sent(model.StatusSent), ActionCancel, want{status: model.StatusCancelled, ok: true, sentAt: &earlier, cancelled: true}},
		{"cancel with payments", State{Status: model.StatusSent, HasPayments: true}, ActionCancel, want{}},
		{"cancel paid", st(model.StatusPaid), ActionCancel, want{}},
		{"cancel cancelled", st(model.StatusCancelled), ActionCancel, want{}},
		{"cancel uncollectible", st(model.StatusUncollectible), ActionCancel, want{}},

		{"undo cancel → open", State{Status: model.StatusCancelled, CancelledAt: &earlier}, ActionUndoCancel, want{status: model.StatusOpen, ok: true}},
		{"undo cancel → sent", State{Status: model.StatusCancelled, CancelledAt: &earlier, SentAt: &earlier}, ActionUndoCancel, want{status: model.StatusSent, ok: true, sentAt: &earlier}},
		{"undo cancel open", st(model.StatusOpen), ActionUndoCancel, want{}},

		{"uncollectible open", st(model.StatusOpen), ActionMarkAsUncollectible, want{status: model.StatusUncollectible, ok: true, uncollectible: true}},
		{"uncollectible sent with payments", State{Status: model.StatusSent, SentAt: &earlier, HasPayments: true}, ActionMarkAsUncollectible, want{status: model.StatusUncollectible, ok: true, sentAt: &earlier, uncollectible: true}},
		{"uncollectible paid", st(model.StatusPaid), ActionMarkAsUncollectible, want{}},
		{"uncollectible cancelled", st(model.StatusCancelled), ActionMarkAsUncollectible, want{}},
		{"uncollectible twice", st(model.StatusUncollectible), ActionMarkAsUncollectible, want{}},

		{"undo uncollectible → open", State{Status: model.StatusUncollectible, UncollectibleAt: &earlier}, ActionUndoUncollectible, want{status: model.StatusOpen, ok: true}},
		{"undo uncollectible → sent", State{Status: model.StatusUncollectible, UncollectibleAt: &earlier, SentAt: &earlier}, ActionUndoUncollectible, want{status: model.StatusSent, ok: true, sentAt: &earlier}},
		{"undo uncollectible sent", sent(model.StatusSent), ActionUndoUncollectible, want{}},

		{"lock paid", st(model.StatusPaid), ActionLock, want{status: model.StatusPaid, ok: true, locked: true}},
		{"lock cancelled", st(model.StatusCancelled), ActionLock, want{status: model.StatusCancelled, ok: true, locked: true}},
		{"unlock", State{Status: model.StatusOpen, LockedAt: &earlier}, ActionUnlock, want{status: model.StatusOpen, ok: true}},
		{"unknown", st(model.StatusOpen), "explode", want{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyAction(tt.in, tt.action, now)
			if !tt.want.ok {
				var te *TransitionError
				if !errors.As(err, &te) || te.Error() == "" {
					t.Fatalf("want TransitionError, got %v (%+v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.want.status ||
				!reflect.DeepEqual(got.SentAt, tt.want.sentAt) ||
				(got.CancelledAt != nil) != tt.want.cancelled ||
				(got.UncollectibleAt != nil) != tt.want.uncollectible ||
				(got.LockedAt != nil) != tt.want.locked {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDates(t *testing.T) {
	if !ValidDate("2026-02-28") || ValidDate("2026-02-30") || ValidDate("2026-2-3") || ValidDate("") {
		t.Error("ValidDate")
	}
	tests := []struct {
		on   string
		days int
		want string
	}{
		{"2026-03-15", 14, "2026-03-29"},
		{"2026-12-25", 14, "2027-01-08"},
		{"2028-02-15", 14, "2028-02-29"}, // leap year
		{"2026-03-15", 0, "2026-03-15"},
	}
	for _, tt := range tests {
		got, err := DueOn(tt.on, tt.days)
		if err != nil || got != tt.want {
			t.Errorf("DueOn(%s, %d) = %s, %v; want %s", tt.on, tt.days, got, err, tt.want)
		}
	}
	if _, err := AddDays("nope", 1); err == nil {
		t.Error("AddDays accepted an invalid date")
	}
	if Today(time.Date(2026, 3, 5, 23, 0, 0, 0, time.UTC)) != "2026-03-05" {
		t.Error("Today")
	}
}
