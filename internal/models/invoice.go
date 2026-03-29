// Package models obsahuje GORM struktury sdílené napříč celou aplikací.
// Peněžní hodnoty jsou vždy int64 v haléřích (1 Kč = 100 haléřů)
// aby se předešlo chybám plovoucí řádové čárky při zaokrouhlování.
package models

import "fmt"

type DocumentType string

const (
	DocInvoice    DocumentType = "invoice"
	DocProforma   DocumentType = "proforma"
	DocCorrection DocumentType = "correction"
	DocTaxDoc     DocumentType = "tax_document"
)

type InvoiceStatus string

const (
	StatusOpen    InvoiceStatus = "open"    // neuhrazená (dříve "open"+"sent")
	StatusOverdue InvoiceStatus = "overdue" // po splatnosti — počítáno dynamicky z due date
	StatusPaid    InvoiceStatus = "paid"    // uhrazená
)

type PaymentMethod string

const (
	PaymentBank   PaymentMethod = "bank"
	PaymentCash   PaymentMethod = "cash"
	PaymentCard   PaymentMethod = "card"
	PaymentCOD    PaymentMethod = "cod"
	PaymentPayPal PaymentMethod = "paypal"
)

type Language string

const (
	LangCS Language = "cs"
	LangEN Language = "en"
	LangDE Language = "de"
	LangSK Language = "sk"
)

// InvoiceLine is one line item on the invoice (renamed from InvoiceItem to match Fakturoid)
type InvoiceLine struct {
	Base
	InvoiceID   uint  `gorm:"not null;index"  json:"invoice_id"`
	PriceItemID *uint `gorm:"index"           json:"price_item_id,omitempty"` // odkaz na ceníkovou položku
	Position    int   `gorm:"not null"        json:"position"`
	Name        string `gorm:"not null"       json:"name"`

	Quantity string `gorm:"not null;default:'1'" json:"quantity"`  // stored as decimal string e.g. "1.5"
	UnitName string `gorm:"size:20"              json:"unit_name"` // ks, hod, km, …

	// Prices in haléře (int64)
	UnitPriceHal int64 `gorm:"not null" json:"unit_price_hal"`

	// VAT-ready (neplátce = 0)
	VatRateBps int32 `gorm:"not null;default:0" json:"vat_rate_bps"` // 2100 = 21%

	// Computed (filled by Recalculate)
	UnitPriceWithoutVatHal  int64 `gorm:"not null;default:0" json:"unit_price_without_vat_hal"`
	UnitPriceWithVatHal     int64 `gorm:"not null;default:0" json:"unit_price_with_vat_hal"`
	TotalPriceWithoutVatHal int64 `gorm:"not null;default:0" json:"total_price_without_vat_hal"`
	TotalVatHal             int64 `gorm:"not null;default:0" json:"total_vat_hal"`
	TotalHal                int64 `gorm:"not null;default:0" json:"total_hal"`
}

// Invoice is the main document entity — aligned with Fakturoid v3 attribute set
type Invoice struct {
	Base

	// Document classification
	DocumentType DocumentType  `gorm:"not null;default:'invoice'" json:"document_type"`
	Status       InvoiceStatus `gorm:"not null;default:'open'"    json:"status"`
	Language     Language      `gorm:"size:2;default:'cs'"        json:"language"`

	// Numbering
	Number         string `gorm:"uniqueIndex:idx_invoice_number_user;not null" json:"number"`
	VariableSymbol string `gorm:"size:10"              json:"variable_symbol"`
	OrderNumber    string `gorm:"size:50"              json:"order_number"`
	CustomID       string `gorm:"size:100"             json:"custom_id"`

	// Dates (stored as "YYYY-MM-DD" strings to avoid time.Time JSON issues)
	IssuedOn              string `gorm:"not null" json:"issued_on"`               // date of issue
	TaxableFulfillmentDue string `gorm:"not null" json:"taxable_fulfillment_due"` // DUZP
	Due                   int    `gorm:"not null;default:14" json:"due"`          // days until overdue
	PaidOn                string `json:"paid_on,omitempty"`

	// Owner — 0 = single-user mode (implicit owner), >0 = user ID in multi-user mode
	UserID uint `gorm:"not null;default:0;uniqueIndex:idx_invoice_number_user" json:"user_id,omitempty"`

	// Subject (client) — optional FK + denormalized copy (like Fakturoid)
	SubjectID uint `gorm:"index" json:"subject_id,omitempty"`

	// Your company info (denormalized from account settings at invoice creation)
	YourName           string `json:"your_name"`
	YourStreet         string `json:"your_street"`
	YourCity           string `json:"your_city"`
	YourZip            string `gorm:"size:5" json:"your_zip"`
	YourCountry        string `gorm:"size:2;default:'CZ'" json:"your_country"`
	YourRegistrationNo string `gorm:"size:8" json:"your_registration_no"` // IČO
	YourVatNo          string `gorm:"size:12" json:"your_vat_no"`         // DIČ

	// Client info (denormalized copy for archival integrity)
	ClientName           string `gorm:"not null" json:"client_name"`
	ClientStreet         string `json:"client_street"`
	ClientCity           string `json:"client_city"`
	ClientZip            string `gorm:"size:5"  json:"client_zip"`
	ClientCountry        string `gorm:"size:2;default:'CZ'" json:"client_country"`
	ClientRegistrationNo string `gorm:"size:8" json:"client_registration_no"` // IČO
	ClientVatNo          string `gorm:"size:12" json:"client_vat_no"`          // DIČ

	// Delivery address
	ClientHasDeliveryAddress bool   `gorm:"default:false"       json:"client_has_delivery_address"`
	ClientDeliveryName       string `json:"client_delivery_name"`
	ClientDeliveryStreet     string `json:"client_delivery_street"`
	ClientDeliveryCity       string `json:"client_delivery_city"`
	ClientDeliveryZip        string `gorm:"size:5" json:"client_delivery_zip"`
	ClientDeliveryCountry    string `gorm:"size:2" json:"client_delivery_country"`

	// Payment
	PaymentMethod PaymentMethod `gorm:"not null;default:'bank'" json:"payment_method"`
	BankAccount   string        `json:"bank_account"`
	IBAN          string        `json:"iban"`
	SwiftBIC      string        `json:"swift_bic"`

	// Currency
	Currency     string `gorm:"size:3;default:'CZK'" json:"currency"`
	ExchangeRate string `gorm:"default:'1'"          json:"exchange_rate"` // stored as decimal string

	// Tax settings
	VatExempt               bool   `gorm:"not null;default:true"  json:"vat_exempt"`
	TransferredTaxLiability bool   `gorm:"default:false"          json:"transferred_tax_liability"`
	VatPriceMode            string `gorm:"default:'without_vat'"  json:"vat_price_mode"` // without_vat | with_vat

	// Notes
	Note        string `json:"note"`
	FooterNote  string `json:"footer_note"`
	PrivateNote string `json:"private_note"`
	Tags        string `json:"tags"` // comma-separated

	// Totals (computed, stored for fast queries)
	Subtotal    int64 `gorm:"not null;default:0" json:"subtotal"`      // základ celkem
	TotalVatHal int64 `gorm:"not null;default:0" json:"total_vat_hal"` // DPH celkem
	Total       int64 `gorm:"not null;default:0" json:"total"`         // celková částka

	// Lines
	Lines []InvoiceLine `gorm:"foreignKey:InvoiceID" json:"lines"`
}

// Recalculate přepočítá všechny součty z Lines. Musí se volat před každým Save().
func (inv *Invoice) Recalculate() {
	var subtotal, totalVat, total int64
	for i := range inv.Lines {
		l := &inv.Lines[i]
		// Parse quantity string to float, default 1
		qty := parseQuantity(l.Quantity)

		base := int64(float64(l.UnitPriceHal) * qty)
		vat := base * int64(l.VatRateBps) / 10000

		l.UnitPriceWithoutVatHal = l.UnitPriceHal
		l.UnitPriceWithVatHal = l.UnitPriceHal + l.UnitPriceHal*int64(l.VatRateBps)/10000
		l.TotalPriceWithoutVatHal = base
		l.TotalVatHal = vat
		l.TotalHal = base + vat

		subtotal += base
		totalVat += vat
		total += base + vat
	}
	inv.Subtotal = subtotal
	inv.TotalVatHal = totalVat
	inv.Total = total
}

// parseQuantity converts "1.5" → 1.5, falls back to 1.0
func parseQuantity(s string) float64 {
	if s == "" {
		return 1.0
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil || f <= 0 {
		return 1.0
	}
	return f
}
