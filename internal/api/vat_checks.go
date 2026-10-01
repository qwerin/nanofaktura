package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/vatreg"
)

// vatRegistryTimeout bounds the registry lookup done while serving an expense.
const vatRegistryTimeout = 3 * time.Second

// czechDIC returns the DIČ when vatNo is Czech (CZ prefix, or digits only
// with country CZ/empty), otherwise "".
func czechDIC(vatNo, country string) string {
	v := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(vatNo), " ", ""))
	if !strings.HasPrefix(v, "CZ") && country != "" && !strings.EqualFold(country, "CZ") {
		return ""
	}
	if _, err := vatreg.NormalizeDIC(v); err != nil {
		return ""
	}
	return v
}

// expenseWarnings checks the supplier in the VAT payer registry (SPEC §7.8):
// unreliable payer, or the expense's bank account not among the published
// ones. Registry failures produce no warning (logged only).
func (s *server) expenseWarnings(ctx context.Context, m *model.Expense) []string {
	dic := czechDIC(m.SupplierVatNo, m.SupplierCountry)
	if dic == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, vatRegistryTimeout)
	defer cancel()
	r, err := s.deps.VatRegistry.Check(ctx, dic)
	if err != nil {
		slog.Warn("VAT registry check failed", "dic", dic, "expense_id", m.ID, "err", err)
		return nil
	}
	var out []string
	if r.Reliable != nil && !*r.Reliable {
		w := "Dodavatel je nespolehlivý plátce DPH"
		if r.UnreliableSince != "" {
			w += " (od " + r.UnreliableSince + ")"
		}
		out = append(out, w)
	}
	if !r.Registered && m.VatTotal != 0 {
		out = append(out, "Dodavatel není v registru plátců DPH, ale účtuje DPH – odpočet z tohoto dokladu nelze uplatnit")
	}
	if r.Registered && (m.SupplierBankAccount != "" || m.SupplierIBAN != "") &&
		!r.HasAccount(m.SupplierBankAccount) && !r.HasAccount(m.SupplierIBAN) {
		acc := m.SupplierBankAccount
		if acc == "" {
			acc = m.SupplierIBAN
		}
		out = append(out, "Bankovní účet "+acc+" není zveřejněný v registru plátců DPH")
	}
	return out
}

func (s *server) registerSubjectVatStatus(g huma.API) {
	huma.Get(g, "/subjects/{id}/vat-status", s.subjectVatStatus, func(o *huma.Operation) {
		o.Summary = "VAT payer registry status of the subject's DIČ"
		o.Errors = []int{http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusBadGateway}
	})
}

func (s *server) subjectVatStatus(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[VatRegistryResult], error) {
	var subj model.Subject
	if err := s.scoped(ctx).First(&subj, in.ID).Error; err != nil {
		return nil, dbErr(err, "subject")
	}
	dic := czechDIC(subj.VatNo, subj.Country)
	if dic == "" {
		return nil, apiError(http.StatusUnprocessableEntity, CodeNoVatNo, "the subject has no Czech VAT number (vat_no)")
	}
	r, err := s.deps.VatRegistry.Check(ctx, dic)
	switch {
	case errors.Is(err, vatreg.ErrInvalidDIC):
		return nil, huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		return nil, huma.NewError(http.StatusBadGateway, "VAT payer registry is unavailable, try again later")
	}
	return &Out[VatRegistryResult]{Body: toVatRegistryResult(r)}, nil
}
