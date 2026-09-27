package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/ares"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// Subject is a contact — customer and/or supplier (SPEC §4.4).
type Subject struct {
	ID             uint      `json:"id"`
	Type           string    `json:"type" enum:"customer,supplier,both"`
	CustomID       *string   `json:"custom_id" doc:"Own identifier, unique within the account"`
	Name           string    `json:"name"`
	FullName       string    `json:"full_name" doc:"Contact person"`
	RegistrationNo string    `json:"registration_no" doc:"IČO"`
	VatNo          string    `json:"vat_no" doc:"DIČ"`
	LocalVatNo     string    `json:"local_vat_no" doc:"IČ DPH (Slovakia)"`
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
	DueDays        *int      `json:"due_days" doc:"Overrides the account's default_due_days"`
	Note           string    `json:"note" doc:"Private note"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SubjectCreate struct {
	Type           string  `json:"type,omitempty" enum:"customer,supplier,both" doc:"Default customer"`
	CustomID       *string `json:"custom_id,omitempty" maxLength:"50"`
	Name           string  `json:"name" minLength:"1" maxLength:"200"`
	FullName       string  `json:"full_name,omitempty" maxLength:"200"`
	RegistrationNo string  `json:"registration_no,omitempty" maxLength:"20" doc:"IČO: 8 digits with checksum (7 digits are left-padded)"`
	VatNo          string  `json:"vat_no,omitempty" maxLength:"20"`
	LocalVatNo     string  `json:"local_vat_no,omitempty" maxLength:"20"`
	Street         string  `json:"street,omitempty" maxLength:"200"`
	City           string  `json:"city,omitempty" maxLength:"100"`
	Zip            string  `json:"zip,omitempty" maxLength:"20"`
	Country        string  `json:"country,omitempty" maxLength:"2" doc:"ISO 3166-1 alpha-2, default CZ"`
	Email          string  `json:"email,omitempty" maxLength:"254"`
	EmailCopy      string  `json:"email_copy,omitempty" maxLength:"254"`
	Phone          string  `json:"phone,omitempty" maxLength:"50"`
	Web            string  `json:"web,omitempty" maxLength:"200"`
	BankAccount    string  `json:"bank_account,omitempty" maxLength:"50"`
	IBAN           string  `json:"iban,omitempty" maxLength:"42"`
	SwiftBIC       string  `json:"swift_bic,omitempty" maxLength:"11"`
	DueDays        *int    `json:"due_days,omitempty" minimum:"0" maximum:"365"`
	Note           string  `json:"note,omitempty" maxLength:"5000"`
}

// SubjectPatch: nil fields are left unchanged; custom_id "" clears it.
type SubjectPatch struct {
	Type           *string `json:"type,omitempty" enum:"customer,supplier,both"`
	CustomID       *string `json:"custom_id,omitempty" maxLength:"50"`
	Name           *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	FullName       *string `json:"full_name,omitempty" maxLength:"200"`
	RegistrationNo *string `json:"registration_no,omitempty" maxLength:"20"`
	VatNo          *string `json:"vat_no,omitempty" maxLength:"20"`
	LocalVatNo     *string `json:"local_vat_no,omitempty" maxLength:"20"`
	Street         *string `json:"street,omitempty" maxLength:"200"`
	City           *string `json:"city,omitempty" maxLength:"100"`
	Zip            *string `json:"zip,omitempty" maxLength:"20"`
	Country        *string `json:"country,omitempty" maxLength:"2"`
	Email          *string `json:"email,omitempty" maxLength:"254"`
	EmailCopy      *string `json:"email_copy,omitempty" maxLength:"254"`
	Phone          *string `json:"phone,omitempty" maxLength:"50"`
	Web            *string `json:"web,omitempty" maxLength:"200"`
	BankAccount    *string `json:"bank_account,omitempty" maxLength:"50"`
	IBAN           *string `json:"iban,omitempty" maxLength:"42"`
	SwiftBIC       *string `json:"swift_bic,omitempty" maxLength:"11"`
	DueDays        *int    `json:"due_days,omitempty" minimum:"0" maximum:"365"`
	Note           *string `json:"note,omitempty" maxLength:"5000"`
}

func toSubject(m *model.Subject) Subject {
	return Subject{
		ID: m.ID, Type: m.Type, CustomID: m.CustomID, Name: m.Name, FullName: m.FullName,
		RegistrationNo: m.RegistrationNo, VatNo: m.VatNo, LocalVatNo: m.LocalVatNo,
		Street: m.Street, City: m.City, Zip: m.Zip, Country: m.Country,
		Email: m.Email, EmailCopy: m.EmailCopy, Phone: m.Phone, Web: m.Web,
		BankAccount: m.BankAccount, IBAN: m.IBAN, SwiftBIC: m.SwiftBIC, DueDays: m.DueDays, Note: m.Note,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

var countryRe = regexp.MustCompile(`^[A-Z]{2}$`)

// normalizeSubject trims and validates a subject before saving.
func normalizeSubject(m *model.Subject) error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return invalid("name", "name must not be empty")
	}
	if m.Type == "" {
		m.Type = model.SubjectCustomer
	}
	if m.CustomID != nil {
		if v := strings.TrimSpace(*m.CustomID); v == "" {
			m.CustomID = nil
		} else {
			m.CustomID = &v
		}
	}
	if m.RegistrationNo = strings.TrimSpace(m.RegistrationNo); m.RegistrationNo != "" {
		ico, err := ares.NormalizeICO(m.RegistrationNo)
		if err != nil {
			return invalid("registration_no", err.Error())
		}
		m.RegistrationNo = ico
	}
	m.VatNo = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(m.VatNo), " ", ""))
	m.Country = strings.ToUpper(strings.TrimSpace(m.Country))
	if m.Country == "" {
		m.Country = "CZ"
	}
	if !countryRe.MatchString(m.Country) {
		return invalid("country", "expected an ISO 3166-1 alpha-2 code, e.g. CZ")
	}
	if m.IBAN = spayd.NormalizeIBAN(m.IBAN); m.IBAN != "" && !spayd.ValidIBAN(m.IBAN) {
		return invalid("iban", "invalid IBAN")
	}
	if m.SwiftBIC = strings.ToUpper(strings.TrimSpace(m.SwiftBIC)); m.SwiftBIC != "" && !swiftRe.MatchString(m.SwiftBIC) {
		return invalid("swift_bic", "invalid SWIFT/BIC code")
	}
	return nil
}

// subjectErr maps a save error; the only unique index is (account_id, custom_id).
func subjectErr(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return conflict("custom_id is already used by another subject")
	}
	return dbErr(err, "subject")
}

func (s *server) registerSubjects(g huma.API) {
	huma.Get(g, "/subjects", s.listSubjects)
	huma.Post(g, "/subjects", s.createSubject, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/subjects/{id}", s.getSubject)
	huma.Patch(g, "/subjects/{id}", s.patchSubject, auth.ForEditors)
	huma.Delete(g, "/subjects/{id}", s.deleteSubject, status(http.StatusNoContent), auth.ForEditors)
}

func (s *server) listSubjects(ctx context.Context, in *struct {
	PageParams
	SubjectFilter
}) (*Out[ListResponse[Subject]], error) {
	return paginate(in.SubjectFilter.query(s.scoped(ctx)), in.PageParams, toSubject)
}

func (s *server) getSubject(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[Subject], error) {
	var m model.Subject
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "subject")
	}
	return &Out[Subject]{Body: toSubject(&m)}, nil
}

func (s *server) createSubject(ctx context.Context, in *struct{ Body SubjectCreate }) (*Out[Subject], error) {
	b := in.Body
	m := model.Subject{
		AccountID: auth.AccountFrom(ctx).ID, Type: b.Type, CustomID: b.CustomID, Name: b.Name, FullName: b.FullName,
		RegistrationNo: b.RegistrationNo, VatNo: b.VatNo, LocalVatNo: b.LocalVatNo,
		Street: b.Street, City: b.City, Zip: b.Zip, Country: b.Country,
		Email: b.Email, EmailCopy: b.EmailCopy, Phone: b.Phone, Web: b.Web,
		BankAccount: b.BankAccount, IBAN: b.IBAN, SwiftBIC: b.SwiftBIC, DueDays: b.DueDays, Note: b.Note,
	}
	if err := normalizeSubject(&m); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, subjectErr(err)
	}
	return &Out[Subject]{Body: toSubject(&m)}, nil
}

func (s *server) patchSubject(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body SubjectPatch
}) (*Out[Subject], error) {
	var m model.Subject
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "subject")
	}
	p := in.Body
	if p.CustomID != nil {
		v := *p.CustomID
		m.CustomID = &v // "" becomes NULL in normalizeSubject
	}
	apply(&m.Type, p.Type)
	apply(&m.Name, p.Name)
	apply(&m.FullName, p.FullName)
	apply(&m.RegistrationNo, p.RegistrationNo)
	apply(&m.VatNo, p.VatNo)
	apply(&m.LocalVatNo, p.LocalVatNo)
	apply(&m.Street, p.Street)
	apply(&m.City, p.City)
	apply(&m.Zip, p.Zip)
	apply(&m.Country, p.Country)
	apply(&m.Email, p.Email)
	apply(&m.EmailCopy, p.EmailCopy)
	apply(&m.Phone, p.Phone)
	apply(&m.Web, p.Web)
	apply(&m.BankAccount, p.BankAccount)
	apply(&m.IBAN, p.IBAN)
	apply(&m.SwiftBIC, p.SwiftBIC)
	if p.DueDays != nil {
		v := *p.DueDays
		m.DueDays = &v
	}
	apply(&m.Note, p.Note)
	if err := normalizeSubject(&m); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Save(&m).Error; err != nil {
		return nil, subjectErr(err)
	}
	return &Out[Subject]{Body: toSubject(&m)}, nil
}

func (s *server) deleteSubject(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model.Subject
		if err := tx.Scopes(inAccount(ctx)).First(&m, in.ID).Error; err != nil {
			return dbErr(err, "subject")
		}
		var n int64
		if err := tx.Model(&model.Invoice{}).Scopes(inAccount(ctx)).Where("subject_id = ?", m.ID).Count(&n).Error; err != nil {
			return dbErr(err, "invoice")
		}
		if n > 0 {
			return conflict("subject has invoices and cannot be deleted")
		}
		if err := tx.Delete(&m).Error; err != nil {
			return dbErr(err, "subject")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}
