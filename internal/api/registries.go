package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/cnb"
	"github.com/qwerin/nanofaktura/internal/vatreg"
	"github.com/qwerin/nanofaktura/internal/vies"
)

// ExchangeRate is a ČNB rate: rate CZK = 1 unit of currency.
type ExchangeRate struct {
	Currency string `json:"currency" example:"EUR"`
	Date     string `json:"date" doc:"Requested date" example:"2026-09-26"`
	RateDate string `json:"rate_date" doc:"Date of the ČNB list the rate comes from (last working day for weekends/holidays)" example:"2026-09-25"`
	Rate     string `json:"rate" doc:"CZK per 1 unit, decimal string with ≥ 3 decimal places" example:"24.350"`
}

// ViesResult is the VIES check of an EU VAT number.
type ViesResult struct {
	VatNo       string `json:"vat_no" example:"CZ27082440"`
	CountryCode string `json:"country_code" doc:"VIES member state code (EL = Greece)"`
	Valid       bool   `json:"valid"`
	Name        string `json:"name" doc:"Empty when the member state does not disclose it"`
	Address     string `json:"address" doc:"Lines separated by \\n; empty when not disclosed"`
}

// VatRegistryAccount is a bank account published in the Czech VAT payer registry.
type VatRegistryAccount struct {
	Number      string `json:"number" doc:"Czech account \"prefix-number/bank\" or the published foreign number"`
	IBAN        string `json:"iban"`
	PublishedOn string `json:"published_on" format:"date"`
}

// VatRegistryResult is the status of a Czech VAT payer.
type VatRegistryResult struct {
	VatNo             string               `json:"vat_no" example:"CZ27082440"`
	Registered        bool                 `json:"registered" doc:"false = the DIČ is not in the register of VAT payers"`
	Reliable          *bool                `json:"reliable" nullable:"true" doc:"null when not registered; false = unreliable payer"`
	UnreliableSince   string               `json:"unreliable_since,omitempty" format:"date"`
	Name              string               `json:"name"`
	Address           string               `json:"address"`
	Street            string               `json:"street"`
	City              string               `json:"city"`
	Zip               string               `json:"zip"`
	PublishedAccounts []VatRegistryAccount `json:"published_accounts" nullable:"false"`
}

func (s *server) registerExchangeRates(authed huma.API) {
	huma.Get(authed, "/api/exchange-rates", s.getExchangeRate, func(o *huma.Operation) {
		o.Summary = "ČNB exchange rate of a currency on a day"
		o.Errors = []int{http.StatusUnprocessableEntity, http.StatusNotFound, http.StatusBadGateway}
	})
}

func (s *server) registerVies(authed huma.API) {
	huma.Get(authed, "/api/vies/{vat_no}", s.checkVies, func(o *huma.Operation) {
		o.Summary = "Check an EU VAT number in VIES"
		o.Errors = []int{http.StatusUnprocessableEntity, http.StatusBadGateway}
	})
}

func (s *server) registerVatRegistry(authed huma.API) {
	huma.Get(authed, "/api/vat-registry/{dic}", s.checkVatRegistry, func(o *huma.Operation) {
		o.Summary = "Czech VAT payer status (unreliable payer, published bank accounts)"
		o.Errors = []int{http.StatusUnprocessableEntity, http.StatusBadGateway}
	})
}

func queryInvalid(field, msg, value string) error {
	return huma.NewError(http.StatusUnprocessableEntity, "validation failed",
		&huma.ErrorDetail{Location: field, Message: msg, Value: value})
}

func (s *server) getExchangeRate(ctx context.Context, in *struct {
	Currency string `query:"currency" required:"true" doc:"ISO 4217 code, e.g. EUR"`
	Date     string `query:"date" doc:"YYYY-MM-DD, default today (Prague time); future dates get the latest rate"`
}) (*Out[ExchangeRate], error) {
	date := in.Date
	if date == "" {
		date = cnb.Today(s.deps.Now())
	}
	rate, rateDate, err := s.deps.CNB.Rate(ctx, in.Currency, date)
	switch {
	case errors.Is(err, cnb.ErrInvalidCurrency):
		return nil, queryInvalid("query.currency", err.Error(), in.Currency)
	case errors.Is(err, cnb.ErrInvalidDate):
		return nil, queryInvalid("query.date", err.Error(), in.Date)
	case errors.Is(err, cnb.ErrUnknownCurrency):
		return nil, notFound("exchange rate")
	case err != nil:
		return nil, huma.NewError(http.StatusBadGateway, "ČNB exchange rates are unavailable, try again later")
	}
	return &Out[ExchangeRate]{Body: ExchangeRate{Currency: strings.ToUpper(strings.TrimSpace(in.Currency)), Date: date, RateDate: rateDate, Rate: rate}}, nil
}

func (s *server) checkVies(ctx context.Context, in *struct {
	VatNo string `path:"vat_no" doc:"EU VAT number with country prefix, e.g. DE811907980"`
}) (*Out[ViesResult], error) {
	r, err := s.deps.VIES.Check(ctx, in.VatNo)
	switch {
	case errors.Is(err, vies.ErrInvalidInput):
		return nil, queryInvalid("path.vat_no", err.Error(), in.VatNo)
	case err != nil:
		return nil, huma.NewError(http.StatusBadGateway, "VIES is unavailable, try again later")
	}
	return &Out[ViesResult]{Body: ViesResult{VatNo: r.VatNo(), CountryCode: r.CountryCode, Valid: r.Valid,
		Name: r.Name, Address: r.Address}}, nil
}

func (s *server) checkVatRegistry(ctx context.Context, in *struct {
	DIC string `path:"dic" doc:"Czech DIČ, with or without the CZ prefix"`
}) (*Out[VatRegistryResult], error) {
	r, err := s.deps.VatRegistry.Check(ctx, in.DIC)
	switch {
	case errors.Is(err, vatreg.ErrInvalidDIC):
		return nil, queryInvalid("path.dic", err.Error(), in.DIC)
	case err != nil:
		return nil, huma.NewError(http.StatusBadGateway, "VAT payer registry is unavailable, try again later")
	}
	return &Out[VatRegistryResult]{Body: toVatRegistryResult(r)}, nil
}

func toVatRegistryResult(r *vatreg.Result) VatRegistryResult {
	out := VatRegistryResult{VatNo: r.VatNo, Registered: r.Registered, Reliable: r.Reliable,
		UnreliableSince: r.UnreliableSince, Name: r.Name, Address: r.Address(), Street: r.Street, City: r.City, Zip: r.Zip,
		PublishedAccounts: make([]VatRegistryAccount, 0, len(r.Accounts))}
	for _, a := range r.Accounts {
		out.PublishedAccounts = append(out.PublishedAccounts, VatRegistryAccount{Number: a.Number, IBAN: a.IBAN, PublishedOn: a.PublishedOn})
	}
	return out
}
