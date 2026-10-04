package pdf

import (
	"fmt"

	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// SampleSpec describes demo data for previews and visual checks.
type SampleSpec struct {
	DocumentType  string // invoice (default) | proforma | correction
	VatPayer      bool
	Lines         int    // number of lines; 0 = 5
	Status        string // default open
	PaidPartially bool   // one payment of ~40 % of the total
	ReverseCharge bool
	Language      string // default cs
}

var sampleItems = []struct {
	name  string
	unit  string
	qty   int64 // thousandths
	price int64 // minor units
	rate  int32
}{
	{"Vývoj webové aplikace – backend (REST API, databáze, autentizace)", "hod", 42500, 1_250_00, 2100},
	{"Konzultace a analýza požadavků", "hod", 6000, 1_500_00, 2100},
	{"Hosting a správa serveru – měsíční paušál", "měs.", 3000, 890_00, 2100},
	{"Odborná publikace „Účetnictví pro živnostníky“", "ks", 2000, 459_00, 1200},
	{"Grafický návrh loga a vizuální identity včetně tří revizí a předání zdrojových souborů", "ks", 1000, 18_900_00, 2100},
	{"Školení uživatelů (půldenní workshop)", "ks", 1000, 7_500_00, 2100},
	{"Doprava a cestovné", "km", 186500, 5_60, 2100},
	{"Export do ISDOC a napojení na účetní systém", "hod", 3500, 1_250_00, 2100},
}

// Sample returns a realistic, internally consistent invoice and account for
// previews (e.g. template settings) and for visual checks of the templates.
// For a correction the related number to pass in Options is "2026-0041".
func Sample(s SampleSpec) (*model.Invoice, *model.Account) {
	if s.DocumentType == "" {
		s.DocumentType = model.DocInvoice
	}
	if s.Lines <= 0 {
		s.Lines = 5
	}
	if s.Status == "" {
		s.Status = model.StatusOpen
	}
	if s.Language == "" {
		s.Language = "cs"
	}
	vatMode := model.VatModeNonPayer
	vatNo := ""
	if s.VatPayer {
		vatMode, vatNo = model.VatModePayer, "CZ27074358"
	}
	bank, _ := spayd.ParseAccount("19-2000145399/0800")

	acc := &model.Account{
		ID: 1, Slug: "studio-kovar", Name: "Studio Kovář s.r.o.", RegistrationNo: "27074358", VatNo: vatNo,
		Street: "Vinohradská 1597/174", City: "Praha 3", Zip: "130 00", Country: "CZ",
		Email: "fakturace@studiokovar.cz", Phone: "+420 603 123 456", Web: "www.studiokovar.cz",
		VatMode:         vatMode,
		RegisteredBy:    "Společnost je zapsána v obchodním rejstříku vedeném Městským soudem v Praze, oddíl C, vložka 123456.",
		DefaultCurrency: "CZK", DefaultDueDays: 14, DefaultPaymentMethod: "bank", DefaultLanguage: "cs",
	}

	number := map[string]string{model.DocProforma: "Z2026-0012", model.DocCorrection: "D2026-0003"}[s.DocumentType]
	if number == "" {
		number = "2026-0042"
	}
	inv := &model.Invoice{
		ID: 42, AccountID: acc.ID, DocumentType: s.DocumentType, Number: number,
		VariableSymbol: spayd.Digits(number, 10), Status: s.Status, SubjectID: new(uint(7)),

		ClientName: "ACME Technologies a.s.", ClientFullName: "Ing. Jana Nováková",
		ClientRegistrationNo: "45317054", ClientVatNo: "CZ45317054",
		ClientStreet: "Karlovo náměstí 2097/10", ClientCity: "Brno", ClientZip: "602 00", ClientCountry: "CZ",
		ClientEmail: "ucetni@acme.cz",

		YourName: acc.Name, YourRegistrationNo: acc.RegistrationNo, YourVatNo: acc.VatNo,
		YourStreet: acc.Street, YourCity: acc.City, YourZip: acc.Zip, YourCountry: "CZ",
		YourRegisteredBy: acc.RegisteredBy, YourVatMode: vatMode,

		IssuedOn: "2026-03-15", DueDays: 14, DueOn: "2026-03-29",
		Currency: "CZK", ExchangeRate: "1", Language: s.Language, PaymentMethod: "bank",
		BankAccount: "19-2000145399/0800", IBAN: bank.IBAN(), SwiftBIC: bank.SWIFT(),
		OrderNumber:   "OBJ-2026-118",
		Note:          "Fakturujeme Vám dodávku dle objednávky a akceptačního protokolu ze dne 12. 3. 2026.",
		FooterNote:    "Děkujeme za spolupráci. V případě dotazů k faktuře nás kontaktujte na fakturace@studiokovar.cz.",
		RoundTotal:    !s.VatPayer,
		ReverseCharge: s.ReverseCharge,
	}
	if s.VatPayer && s.DocumentType != model.DocProforma {
		inv.TaxableFulfillmentDue = "2026-03-15"
	}
	if s.ReverseCharge {
		inv.ClientName, inv.ClientCountry = "Müller Software GmbH", "DE"
		inv.ClientStreet, inv.ClientCity, inv.ClientZip = "Friedrichstraße 123", "Berlin", "10117"
		inv.ClientRegistrationNo, inv.ClientVatNo, inv.ClientFullName = "", "DE811907980", ""
	}

	for i := range s.Lines {
		it := sampleItems[i%len(sampleItems)]
		name := it.name
		if s.Lines > len(sampleItems) {
			name = fmt.Sprintf("%s (%d)", it.name, i+1)
		}
		ln := model.InvoiceLine{
			ID: uint(i + 1), InvoiceID: inv.ID, Position: i + 1, Name: name,
			QuantityMilli: it.qty, UnitName: it.unit, UnitPrice: it.price, VatRateBps: it.rate,
		}
		if !s.VatPayer {
			ln.VatRateBps = 0
		}
		if s.DocumentType == model.DocCorrection {
			ln.QuantityMilli = -ln.QuantityMilli
		}
		inv.Lines = append(inv.Lines, ln)
	}
	if s.DocumentType == model.DocCorrection {
		inv.Lines = inv.Lines[:min(2, len(inv.Lines))]
		inv.Note = "Oprava základu daně z důvodu slevy poskytnuté po dodání."
	}

	t, err := billing.Calculate(billingLines(inv), billingOptions(inv, s.VatPayer))
	if err == nil {
		for i := range inv.Lines {
			inv.Lines[i].Base, inv.Lines[i].Vat, inv.Lines[i].Total = t.Lines[i].Base, t.Lines[i].Vat, t.Lines[i].Total
		}
		inv.Subtotal, inv.VatTotal, inv.Rounding, inv.Total = t.Subtotal, t.VatTotal, t.Rounding, t.Total
	}

	switch {
	case s.Status == model.StatusPaid:
		inv.PaidOn = "2026-03-20"
		inv.Payments = []model.Payment{{ID: 1, AccountID: acc.ID, InvoiceID: inv.ID, PaidOn: inv.PaidOn, Amount: inv.Total}}
	case s.PaidPartially:
		amt := billing.RoundTo(inv.Total*4/10, 100)
		inv.Payments = []model.Payment{{ID: 1, AccountID: acc.ID, InvoiceID: inv.ID, PaidOn: "2026-03-18", Amount: amt}}
	}
	for _, p := range inv.Payments {
		inv.PaidAmount += p.Amount
	}
	return inv, acc
}
