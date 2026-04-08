package pdf

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/qwerin/nanofaktura/internal/models"
)

// Spayd builds a Czech QR platba string in SPAYD 1.0 format.
// Returns "" if IBAN is missing or not a Czech account (CZ prefix).
func Spayd(inv *models.Invoice) string {
	iban := inv.IBAN
	if !strings.HasPrefix(strings.ToUpper(iban), "CZ") {
		return "" // QR Platba is only for Czech (CZ) accounts
	}

	amount := fmt.Sprintf("%.2f", float64(inv.Total)/100.0)

	msg := inv.Number
	if len(msg) > 60 {
		msg = msg[:60]
	}

	s := "SPD*1.0"
	s += "*ACC:" + iban
	s += "*AM:" + amount
	s += "*CC:" + inv.Currency
	s += "*MSG:" + msg
	vs := inv.VariableSymbol
	if vs == "" {
		vs = onlyDigits(inv.Number)
	}
	if vs != "" {
		s += "*X-VS:" + vs
	}
	return s
}

// onlyDigits returns only the digit characters from s, truncated to 10 (X-VS limit).
func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) > 10 {
		d = d[len(d)-10:] // keep trailing digits (sequence number is more significant than year)
	}
	return d
}
