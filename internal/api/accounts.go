package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Account is the company profile and invoicing defaults (SPEC §4.1).
type Account struct {
	Slug                 string       `json:"slug"`
	Role                 string       `json:"role" enum:"owner,admin,accountant,member" doc:"Current user's role"`
	Name                 string       `json:"name"`
	RegistrationNo       string       `json:"registration_no"`
	VatNo                string       `json:"vat_no"`
	Street               string       `json:"street"`
	City                 string       `json:"city"`
	Zip                  string       `json:"zip"`
	Country              string       `json:"country"`
	Email                string       `json:"email"`
	Phone                string       `json:"phone"`
	Web                  string       `json:"web"`
	VatMode              string       `json:"vat_mode" enum:"non_vat_payer,vat_payer,identified_person"`
	RegisteredBy         string       `json:"registered_by"`
	DefaultCurrency      string       `json:"default_currency"`
	DefaultDueDays       int          `json:"default_due_days"`
	DefaultPaymentMethod string       `json:"default_payment_method" enum:"bank,cash,card,cod,paypal,custom"`
	DefaultLanguage      string       `json:"default_language" enum:"cs,en,sk,de"`
	DefaultNote          string       `json:"default_note"`
	DefaultFooterNote    string       `json:"default_footer_note"`
	RoundTotal           bool         `json:"round_total"`
	DefaultVatRateBps    int32        `json:"default_vat_rate_bps"`
	LogoAttachmentID     *uint        `json:"logo_attachment_id,omitempty" doc:"Logo image attachment (PNG/JPEG)"`
	StampAttachmentID    *uint        `json:"stamp_attachment_id,omitempty" doc:"Signature/stamp image attachment (PNG/JPEG)"`
	VatPeriod            string       `json:"vat_period" enum:"month,quarter" doc:"VAT period of a VAT payer (reports)"`
	TaxOffice            string       `json:"c_ufo" doc:"EPO code of the tax office (finanční úřad), e.g. 451"`
	TaxOfficeBranch      string       `json:"c_pracufo" doc:"EPO code of the territorial workplace, e.g. 2001"`
	PdfTemplate          string       `json:"pdf_template" enum:"classic,modern,minimal" doc:"Default PDF template"`
	PdfAccent            string       `json:"pdf_accent" doc:"Accent colour #RRGGBB; empty = template default"`
	PdfShowQR            bool         `json:"pdf_show_qr" doc:"QR Platba on PDFs"`
	PdfFooter            string       `json:"pdf_footer" doc:"Extra footer text on every PDF page"`
	OnboardedAt          *time.Time   `json:"onboarded_at,omitempty" doc:"When the onboarding was finished (null = show onboarding)"`
	Capabilities         Capabilities `json:"capabilities" doc:"What the current user's role may do (derived from role)"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`

	AccountEmailSettings // emails.go
}

type AccountCreate struct {
	Name string `json:"name" minLength:"1" maxLength:"200"`
}

// AccountPatch: nil fields are left unchanged.
type AccountPatch struct {
	Name                 *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	RegistrationNo       *string `json:"registration_no,omitempty" maxLength:"20"`
	VatNo                *string `json:"vat_no,omitempty" maxLength:"20"`
	Street               *string `json:"street,omitempty" maxLength:"200"`
	City                 *string `json:"city,omitempty" maxLength:"100"`
	Zip                  *string `json:"zip,omitempty" maxLength:"20"`
	Country              *string `json:"country,omitempty" pattern:"^[A-Z]{2}$" doc:"ISO 3166-1 alpha-2"`
	Email                *string `json:"email,omitempty" maxLength:"254"`
	Phone                *string `json:"phone,omitempty" maxLength:"50"`
	Web                  *string `json:"web,omitempty" maxLength:"200"`
	VatMode              *string `json:"vat_mode,omitempty" enum:"non_vat_payer,vat_payer,identified_person"`
	RegisteredBy         *string `json:"registered_by,omitempty" maxLength:"500"`
	DefaultCurrency      *string `json:"default_currency,omitempty" pattern:"^[A-Z]{3}$" doc:"ISO 4217"`
	DefaultDueDays       *int    `json:"default_due_days,omitempty" minimum:"0" maximum:"365"`
	DefaultPaymentMethod *string `json:"default_payment_method,omitempty" enum:"bank,cash,card,cod,paypal,custom"`
	DefaultLanguage      *string `json:"default_language,omitempty" enum:"cs,en,sk,de"`
	DefaultNote          *string `json:"default_note,omitempty" maxLength:"5000"`
	DefaultFooterNote    *string `json:"default_footer_note,omitempty" maxLength:"5000"`
	RoundTotal           *bool   `json:"round_total,omitempty"`
	DefaultVatRateBps    *int32  `json:"default_vat_rate_bps,omitempty" minimum:"0" maximum:"10000"`
	LogoAttachmentID     *uint   `json:"logo_attachment_id,omitempty" doc:"PNG/JPEG attachment of this account; 0 removes the logo"`
	StampAttachmentID    *uint   `json:"stamp_attachment_id,omitempty" doc:"PNG/JPEG attachment of this account; 0 removes the stamp"`
	VatPeriod            *string `json:"vat_period,omitempty" enum:"month,quarter"`
	TaxOffice            *string `json:"c_ufo,omitempty" pattern:"^[0-9]{0,3}$" doc:"EPO c_ufo; empty clears"`
	TaxOfficeBranch      *string `json:"c_pracufo,omitempty" pattern:"^[0-9]{0,4}$" doc:"EPO c_pracufo; empty clears"`
	PdfTemplate          *string `json:"pdf_template,omitempty" enum:"classic,modern,minimal"`
	PdfAccent            *string `json:"pdf_accent,omitempty" pattern:"^(#[0-9A-Fa-f]{6})?$" doc:"#RRGGBB; empty = template default"`
	PdfShowQR            *bool   `json:"pdf_show_qr,omitempty"`
	PdfFooter            *string `json:"pdf_footer,omitempty" maxLength:"500"`
	Onboarded            *bool   `json:"onboarded,omitempty" doc:"true marks the onboarding as finished (sets onboarded_at), false shows it again"`

	AccountEmailSettingsPatch // emails.go
}

func toAccount(a *model.Account, role string) Account {
	return Account{
		Slug: a.Slug, Role: role, Name: a.Name, RegistrationNo: a.RegistrationNo, VatNo: a.VatNo,
		Street: a.Street, City: a.City, Zip: a.Zip, Country: a.Country, Email: a.Email, Phone: a.Phone, Web: a.Web,
		VatMode: a.VatMode, RegisteredBy: a.RegisteredBy, DefaultCurrency: a.DefaultCurrency,
		DefaultDueDays: a.DefaultDueDays, DefaultPaymentMethod: a.DefaultPaymentMethod,
		DefaultLanguage: a.DefaultLanguage, DefaultNote: a.DefaultNote, DefaultFooterNote: a.DefaultFooterNote,
		RoundTotal: a.RoundTotal, DefaultVatRateBps: a.DefaultVatRateBps,
		LogoAttachmentID: a.LogoAttachmentID, StampAttachmentID: a.StampAttachmentID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		VatPeriod: defaultStr(a.VatPeriod, model.VatPeriodMonth), TaxOffice: a.TaxOffice, TaxOfficeBranch: a.TaxOfficeBranch,
		PdfTemplate: defaultStr(a.PdfTemplate, "classic"), PdfAccent: a.PdfAccent, PdfShowQR: !a.PdfHideQR, PdfFooter: a.PdfFooter,
		OnboardedAt: a.OnboardedAt, Capabilities: capabilitiesOf(role),
		AccountEmailSettings: toAccountEmailSettings(a),
	}
}

// Capabilities are the permissions of a role (SPEC §7.13) for the client UI;
// the server enforces them per operation independently.
type Capabilities struct {
	Edit           bool `json:"edit" doc:"Create/modify documents, subjects, expenses, payments, attachments"`
	ManageSettings bool `json:"manage_settings" doc:"Account settings, bank accounts, number formats, webhooks"`
	ManageMembers  bool `json:"manage_members" doc:"Invite and manage members"`
	ManageOwners   bool `json:"manage_owners" doc:"Add/remove owners and change their role"`
	ViewReports    bool `json:"view_reports" doc:"Tax reports"`
	Export         bool `json:"export" doc:"Exports (CSV, XLSX, PDF ZIP)"`
}

func capabilitiesOf(role string) Capabilities {
	owner, admin := role == model.RoleOwner, role == model.RoleAdmin
	editor := owner || admin || role == model.RoleMember
	bookkeeper := owner || admin || role == model.RoleAccountant
	return Capabilities{Edit: editor, ManageSettings: owner || admin, ManageMembers: owner || admin,
		ManageOwners: owner, ViewReports: bookkeeper, Export: role != ""}
}

// defaultNumberFormats are created with every account (SPEC §4.3).
var defaultNumberFormats = []model.NumberFormat{
	{DocumentType: model.DocInvoice, Format: "{YYYY}-{NNNN}", IsDefault: true},
	{DocumentType: model.DocProforma, Format: "Z{YYYY}-{NNNN}", IsDefault: true},
	{DocumentType: model.DocCorrection, Format: "D{YYYY}-{NNNN}", IsDefault: true},
	{DocumentType: model.DocExpense, Format: "N{YYYY}-{NNNN}", IsDefault: true},
}

// newAccount creates an account with default settings and number formats
// and makes ownerID its owner. Must run inside a transaction.
func newAccount(tx *gorm.DB, name string, ownerID uint) (*model.Account, error) {
	name = strings.TrimSpace(name)
	slug, err := uniqueSlug(tx, name)
	if err != nil {
		return nil, dbErr(err, "account")
	}
	acc := model.Account{
		Slug: slug, Name: name, Country: "CZ", VatMode: model.VatModeNonPayer,
		DefaultCurrency: "CZK", DefaultDueDays: 14, DefaultPaymentMethod: "bank",
		DefaultLanguage: "cs", DefaultVatRateBps: 2100,
	}
	if err := tx.Create(&acc).Error; err != nil {
		return nil, dbErr(err, "account")
	}
	if err := tx.Create(&model.Membership{UserID: ownerID, AccountID: acc.ID, Role: model.RoleOwner}).Error; err != nil {
		return nil, dbErr(err, "membership")
	}
	for _, nf := range defaultNumberFormats {
		nf.AccountID = acc.ID
		if err := tx.Create(&nf).Error; err != nil {
			return nil, dbErr(err, "number format")
		}
	}
	return &acc, nil
}

func (s *server) registerAccounts(authed, account huma.API) {
	huma.Get(authed, "/api/accounts", s.listAccounts)
	huma.Post(authed, "/api/accounts", s.createAccount, status(http.StatusCreated))
	huma.Get(account, "", s.getAccount)
	huma.Patch(account, "", s.patchAccount, auth.ForManagers)
}

// accountRow is an account joined with the current user's membership role.
type accountRow struct {
	model.Account
	Role string
}

func (s *server) listAccounts(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[Account]], error) {
	q := s.db.WithContext(ctx).Table("accounts").
		Select("accounts.*, memberships.role").
		Joins("JOIN memberships ON memberships.account_id = accounts.id").
		Where("memberships.user_id = ?", auth.UserFrom(ctx).ID).
		Order("accounts.name")
	return paginate(q, in.PageParams, func(r *accountRow) Account { return toAccount(&r.Account, r.Role) })
}

func (s *server) createAccount(ctx context.Context, in *struct{ Body AccountCreate }) (*Out[Account], error) {
	var acc *model.Account
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		acc, err = newAccount(tx, in.Body.Name, auth.UserFrom(ctx).ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[Account]{Body: toAccount(acc, model.RoleOwner)}, nil
}

func (s *server) getAccount(ctx context.Context, _ *struct{}) (*Out[Account], error) {
	return &Out[Account]{Body: toAccount(auth.AccountFrom(ctx), auth.RoleFrom(ctx))}, nil
}

func (s *server) patchAccount(ctx context.Context, in *struct{ Body AccountPatch }) (*Out[Account], error) {
	acc := *auth.AccountFrom(ctx)
	p := in.Body
	if p.Name != nil {
		trimmed := strings.TrimSpace(*p.Name)
		p.Name = &trimmed
	}
	apply(&acc.Name, p.Name)
	apply(&acc.RegistrationNo, p.RegistrationNo)
	apply(&acc.VatNo, p.VatNo)
	apply(&acc.Street, p.Street)
	apply(&acc.City, p.City)
	apply(&acc.Zip, p.Zip)
	apply(&acc.Country, p.Country)
	apply(&acc.Email, p.Email)
	apply(&acc.Phone, p.Phone)
	apply(&acc.Web, p.Web)
	apply(&acc.VatMode, p.VatMode)
	apply(&acc.RegisteredBy, p.RegisteredBy)
	apply(&acc.DefaultCurrency, p.DefaultCurrency)
	apply(&acc.DefaultDueDays, p.DefaultDueDays)
	apply(&acc.DefaultPaymentMethod, p.DefaultPaymentMethod)
	apply(&acc.DefaultLanguage, p.DefaultLanguage)
	apply(&acc.DefaultNote, p.DefaultNote)
	apply(&acc.DefaultFooterNote, p.DefaultFooterNote)
	apply(&acc.RoundTotal, p.RoundTotal)
	apply(&acc.DefaultVatRateBps, p.DefaultVatRateBps)
	apply(&acc.VatPeriod, p.VatPeriod)
	apply(&acc.TaxOffice, p.TaxOffice)
	apply(&acc.TaxOfficeBranch, p.TaxOfficeBranch)
	apply(&acc.PdfTemplate, p.PdfTemplate)
	apply(&acc.PdfAccent, p.PdfAccent)
	apply(&acc.PdfFooter, p.PdfFooter)
	if p.PdfShowQR != nil {
		acc.PdfHideQR = !*p.PdfShowQR
	}
	if p.Onboarded != nil {
		switch {
		case !*p.Onboarded:
			acc.OnboardedAt = nil
		case acc.OnboardedAt == nil:
			now := s.deps.Now()
			acc.OnboardedAt = &now
		}
	}
	if acc.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	if err := applyEmailSettings(&acc, &p.AccountEmailSettingsPatch); err != nil {
		return nil, err
	}
	for _, img := range []struct {
		field string
		src   *uint
		dst   **uint
	}{{"logo_attachment_id", p.LogoAttachmentID, &acc.LogoAttachmentID}, {"stamp_attachment_id", p.StampAttachmentID, &acc.StampAttachmentID}} {
		if img.src == nil {
			continue
		}
		if *img.src == 0 {
			*img.dst = nil
			continue
		}
		if err := s.checkImageAttachment(ctx, img.field, *img.src); err != nil {
			return nil, err
		}
		id := *img.src
		*img.dst = &id
	}
	if err := s.db.WithContext(ctx).Save(&acc).Error; err != nil {
		return nil, dbErr(err, "account")
	}
	return &Out[Account]{Body: toAccount(&acc, auth.RoleFrom(ctx))}, nil
}

// checkImageAttachment: the attachment must belong to the account and be a
// PNG or JPEG (the formats the PDF renderer embeds) → 422 on field otherwise.
func (s *server) checkImageAttachment(ctx context.Context, field string, id uint) error {
	var a model.Attachment
	if err := s.scoped(ctx).First(&a, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid(field, "attachment not found")
		}
		return dbErr(err, "attachment")
	}
	if a.ContentType != "image/png" && a.ContentType != "image/jpeg" {
		return invalid(field, "attachment must be a PNG or JPEG image")
	}
	return nil
}
