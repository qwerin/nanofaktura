package pdf

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"

	pdfread "github.com/ledongthuc/pdf"

	"github.com/qwerin/nanofaktura/internal/model"
)

// extracted is the text of a rendered PDF. exact is true when poppler's
// pdftotext was used; the pure-Go fallback only decodes ASCII reliably.
type extracted struct {
	text  string
	pages int
	exact bool
}

func extract(t *testing.T, b []byte) extracted {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", b[:min(len(b), 16)])
	}
	r, err := pdfread.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("parse pdf: %v", err)
	}
	e := extracted{pages: r.NumPage()}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		cmd := exec.Command("pdftotext", "-", "-")
		cmd.Stdin = bytes.NewReader(b)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("pdftotext: %v", err)
		}
		e.text, e.exact = string(out), true
		return e
	}
	var sb strings.Builder
	for i := 1; i <= e.pages; i++ {
		for _, tx := range r.Page(i).Content().Text {
			sb.WriteString(tx.S)
		}
		sb.WriteString("\n")
	}
	e.text = sb.String()
	return e
}

// has checks s is in the text; strings with non-ASCII chars are only checked
// with the exact extractor.
func (e extracted) has(t *testing.T, s string) {
	t.Helper()
	if !e.exact && !isASCII(s) {
		return
	}
	if !strings.Contains(squash(e.text), squash(s)) {
		t.Errorf("text lacks %q", s)
	}
}

func (e extracted) lacks(t *testing.T, s string) {
	t.Helper()
	if !e.exact && !isASCII(s) {
		return
	}
	if strings.Contains(squash(e.text), squash(s)) {
		t.Errorf("text unexpectedly contains %q", s)
	}
}

// squash lowercases s and removes all whitespace (incl. NBSP) so layout line
// breaks and uppercase headings don't matter.
func squash(s string) string {
	s = strings.ToLower(s)
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ' ' || r == '\f'
	}), "")
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func render(t *testing.T, spec SampleSpec, opt Options) extracted {
	t.Helper()
	inv, acc := Sample(spec)
	b, err := Render(inv, acc, opt)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return extract(t, b)
}

func TestRenderMatrix(t *testing.T) {
	for _, tpl := range Templates {
		for _, docType := range []string{model.DocInvoice, model.DocProforma, model.DocCorrection} {
			for _, payer := range []bool{true, false} {
				name := tpl + "/" + docType
				if payer {
					name += "/payer"
				}
				t.Run(name, func(t *testing.T) {
					inv, acc := Sample(SampleSpec{DocumentType: docType, VatPayer: payer})
					opt := Options{Template: tpl, ShowQR: true}
					if docType == model.DocCorrection {
						opt.RelatedNumber = "2026-0041"
					}
					b, err := Render(inv, acc, opt)
					if err != nil {
						t.Fatal(err)
					}
					e := extract(t, b)
					if e.pages != 1 {
						t.Errorf("pages = %d", e.pages)
					}
					e.has(t, inv.Number)
					e.has(t, "Studio Kovář s.r.o.")
					e.has(t, "ACME Technologies a.s.")
					e.has(t, "27074358")
					e.has(t, "Strana 1 / 1")
					if docType == model.DocCorrection {
						e.lacks(t, "QR Platba") // negative amount
						e.has(t, "Oprava dokladu č. 2026-0041")
					} else {
						e.has(t, "QR Platba")
					}
					switch {
					case docType == model.DocProforma:
						e.has(t, "Zálohová faktura")
						e.has(t, "Nejedná se o daňový doklad")
					case docType == model.DocInvoice && payer:
						e.has(t, "Faktura – daňový doklad")
					case docType == model.DocCorrection && payer:
						e.has(t, "Opravný daňový doklad")
					}
					if payer {
						e.has(t, "Rekapitulace DPH")
						e.has(t, "DIČ: CZ27074358")
						e.lacks(t, "Neplátce DPH")
						if docType != model.DocProforma {
							e.has(t, "Datum zdan. plnění")
						}
					} else {
						e.has(t, "Neplátce DPH")
						e.lacks(t, "Rekapitulace DPH")
						e.lacks(t, "Datum zdan. plnění")
					}
					e.has(t, "Celkem k úhradě")
				})
			}
		}
	}
}

func TestRenderManyLines(t *testing.T) {
	for _, tpl := range Templates {
		t.Run(tpl, func(t *testing.T) {
			e := render(t, SampleSpec{VatPayer: true, Lines: 60}, Options{Template: tpl, ShowQR: true})
			if e.pages < 3 {
				t.Fatalf("60 lines on %d pages", e.pages)
			}
			e.has(t, "(1)")
			e.has(t, "(60)")
			e.has(t, "Strana 2 / ")
			e.has(t, "pokračování")
			e.has(t, "Rekapitulace DPH")
		})
	}
}

func TestRenderStatesAndLanguages(t *testing.T) {
	e := render(t, SampleSpec{VatPayer: true, Status: model.StatusPaid}, Options{ShowQR: true})
	e.has(t, "ZAPLACENO")
	e.lacks(t, "QR Platba") // nothing left to pay

	e = render(t, SampleSpec{VatPayer: true, PaidPartially: true}, Options{Template: TemplateModern, ShowQR: true})
	e.has(t, "Zaplaceno")
	e.has(t, "Zbývá uhradit")
	e.has(t, "QR Platba")

	e = render(t, SampleSpec{VatPayer: true, ReverseCharge: true, Language: "en"}, Options{Template: TemplateMinimal})
	e.has(t, "Invoice – tax document")
	e.has(t, "Reverse charge")
	e.has(t, "Germany")
	e.has(t, "Page 1 / 1")
	e.lacks(t, "QR Platba") // ShowQR off

	e = render(t, SampleSpec{DocumentType: model.DocProforma}, Options{Language: "de"})
	e.has(t, "Vorauszahlungsrechnung")
	e.has(t, "Seite 1 / 1")

	e = render(t, SampleSpec{Status: model.StatusCancelled}, Options{Language: "sk", ShowQR: true})
	e.has(t, "Faktúra")
	e.has(t, "STORNO")
	e.lacks(t, "QR Platba")
}

func TestRenderWithImagesAndAccent(t *testing.T) {
	// 1×1 PNG
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89" +
		"\x00\x00\x00\rIDATx\x9cc\xf8\xcf\xc0\xf0\x1f\x00\x05\x00\x01\xff\x89\x99=\x1d\x00\x00\x00\x00IEND\xaeB`\x82")
	for _, tpl := range Templates {
		inv, acc := Sample(SampleSpec{VatPayer: true})
		for _, accent := range []string{"#0f766e", "fde68a", "nonsense"} {
			b, err := Render(inv, acc, Options{Template: tpl, Accent: accent, Logo: png, Stamp: png, ShowQR: true})
			if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
				t.Fatalf("%s %s: %v", tpl, accent, err)
			}
		}
	}
}

func TestRenderErrors(t *testing.T) {
	inv, acc := Sample(SampleSpec{})
	if _, err := Render(inv, acc, Options{Template: "fancy"}); !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("unknown template: %v", err)
	}
	if _, err := Render(nil, acc, Options{}); err == nil {
		t.Fatal("nil invoice accepted")
	}
	// minimal data: no account, no lines, no snapshot
	b, err := Render(&model.Invoice{Number: "1", Status: model.StatusOpen}, nil, Options{ShowQR: true})
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("empty invoice: %v", err)
	}
}

func TestQRConditions(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*model.Invoice, *Options)
		want bool
	}{
		{"default", func(*model.Invoice, *Options) {}, true},
		{"not requested", func(_ *model.Invoice, o *Options) { o.ShowQR = false }, false},
		{"EUR", func(i *model.Invoice, _ *Options) { i.Currency = "EUR" }, false},
		{"foreign IBAN", func(i *model.Invoice, _ *Options) { i.IBAN = "DE89370400440532013000" }, false},
		{"cash", func(i *model.Invoice, _ *Options) { i.PaymentMethod = "cash" }, false},
		{"paid", func(i *model.Invoice, _ *Options) { i.PaidAmount = i.Total }, false},
		{"cancelled", func(i *model.Invoice, _ *Options) { i.Status = model.StatusCancelled }, false},
	}
	for _, c := range cases {
		inv, acc := Sample(SampleSpec{VatPayer: true})
		opt := Options{ShowQR: true}
		c.mod(inv, &opt)
		d := newDoc(inv, acc, opt)
		if (d.qr != "") != c.want {
			t.Errorf("%s: qr = %q", c.name, d.qr)
		}
	}
	inv, acc := Sample(SampleSpec{VatPayer: true, PaidPartially: true})
	d := newDoc(inv, acc, Options{ShowQR: true})
	due := inv.Total - inv.PaidAmount
	wantAM := "AM:" + strings.TrimLeft(strings.ReplaceAll(newLocale("en").amount(due), ",", ""), "") // 47658.11
	if !strings.Contains(d.qr, wantAM) || !strings.Contains(d.qr, "X-VS:20260042") || strings.Contains(d.qr, "GIBACZPX") {
		t.Fatalf("payload %q (want %s, VS, no SWIFT)", d.qr, wantAM)
	}
}

func TestVatRecap(t *testing.T) {
	inv := &model.Invoice{Lines: []model.InvoiceLine{
		{QuantityMilli: 1000, UnitPrice: 10000, VatRateBps: 1200},
		{QuantityMilli: 3000, UnitPrice: 3333, VatRateBps: 2100},
		{QuantityMilli: 1000, UnitPrice: 101, VatRateBps: 2100},
	}}
	got := vatRecap(inv)
	if len(got) != 2 || got[0] != (recapRow{2100, 10100, 2121, 12221}) || got[1] != (recapRow{1200, 10000, 1200, 11200}) {
		t.Fatalf("net: %+v", got)
	}
	inv.PricesIncludeVat = true
	got = vatRecap(inv)
	if got[0] != (recapRow{2100, 8347, 1753, 10100}) {
		t.Fatalf("gross: %+v", got)
	}
	inv.ReverseCharge = true
	if got = vatRecap(inv); got[0].vat != 0 || got[0].base != got[0].total {
		t.Fatalf("reverse charge: %+v", got)
	}
}

func TestLocaleFormatting(t *testing.T) {
	cases := []struct {
		lang                          string
		money, moneyEUR, date, qty, v string
	}{
		{"cs", "−1 234 567,05 Kč", "12,50 €", "5. 3. 2026", "1 500,25", "12,5 %"},
		{"sk", "−1 234 567,05 Kč", "12,50 €", "5. 3. 2026", "1 500,25", "12,5 %"},
		{"en", "−1,234,567.05 CZK", "€12.50", "5 Mar 2026", "1,500.25", "12.5%"},
		{"de", "−1.234.567,05 CZK", "12,50 €", "05.03.2026", "1.500,25", "12,5 %"},
	}
	for _, c := range cases {
		l := newLocale(c.lang)
		n := func(s string) string { return strings.ReplaceAll(s, " ", " ") }
		got := []string{n(l.money(-123456705, "CZK")), n(l.money(1250, "eur")), n(l.date("2026-03-05")),
			n(l.quantity(1500250)), n(l.vatRate(1250))}
		want := []string{c.money, c.moneyEUR, c.date, c.qty, c.v}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: got %q, want %q", c.lang, got[i], want[i])
			}
		}
	}
	if q := newLocale("cs").quantity(-2000); q != "−2" {
		t.Errorf("quantity -2: %q", q)
	}
	if newLocale("xx").lang != "cs" {
		t.Error("unknown language must fall back to cs")
	}
	if formatIBAN("cz6508000000192000145399") != "CZ65 0800 0000 1920 0014 5399" {
		t.Error("formatIBAN")
	}
}

func TestTranslationsComplete(t *testing.T) {
	for key, byLang := range translations {
		for _, l := range Languages {
			if byLang[l] == "" {
				t.Errorf("missing %s translation of %q", l, key)
			}
		}
	}
}
