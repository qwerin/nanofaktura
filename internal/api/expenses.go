package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// Expenses (SPEC §7.2): received documents with lines computed exactly like
// invoice lines (billing.Calculate), their own payments (ExpensePayment) and
// stored status open|paid (overdue derived on read).

// ---- output DTOs ----

// ExpenseSummary is an expense without lines, payments and VAT recap (list items).
type ExpenseSummary struct {
	ID             uint   `json:"id"`
	Number         string `json:"number" doc:"Internal number (number format of document type expense)"`
	OriginalNumber string `json:"original_number" doc:"The supplier's document number"`
	VariableSymbol string `json:"variable_symbol"`
	Status         string `json:"status" enum:"open,overdue,paid" doc:"Stored status, or overdue when open and due_on < today"`
	SubjectID      *uint  `json:"subject_id,omitempty" doc:"The supplier"`

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
	LockedAt              *time.Time `json:"locked_at,omitempty"`

	Currency         string   `json:"currency"`
	ExchangeRate     string   `json:"exchange_rate"`
	PaymentMethod    string   `json:"payment_method"`
	Category         string   `json:"category"`
	Description      string   `json:"description"`
	PrivateNote      string   `json:"private_note"`
	Tags             []string `json:"tags" nullable:"false"`
	TaxDeductible    bool     `json:"tax_deductible" doc:"Income tax: counts as a tax-deductible expense"`
	VatDeductible    bool     `json:"vat_deductible" doc:"VAT: the VAT deduction is claimed in the VAT return (independent of tax_deductible)"`
	PricesIncludeVat bool     `json:"prices_include_vat"`
	RoundTotal       bool     `json:"round_total"`
	ReverseCharge    bool     `json:"reverse_charge" doc:"The supplier charged no VAT and the recipient self-assesses it (EU, § 92a, services from outside the EU); line rates are the recipient's"`
	SupplyType       string   `json:"supply_type" enum:"services,goods" doc:"Reverse charge from the EU: services or goods"`

	Subtotal        int64 `json:"subtotal"`
	VatTotal        int64 `json:"vat_total"`
	Rounding        int64 `json:"rounding"`
	Total           int64 `json:"total"`
	PaidAmount      int64 `json:"paid_amount"`
	RemainingAmount int64 `json:"remaining_amount"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Expense is the expense detail.
type Expense struct {
	ExpenseSummary
	Lines    []ExpenseLine    `json:"lines" nullable:"false"`
	Payments []ExpensePayment `json:"payments" nullable:"false"`
	VatRecap []VatRecapItem   `json:"vat_recap" nullable:"false"`
	// Warnings from the VAT payer registry (unreliable supplier, unpublished
	// bank account); only on GET /expenses/{id}, omitted when there are none.
	Warnings    []string     `json:"warnings,omitempty"`
	Attachments []Attachment `json:"attachments" nullable:"false"`
}

type ExpenseLine struct {
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

type ExpensePayment struct {
	ID        uint      `json:"id"`
	ExpenseID uint      `json:"expense_id"`
	PaidOn    string    `json:"paid_on"`
	Amount    int64     `json:"amount"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

func toExpensePayment(p *model.ExpensePayment) ExpensePayment {
	return ExpensePayment{ID: p.ID, ExpenseID: p.ExpenseID, PaidOn: p.PaidOn, Amount: p.Amount, Note: p.Note, CreatedAt: p.CreatedAt}
}

func toExpenseSummary(m *model.Expense, today string) ExpenseSummary {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	return ExpenseSummary{
		ID: m.ID, Number: m.Number, OriginalNumber: m.OriginalNumber, VariableSymbol: m.VariableSymbol,
		Status: billing.EffectiveStatus(m.Status, m.DueOn, today), SubjectID: m.SubjectID,

		SupplierName: m.SupplierName, SupplierFullName: m.SupplierFullName, SupplierRegistrationNo: m.SupplierRegistrationNo,
		SupplierVatNo: m.SupplierVatNo, SupplierStreet: m.SupplierStreet, SupplierCity: m.SupplierCity,
		SupplierZip: m.SupplierZip, SupplierCountry: m.SupplierCountry, SupplierBankAccount: m.SupplierBankAccount,
		SupplierIBAN: m.SupplierIBAN, SupplierSwiftBIC: m.SupplierSwiftBIC,

		IssuedOn: m.IssuedOn, TaxableFulfillmentDue: m.TaxableFulfillmentDue, DueOn: m.DueOn, PaidOn: m.PaidOn, LockedAt: m.LockedAt,

		Currency: m.Currency, ExchangeRate: m.ExchangeRate, PaymentMethod: m.PaymentMethod, Category: m.Category,
		Description: m.Description, PrivateNote: m.PrivateNote, Tags: tags, TaxDeductible: m.TaxDeductible,
		VatDeductible: m.VatDeductible, PricesIncludeVat: m.PricesIncludeVat, RoundTotal: m.RoundTotal,
		ReverseCharge: m.ReverseCharge, SupplyType: defaultStr(m.SupplyType, model.SupplyServices),

		Subtotal: m.Subtotal, VatTotal: m.VatTotal, Rounding: m.Rounding, Total: m.Total,
		PaidAmount: m.PaidAmount, RemainingAmount: m.Total - m.PaidAmount,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// toExpense converts an expense loaded with Lines and Payments.
func toExpense(m *model.Expense, today string) Expense {
	out := Expense{
		ExpenseSummary: toExpenseSummary(m, today),
		Lines:          make([]ExpenseLine, len(m.Lines)),
		Payments:       make([]ExpensePayment, len(m.Payments)),
		VatRecap:       []VatRecapItem{},
	}
	for i, l := range m.Lines {
		out.Lines[i] = ExpenseLine{
			ID: l.ID, Position: l.Position, PriceItemID: l.PriceItemID, Name: l.Name,
			Quantity: billing.FormatQuantity(l.QuantityMilli), UnitName: l.UnitName, UnitPrice: l.UnitPrice,
			VatRateBps: l.VatRateBps, Base: l.Base, Vat: l.Vat, Total: l.Total,
		}
	}
	for i := range m.Payments {
		out.Payments[i] = toExpensePayment(&m.Payments[i])
	}
	if t, err := billing.Calculate(expenseBillingLines(m.Lines), expenseOptions(m)); err == nil {
		for _, r := range t.VatRecap {
			out.VatRecap = append(out.VatRecap, VatRecapItem{VatRateBps: r.VatRateBps, Base: r.Base, Vat: r.Vat, Total: r.Total})
		}
	}
	return out
}

// ---- input DTOs ----

// ExpenseSupplierFields override the supplier_* snapshot taken from the subject.
type ExpenseSupplierFields struct {
	SupplierName           *string `json:"supplier_name,omitempty" maxLength:"200"`
	SupplierFullName       *string `json:"supplier_full_name,omitempty" maxLength:"200"`
	SupplierRegistrationNo *string `json:"supplier_registration_no,omitempty" maxLength:"20"`
	SupplierVatNo          *string `json:"supplier_vat_no,omitempty" maxLength:"20"`
	SupplierStreet         *string `json:"supplier_street,omitempty" maxLength:"200"`
	SupplierCity           *string `json:"supplier_city,omitempty" maxLength:"100"`
	SupplierZip            *string `json:"supplier_zip,omitempty" maxLength:"20"`
	SupplierCountry        *string `json:"supplier_country,omitempty" maxLength:"2"`
	SupplierBankAccount    *string `json:"supplier_bank_account,omitempty" maxLength:"50"`
	SupplierIBAN           *string `json:"supplier_iban,omitempty" maxLength:"42"`
	SupplierSwiftBIC       *string `json:"supplier_swift_bic,omitempty" maxLength:"11"`
}

func (f *ExpenseSupplierFields) applyTo(m *model.Expense) {
	apply(&m.SupplierName, f.SupplierName)
	apply(&m.SupplierFullName, f.SupplierFullName)
	apply(&m.SupplierRegistrationNo, f.SupplierRegistrationNo)
	apply(&m.SupplierVatNo, f.SupplierVatNo)
	apply(&m.SupplierStreet, f.SupplierStreet)
	apply(&m.SupplierCity, f.SupplierCity)
	apply(&m.SupplierZip, f.SupplierZip)
	apply(&m.SupplierCountry, f.SupplierCountry)
	apply(&m.SupplierBankAccount, f.SupplierBankAccount)
	apply(&m.SupplierIBAN, f.SupplierIBAN)
	apply(&m.SupplierSwiftBIC, f.SupplierSwiftBIC)
}

// ExpenseCreate: lines use the same input as invoice lines.
type ExpenseCreate struct {
	Number         string  `json:"number,omitempty" maxLength:"50" doc:"Custom internal number; default: next number of the default expense number format"`
	OriginalNumber string  `json:"original_number,omitempty" maxLength:"100"`
	VariableSymbol *string `json:"variable_symbol,omitempty" pattern:"^[0-9]{0,10}$" doc:"Default: digits of original_number (last 10)"`
	SubjectID      *uint   `json:"subject_id,omitempty" doc:"Supplier; snapshots supplier_*. Without it supplier_name is required"`
	ExpenseSupplierFields

	IssuedOn              string  `json:"issued_on,omitempty" format:"date" doc:"Default today"`
	TaxableFulfillmentDue *string `json:"taxable_fulfillment_due,omitempty" doc:"YYYY-MM-DD or empty; default issued_on"`
	DueOn                 string  `json:"due_on,omitempty" format:"date" doc:"Default: issued_on + due_days"`
	DueDays               *int    `json:"due_days,omitempty" minimum:"0" maximum:"365" doc:"Used when due_on is empty; default: the supplier's due_days, else the account's default_due_days"`

	Currency         string   `json:"currency,omitempty" pattern:"^[A-Z]{3}$" doc:"Default: account default_currency"`
	ExchangeRate     string   `json:"exchange_rate,omitempty" pattern:"^[0-9]{1,6}([.][0-9]{1,6})?$" doc:"CZK per unit, > 0; default: ČNB rate of the DUZP for a foreign currency, 1 for CZK"`
	PaymentMethod    string   `json:"payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom" doc:"Default: account default_payment_method"`
	Category         string   `json:"category,omitempty" maxLength:"100"`
	Description      string   `json:"description,omitempty" maxLength:"5000"`
	PrivateNote      string   `json:"private_note,omitempty" maxLength:"5000"`
	Tags             []string `json:"tags,omitempty" maxItems:"50"`
	TaxDeductible    *bool    `json:"tax_deductible,omitempty" doc:"Default true"`
	VatDeductible    *bool    `json:"vat_deductible,omitempty" doc:"Default true"`
	PricesIncludeVat bool     `json:"prices_include_vat,omitempty"`
	RoundTotal       bool     `json:"round_total,omitempty"`
	ReverseCharge    bool     `json:"reverse_charge,omitempty"`
	SupplyType       string   `json:"supply_type,omitempty" enum:"services,goods"`

	Lines []InvoiceLineInput `json:"lines" minItems:"1" maxItems:"500"`
}

// ExpensePatch: nil fields are left unchanged; lines (when present) replace
// the whole list (same rules as invoice lines).
type ExpensePatch struct {
	Number         *string `json:"number,omitempty" minLength:"1" maxLength:"50"`
	OriginalNumber *string `json:"original_number,omitempty" maxLength:"100"`
	VariableSymbol *string `json:"variable_symbol,omitempty" pattern:"^[0-9]{0,10}$"`
	SubjectID      *uint   `json:"subject_id,omitempty" minimum:"1" doc:"Changing the supplier re-snapshots supplier_* (unless sent explicitly)"`
	ClearSubject   bool    `json:"clear_subject,omitempty" doc:"Unlink the supplier contact (subject_id → null); supplier_* stay as free text"`
	ExpenseSupplierFields

	IssuedOn              *string `json:"issued_on,omitempty" format:"date"`
	TaxableFulfillmentDue *string `json:"taxable_fulfillment_due,omitempty"`
	DueOn                 *string `json:"due_on,omitempty" format:"date"`

	Currency         *string  `json:"currency,omitempty" pattern:"^[A-Z]{3}$"`
	ExchangeRate     *string  `json:"exchange_rate,omitempty" pattern:"^[0-9]{1,6}([.][0-9]{1,6})?$"`
	PaymentMethod    *string  `json:"payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom"`
	Category         *string  `json:"category,omitempty" maxLength:"100"`
	Description      *string  `json:"description,omitempty" maxLength:"5000"`
	PrivateNote      *string  `json:"private_note,omitempty" maxLength:"5000"`
	Tags             []string `json:"tags,omitempty" maxItems:"50" doc:"Replaces all tags; [] clears"`
	TaxDeductible    *bool    `json:"tax_deductible,omitempty"`
	VatDeductible    *bool    `json:"vat_deductible,omitempty"`
	PricesIncludeVat *bool    `json:"prices_include_vat,omitempty"`
	RoundTotal       *bool    `json:"round_total,omitempty"`
	ReverseCharge    *bool    `json:"reverse_charge,omitempty"`
	SupplyType       *string  `json:"supply_type,omitempty" enum:"services,goods"`

	Lines []InvoiceLineInput `json:"lines,omitempty" minItems:"1" maxItems:"500"`
}

type ExpensePaymentCreate struct {
	PaidOn string `json:"paid_on,omitempty" format:"date" doc:"Default today"`
	Amount *int64 `json:"amount,omitempty" doc:"Minor units, non-zero; default: remaining_amount"`
	Note   string `json:"note,omitempty" maxLength:"500"`
}

// ExpensePaymentResult is the created payment and the updated expense.
type ExpensePaymentResult struct {
	Payment ExpensePayment `json:"payment"`
	Expense Expense        `json:"expense"`
}

// ExpenseCategories are the distinct categories used by the account's expenses.
type ExpenseCategories struct {
	Items []string `json:"items" nullable:"false"`
}

// ---- routes ----

func (s *server) registerExpenses(g huma.API) {
	huma.Get(g, "/expenses", s.listExpenses)
	huma.Post(g, "/expenses", s.createExpense, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/expenses/categories", s.expenseCategories)
	huma.Get(g, "/expenses/{id}", s.getExpense)
	huma.Patch(g, "/expenses/{id}", s.patchExpense, auth.ForEditors)
	huma.Delete(g, "/expenses/{id}", s.deleteExpense, status(http.StatusNoContent), auth.ForEditors)
	huma.Post(g, "/expenses/{id}/actions/{action}", s.expenseAction, auth.ForEditors)
	huma.Post(g, "/expenses/{id}/payments", s.createExpensePayment, status(http.StatusCreated), auth.ForEditors)
	huma.Delete(g, "/expenses/{id}/payments/{payment_id}", s.deleteExpensePayment, status(http.StatusNoContent), auth.ForEditors)
}

type expenseID struct {
	ID uint `path:"id"`
}

func (s *server) listExpenses(ctx context.Context, in *struct {
	PageParams
	ExpenseFilter
}) (*Out[DocumentList[ExpenseSummary]], error) {
	today := s.today()
	page, err := paginate(in.ExpenseFilter.query(s.scoped(ctx), today), in.PageParams, func(m *model.Expense) ExpenseSummary {
		return toExpenseSummary(m, today)
	})
	if err != nil {
		return nil, err
	}
	sums, err := currencySums(in.ExpenseFilter.where(s.scoped(ctx), today))
	if err != nil {
		return nil, err
	}
	return &Out[DocumentList[ExpenseSummary]]{Body: DocumentList[ExpenseSummary]{ListResponse: page.Body, Sums: sums}}, nil
}

func (s *server) expenseCategories(ctx context.Context, in *struct {
	Query string `query:"query" doc:"Case-insensitive substring"`
}) (*Out[ExpenseCategories], error) {
	q := s.scoped(ctx).Model(&model.Expense{}).Where("category <> ''")
	if qs := strings.TrimSpace(in.Query); qs != "" {
		q = q.Where(`LOWER(category) LIKE ? ESCAPE '\'`, likePattern(qs))
	}
	out := ExpenseCategories{Items: []string{}}
	if err := q.Distinct("category").Order("category").Limit(200).Pluck("category", &out.Items).Error; err != nil {
		return nil, dbErr(err, "categories")
	}
	return &Out[ExpenseCategories]{Body: out}, nil
}

// loadExpense loads an expense of the current account with ordered lines and payments.
func loadExpense(ctx context.Context, db *gorm.DB, id uint) (*model.Expense, error) {
	var m model.Expense
	err := db.Scopes(inAccount(ctx)).
		Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("position, id") }).
		Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("paid_on, id") }).
		First(&m, id).Error
	if err != nil {
		return nil, dbErr(err, "expense")
	}
	return &m, nil
}

// loadExpenseForUpdate is loadInvoiceForUpdate for expenses: it locks the
// expense row first; use it in every transaction that changes the expense's
// money or status.
func loadExpenseForUpdate(ctx context.Context, tx *gorm.DB, id uint) (*model.Expense, error) {
	var row model.Expense
	if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Scopes(inAccount(ctx)).
		Select("id").First(&row, id).Error; err != nil {
		return nil, dbErr(err, "expense")
	}
	return loadExpense(ctx, tx, id)
}

func (s *server) getExpense(ctx context.Context, in *expenseID) (*Out[Expense], error) {
	m, err := loadExpense(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	out := toExpense(m, s.today())
	out.Warnings = s.expenseWarnings(ctx, m)
	if out.Attachments, err = ownerAttachments(ctx, s.db.WithContext(ctx), model.OwnerExpense, m.ID); err != nil {
		return nil, err
	}
	return &Out[Expense]{Body: out}, nil
}

// mutateExpense runs fn in a transaction and returns the expense whose id fn
// returned, reloaded inside the same transaction.
func (s *server) mutateExpense(ctx context.Context, fn func(tx *gorm.DB) (uint, error)) (*Out[Expense], error) {
	var out *Out[Expense]
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, err := fn(tx)
		if err != nil {
			return err
		}
		m, err := loadExpense(ctx, tx, id)
		if err != nil {
			return err
		}
		e := toExpense(m, s.today())
		if e.Attachments, err = ownerAttachments(ctx, tx, model.OwnerExpense, m.ID); err != nil {
			return err
		}
		out = &Out[Expense]{Body: e}
		return nil
	})
	return out, err
}

func (s *server) createExpense(ctx context.Context, in *struct{ Body ExpenseCreate }) (*Out[Expense], error) {
	b := &in.Body
	acc := auth.AccountFrom(ctx)
	var err error
	if b.ExchangeRate, err = s.defaultExchangeRate(ctx, b.Currency, b.ExchangeRate, strOrEmpty(b.TaxableFulfillmentDue), b.IssuedOn); err != nil {
		return nil, err
	}
	return s.mutateExpense(ctx, func(tx *gorm.DB) (uint, error) {
		m := &model.Expense{
			AccountID: acc.ID, Status: model.StatusOpen, OriginalNumber: strings.TrimSpace(b.OriginalNumber),
			Currency: defaultStr(b.Currency, acc.DefaultCurrency), ExchangeRate: defaultStr(b.ExchangeRate, "1"),
			PaymentMethod: defaultStr(b.PaymentMethod, acc.DefaultPaymentMethod),
			Category:      strings.TrimSpace(b.Category), Description: b.Description, PrivateNote: b.PrivateNote,
			Tags: normalizeTags(b.Tags), TaxDeductible: true, VatDeductible: true, PricesIncludeVat: b.PricesIncludeVat,
			RoundTotal: b.RoundTotal, ReverseCharge: b.ReverseCharge, SupplyType: b.SupplyType,
		}
		apply(&m.TaxDeductible, b.TaxDeductible)
		apply(&m.VatDeductible, b.VatDeductible)
		if b.SubjectID != nil {
			subj, err := findSubject(ctx, tx, *b.SubjectID)
			if err != nil {
				return 0, err
			}
			m.SubjectID = &subj.ID
			snapshotSupplier(m, subj)
		}
		b.ExpenseSupplierFields.applyTo(m)
		if m.SupplierName = strings.TrimSpace(m.SupplierName); m.SupplierName == "" {
			return 0, invalid("supplier_name", "subject_id or supplier_name is required")
		}

		m.IssuedOn = defaultStr(b.IssuedOn, s.today())
		if !billing.ValidDate(m.IssuedOn) {
			return 0, invalid("issued_on", "invalid date")
		}
		m.TaxableFulfillmentDue = m.IssuedOn
		apply(&m.TaxableFulfillmentDue, b.TaxableFulfillmentDue)
		m.DueOn = b.DueOn
		if m.DueOn == "" {
			days := acc.DefaultDueDays
			if b.DueDays != nil {
				days = *b.DueDays
			}
			m.DueOn, _ = billing.DueOn(m.IssuedOn, days)
		}
		if len(b.Lines) == 0 {
			return 0, invalid("lines", "at least one line is required")
		}
		if err := checkPriceItems(ctx, tx, b.Lines); err != nil {
			return 0, err
		}
		var err error
		if m.Lines, err = buildExpenseLines(b.Lines, nil, acc.DefaultVatRateBps); err != nil {
			return 0, err
		}
		if err := recalcExpense(m); err != nil {
			return 0, err
		}

		if n := strings.TrimSpace(b.Number); n != "" {
			m.Number = n
		} else if m.Number, err = s.deps.NextNumber(tx, acc.ID, model.DocExpense, m.IssuedOn); err != nil {
			return 0, numberingErr(err)
		}
		m.VariableSymbol = spayd.Digits(m.OriginalNumber, 10)
		apply(&m.VariableSymbol, b.VariableSymbol)

		if err := tx.Create(m).Error; err != nil {
			return 0, expenseNumberErr(err, m.Number)
		}
		if err := syncExpenseStock(ctx, tx, m); err != nil {
			return 0, err
		}
		return m.ID, recordExpense(ctx, tx, events.ExpenseCreated, m)
	})
}

func (s *server) patchExpense(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body ExpensePatch
}) (*Out[Expense], error) {
	p := &in.Body
	return s.mutateExpense(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := loadExpenseForUpdate(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		if m.LockedAt != nil {
			return 0, conflict(CodeLocked, "the expense is locked; unlock it first")
		}
		if p.ClearSubject {
			if p.SubjectID != nil {
				return 0, invalid("clear_subject", "clear_subject and subject_id cannot be combined")
			}
			m.SubjectID = nil
		}
		if p.SubjectID != nil && (m.SubjectID == nil || *p.SubjectID != *m.SubjectID) {
			subj, err := findSubject(ctx, tx, *p.SubjectID)
			if err != nil {
				return 0, err
			}
			m.SubjectID = &subj.ID
			snapshotSupplier(m, subj)
		}
		p.ExpenseSupplierFields.applyTo(m)
		if p.Number != nil {
			if m.Number = strings.TrimSpace(*p.Number); m.Number == "" {
				return 0, invalid("number", "number must not be empty")
			}
		}
		if p.OriginalNumber != nil {
			m.OriginalNumber = strings.TrimSpace(*p.OriginalNumber)
		}
		apply(&m.VariableSymbol, p.VariableSymbol)
		apply(&m.IssuedOn, p.IssuedOn)
		if !billing.ValidDate(m.IssuedOn) {
			return 0, invalid("issued_on", "invalid date")
		}
		apply(&m.TaxableFulfillmentDue, p.TaxableFulfillmentDue)
		apply(&m.DueOn, p.DueOn)
		if p.Currency != nil && *p.Currency != m.Currency && len(m.Payments) > 0 {
			return 0, conflict(CodeCurrencyHasPayments, "the expense has payments in "+m.Currency+"; delete them before changing the currency")
		}
		apply(&m.Currency, p.Currency)
		apply(&m.ExchangeRate, p.ExchangeRate)
		apply(&m.PaymentMethod, p.PaymentMethod)
		if p.Category != nil {
			m.Category = strings.TrimSpace(*p.Category)
		}
		apply(&m.Description, p.Description)
		apply(&m.PrivateNote, p.PrivateNote)
		if p.Tags != nil {
			m.Tags = normalizeTags(p.Tags)
		}
		apply(&m.TaxDeductible, p.TaxDeductible)
		apply(&m.VatDeductible, p.VatDeductible)
		apply(&m.PricesIncludeVat, p.PricesIncludeVat)
		apply(&m.RoundTotal, p.RoundTotal)
		apply(&m.ReverseCharge, p.ReverseCharge)
		apply(&m.SupplyType, p.SupplyType)
		if p.Lines != nil {
			if err := checkPriceItems(ctx, tx, p.Lines); err != nil {
				return 0, err
			}
			if m.Lines, err = buildExpenseLines(p.Lines, m.Lines, auth.AccountFrom(ctx).DefaultVatRateBps); err != nil {
				return 0, err
			}
		}
		if err := recalcExpense(m); err != nil {
			return 0, err
		}
		applyExpensePayments(m)

		if err := tx.Omit(clause.Associations).Save(m).Error; err != nil {
			return 0, expenseNumberErr(err, m.Number)
		}
		if err := saveExpenseLines(tx, m); err != nil {
			return 0, err
		}
		if err := syncExpenseStock(ctx, tx, m); err != nil {
			return 0, err
		}
		return m.ID, recordExpense(ctx, tx, events.ExpenseUpdated, m)
	})
}

func (s *server) deleteExpense(ctx context.Context, in *expenseID) (*NoContent, error) {
	var files []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadExpenseForUpdate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if m.LockedAt != nil {
			return conflict(CodeLocked, "the expense is locked; unlock it first")
		}
		if len(m.Payments) > 0 {
			return conflict(CodeHasPayments, "the expense has payments; delete them first")
		}
		if files, err = deleteOwnerAttachments(ctx, tx, model.OwnerExpense, m.ID); err != nil {
			return err
		}
		if err := rewriteDocStock(ctx, tx, docStock{"expense_id", m.ID}, false, "", "", nil); err != nil {
			return err
		}
		if err := tx.Where("expense_id = ?", m.ID).Delete(&model.ExpenseLine{}).Error; err != nil {
			return dbErr(err, "expense")
		}
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "expense")
		}
		return recordExpense(ctx, tx, events.ExpenseDeleted, m)
	})
	if err != nil {
		return nil, err
	}
	s.removeFiles(ctx, files)
	return &NoContent{}, nil
}

func (s *server) expenseAction(ctx context.Context, in *struct {
	ID     uint   `path:"id"`
	Action string `path:"action" enum:"lock,unlock"`
}) (*Out[Expense], error) {
	return s.mutateExpense(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := loadExpenseForUpdate(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		var locked *time.Time
		if in.Action == billing.ActionLock {
			now := s.deps.Now()
			locked = &now
		}
		if err := tx.Model(m).Update("locked_at", locked).Error; err != nil {
			return 0, dbErr(err, "expense")
		}
		name := events.ExpenseUnlocked
		if locked != nil {
			name = events.ExpenseLocked
		}
		return m.ID, recordExpense(ctx, tx, name, m)
	})
}

func (s *server) createExpensePayment(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body ExpensePaymentCreate
}) (*Out[ExpensePaymentResult], error) {
	var res ExpensePaymentResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadExpenseForUpdate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		paidOn := defaultStr(in.Body.PaidOn, s.today())
		if !billing.ValidDate(paidOn) {
			return invalid("paid_on", "invalid date")
		}
		amount := m.Total - m.PaidAmount
		if in.Body.Amount != nil {
			if amount = *in.Body.Amount; amount == 0 {
				return invalid("amount", "amount must not be zero")
			}
		} else if amount == 0 {
			return conflict(CodeNothingToPay, "nothing to pay: the remaining amount is 0")
		}
		p, err := addExpensePayment(ctx, tx, m, paidOn, amount, in.Body.Note)
		if err != nil {
			return err
		}
		if m, err = loadExpense(ctx, tx, m.ID); err != nil {
			return err
		}
		res = ExpensePaymentResult{Payment: toExpensePayment(p), Expense: toExpense(m, s.today())}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[ExpensePaymentResult]{Body: res}, nil
}

func (s *server) deleteExpensePayment(ctx context.Context, in *struct {
	ID        uint `path:"id"`
	PaymentID uint `path:"payment_id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockBankTxsOfPayment(ctx, tx, in.PaymentID); err != nil {
			return err
		}
		m, err := loadExpenseForUpdate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		idx := -1
		for i, p := range m.Payments {
			if p.ID == in.PaymentID {
				idx = i
			}
		}
		if idx < 0 {
			return notFound("payment")
		}
		if err := tx.Delete(&model.ExpensePayment{}, in.PaymentID).Error; err != nil {
			return dbErr(err, "payment")
		}
		if err := unlinkBankPayment(tx, "matched_expense_id", m.ID, in.PaymentID); err != nil {
			return err
		}
		p := m.Payments[idx]
		if err := refreshExpensePayments(tx, m); err != nil {
			return err
		}
		return recordExpensePayment(ctx, tx, events.ExpensePaymentDeleted, m, &p, model.StatusPaid)
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// ---- helpers ----

// addExpensePayment stores a payment of m, recomputes m's paid amount and
// status and records expense_payment.created (+ expense.paid).
func addExpensePayment(ctx context.Context, tx *gorm.DB, m *model.Expense, paidOn string, amount int64, note string) (*model.ExpensePayment, error) {
	p := model.ExpensePayment{AccountID: m.AccountID, ExpenseID: m.ID, PaidOn: paidOn, Amount: amount, Note: note}
	if err := tx.Create(&p).Error; err != nil {
		return nil, dbErr(err, "payment")
	}
	prev := m.Status
	if err := refreshExpensePayments(tx, m); err != nil {
		return nil, err
	}
	return &p, recordExpensePayment(ctx, tx, events.ExpensePaymentCreated, m, &p, prev)
}

// refreshExpensePayments is refreshInvoicePayments for expenses.
func refreshExpensePayments(tx *gorm.DB, m *model.Expense) error {
	if err := tx.Where("expense_id = ?", m.ID).Order("paid_on, id").Find(&m.Payments).Error; err != nil {
		return dbErr(err, "payment")
	}
	applyExpensePayments(m)
	return dbErrOrNil(tx.Model(&model.Expense{}).Where("id = ?", m.ID).UpdateColumns(map[string]any{
		"paid_amount": m.PaidAmount, "status": m.Status, "paid_on": m.PaidOn, "updated_at": time.Now(),
	}).Error, "expense")
}

func snapshotSupplier(m *model.Expense, s *model.Subject) {
	m.SupplierName, m.SupplierFullName = s.Name, s.FullName
	m.SupplierRegistrationNo, m.SupplierVatNo = s.RegistrationNo, s.VatNo
	m.SupplierStreet, m.SupplierCity, m.SupplierZip, m.SupplierCountry = s.Street, s.City, s.Zip, s.Country
	m.SupplierBankAccount, m.SupplierIBAN, m.SupplierSwiftBIC = s.BankAccount, s.IBAN, s.SwiftBIC
}

// buildExpenseLines converts input lines with the invoice line rules
// (buildLines); ids must belong to existing lines of the expense.
func buildExpenseLines(in []InvoiceLineInput, existing []model.ExpenseLine, defaultRate int32) ([]model.ExpenseLine, error) {
	known := make(map[uint]bool, len(existing))
	for _, l := range existing {
		known[l.ID] = true
	}
	// ids are checked here against expense lines; buildLines gets the lines
	// without ids (it only knows invoice lines) and the ids are put back below
	stripped := make([]InvoiceLineInput, len(in))
	for i, l := range in {
		if l.ID != nil {
			if !known[*l.ID] {
				return nil, invalid(fmt.Sprintf("lines[%d].id", i), "line not found on this expense")
			}
			delete(known, *l.ID) // a line id may appear only once
		}
		stripped[i] = l
		stripped[i].ID = nil
	}
	lines, err := buildLines(stripped, nil, defaultRate)
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseLine, len(lines))
	for i, l := range lines {
		out[i] = model.ExpenseLine{
			PriceItemID: l.PriceItemID, Name: l.Name, QuantityMilli: l.QuantityMilli,
			UnitName: l.UnitName, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps,
		}
		if in[i].ID != nil {
			out[i].ID = *in[i].ID
		}
	}
	return out, nil
}

func expenseBillingLines(lines []model.ExpenseLine) []billing.Line {
	out := make([]billing.Line, len(lines))
	for i, l := range lines {
		out[i] = billing.Line{QuantityMilli: l.QuantityMilli, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps}
	}
	return out
}

// expenseOptions: the supplier's VAT is recorded as charged, regardless of
// the account's own VAT mode.
func expenseOptions(m *model.Expense) billing.Options {
	return billing.Options{PricesIncludeVAT: m.PricesIncludeVat, RoundTotal: m.RoundTotal, ReverseCharge: m.ReverseCharge}
}

// recalcExpense validates dates and recomputes positions and all totals.
func recalcExpense(m *model.Expense) error {
	if m.TaxableFulfillmentDue != "" && !billing.ValidDate(m.TaxableFulfillmentDue) {
		return invalid("taxable_fulfillment_due", "invalid date")
	}
	if _, err := billing.ParseRate(m.ExchangeRate); err != nil {
		return invalid("exchange_rate", "the exchange rate must be a positive number")
	}
	if !billing.ValidDate(m.DueOn) {
		return invalid("due_on", "invalid date")
	}
	for i := range m.Lines {
		m.Lines[i].Position = i + 1
	}
	t, err := billing.Calculate(expenseBillingLines(m.Lines), expenseOptions(m))
	if err != nil {
		return invalid("lines", "amounts are out of range")
	}
	for i, la := range t.Lines {
		m.Lines[i].Base, m.Lines[i].Vat, m.Lines[i].Total = la.Base, la.Vat, la.Total
	}
	m.Subtotal, m.VatTotal, m.Rounding, m.Total = t.Subtotal, t.VatTotal, t.Rounding, t.Total
	return nil
}

// applyExpensePayments recomputes paid_amount, status and paid_on.
func applyExpensePayments(m *model.Expense) {
	m.PaidAmount = 0
	last := ""
	for _, p := range m.Payments {
		m.PaidAmount += p.Amount
		if p.PaidOn > last {
			last = p.PaidOn
		}
	}
	m.Status = billing.PaymentStatus(m.Status, m.Total, m.PaidAmount, len(m.Payments), false)
	if m.Status == model.StatusPaid {
		m.PaidOn = last
	} else {
		m.PaidOn = ""
	}
}

// saveExpenseLines stores m.Lines: removed lines are deleted, lines with id
// updated, new lines inserted.
func saveExpenseLines(tx *gorm.DB, m *model.Expense) error {
	keep := []uint{}
	for _, l := range m.Lines {
		if l.ID != 0 {
			keep = append(keep, l.ID)
		}
	}
	del := tx.Where("expense_id = ?", m.ID)
	if len(keep) > 0 {
		del = del.Where("id NOT IN ?", keep)
	}
	if err := del.Delete(&model.ExpenseLine{}).Error; err != nil {
		return dbErr(err, "expense line")
	}
	for i := range m.Lines {
		m.Lines[i].ExpenseID = m.ID
		if err := tx.Save(&m.Lines[i]).Error; err != nil {
			return dbErr(err, "expense line")
		}
	}
	return nil
}

// syncExpenseStock rewrites the stock moves of an expense: purchased
// stock-tracked items move in (negative quantities out).
func syncExpenseStock(ctx context.Context, tx *gorm.DB, m *model.Expense) error {
	lines := make([]stockLine, len(m.Lines))
	for i, l := range m.Lines {
		lines[i] = stockLine{PriceItemID: l.PriceItemID, QuantityMilli: l.QuantityMilli}
	}
	return rewriteDocStock(ctx, tx, docStock{"expense_id", m.ID}, true, m.IssuedOn, model.StockIn, lines)
}

func expenseNumberErr(err error, number string) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return conflict(CodeAlreadyExists, "expense number "+number+" already exists")
	}
	return dbErr(err, "expense")
}
