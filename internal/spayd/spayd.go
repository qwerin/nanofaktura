// Package spayd builds Czech "QR Platba" payloads (SPAYD 1.0) and handles
// Czech bank account numbers: mod-11 validation, IBAN conversion, SWIFT lookup.
package spayd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ErrInvalidAccount is returned for a malformed Czech account number or one
// failing the mod-11 check.
var ErrInvalidAccount = errors.New("invalid Czech bank account number")

var accountRe = regexp.MustCompile(`^(?:(\d{1,6})-)?(\d{2,10})/(\d{4})$`)

var mod11Weights = [10]int{6, 3, 7, 9, 10, 5, 8, 4, 2, 1}

func mod11(part string) bool {
	part = fmt.Sprintf("%010s", part)
	sum := 0
	for i, r := range part {
		sum += int(r-'0') * mod11Weights[i]
	}
	return sum%11 == 0
}

// Account is a parsed Czech account number "prefix-number/bank".
type Account struct {
	Prefix, Number, Bank string
}

// ParseAccount parses and validates a Czech account number ("19-2000145399/0800").
func ParseAccount(s string) (Account, error) {
	m := accountRe.FindStringSubmatch(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if m == nil || !mod11(m[1]) || !mod11(m[2]) {
		return Account{}, ErrInvalidAccount
	}
	return Account{Prefix: m[1], Number: m[2], Bank: m[3]}, nil
}

// IBAN returns the Czech IBAN of the account.
func (a Account) IBAN() string {
	bban := a.Bank + fmt.Sprintf("%06s%010s", a.Prefix, a.Number)
	check := 98 - mod97(bban+"123500") // "CZ" = 12 35, check digits 00
	return fmt.Sprintf("CZ%02d%s", check, bban)
}

// SWIFT returns the BIC of the account's bank, or "" if unknown.
func (a Account) SWIFT() string { return bankSWIFT[a.Bank] }

// ValidIBAN reports whether s is a structurally valid IBAN (ISO 13616 mod-97).
func ValidIBAN(s string) bool {
	s = NormalizeIBAN(s)
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	var digits strings.Builder
	for _, r := range s[4:] + s[:4] {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			fmt.Fprintf(&digits, "%d", r-'A'+10)
		default:
			return false
		}
	}
	return mod97(digits.String()) == 1
}

// NormalizeIBAN uppercases s and strips spaces.
func NormalizeIBAN(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

func mod97(digits string) int {
	rem := 0
	for _, r := range digits {
		rem = (rem*10 + int(r-'0')) % 97
	}
	return rem
}

// Payment is the input of Build.
type Payment struct {
	IBAN           string
	SWIFT          string // optional
	Amount         int64  // minor units (haléře)
	Currency       string // ISO 4217, default CZK
	VariableSymbol string // digits only, max 10; non-digits are dropped
	DueOn          string // YYYY-MM-DD, optional
	Message        string // optional, max 60 chars
	RecipientName  string // optional, max 35 chars
}

// MaxAmount is the largest amount of a QR Platba (AM has at most 10
// characters: 9 999 999.99).
const MaxAmount = 9_999_999_99

// Build returns the SPAYD string for p, or "" if the IBAN is not a Czech one
// (QR Platba is a Czech domestic standard) or the amount is not positive or
// too large for the format.
func Build(p Payment) string {
	iban := NormalizeIBAN(p.IBAN)
	if !strings.HasPrefix(iban, "CZ") || !ValidIBAN(iban) || p.Amount <= 0 || p.Amount > MaxAmount {
		return ""
	}
	acc := iban
	if p.SWIFT != "" {
		acc += "+" + strings.ToUpper(p.SWIFT)
	}
	cur := p.Currency
	if cur == "" {
		cur = "CZK"
	}
	parts := []string{
		"SPD*1.0",
		"ACC:" + acc,
		fmt.Sprintf("AM:%d.%02d", p.Amount/100, p.Amount%100),
		"CC:" + strings.ToUpper(cur),
	}
	if p.DueOn != "" {
		parts = append(parts, "DT:"+strings.ReplaceAll(p.DueOn, "-", ""))
	}
	if msg := clean(p.Message, 60); msg != "" {
		parts = append(parts, "MSG:"+msg)
	}
	if rn := clean(p.RecipientName, 35); rn != "" {
		parts = append(parts, "RN:"+rn)
	}
	if vs := Digits(p.VariableSymbol, 10); vs != "" {
		parts = append(parts, "X-VS:"+vs)
	}
	return strings.Join(parts, "*")
}

// Digits keeps only the digits of s and returns at most the last max of them
// (the sequence part of a document number is at the end).
func Digits(s string, max int) string {
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if len(d) > max {
		d = d[len(d)-max:]
	}
	return d
}

// clean removes the '*' separator and control chars and truncates to max runes.
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r == '*' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

var bankSWIFT = map[string]string{
	"0100": "KOMBCZPP", "0300": "CEKOCZPP", "0600": "AGBACZPP", "0710": "CNBACZPP",
	"0800": "GIBACZPX", "2010": "FIOBCZPP", "2060": "CITFCZPP", "2250": "CTASCZ22",
	"2600": "CITICZPX", "2700": "BACXCZPP", "3030": "AIRACZPP", "3060": "BPKOCZPP",
	"3500": "INGBCZPP", "4000": "EXPNCZPP", "4300": "CMZRCZP1", "5500": "RZBCCZPP",
	"5800": "JTBPCZPP", "6000": "PMBPCZPP", "6100": "EQBKCZPP", "6200": "COBACZPX",
	"6210": "BREXCZPP", "6700": "SUBACZPP", "6800": "VBOECZ2X", "7910": "DEUTCZPX",
	"8040": "OBKLCZ2X", "8250": "BKCHCZPP",
}
