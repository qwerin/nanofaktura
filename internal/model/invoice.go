package model

import "time"

// Invoice covers all document types (invoice, proforma, correction).
// client_*, your_* and bank fields are snapshots taken at creation time.
// Totals are stored so they can be filtered/aggregated in SQL; they must be
// recomputed (internal/billing) whenever lines or payments change.
type Invoice struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index;uniqueIndex:idx_invoices_account_type_number"`
	DocumentType   string `gorm:"not null;uniqueIndex:idx_invoices_account_type_number"`
	Number         string `gorm:"not null;uniqueIndex:idx_invoices_account_type_number"`
	VariableSymbol string
	Status         string `gorm:"not null;index"`
	SubjectID      uint   `gorm:"not null;index"`
	RelatedID      *uint  `gorm:"index"`
	PublicToken    string `gorm:"size:64;uniqueIndex"` // random, for the public client link

	ClientName           string
	ClientFullName       string
	ClientRegistrationNo string
	ClientVatNo          string
	ClientStreet         string
	ClientCity           string
	ClientZip            string
	ClientCountry        string
	ClientEmail          string

	YourName           string
	YourRegistrationNo string
	YourVatNo          string
	YourStreet         string
	YourCity           string
	YourZip            string
	YourCountry        string
	YourRegisteredBy   string
	YourVatMode        string

	IssuedOn              string `gorm:"not null;index"`
	TaxableFulfillmentDue string
	DueDays               int
	DueOn                 string `gorm:"index"`
	SentAt                *time.Time
	PaidOn                string
	CancelledAt           *time.Time
	UncollectibleAt       *time.Time
	LockedAt              *time.Time
	PublicViewedAt        *time.Time // first view of the public client link

	Currency            string `gorm:"not null"`
	ExchangeRate        string `gorm:"not null"`
	Language            string `gorm:"not null"`
	PaymentMethod       string `gorm:"not null"`
	CustomPaymentMethod string
	BankAccountID       *uint
	BankAccount         string
	IBAN                string
	SwiftBIC            string

	OrderNumber      string
	Note             string
	FooterNote       string
	PrivateNote      string
	Tags             []string `gorm:"serializer:json"`
	PricesIncludeVat bool
	RoundTotal       bool
	ReverseCharge    bool

	Subtotal   int64
	VatTotal   int64
	Rounding   int64
	Total      int64
	PaidAmount int64

	Lines    []InvoiceLine `gorm:"constraint:OnDelete:CASCADE"`
	Payments []Payment     `gorm:"constraint:OnDelete:CASCADE"`

	SearchText string // search.Text(SearchSource()), set by BeforeSave
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type InvoiceLine struct {
	ID            uint
	InvoiceID     uint   `gorm:"not null;index"`
	PriceItemID   *uint  `gorm:"index"` // price list item the line came from (optional)
	Position      int    `gorm:"not null"`
	Name          string `gorm:"not null"`
	QuantityMilli int64  `gorm:"not null"` // 1500 = 1.5
	UnitName      string
	UnitPrice     int64
	VatRateBps    int32
	Base          int64
	Vat           int64
	Total         int64
}

type Payment struct {
	ID        uint
	AccountID uint   `gorm:"not null;index"`
	InvoiceID uint   `gorm:"not null;index"`
	PaidOn    string `gorm:"not null"`
	Amount    int64  `gorm:"not null"`
	Note      string
	CreatedAt time.Time
}
