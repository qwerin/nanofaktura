package billing

import (
	"errors"
	"math/big"
)

// ErrOverflow is returned when an amount does not fit into int64.
var ErrOverflow = errors.New("amount out of range")

// MulDivRound returns a×b/den rounded half away from zero, computed exactly
// with big integers. ok is false when den is 0 or the result does not fit
// into int64.
func MulDivRound(a, b, den int64) (v int64, ok bool) {
	if den == 0 {
		return 0, false
	}
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	d := big.NewInt(den)
	if d.Sign() < 0 {
		n.Neg(n)
		d.Neg(d)
	}
	neg := n.Sign() < 0
	n.Abs(n)
	// floor((2n + d) / 2d) = n/d rounded half up, for n ≥ 0
	n.Lsh(n, 1).Add(n, d)
	n.Quo(n, d.Lsh(d, 1))
	if neg {
		n.Neg(n)
	}
	if !n.IsInt64() {
		return 0, false
	}
	return n.Int64(), true
}

// DivRound returns num/den rounded half away from zero (0 when den is 0).
func DivRound(num, den int64) int64 {
	v, _ := MulDivRound(num, 1, den)
	return v
}

// RoundTo rounds v to a multiple of unit (> 0), half away from zero:
// RoundTo(12350, 100) = 12400, RoundTo(-12350, 100) = -12400.
func RoundTo(v, unit int64) int64 {
	return DivRound(v, unit) * unit
}

// addChecked returns a+b; ok is false on int64 overflow.
func addChecked(a, b int64) (int64, bool) {
	s := a + b
	if (a > 0 && b > 0 && s < 0) || (a < 0 && b < 0 && s >= 0) {
		return 0, false
	}
	return s, true
}
