package models

// SubjectType rozlišuje zákazníka od dodavatele/partnera
type SubjectType string

const (
	SubjectCustomer SubjectType = "customer"
	SubjectSupplier SubjectType = "supplier"
	SubjectBoth     SubjectType = "both"
)

// Subject represents a business contact (client or supplier).
// Equivalent to Fakturoid's "subjects" resource.
type Subject struct {
	Base

	// Owner — 0 = single-user mode, >0 = user ID in multi-user mode
	UserID uint `gorm:"not null;default:0;index" json:"user_id,omitempty"`

	// CustomID je volitelný uživatelský identifikátor. Unique pouze pokud neprázdný
	// — proto *string (NULL se do unique indexu nepočítá, prázdný string ano).
	CustomID *string `gorm:"size:100;uniqueIndex" json:"custom_id,omitempty"`
	Type     SubjectType `gorm:"not null;default:'customer'" json:"type"`

	Name           string `gorm:"not null" json:"name"`
	Street         string `json:"street"`
	City           string `json:"city"`
	Zip            string `gorm:"size:5"  json:"zip"`
	Country        string `gorm:"size:2;default:'CZ'" json:"country"`
	RegistrationNo string `gorm:"size:8"  json:"registration_no"` // IČO
	VatNo          string `gorm:"size:12" json:"vat_no"`          // DIČ
	LocalVatNo     string `gorm:"size:20" json:"local_vat_no"`    // pro zahraniční

	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Website string `json:"website"`

	BankAccount string `json:"bank_account"`
	IBAN        string `json:"iban"`

	Note string `json:"note"`

	// Defaults applied when creating a new invoice for this subject
	DefaultPaymentMethod PaymentMethod `gorm:"default:'bank'" json:"default_payment_method"`
	DefaultDue           int           `gorm:"default:14"     json:"default_due"`
}
