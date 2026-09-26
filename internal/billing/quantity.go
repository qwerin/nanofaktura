package billing

import (
	"errors"
	"strconv"
	"strings"
)

// ErrInvalidQuantity is returned by ParseQuantity for malformed input.
var ErrInvalidQuantity = errors.New("invalid quantity: expected a decimal number with at most 3 decimal places")

// maxQuantityIntDigits bounds the integer part so the milli value always fits int64.
const maxQuantityIntDigits = 12

// ParseQuantity converts a decimal string ("1", "-2.5", "0,125", "+3") to
// thousandths. Both '.' and ',' are accepted as decimal separator; at most
// 3 decimal places and 12 integer digits; surrounding spaces are ignored.
func ParseQuantity(s string) (int64, error) {
	s = strings.TrimSpace(s)
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	intPart, frac, hasSep := strings.Cut(strings.Replace(s, ",", ".", 1), ".")
	if intPart == "" || len(intPart) > maxQuantityIntDigits || !allDigits(intPart) {
		return 0, ErrInvalidQuantity
	}
	if hasSep && (frac == "" || len(frac) > 3 || !allDigits(frac)) {
		return 0, ErrInvalidQuantity
	}
	frac += strings.Repeat("0", 3-len(frac))
	v, _ := strconv.ParseInt(intPart+frac, 10, 64) // ≤ 15 digits, cannot fail
	if neg {
		v = -v
	}
	return v, nil
}

// FormatQuantity renders thousandths as the shortest decimal string:
// 1500 → "1.5", 1000 → "1", -250 → "-0.25", 0 → "0".
func FormatQuantity(milli int64) string {
	sign := ""
	u := uint64(milli)
	if milli < 0 {
		sign, u = "-", -u
	}
	out := sign + strconv.FormatUint(u/1000, 10)
	if frac := u % 1000; frac != 0 {
		out += "." + strings.TrimRight(strconv.FormatUint(frac+1000, 10)[1:], "0")
	}
	return out
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
