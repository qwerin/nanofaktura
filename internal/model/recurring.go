package model

import "time"

// InvoiceTemplate holds the content of an invoice to be issued repeatedly
// (manually via create-invoice or by a Recurring). Empty/nil fields take the
// same defaults as a new invoice at the time of issuing (subject, account).
type InvoiceTemplate struct {
	ID                  uint
	AccountID           uint   `gorm:"not null;index"`
	Name                string `gorm:"not null"`
	DocumentType        string `gorm:"not null"` // invoice | proforma (default of create-invoice)
	SubjectID           uint   `gorm:"not null;index"`
	DueDays             *int
	Currency            string
	ExchangeRate        string
	Language            string
	PaymentMethod       string
	CustomPaymentMethod string
	BankAccountID       *uint
	OrderNumber         string
	Note                *string // nil = account default_note
	FooterNote          *string // nil = account default_footer_note
	PrivateNote         string
	Tags                []string `gorm:"serializer:json"`
	PricesIncludeVat    bool
	RoundTotal          *bool
	ReverseCharge       bool
	// Lines are stored as JSON: a template's lines are never queried on
	// their own and are always replaced as a whole.
	Lines     []TemplateLine `gorm:"serializer:json"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TemplateLine is one line of an InvoiceTemplate (texts may contain the date
// placeholders of billing.RenderDatePlaceholders).
type TemplateLine struct {
	PriceItemID   *uint  `json:"price_item_id,omitempty"`
	Name          string `json:"name"`
	QuantityMilli int64  `json:"quantity_milli"`
	UnitName      string `json:"unit_name,omitempty"`
	UnitPrice     int64  `json:"unit_price"`
	VatRateBps    *int32 `json:"vat_rate_bps,omitempty"` // nil = account default
}

// Recurring issues an invoice from a template every MonthsPeriod months.
// NextOccurrenceOn is the issue date of the next invoice; the day of every
// occurrence is DayOfMonth (or the day of StartOn) clamped to the month length.
type Recurring struct {
	ID               uint
	AccountID        uint   `gorm:"not null;index"`
	Name             string `gorm:"not null"`
	TemplateID       uint   `gorm:"not null;index"`
	StartOn          string `gorm:"not null"`
	NextOccurrenceOn string `gorm:"not null;index"`
	EndOn            string // "" = no end
	MonthsPeriod     int    `gorm:"not null"`
	DayOfMonth       *int
	IssueAs          string `gorm:"not null"` // invoice | proforma
	SendEmail        bool
	Active           bool `gorm:"index"`
	LastInvoiceID    *uint
	LastRunAt        *time.Time
	LastError        string // last failed generation (cleared on success)
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AnchorDayIfSet is DayOfMonth, or 0 when not set.
func (r *Recurring) AnchorDayIfSet() int {
	if r.DayOfMonth != nil {
		return *r.DayOfMonth
	}
	return 0
}

// AnchorDay is the day of month of every occurrence (before clamping).
func (r *Recurring) AnchorDay() int {
	if r.DayOfMonth != nil && *r.DayOfMonth > 0 {
		return *r.DayOfMonth
	}
	if len(r.StartOn) == 10 {
		d := int(r.StartOn[8]-'0')*10 + int(r.StartOn[9]-'0')
		if d >= 1 && d <= 31 {
			return d
		}
	}
	return 1
}
