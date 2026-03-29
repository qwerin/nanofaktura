package pdf

// Minimalistická šablona – bílá, jen tenké linky, vzdušné rozmístění.

import (
	"fmt"

	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
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

var minAccent = props.Color{Red: 124, Green: 58, Blue: 237}  // violet-600
var minGray   = props.Color{Red: 107, Green: 114, Blue: 128} // gray-500
var minLine   = props.Color{Red: 209, Green: 213, Blue: 219} // gray-300
var minDark   = props.Color{Red: 17, Green: 24, Blue: 39}    // gray-900

func generateMinimal(m core.Maroto, inv *models.Invoice, qrBytes []byte) {
	minimalHeader(m, inv)
	minimalParties(m, inv)
	minimalItems(m, inv)
	minimalTotals(m, inv, qrBytes)
}

func minimalHeader(m core.Maroto, inv *models.Invoice) {
	m.AddRows(
		row.New(6),
		row.New(10).Add(
			col.New(8).Add(text.New("Faktura", props.Text{
				Size: 20, Style: fontstyle.Bold, Color: &minDark,
			})),
			col.New(4).Add(text.New(inv.Number, props.Text{
				Size: 10, Align: align.Right, Color: &minGray, Top: 3,
			})),
		),
		row.New(5).WithStyle(&props.Cell{BorderType: border.Bottom, BorderColor: &minLine}).Add(
			col.New(3).Add(text.New(
				func() string {
					if inv.IssuedOn != "" {
						return "Vystaveno " + formatDate(inv.IssuedOn)
					}
					return ""
				}(),
				props.Text{Size: 8, Color: &minGray, Top: 1},
			)),
			col.New(3).Add(text.New(
				func() string {
					if d := dueDateStr(inv.IssuedOn, inv.Due); d != "" {
						return "Splatnost " + d
					}
					return ""
				}(),
				props.Text{Size: 8, Color: &minGray, Top: 1},
			)),
			col.New(3).Add(text.New(
				func() string {
					if inv.TaxableFulfillmentDue != "" && inv.TaxableFulfillmentDue != inv.IssuedOn {
						return "DUZP " + formatDate(inv.TaxableFulfillmentDue)
					}
					return ""
				}(),
				props.Text{Size: 8, Color: &minGray, Top: 1},
			)),
			col.New(3).Add(text.New(
				func() string {
					if inv.VariableSymbol != "" {
						return "VS " + inv.VariableSymbol
					}
					return ""
				}(),
				props.Text{Size: 8, Color: &minGray, Align: align.Right, Top: 1},
			)),
		),
		row.New(8),
	)
}

func minimalParties(m core.Maroto, inv *models.Invoice) {
	m.AddRows(
		row.New(6).Add(
			col.New(6).Add(text.New("Dodavatel", props.Text{Size: 7, Color: &minAccent})),
			col.New(6).Add(text.New("Odběratel", props.Text{Size: 7, Color: &minAccent})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New(inv.YourName, props.Text{Size: 9, Style: fontstyle.Bold, Color: &minDark})),
			col.New(6).Add(text.New(inv.ClientName, props.Text{Size: 9, Style: fontstyle.Bold, Color: &minDark})),
		),
	)

	yourIco := formatIcoDic(inv.YourRegistrationNo, inv.YourVatNo)
	clientIco := formatIcoDic(inv.ClientRegistrationNo, inv.ClientVatNo)
	if yourIco != "" || clientIco != "" {
		m.AddRows(row.New(5).Add(
			col.New(6).Add(text.New(yourIco, props.Text{Size: 8, Color: &minGray})),
			col.New(6).Add(text.New(clientIco, props.Text{Size: 8, Color: &minGray})),
		))
	}

	yourAddr := formatAddress(inv.YourStreet, inv.YourZip, inv.YourCity)
	clientAddr := formatAddress(inv.ClientStreet, inv.ClientZip, inv.ClientCity)
	m.AddRows(
		row.New(5).Add(
			col.New(6).Add(text.New(yourAddr, props.Text{Size: 8, Color: &minGray})),
			col.New(6).Add(text.New(clientAddr, props.Text{Size: 8, Color: &minGray})),
		),
		row.New(8),
	)
}

func minimalItems(m core.Maroto, inv *models.Invoice) {
	lineStyle := &props.Cell{BorderType: border.Bottom, BorderColor: &minLine}
	if inv.VatExempt {
		m.AddRows(row.New(6).WithStyle(lineStyle).Add(
			col.New(6).Add(text.New("Popis", props.Text{Size: 7, Color: &minGray})),
			col.New(2).Add(text.New("Mn.", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(2).Add(text.New("Cena/j.", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(2).Add(text.New("Celkem", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
		))
		for _, l := range inv.Lines {
			qty := fmt.Sprintf("%s %s", l.Quantity, l.UnitName)
			m.AddRows(row.New(7).WithStyle(lineStyle).Add(
				col.New(6).Add(text.New(l.Name, props.Text{Size: 8, Color: &minDark, Top: 1.5})),
				col.New(2).Add(text.New(qty, props.Text{Size: 8, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(2).Add(text.New(formatHal(l.UnitPriceHal), props.Text{Size: 8, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(2).Add(text.New(formatHal(l.TotalHal), props.Text{Size: 8, Color: &minDark, Align: align.Right, Top: 1.5})),
			))
		}
	} else {
		m.AddRows(row.New(6).WithStyle(lineStyle).Add(
			col.New(4).Add(text.New("Popis", props.Text{Size: 7, Color: &minGray})),
			col.New(2).Add(text.New("Mn.", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(1).Add(text.New("Cena/j.", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(1).Add(text.New("Sazba", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(2).Add(text.New("Základ", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(1).Add(text.New("DPH", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
			col.New(1).Add(text.New("Celkem", props.Text{Size: 7, Color: &minGray, Align: align.Right})),
		))
		for _, l := range inv.Lines {
			qty := fmt.Sprintf("%s %s", l.Quantity, l.UnitName)
			m.AddRows(row.New(7).WithStyle(lineStyle).Add(
				col.New(4).Add(text.New(l.Name, props.Text{Size: 8, Color: &minDark, Top: 1.5})),
				col.New(2).Add(text.New(qty, props.Text{Size: 8, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(1).Add(text.New(formatHal(l.UnitPriceHal), props.Text{Size: 7, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(1).Add(text.New(vatRateLabel(l.VatRateBps), props.Text{Size: 7, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(2).Add(text.New(formatHal(l.TotalPriceWithoutVatHal), props.Text{Size: 7, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(1).Add(text.New(formatHal(l.TotalVatHal), props.Text{Size: 7, Color: &minGray, Align: align.Right, Top: 1.5})),
				col.New(1).Add(text.New(formatHal(l.TotalHal), props.Text{Size: 8, Color: &minDark, Align: align.Right, Top: 1.5})),
			))
		}
	}
	m.AddRows(row.New(6))
}

func minimalTotals(m core.Maroto, inv *models.Invoice, qrBytes []byte) {
	hasQR := len(qrBytes) > 0

	if inv.TransferredTaxLiability {
		m.AddRows(row.New(6).Add(col.New(12).Add(text.New(
			"Přenesená daňová povinnost – DPH odvede odběratel.",
			props.Text{Size: 8, Color: &minGray, Align: align.Right},
		))))
	} else if !inv.VatExempt {
		for _, g := range vatGroups(inv) {
			label := fmt.Sprintf("%d %%", g.RateBps/100)
			m.AddRows(
				row.New(5).Add(
					col.New(10).Add(text.New("Základ DPH "+label, props.Text{Size: 8, Color: &minGray, Align: align.Right})),
					col.New(2).Add(text.New(formatHal(g.BaseHal)+" Kč", props.Text{Size: 8, Align: align.Right})),
				),
				row.New(5).Add(
					col.New(10).Add(text.New("DPH "+label, props.Text{Size: 8, Color: &minGray, Align: align.Right})),
					col.New(2).Add(text.New(formatHal(g.VatHal)+" Kč", props.Text{Size: 8, Align: align.Right})),
				),
			)
		}
	}

	totalLineStyle := &props.Cell{BorderType: border.Top, BorderColor: &minAccent, BorderThickness: 0.6}

	if hasQR {
		m.AddRows(
			row.New(22).WithStyle(totalLineStyle).Add(
				col.New(3).Add(image.NewFromBytes(qrBytes, extension.Png,
					props.Rect{Percent: 85, Center: true},
				)),
				col.New(7).Add(text.New("", props.Text{})),
				col.New(2).Add(text.New(formatHal(inv.Total)+" Kč", props.Text{
					Size: 12, Style: fontstyle.Bold, Align: align.Right, Color: &minAccent, Top: 8,
				})),
			),
		)
	} else {
		m.AddRows(
			row.New(10).WithStyle(totalLineStyle).Add(
				col.New(10).Add(text.New("Celkem k úhradě", props.Text{
					Size: 9, Style: fontstyle.Bold, Align: align.Right, Top: 3,
				})),
				col.New(2).Add(text.New(formatHal(inv.Total)+" Kč", props.Text{
					Size: 12, Style: fontstyle.Bold, Align: align.Right, Color: &minAccent, Top: 2,
				})),
			),
		)
	}

	m.AddRows(
		row.New(6),
		row.New(5).Add(col.New(12).Add(text.New(
			"Účet: "+inv.BankAccount,
			props.Text{Size: 8, Color: &minGray},
		))),
	)
	if inv.VatExempt {
		m.AddRows(row.New(5).Add(col.New(12).Add(text.New(
			"Fyzická osoba není plátcem DPH.",
			props.Text{Size: 7, Color: &minGray},
		))))
	}
	if inv.Note != "" {
		m.AddRows(row.New(6).Add(col.New(12).Add(text.New(
			inv.Note, props.Text{Size: 8, Color: &minGray},
		))))
	}
}
