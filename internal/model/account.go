package model

import "time"

// Account is a company / sole trader profile including invoicing defaults.
type Account struct {
	ID                   uint
	Slug                 string `gorm:"not null;uniqueIndex"`
	Name                 string `gorm:"not null"`
	RegistrationNo       string
	VatNo                string
	Street               string
	City                 string
	Zip                  string
	Country              string
	Email                string
	Phone                string
	Web                  string
	VatMode              string
	RegisteredBy         string
	DefaultCurrency      string
	DefaultDueDays       int
	DefaultPaymentMethod string
	DefaultLanguage      string
	DefaultNote          string
	DefaultFooterNote    string
	RoundTotal           bool
	DefaultVatRateBps    int32
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type BankAccount struct {
	ID        uint
	AccountID uint   `gorm:"not null;index"`
	Name      string `gorm:"not null"`
	Currency  string `gorm:"not null"`
	Number    string // Czech format "123456789/0800", optional
	IBAN      string
	SwiftBIC  string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NumberFormat is a document numbering series, e.g. "{YYYY}-{NNNN}".
type NumberFormat struct {
	ID           uint
	AccountID    uint   `gorm:"not null;index"`
	DocumentType string `gorm:"not null"`
	Format       string `gorm:"not null"`
	IsDefault    bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NumberCounter holds the last issued number of a format within a period
// ("2026", "2026-03" or "" when the format has no date placeholder).
type NumberCounter struct {
	ID             uint
	NumberFormatID uint   `gorm:"not null;uniqueIndex:idx_number_counters_format_period"`
	Period         string `gorm:"not null;uniqueIndex:idx_number_counters_format_period"`
	LastNumber     int64  `gorm:"not null"`
}
