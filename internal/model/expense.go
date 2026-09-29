package model

import "time"

// Expense is a received (purchase) document (SPEC §7.2). supplier_* are
// snapshots of the subject taken when the subject is set. Totals are stored
// and recomputed (internal/billing) whenever lines or payments change.
// Stored status is StatusOpen or StatusPaid; overdue is derived on read.
type Expense struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index;uniqueIndex:idx_expenses_account_number"`
	Number         string `gorm:"not null;uniqueIndex:idx_expenses_account_number"`
	OriginalNumber string // the supplier's document number
	VariableSymbol string
	Status         string `gorm:"not null;index"`
	SubjectID      *uint  `gorm:"index"`

	SupplierName           string
	SupplierFullName       string
	SupplierRegistrationNo string
	SupplierVatNo          string
	SupplierStreet         string
	SupplierCity           string
	SupplierZip            string
	SupplierCountry        string
	SupplierBankAccount    string
	SupplierIBAN           string
	SupplierSwiftBIC       string

	IssuedOn              string `gorm:"not null;index"`
	TaxableFulfillmentDue string
	DueOn                 string `gorm:"index"`
	PaidOn                string
	LockedAt              *time.Time

	Currency         string `gorm:"not null"`
	ExchangeRate     string `gorm:"not null"`
	PaymentMethod    string `gorm:"not null"`
	Category         string `gorm:"index"`
	Description      string
	PrivateNote      string
	Tags             []string `gorm:"serializer:json"`
	TaxDeductible    bool
	PricesIncludeVat bool
	RoundTotal       bool

	Subtotal   int64
	VatTotal   int64
	Rounding   int64
	Total      int64
	PaidAmount int64

	Lines    []ExpenseLine    `gorm:"constraint:OnDelete:CASCADE"`
	Payments []ExpensePayment `gorm:"constraint:OnDelete:CASCADE"`

	SearchText string // search.Text(SearchSource()), set by BeforeSave
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ExpenseLine has the same structure and computation as InvoiceLine.
type ExpenseLine struct {
	ID            uint
	ExpenseID     uint   `gorm:"not null;index"`
	PriceItemID   *uint  `gorm:"index"`
	Position      int    `gorm:"not null"`
	Name          string `gorm:"not null"`
	QuantityMilli int64  `gorm:"not null"`
	UnitName      string
	UnitPrice     int64
	VatRateBps    int32
	Base          int64
	Vat           int64
	Total         int64
}

// ExpensePayment is a payment of an expense (mirrors Payment of invoices).
type ExpensePayment struct {
	ID        uint
	AccountID uint   `gorm:"not null;index"`
	ExpenseID uint   `gorm:"not null;index"`
	PaidOn    string `gorm:"not null"`
	Amount    int64  `gorm:"not null"`
	Note      string
	CreatedAt time.Time
}
