package billing

import (
	"testing"
	"time"
)

func TestAddMonths(t *testing.T) {
	for _, c := range []struct {
		date   string
		months int
		anchor int
		want   string
	}{
		{"2026-01-31", 1, 31, "2026-02-28"},
		{"2028-01-31", 1, 31, "2028-02-29"}, // leap year
		{"2026-02-28", 1, 31, "2026-03-31"}, // anchor restores the day
		{"2026-01-31", 1, 0, "2026-02-28"},  // anchor = day of date
		{"2026-03-31", 1, 0, "2026-04-30"},
		{"2026-11-15", 3, 15, "2027-02-15"},
		{"2026-12-01", 1, 1, "2027-01-01"},
		{"2026-05-31", 12, 31, "2027-05-31"},
		{"2026-03-10", -1, 10, "2026-02-10"},
		{"2026-01-15", 1, 40, "2026-02-28"}, // anchor > 31 clamps
	} {
		got, err := AddMonths(c.date, c.months, c.anchor)
		if err != nil || got != c.want {
			t.Errorf("AddMonths(%s, %d, %d) = %s, %v; want %s", c.date, c.months, c.anchor, got, err, c.want)
		}
	}
	if _, err := AddMonths("2026-02-30", 1, 0); err == nil {
		t.Error("invalid date accepted")
	}
}

func TestAddMonthsNoDrift(t *testing.T) {
	d := "2026-01-31"
	want := []string{"2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30"}
	for _, w := range want {
		var err error
		if d, err = AddMonths(d, 1, 31); err != nil || d != w {
			t.Fatalf("got %s, want %s (%v)", d, w, err)
		}
	}
}

func TestFirstOccurrence(t *testing.T) {
	for _, c := range []struct {
		start string
		dom   int
		want  string
	}{
		{"2026-03-15", 0, "2026-03-15"},
		{"2026-03-15", 20, "2026-03-20"},
		{"2026-03-15", 15, "2026-03-15"},
		{"2026-03-15", 1, "2026-04-01"},
		{"2026-02-10", 31, "2026-02-28"},
		{"2026-12-20", 5, "2027-01-05"},
	} {
		got, err := FirstOccurrence(c.start, c.dom)
		if err != nil || got != c.want {
			t.Errorf("FirstOccurrence(%s, %d) = %s, %v; want %s", c.start, c.dom, got, err, c.want)
		}
	}
}

func TestMonthName(t *testing.T) {
	for _, c := range []struct {
		m    time.Month
		lang string
		want string
	}{
		{time.September, "cs", "září"},
		{time.January, "cs", "leden"},
		{time.December, "sk", "december"},
		{time.March, "de", "März"},
		{time.May, "en", "May"},
		{time.July, "xx", "červenec"},
	} {
		if got := MonthName(c.m, c.lang); got != c.want {
			t.Errorf("MonthName(%v, %s) = %s, want %s", c.m, c.lang, got, c.want)
		}
	}
}

func TestRenderDatePlaceholders(t *testing.T) {
	for _, c := range []struct {
		text, date, lang, want string
	}{
		{"Služby za měsíc {MONTH_NAME} {YEAR}", "2026-09-01", "cs", "Služby za měsíc září 2026"},
		{"Hosting {MONTH}/{YEAR}", "2026-09-01", "cs", "Hosting 09/2026"},
		{"Za {PREV_MONTH_NAME}", "2026-01-05", "cs", "Za prosinec"},
		{"{PREV_MONTH}/{PREV_YEAR}", "2026-01-05", "cs", "12/2025"},
		{"Next: {NEXT_MONTH_NAME} {NEXT_MONTH}", "2026-12-31", "en", "Next: January 01"},
		{"Q{QUARTER}/{YEAR}", "2026-09-30", "en", "Q3/2026"},
		{"Q{QUARTER}", "2026-01-01", "en", "Q1"},
		{"Q{QUARTER}", "2026-12-01", "en", "Q4"},
		{"{MONTH_NAME}", "2026-03-01", "de", "März"},
		{"{MONTH_NAME}", "2026-03-01", "sk", "marec"},
		{"{UNKNOWN} {MONTH}", "2026-03-01", "cs", "{UNKNOWN} 03"},
		{"broken {MONTH", "2026-03-01", "cs", "broken {MONTH"},
		{"no placeholders", "2026-03-01", "cs", "no placeholders"},
		{"{MONTH}", "bad-date", "cs", "{MONTH}"},
		{"{month}", "2026-03-01", "cs", "{month}"}, // case-sensitive
	} {
		if got := RenderDatePlaceholders(c.text, c.date, c.lang); got != c.want {
			t.Errorf("Render(%q, %s, %s) = %q, want %q", c.text, c.date, c.lang, got, c.want)
		}
	}
}
