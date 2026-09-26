// Package model contains the GORM persistence structs (no API/huma concerns).
//
// Conventions (docs/SPEC.md §2): money is int64 in the smallest currency unit,
// quantities are int64 thousandths (QuantityMilli), VAT rates are basis points,
// dates are "YYYY-MM-DD" strings (*On fields) and instants are time.Time (*At).
// Every domain record carries AccountID and must always be queried scoped by it.
package model

// All returns every model for AutoMigrate.
func All() []any {
	return []any{
		&User{}, &Account{}, &Membership{}, &Session{}, &APIToken{},
		&BankAccount{}, &NumberFormat{}, &NumberCounter{},
		&Subject{}, &Invoice{}, &InvoiceLine{}, &Payment{},
		&Invitation{}, &Attachment{},
	}
}
