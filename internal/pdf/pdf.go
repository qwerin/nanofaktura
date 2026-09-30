// Package pdf renders invoices (all document types) to PDF using Maroto v2
// with embedded DejaVu fonts (full Czech/Slovak/German diacritics).
//
// Three templates share one content model and differ in styling:
//
//   - classic — accent rule under the title, filled accent table header, zebra rows,
//     accent "total due" box;
//   - modern  — full-bleed accent band, tinted party cards, key-facts strip,
//     payment details next to the QR code;
//   - minimal — monochrome, hairlines only, accent used for the amount due.
//
// Money is formatted from int64 minor units, never floats.
package pdf

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/pagesize"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"github.com/johnfercher/maroto/v2/pkg/fontrepository"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// Templates.
const (
	TemplateClassic = "classic"
	TemplateModern  = "modern"
	TemplateMinimal = "minimal"
)

// Templates lists the available template names.
var Templates = []string{TemplateClassic, TemplateModern, TemplateMinimal}

// ErrUnknownTemplate is returned by Render for an unsupported template name.
var ErrUnknownTemplate = errors.New("pdf: unknown template")

// Options controls the look of the rendered document.
type Options struct {
	Template string // classic (default) | modern | minimal
	Accent   string // "#RRGGBB"; empty or invalid = template default
	Language string // cs | en | sk | de; empty = invoice.Language → account default → cs
	Logo     []byte // optional PNG/JPEG shown in the header
	Stamp    []byte // optional PNG/JPEG stamp/signature shown under the totals
	ShowQR   bool   // QR Platba (only CZK, Czech IBAN, bank transfer, amount due > 0)
	Footer   string // extra footer text of the account (every page, after the registration note)

	// RelatedNumber is the number of the document referenced by RelatedID
	// (the corrected invoice for a correction, the proforma for a final invoice).
	RelatedNumber string
}

//go:embed fonts/DejaVuSans.ttf
var fontRegular []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var fontBold []byte

//go:embed fonts/DejaVuSans-Oblique.ttf
var fontItalic []byte

//go:embed fonts/DejaVuSans-BoldOblique.ttf
var fontBoldItalic []byte

const fontFamily = "dejavu"

var (
	fontsOnce sync.Once
	fonts     []entity.CustomFont
	fontsErr  error
)

func loadFonts() ([]entity.CustomFont, error) {
	fontsOnce.Do(func() {
		fonts, fontsErr = fontrepository.New().
			AddUTF8FontFromBytes(fontFamily, fontstyle.Normal, fontRegular).
			AddUTF8FontFromBytes(fontFamily, fontstyle.Bold, fontBold).
			AddUTF8FontFromBytes(fontFamily, fontstyle.Italic, fontItalic).
			AddUTF8FontFromBytes(fontFamily, fontstyle.BoldItalic, fontBoldItalic).
			Load()
	})
	return fonts, fontsErr
}

// Page geometry (mm).
const (
	pageW, pageH = 210.0, 297.0
	marginL      = 16.0
	marginR      = 16.0
	marginT      = 14.0
	marginB      = 16.0
	contentW     = pageW - marginL - marginR
	gridSize     = 24
)

// Render renders inv (with Lines and Payments loaded) as a PDF document.
// acc supplies contact details for the footer and fallbacks; it may be nil.
func Render(inv *model.Invoice, acc *model.Account, opt Options) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("pdf: nil invoice")
	}
	if opt.Template == "" {
		opt.Template = TemplateClassic
	}
	th, ok := themeFor(opt.Template)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownTemplate, opt.Template)
	}
	if c, ok := parseHex(opt.Accent); ok {
		th = th.withAccent(c)
	}

	// images are validated (dimensions) and re-encoded before the PDF library
	// parses them; invalid or oversized ones are left out
	opt.Logo, opt.Stamp = normalizeImage(opt.Logo), normalizeImage(opt.Stamp)
	d := newDoc(inv, acc, opt)

	fs, err := loadFonts()
	if err != nil {
		return nil, fmt.Errorf("pdf: load fonts: %w", err)
	}
	cfs := make([]entity.CustomFont, len(fs))
	copy(cfs, fs)

	muted := th.muted
	cfg := config.NewBuilder().
		WithPageSize(pagesize.A4).
		WithLeftMargin(marginL).WithRightMargin(marginR).
		WithTopMargin(marginT).WithBottomMargin(marginB).
		WithMaxGridSize(gridSize).
		WithCustomFonts(cfs).
		WithDefaultFont(&props.Font{Family: fontFamily, Style: fontstyle.Normal, Size: 9, Color: &th.ink}).
		WithPageNumber(props.PageNumber{
			Pattern: d.l.t(lPage) + " {current} / {total}",
			Place:   props.RightBottom, Family: fontFamily, Size: 7, Color: &muted,
		}).
		WithTitle(strings.TrimSpace(d.title+" "+inv.Number), true).
		WithAuthor(d.supplier.name, true).
		WithSubject(d.title, true).
		WithCreator("NanoFaktura", false).
		WithCompression(true).
		Build()

	r := &renderer{m: maroto.New(cfg), d: d, th: th, opt: opt}
	if err := r.build(); err != nil {
		return nil, err
	}
	out, err := r.m.Generate()
	if err != nil {
		return nil, fmt.Errorf("pdf: generate: %w", err)
	}
	b := out.GetBytes()
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		return nil, errors.New("pdf: generator returned invalid output")
	}
	return b, nil
}

// party is a formatted supplier/customer block.
type party struct {
	name, fullName, street, cityLine, country string
	regNo, vatNo                              string
	vatNote                                   string
}

// recapRow is one VAT rate in the VAT recapitulation.
type recapRow struct {
	rate             int32
	base, vat, total int64
}

// doc is the invoice prepared for display (all strings localized).
type doc struct {
	inv      *model.Invoice
	acc      *model.Account
	l        locale
	payer    bool // supplier is a VAT payer → VAT columns, DUZP, recap
	title    string
	subtitle string
	supplier party
	customer party
	currency string
	paid     int64
	due      int64 // remaining amount
	recap    []recapRow
	qr       string // SPAYD payload or ""
	logoExt  string
	stampExt string
}

func newDoc(inv *model.Invoice, acc *model.Account, opt Options) *doc {
	lang := opt.Language
	if lang == "" {
		lang = inv.Language
	}
	if lang == "" && acc != nil {
		lang = acc.DefaultLanguage
	}
	d := &doc{inv: inv, acc: acc, l: newLocale(lang), currency: inv.Currency}
	if d.currency == "" {
		d.currency = "CZK"
	}
	vatMode := inv.YourVatMode
	if vatMode == "" && acc != nil {
		vatMode = acc.VatMode
	}
	d.payer = vatMode == model.VatModePayer

	// Title and subtitle per document type.
	switch inv.DocumentType {
	case model.DocProforma:
		d.title = d.l.t(lTitleProforma)
		d.subtitle = d.l.t(lSubNotTaxDoc)
	case model.DocCorrection:
		d.title = d.l.t(lTitleCorrection)
		if d.payer {
			d.title = d.l.t(lTitleCorrectionPayer)
		}
		if opt.RelatedNumber != "" {
			d.subtitle = d.l.t(lSubCorrectionOf, opt.RelatedNumber)
		}
	default:
		d.title = d.l.t(lTitleInvoice)
		if d.payer {
			d.title = d.l.t(lTitleInvoicePayer)
		}
		if opt.RelatedNumber != "" {
			d.subtitle = d.l.t(lSubRelatedProforma, opt.RelatedNumber)
		}
	}

	// Parties (snapshots on the invoice; account as a fallback for the supplier).
	sup := party{
		name: inv.YourName, street: inv.YourStreet, cityLine: joinNonEmpty(" ", inv.YourZip, inv.YourCity),
		regNo: inv.YourRegistrationNo, vatNo: inv.YourVatNo,
	}
	if sup.name == "" && acc != nil {
		sup = party{name: acc.Name, street: acc.Street, cityLine: joinNonEmpty(" ", acc.Zip, acc.City),
			regNo: acc.RegistrationNo, vatNo: acc.VatNo}
	}
	supCountry := firstNonEmpty(inv.YourCountry, "CZ")
	cliCountry := firstNonEmpty(inv.ClientCountry, supCountry)
	switch vatMode {
	case model.VatModeNonPayer, "":
		sup.vatNote = d.l.t(lNonPayer)
	case model.VatModeIdentifiedPerson:
		sup.vatNote = d.l.t(lIdentifiedPerson)
	}
	cli := party{
		name: inv.ClientName, fullName: inv.ClientFullName, street: inv.ClientStreet,
		cityLine: joinNonEmpty(" ", inv.ClientZip, inv.ClientCity),
		regNo:    inv.ClientRegistrationNo, vatNo: inv.ClientVatNo,
	}
	if !strings.EqualFold(supCountry, cliCountry) {
		sup.country = d.l.country(supCountry)
		cli.country = d.l.country(cliCountry)
	}
	d.supplier, d.customer = sup, cli

	// Payments.
	d.paid = inv.PaidAmount
	if d.paid == 0 {
		for _, p := range inv.Payments {
			d.paid += p.Amount
		}
	}
	d.due = inv.Total - d.paid

	if d.payer {
		d.recap = vatRecap(inv)
	}

	if len(opt.Logo) > 0 {
		d.logoExt = imageExt(opt.Logo)
	}
	if len(opt.Stamp) > 0 {
		d.stampExt = imageExt(opt.Stamp)
	}

	if opt.ShowQR && strings.EqualFold(d.currency, "CZK") && d.due > 0 &&
		(inv.PaymentMethod == "" || inv.PaymentMethod == "bank") &&
		inv.Status != model.StatusCancelled && inv.Status != model.StatusUncollectible {
		// SWIFT is deliberately omitted: with it some Czech banking apps treat
		// the QR as a foreign payment.
		d.qr = spayd.Build(spayd.Payment{
			IBAN:           inv.IBAN,
			Amount:         d.due,
			Currency:       "CZK",
			VariableSymbol: inv.VariableSymbol,
			DueOn:          inv.DueOn,
			Message:        strings.TrimSpace(d.l.t(lTitleInvoice) + " " + inv.Number),
			RecipientName:  sup.name,
		})
	}
	return d
}

// vatRecap computes the VAT recapitulation with the billing rules
// (VAT per rate, highest rate first).
func vatRecap(inv *model.Invoice) []recapRow {
	t, err := billing.Calculate(billingLines(inv), billingOptions(inv, true))
	if err != nil {
		return nil
	}
	rows := make([]recapRow, len(t.VatRecap))
	for i, r := range t.VatRecap {
		rows[i] = recapRow{rate: r.VatRateBps, base: r.Base, vat: r.Vat, total: r.Total}
	}
	return rows
}

func billingLines(inv *model.Invoice) []billing.Line {
	out := make([]billing.Line, len(inv.Lines))
	for i, l := range inv.Lines {
		out[i] = billing.Line{QuantityMilli: l.QuantityMilli, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps}
	}
	return out
}

func billingOptions(inv *model.Invoice, payer bool) billing.Options {
	return billing.Options{
		PricesIncludeVAT: inv.PricesIncludeVat, ReverseCharge: inv.ReverseCharge,
		RoundTotal: inv.RoundTotal, NonVATPayer: !payer,
	}
}

func imageExt(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG")):
		return "png"
	case bytes.HasPrefix(b, []byte("\xff\xd8")):
		return "jpg"
	}
	return ""
}

func parseHex(s string) (props.Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return props.Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return props.Color{}, false
	}
	return props.Color{Red: int(v >> 16 & 0xff), Green: int(v >> 8 & 0xff), Blue: int(v & 0xff)}, true
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
