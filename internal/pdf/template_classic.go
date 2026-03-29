package pdf

// Klasická šablona – design dle reference: FAKTURA + číslo, DAŇOVÝ DOKLAD,
// pruhy dodavatel/odběratel, info bar, tabulka s DPH sloupci, DPH sumář dole.

import (
	"fmt"

	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/qwerin/nanofaktura/internal/models"
)

var headerBg = props.Color{Red: 30, Green: 41, Blue: 59}    // slate-800 (unused, kept for compat)
var accentBg = props.Color{Red: 124, Green: 58, Blue: 237}  // violet-600
var lightBg  = props.Color{Red: 248, Green: 250, Blue: 252} // slate-50
var white    = props.Color{Red: 255, Green: 255, Blue: 255}
var black    = props.Color{Red: 15, Green: 23, Blue: 42}
var muted    = props.Color{Red: 100, Green: 116, Blue: 139}

func generateClassic(m core.Maroto, inv *models.Invoice, qrBytes []byte) {
	classicHeader(m, inv)
	classicParties(m, inv)
	classicInfoBar(m, inv)
	classicItems(m, inv)
	classicTotals(m, inv, qrBytes)
}

// ── Header ──────────────────────────────────────────────────────────────────

func classicHeader(m core.Maroto, inv *models.Invoice) {
	// "FAKTURA [číslo]" na jednom řádku
	m.AddRows(
		row.New(18).Add(
			col.New(4).Add(text.New("FAKTURA", props.Text{
				Size: 22, Style: fontstyle.Bold, Color: &black, Top: 5,
			})),
			col.New(5).Add(text.New(inv.Number, props.Text{
				Size: 18, Style: fontstyle.Bold, Color: &accentBg, Top: 6,
			})),
			col.New(3).Add(text.New("", props.Text{})), // prostor pro logo
		),
	)

	// Subtitle: typ dokladu
	subtitle := ""
	switch inv.DocumentType {
	case models.DocProforma:
		subtitle = "ZÁLOHOVÁ FAKTURA"
	case models.DocCorrection:
		subtitle = "OPRAVNÝ DAŇOVÝ DOKLAD"
	case models.DocTaxDoc:
		subtitle = "DAŇOVÝ DOKLAD K PŘIJATÉ PLATBĚ"
	default:
		if !inv.VatExempt {
			subtitle = "DAŇOVÝ DOKLAD"
		}
	}
	if subtitle != "" {
		m.AddRows(row.New(5).Add(
			col.New(12).Add(text.New(subtitle, props.Text{Size: 8, Color: &muted})),
		))
	}

	// Violet oddělovací linka
	m.AddRows(
		row.New(3).Add(col.New(12).Add(line.New(props.Line{Color: &accentBg, Thickness: 0.9}))),
	)
}

// ── Parties ─────────────────────────────────────────────────────────────────

func classicParties(m core.Maroto, inv *models.Invoice) {
	m.AddRows(row.New(5))

	leftBorder := &props.Cell{BorderType: border.Left, BorderColor: &accentBg, BorderThickness: 3}

	// DODAVATEL / ODBĚRATEL labels
	m.AddRows(row.New(5).Add(
		col.New(6).WithStyle(leftBorder).Add(
			text.New("DODAVATEL", props.Text{Size: 7, Style: fontstyle.Bold, Color: &accentBg, Left: 3, Top: 0.5}),
		),
		col.New(6).WithStyle(leftBorder).Add(
			text.New("ODBĚRATEL", props.Text{Size: 7, Style: fontstyle.Bold, Color: &accentBg, Left: 3, Top: 0.5}),
		),
	))

	// Jména
	m.AddRows(row.New(7).Add(
		col.New(6).WithStyle(leftBorder).Add(
			text.New(inv.YourName, props.Text{Size: 10, Style: fontstyle.Bold, Left: 3, Top: 1}),
		),
		col.New(6).WithStyle(leftBorder).Add(
			text.New(inv.ClientName, props.Text{Size: 10, Style: fontstyle.Bold, Left: 3, Top: 1}),
		),
	))

	// Adresy
	yourAddr := formatAddress(inv.YourStreet, inv.YourZip, inv.YourCity)
	clientAddr := formatAddress(inv.ClientStreet, inv.ClientZip, inv.ClientCity)
	if yourAddr != "" || clientAddr != "" {
		m.AddRows(row.New(5).Add(
			col.New(6).WithStyle(leftBorder).Add(
				text.New(yourAddr, props.Text{Size: 8, Color: &muted, Left: 3}),
			),
			col.New(6).WithStyle(leftBorder).Add(
				text.New(clientAddr, props.Text{Size: 8, Color: &muted, Left: 3}),
			),
		))
	}

	// IČO
	if inv.YourRegistrationNo != "" || inv.ClientRegistrationNo != "" {
		yourIco := ""
		if inv.YourRegistrationNo != "" {
			yourIco = "IČO   " + inv.YourRegistrationNo
		}
		clientIco := ""
		if inv.ClientRegistrationNo != "" {
			clientIco = "IČO   " + inv.ClientRegistrationNo
		}
		m.AddRows(row.New(5).Add(
			col.New(6).WithStyle(leftBorder).Add(
				text.New(yourIco, props.Text{Size: 8, Color: &muted, Left: 3}),
			),
			col.New(6).WithStyle(leftBorder).Add(
				text.New(clientIco, props.Text{Size: 8, Color: &muted, Left: 3}),
			),
		))
	}

	// DIČ
	if inv.YourVatNo != "" || inv.ClientVatNo != "" {
		yourDic := ""
		if inv.YourVatNo != "" {
			yourDic = "DIČ   " + inv.YourVatNo
		}
		clientDic := ""
		if inv.ClientVatNo != "" {
			clientDic = "DIČ   " + inv.ClientVatNo
		}
		m.AddRows(row.New(5).Add(
			col.New(6).WithStyle(leftBorder).Add(
				text.New(yourDic, props.Text{Size: 8, Color: &muted, Left: 3}),
			),
			col.New(6).WithStyle(leftBorder).Add(
				text.New(clientDic, props.Text{Size: 8, Color: &muted, Left: 3}),
			),
		))
	}

	m.AddRows(row.New(6))
}

// ── Info bar (data + platba) ─────────────────────────────────────────────────

func classicInfoBar(m core.Maroto, inv *models.Invoice) {
	thinLine := props.Line{Color: &muted, Thickness: 0.25}
	m.AddRows(row.New(2).Add(col.New(12).Add(line.New(thinLine))))

	issued   := formatDate(inv.IssuedOn)
	splatnost := dueDateStr(inv.IssuedOn, inv.Due)
	duzp := ""
	if inv.TaxableFulfillmentDue != "" {
		duzp = formatDate(inv.TaxableFulfillmentDue)
	}

	// Řádek 1: labely dat
	m.AddRows(row.New(5).Add(
		col.New(4).Add(text.New("Datum vystavení", props.Text{Size: 7, Color: &muted, Top: 1.5})),
		col.New(4).Add(text.New("Datum splatnosti", props.Text{Size: 7, Color: &muted, Top: 1.5})),
		col.New(4).Add(text.New("Datum zdan. plnění", props.Text{Size: 7, Color: &muted, Top: 1.5})),
	))
	// Řádek 2: hodnoty dat
	m.AddRows(row.New(6).Add(
		col.New(4).Add(text.New(issued, props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(4).Add(text.New(splatnost, props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(4).Add(text.New(duzp, props.Text{Size: 9, Style: fontstyle.Bold})),
	))

	m.AddRows(row.New(3))

	// Řádek 3: labely platby
	m.AddRows(row.New(5).Add(
		col.New(4).Add(text.New("Bankovní účet", props.Text{Size: 7, Color: &muted, Top: 1.5})),
		col.New(4).Add(text.New("Variabilní symbol", props.Text{Size: 7, Color: &muted, Top: 1.5})),
		col.New(4).Add(text.New("Způsob platby", props.Text{Size: 7, Color: &muted, Top: 1.5})),
	))
	// Řádek 4: hodnoty platby
	m.AddRows(row.New(6).Add(
		col.New(4).Add(text.New(inv.BankAccount, props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(4).Add(text.New(inv.VariableSymbol, props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(4).Add(text.New(paymentMethodLabel(inv.PaymentMethod), props.Text{Size: 9, Style: fontstyle.Bold})),
	))

	m.AddRows(row.New(2).Add(col.New(12).Add(line.New(thinLine))))
	m.AddRows(row.New(6))
}

// ── Items ────────────────────────────────────────────────────────────────────

func classicItems(m core.Maroto, inv *models.Invoice) {
	m.AddRows(row.New(6).Add(col.New(12).Add(
		text.New("Fakturujeme Vám následující položky:", props.Text{Size: 8, Color: &muted, Style: fontstyle.Italic}),
	)))

	hdrStyle := &props.Cell{BackgroundColor: &lightBg}
	rowBorder := &props.Color{Red: 226, Green: 232, Blue: 240}

	if inv.VatExempt {
		// Neplátce: Počet | MJ | Popis | Cena/MJ | Celkem
		m.AddRows(row.New(7).WithStyle(hdrStyle).Add(
			col.New(1).Add(text.New("Počet", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Top: 2})),
			col.New(1).Add(text.New("MJ", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Top: 2})),
			col.New(6).Add(text.New("Popis", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Top: 2})),
			col.New(2).Add(text.New("Cena za MJ", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 2})),
			col.New(2).Add(text.New("Celkem", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 2})),
		))
		for i, l := range inv.Lines {
			bg := &white
			if i%2 == 1 {
				bg = &lightBg
			}
			m.AddRows(row.New(7).WithStyle(&props.Cell{BackgroundColor: bg, BorderType: border.Bottom, BorderColor: rowBorder}).Add(
				col.New(1).Add(text.New(l.Quantity, props.Text{Size: 8, Top: 2})),
				col.New(1).Add(text.New(l.UnitName, props.Text{Size: 8, Color: &muted, Top: 2})),
				col.New(6).Add(text.New(l.Name, props.Text{Size: 8, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.UnitPriceHal)+" Kč", props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.TotalHal)+" Kč", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right, Top: 2})),
			))
		}
	} else {
		// Plátce DPH: Počet | MJ | Popis | DPH | Cena za MJ | Celkem bez DPH
		m.AddRows(row.New(7).WithStyle(hdrStyle).Add(
			col.New(1).Add(text.New("Počet", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Top: 2})),
			col.New(1).Add(text.New("MJ", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Top: 2})),
			col.New(5).Add(text.New("Popis", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Top: 2})),
			col.New(1).Add(text.New("DPH", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 2})),
			col.New(2).Add(text.New("Cena za MJ", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 2})),
			col.New(2).Add(text.New("Celkem bez DPH", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 2})),
		))
		for i, l := range inv.Lines {
			bg := &white
			if i%2 == 1 {
				bg = &lightBg
			}
			m.AddRows(row.New(7).WithStyle(&props.Cell{BackgroundColor: bg, BorderType: border.Bottom, BorderColor: rowBorder}).Add(
				col.New(1).Add(text.New(l.Quantity, props.Text{Size: 8, Top: 2})),
				col.New(1).Add(text.New(l.UnitName, props.Text{Size: 8, Color: &muted, Top: 2})),
				col.New(5).Add(text.New(l.Name, props.Text{Size: 8, Top: 2})),
				col.New(1).Add(text.New(vatRateLabel(l.VatRateBps), props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.UnitPriceHal)+" Kč", props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.TotalPriceWithoutVatHal)+" Kč", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right, Top: 2})),
			))
		}
	}
	m.AddRows(row.New(5))
}

// ── Totals ───────────────────────────────────────────────────────────────────

func classicTotals(m core.Maroto, inv *models.Invoice, qrBytes []byte) {
	// DPH sumář pro plátce (tabulka SAZBA | ZÁKLAD | DPH)
	if !inv.VatExempt && !inv.TransferredTaxLiability {
		m.AddRows(row.New(6).WithStyle(&props.Cell{BackgroundColor: &lightBg}).Add(
			col.New(5).Add(text.New("", props.Text{})),
			col.New(2).Add(text.New("SAZBA", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 1.5})),
			col.New(3).Add(text.New("ZÁKLAD", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 1.5})),
			col.New(2).Add(text.New("DPH", props.Text{Size: 7, Style: fontstyle.Bold, Color: &muted, Align: align.Right, Top: 1.5})),
		))
		for _, g := range vatGroups(inv) {
			m.AddRows(row.New(5).Add(
				col.New(5).Add(text.New("", props.Text{})),
				col.New(2).Add(text.New(vatRateLabel(g.RateBps), props.Text{Size: 8, Align: align.Right, Top: 1})),
				col.New(3).Add(text.New(formatHal(g.BaseHal)+" Kč", props.Text{Size: 8, Align: align.Right, Top: 1})),
				col.New(2).Add(text.New(formatHal(g.VatHal)+" Kč", props.Text{Size: 8, Align: align.Right, Top: 1})),
			))
		}
	}

	// Speciální poznámky
	if inv.TransferredTaxLiability {
		m.AddRows(row.New(6).Add(col.New(12).Add(text.New(
			"Přenesená daňová povinnost – DPH odvede odběratel.",
			props.Text{Size: 8, Color: &muted, Align: align.Right},
		))))
	}
	if inv.VatExempt {
		m.AddRows(row.New(6).Add(col.New(12).Add(text.New(
			"Fyzická osoba není plátcem DPH.",
			props.Text{Size: 8, Color: &muted, Align: align.Right},
		))))
	}

	// Celková částka
	m.AddRows(
		row.New(3).Add(col.New(12).Add(line.New(props.Line{Color: &accentBg, Thickness: 0.9}))),
		row.New(11).Add(
			col.New(7).Add(text.New("CELKEM K ÚHRADĚ", props.Text{
				Size: 11, Style: fontstyle.Bold, Align: align.Right, Top: 3,
			})),
			col.New(5).Add(text.New(formatHal(inv.Total)+" Kč", props.Text{
				Size: 13, Style: fontstyle.Bold, Align: align.Right, Color: &accentBg, Top: 2,
			})),
		),
	)

	// Poznámka
	if inv.Note != "" {
		m.AddRows(
			row.New(3),
			row.New(5).Add(col.New(12).Add(text.New(
				"Poznámka: "+inv.Note, props.Text{Size: 8, Color: &muted},
			))),
		)
	}

	// Footer: QR vlevo, podpisová linka vpravo
	hasQR := len(qrBytes) > 0
	if hasQR {
		m.AddRows(
			row.New(5),
			row.New(26).Add(
				col.New(3).Add(image.NewFromBytes(qrBytes, extension.Png, props.Rect{Percent: 92, Center: true})),
				col.New(3).Add(text.New("QR Platba", props.Text{Size: 7, Color: &muted, Top: 22})),
				col.New(6).Add(text.New("", props.Text{})),
			),
		)
	}

	if inv.FooterNote != "" {
		m.AddRows(row.New(5).Add(col.New(12).Add(text.New(
			inv.FooterNote, props.Text{Size: 7, Color: &muted},
		))))
	}

	// Neplátce DPH info
	_ = fmt.Sprintf // suppress unused import if needed
}
