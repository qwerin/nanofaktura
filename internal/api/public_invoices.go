package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// minPublicTokenLen: shorter tokens are rejected without a DB lookup.
const minPublicTokenLen = 24

// PublicParty is a supplier or customer as printed on the document.
type PublicParty struct {
	Name           string `json:"name"`
	FullName       string `json:"full_name,omitempty" doc:"Customer's contact person"`
	RegistrationNo string `json:"registration_no"`
	VatNo          string `json:"vat_no"`
	Street         string `json:"street"`
	City           string `json:"city"`
	Zip            string `json:"zip"`
	Country        string `json:"country"`
	RegisteredBy   string `json:"registered_by,omitempty" doc:"Supplier's registration (commercial register)"`
	VatMode        string `json:"vat_mode,omitempty" enum:"non_vat_payer,vat_payer,identified_person" doc:"Supplier only"`
	Email          string `json:"email,omitempty" doc:"Supplier only: contact e-mail"`
	Phone          string `json:"phone,omitempty" doc:"Supplier only"`
	Web            string `json:"web,omitempty" doc:"Supplier only"`
}

// PublicInvoiceLine is a document line without internal ids.
type PublicInvoiceLine struct {
	Name       string `json:"name"`
	Quantity   string `json:"quantity"`
	UnitName   string `json:"unit_name"`
	UnitPrice  int64  `json:"unit_price"`
	VatRateBps int32  `json:"vat_rate_bps"`
	Base       int64  `json:"base"`
	Vat        int64  `json:"vat"`
	Total      int64  `json:"total"`
}

// PublicInvoice is what the client sees behind the public link: the
// content of the PDF, without internal data (ids, private note, tags…).
type PublicInvoice struct {
	DocumentType   string `json:"document_type" enum:"invoice,proforma,correction"`
	Number         string `json:"number"`
	VariableSymbol string `json:"variable_symbol"`
	Status         string `json:"status" enum:"open,sent,overdue,paid,cancelled,uncollectible"`
	Cancelled      bool   `json:"cancelled" doc:"The document was cancelled (still viewable, show it as void)"`
	RelatedNumber  string `json:"related_number,omitempty" doc:"Corrected invoice / proforma number"`

	Supplier PublicParty `json:"supplier"`
	Customer PublicParty `json:"customer"`

	IssuedOn              string `json:"issued_on"`
	TaxableFulfillmentDue string `json:"taxable_fulfillment_due"`
	DueOn                 string `json:"due_on"`
	PaidOn                string `json:"paid_on"`

	Currency            string `json:"currency"`
	ExchangeRate        string `json:"exchange_rate"`
	Language            string `json:"language"`
	PaymentMethod       string `json:"payment_method" enum:"bank,cash,card,cod,paypal,custom"`
	CustomPaymentMethod string `json:"custom_payment_method"`
	BankAccount         string `json:"bank_account"`
	IBAN                string `json:"iban"`
	SwiftBIC            string `json:"swift_bic"`
	Spayd               string `json:"spayd" doc:"QR Platba (SPAYD) payload; empty when there is nothing to pay by a Czech bank transfer"`

	OrderNumber      string `json:"order_number"`
	Note             string `json:"note"`
	FooterNote       string `json:"footer_note"`
	PricesIncludeVat bool   `json:"prices_include_vat"`
	ReverseCharge    bool   `json:"reverse_charge"`

	Lines    []PublicInvoiceLine `json:"lines" nullable:"false"`
	VatRecap []VatRecapItem      `json:"vat_recap" nullable:"false"`

	Subtotal        int64 `json:"subtotal"`
	VatTotal        int64 `json:"vat_total"`
	Rounding        int64 `json:"rounding"`
	Total           int64 `json:"total"`
	PaidAmount      int64 `json:"paid_amount"`
	RemainingAmount int64 `json:"remaining_amount"`
}

type publicTokenInput struct {
	Token string `path:"token" doc:"Public token of the invoice"`
}

func (s *server) registerPublicInvoices(public, account huma.API) {
	huma.Register(public, huma.Operation{
		OperationID: "get-public-invoice", Method: http.MethodGet, Path: "/api/public/invoices/{token}",
		Summary: "Invoice behind a public client link", Tags: []string{"Public"},
	}, s.getPublicInvoice)
	huma.Register(public, huma.Operation{
		OperationID: "get-public-invoice-pdf", Method: http.MethodGet, Path: "/api/public/invoices/{token}/pdf",
		Summary: "PDF of a public invoice", Tags: []string{"Public"}, Responses: fileResponses("application/pdf", "PDF document"),
	}, s.getPublicInvoicePDF)
	huma.Register(public, huma.Operation{
		OperationID: "get-public-invoice-isdoc", Method: http.MethodGet, Path: "/api/public/invoices/{token}/isdoc",
		Summary: "ISDOC of a public invoice", Tags: []string{"Public"}, Responses: fileResponses("application/xml", "ISDOC document"),
	}, s.getPublicInvoiceISDOC)
	huma.Register(account, huma.Operation{
		OperationID: "get-pdf-preview", Method: http.MethodGet, Path: "/pdf-preview",
		Summary: "PDF template preview with sample data", Tags: []string{"Accounts"}, Responses: fileResponses("application/pdf", "PDF document"),
	}, s.getPDFPreview)
}

// publicInvoice resolves a token to an invoice (with lines and payments) and
// returns a context whose current account is the invoice's account. The
// first view is recorded. Unknown tokens → 404.
func (s *server) publicInvoice(ctx context.Context, token string) (context.Context, *model.Invoice, error) {
	if len(token) < minPublicTokenLen || len(token) > 64 {
		return nil, nil, notFound("invoice")
	}
	var m model.Invoice
	err := s.db.WithContext(ctx).
		Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("position, id") }).
		Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("paid_on, id") }).
		Where("public_token = ?", token).First(&m).Error
	if err != nil {
		return nil, nil, dbErr(err, "invoice")
	}
	var acc model.Account
	if err := s.db.WithContext(ctx).First(&acc, m.AccountID).Error; err != nil {
		return nil, nil, dbErr(err, "invoice")
	}
	actx := auth.WithAccount(ctx, &acc, "")
	if m.PublicViewedAt == nil {
		now := s.deps.Now()
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&model.Invoice{}).
				Where("id = ? AND public_viewed_at IS NULL", m.ID).Update("public_viewed_at", now)
			if res.Error != nil || res.RowsAffected == 0 {
				return dbErrOrNil(res.Error, "invoice")
			}
			return recordInvoice(actx, tx, events.PublicViewed, &m) // first view only
		})
		if err != nil {
			return nil, nil, err
		}
		m.PublicViewedAt = &now
	}
	return actx, &m, nil
}

func (s *server) getPublicInvoice(ctx context.Context, in *publicTokenInput) (*Out[PublicInvoice], error) {
	ctx, m, err := s.publicInvoice(ctx, in.Token)
	if err != nil {
		return nil, err
	}
	acc := auth.AccountFrom(ctx)
	out := PublicInvoice{
		DocumentType: m.DocumentType, Number: m.Number, VariableSymbol: m.VariableSymbol,
		Status: billing.EffectiveStatus(m.Status, m.DueOn, s.today()), Cancelled: m.Status == model.StatusCancelled,
		Supplier: PublicParty{
			Name: m.YourName, RegistrationNo: m.YourRegistrationNo, VatNo: m.YourVatNo, Street: m.YourStreet,
			City: m.YourCity, Zip: m.YourZip, Country: m.YourCountry, RegisteredBy: m.YourRegisteredBy,
			VatMode: m.YourVatMode, Email: acc.Email, Phone: acc.Phone, Web: acc.Web,
		},
		Customer: PublicParty{
			Name: m.ClientName, FullName: m.ClientFullName, RegistrationNo: m.ClientRegistrationNo, VatNo: m.ClientVatNo,
			Street: m.ClientStreet, City: m.ClientCity, Zip: m.ClientZip, Country: m.ClientCountry,
		},
		IssuedOn: m.IssuedOn, TaxableFulfillmentDue: m.TaxableFulfillmentDue, DueOn: m.DueOn, PaidOn: m.PaidOn,
		Currency: m.Currency, ExchangeRate: m.ExchangeRate, Language: m.Language, PaymentMethod: m.PaymentMethod,
		CustomPaymentMethod: m.CustomPaymentMethod, BankAccount: m.BankAccount, IBAN: m.IBAN, SwiftBIC: m.SwiftBIC,
		Spayd:       publicSpayd(m),
		OrderNumber: m.OrderNumber, Note: m.Note, FooterNote: m.FooterNote,
		PricesIncludeVat: m.PricesIncludeVat, ReverseCharge: m.ReverseCharge,
		Lines: make([]PublicInvoiceLine, len(m.Lines)), VatRecap: []VatRecapItem{},
		Subtotal: m.Subtotal, VatTotal: m.VatTotal, Rounding: m.Rounding, Total: m.Total,
		PaidAmount: m.PaidAmount, RemainingAmount: m.Total - m.PaidAmount,
	}
	if m.RelatedID != nil {
		var rel model.Invoice
		if err := s.scoped(ctx).Select("number").First(&rel, *m.RelatedID).Error; err == nil {
			out.RelatedNumber = rel.Number
		}
	}
	for i, l := range m.Lines {
		out.Lines[i] = PublicInvoiceLine{
			Name: l.Name, Quantity: billing.FormatQuantity(l.QuantityMilli), UnitName: l.UnitName, UnitPrice: l.UnitPrice,
			VatRateBps: l.VatRateBps, Base: l.Base, Vat: l.Vat, Total: l.Total,
		}
	}
	if m.YourVatMode == model.VatModePayer {
		out.VatRecap = toInvoice(m, s.today()).VatRecap
	}
	return &Out[PublicInvoice]{Body: out}, nil
}

// publicSpayd mirrors the QR conditions of the PDF: bank transfer in CZK to
// a Czech IBAN, something left to pay, not cancelled/uncollectible.
func publicSpayd(m *model.Invoice) string {
	due := m.Total - m.PaidAmount
	if due <= 0 || !strings.EqualFold(m.Currency, "CZK") || (m.PaymentMethod != "" && m.PaymentMethod != "bank") ||
		m.Status == model.StatusCancelled || m.Status == model.StatusUncollectible {
		return ""
	}
	title := map[string]string{"en": "Invoice", "sk": "Faktúra", "de": "Rechnung"}[m.Language]
	if title == "" {
		title = "Faktura"
	}
	return spayd.Build(spayd.Payment{
		IBAN: m.IBAN, Amount: due, Currency: "CZK", VariableSymbol: m.VariableSymbol, DueOn: m.DueOn,
		Message: title + " " + m.Number, RecipientName: m.YourName,
	})
}

func (s *server) getPublicInvoicePDF(ctx context.Context, in *publicTokenInput) (*FileOutput, error) {
	ctx, m, err := s.publicInvoice(ctx, in.Token)
	if err != nil {
		return nil, err
	}
	b, err := s.renderInvoicePDF(ctx, m, pdf.Options{})
	if err != nil {
		return nil, huma.Error500InternalServerError("pdf rendering failed", err)
	}
	return &FileOutput{ContentType: "application/pdf", ContentDisposition: `inline; filename="` + pdfFilename(m.Number) + `"`, Body: b}, nil
}

func (s *server) getPublicInvoiceISDOC(ctx context.Context, in *publicTokenInput) (*FileOutput, error) {
	ctx, m, err := s.publicInvoice(ctx, in.Token)
	if err != nil {
		return nil, err
	}
	b, err := s.renderISDOC(ctx, m)
	if err != nil {
		return nil, err
	}
	return &FileOutput{ContentType: "application/xml", ContentDisposition: `attachment; filename="` + docFilename(m.Number, ".isdoc") + `"`, Body: b}, nil
}

// getPDFPreview renders sample data with the account's own company profile,
// logo and stamp (settings preview, SPEC §7.14).
func (s *server) getPDFPreview(ctx context.Context, in *struct {
	Template     string `query:"template" enum:"classic,modern,minimal" doc:"PDF template (default classic)"`
	Lang         string `query:"lang" enum:"cs,en,sk,de" doc:"Language (default: account default language)"`
	DocumentType string `query:"document_type" enum:"invoice,proforma,correction" doc:"Default invoice"`
	Accent       string `query:"accent" pattern:"^(#[0-9A-Fa-f]{6})?$" doc:"Unsaved pdf_accent to preview"`
	ShowQR       string `query:"show_qr" enum:"true,false" doc:"Unsaved pdf_show_qr to preview"`
	Footer       string `query:"footer" maxLength:"500" doc:"Unsaved pdf_footer to preview"`
}) (*FileOutput, error) {
	acc := auth.AccountFrom(ctx)
	if in.Accent != "" || in.ShowQR != "" || in.Footer != "" {
		// preview unsaved appearance settings on a copy of the account
		cp := *acc
		cp.PdfAccent = defaultStr(in.Accent, cp.PdfAccent)
		cp.PdfFooter = defaultStr(in.Footer, cp.PdfFooter)
		if in.ShowQR != "" {
			cp.PdfHideQR = in.ShowQR == "false"
		}
		ctx = auth.WithAccount(ctx, &cp, auth.RoleFrom(ctx))
		acc = &cp
	}
	lang := defaultStr(in.Lang, acc.DefaultLanguage)
	inv, _ := pdf.Sample(pdf.SampleSpec{DocumentType: in.DocumentType, VatPayer: acc.VatMode == model.VatModePayer, Language: lang})
	snapshotYour(inv, acc)
	inv.AccountID = acc.ID
	inv.RelatedID = nil
	opt := pdf.Options{Template: in.Template, Language: lang}
	if inv.DocumentType == model.DocCorrection {
		opt.RelatedNumber = "2026-0041"
	}
	b, err := s.renderInvoicePDF(ctx, inv, opt)
	if err != nil {
		return nil, huma.Error500InternalServerError("pdf rendering failed", err)
	}
	return &FileOutput{ContentType: "application/pdf", ContentDisposition: `inline; filename="nahled.pdf"`, Body: b}, nil
}
