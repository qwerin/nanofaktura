// Package reports computes tax reports from documents: the VAT return
// (přiznání k DPH, EPO form DPHDP3), the VAT control statement (kontrolní
// hlášení, DPHKH1) and the income-tax overview of a sole trader
// (flat-rate expenses). It is pure domain logic without DB/HTTP; amounts are
// int64 minor units (haléře) in CZK.
package reports

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Period is a VAT period: one month or one quarter of a year.
type Period struct {
	Year    int
	Month   int // 1–12 for a monthly period, else 0
	Quarter int // 1–4 for a quarterly period, else 0
}

// ErrInvalidPeriod is returned by ParsePeriod.
var ErrInvalidPeriod = errors.New(`invalid period, expected "YYYY-MM" or "YYYY-Qn"`)

var periodRe = regexp.MustCompile(`^(\d{4})-(?:(0[1-9]|1[0-2])|[Qq]([1-4]))$`)

// ParsePeriod parses "2026-09" (month) or "2026-Q3" (quarter).
func ParsePeriod(s string) (Period, error) {
	m := periodRe.FindStringSubmatch(s)
	if m == nil {
		return Period{}, ErrInvalidPeriod
	}
	p := Period{}
	p.Year, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		p.Month, _ = strconv.Atoi(m[2])
	} else {
		p.Quarter, _ = strconv.Atoi(m[3])
	}
	return p, nil
}

// PreviousPeriod is the last complete period before today (the one a
// return is typically filed for); quarterly selects quarters.
func PreviousPeriod(today time.Time, quarterly bool) Period {
	if quarterly {
		q := (int(today.Month())-1)/3 + 1
		if q == 1 {
			return Period{Year: today.Year() - 1, Quarter: 4}
		}
		return Period{Year: today.Year(), Quarter: q - 1}
	}
	prev := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	return Period{Year: prev.Year(), Month: int(prev.Month())}
}

// String is the canonical form accepted by ParsePeriod.
func (p Period) String() string {
	if p.Quarter != 0 {
		return fmt.Sprintf("%04d-Q%d", p.Year, p.Quarter)
	}
	return fmt.Sprintf("%04d-%02d", p.Year, p.Month)
}

// Quarterly reports whether p is a quarter.
func (p Period) Quarterly() bool { return p.Quarter != 0 }

// Range returns the first and last day of p ("YYYY-MM-DD").
func (p Period) Range() (from, to string) {
	first, months := p.Month, 1
	if p.Quarter != 0 {
		first, months = (p.Quarter-1)*3+1, 3
	}
	start := time.Date(p.Year, time.Month(first), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, months, -1)
	return start.Format(time.DateOnly), end.Format(time.DateOnly)
}

// Contains reports whether the date "YYYY-MM-DD" lies in p.
func (p Period) Contains(date string) bool {
	from, to := p.Range()
	return date >= from && date <= to
}
