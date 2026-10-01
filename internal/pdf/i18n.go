package pdf

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qwerin/nanofaktura/internal/billing"
)

// Languages supported by the PDF labels.
var Languages = []string{"cs", "en", "sk", "de"}

const nbsp = " " // keeps "12 345,60 Kč" on one line

// Label keys. Every key must have a value for every language (tested).
const (
	lTitleInvoicePayer    = "title_invoice_payer"
	lTitleInvoice         = "title_invoice"
	lTitleProforma        = "title_proforma"
	lTitleCorrectionPayer = "title_correction_payer"
	lTitleCorrection      = "title_correction"
	lSubNotTaxDoc         = "sub_not_tax_doc"
	lSubCorrectionOf      = "sub_correction_of"
	lSubRelatedProforma   = "sub_related_proforma"
	lNumber               = "number"
	lSupplier             = "supplier"
	lCustomer             = "customer"
	lRegNo                = "reg_no"
	lVatNo                = "vat_no"
	lNonPayer             = "non_payer"
	lIdentifiedPerson     = "identified_person"
	lIssuedOn             = "issued_on"
	lTaxableOn            = "taxable_on"
	lDueOn                = "due_on"
	lPaymentDetails       = "payment_details"
	lDates                = "dates"
	lMethod               = "method"
	lAccountNo            = "account_no"
	lIBAN                 = "iban"
	lSWIFT                = "swift"
	lVS                   = "vs"
	lOrderNo              = "order_no"
	lItem                 = "item"
	lQty                  = "qty"
	lUnitPrice            = "unit_price"
	lUnitPriceVat         = "unit_price_vat"
	lVatRate              = "vat_rate"
	lBase                 = "base"
	lLineTotal            = "line_total"
	lVatRecap             = "vat_recap"
	lRate                 = "rate"
	lVat                  = "vat"
	lSubtotal             = "subtotal"
	lVatTotal             = "vat_total"
	lRounding             = "rounding"
	lTotal                = "total"
	lToPay                = "to_pay"
	lPaid                 = "paid"
	lRemaining            = "remaining"
	lReverseCharge        = "reverse_charge"
	lQR                   = "qr"
	lStampPaid            = "stamp_paid"
	lStampCancelled       = "stamp_cancelled"
	lPage                 = "page"
	lContinued            = "continued"
	lPayBank              = "pay_bank"
	lPayCash              = "pay_cash"
	lPayCard              = "pay_card"
	lPayCOD               = "pay_cod"
	lPayPaypal            = "pay_paypal"
	lTitleTaxDocument     = "title_tax_document"
	lTitleTaxDocumentCorr = "title_tax_document_corr"
	lSubAdvanceFor        = "sub_advance_for"
	lExemptGoods          = "exempt_goods"
	lCorrectionReason     = "correction_reason"
	lCZKRecap             = "czk_recap"
	lDeposits             = "deposits"
	lVatAfterDeposits     = "vat_after_deposits"
	lLocalVatNo           = "local_vat_no"
	lRCRate               = "rc_rate"
	lDocument             = "document"
)

// translations: key → language → text. Format verbs (%s) are filled by the caller.
var translations = map[string]map[string]string{
	lTitleInvoicePayer:    {"cs": "Faktura – daňový doklad", "en": "Invoice – tax document", "sk": "Faktúra – daňový doklad", "de": "Rechnung – Steuerbeleg"},
	lTitleInvoice:         {"cs": "Faktura", "en": "Invoice", "sk": "Faktúra", "de": "Rechnung"},
	lTitleProforma:        {"cs": "Zálohová faktura", "en": "Proforma invoice", "sk": "Zálohová faktúra", "de": "Vorauszahlungsrechnung"},
	lTitleCorrectionPayer: {"cs": "Opravný daňový doklad", "en": "Corrective tax document", "sk": "Opravný daňový doklad", "de": "Rechnungskorrektur"},
	lTitleCorrection:      {"cs": "Opravná faktura", "en": "Credit note", "sk": "Opravná faktúra", "de": "Gutschrift"},
	lSubNotTaxDoc:         {"cs": "Nejedná se o daňový doklad", "en": "This is not a tax document", "sk": "Nejde o daňový doklad", "de": "Dies ist kein Steuerbeleg"},
	lSubCorrectionOf:      {"cs": "Oprava dokladu č. %s", "en": "Correction of document No. %s", "sk": "Oprava dokladu č. %s", "de": "Korrektur zu Beleg Nr. %s"},
	lSubRelatedProforma:   {"cs": "Vyúčtování zálohové faktury č. %s", "en": "Settles proforma invoice No. %s", "sk": "Vyúčtovanie zálohovej faktúry č. %s", "de": "Abrechnung der Vorauszahlung Nr. %s"},
	lNumber:               {"cs": "Číslo dokladu", "en": "Document No.", "sk": "Číslo dokladu", "de": "Belegnummer"},
	lSupplier:             {"cs": "Dodavatel", "en": "Supplier", "sk": "Dodávateľ", "de": "Lieferant"},
	lCustomer:             {"cs": "Odběratel", "en": "Customer", "sk": "Odberateľ", "de": "Kunde"},
	lRegNo:                {"cs": "IČO", "en": "Reg. No.", "sk": "IČO", "de": "Reg.-Nr."},
	lVatNo:                {"cs": "DIČ", "en": "VAT No.", "sk": "DIČ", "de": "USt-IdNr."},
	lNonPayer:             {"cs": "Neplátce DPH", "en": "Not registered for VAT", "sk": "Neplatiteľ DPH", "de": "Nicht umsatzsteuerpflichtig"},
	lIdentifiedPerson:     {"cs": "Identifikovaná osoba k DPH", "en": "VAT identified person", "sk": "Identifikovaná osoba pre DPH", "de": "Für USt-Zwecke erfasst"},
	lIssuedOn:             {"cs": "Datum vystavení", "en": "Issue date", "sk": "Dátum vystavenia", "de": "Rechnungsdatum"},
	lTaxableOn:            {"cs": "Datum zdan. plnění", "en": "Tax point date", "sk": "Dátum dodania", "de": "Leistungsdatum"},
	lDueOn:                {"cs": "Datum splatnosti", "en": "Due date", "sk": "Dátum splatnosti", "de": "Fällig am"},
	lPaymentDetails:       {"cs": "Platební údaje", "en": "Payment details", "sk": "Platobné údaje", "de": "Zahlungsangaben"},
	lDates:                {"cs": "Termíny", "en": "Dates", "sk": "Termíny", "de": "Termine"},
	lMethod:               {"cs": "Způsob platby", "en": "Payment method", "sk": "Spôsob úhrady", "de": "Zahlungsart"},
	lAccountNo:            {"cs": "Číslo účtu", "en": "Account No.", "sk": "Číslo účtu", "de": "Kontonummer"},
	lIBAN:                 {"cs": "IBAN", "en": "IBAN", "sk": "IBAN", "de": "IBAN"},
	lSWIFT:                {"cs": "SWIFT", "en": "SWIFT/BIC", "sk": "SWIFT", "de": "BIC"},
	lVS:                   {"cs": "Variabilní symbol", "en": "Variable symbol", "sk": "Variabilný symbol", "de": "Variabler Symbol"},
	lOrderNo:              {"cs": "Číslo objednávky", "en": "Order No.", "sk": "Číslo objednávky", "de": "Bestellnummer"},
	lItem:                 {"cs": "Položka", "en": "Description", "sk": "Položka", "de": "Bezeichnung"},
	lQty:                  {"cs": "Množství", "en": "Qty", "sk": "Množstvo", "de": "Menge"},
	lUnitPrice:            {"cs": "Cena/MJ", "en": "Unit price", "sk": "Cena/MJ", "de": "Einzelpreis"},
	lUnitPriceVat:         {"cs": "Cena/MJ s DPH", "en": "Unit price incl. VAT", "sk": "Cena/MJ s DPH", "de": "Einzelpreis brutto"},
	lVatRate:              {"cs": "DPH", "en": "VAT", "sk": "DPH", "de": "USt."},
	lBase:                 {"cs": "Bez DPH", "en": "Net", "sk": "Bez DPH", "de": "Netto"},
	lLineTotal:            {"cs": "Celkem", "en": "Total", "sk": "Spolu", "de": "Gesamt"},
	lVatRecap:             {"cs": "Rekapitulace DPH", "en": "VAT summary", "sk": "Rekapitulácia DPH", "de": "USt.-Übersicht"},
	lRate:                 {"cs": "Sazba", "en": "Rate", "sk": "Sadzba", "de": "Satz"},
	lVat:                  {"cs": "DPH", "en": "VAT", "sk": "DPH", "de": "USt."},
	lSubtotal:             {"cs": "Celkem bez DPH", "en": "Total excl. VAT", "sk": "Spolu bez DPH", "de": "Summe netto"},
	lVatTotal:             {"cs": "DPH celkem", "en": "VAT total", "sk": "DPH spolu", "de": "USt. gesamt"},
	lRounding:             {"cs": "Zaokrouhlení", "en": "Rounding", "sk": "Zaokrúhlenie", "de": "Rundung"},
	lTotal:                {"cs": "Celkem", "en": "Total", "sk": "Spolu", "de": "Gesamtbetrag"},
	lToPay:                {"cs": "Celkem k úhradě", "en": "Total due", "sk": "Spolu na úhradu", "de": "Zu zahlen"},
	lPaid:                 {"cs": "Zaplaceno", "en": "Paid", "sk": "Uhradené", "de": "Bezahlt"},
	lRemaining:            {"cs": "Zbývá uhradit", "en": "Balance due", "sk": "Zostáva uhradiť", "de": "Offener Betrag"},
	lReverseCharge:        {"cs": "Daň odvede zákazník", "en": "Reverse charge – VAT to be accounted for by the customer", "sk": "Prenesenie daňovej povinnosti", "de": "Steuerschuldnerschaft des Leistungsempfängers"},
	lQR:                   {"cs": "QR Platba", "en": "QR Platba", "sk": "QR Platba", "de": "QR Platba"},
	lStampPaid:            {"cs": "ZAPLACENO", "en": "PAID", "sk": "ZAPLATENÉ", "de": "BEZAHLT"},
	lStampCancelled:       {"cs": "STORNO", "en": "CANCELLED", "sk": "STORNO", "de": "STORNIERT"},
	lPage:                 {"cs": "Strana", "en": "Page", "sk": "Strana", "de": "Seite"},
	lContinued:            {"cs": "pokračování", "en": "continued", "sk": "pokračovanie", "de": "Fortsetzung"},
	lPayBank:              {"cs": "Bankovní převod", "en": "Bank transfer", "sk": "Bankový prevod", "de": "Überweisung"},
	lPayCash:              {"cs": "Hotově", "en": "Cash", "sk": "V hotovosti", "de": "Bar"},
	lPayCard:              {"cs": "Platební karta", "en": "Card", "sk": "Platobná karta", "de": "Karte"},
	lPayCOD:               {"cs": "Dobírka", "en": "Cash on delivery", "sk": "Dobierka", "de": "Nachnahme"},
	lPayPaypal:            {"cs": "PayPal", "en": "PayPal", "sk": "PayPal", "de": "PayPal"},
	lTitleTaxDocument: {"cs": "Daňový doklad k přijaté platbě", "en": "Tax document for a received payment",
		"sk": "Daňový doklad k prijatej platbe", "de": "Steuerbeleg über erhaltene Anzahlung"},
	lTitleTaxDocumentCorr: {"cs": "Opravný daňový doklad k přijaté platbě", "en": "Corrective tax document for a received payment",
		"sk": "Opravný daňový doklad k prijatej platbe", "de": "Korrektur des Steuerbelegs über erhaltene Anzahlung"},
	lSubAdvanceFor: {"cs": "K zálohové faktuře č. %s", "en": "For proforma invoice No. %s", "sk": "K zálohovej faktúre č. %s",
		"de": "Zur Vorauszahlungsrechnung Nr. %s"},
	lExemptGoods: {"cs": "Osvobozeno od daně – dodání zboží do jiného členského státu (§ 64 zákona o DPH)",
		"en": "VAT exempt intra-Community supply of goods (Art. 138 of Directive 2006/112/EC)",
		"sk": "Oslobodené od dane – dodanie tovaru do iného členského štátu (čl. 138 smernice 2006/112/ES)",
		"de": "Steuerfreie innergemeinschaftliche Lieferung (Art. 138 MwStSystRL)"},
	lCorrectionReason: {"cs": "Důvod opravy: %s", "en": "Reason for the correction: %s", "sk": "Dôvod opravy: %s", "de": "Grund der Korrektur: %s"},
	lCZKRecap: {"cs": "DPH v Kč (kurz ČNB 1 %s = %s Kč)", "en": "VAT in CZK (CNB rate 1 %s = %s CZK)",
		"sk": "DPH v CZK (kurz ČNB 1 %s = %s CZK)", "de": "USt. in CZK (Kurs der ČNB 1 %s = %s CZK)"},
	lDeposits:         {"cs": "Odpočet záloh", "en": "Advance payments deducted", "sk": "Odpočet záloh", "de": "Abzug der Anzahlungen"},
	lVatAfterDeposits: {"cs": "DPH po odpočtu záloh", "en": "VAT after deducting advances", "sk": "DPH po odpočte záloh", "de": "USt. nach Abzug der Anzahlungen"},
	lLocalVatNo:       {"cs": "IČ DPH", "en": "VAT ID", "sk": "IČ DPH", "de": "USt-IdNr. (SK)"},
	lRCRate:           {"cs": "PDP", "en": "RC", "sk": "PDP", "de": "RC"},
	lDocument:         {"cs": "Doklad", "en": "Document", "sk": "Doklad", "de": "Beleg"},
}

var countryNames = map[string]map[string]string{
	"CZ": {"cs": "Česká republika", "en": "Czech Republic", "sk": "Česká republika", "de": "Tschechien"},
	"SK": {"cs": "Slovensko", "en": "Slovakia", "sk": "Slovensko", "de": "Slowakei"},
	"DE": {"cs": "Německo", "en": "Germany", "sk": "Nemecko", "de": "Deutschland"},
	"AT": {"cs": "Rakousko", "en": "Austria", "sk": "Rakúsko", "de": "Österreich"},
	"PL": {"cs": "Polsko", "en": "Poland", "sk": "Poľsko", "de": "Polen"},
	"HU": {"cs": "Maďarsko", "en": "Hungary", "sk": "Maďarsko", "de": "Ungarn"},
	"GB": {"cs": "Spojené království", "en": "United Kingdom", "sk": "Spojené kráľovstvo", "de": "Vereinigtes Königreich"},
	"US": {"cs": "Spojené státy", "en": "United States", "sk": "Spojené štáty", "de": "Vereinigte Staaten"},
	"NL": {"cs": "Nizozemsko", "en": "Netherlands", "sk": "Holandsko", "de": "Niederlande"},
	"FR": {"cs": "Francie", "en": "France", "sk": "Francúzsko", "de": "Frankreich"},
}

var monthsEN = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// locale formats labels, money, dates and numbers for one language.
type locale struct{ lang string }

func newLocale(lang string) locale {
	for _, l := range Languages {
		if l == lang {
			return locale{lang}
		}
	}
	return locale{"cs"}
}

// t returns the label for key, formatted with args when given.
func (l locale) t(key string, args ...any) string {
	s := translations[key][l.lang]
	if s == "" {
		s = translations[key]["cs"]
	}
	if len(args) > 0 {
		s = fmt.Sprintf(s, args...)
	}
	return s
}

func (l locale) separators() (thousands, decimal string) {
	switch l.lang {
	case "en":
		return ",", "."
	case "de":
		return ".", ","
	default:
		return nbsp, ","
	}
}

// groupDigits inserts the thousands separator into a string of digits.
func groupDigits(digits, sep string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// amount formats minor units as "12 345,60" (no currency).
func (l locale) amount(minor int64) string {
	th, dec := l.separators()
	sign := ""
	if minor < 0 {
		sign = "−" // U+2212 minus
		minor = -minor
	}
	return fmt.Sprintf("%s%s%s%02d", sign, groupDigits(strconv.FormatInt(minor/100, 10), th), dec, minor%100)
}

// money formats minor units with the currency: "12 345,60 Kč", "€1,234.50", "1.234,50 EUR".
func (l locale) money(minor int64, currency string) string {
	currency = strings.ToUpper(currency)
	if currency == "" {
		currency = "CZK"
	}
	sym := currency
	switch currency {
	case "CZK":
		if l.lang == "cs" || l.lang == "sk" {
			sym = "Kč"
		}
	case "EUR":
		sym = "€"
	case "USD":
		sym = "$"
	case "GBP":
		sym = "£"
	}
	if l.lang == "en" && len([]rune(sym)) == 1 {
		a := l.amount(minor)
		if strings.HasPrefix(a, "−") {
			return "−" + sym + strings.TrimPrefix(a, "−")
		}
		return sym + a
	}
	return l.amount(minor) + nbsp + sym
}

// date formats "YYYY-MM-DD" per language; unparsable input is returned as is.
func (l locale) date(iso string) string {
	d, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	switch l.lang {
	case "en":
		return fmt.Sprintf("%d %s %d", d.Day(), monthsEN[d.Month()-1], d.Year())
	case "de":
		return d.Format("02.01.2006")
	default:
		return fmt.Sprintf("%d.%s%d.%s%d", d.Day(), nbsp, int(d.Month()), nbsp, d.Year())
	}
}

// quantity formats thousandths: 1500 → "1,5", -2000 → "−2".
func (l locale) quantity(milli int64) string {
	q := billing.FormatQuantity(milli)
	sign := ""
	if strings.HasPrefix(q, "-") {
		sign, q = "−", q[1:]
	}
	intPart, frac, _ := strings.Cut(q, ".")
	th, dec := l.separators()
	s := sign + groupDigits(intPart, th)
	if frac != "" {
		s += dec + frac
	}
	return s
}

// rate formats an exchange rate of billing.ParseRate ("24,355"; en "24.355").
func (l locale) rate(r int64) string {
	_, dec := l.separators()
	s := strconv.FormatInt(r/billing.RateScale, 10)
	if frac := strings.TrimRight(fmt.Sprintf("%06d", r%billing.RateScale), "0"); frac != "" {
		s += dec + frac
	}
	return s
}

// vatRate formats basis points: 2100 → "21 %", 1250 → "12,5 %" (en: "21%").
func (l locale) vatRate(bps int32) string {
	_, dec := l.separators()
	s := strconv.Itoa(int(bps / 100))
	if frac := bps % 100; frac != 0 {
		if frac < 0 {
			frac = -frac
		}
		s += dec + strings.TrimRight(fmt.Sprintf("%02d", frac), "0")
	}
	if l.lang == "en" {
		return s + "%"
	}
	return s + nbsp + "%"
}

func (l locale) country(code string) string {
	if n := countryNames[strings.ToUpper(code)][l.lang]; n != "" {
		return n
	}
	return code
}

func (l locale) paymentMethod(method, custom string) string {
	switch method {
	case "bank":
		return l.t(lPayBank)
	case "cash":
		return l.t(lPayCash)
	case "card":
		return l.t(lPayCard)
	case "cod":
		return l.t(lPayCOD)
	case "paypal":
		return l.t(lPayPaypal)
	case "custom":
		return custom
	}
	return method
}
