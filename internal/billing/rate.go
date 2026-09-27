package billing

import (
	"errors"
	"strconv"
	"strings"
)

// RateScale is the fixed-point scale of exchange rates: 1 CZK/unit = 1_000_000.
const RateScale = 1_000_000

// ErrInvalidRate is returned by ParseRate for malformed or non-positive rates.
var ErrInvalidRate = errors.New("invalid exchange rate")

// ParseRate parses a decimal exchange rate ("24.355", "1", "25,1"; max 6
// decimal places) into millionths. Empty means 1.
func ParseRate(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if s == "" {
		return RateScale, nil
	}
	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" || !allDigits(intPart) || len(intPart) > 9 || len(frac) > 6 || (frac != "" && !allDigits(frac)) {
		return 0, ErrInvalidRate
	}
	whole, _ := strconv.ParseInt(intPart, 10, 64)
	frac += strings.Repeat("0", 6-len(frac))
	f, _ := strconv.ParseInt(frac, 10, 64)
	v := whole*RateScale + f
	if v <= 0 {
		return 0, ErrInvalidRate
	}
	return v, nil
}

// ToLocal converts an amount in minor units of a foreign currency to minor
// units of the local currency with a rate from ParseRate (half away from zero).
func ToLocal(amount, rate int64) int64 {
	v, ok := MulDivRound(amount, rate, RateScale)
	if !ok {
		return 0
	}
	return v
}
