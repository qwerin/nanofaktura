package backup

import "time"

// Export DTOs of format version 1. They are the stable, versioned file format
// of a backup (SPEC §7.16): never GORM models nor API outputs. Field names
// mirror the model fields (convert copies them by name), JSON names are
// snake_case; IDs are the original IDs of the exporting instance and are only
// used to resolve references inside the backup. Secrets are never part of
// them (see fieldSkips in the tests).
//
// Changing a DTO incompatibly requires a new Version and a migration of the
// older version in Import.

// Account is account.json: the profile and all settings.
type Account struct {
	ID                   uint                    `json:"id"`
	Slug                 string                  `json:"slug"`
	Name                 string                  `json:"name"`
	RegistrationNo       string                  `json:"registration_no"`
	VatNo                string                  `json:"vat_no"`
	Street               string                  `json:"street"`
	City                 string                  `json:"city"`
	Zip                  string                  `json:"zip"`
	Country              string                  `json:"country"`
	Email                string                  `json:"email"`
	Phone                string                  `json:"phone"`
	Web                  string                  `json:"web"`
	VatMode              string                  `json:"vat_mode"`
	RegisteredBy         string                  `json:"registered_by"`
	DefaultCurrency      string                  `json:"default_currency"`
	DefaultDueDays       int                     `json:"default_due_days"`
	DefaultPaymentMethod string                  `json:"default_payment_method"`
	DefaultLanguage      string                  `json:"default_language"`
	DefaultNote          string                  `json:"default_note"`
	DefaultFooterNote    string                  `json:"default_footer_note"`
	RoundTotal           bool                    `json:"round_total"`
	DefaultVatRateBps    int32                   `json:"default_vat_rate_bps"`
	VatPeriod            string                  `json:"vat_period"`
	TaxOffice            string                  `json:"tax_office"`
	TaxOfficeBranch      string                  `json:"tax_office_branch"`
	LogoAttachmentID     *uint                   `json:"logo_attachment_id"`
	StampAttachmentID    *uint                   `json:"stamp_attachment_id"`
	PdfTemplate          string                  `json:"pdf_template"`
	PdfAccent            string                  `json:"pdf_accent"`
	PdfHideQR            bool                    `json:"pdf_hide_qr"`
	PdfFooter            string                  `json:"pdf_footer"`
	OnboardedAt          *time.Time              `json:"onboarded_at"`
	EmailReplyTo         string                  `json:"email_reply_to"`
	EmailSignature       string                  `json:"email_signature"`
	EmailTemplates       map[string]MailTemplate `json:"email_templates"`
	RemindersEnabled     bool                    `json:"reminders_enabled"`
	ReminderDaysAfterDue []int                   `json:"reminder_days_after_due"`
	PaidThanksEnabled    bool                    `json:"paid_thanks_enabled"`
	CreatedAt            time.Time               `json:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at"`
}

type MailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type BankAccount struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Currency     string     `json:"currency"`
	Number       string     `json:"number"`
	IBAN         string     `json:"iban"`
	SwiftBIC     string     `json:"swift_bic"`
	IsDefault    bool       `json:"is_default"`
	SyncProvider string     `json:"sync_provider"`
	HadFioToken  bool       `json:"had_fio_token"` // the token itself is never exported
	SyncFrom     string     `json:"sync_from"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	Balance      *int64     `json:"balance"`
	BalanceOn    string     `json:"balance_on"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type NumberFormat struct {
	ID           uint            `json:"id"`
	DocumentType string          `json:"document_type"`
	Format       string          `json:"format"`
	IsDefault    bool            `json:"is_default"`
	Counters     []NumberCounter `json:"counters"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type NumberCounter struct {
	Period     string `json:"period"`
	LastNumber int64  `json:"last_number"`
}

type Subject struct {
	ID             uint      `json:"id"`
	CustomID       *string   `json:"custom_id"`
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	FullName       string    `json:"full_name"`
	RegistrationNo string    `json:"registration_no"`
	VatNo          string    `json:"vat_no"`
	LocalVatNo     string    `json:"local_vat_no"`
	Street         string    `json:"street"`
	City           string    `json:"city"`
	Zip            string    `json:"zip"`
	Country        string    `json:"country"`
	Email          string    `json:"email"`
	EmailCopy      string    `json:"email_copy"`
	Phone          string    `json:"phone"`
	Web            string    `json:"web"`
	BankAccount    string    `json:"bank_account"`
	IBAN           string    `json:"iban"`
	SwiftBIC       string    `json:"swift_bic"`
	DueDays        *int      `json:"due_days"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type PriceItem struct {
	ID                 uint       `json:"id"`
	Name               string     `json:"name"`
	SKU                string     `json:"sku"`
	UnitName           string     `json:"unit_name"`
	UnitPrice          int64      `json:"unit_price"`
	VatRateBps         int32      `json:"vat_rate_bps"`
	PricesIncludeVat   bool       `json:"prices_include_vat"`
	Currency           string     `json:"currency"`
	TrackStock         bool       `json:"track_stock"`
	StockQuantityMilli int64      `json:"stock_quantity_milli"`
	MinStockMilli      *int64     `json:"min_stock_milli"`
	ArchivedAt         *time.Time `json:"archived_at"`
	Note               string     `json:"note"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type StockMove struct {
	ID            uint      `json:"id"`
	PriceItemID   uint      `json:"price_item_id"`
	Direction     string    `json:"direction"`
	QuantityMilli int64     `json:"quantity_milli"`
	MovedOn       string    `json:"moved_on"`
	Note          string    `json:"note"`
	InvoiceID     *uint     `json:"invoice_id"`
	ExpenseID     *uint     `json:"expense_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type Invoice struct {
	ID             uint   `json:"id"`
	DocumentType   string `json:"document_type"`
	Number         string `json:"number"`
	VariableSymbol string `json:"variable_symbol"`
	Status         string `json:"status"`
	SubjectID      uint   `json:"subject_id"`
	RelatedID      *uint  `json:"related_id"`
	RecurringID    *uint  `json:"recurring_id"`

	ClientName           string `json:"client_name"`
	ClientFullName       string `json:"client_full_name"`
	ClientRegistrationNo string `json:"client_registration_no"`
	ClientVatNo          string `json:"client_vat_no"`
	ClientStreet         string `json:"client_street"`
	ClientCity           string `json:"client_city"`
	ClientZip            string `json:"client_zip"`
	ClientCountry        string `json:"client_country"`
	ClientEmail          string `json:"client_email"`

	YourName           string `json:"your_name"`
	YourRegistrationNo string `json:"your_registration_no"`
	YourVatNo          string `json:"your_vat_no"`
	YourStreet         string `json:"your_street"`
	YourCity           string `json:"your_city"`
	YourZip            string `json:"your_zip"`
	YourCountry        string `json:"your_country"`
	YourRegisteredBy   string `json:"your_registered_by"`
	YourVatMode        string `json:"your_vat_mode"`

	IssuedOn              string     `json:"issued_on"`
	TaxableFulfillmentDue string     `json:"taxable_fulfillment_due"`
	DueDays               int        `json:"due_days"`
	DueOn                 string     `json:"due_on"`
	SentAt                *time.Time `json:"sent_at"`
	PaidOn                string     `json:"paid_on"`
	CancelledAt           *time.Time `json:"cancelled_at"`
	UncollectibleAt       *time.Time `json:"uncollectible_at"`
	LockedAt              *time.Time `json:"locked_at"`
	PublicViewedAt        *time.Time `json:"public_viewed_at"`

	Currency            string `json:"currency"`
	ExchangeRate        string `json:"exchange_rate"`
	Language            string `json:"language"`
	PaymentMethod       string `json:"payment_method"`
	CustomPaymentMethod string `json:"custom_payment_method"`
	BankAccountID       *uint  `json:"bank_account_id"`
	BankAccount         string `json:"bank_account"`
	IBAN                string `json:"iban"`
	SwiftBIC            string `json:"swift_bic"`

	OrderNumber      string   `json:"order_number"`
	Note             string   `json:"note"`
	FooterNote       string   `json:"footer_note"`
	PrivateNote      string   `json:"private_note"`
	Tags             []string `json:"tags"`
	PricesIncludeVat bool     `json:"prices_include_vat"`
	RoundTotal       bool     `json:"round_total"`
	ReverseCharge    bool     `json:"reverse_charge"`
	SupplyType       string   `json:"supply_type,omitempty"`
	CorrectionReason string   `json:"correction_reason,omitempty"`
	ClientLocalVatNo string   `json:"client_local_vat_no,omitempty"`

	Subtotal   int64 `json:"subtotal"`
	VatTotal   int64 `json:"vat_total"`
	Rounding   int64 `json:"rounding"`
	Total      int64 `json:"total"`
	PaidAmount int64 `json:"paid_amount"`

	Lines    []Line    `json:"lines"`
	Payments []Payment `json:"payments"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Line is a line of an invoice or an expense.
type Line struct {
	ID            uint   `json:"id"`
	PriceItemID   *uint  `json:"price_item_id"`
	Position      int    `json:"position"`
	Name          string `json:"name"`
	QuantityMilli int64  `json:"quantity_milli"`
	UnitName      string `json:"unit_name"`
	UnitPrice     int64  `json:"unit_price"`
	VatRateBps    int32  `json:"vat_rate_bps"`
	Base          int64  `json:"base"`
	Vat           int64  `json:"vat"`
	Total         int64  `json:"total"`
}

// Payment is a payment of an invoice or an expense.
type Payment struct {
	ID     uint   `json:"id"`
	PaidOn string `json:"paid_on"`
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
	// invoice payments only: the tax document of a proforma payment and the
	// proforma payment a mirrored payment comes from (ids of the backup)
	TaxDocumentID   *uint     `json:"tax_document_id,omitempty"`
	SourcePaymentID *uint     `json:"source_payment_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Expense struct {
	ID             uint   `json:"id"`
	Number         string `json:"number"`
	OriginalNumber string `json:"original_number"`
	VariableSymbol string `json:"variable_symbol"`
	Status         string `json:"status"`
	SubjectID      *uint  `json:"subject_id"`

	SupplierName           string `json:"supplier_name"`
	SupplierFullName       string `json:"supplier_full_name"`
	SupplierRegistrationNo string `json:"supplier_registration_no"`
	SupplierVatNo          string `json:"supplier_vat_no"`
	SupplierStreet         string `json:"supplier_street"`
	SupplierCity           string `json:"supplier_city"`
	SupplierZip            string `json:"supplier_zip"`
	SupplierCountry        string `json:"supplier_country"`
	SupplierBankAccount    string `json:"supplier_bank_account"`
	SupplierIBAN           string `json:"supplier_iban"`
	SupplierSwiftBIC       string `json:"supplier_swift_bic"`

	IssuedOn              string     `json:"issued_on"`
	TaxableFulfillmentDue string     `json:"taxable_fulfillment_due"`
	DueOn                 string     `json:"due_on"`
	PaidOn                string     `json:"paid_on"`
	LockedAt              *time.Time `json:"locked_at"`

	Currency         string   `json:"currency"`
	ExchangeRate     string   `json:"exchange_rate"`
	PaymentMethod    string   `json:"payment_method"`
	Category         string   `json:"category"`
	Description      string   `json:"description"`
	PrivateNote      string   `json:"private_note"`
	Tags             []string `json:"tags"`
	TaxDeductible    bool     `json:"tax_deductible"`
	VatDeductible    *bool    `json:"vat_deductible,omitempty" doc:"missing in older backups = tax_deductible"`
	PricesIncludeVat bool     `json:"prices_include_vat"`
	RoundTotal       bool     `json:"round_total"`
	ReverseCharge    bool     `json:"reverse_charge,omitempty"`
	SupplyType       string   `json:"supply_type,omitempty"`

	Subtotal   int64 `json:"subtotal"`
	VatTotal   int64 `json:"vat_total"`
	Rounding   int64 `json:"rounding"`
	Total      int64 `json:"total"`
	PaidAmount int64 `json:"paid_amount"`

	Lines    []Line    `json:"lines"`
	Payments []Payment `json:"payments"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Template struct {
	ID                  uint           `json:"id"`
	Name                string         `json:"name"`
	DocumentType        string         `json:"document_type"`
	SubjectID           uint           `json:"subject_id"`
	DueDays             *int           `json:"due_days"`
	Currency            string         `json:"currency"`
	ExchangeRate        string         `json:"exchange_rate"`
	Language            string         `json:"language"`
	PaymentMethod       string         `json:"payment_method"`
	CustomPaymentMethod string         `json:"custom_payment_method"`
	BankAccountID       *uint          `json:"bank_account_id"`
	OrderNumber         string         `json:"order_number"`
	Note                *string        `json:"note"`
	FooterNote          *string        `json:"footer_note"`
	PrivateNote         string         `json:"private_note"`
	Tags                []string       `json:"tags"`
	PricesIncludeVat    bool           `json:"prices_include_vat"`
	RoundTotal          *bool          `json:"round_total"`
	ReverseCharge       bool           `json:"reverse_charge"`
	Lines               []TemplateLine `json:"lines"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type TemplateLine struct {
	PriceItemID   *uint  `json:"price_item_id"`
	Name          string `json:"name"`
	QuantityMilli int64  `json:"quantity_milli"`
	UnitName      string `json:"unit_name"`
	UnitPrice     int64  `json:"unit_price"`
	VatRateBps    *int32 `json:"vat_rate_bps"`
}

type Recurring struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	TemplateID       uint       `json:"template_id"`
	StartOn          string     `json:"start_on"`
	NextOccurrenceOn string     `json:"next_occurrence_on"`
	EndOn            string     `json:"end_on"`
	MonthsPeriod     int        `json:"months_period"`
	DayOfMonth       *int       `json:"day_of_month"`
	IssueAs          string     `json:"issue_as"`
	SendEmail        bool       `json:"send_email"`
	Active           bool       `json:"active"`
	LastInvoiceID    *uint      `json:"last_invoice_id"`
	LastRunAt        *time.Time `json:"last_run_at"`
	LastError        string     `json:"last_error"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type BankTransaction struct {
	ID                  uint              `json:"id"`
	BankAccountID       uint              `json:"bank_account_id"`
	ExternalID          string            `json:"external_id"`
	BookedOn            string            `json:"booked_on"`
	Amount              int64             `json:"amount"`
	Currency            string            `json:"currency"`
	CounterpartyAccount string            `json:"counterparty_account"`
	CounterpartyName    string            `json:"counterparty_name"`
	VariableSymbol      string            `json:"variable_symbol"`
	ConstantSymbol      string            `json:"constant_symbol"`
	SpecificSymbol      string            `json:"specific_symbol"`
	Message             string            `json:"message"`
	MatchedInvoiceID    *uint             `json:"matched_invoice_id"`
	MatchedExpenseID    *uint             `json:"matched_expense_id"`
	PaymentID           *uint             `json:"payment_id"` // invoice or expense payment (by the matched document)
	AutoMatched         bool              `json:"auto_matched"`
	Ignored             bool              `json:"ignored"`
	Suggestions         []MatchSuggestion `json:"suggestions"`
	SuggestionCount     int               `json:"suggestion_count"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

type MatchSuggestion struct {
	InvoiceID *uint    `json:"invoice_id"`
	ExpenseID *uint    `json:"expense_id"`
	Number    string   `json:"number"`
	Name      string   `json:"name"`
	Remaining int64    `json:"remaining"`
	Score     int      `json:"score"`
	Reasons   []string `json:"reasons"`
}

type Todo struct {
	ID            uint       `json:"id"`
	Key           *string    `json:"key"` // automatic todos: "<name>:<related id>"
	Name          string     `json:"name"`
	Text          string     `json:"text"`
	RelatedType   string     `json:"related_type"`
	RelatedID     *uint      `json:"related_id"`
	DueOn         string     `json:"due_on"`
	CompletedAt   *time.Time `json:"completed_at"`
	AutoCompleted bool       `json:"auto_completed"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Event struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name"`
	SubjectType string         `json:"subject_type"`
	SubjectID   uint           `json:"subject_id"`
	Text        string         `json:"text"`
	Data        map[string]any `json:"data"`
	CreatedAt   time.Time      `json:"created_at"`
}

type EmailLog struct {
	ID           uint       `json:"id"`
	InvoiceID    uint       `json:"invoice_id"`
	Kind         string     `json:"kind"`
	To           []string   `json:"to"`
	Cc           []string   `json:"cc"`
	Subject      string     `json:"subject"`
	Body         string     `json:"body"`
	Attachments  []string   `json:"attachments"`
	ReminderStep int        `json:"reminder_step"`
	Automatic    bool       `json:"automatic"`
	SentAt       *time.Time `json:"sent_at"`
	Error        string     `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Webhook struct {
	ID                  uint       `json:"id"`
	URL                 string     `json:"url"`
	Description         string     `json:"description"`
	Events              []string   `json:"events"`
	HadSecret           bool       `json:"had_secret"` // the secret itself is never exported
	Active              bool       `json:"active"`
	LastStatus          int        `json:"last_status"`
	LastDeliveredAt     *time.Time `json:"last_delivered_at"`
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	DisabledAt          *time.Time `json:"disabled_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// Member is informative only (users are not transferred between instances).
type Member struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type Attachment struct {
	ID          uint      `json:"id"`
	OwnerType   string    `json:"owner_type"`
	OwnerID     uint      `json:"owner_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Path        string    `json:"path"` // "attachments/<id>/<filename>" in the ZIP; "" = content missing at export
	CreatedAt   time.Time `json:"created_at"`
}

// Manifest is manifest.json.
type Manifest struct {
	Format     string              `json:"format"` // Format
	Version    int                 `json:"version"`
	ExportedAt time.Time           `json:"exported_at"`
	AppVersion string              `json:"app_version"`
	Account    ManifestAccount     `json:"account"`
	Counts     map[string]int      `json:"counts"`
	Files      map[string]FileInfo `json:"files"` // every other file of the ZIP
}

type ManifestAccount struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type FileInfo struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
