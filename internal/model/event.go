package model

import "time"

// Event is one entry of the account's activity log (SPEC §7.9). It is
// written by internal/events.Record inside the transaction of the change it
// describes, so a rollback removes it as well. SubjectType/SubjectID name the
// record the event is about ("invoice", 12); Text is a Czech sentence for
// people, Data the key fields for machines (also the webhook payload).
type Event struct {
	ID          uint
	AccountID   uint           `gorm:"not null;index:idx_events_account_created;index:idx_events_subject"`
	UserID      *uint          // nil = scheduler, public link, system
	Name        string         `gorm:"not null;size:64;index"`
	SubjectType string         `gorm:"not null;size:32;index:idx_events_subject"`
	SubjectID   uint           `gorm:"not null;index:idx_events_subject"`
	Text        string         `gorm:"not null"`
	Data        map[string]any `gorm:"serializer:json"`
	CreatedAt   time.Time      `gorm:"not null;index:idx_events_account_created"`
}

// Todo is a task of the account (SPEC §7.9). Automatic todos have a Key
// ("invoice.overdue:12") unique per account, so they are upserted instead of
// duplicated and completed automatically once the condition is resolved;
// manual todos have Key nil.
type Todo struct {
	ID          uint
	AccountID   uint       `gorm:"not null;index;uniqueIndex:idx_todos_account_key"`
	Key         *string    `gorm:"size:100;uniqueIndex:idx_todos_account_key"`
	Name        string     `gorm:"not null;size:64"` // kind: invoice.overdue, bank.suggested, stock.low, recurring.failed, manual
	Text        string     `gorm:"not null"`
	RelatedType string     `gorm:"size:32;index:idx_todos_related"`
	RelatedID   *uint      `gorm:"index:idx_todos_related"`
	DueOn       string     // "YYYY-MM-DD" or ""
	UserID      *uint      // author of a manual todo
	CompletedAt *time.Time `gorm:"index"`
	// AutoCompleted: completed by the system (condition resolved); such a
	// todo is reopened when the condition returns. A todo completed by a
	// person stays completed.
	AutoCompleted bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Todo names (kinds).
const (
	TodoManual          = "manual"
	TodoInvoiceOverdue  = "invoice.overdue"
	TodoBankSuggested   = "bank.suggested"
	TodoStockLow        = "stock.low"
	TodoRecurringFailed = "recurring.failed"
)

// Webhook delivers the account's events to an URL (SPEC §7.10). Secret is
// encrypted with secret.Box (write-only in the API).
type Webhook struct {
	ID                  uint
	AccountID           uint   `gorm:"not null;index"`
	URL                 string `gorm:"not null"`
	Description         string
	Events              []string `gorm:"serializer:json"` // "*", "invoice.*", "invoice.paid"
	Secret              string   `gorm:"not null"`
	Active              bool     `gorm:"not null"`
	LastStatus          int      // HTTP status of the last attempt (0 = network error / none)
	LastDeliveredAt     *time.Time
	LastError           string
	ConsecutiveFailures int        // finally failed deliveries in a row (reset on success)
	DisabledAt          *time.Time // automatically disabled after too many failures
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Webhook delivery states.
const (
	DeliveryPending   = "pending"
	DeliveryDelivered = "delivered"
	DeliveryFailed    = "failed"
)

// WebhookDelivery is one queued/attempted POST of an event to a webhook.
// Payload is the exact JSON body (snapshot taken when the event happened).
type WebhookDelivery struct {
	ID             uint
	AccountID      uint   `gorm:"not null;index"`
	WebhookID      uint   `gorm:"not null;index"`
	EventID        *uint  // nil for pings
	EventName      string `gorm:"not null;size:64"`
	Payload        string `gorm:"not null"`
	Status         string `gorm:"not null;size:16;index:idx_webhook_deliveries_due"`
	Attempts       int
	NextAttemptAt  *time.Time `gorm:"index:idx_webhook_deliveries_due"`
	LastAttemptAt  *time.Time
	ResponseStatus int
	ResponseBody   string // first 1 KB
	Error          string
	DurationMs     int64
	CreatedAt      time.Time
}
