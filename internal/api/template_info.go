package api

import (
	"context"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Denormalised display data of templates and recurring invoices (subject
// name, total), so lists need no extra requests.

// templateTotal is the total of an invoice issued from t now (account
// defaults for empty rates / rounding, same VAT rules as invoices).
func templateTotal(acc *model.Account, t *model.InvoiceTemplate) (int64, string) {
	lines := make([]billing.Line, len(t.Lines))
	for i, l := range t.Lines {
		rate := acc.DefaultVatRateBps
		if l.VatRateBps != nil {
			rate = *l.VatRateBps
		}
		lines[i] = billing.Line{QuantityMilli: l.QuantityMilli, UnitPrice: l.UnitPrice, VatRateBps: rate}
	}
	round := acc.RoundTotal
	if t.RoundTotal != nil {
		round = *t.RoundTotal
	}
	tot, err := billing.Calculate(lines, billing.Options{
		PricesIncludeVAT: t.PricesIncludeVat, ReverseCharge: t.ReverseCharge, RoundTotal: round,
		NonVATPayer: billing.ChargesNoVAT(acc.VatMode, t.ReverseCharge),
	})
	if err != nil {
		return 0, ""
	}
	return tot.Total, defaultStr(t.Currency, defaultStr(acc.DefaultCurrency, "CZK"))
}

// subjectNames maps subject ids of the current account to their names.
func subjectNames(ctx context.Context, db *gorm.DB, ids []uint) (map[uint]string, error) {
	out := map[uint]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []model.Subject
	if err := db.Scopes(inAccount(ctx)).Select("id", "name").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, dbErr(err, "subjects")
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// templatesOut converts templates with subject names and totals.
func templatesOut(ctx context.Context, db *gorm.DB, ms []model.InvoiceTemplate) ([]Template, error) {
	ids := make([]uint, len(ms))
	for i := range ms {
		ids[i] = ms[i].SubjectID
	}
	names, err := subjectNames(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	acc := auth.AccountFrom(ctx)
	out := make([]Template, len(ms))
	for i := range ms {
		out[i] = toTemplate(&ms[i])
		out[i].SubjectName = names[ms[i].SubjectID]
		out[i].Total, out[i].TotalCurrency = templateTotal(acc, &ms[i])
	}
	return out, nil
}

func (s *server) templateOut(ctx context.Context, m *model.InvoiceTemplate) (*Out[Template], error) {
	out, err := templatesOut(ctx, s.db.WithContext(ctx), []model.InvoiceTemplate{*m})
	if err != nil {
		return nil, err
	}
	return &Out[Template]{Body: out[0]}, nil
}

// recurringsOut converts recurring invoices with their template's name,
// subject and total.
func recurringsOut(ctx context.Context, db *gorm.DB, ms []model.Recurring) ([]Recurring, error) {
	ids := make([]uint, len(ms))
	for i := range ms {
		ids[i] = ms[i].TemplateID
	}
	var tpls []model.InvoiceTemplate
	if len(ids) > 0 {
		if err := db.Scopes(inAccount(ctx)).Where("id IN ?", ids).Find(&tpls).Error; err != nil {
			return nil, dbErr(err, "templates")
		}
	}
	info, err := templatesOut(ctx, db, tpls)
	if err != nil {
		return nil, err
	}
	byID := map[uint]*Template{}
	for i := range info {
		byID[info[i].ID] = &info[i]
	}
	out := make([]Recurring, len(ms))
	for i := range ms {
		out[i] = toRecurring(&ms[i])
		if t := byID[ms[i].TemplateID]; t != nil {
			out[i].TemplateName, out[i].SubjectID, out[i].SubjectName = t.Name, t.SubjectID, t.SubjectName
			out[i].Total, out[i].TotalCurrency = t.Total, t.TotalCurrency
		}
	}
	return out, nil
}

func (s *server) recurringOut(ctx context.Context, m *model.Recurring) (*Out[Recurring], error) {
	out, err := recurringsOut(ctx, s.db.WithContext(ctx), []model.Recurring{*m})
	if err != nil {
		return nil, err
	}
	return &Out[Recurring]{Body: out[0]}, nil
}

// listOut runs paginate on models and converts the page with conv (batch
// converters that need extra queries).
func listOut[M, T any](q *gorm.DB, p PageParams, conv func([]M) ([]T, error)) (*Out[ListResponse[T]], error) {
	page, err := paginate(q, p, func(m *M) M { return *m })
	if err != nil {
		return nil, err
	}
	items, err := conv(page.Body.Items)
	if err != nil {
		return nil, err
	}
	b := page.Body
	return &Out[ListResponse[T]]{Body: ListResponse[T]{Items: items, Page: b.Page, PerPage: b.PerPage, Total: b.Total}}, nil
}
