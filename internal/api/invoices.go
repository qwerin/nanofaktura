package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/numbering"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// ---- output DTOs ----

// InvoiceSummary is an invoice without lines, payments and VAT recap
// (list items). Invoice extends it for the detail.
type InvoiceSummary struct {
	ID             uint   `json:"id"`
	DocumentType   string `json:"document_type" enum:"invoice,proforma,correction,tax_document" doc:"tax_document = tax document for a received proforma payment (issued automatically for VAT payers)"`
	Number         string `json:"number"`
	VariableSymbol string `json:"variable_symbol"`
	Status         string `json:"status" enum:"open,sent,overdue,paid,cancelled,uncollectible" doc:"Stored status, or overdue when open/sent and due_on < today"`
	SubjectID      uint   `json:"subject_id"`
	RelatedID      *uint  `json:"related_id,omitempty" doc:"Correction → corrected invoice, final invoice → proforma"`
	RecurringID    *uint  `json:"recurring_id,omitempty" doc:"The recurring invoice that generated this document"`
	PublicToken    string `json:"public_token" doc:"Token of the public client link"`

	ClientName           string `json:"client_name"`
	ClientFullName       string `json:"client_full_name"`
	ClientRegistrationNo string `json:"client_registration_no"`
	ClientVatNo          string `json:"client_vat_no"`
	ClientStreet         string `json:"client_street"`
	ClientCity           string `json:"client_city"`
	ClientZip            string `json:"client_zip"`
	ClientCountry        string `json:"client_country"`
	ClientEmail          string `json:"client_email"`
	ClientLocalVatNo     string `json:"client_local_vat_no" doc:"Customer's local VAT number (Slovak IČ DPH), printed next to the DIČ"`

	YourName           string `json:"your_name"`
	YourRegistrationNo string `json:"your_registration_no"`
	YourVatNo          string `json:"your_vat_no"`
	YourStreet         string `json:"your_street"`
	YourCity           string `json:"your_city"`
	YourZip            string `json:"your_zip"`
	YourCountry        string `json:"your_country"`
	YourRegisteredBy   string `json:"your_registered_by"`
	YourVatMode        string `json:"your_vat_mode" enum:"non_vat_payer,vat_payer,identified_person"`

	IssuedOn              string     `json:"issued_on"`
	TaxableFulfillmentDue string     `json:"taxable_fulfillment_due"`
	DueDays               int        `json:"due_days"`
	DueOn                 string     `json:"due_on"`
	SentAt                *time.Time `json:"sent_at,omitempty"`
	PaidOn                string     `json:"paid_on"`
	CancelledAt           *time.Time `json:"cancelled_at,omitempty"`
	UncollectibleAt       *time.Time `json:"uncollectible_at,omitempty"`
	LockedAt              *time.Time `json:"locked_at,omitempty"`
	PublicViewedAt        *time.Time `json:"public_viewed_at,omitempty" doc:"First view of the public client link"`

	Currency            string `json:"currency"`
	ExchangeRate        string `json:"exchange_rate"`
	Language            string `json:"language" enum:"cs,en,sk,de"`
	PaymentMethod       string `json:"payment_method" enum:"bank,cash,card,cod,paypal,custom"`
	CustomPaymentMethod string `json:"custom_payment_method"`
	BankAccountID       *uint  `json:"bank_account_id,omitempty"`
	BankAccount         string `json:"bank_account"`
	IBAN                string `json:"iban"`
	SwiftBIC            string `json:"swift_bic"`

	OrderNumber      string   `json:"order_number"`
	Note             string   `json:"note"`
	FooterNote       string   `json:"footer_note"`
	PrivateNote      string   `json:"private_note"`
	Tags             []string `json:"tags" nullable:"false"`
	PricesIncludeVat bool     `json:"prices_include_vat"`
	RoundTotal       bool     `json:"round_total"`
	ReverseCharge    bool     `json:"reverse_charge"`
	SupplyType       string   `json:"supply_type" enum:"services,goods" doc:"EU reverse charge: services (§ 9/1, 'daň odvede zákazník') or goods (§ 64, exempt supply of goods)"`
	CorrectionReason string   `json:"correction_reason" doc:"Reason of a correction (§ 45 ZDPH); required for VAT payers"`

	Subtotal        int64 `json:"subtotal" doc:"Sum of VAT bases"`
	VatTotal        int64 `json:"vat_total"`
	Rounding        int64 `json:"rounding"`
	Total           int64 `json:"total"`
	PaidAmount      int64 `json:"paid_amount"`
	RemainingAmount int64 `json:"remaining_amount"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Invoice is the invoice detail (SPEC §4.5).
type Invoice struct {
	InvoiceSummary
	Lines       []InvoiceLine  `json:"lines" nullable:"false"`
	Payments    []Payment      `json:"payments" nullable:"false"`
	VatRecap    []VatRecapItem `json:"vat_recap" nullable:"false"`
	Warnings    []DocWarning   `json:"warnings" nullable:"false" doc:"Things the issuer should check (not errors)"`
	Attachments []Attachment   `json:"attachments" nullable:"false"`
	// RelatedDocuments are the documents referring to this one (corrections
	// of an invoice; tax documents and the final invoice of a proforma).
	RelatedDocuments []RelatedDocument `json:"related_documents" nullable:"false"`
	// Deposits are the tax documents of the proforma deducted on a final
	// invoice ("odpočet zálohy"), in the document currency.
	Deposits []Deposit `json:"deposits" nullable:"false"`
}

// RelatedDocument is a document whose related_id points to the invoice.
type RelatedDocument struct {
	ID           uint   `json:"id"`
	DocumentType string `json:"document_type" enum:"invoice,proforma,correction,tax_document"`
	Number       string `json:"number"`
	Status       string `json:"status"`
	Total        int64  `json:"total"`
}

// Deposit is a tax document for a received advance deducted on the final invoice.
type Deposit struct {
	TaxDocumentID uint           `json:"tax_document_id"`
	Number        string         `json:"number"`
	TaxPointDate  string         `json:"taxable_fulfillment_due"`
	VatRecap      []VatRecapItem `json:"vat_recap" nullable:"false"`
	Total         int64          `json:"total"`
}

// DocWarning is a non-blocking problem of a document.
type DocWarning struct {
	Code    string `json:"code" enum:"no_bank_account" doc:"no_bank_account: bank transfer, but no bank account in the invoice currency — the document has no payment details (account number, QR)"`
	Message string `json:"message"`
}

// invoiceWarnings derives the warnings of an invoice from its stored data.
func invoiceWarnings(m *model.Invoice) []DocWarning {
	out := []DocWarning{}
	if m.PaymentMethod == "bank" && m.IBAN == "" && m.BankAccount == "" && m.DocumentType != model.DocCorrection {
		out = append(out, DocWarning{Code: "no_bank_account",
			Message: "no bank account in " + m.Currency + ": the document has no payment details; add a bank account in this currency"})
	}
	return out
}

type InvoiceLine struct {
	ID          uint   `json:"id"`
	Position    int    `json:"position"`
	PriceItemID *uint  `json:"price_item_id,omitempty"`
	Name        string `json:"name"`
	Quantity    string `json:"quantity" example:"1.5"`
	UnitName    string `json:"unit_name"`
	UnitPrice   int64  `json:"unit_price"`
	VatRateBps  int32  `json:"vat_rate_bps"`
	Base        int64  `json:"base"`
	Vat         int64  `json:"vat"`
	Total       int64  `json:"total"`
}

type VatRecapItem struct {
	VatRateBps int32 `json:"vat_rate_bps"`
	Base       int64 `json:"base"`
	Vat        int64 `json:"vat"`
	Total      int64 `json:"total"`
}

type Payment struct {
	ID              uint      `json:"id"`
	InvoiceID       uint      `json:"invoice_id"`
	PaidOn          string    `json:"paid_on"`
	Amount          int64     `json:"amount"`
	Note            string    `json:"note"`
	TaxDocumentID   *uint     `json:"tax_document_id,omitempty" doc:"Proforma payment of a VAT payer: its tax document"`
	SourcePaymentID *uint     `json:"source_payment_id,omitempty" doc:"Payment taken over from a proforma payment (final invoice, tax document); delete that one instead"`
	CreatedAt       time.Time `json:"created_at"`
}

func toPayment(p *model.Payment) Payment {
	return Payment{ID: p.ID, InvoiceID: p.InvoiceID, PaidOn: p.PaidOn, Amount: p.Amount, Note: p.Note,
		TaxDocumentID: p.TaxDocumentID, SourcePaymentID: p.SourcePaymentID, CreatedAt: p.CreatedAt}
}

func toInvoiceSummary(m *model.Invoice, today string) InvoiceSummary {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	return InvoiceSummary{
		ID: m.ID, DocumentType: m.DocumentType, Number: m.Number, VariableSymbol: m.VariableSymbol,
		Status: billing.EffectiveStatus(m.Status, m.DueOn, today), SubjectID: m.SubjectID, RelatedID: m.RelatedID, RecurringID: m.RecurringID,
		PublicToken: m.PublicToken,

		ClientName: m.ClientName, ClientFullName: m.ClientFullName, ClientRegistrationNo: m.ClientRegistrationNo,
		ClientVatNo: m.ClientVatNo, ClientStreet: m.ClientStreet, ClientCity: m.ClientCity, ClientZip: m.ClientZip,
		ClientCountry: m.ClientCountry, ClientEmail: m.ClientEmail, ClientLocalVatNo: m.ClientLocalVatNo,

		YourName: m.YourName, YourRegistrationNo: m.YourRegistrationNo, YourVatNo: m.YourVatNo,
		YourStreet: m.YourStreet, YourCity: m.YourCity, YourZip: m.YourZip, YourCountry: m.YourCountry,
		YourRegisteredBy: m.YourRegisteredBy, YourVatMode: m.YourVatMode,

		IssuedOn: m.IssuedOn, TaxableFulfillmentDue: m.TaxableFulfillmentDue, DueDays: m.DueDays, DueOn: m.DueOn,
		SentAt: m.SentAt, PaidOn: m.PaidOn, CancelledAt: m.CancelledAt, UncollectibleAt: m.UncollectibleAt, LockedAt: m.LockedAt,
		PublicViewedAt: m.PublicViewedAt,

		Currency: m.Currency, ExchangeRate: m.ExchangeRate, Language: m.Language, PaymentMethod: m.PaymentMethod,
		CustomPaymentMethod: m.CustomPaymentMethod, BankAccountID: m.BankAccountID, BankAccount: m.BankAccount,
		IBAN: m.IBAN, SwiftBIC: m.SwiftBIC,

		OrderNumber: m.OrderNumber, Note: m.Note, FooterNote: m.FooterNote, PrivateNote: m.PrivateNote, Tags: tags,
		PricesIncludeVat: m.PricesIncludeVat, RoundTotal: m.RoundTotal, ReverseCharge: m.ReverseCharge,
		SupplyType: defaultStr(m.SupplyType, model.SupplyServices), CorrectionReason: m.CorrectionReason,

		Subtotal: m.Subtotal, VatTotal: m.VatTotal, Rounding: m.Rounding, Total: m.Total,
		PaidAmount: m.PaidAmount, RemainingAmount: m.Total - m.PaidAmount,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// toInvoice converts an invoice loaded with Lines and Payments.
func toInvoice(m *model.Invoice, today string) Invoice {
	out := Invoice{
		InvoiceSummary:   toInvoiceSummary(m, today),
		Lines:            make([]InvoiceLine, len(m.Lines)),
		Payments:         make([]Payment, len(m.Payments)),
		VatRecap:         []VatRecapItem{},
		Warnings:         invoiceWarnings(m),
		Attachments:      []Attachment{},
		RelatedDocuments: []RelatedDocument{},
		Deposits:         []Deposit{},
	}
	for i, l := range m.Lines {
		out.Lines[i] = InvoiceLine{
			ID: l.ID, Position: l.Position, PriceItemID: l.PriceItemID, Name: l.Name,
			Quantity: billing.FormatQuantity(l.QuantityMilli), UnitName: l.UnitName, UnitPrice: l.UnitPrice,
			VatRateBps: l.VatRateBps, Base: l.Base, Vat: l.Vat, Total: l.Total,
		}
	}
	for i := range m.Payments {
		out.Payments[i] = toPayment(&m.Payments[i])
	}
	if t, err := billing.Calculate(billingLines(m.Lines), billingOptions(m)); err == nil {
		for _, r := range t.VatRecap {
			out.VatRecap = append(out.VatRecap, VatRecapItem{VatRateBps: r.VatRateBps, Base: r.Base, Vat: r.Vat, Total: r.Total})
		}
	}
	return out
}

// ---- input DTOs ----

// InvoiceLineInput is one line of a create/patch body. In PATCH the line
// list replaces the stored one: lines with id update that line, lines
// without id are inserted, missing lines are deleted. Every line object is
// complete — omitted optional fields take their defaults.
type InvoiceLineInput struct {
	ID          *uint  `json:"id,omitempty" doc:"PATCH only: existing line to update"`
	PriceItemID *uint  `json:"price_item_id,omitempty"`
	Name        string `json:"name" minLength:"1" maxLength:"500"`
	Quantity    string `json:"quantity,omitempty" maxLength:"20" example:"1.5" doc:"Decimal ('.' or ','), max 3 places, may be negative; default 1"`
	UnitName    string `json:"unit_name,omitempty" maxLength:"20"`
	UnitPrice   int64  `json:"unit_price" minimum:"-100000000000000" maximum:"100000000000000" doc:"Minor units; VAT included when prices_include_vat"`
	VatRateBps  *int32 `json:"vat_rate_bps,omitempty" minimum:"0" maximum:"10000" doc:"Default: account default_vat_rate_bps; forced to 0 for non VAT payers"`
}

// InvoiceSnapshotFields override the client_*, your_* and bank snapshots
// (taken from the subject, the account and the bank account).
type InvoiceSnapshotFields struct {
	ClientName           *string `json:"client_name,omitempty" maxLength:"200"`
	ClientFullName       *string `json:"client_full_name,omitempty" maxLength:"200"`
	ClientRegistrationNo *string `json:"client_registration_no,omitempty" maxLength:"20"`
	ClientVatNo          *string `json:"client_vat_no,omitempty" maxLength:"20"`
	ClientStreet         *string `json:"client_street,omitempty" maxLength:"200"`
	ClientCity           *string `json:"client_city,omitempty" maxLength:"100"`
	ClientZip            *string `json:"client_zip,omitempty" maxLength:"20"`
	ClientCountry        *string `json:"client_country,omitempty" maxLength:"2"`
	ClientEmail          *string `json:"client_email,omitempty" maxLength:"254"`
	ClientLocalVatNo     *string `json:"client_local_vat_no,omitempty" maxLength:"20"`

	YourName           *string `json:"your_name,omitempty" maxLength:"200"`
	YourRegistrationNo *string `json:"your_registration_no,omitempty" maxLength:"20"`
	YourVatNo          *string `json:"your_vat_no,omitempty" maxLength:"20"`
	YourStreet         *string `json:"your_street,omitempty" maxLength:"200"`
	YourCity           *string `json:"your_city,omitempty" maxLength:"100"`
	YourZip            *string `json:"your_zip,omitempty" maxLength:"20"`
	YourCountry        *string `json:"your_country,omitempty" maxLength:"2"`
	YourRegisteredBy   *string `json:"your_registered_by,omitempty" maxLength:"500"`
	YourVatMode        *string `json:"your_vat_mode,omitempty" enum:"non_vat_payer,vat_payer,identified_person"`

	BankAccount *string `json:"bank_account,omitempty" maxLength:"50"`
	IBAN        *string `json:"iban,omitempty" maxLength:"42"`
	SwiftBIC    *string `json:"swift_bic,omitempty" maxLength:"11"`
}

func (f *InvoiceSnapshotFields) applyTo(m *model.Invoice) {
	apply(&m.ClientName, f.ClientName)
	apply(&m.ClientFullName, f.ClientFullName)
	apply(&m.ClientRegistrationNo, f.ClientRegistrationNo)
	apply(&m.ClientVatNo, f.ClientVatNo)
	apply(&m.ClientStreet, f.ClientStreet)
	apply(&m.ClientCity, f.ClientCity)
	apply(&m.ClientZip, f.ClientZip)
	apply(&m.ClientCountry, f.ClientCountry)
	apply(&m.ClientEmail, f.ClientEmail)
	apply(&m.ClientLocalVatNo, f.ClientLocalVatNo)
	apply(&m.YourName, f.YourName)
	apply(&m.YourRegistrationNo, f.YourRegistrationNo)
	apply(&m.YourVatNo, f.YourVatNo)
	apply(&m.YourStreet, f.YourStreet)
	apply(&m.YourCity, f.YourCity)
	apply(&m.YourZip, f.YourZip)
	apply(&m.YourCountry, f.YourCountry)
	apply(&m.YourRegisteredBy, f.YourRegisteredBy)
	apply(&m.YourVatMode, f.YourVatMode)
	apply(&m.BankAccount, f.BankAccount)
	apply(&m.IBAN, f.IBAN)
	apply(&m.SwiftBIC, f.SwiftBIC)
}

type InvoiceCreate struct {
	DocumentType   string  `json:"document_type,omitempty" enum:"invoice,proforma,correction" doc:"Default invoice"`
	Number         string  `json:"number,omitempty" maxLength:"50" doc:"Custom number (the counter is not advanced); default: next number of the default number format"`
	VariableSymbol *string `json:"variable_symbol,omitempty" pattern:"^[0-9]{0,10}$" doc:"Default: digits of the number (last 10)"`
	SubjectID      uint    `json:"subject_id" minimum:"1"`
	RelatedID      *uint   `json:"related_id,omitempty" doc:"Required for corrections (the corrected invoice)"`
	InvoiceSnapshotFields

	IssuedOn              string  `json:"issued_on,omitempty" format:"date" doc:"Default today"`
	TaxableFulfillmentDue *string `json:"taxable_fulfillment_due,omitempty" doc:"YYYY-MM-DD or empty; default issued_on for VAT payers, empty otherwise"`
	DueDays               *int    `json:"due_days,omitempty" minimum:"0" maximum:"365" doc:"Default: subject due_days, then account default_due_days"`

	Currency            string   `json:"currency,omitempty" pattern:"^[A-Z]{3}$" doc:"Default: account default_currency"`
	ExchangeRate        string   `json:"exchange_rate,omitempty" pattern:"^[0-9]{1,6}([.][0-9]{1,6})?$" doc:"CZK per unit, > 0; default: ČNB rate of the DUZP (issue date) for a foreign currency, 1 for CZK"`
	Language            string   `json:"language,omitempty" enum:"cs,en,sk,de"`
	PaymentMethod       string   `json:"payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom"`
	CustomPaymentMethod string   `json:"custom_payment_method,omitempty" maxLength:"100"`
	BankAccountID       *uint    `json:"bank_account_id,omitempty" doc:"Default: the default bank account of the currency"`
	OrderNumber         string   `json:"order_number,omitempty" maxLength:"100"`
	Note                *string  `json:"note,omitempty" maxLength:"5000" doc:"Default: account default_note"`
	FooterNote          *string  `json:"footer_note,omitempty" maxLength:"5000" doc:"Default: account default_footer_note"`
	PrivateNote         string   `json:"private_note,omitempty" maxLength:"5000"`
	Tags                []string `json:"tags,omitempty" maxItems:"50"`
	PricesIncludeVat    bool     `json:"prices_include_vat,omitempty"`
	RoundTotal          *bool    `json:"round_total,omitempty" doc:"Default: account round_total"`
	ReverseCharge       bool     `json:"reverse_charge,omitempty"`
	SupplyType          string   `json:"supply_type,omitempty" enum:"services,goods" doc:"EU reverse charge: services (default) or goods (§ 64)"`
	CorrectionReason    string   `json:"correction_reason,omitempty" maxLength:"500" doc:"Reason of a correction (required for VAT payers)"`

	Lines []InvoiceLineInput `json:"lines" minItems:"1" maxItems:"500"`
}

// InvoicePatch: nil fields are left unchanged; lines (when present) replace
// the whole list.
type InvoicePatch struct {
	Number         *string `json:"number,omitempty" minLength:"1" maxLength:"50"`
	VariableSymbol *string `json:"variable_symbol,omitempty" pattern:"^[0-9]{0,10}$"`
	SubjectID      *uint   `json:"subject_id,omitempty" minimum:"1" doc:"Changing the subject re-snapshots client_* (unless sent explicitly)"`
	RelatedID      *uint   `json:"related_id,omitempty"`
	InvoiceSnapshotFields

	IssuedOn              *string `json:"issued_on,omitempty" format:"date"`
	TaxableFulfillmentDue *string `json:"taxable_fulfillment_due,omitempty"`
	DueDays               *int    `json:"due_days,omitempty" minimum:"0" maximum:"365"`

	Currency            *string  `json:"currency,omitempty" pattern:"^[A-Z]{3}$"`
	ExchangeRate        *string  `json:"exchange_rate,omitempty" pattern:"^[0-9]{1,6}([.][0-9]{1,6})?$"`
	Language            *string  `json:"language,omitempty" enum:"cs,en,sk,de"`
	PaymentMethod       *string  `json:"payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom"`
	CustomPaymentMethod *string  `json:"custom_payment_method,omitempty" maxLength:"100"`
	BankAccountID       *uint    `json:"bank_account_id,omitempty" doc:"Re-snapshots bank_account/iban/swift_bic"`
	OrderNumber         *string  `json:"order_number,omitempty" maxLength:"100"`
	Note                *string  `json:"note,omitempty" maxLength:"5000"`
	FooterNote          *string  `json:"footer_note,omitempty" maxLength:"5000"`
	PrivateNote         *string  `json:"private_note,omitempty" maxLength:"5000"`
	Tags                []string `json:"tags,omitempty" maxItems:"50" doc:"Replaces all tags; [] clears"`
	PricesIncludeVat    *bool    `json:"prices_include_vat,omitempty"`
	RoundTotal          *bool    `json:"round_total,omitempty"`
	ReverseCharge       *bool    `json:"reverse_charge,omitempty"`
	SupplyType          *string  `json:"supply_type,omitempty" enum:"services,goods"`
	CorrectionReason    *string  `json:"correction_reason,omitempty" maxLength:"500"`

	Lines []InvoiceLineInput `json:"lines,omitempty" minItems:"1" maxItems:"500"`
}

// ---- routes ----

func (s *server) registerInvoices(g huma.API) {
	huma.Get(g, "/invoices", s.listInvoices)
	huma.Post(g, "/invoices", s.createInvoice, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/invoices/{id}", s.getInvoice)
	huma.Patch(g, "/invoices/{id}", s.patchInvoice, auth.ForEditors)
	huma.Delete(g, "/invoices/{id}", s.deleteInvoice, status(http.StatusNoContent), auth.ForEditors)
}

type invoiceID struct {
	ID uint `path:"id"`
}

func (s *server) today() string { return billing.Today(s.deps.Now()) }

func (s *server) listInvoices(ctx context.Context, in *struct {
	PageParams
	InvoiceFilter
}) (*Out[DocumentList[InvoiceSummary]], error) {
	today := s.today()
	page, err := paginate(in.InvoiceFilter.query(s.scoped(ctx), today), in.PageParams, func(m *model.Invoice) InvoiceSummary {
		return toInvoiceSummary(m, today)
	})
	if err != nil {
		return nil, err
	}
	sums, err := currencySums(in.InvoiceFilter.where(s.scoped(ctx), today))
	if err != nil {
		return nil, err
	}
	return &Out[DocumentList[InvoiceSummary]]{Body: DocumentList[InvoiceSummary]{ListResponse: page.Body, Sums: sums}}, nil
}

// loadInvoice loads an invoice of the current account with ordered lines and payments.
func loadInvoice(ctx context.Context, db *gorm.DB, id uint) (*model.Invoice, error) {
	var m model.Invoice
	err := db.Scopes(inAccount(ctx)).
		Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("position, id") }).
		Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("paid_on, id") }).
		First(&m, id).Error
	if err != nil {
		return nil, dbErr(err, "invoice")
	}
	return &m, nil
}

// loadInvoiceForUpdate locks the invoice row (SELECT … FOR UPDATE on
// PostgreSQL; SQLite serialises transactions on its single connection) and
// then loads it. Every transaction that reads, modifies and writes an
// invoice's money or status must load it this way, so concurrent payments,
// PATCHes, actions and bank matches of the same document run one after
// another and each sees the result of the previous one.
func loadInvoiceForUpdate(ctx context.Context, tx *gorm.DB, id uint) (*model.Invoice, error) {
	var row model.Invoice
	if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Scopes(inAccount(ctx)).
		Select("id").First(&row, id).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	return loadInvoice(ctx, tx, id)
}

// invoiceOut reloads the invoice (inside or after a transaction) and converts it.
func (s *server) invoiceOut(ctx context.Context, db *gorm.DB, id uint) (*Out[Invoice], error) {
	m, err := loadInvoice(ctx, db, id)
	if err != nil {
		return nil, err
	}
	out := toInvoice(m, s.today())
	if out.Attachments, err = ownerAttachments(ctx, db, model.OwnerInvoice, m.ID); err != nil {
		return nil, err
	}
	if out.RelatedDocuments, err = relatedDocuments(ctx, db, m.ID); err != nil {
		return nil, err
	}
	deps, err := invoiceDeposits(ctx, db, m)
	if err != nil {
		return nil, err
	}
	out.Deposits = toDeposits(deps)
	return &Out[Invoice]{Body: out}, nil
}

func (s *server) getInvoice(ctx context.Context, in *invoiceID) (*Out[Invoice], error) {
	return s.invoiceOut(ctx, s.db.WithContext(ctx), in.ID)
}

// mutateInvoice runs fn in a transaction and returns the invoice whose id fn
// returned, reloaded inside the same transaction.
func (s *server) mutateInvoice(ctx context.Context, fn func(tx *gorm.DB) (uint, error)) (*Out[Invoice], error) {
	var out *Out[Invoice]
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, err := fn(tx)
		if err != nil {
			return err
		}
		out, err = s.invoiceOut(ctx, tx, id)
		return err
	})
	return out, err
}

func (s *server) createInvoice(ctx context.Context, in *struct{ Body InvoiceCreate }) (*Out[Invoice], error) {
	b := &in.Body
	var err error
	if b.ExchangeRate, err = s.defaultExchangeRate(ctx, b.Currency, b.ExchangeRate, strOrEmpty(b.TaxableFulfillmentDue), b.IssuedOn); err != nil {
		return nil, err
	}
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := s.createInvoiceTx(ctx, tx, &in.Body)
		if err != nil {
			return 0, err
		}
		return m.ID, nil
	})
}

// createInvoiceTx builds and stores a new invoice from in (all defaults and
// snapshots per SPEC §4.5), assigning its number inside tx.
func (s *server) createInvoiceTx(ctx context.Context, tx *gorm.DB, in *InvoiceCreate) (*model.Invoice, error) {
	acc := auth.AccountFrom(ctx)
	m := &model.Invoice{
		AccountID:    acc.ID,
		DocumentType: defaultStr(in.DocumentType, model.DocInvoice),
		Status:       model.StatusOpen,
		RelatedID:    in.RelatedID,
		PublicToken:  newPublicToken(),
	}
	subj, err := findSubject(ctx, tx, in.SubjectID)
	if err != nil {
		return nil, err
	}
	m.SubjectID = subj.ID
	snapshotClient(m, subj)
	snapshotYour(m, acc)

	m.IssuedOn = defaultStr(in.IssuedOn, s.today())
	if !billing.ValidDate(m.IssuedOn) {
		return nil, invalid("issued_on", "invalid date")
	}
	switch {
	case in.DueDays != nil:
		m.DueDays = *in.DueDays
	case subj.DueDays != nil:
		m.DueDays = *subj.DueDays
	default:
		m.DueDays = acc.DefaultDueDays
	}
	m.Currency = defaultStr(in.Currency, acc.DefaultCurrency)
	m.ExchangeRate = defaultStr(in.ExchangeRate, "1")
	m.Language = defaultStr(in.Language, acc.DefaultLanguage)
	m.PaymentMethod = defaultStr(in.PaymentMethod, acc.DefaultPaymentMethod)
	m.CustomPaymentMethod = in.CustomPaymentMethod
	m.RoundTotal = acc.RoundTotal
	apply(&m.RoundTotal, in.RoundTotal)
	m.Note = acc.DefaultNote
	apply(&m.Note, in.Note)
	m.FooterNote = acc.DefaultFooterNote
	apply(&m.FooterNote, in.FooterNote)
	m.OrderNumber = in.OrderNumber
	m.PrivateNote = in.PrivateNote
	m.Tags = normalizeTags(in.Tags)
	m.PricesIncludeVat = in.PricesIncludeVat
	m.ReverseCharge = in.ReverseCharge
	m.SupplyType = in.SupplyType
	m.CorrectionReason = strings.TrimSpace(in.CorrectionReason)
	if in.ExchangeRate == "" && needsCNBRate(acc, m.Currency) {
		// callers resolve the ČNB rate before the transaction (defaultExchangeRate);
		// never fall back to 1 silently
		return nil, invalid("exchange_rate", "exchange rate of "+m.Currency+" is missing; enter exchange_rate")
	}

	if err := snapshotBank(ctx, tx, m, in.BankAccountID); err != nil {
		return nil, err
	}
	in.InvoiceSnapshotFields.applyTo(m) // explicit overrides win over snapshots

	if m.YourVatMode != model.VatModeNonPayer {
		m.TaxableFulfillmentDue = m.IssuedOn
	}
	if in.TaxableFulfillmentDue != nil {
		m.TaxableFulfillmentDue = *in.TaxableFulfillmentDue
	}
	if err := checkTaxFields(m); err != nil {
		return nil, err
	}
	if err := checkRelated(ctx, tx, m); err != nil {
		return nil, err
	}
	if err := checkPriceItems(ctx, tx, in.Lines); err != nil {
		return nil, err
	}
	if m.Lines, err = buildLines(in.Lines, nil, acc.DefaultVatRateBps); err != nil {
		return nil, err
	}
	if err := recalc(m); err != nil {
		return nil, err
	}

	if n := strings.TrimSpace(in.Number); n != "" {
		m.Number = n
	} else if m.Number, err = s.deps.NextNumber(tx, acc.ID, m.DocumentType, m.IssuedOn); err != nil {
		return nil, numberingErr(err)
	}
	m.VariableSymbol = spayd.Digits(m.Number, 10)
	apply(&m.VariableSymbol, in.VariableSymbol)
	if err := checkTaxNumberUnique(ctx, tx, m); err != nil {
		return nil, err
	}

	if err := tx.Create(m).Error; err != nil {
		return nil, numberErr(err, m.Number)
	}
	if err := syncInvoiceStock(ctx, tx, m); err != nil {
		return nil, err
	}
	return m, recordInvoice(ctx, tx, events.InvoiceCreated, m)
}

func (s *server) patchInvoice(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body InvoicePatch
}) (*Out[Invoice], error) {
	p := &in.Body
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := loadInvoiceForUpdate(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		if err := editable(m); err != nil {
			return 0, err
		}
		if err := checkNotSettled(ctx, tx, m); err != nil {
			return 0, err
		}
		if m.DocumentType == model.DocTaxDocument && (p.Lines != nil || p.Currency != nil || p.ExchangeRate != nil ||
			p.PricesIncludeVat != nil || p.RoundTotal != nil || p.ReverseCharge != nil || p.RelatedID != nil ||
			p.YourVatMode != nil || p.TaxableFulfillmentDue != nil) {
			return 0, conflict(CodeTaxDocumentFixed, "the amounts, currency and dates of a tax document follow its proforma payment; delete the payment instead")
		}
		if p.Currency != nil && *p.Currency != m.Currency && len(m.Payments) > 0 {
			return 0, conflict(CodeCurrencyHasPayments, "the document has payments in "+m.Currency+"; delete them before changing the currency")
		}
		acc := auth.AccountFrom(ctx)

		if p.SubjectID != nil && *p.SubjectID != m.SubjectID {
			subj, err := findSubject(ctx, tx, *p.SubjectID)
			if err != nil {
				return 0, err
			}
			m.SubjectID = subj.ID
			snapshotClient(m, subj)
		}
		if p.RelatedID != nil {
			m.RelatedID = p.RelatedID
		}
		if p.Number != nil {
			if m.Number = strings.TrimSpace(*p.Number); m.Number == "" {
				return 0, invalid("number", "number must not be empty")
			}
			if err := checkTaxNumberUnique(ctx, tx, m); err != nil {
				return 0, err
			}
		}
		apply(&m.VariableSymbol, p.VariableSymbol)
		apply(&m.IssuedOn, p.IssuedOn)
		if !billing.ValidDate(m.IssuedOn) {
			return 0, invalid("issued_on", "invalid date")
		}
		apply(&m.TaxableFulfillmentDue, p.TaxableFulfillmentDue)
		apply(&m.DueDays, p.DueDays)
		currencyChanged := p.Currency != nil && *p.Currency != m.Currency
		apply(&m.Currency, p.Currency)
		apply(&m.ExchangeRate, p.ExchangeRate)
		apply(&m.Language, p.Language)
		apply(&m.PaymentMethod, p.PaymentMethod)
		apply(&m.CustomPaymentMethod, p.CustomPaymentMethod)
		apply(&m.OrderNumber, p.OrderNumber)
		apply(&m.Note, p.Note)
		apply(&m.FooterNote, p.FooterNote)
		apply(&m.PrivateNote, p.PrivateNote)
		if p.Tags != nil {
			m.Tags = normalizeTags(p.Tags)
		}
		apply(&m.PricesIncludeVat, p.PricesIncludeVat)
		apply(&m.RoundTotal, p.RoundTotal)
		apply(&m.ReverseCharge, p.ReverseCharge)
		apply(&m.SupplyType, p.SupplyType)
		if p.CorrectionReason != nil {
			m.CorrectionReason = strings.TrimSpace(*p.CorrectionReason)
		}
		if p.BankAccountID != nil || currencyChanged {
			if err := snapshotBank(ctx, tx, m, p.BankAccountID); err != nil {
				return 0, err
			}
		}
		p.InvoiceSnapshotFields.applyTo(m)
		if err := checkTaxFields(m); err != nil {
			return 0, err
		}
		if err := checkRelated(ctx, tx, m); err != nil {
			return 0, err
		}
		if p.Lines != nil {
			if err := checkPriceItems(ctx, tx, p.Lines); err != nil {
				return 0, err
			}
			if m.Lines, err = buildLines(p.Lines, m.Lines, acc.DefaultVatRateBps); err != nil {
				return 0, err
			}
		}
		if err := recalc(m); err != nil {
			return 0, err
		}
		applyPayments(m)

		if err := tx.Omit(clause.Associations).Save(m).Error; err != nil {
			return 0, numberErr(err, m.Number)
		}
		if err := saveLines(tx, m); err != nil {
			return 0, err
		}
		if err := syncInvoiceStock(ctx, tx, m); err != nil {
			return 0, err
		}
		return m.ID, recordInvoice(ctx, tx, events.InvoiceUpdated, m)
	})
}

func (s *server) deleteInvoice(ctx context.Context, in *invoiceID) (*NoContent, error) {
	var files []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadInvoiceForUpdate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if m.LockedAt != nil {
			return conflict(CodeLocked, "the invoice is locked; unlock it first")
		}
		if m.DocumentType == model.DocTaxDocument {
			return conflict(CodeTaxDocumentFixed, "a tax document is deleted together with its proforma payment")
		}
		// a final invoice may go with the payments it took over from its proforma
		mirrored := 0
		for _, p := range m.Payments {
			if p.SourcePaymentID != nil {
				mirrored++
			}
		}
		if len(m.Payments) > mirrored {
			return conflict(CodeHasPayments, "the invoice has payments; delete them first")
		}
		var ref model.Invoice
		res := tx.Scopes(inAccount(ctx)).Select("id", "number").Where("related_id = ?", m.ID).Limit(1).Find(&ref)
		if res.Error != nil {
			return dbErr(res.Error, "invoice")
		}
		if res.RowsAffected > 0 {
			return conflict(CodeReferenced, "the document is referenced by "+ref.Number+"; delete that document first")
		}
		if files, err = deleteOwnerAttachments(ctx, tx, model.OwnerInvoice, m.ID); err != nil {
			return err
		}
		if err := clearInvoiceStock(ctx, tx, m.ID); err != nil {
			return err
		}
		if err := tx.Where("invoice_id = ?", m.ID).Delete(&model.InvoiceLine{}).Error; err != nil {
			return dbErr(err, "invoice")
		}
		if err := tx.Where("invoice_id = ?", m.ID).Delete(&model.Payment{}).Error; err != nil {
			return dbErr(err, "invoice")
		}
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "invoice")
		}
		if err := recordInvoice(ctx, tx, events.InvoiceDeleted, m); err != nil {
			return err
		}
		if m.DocumentType == model.DocInvoice && m.RelatedID != nil {
			// deleting the final invoice reopens its proforma
			pro, err := loadInvoiceForUpdate(ctx, tx, *m.RelatedID)
			if err != nil {
				return err
			}
			if pro.DocumentType == model.DocProforma && pro.Status == model.StatusPaid {
				return refreshInvoicePayments(tx, pro)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.removeFiles(ctx, files)
	return &NoContent{}, nil
}

// ---- helpers ----

// editable returns 409 when the invoice must not be changed.
func editable(m *model.Invoice) error {
	if m.LockedAt != nil {
		return conflict(CodeLocked, "the invoice is locked; unlock it first")
	}
	if m.Status == model.StatusCancelled || m.Status == model.StatusUncollectible {
		return conflict(CodeNotEditable, "a "+m.Status+" invoice cannot be edited")
	}
	return nil
}

func findSubject(ctx context.Context, tx *gorm.DB, id uint) (*model.Subject, error) {
	var subj model.Subject
	if err := tx.Scopes(inAccount(ctx)).First(&subj, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, invalid("subject_id", "subject not found")
		}
		return nil, dbErr(err, "subject")
	}
	return &subj, nil
}

// checkRelated validates related_id: required for corrections (an invoice of
// the account), otherwise optional but must exist in the account.
func checkRelated(ctx context.Context, tx *gorm.DB, m *model.Invoice) error {
	if m.RelatedID == nil {
		if m.DocumentType == model.DocCorrection {
			return invalid("related_id", "a correction requires related_id (the corrected invoice)")
		}
		return nil
	}
	if m.ID != 0 && *m.RelatedID == m.ID {
		return invalid("related_id", "a document cannot relate to itself")
	}
	var rel model.Invoice
	if err := tx.Scopes(inAccount(ctx)).Select("id", "document_type").First(&rel, *m.RelatedID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid("related_id", "related document not found")
		}
		return dbErr(err, "invoice")
	}
	if m.DocumentType == model.DocCorrection && rel.DocumentType != model.DocInvoice {
		return invalid("related_id", "a correction must relate to an invoice")
	}
	if m.DocumentType == model.DocTaxDocument && rel.DocumentType != model.DocProforma {
		return invalid("related_id", "a tax document must relate to a proforma")
	}
	return nil
}

// isTaxDocument: m is a tax document of its issuer (DUZP and the VAT
// wording are mandatory) — every non-proforma document of a VAT payer, and
// reverse-charge documents of an identified person (EU services, § 28 odst. 2).
func isTaxDocument(m *model.Invoice) bool {
	if m.DocumentType == model.DocProforma {
		return false
	}
	return m.YourVatMode == model.VatModePayer || (m.YourVatMode == model.VatModeIdentifiedPerson && m.ReverseCharge)
}

// checkTaxFields validates the data a tax document must carry.
func checkTaxFields(m *model.Invoice) error {
	if isTaxDocument(m) && m.TaxableFulfillmentDue == "" {
		return invalid("taxable_fulfillment_due", "the date of the taxable supply (DUZP) is required on a tax document")
	}
	if m.DocumentType == model.DocCorrection && m.YourVatMode == model.VatModePayer && m.CorrectionReason == "" {
		return invalid("correction_reason", "the reason of the correction is required on a corrective tax document (§ 45)")
	}
	return nil
}

// taxNumberTypes share one evidence-number space: tax documents must be
// identified uniquely by their number (§ 29 odst. 1 písm. e ZDPH).
var taxNumberTypes = []string{model.DocInvoice, model.DocCorrection, model.DocTaxDocument}

// checkTaxNumberUnique answers 409 when another invoice, correction or tax
// document of the account already has m's number.
func checkTaxNumberUnique(ctx context.Context, tx *gorm.DB, m *model.Invoice) error {
	if !slices.Contains(taxNumberTypes, m.DocumentType) {
		return nil
	}
	var n int64
	if err := tx.Model(&model.Invoice{}).Scopes(inAccount(ctx)).
		Where("document_type IN ? AND number = ? AND id <> ?", taxNumberTypes, m.Number, m.ID).Count(&n).Error; err != nil {
		return dbErr(err, "invoice")
	}
	if n > 0 {
		return conflict(CodeAlreadyExists, "document number "+m.Number+" already exists")
	}
	return nil
}

// needsCNBRate: a document in currency needs an exchange rate (the ČNB
// rate by default) — a foreign currency of an account keeping CZK.
func needsCNBRate(acc *model.Account, currency string) bool {
	return currency != "" && currency != acc.DefaultCurrency && acc.DefaultCurrency == "CZK"
}

func snapshotClient(m *model.Invoice, s *model.Subject) {
	m.ClientName, m.ClientFullName = s.Name, s.FullName
	m.ClientRegistrationNo, m.ClientVatNo = s.RegistrationNo, s.VatNo
	m.ClientStreet, m.ClientCity, m.ClientZip, m.ClientCountry = s.Street, s.City, s.Zip, s.Country
	m.ClientEmail, m.ClientLocalVatNo = s.Email, s.LocalVatNo
}

func snapshotYour(m *model.Invoice, a *model.Account) {
	m.YourName, m.YourRegistrationNo, m.YourVatNo = a.Name, a.RegistrationNo, a.VatNo
	m.YourStreet, m.YourCity, m.YourZip, m.YourCountry = a.Street, a.City, a.Zip, a.Country
	m.YourRegisteredBy, m.YourVatMode = a.RegisteredBy, a.VatMode
}

// snapshotBank copies the bank account id (or the default bank account of the
// invoice currency when id is nil) into the invoice; no bank account of the
// currency clears the bank fields.
func snapshotBank(ctx context.Context, tx *gorm.DB, m *model.Invoice, id *uint) error {
	var ba model.BankAccount
	q := tx.Scopes(inAccount(ctx))
	var err error
	if id != nil {
		err = q.First(&ba, *id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid("bank_account_id", "bank account not found")
		}
	} else {
		res := q.Where("currency = ?", m.Currency).Order("is_default DESC, id").Limit(1).Find(&ba)
		if err = res.Error; err == nil && res.RowsAffected == 0 {
			m.BankAccountID, m.BankAccount, m.IBAN, m.SwiftBIC = nil, "", "", ""
			return nil
		}
	}
	if err != nil {
		return dbErr(err, "bank account")
	}
	m.BankAccountID, m.BankAccount, m.IBAN, m.SwiftBIC = &ba.ID, ba.Number, ba.IBAN, ba.SwiftBIC
	return nil
}

// buildLines converts input lines; lines with id must be among existing
// (they keep their id so they are updated).
func buildLines(in []InvoiceLineInput, existing []model.InvoiceLine, defaultRate int32) ([]model.InvoiceLine, error) {
	known := make(map[uint]bool, len(existing))
	for _, l := range existing {
		known[l.ID] = true
	}
	out := make([]model.InvoiceLine, len(in))
	for i, l := range in {
		field := fmt.Sprintf("lines[%d]", i)
		ml := model.InvoiceLine{PriceItemID: l.PriceItemID, UnitName: strings.TrimSpace(l.UnitName), UnitPrice: l.UnitPrice, VatRateBps: defaultRate}
		if l.ID != nil {
			if !known[*l.ID] {
				return nil, invalid(field+".id", "line not found on this invoice")
			}
			delete(known, *l.ID) // a line id may appear only once
			ml.ID = *l.ID
		}
		if ml.Name = strings.TrimSpace(l.Name); ml.Name == "" {
			return nil, invalid(field+".name", "name must not be empty")
		}
		ml.QuantityMilli = 1000
		if l.Quantity != "" {
			q, err := billing.ParseQuantity(l.Quantity)
			if err != nil {
				return nil, invalid(field+".quantity", err.Error())
			}
			ml.QuantityMilli = q
		}
		apply(&ml.VatRateBps, l.VatRateBps)
		out[i] = ml
	}
	return out, nil
}

func billingLines(lines []model.InvoiceLine) []billing.Line {
	out := make([]billing.Line, len(lines))
	for i, l := range lines {
		out[i] = billing.Line{QuantityMilli: l.QuantityMilli, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps}
	}
	return out
}

func billingOptions(m *model.Invoice) billing.Options {
	return billing.Options{
		PricesIncludeVAT: m.PricesIncludeVat, ReverseCharge: m.ReverseCharge,
		RoundTotal: m.RoundTotal, NonVATPayer: billing.ChargesNoVAT(m.YourVatMode, m.ReverseCharge),
	}
}

// recalc validates dates and recomputes positions, forced VAT rates, due_on
// and all line and document totals.
func recalc(m *model.Invoice) error {
	if m.TaxableFulfillmentDue != "" && !billing.ValidDate(m.TaxableFulfillmentDue) {
		return invalid("taxable_fulfillment_due", "invalid date")
	}
	if _, err := billing.ParseRate(m.ExchangeRate); err != nil {
		return invalid("exchange_rate", "the exchange rate must be a positive number")
	}
	if m.SupplyType != "" && m.SupplyType != model.SupplyServices && m.SupplyType != model.SupplyGoods {
		return invalid("supply_type", "supply_type must be services or goods")
	}
	due, err := billing.DueOn(m.IssuedOn, m.DueDays)
	if err != nil {
		return invalid("issued_on", "invalid date")
	}
	m.DueOn = due
	opts := billingOptions(m)
	since2024 := defaultStr(m.TaxableFulfillmentDue, m.IssuedOn) >= "2024-01-01"
	for i := range m.Lines {
		m.Lines[i].Position = i + 1
		m.Lines[i].VatRateBps = billing.EffectiveRate(m.Lines[i].VatRateBps, opts)
		if r := m.Lines[i].VatRateBps; m.YourVatMode == model.VatModePayer && since2024 && r != 0 && r != 1200 && r != 2100 {
			// statutory rates since 1. 1. 2024 (older documents may keep 10/15 %)
			return invalid(fmt.Sprintf("lines[%d].vat_rate_bps", i), "VAT rates since 2024 are 21 %, 12 % and 0 %")
		}
	}
	t, err := billing.Calculate(billingLines(m.Lines), opts)
	if err != nil {
		return invalid("lines", "amounts are out of range")
	}
	for i, la := range t.Lines {
		m.Lines[i].Base, m.Lines[i].Vat, m.Lines[i].Total = la.Base, la.Vat, la.Total
	}
	m.Subtotal, m.VatTotal, m.Rounding, m.Total = t.Subtotal, t.VatTotal, t.Rounding, t.Total
	return nil
}

// applyPayments recomputes paid_amount, status and paid_on from m.Payments.
func applyPayments(m *model.Invoice) {
	m.PaidAmount = 0
	last := ""
	for _, p := range m.Payments {
		m.PaidAmount += p.Amount
		if p.PaidOn > last {
			last = p.PaidOn
		}
	}
	m.Status = billing.PaymentStatus(m.Status, m.Total, m.PaidAmount, len(m.Payments), m.SentAt != nil)
	switch m.Status {
	case model.StatusPaid:
		m.PaidOn = last
	case model.StatusOpen, model.StatusSent:
		m.PaidOn = ""
	}
}

// saveLines stores m.Lines: removed lines are deleted, lines with id updated,
// new lines inserted.
func saveLines(tx *gorm.DB, m *model.Invoice) error {
	keep := []uint{}
	for _, l := range m.Lines {
		if l.ID != 0 {
			keep = append(keep, l.ID)
		}
	}
	del := tx.Where("invoice_id = ?", m.ID)
	if len(keep) > 0 {
		del = del.Where("id NOT IN ?", keep)
	}
	if err := del.Delete(&model.InvoiceLine{}).Error; err != nil {
		return dbErr(err, "invoice line")
	}
	for i := range m.Lines {
		m.Lines[i].InvoiceID = m.ID
		if err := tx.Save(&m.Lines[i]).Error; err != nil {
			return dbErr(err, "invoice line")
		}
	}
	return nil
}

func normalizeTags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func newPublicToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)                            // never fails (crypto/rand)
	return base64.RawURLEncoding.EncodeToString(b) // 32 chars
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// numberErr maps a unique violation on (account, type, number) to 409.
func numberErr(err error, number string) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return conflict(CodeAlreadyExists, "document number "+number+" already exists")
	}
	return dbErr(err, "invoice")
}

// dbErrOrNil is dbErr that passes nil through.
func dbErrOrNil(err error, what string) error {
	if err == nil {
		return nil
	}
	return dbErr(err, what)
}

// numberingErr maps errors of Deps.NextNumber: no number format → 409,
// invalid date → 422, huma errors pass through, anything else → 500.
func numberingErr(err error) error {
	var se huma.StatusError
	switch {
	case errors.Is(err, numbering.ErrNoFormat):
		return conflict(CodeNoNumberFormat, err.Error()+"; create a number format first")
	case errors.Is(err, numbering.ErrInvalidDate):
		return invalid("issued_on", err.Error())
	case errors.As(err, &se):
		return err
	default:
		return huma.Error500InternalServerError("cannot assign a document number", err)
	}
}
