package model

import "time"

// EET sale states.
const (
	EETPending   = "pending"   // not confirmed yet; the job keeps sending it
	EETConfirmed = "confirmed" // registered, POK received
	EETFailed    = "failed"    // refused (critical error); needs a fix and a retry
	EETCancelled = "cancelled" // the payment was deleted before the sale was ever sent
)

// EETConfig is the registration-of-sales (EET 2.0, SPEC §7.19) setup of an
// account: who registers (EIC), where (unit, register), in which environment
// and the cash-register certificate (PEM encrypted with secret.Box).
type EETConfig struct {
	ID          uint
	AccountID   uint   `gorm:"not null;uniqueIndex"`
	Enabled     bool   `gorm:"not null;default:false"`
	Env         string `gorm:"not null;default:''"` // eet.EnvPlayground | eet.EnvProduction
	EIC         string `gorm:"not null;default:''"`
	UnitID      string `gorm:"not null;default:''"` // id_jednotky from DIS+
	RegisterID  string `gorm:"not null;default:''"` // id_pokl, the account's own name
	Certificate string `gorm:"not null;default:''"` // encrypted; never leaves the server
	// CertEIC / CertNotAfter describe the stored certificate (no secrets).
	CertEIC      string `gorm:"not null;default:''"`
	CertNotAfter *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// EETSale is one sale registered (or to be registered) with the tax
// authority: a contact payment of an invoice, or the cancellation (negative
// amount) of a registered sale whose payment was deleted. The registered
// data (EIC … Amount) never change once created; resends only change the
// attempt bookkeeping.
type EETSale struct {
	ID        uint
	AccountID uint `gorm:"not null;index"`
	InvoiceID uint `gorm:"not null;index"`
	// InvoiceNumber is a snapshot for the list (the invoice may be deleted later).
	InvoiceNumber string `gorm:"not null;default:''"`
	// PaymentID: the payment this sale registers; nil once it was deleted
	// and for cancellations.
	PaymentID *uint `gorm:"index"`
	// CancelsID: a cancellation → the sale it cancels.
	CancelsID *uint `gorm:"index"`

	Env        string    `gorm:"not null"`
	EIC        string    `gorm:"not null"`
	UnitID     string    `gorm:"not null"`
	RegisterID string    `gorm:"not null"`
	SerialNo   string    `gorm:"not null;default:''"` // porad_cis = the sale's ID at creation
	SoldAt     time.Time `gorm:"not null"`
	Amount     int64     `gorm:"not null"` // CZK minor units

	Status        string `gorm:"not null;index"`
	POK           string `gorm:"not null;default:''"`
	Attempts      int    `gorm:"not null;default:0"`
	LastAttemptAt *time.Time
	NextAttemptAt *time.Time `gorm:"index"` // pending: when the job sends it (again)
	ConfirmedAt   *time.Time
	ErrorCode     int    `gorm:"not null;default:0"`
	Error         string `gorm:"not null;default:''"`
	Warnings      string `gorm:"not null;default:''"` // "code: text" lines of the confirmation
	TransactionID string `gorm:"not null;default:''"` // X-Global-Transaction-Id of the last answer
	CreatedAt     time.Time
}
