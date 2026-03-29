package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"

	"github.com/qwerin/nanofaktura/internal/models"
)

// spayd builds a Czech QR platba string in SPAYD 1.0 format.
func spayd(inv *models.Invoice) string {
	iban := inv.IBAN
	if iban == "" {
		return "" // can't generate without IBAN
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
	if inv.VariableSymbol != "" {
		s += "*X-VS:" + inv.VariableSymbol
	}
	return s
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
