package api

import (
	"context"
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
	Slug                 string    `json:"slug"`
	Role                 string    `json:"role" enum:"owner,member" doc:"Current user's role"`
	Name                 string    `json:"name"`
	RegistrationNo       string    `json:"registration_no"`
	VatNo                string    `json:"vat_no"`
	Street               string    `json:"street"`
	City                 string    `json:"city"`
	Zip                  string    `json:"zip"`
	Country              string    `json:"country"`
	Email                string    `json:"email"`
	Phone                string    `json:"phone"`
	Web                  string    `json:"web"`
	VatMode              string    `json:"vat_mode" enum:"non_vat_payer,vat_payer,identified_person"`
	RegisteredBy         string    `json:"registered_by"`
	DefaultCurrency      string    `json:"default_currency"`
	DefaultDueDays       int       `json:"default_due_days"`
	DefaultPaymentMethod string    `json:"default_payment_method" enum:"bank,cash,card,cod,paypal,custom"`
	DefaultLanguage      string    `json:"default_language" enum:"cs,en"`
	DefaultNote          string    `json:"default_note"`
	DefaultFooterNote    string    `json:"default_footer_note"`
	RoundTotal           bool      `json:"round_total"`
	DefaultVatRateBps    int32     `json:"default_vat_rate_bps"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
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
	DefaultLanguage      *string `json:"default_language,omitempty" enum:"cs,en"`
	DefaultNote          *string `json:"default_note,omitempty" maxLength:"5000"`
	DefaultFooterNote    *string `json:"default_footer_note,omitempty" maxLength:"5000"`
	RoundTotal           *bool   `json:"round_total,omitempty"`
	DefaultVatRateBps    *int32  `json:"default_vat_rate_bps,omitempty" minimum:"0" maximum:"10000"`
}

func toAccount(a *model.Account, role string) Account {
	return Account{
		Slug: a.Slug, Role: role, Name: a.Name, RegistrationNo: a.RegistrationNo, VatNo: a.VatNo,
		Street: a.Street, City: a.City, Zip: a.Zip, Country: a.Country, Email: a.Email, Phone: a.Phone, Web: a.Web,
		VatMode: a.VatMode, RegisteredBy: a.RegisteredBy, DefaultCurrency: a.DefaultCurrency,
		DefaultDueDays: a.DefaultDueDays, DefaultPaymentMethod: a.DefaultPaymentMethod,
		DefaultLanguage: a.DefaultLanguage, DefaultNote: a.DefaultNote, DefaultFooterNote: a.DefaultFooterNote,
		RoundTotal: a.RoundTotal, DefaultVatRateBps: a.DefaultVatRateBps, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// defaultNumberFormats are created with every account (SPEC §4.3).
var defaultNumberFormats = []model.NumberFormat{
	{DocumentType: model.DocInvoice, Format: "{YYYY}-{NNNN}", IsDefault: true},
	{DocumentType: model.DocProforma, Format: "Z{YYYY}-{NNNN}", IsDefault: true},
	{DocumentType: model.DocCorrection, Format: "D{YYYY}-{NNNN}", IsDefault: true},
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
	huma.Patch(account, "", s.patchAccount)
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
	if err := auth.RequireOwner(ctx); err != nil {
		return nil, err
	}
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
	if acc.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	if err := s.db.WithContext(ctx).Save(&acc).Error; err != nil {
		return nil, dbErr(err, "account")
	}
	return &Out[Account]{Body: toAccount(&acc, auth.RoleFrom(ctx))}, nil
}
