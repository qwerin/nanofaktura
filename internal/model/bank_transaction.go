package model

import "time"

// BankTransaction is one movement imported from a bank statement or the Fio
// API (SPEC §7.6). ExternalID is unique per bank account, so re-importing a
// statement skips known movements. Amount is signed (+ incoming).
//
// A matched transaction has PaymentID set together with exactly one of
// MatchedInvoiceID (PaymentID → Payment) or MatchedExpenseID (PaymentID →
// ExpensePayment). Suggestions are kept for unmatched transactions.
type BankTransaction struct {
	ID                  uint
	AccountID           uint   `gorm:"not null;index"`
	BankAccountID       uint   `gorm:"not null;uniqueIndex:idx_bank_transactions_external"`
	ExternalID          string `gorm:"not null;size:100;uniqueIndex:idx_bank_transactions_external"`
	BookedOn            string `gorm:"not null;index"`
	Amount              int64  `gorm:"not null"`
	Currency            string `gorm:"not null"`
	CounterpartyAccount string
	CounterpartyName    string
	VariableSymbol      string `gorm:"index"`
	ConstantSymbol      string
	SpecificSymbol      string
	Message             string

	MatchedInvoiceID *uint `gorm:"index"`
	MatchedExpenseID *uint `gorm:"index"`
	PaymentID        *uint
	AutoMatched      bool
	Ignored          bool

	Suggestions     []MatchSuggestion `gorm:"serializer:json"`
	SuggestionCount int               // len(Suggestions), for filtering in SQL

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MatchSuggestion is a candidate document for an unmatched transaction
// (snapshot of the document at matching time).
type MatchSuggestion struct {
	InvoiceID *uint    `json:"invoice_id,omitempty"`
	ExpenseID *uint    `json:"expense_id,omitempty"`
	Number    string   `json:"number"`
	Name      string   `json:"name"`
	Remaining int64    `json:"remaining"`
	Score     int      `json:"score"`
	Reasons   []string `json:"reasons"`
}
