package billing

import (
	"strconv"
	"strings"
	"time"
)

// daysIn returns the number of days of month m in year y.
func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// clampedDate is year/month with day clamped to the month length (day ≥ 1).
func clampedDate(y int, m time.Month, day int) time.Time {
	// normalize month overflow first (time.Date would also roll the day over)
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	y, m = first.Year(), first.Month()
	if day < 1 {
		day = 1
	}
	if n := daysIn(y, m); day > n {
		day = n
	}
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

// AddMonths returns date + months with the day set to anchorDay clamped to
// the length of the target month: AddMonths("2026-01-31", 1, 31) =
// "2026-02-28", AddMonths("2026-02-28", 1, 31) = "2026-03-31" (the anchor
// day prevents drift). anchorDay ≤ 0 uses the day of date.
func AddMonths(date string, months, anchorDay int) (string, error) {
	t, err := time.Parse(DateLayout, date)
	if err != nil {
		return "", err
	}
	if anchorDay <= 0 {
		anchorDay = t.Day()
	}
	return clampedDate(t.Year(), t.Month()+time.Month(months), anchorDay).Format(DateLayout), nil
}

// FirstOccurrence is the first date ≥ start whose day is dayOfMonth (clamped
// to the month length); dayOfMonth ≤ 0 means start itself.
func FirstOccurrence(start string, dayOfMonth int) (string, error) {
	t, err := time.Parse(DateLayout, start)
	if err != nil {
		return "", err
	}
	if dayOfMonth <= 0 {
		return start, nil
	}
	d := clampedDate(t.Year(), t.Month(), dayOfMonth)
	if d.Before(t) {
		d = clampedDate(t.Year(), t.Month()+1, dayOfMonth)
	}
	return d.Format(DateLayout), nil
}

// monthNames are nominative month names. Czech and Slovak are lowercase
// (as in running text: "za měsíc září"), English and German capitalized.
var monthNames = map[string][12]string{
	"cs": {"leden", "únor", "březen", "duben", "květen", "červen", "červenec", "srpen", "září", "říjen", "listopad", "prosinec"},
	"sk": {"január", "február", "marec", "apríl", "máj", "jún", "júl", "august", "september", "október", "november", "december"},
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"de": {"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
}

// MonthName returns the nominative name of month (1–12) in lang (cs, sk,
// en, de; anything else → cs).
func MonthName(month time.Month, lang string) string {
	names, ok := monthNames[lang]
	if !ok {
		names = monthNames["cs"]
	}
	return names[(int(month)+11)%12]
}

// DateVars are the values of the date placeholders for date ("YYYY-MM-DD")
// in lang; see RenderDatePlaceholders.
func DateVars(date, lang string) (map[string]string, error) {
	t, err := time.Parse(DateLayout, date)
	if err != nil {
		return nil, err
	}
	prev := clampedDate(t.Year(), t.Month()-1, 1)
	next := clampedDate(t.Year(), t.Month()+1, 1)
	return map[string]string{
		"MONTH":           twoDigits(int(t.Month())),
		"MONTH_NAME":      MonthName(t.Month(), lang),
		"PREV_MONTH":      twoDigits(int(prev.Month())),
		"PREV_MONTH_NAME": MonthName(prev.Month(), lang),
		"NEXT_MONTH":      twoDigits(int(next.Month())),
		"NEXT_MONTH_NAME": MonthName(next.Month(), lang),
		"YEAR":            strconv.Itoa(t.Year()),
		"PREV_YEAR":       strconv.Itoa(t.Year() - 1),
		"NEXT_YEAR":       strconv.Itoa(t.Year() + 1),
		"QUARTER":         strconv.Itoa((int(t.Month())-1)/3 + 1),
	}, nil
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// RenderDatePlaceholders replaces the date placeholders of recurring
// invoices in text: {MONTH} (MM), {MONTH_NAME}, {PREV_MONTH}, {PREV_MONTH_NAME},
// {NEXT_MONTH}, {NEXT_MONTH_NAME}, {YEAR}, {PREV_YEAR}, {NEXT_YEAR},
// {QUARTER} (1–4), computed from date (the issue date) in lang. Month names
// are nominative ("Služby za měsíc {MONTH_NAME}" → "Služby za měsíc září").
// Unknown placeholders are kept; an invalid date returns text unchanged.
func RenderDatePlaceholders(text, date, lang string) string {
	if !strings.Contains(text, "{") {
		return text
	}
	vars, err := DateVars(date, lang)
	if err != nil {
		return text
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(text, '{')
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		j := strings.IndexByte(text[i:], '}')
		if j < 0 {
			b.WriteString(text)
			return b.String()
		}
		b.WriteString(text[:i])
		if v, ok := vars[text[i+1:i+j]]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(text[i : i+j+1])
		}
		text = text[i+j+1:]
	}
}
