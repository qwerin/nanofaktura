package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// BankAccount is a bank account of the company (SPEC §4.2).
type BankAccount struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Currency  string    `json:"currency"`
	Number    string    `json:"number" doc:"Czech account number, e.g. 19-2000145399/0800; empty for foreign accounts"`
	IBAN      string    `json:"iban"`
	SwiftBIC  string    `json:"swift_bic"`
	IsDefault bool      `json:"is_default" doc:"Default account for invoices in its currency"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BankAccountCreate struct {
	Name      string `json:"name" minLength:"1" maxLength:"100"`
	Currency  string `json:"currency,omitempty" pattern:"^[A-Z]{3}$" doc:"ISO 4217; default is the account's default currency"`
	Number    string `json:"number,omitempty" maxLength:"30" doc:"Czech account number; IBAN and SWIFT are derived when empty"`
	IBAN      string `json:"iban,omitempty" maxLength:"42"`
	SwiftBIC  string `json:"swift_bic,omitempty" maxLength:"11"`
	IsDefault bool   `json:"is_default,omitempty" doc:"The first account of a currency is always default"`
}

// BankAccountPatch: nil fields are left unchanged. Changing number re-derives
// IBAN and SWIFT unless they are sent too.
type BankAccountPatch struct {
	Name      *string `json:"name,omitempty" minLength:"1" maxLength:"100"`
	Currency  *string `json:"currency,omitempty" pattern:"^[A-Z]{3}$"`
	Number    *string `json:"number,omitempty" maxLength:"30"`
	IBAN      *string `json:"iban,omitempty" maxLength:"42"`
	SwiftBIC  *string `json:"swift_bic,omitempty" maxLength:"11"`
	IsDefault *bool   `json:"is_default,omitempty"`
}

func toBankAccount(m *model.BankAccount) BankAccount {
	return BankAccount{ID: m.ID, Name: m.Name, Currency: m.Currency, Number: m.Number, IBAN: m.IBAN,
		SwiftBIC: m.SwiftBIC, IsDefault: m.IsDefault, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

var swiftRe = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)

// normalizeBankAccount validates number/IBAN/SWIFT and fills the derived fields.
// numberChanged tells whether IBAN/SWIFT should be re-derived from the number
// when the client did not send them (ibanSent/swiftSent).
func normalizeBankAccount(m *model.BankAccount, numberChanged, ibanSent, swiftSent bool) error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return invalid("name", "name must not be empty")
	}
	m.Number = strings.ReplaceAll(strings.TrimSpace(m.Number), " ", "")
	m.IBAN = spayd.NormalizeIBAN(m.IBAN)
	m.SwiftBIC = strings.ToUpper(strings.TrimSpace(m.SwiftBIC))
	if m.Number != "" {
		acc, err := spayd.ParseAccount(m.Number)
		if err != nil {
			return invalid("number", "invalid Czech account number (expected [prefix-]number/bank, mod-11 checksum)")
		}
		if numberChanged && !ibanSent || m.IBAN == "" {
			m.IBAN = acc.IBAN()
		}
		if numberChanged && !swiftSent || m.SwiftBIC == "" {
			m.SwiftBIC = acc.SWIFT()
		}
		if m.IBAN != acc.IBAN() {
			return invalid("iban", "IBAN does not match the account number")
		}
	}
	if m.Number == "" && m.IBAN == "" {
		return invalid("number", "number or iban is required")
	}
	if m.IBAN != "" && !spayd.ValidIBAN(m.IBAN) {
		return invalid("iban", "invalid IBAN")
	}
	if m.SwiftBIC != "" && !swiftRe.MatchString(m.SwiftBIC) {
		return invalid("swift_bic", "invalid SWIFT/BIC code")
	}
	return nil
}

// makeDefaultBankAccount unsets is_default on the other accounts in m's currency.
func makeDefaultBankAccount(ctx context.Context, tx *gorm.DB, m *model.BankAccount) error {
	return tx.Model(&model.BankAccount{}).Scopes(inAccount(ctx)).
		Where("currency = ? AND id <> ? AND is_default = ?", m.Currency, m.ID, true).
		Update("is_default", false).Error
}

func (s *server) registerBankAccounts(g huma.API) {
	huma.Get(g, "/bank-accounts", s.listBankAccounts)
	huma.Post(g, "/bank-accounts", s.createBankAccount, status(http.StatusCreated), auth.ForManagers)
	huma.Get(g, "/bank-accounts/{id}", s.getBankAccount)
	huma.Patch(g, "/bank-accounts/{id}", s.patchBankAccount, auth.ForManagers)
	huma.Delete(g, "/bank-accounts/{id}", s.deleteBankAccount, status(http.StatusNoContent), auth.ForManagers)
}

func (s *server) listBankAccounts(ctx context.Context, in *struct {
	PageParams
	Currency string `query:"currency" pattern:"^[A-Z]{3}$"`
}) (*Out[ListResponse[BankAccount]], error) {
	q := s.scoped(ctx).Model(&model.BankAccount{}).Order("currency, is_default DESC, name, id")
	if in.Currency != "" {
		q = q.Where("currency = ?", in.Currency)
	}
	return paginate(q, in.PageParams, toBankAccount)
}

func (s *server) getBankAccount(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[BankAccount], error) {
	var m model.BankAccount
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "bank account")
	}
	return &Out[BankAccount]{Body: toBankAccount(&m)}, nil
}

func (s *server) createBankAccount(ctx context.Context, in *struct{ Body BankAccountCreate }) (*Out[BankAccount], error) {
	acc := auth.AccountFrom(ctx)
	b := in.Body
	m := model.BankAccount{AccountID: acc.ID, Name: b.Name, Currency: b.Currency, Number: b.Number,
		IBAN: b.IBAN, SwiftBIC: b.SwiftBIC, IsDefault: b.IsDefault}
	if m.Currency == "" {
		m.Currency = acc.DefaultCurrency
	}
	if err := normalizeBankAccount(&m, true, b.IBAN != "", b.SwiftBIC != ""); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !m.IsDefault {
			var n int64
			if err := tx.Model(&model.BankAccount{}).Scopes(inAccount(ctx)).
				Where("currency = ? AND is_default = ?", m.Currency, true).Count(&n).Error; err != nil {
				return dbErr(err, "bank account")
			}
			m.IsDefault = n == 0
		}
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "bank account")
		}
		if m.IsDefault {
			if err := makeDefaultBankAccount(ctx, tx, &m); err != nil {
				return dbErr(err, "bank account")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[BankAccount]{Body: toBankAccount(&m)}, nil
}

func (s *server) patchBankAccount(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body BankAccountPatch
}) (*Out[BankAccount], error) {
	var m model.BankAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Scopes(inAccount(ctx)).First(&m, in.ID).Error; err != nil {
			return dbErr(err, "bank account")
		}
		p := in.Body
		numberChanged := p.Number != nil && strings.ReplaceAll(strings.TrimSpace(*p.Number), " ", "") != m.Number
		apply(&m.Name, p.Name)
		apply(&m.Currency, p.Currency)
		apply(&m.Number, p.Number)
		apply(&m.IBAN, p.IBAN)
		apply(&m.SwiftBIC, p.SwiftBIC)
		apply(&m.IsDefault, p.IsDefault)
		if err := normalizeBankAccount(&m, numberChanged, p.IBAN != nil, p.SwiftBIC != nil); err != nil {
			return err
		}
		if err := tx.Save(&m).Error; err != nil {
			return dbErr(err, "bank account")
		}
		if m.IsDefault {
			if err := makeDefaultBankAccount(ctx, tx, &m); err != nil {
				return dbErr(err, "bank account")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[BankAccount]{Body: toBankAccount(&m)}, nil
}

// deleteBankAccount removes the account; invoices keep their bank snapshot.
// When the default of a currency is deleted, the oldest remaining account in
// that currency becomes the default.
func (s *server) deleteBankAccount(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model.BankAccount
		if err := tx.Scopes(inAccount(ctx)).First(&m, in.ID).Error; err != nil {
			return dbErr(err, "bank account")
		}
		if err := tx.Delete(&m).Error; err != nil {
			return dbErr(err, "bank account")
		}
		if !m.IsDefault {
			return nil
		}
		var next model.BankAccount
		err := tx.Scopes(inAccount(ctx)).Where("currency = ?", m.Currency).Order("id").Limit(1).Find(&next).Error
		if err != nil {
			return dbErr(err, "bank account")
		}
		if next.ID != 0 {
			if err := tx.Model(&next).Update("is_default", true).Error; err != nil {
				return dbErr(err, "bank account")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}
