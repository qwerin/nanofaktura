package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strings"
	"unicode"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"

	"github.com/qwerin/nanofaktura/internal/models"
)

// spayd builds a Czech QR platba string in SPAYD 1.0 format.
// Returns "" if IBAN is missing or not a Czech account (CZ prefix).
func spayd(inv *models.Invoice) string {
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

// QRPlatba generates a PNG-encoded QR platba image for the invoice.
// Returns nil, nil when IBAN is missing.
func QRPlatba(inv *models.Invoice) ([]byte, error) {
	content := spayd(inv)
	if content == "" {
		return nil, nil
	}

	code, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}
	scaled, err := barcode.Scale(code, 180, 180)
	if err != nil {
		return nil, fmt.Errorf("qr scale: %w", err)
	}

	// Maroto nepodporuje 16-bit PNG — překreslíme do 8-bit NRGBA.
	dst := image.NewNRGBA(scaled.Bounds())
	draw.Draw(dst, dst.Bounds(), scaled, scaled.Bounds().Min, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("qr png: %w", err)
	}
	return buf.Bytes(), nil
}
