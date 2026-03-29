package models

import (
	"fmt"
	"strings"
	"time"
)

// Settings is a singleton record (always ID=1) with account-level configuration.
// Prefills "your_*" fields on new invoices and drives number generation.
type Settings struct {
	Base

	// Owner — 0 = single-user mode (ID=1 singleton), >0 = user ID in multi-user mode
	UserID uint `gorm:"not null;default:0;index" json:"user_id,omitempty"`

	// Company identity
	CompanyName    string `json:"company_name"`
	CompanyStreet  string `json:"company_street"`
	CompanyCity    string `json:"company_city"`
	CompanyZip     string `gorm:"size:5"  json:"company_zip"`
	CompanyCountry string `gorm:"size:2;default:'CZ'" json:"company_country"`
	RegistrationNo string `gorm:"size:8"  json:"registration_no"`  // IČO
	VatNo          string `gorm:"size:12" json:"vat_no"`           // DIČ
	VatExempt      bool   `gorm:"default:true" json:"vat_exempt"`

	// Payment defaults (used when no bank account on invoice)
	BankAccount string `json:"bank_account"`
	IBAN        string `json:"iban"`
	SwiftBIC    string `json:"swift_bic"`

	// Invoice defaults
	DefaultDue           int    `gorm:"default:14"    json:"default_due"`
	DefaultCurrency      string `gorm:"size:3;default:'CZK'" json:"default_currency"`
	DefaultPaymentMethod string `gorm:"default:'bank'" json:"default_payment_method"`
	DefaultNote          string `json:"default_note"`

	// PDF template: "classic" | "modern" | "minimal"
	InvoiceTemplate string `gorm:"size:20;default:'classic'" json:"invoice_template"`

	// Number formats (has-many)
	NumberFormats []NumberFormat `gorm:"foreignKey:SettingsID" json:"number_formats"`
}

// NumberFormat defines a document numbering sequence.
// Pattern tokens: {YYYY} full year, {YY} 2-digit year, {NNN} zero-padded counter.
type NumberFormat struct {
	Base
	SettingsID   uint   `gorm:"not null;index" json:"settings_id"`
	DocumentType string `gorm:"not null;default:'invoice'" json:"document_type"` // invoice|proforma|correction
	Label        string `json:"label"`   // user label, e.g. "Faktury 2025"
	Pattern      string `gorm:"not null;default:'{YYYY}{NNN}'" json:"pattern"`
	NextNumber   int    `gorm:"not null;default:1" json:"next_number"`
	PaddingWidth int    `gorm:"not null;default:3" json:"padding_width"` // digits for {NNN}
}

// Generate returns the formatted number for the current NextNumber and increments NextNumber.
// Call this inside a transaction to avoid races.
func (nf *NumberFormat) Generate() string {
	year := time.Now().Year()
	n := nf.NextNumber
	result := nf.Pattern
	result = strings.ReplaceAll(result, "{YYYY}", fmt.Sprintf("%04d", year))
	result = strings.ReplaceAll(result, "{YY}", fmt.Sprintf("%02d", year%100))
	result = strings.ReplaceAll(result, "{NNN}", fmt.Sprintf("%0*d", nf.PaddingWidth, n))
	nf.NextNumber++
	return result
}

// DefaultSettings returns a pre-populated Settings with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		CompanyCountry:       "CZ",
		VatExempt:            true,
		DefaultDue:           14,
		DefaultCurrency:      "CZK",
		DefaultPaymentMethod: "bank",
		NumberFormats: []NumberFormat{
			{DocumentType: "invoice",  Label: "Faktury",  Pattern: "{YYYY}{NNN}", NextNumber: 1, PaddingWidth: 3},
			{DocumentType: "proforma", Label: "Proformy", Pattern: "PF{YYYY}{NNN}", NextNumber: 1, PaddingWidth: 3},
		},
	}
}
