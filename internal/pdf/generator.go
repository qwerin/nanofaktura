// Package pdf generuje vektorové PDF faktury pomocí knihovny Maroto v2.
// Podporuje 3 šablony (classic, modern, minimal) a QR platbu (SPAYD 1.0).
package pdf

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/johnfercher/maroto/v2/pkg/repository"

	"github.com/qwerin/nanofaktura/internal/models"
)

//go:embed fonts/DejaVuSans.ttf
var fontRegular []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var fontBold []byte

//go:embed fonts/DejaVuSans-Oblique.ttf
var fontItalic []byte

//go:embed fonts/DejaVuSans-BoldOblique.ttf
var fontBoldItalic []byte

const fontFamily = "DejaVu"

// InvoiceRequest jsou vstupní data pro generátor PDF.
type InvoiceRequest struct {
	Invoice  *models.Invoice
	Template string // "classic" | "modern" | "minimal"; prázdný = "classic"
}

// Generate vytvoří PDF faktury a vrátí bajty připravené k odeslání nebo uložení.
func Generate(req InvoiceRequest) ([]byte, error) {
	inv := req.Invoice

	tmpl := req.Template
	if tmpl == "" {
		tmpl = "classic"
	}

	fonts, err := repository.New().
		AddUTF8FontFromBytes(fontFamily, fontstyle.Normal, fontRegular).
		AddUTF8FontFromBytes(fontFamily, fontstyle.Bold, fontBold).
		AddUTF8FontFromBytes(fontFamily, fontstyle.Italic, fontItalic).
		AddUTF8FontFromBytes(fontFamily, fontstyle.BoldItalic, fontBoldItalic).
		Load()
	if err != nil {
		return nil, fmt.Errorf("pdf: načtení fontu: %w", err)
	}

	cfg := config.NewBuilder().
		WithCustomFonts(fonts).
		WithDefaultFont(&props.Font{Family: fontFamily}).
		WithPageNumber(props.PageNumber{
			Pattern: "Strana {current}/{total}",
			Place:   props.Bottom,
		}).
		Build()

	m := maroto.New(cfg)

	spaydStr := Spayd(inv)

	switch tmpl {
	case "modern":
		generateModern(m, inv, spaydStr)
	case "minimal":
		generateMinimal(m, inv, spaydStr)
	default:
		generateClassic(m, inv, spaydStr)
	}

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("pdf: generování: %w", err)
	}
	return doc.GetBytes(), nil
}

// ── Sdílené helper funkce ────────────────────────────────────────────────────

// formatHal převede haléře na čitelný řetězec ve formátu "1 234,56".
func formatHal(hal int64) string {
	kc := hal / 100
	haler := hal % 100
	if haler < 0 {
		haler = -haler
	}
	return fmt.Sprintf("%d,%02d", kc, haler)
}

// formatDate converts YYYY-MM-DD → DD.MM.YYYY.
func formatDate(iso string) string {
	if len(iso) != 10 {
		return iso
	}
	parts := strings.Split(iso, "-")
	if len(parts) != 3 {
		return iso
	}
	return parts[2] + "." + parts[1] + "." + parts[0]
}

// formatIcoDic returns "IČO: ... DIČ: ..." or individual parts.
func formatIcoDic(ico, dic string) string {
	var parts []string
	if ico != "" {
		parts = append(parts, "IČO: "+ico)
	}
	if dic != "" {
		parts = append(parts, "DIČ: "+dic)
	}
	return strings.Join(parts, "  ")
}

// dueDateStr vrátí datum splatnosti jako "DD.MM.YYYY" (issued_on + due dnů).
func dueDateStr(issuedOn string, due int) string {
	t, err := time.Parse("2006-01-02", issuedOn)
	if err != nil || issuedOn == "" {
		return ""
	}
	return t.AddDate(0, 0, due).Format("02.01.2006")
}

// vatRateLabel vrátí "21 %" pro 2100, "0 %" pro 0 atd.
func vatRateLabel(bps int32) string {
	return fmt.Sprintf("%d %%", bps/100)
}

// VatGroup je součet položek se stejnou sazbou DPH.
type VatGroup struct {
	RateBps  int32
	BaseHal  int64
	VatHal   int64
	TotalHal int64
}

// vatGroups seskupí řádky faktury dle sazby DPH, seřazené sestupně dle sazby.
func vatGroups(inv *models.Invoice) []VatGroup {
	m := make(map[int32]*VatGroup)
	for _, l := range inv.Lines {
		g, ok := m[l.VatRateBps]
		if !ok {
			g = &VatGroup{RateBps: l.VatRateBps}
			m[l.VatRateBps] = g
		}
		g.BaseHal += l.TotalPriceWithoutVatHal
		g.VatHal += l.TotalVatHal
		g.TotalHal += l.TotalHal
	}
	// seřadit sestupně dle sazby
	groups := make([]VatGroup, 0, len(m))
	for _, g := range m {
		groups = append(groups, *g)
	}
	for i := 0; i < len(groups)-1; i++ {
		for j := i + 1; j < len(groups); j++ {
			if groups[j].RateBps > groups[i].RateBps {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}
	return groups
}

// paymentMethodLabel vrátí českou zkratku způsobu platby.
func paymentMethodLabel(m models.PaymentMethod) string {
	switch m {
	case models.PaymentBank:
		return "Převodem"
	case models.PaymentCash:
		return "Hotovost"
	case models.PaymentCard:
		return "Kartou"
	case models.PaymentCOD:
		return "Dobírka"
	default:
		return "Jiná"
	}
}

// formatAddress combines street and city/zip.
func formatAddress(street, zip, city string) string {
	var parts []string
	if street != "" {
		parts = append(parts, street)
	}
	cityZip := strings.TrimSpace(zip + " " + city)
	if cityZip != "" {
		parts = append(parts, cityZip)
	}
	return strings.Join(parts, ", ")
}
