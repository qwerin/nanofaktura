package model

import "time"

// Attachment owner types.
const (
	OwnerInvoice = "invoice"
	OwnerExpense = "expense"
	OwnerSubject = "subject"
	OwnerAccount = "account" // OwnerID = the account's own ID (logo, stamp, documents)
)

// Attachment is file metadata; the content lives in storage under StorageKey.
type Attachment struct {
	ID          uint
	AccountID   uint   `gorm:"not null;index"`
	OwnerType   string `gorm:"not null;index:idx_attachments_owner"`
	OwnerID     uint   `gorm:"not null;index:idx_attachments_owner"`
	Filename    string `gorm:"not null"`
	ContentType string `gorm:"not null"` // sniffed from the content, never taken from the client
	Size        int64  `gorm:"not null"`
	StorageKey  string `gorm:"not null;uniqueIndex"`
	CreatedAt   time.Time
}
