package model

import "time"

// E-mail kinds (EmailLog.Kind, e-mail templates).
const (
	EmailInvoice    = "invoice"
	EmailReminder   = "reminder"
	EmailPaidThanks = "paid_thanks"
)

// MailTemplate is a user override of an e-mail text (placeholders {number} …).
type MailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// AccountMailSettings are the e-mail and reminder settings of an Account
// (embedded, the columns live in the accounts table).
type AccountMailSettings struct {
	EmailReplyTo   string                  // "" = account e-mail
	EmailSignature string                  // appended to the default texts
	EmailTemplates map[string]MailTemplate `gorm:"serializer:json"` // key "kind:lang"; missing = built-in default
	// RemindersEnabled turns on automatic reminders of overdue invoices.
	RemindersEnabled bool
	// ReminderDaysAfterDue are the reminder steps in days after due_on; nil = [3, 14, 30].
	ReminderDaysAfterDue []int `gorm:"serializer:json"`
	// PaidThanksEnabled sends a thank-you e-mail when an invoice gets fully paid.
	PaidThanksEnabled bool
}

// EmailLog is one e-mail sent (or attempted) about an invoice.
type EmailLog struct {
	ID           uint
	AccountID    uint     `gorm:"not null;index"`
	InvoiceID    uint     `gorm:"not null;index"`
	Kind         string   `gorm:"not null"`
	To           []string `gorm:"serializer:json"`
	Cc           []string `gorm:"serializer:json"`
	Subject      string
	Body         string
	Attachments  []string `gorm:"serializer:json"` // file names
	ReminderStep int      // automatic reminder: days after due; 0 = manual/other
	Automatic    bool     // sent by the scheduler / a hook, not by a user
	SentAt       *time.Time
	Error        string
	CreatedAt    time.Time
}
