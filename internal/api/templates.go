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

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

// ---- DTOs ----

// TemplateLine is a line of an invoice template. Texts may contain the date
// placeholders {MONTH}, {MONTH_NAME}, {PREV_MONTH_NAME}, {NEXT_MONTH_NAME},
// {YEAR}, {PREV_YEAR}, {QUARTER} … filled in from the issue date.
type TemplateLine struct {
	PriceItemID *uint  `json:"price_item_id,omitempty"`
	Name        string `json:"name"`
	Quantity    string `json:"quantity" example:"1"`
	UnitName    string `json:"unit_name"`
	UnitPrice   int64  `json:"unit_price"`
	VatRateBps  *int32 `json:"vat_rate_bps,omitempty" doc:"Omitted = account default_vat_rate_bps at issue time"`
}

// Template is an invoice template (SPEC §7.3). Empty/omitted fields take the
// defaults of a new invoice when an invoice is issued from it.
type Template struct {
	ID                  uint           `json:"id"`
	Name                string         `json:"name"`
	DocumentType        string         `json:"document_type" enum:"invoice,proforma"`
	SubjectID           uint           `json:"subject_id"`
	SubjectName         string         `json:"subject_name" doc:"Current name of the subject"`
	Total               int64          `json:"total" doc:"Total of an invoice issued from the template now (account defaults applied)"`
	TotalCurrency       string         `json:"total_currency" doc:"Currency of total (template currency or account default)"`
	DueDays             *int           `json:"due_days,omitempty"`
	Currency            string         `json:"currency" doc:"Empty = account default"`
	ExchangeRate        string         `json:"exchange_rate" doc:"Empty = 1"`
	Language            string         `json:"language" doc:"cs|en|sk|de; empty = account default"`
	PaymentMethod       string         `json:"payment_method" doc:"Empty = account default"`
	CustomPaymentMethod string         `json:"custom_payment_method"`
	BankAccountID       *uint          `json:"bank_account_id,omitempty"`
	OrderNumber         string         `json:"order_number"`
	Note                *string        `json:"note,omitempty" doc:"Omitted = account default_note"`
	FooterNote          *string        `json:"footer_note,omitempty" doc:"Omitted = account default_footer_note"`
	PrivateNote         string         `json:"private_note"`
	Tags                []string       `json:"tags" nullable:"false"`
	PricesIncludeVat    bool           `json:"prices_include_vat"`
	RoundTotal          *bool          `json:"round_total,omitempty" doc:"Omitted = account round_total"`
	ReverseCharge       bool           `json:"reverse_charge"`
	Lines               []TemplateLine `json:"lines" nullable:"false"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// TemplateLineInput is a line of a template create/patch body.
type TemplateLineInput struct {
	PriceItemID *uint  `json:"price_item_id,omitempty"`
	Name        string `json:"name" minLength:"1" maxLength:"500"`
	Quantity    string `json:"quantity,omitempty" maxLength:"20" example:"1.5" doc:"Decimal, default 1"`
	UnitName    string `json:"unit_name,omitempty" maxLength:"20"`
	UnitPrice   int64  `json:"unit_price" minimum:"-100000000000000" maximum:"100000000000000"`
	VatRateBps  *int32 `json:"vat_rate_bps,omitempty" minimum:"0" maximum:"10000"`
}

type TemplateCreate struct {
	Name                string              `json:"name" minLength:"1" maxLength:"200"`
	DocumentType        string              `json:"document_type,omitempty" enum:"invoice,proforma" doc:"Default invoice"`
	SubjectID           uint                `json:"subject_id" minimum:"1"`
	DueDays             *int                `json:"due_days,omitempty" minimum:"0" maximum:"365"`
	Currency            string              `json:"currency,omitempty" pattern:"^[A-Z]{3}$"`
	ExchangeRate        string              `json:"exchange_rate,omitempty" pattern:"^[0-9]{1,6}([.][0-9]{1,6})?$"`
	Language            string              `json:"language,omitempty" enum:"cs,en,sk,de"`
	PaymentMethod       string              `json:"payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom"`
	CustomPaymentMethod string              `json:"custom_payment_method,omitempty" maxLength:"100"`
	BankAccountID       *uint               `json:"bank_account_id,omitempty"`
	OrderNumber         string              `json:"order_number,omitempty" maxLength:"100"`
	Note                *string             `json:"note,omitempty" maxLength:"5000"`
	FooterNote          *string             `json:"footer_note,omitempty" maxLength:"5000"`
	PrivateNote         string              `json:"private_note,omitempty" maxLength:"5000"`
	Tags                []string            `json:"tags,omitempty" maxItems:"50"`
	PricesIncludeVat    bool                `json:"prices_include_vat,omitempty"`
	RoundTotal          *bool               `json:"round_total,omitempty"`
	ReverseCharge       bool                `json:"reverse_charge,omitempty"`
	Lines               []TemplateLineInput `json:"lines" minItems:"1" maxItems:"500"`
}

// TemplatePatch: nil fields are left unchanged; lines replace the list.
type TemplatePatch struct {
	Name                *string             `json:"name,omitempty" minLength:"1" maxLength:"200"`
	DocumentType        *string             `json:"document_type,omitempty" enum:"invoice,proforma"`
	SubjectID           *uint               `json:"subject_id,omitempty" minimum:"1"`
	DueDays             *int                `json:"due_days,omitempty" minimum:"0" maximum:"365"`
	Currency            *string             `json:"currency,omitempty" pattern:"^([A-Z]{3})?$" doc:"\"\" = account default"`
	ExchangeRate        *string             `json:"exchange_rate,omitempty" pattern:"^([0-9]{1,6}([.][0-9]{1,6})?)?$"`
	Language            *string             `json:"language,omitempty" enum:"cs,en,sk,de,"`
	PaymentMethod       *string             `json:"payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom,"`
	CustomPaymentMethod *string             `json:"custom_payment_method,omitempty" maxLength:"100"`
	BankAccountID       *uint               `json:"bank_account_id,omitempty" doc:"0 = default bank account of the currency"`
	OrderNumber         *string             `json:"order_number,omitempty" maxLength:"100"`
	Note                *string             `json:"note,omitempty" maxLength:"5000"`
	FooterNote          *string             `json:"footer_note,omitempty" maxLength:"5000"`
	PrivateNote         *string             `json:"private_note,omitempty" maxLength:"5000"`
	Tags                []string            `json:"tags,omitempty" maxItems:"50"`
	PricesIncludeVat    *bool               `json:"prices_include_vat,omitempty"`
	RoundTotal          *bool               `json:"round_total,omitempty"`
	ReverseCharge       *bool               `json:"reverse_charge,omitempty"`
	Lines               []TemplateLineInput `json:"lines,omitempty" minItems:"1" maxItems:"500"`
}

// TemplateIssue is the optional body of POST /templates/{id}/create-invoice.
type TemplateIssue struct {
	IssuedOn     string `json:"issued_on,omitempty" format:"date" doc:"Default today; also the date of the placeholders"`
	DocumentType string `json:"document_type,omitempty" enum:"invoice,proforma" doc:"Default: the template's document_type"`
	Draft        bool   `json:"draft,omitempty" doc:"Create a draft (no number until issued)"`
}

// SaveAsTemplate is the optional body of POST /invoices/{id}/save-as-template.
type SaveAsTemplate struct {
	Name string `json:"name,omitempty" maxLength:"200" doc:"Default: client name and invoice number"`
}

func toTemplate(m *model.InvoiceTemplate) Template {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	out := Template{
		ID: m.ID, Name: m.Name, DocumentType: m.DocumentType, SubjectID: m.SubjectID, DueDays: m.DueDays,
		Currency: m.Currency, ExchangeRate: m.ExchangeRate, Language: m.Language, PaymentMethod: m.PaymentMethod,
		CustomPaymentMethod: m.CustomPaymentMethod, BankAccountID: m.BankAccountID, OrderNumber: m.OrderNumber,
		Note: m.Note, FooterNote: m.FooterNote, PrivateNote: m.PrivateNote, Tags: tags,
		PricesIncludeVat: m.PricesIncludeVat, RoundTotal: m.RoundTotal, ReverseCharge: m.ReverseCharge,
		Lines: make([]TemplateLine, len(m.Lines)), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	for i, l := range m.Lines {
		out.Lines[i] = TemplateLine{
			PriceItemID: l.PriceItemID, Name: l.Name, Quantity: billing.FormatQuantity(l.QuantityMilli),
			UnitName: l.UnitName, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps,
		}
	}
	return out
}

// ---- routes ----

func (s *server) registerTemplates(g huma.API) {
	huma.Get(g, "/templates", s.listTemplates)
	huma.Post(g, "/templates", s.createTemplate, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/templates/{id}", s.getTemplate)
	huma.Patch(g, "/templates/{id}", s.patchTemplate, auth.ForEditors)
	huma.Delete(g, "/templates/{id}", s.deleteTemplate, status(http.StatusNoContent), auth.ForEditors)
	huma.Post(g, "/templates/{id}/create-invoice", s.createInvoiceFromTemplate, status(http.StatusCreated), auth.ForEditors)
	huma.Post(g, "/invoices/{id}/save-as-template", s.saveAsTemplate, status(http.StatusCreated), auth.ForEditors)
}

func (s *server) listTemplates(ctx context.Context, in *struct {
	PageParams
	Query     string `query:"query" doc:"Name (case-insensitive substring)"`
	SubjectID uint   `query:"subject_id"`
}) (*Out[ListResponse[Template]], error) {
	q := s.scoped(ctx).Model(&model.InvoiceTemplate{}).Order("LOWER(name), id")
	if qs := strings.TrimSpace(in.Query); qs != "" {
		q = q.Where(`LOWER(name) LIKE ? ESCAPE '\'`, likePattern(qs))
	}
	if in.SubjectID != 0 {
		q = q.Where("subject_id = ?", in.SubjectID)
	}
	return listOut(q, in.PageParams, func(ms []model.InvoiceTemplate) ([]Template, error) {
		return templatesOut(ctx, s.db.WithContext(ctx), ms)
	})
}

func (s *server) loadTemplate(ctx context.Context, db *gorm.DB, id uint) (*model.InvoiceTemplate, error) {
	var m model.InvoiceTemplate
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "template")
	}
	return &m, nil
}

func (s *server) getTemplate(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[Template], error) {
	m, err := s.loadTemplate(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	return s.templateOut(ctx, m)
}

func (s *server) createTemplate(ctx context.Context, in *struct{ Body TemplateCreate }) (*Out[Template], error) {
	b := &in.Body
	m := &model.InvoiceTemplate{
		AccountID: auth.AccountFrom(ctx).ID, Name: strings.TrimSpace(b.Name),
		DocumentType: defaultStr(b.DocumentType, model.DocInvoice), SubjectID: b.SubjectID, DueDays: b.DueDays,
		Currency: b.Currency, ExchangeRate: b.ExchangeRate, Language: b.Language, PaymentMethod: b.PaymentMethod,
		CustomPaymentMethod: b.CustomPaymentMethod, BankAccountID: b.BankAccountID, OrderNumber: b.OrderNumber,
		Note: b.Note, FooterNote: b.FooterNote, PrivateNote: b.PrivateNote, Tags: normalizeTags(b.Tags),
		PricesIncludeVat: b.PricesIncludeVat, RoundTotal: b.RoundTotal, ReverseCharge: b.ReverseCharge,
	}
	var err error
	if m.Lines, err = templateLines(b.Lines); err != nil {
		return nil, err
	}
	if err := s.checkTemplate(ctx, s.db.WithContext(ctx), m); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		return nil, dbErr(err, "template")
	}
	return s.templateOut(ctx, m)
}

func (s *server) patchTemplate(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body TemplatePatch
}) (*Out[Template], error) {
	db := s.db.WithContext(ctx)
	m, err := s.loadTemplate(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	p := &in.Body
	if p.Name != nil {
		m.Name = strings.TrimSpace(*p.Name)
	}
	apply(&m.DocumentType, p.DocumentType)
	apply(&m.SubjectID, p.SubjectID)
	if p.DueDays != nil {
		m.DueDays = p.DueDays
	}
	apply(&m.Currency, p.Currency)
	apply(&m.ExchangeRate, p.ExchangeRate)
	apply(&m.Language, p.Language)
	apply(&m.PaymentMethod, p.PaymentMethod)
	apply(&m.CustomPaymentMethod, p.CustomPaymentMethod)
	if p.BankAccountID != nil {
		if *p.BankAccountID == 0 {
			m.BankAccountID = nil
		} else {
			m.BankAccountID = p.BankAccountID
		}
	}
	apply(&m.OrderNumber, p.OrderNumber)
	if p.Note != nil {
		m.Note = p.Note
	}
	if p.FooterNote != nil {
		m.FooterNote = p.FooterNote
	}
	apply(&m.PrivateNote, p.PrivateNote)
	if p.Tags != nil {
		m.Tags = normalizeTags(p.Tags)
	}
	apply(&m.PricesIncludeVat, p.PricesIncludeVat)
	if p.RoundTotal != nil {
		m.RoundTotal = p.RoundTotal
	}
	apply(&m.ReverseCharge, p.ReverseCharge)
	if p.Lines != nil {
		if m.Lines, err = templateLines(p.Lines); err != nil {
			return nil, err
		}
	}
	if err := s.checkTemplate(ctx, db, m); err != nil {
		return nil, err
	}
	if err := db.Save(m).Error; err != nil {
		return nil, dbErr(err, "template")
	}
	return s.templateOut(ctx, m)
}

func (s *server) deleteTemplate(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadTemplate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.Recurring{}).Scopes(inAccount(ctx)).Where("template_id = ?", m.ID).Count(&n).Error; err != nil {
			return dbErr(err, "recurring")
		}
		if n > 0 {
			return conflict(CodeUsedByRecurring, "the template is used by a recurring invoice; delete the recurring invoice first")
		}
		return dbErrOrNil(tx.Delete(m).Error, "template")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) createInvoiceFromTemplate(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body *TemplateIssue
}) (*Out[Invoice], error) {
	b := TemplateIssue{}
	if in.Body != nil {
		b = *in.Body
	}
	issuedOn := defaultStr(b.IssuedOn, s.today())
	t, err := s.loadTemplate(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	body := templateInvoice(ctx, t, defaultStr(b.DocumentType, t.DocumentType), issuedOn)
	body.Draft = b.Draft
	if body.ExchangeRate, err = s.templateRate(ctx, t, issuedOn); err != nil {
		return nil, err
	}
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := s.createInvoiceTx(ctx, tx, &body)
		if err != nil {
			return 0, err
		}
		return m.ID, nil
	})
}

// templateRate is the exchange rate of an invoice issued from template t on
// issuedOn: the template's rate when set (an explicit fixed rate), else the
// ČNB rate of the issue date (= DUZP) for a foreign currency. It calls the
// ČNB client, so it must run outside a transaction.
func (s *server) templateRate(ctx context.Context, t *model.InvoiceTemplate, issuedOn string) (string, error) {
	return s.defaultExchangeRate(ctx, defaultStr(t.Currency, auth.AccountFrom(ctx).DefaultCurrency), t.ExchangeRate, issuedOn)
}

func (s *server) saveAsTemplate(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body *SaveAsTemplate
}) (*Out[Template], error) {
	db := s.db.WithContext(ctx)
	inv, err := loadInvoice(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	if inv.DocumentType == model.DocCorrection {
		return nil, conflict(CodeCorrectionTemplate, "a correction cannot be saved as a template")
	}
	if inv.DocumentType == model.DocTaxDocument {
		return nil, conflict(CodeTaxDocumentFixed, "a tax document for a received payment cannot be saved as a template")
	}
	if inv.SubjectID == nil {
		return nil, conflict(CodeTemplateNeedsSubject, "an invoice without a contact cannot be saved as a template")
	}
	name := ""
	if in.Body != nil {
		name = strings.TrimSpace(in.Body.Name)
	}
	if name == "" {
		name = strings.TrimSpace(inv.ClientName + " " + inv.Number)
	}
	dueDays, note, footer, round := inv.DueDays, inv.Note, inv.FooterNote, inv.RoundTotal
	m := &model.InvoiceTemplate{
		AccountID: inv.AccountID, Name: name, DocumentType: inv.DocumentType, SubjectID: *inv.SubjectID,
		DueDays: &dueDays, Currency: inv.Currency, ExchangeRate: inv.ExchangeRate, Language: inv.Language,
		PaymentMethod: inv.PaymentMethod, CustomPaymentMethod: inv.CustomPaymentMethod, BankAccountID: inv.BankAccountID,
		OrderNumber: inv.OrderNumber, Note: &note, FooterNote: &footer, PrivateNote: inv.PrivateNote,
		Tags: normalizeTags(inv.Tags), PricesIncludeVat: inv.PricesIncludeVat, RoundTotal: &round,
		ReverseCharge: inv.ReverseCharge, Lines: make([]model.TemplateLine, len(inv.Lines)),
	}
	for i, l := range inv.Lines {
		rate := l.VatRateBps
		m.Lines[i] = model.TemplateLine{
			PriceItemID: l.PriceItemID, Name: l.Name, QuantityMilli: l.QuantityMilli,
			UnitName: l.UnitName, UnitPrice: l.UnitPrice, VatRateBps: &rate,
		}
	}
	if m.BankAccountID != nil { // the bank account may have been deleted since
		var n int64
		if err := db.Model(&model.BankAccount{}).Scopes(inAccount(ctx)).Where("id = ?", *m.BankAccountID).Count(&n).Error; err != nil {
			return nil, dbErr(err, "bank account")
		}
		if n == 0 {
			m.BankAccountID = nil
		}
	}
	if err := db.Create(m).Error; err != nil {
		return nil, dbErr(err, "template")
	}
	return s.templateOut(ctx, m)
}

// ---- helpers ----

func templateLines(in []TemplateLineInput) ([]model.TemplateLine, error) {
	out := make([]model.TemplateLine, len(in))
	for i, l := range in {
		field := fmt.Sprintf("lines[%d]", i)
		ml := model.TemplateLine{PriceItemID: l.PriceItemID, UnitName: strings.TrimSpace(l.UnitName), UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps}
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
		out[i] = ml
	}
	return out, nil
}

// checkTemplate validates the name and references (subject, bank account) → 422.
func (s *server) checkTemplate(ctx context.Context, db *gorm.DB, m *model.InvoiceTemplate) error {
	if m.Name == "" {
		return invalid("name", "name must not be empty")
	}
	if _, err := findSubject(ctx, db, m.SubjectID); err != nil {
		return err
	}
	if m.BankAccountID != nil {
		var ba model.BankAccount
		if err := db.Scopes(inAccount(ctx)).Select("id").First(&ba, *m.BankAccountID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return invalid("bank_account_id", "bank account not found")
			}
			return dbErr(err, "bank account")
		}
	}
	return nil
}

// templateInvoice builds the create body of an invoice issued from t on
// issuedOn: date placeholders in line names, notes and the order number are
// filled in (language: template → account default).
func templateInvoice(ctx context.Context, t *model.InvoiceTemplate, docType, issuedOn string) InvoiceCreate {
	acc := auth.AccountFrom(ctx)
	lang := defaultStr(t.Language, acc.DefaultLanguage)
	fill := func(s string) string { return billing.RenderDatePlaceholders(s, issuedOn, lang) }
	note, footer := acc.DefaultNote, acc.DefaultFooterNote
	apply(&note, t.Note)
	apply(&footer, t.FooterNote)
	note, footer = fill(note), fill(footer)
	body := InvoiceCreate{
		DocumentType: docType, SubjectID: &t.SubjectID, IssuedOn: issuedOn, DueDays: t.DueDays,
		Currency: t.Currency, ExchangeRate: t.ExchangeRate, Language: t.Language,
		PaymentMethod: t.PaymentMethod, CustomPaymentMethod: t.CustomPaymentMethod, BankAccountID: t.BankAccountID,
		OrderNumber: fill(t.OrderNumber), Note: &note, FooterNote: &footer, PrivateNote: fill(t.PrivateNote),
		Tags: t.Tags, PricesIncludeVat: t.PricesIncludeVat, RoundTotal: t.RoundTotal, ReverseCharge: t.ReverseCharge,
		Lines: make([]InvoiceLineInput, len(t.Lines)),
	}
	for i, l := range t.Lines {
		body.Lines[i] = InvoiceLineInput{
			PriceItemID: l.PriceItemID, Name: fill(l.Name), Quantity: billing.FormatQuantity(l.QuantityMilli),
			UnitName: l.UnitName, UnitPrice: l.UnitPrice, VatRateBps: l.VatRateBps,
		}
	}
	return body
}
