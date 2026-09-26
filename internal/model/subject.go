package model

import "time"

// Subject is a contact (customer and/or supplier).
type Subject struct {
	ID             uint
	AccountID      uint    `gorm:"not null;index;uniqueIndex:idx_subjects_account_custom_id"`
	CustomID       *string `gorm:"uniqueIndex:idx_subjects_account_custom_id"` // NULL is excluded from uniqueness
	Type           string  `gorm:"not null"`
	Name           string  `gorm:"not null"`
	FullName       string
	RegistrationNo string
	VatNo          string
	LocalVatNo     string
	Street         string
	City           string
	Zip            string
	Country        string
	Email          string
	EmailCopy      string
	Phone          string
	Web            string
	BankAccount    string
	IBAN           string
	SwiftBIC       string
	DueDays        *int // overrides Account.DefaultDueDays
	Note           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
