package pdf

// Moderní šablona – čistý design s barevným pruhem vlevo a lehkou tabulkou.

import (
	"fmt"

	"github.com/johnfercher/maroto/v2/pkg/components/code"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/qwerin/nanofaktura/internal/models"
)

var modernAccent  = props.Color{Red: 124, Green: 58, Blue: 237}  // violet-600
var modernAccentL = props.Color{Red: 237, Green: 233, Blue: 254} // violet-100
var modernGray    = props.Color{Red: 71, Green: 85, Blue: 105}   // slate-600
var modernLight   = props.Color{Red: 248, Green: 250, Blue: 252} // slate-50
var modernBorder  = props.Color{Red: 226, Green: 232, Blue: 240} // slate-200

func generateModern(m core.Maroto, inv *models.Invoice, spaydStr string) {
	modernHeader(m, inv)
	modernParties(m, inv)
	modernItems(m, inv)
	modernTotals(m, inv, spaydStr)
}

func modernHeader(m core.Maroto, inv *models.Invoice) {
	// Accent strip + large invoice number
	m.AddRows(
		row.New(22).Add(
			// Left violet stripe
			col.New(1).WithStyle(&props.Cell{BackgroundColor: &modernAccent}).Add(
				text.New("", props.Text{}),
			),
			// Title area
			col.New(7).Add(
				text.New("Faktura", props.Text{
					Size: 24, Style: fontstyle.Bold, Color: &modernAccent, Top: 5,
				}),
			),
			// Number right
			col.New(4).Add(
				text.New(inv.Number, props.Text{
					Size: 11, Style: fontstyle.Bold, Align: align.Right, Color: &modernGray, Top: 8,
				}),
			),
		),
	)

	// Date bar
	issued := ""
	due := ""
	if inv.IssuedOn != "" {
		issued = "Vystaveno: " + formatDate(inv.IssuedOn)
	}
	if d := dueDateStr(inv.IssuedOn, inv.Due); d != "" {
		due = "Splatnost: " + d
	}
	duzp := ""
	if inv.TaxableFulfillmentDue != "" && inv.TaxableFulfillmentDue != inv.IssuedOn {
		duzp = "DUZP: " + formatDate(inv.TaxableFulfillmentDue)
	}
	m.AddRows(
		row.New(6).WithStyle(&props.Cell{BackgroundColor: &modernAccentL}).Add(
			col.New(1).WithStyle(&props.Cell{BackgroundColor: &modernAccent}).Add(text.New("", props.Text{})),
			col.New(4).Add(text.New(issued, props.Text{Size: 8, Color: &modernGray, Top: 1.5})),
			col.New(4).Add(text.New(due, props.Text{Size: 8, Color: &modernGray, Top: 1.5})),
			col.New(3).Add(text.New(duzp, props.Text{Size: 8, Color: &modernGray, Top: 1.5})),
		),
		row.New(6),
	)
}

func modernParties(m core.Maroto, inv *models.Invoice) {
	m.AddRows(
		row.New(6).Add(
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New("DODAVATEL", props.Text{Size: 7, Style: fontstyle.Bold, Color: &modernAccent})),
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New("ODBĚRATEL", props.Text{Size: 7, Style: fontstyle.Bold, Color: &modernAccent})),
		),
		row.New(7).Add(
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New(inv.YourName, props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New(inv.ClientName, props.Text{Size: 10, Style: fontstyle.Bold})),
		),
	)

	yourIco := formatIcoDic(inv.YourRegistrationNo, inv.YourVatNo)
	clientIco := formatIcoDic(inv.ClientRegistrationNo, inv.ClientVatNo)
	if yourIco != "" || clientIco != "" {
		m.AddRows(row.New(5).Add(
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New(yourIco, props.Text{Size: 8, Color: &modernGray})),
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New(clientIco, props.Text{Size: 8, Color: &modernGray})),
		))
	}

	yourAddr := formatAddress(inv.YourStreet, inv.YourZip, inv.YourCity)
	clientAddr := formatAddress(inv.ClientStreet, inv.ClientZip, inv.ClientCity)
	m.AddRows(
		row.New(5).Add(
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New(yourAddr, props.Text{Size: 8, Color: &modernGray})),
			col.New(1).Add(text.New("", props.Text{})),
			col.New(5).Add(text.New(clientAddr, props.Text{Size: 8, Color: &modernGray})),
		),
		row.New(8),
	)
}

func modernItems(m core.Maroto, inv *models.Invoice) {
	hdrStyle := &props.Cell{BorderType: border.Bottom, BorderColor: &modernAccent, BorderThickness: 1.2}
	if inv.VatExempt {
		m.AddRows(row.New(7).WithStyle(hdrStyle).Add(
			col.New(6).Add(text.New("Popis", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray})),
			col.New(2).Add(text.New("Množství", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(2).Add(text.New("Cena/jed.", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(2).Add(text.New("Celkem", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
		))
		for i, l := range inv.Lines {
			bg := &white
			if i%2 == 1 {
				bg = &modernLight
			}
			qty := fmt.Sprintf("%s %s", l.Quantity, l.UnitName)
			m.AddRows(row.New(7).WithStyle(&props.Cell{BackgroundColor: bg}).Add(
				col.New(6).Add(text.New(l.Name, props.Text{Size: 8, Top: 2})),
				col.New(2).Add(text.New(qty, props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.UnitPriceHal), props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.TotalHal), props.Text{Size: 8, Align: align.Right, Top: 2})),
			))
		}
	} else {
		m.AddRows(row.New(7).WithStyle(hdrStyle).Add(
			col.New(4).Add(text.New("Popis", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray})),
			col.New(2).Add(text.New("Množství", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(2).Add(text.New("Cena/jed.", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(1).Add(text.New("Sazba", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(1).Add(text.New("Základ", props.Text{Size: 7, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(1).Add(text.New("DPH", props.Text{Size: 8, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
			col.New(1).Add(text.New("Celkem", props.Text{Size: 7, Style: fontstyle.Bold, Color: &modernGray, Align: align.Right})),
		))
		for i, l := range inv.Lines {
			bg := &white
			if i%2 == 1 {
				bg = &modernLight
			}
			qty := fmt.Sprintf("%s %s", l.Quantity, l.UnitName)
			m.AddRows(row.New(7).WithStyle(&props.Cell{BackgroundColor: bg}).Add(
				col.New(4).Add(text.New(l.Name, props.Text{Size: 8, Top: 2})),
				col.New(2).Add(text.New(qty, props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(2).Add(text.New(formatHal(l.UnitPriceHal), props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(1).Add(text.New(vatRateLabel(l.VatRateBps), props.Text{Size: 8, Align: align.Right, Top: 2})),
				col.New(1).Add(text.New(formatHal(l.TotalPriceWithoutVatHal), props.Text{Size: 7, Align: align.Right, Top: 2})),
				col.New(1).Add(text.New(formatHal(l.TotalVatHal), props.Text{Size: 7, Align: align.Right, Top: 2})),
				col.New(1).Add(text.New(formatHal(l.TotalHal), props.Text{Size: 7, Align: align.Right, Top: 2})),
			))
		}
	}
	m.AddRows(row.New(4))
}

func modernTotals(m core.Maroto, inv *models.Invoice, spaydStr string) {
	hasQR := spaydStr != ""

	if inv.TransferredTaxLiability {
		m.AddRows(row.New(6).Add(col.New(12).Add(text.New(
			"Přenesená daňová povinnost – DPH odvede odběratel.",
			props.Text{Size: 8, Color: &modernGray, Align: align.Right},
		))))
	} else if !inv.VatExempt {
		for _, g := range vatGroups(inv) {
			label := fmt.Sprintf("%d %%", g.RateBps/100)
			m.AddRows(
				row.New(5).Add(
					col.New(10).Add(text.New("Základ DPH "+label, props.Text{Size: 8, Color: &modernGray, Align: align.Right})),
					col.New(2).Add(text.New(formatHal(g.BaseHal)+" Kč", props.Text{Size: 8, Align: align.Right})),
				),
				row.New(5).Add(
					col.New(10).Add(text.New("DPH "+label, props.Text{Size: 8, Color: &modernGray, Align: align.Right})),
					col.New(2).Add(text.New(formatHal(g.VatHal)+" Kč", props.Text{Size: 8, Align: align.Right})),
				),
			)
		}
	}

	// QR + total side by side
	if hasQR {
		m.AddRows(
			row.New(24).WithStyle(&props.Cell{
				BorderType: border.Top, BorderColor: &modernAccent, BorderThickness: 1.2,
			}).Add(
				col.New(3).Add(code.NewQr(spaydStr, props.Rect{Percent: 90, Center: true})),
				col.New(5).Add(text.New("Platba QR kódem", props.Text{Size: 7, Color: &modernGray, Top: 8})),
				col.New(2).Add(text.New("CELKEM", props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right, Top: 9})),
				col.New(2).Add(text.New(formatHal(inv.Total)+" Kč", props.Text{
					Size: 11, Style: fontstyle.Bold, Align: align.Right, Color: &modernAccent, Top: 8,
				})),
			),
		)
	} else {
		m.AddRows(
			row.New(10).WithStyle(&props.Cell{
				BorderType: border.Top, BorderColor: &modernAccent, BorderThickness: 1.2,
			}).Add(
				col.New(10).Add(text.New("CELKEM K ÚHRADĚ", props.Text{
					Size: 10, Style: fontstyle.Bold, Align: align.Right, Top: 3,
				})),
				col.New(2).Add(text.New(formatHal(inv.Total)+" Kč", props.Text{
					Size: 11, Style: fontstyle.Bold, Align: align.Right, Color: &modernAccent, Top: 3,
				})),
			),
		)
	}

	// Payment footer
	paymentLine := "Bankovní účet: " + inv.BankAccount
	if inv.VariableSymbol != "" {
		paymentLine += "   VS: " + inv.VariableSymbol
	}
	m.AddRows(
		row.New(5),
		row.New(5).Add(col.New(12).Add(text.New(
			paymentLine,
			props.Text{Size: 8, Color: &modernGray},
		))),
	)
	if inv.VatExempt {
		m.AddRows(row.New(5).Add(col.New(12).Add(text.New(
			"Fyzická osoba není plátcem DPH.",
			props.Text{Size: 7, Color: &modernGray},
		))))
	}
	if inv.Note != "" {
		m.AddRows(row.New(5).Add(col.New(12).Add(text.New(
			"Poznámka: "+inv.Note,
			props.Text{Size: 8, Color: &modernGray},
		))))
	}
}
