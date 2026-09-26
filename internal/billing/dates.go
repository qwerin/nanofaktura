package billing

import "time"

// DateLayout is the layout of every *_on field.
const DateLayout = "2006-01-02"

// ValidDate reports whether s is a real calendar date "YYYY-MM-DD".
func ValidDate(s string) bool {
	_, err := time.Parse(DateLayout, s)
	return err == nil
}

// Today formats now as "YYYY-MM-DD" in now's location.
func Today(now time.Time) string {
	return now.Format(DateLayout)
}

// AddDays returns date + days ("YYYY-MM-DD").
func AddDays(date string, days int) (string, error) {
	t, err := time.Parse(DateLayout, date)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, days).Format(DateLayout), nil
}

// DueOn is the due date of a document issued on issuedOn payable in dueDays.
func DueOn(issuedOn string, dueDays int) (string, error) {
	return AddDays(issuedOn, dueDays)
}
