package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/ares"
)

// AresSubject is a subject found in the ARES register, ready to prefill a Subject.
type AresSubject struct {
	RegistrationNo string `json:"registration_no"`
	VatNo          string `json:"vat_no"`
	Name           string `json:"name"`
	Street         string `json:"street"`
	City           string `json:"city"`
	Zip            string `json:"zip"`
	Country        string `json:"country"`
}

func (s *server) registerAres(authed huma.API) {
	huma.Get(authed, "/api/ares/{ico}", s.aresLookup, func(o *huma.Operation) {
		o.Summary = "Look up a subject in ARES by IČO"
		o.Errors = []int{http.StatusUnprocessableEntity, http.StatusNotFound, http.StatusBadGateway}
	})
}

func (s *server) aresLookup(ctx context.Context, in *struct {
	ICO string `path:"ico" doc:"IČO (8 digits, 7 are left-padded)"`
}) (*Out[AresSubject], error) {
	ico, err := ares.NormalizeICO(in.ICO)
	if err != nil {
		return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
			&huma.ErrorDetail{Location: "path.ico", Message: err.Error(), Value: in.ICO})
	}
	r, err := s.deps.ARES.Lookup(ctx, ico)
	switch {
	case errors.Is(err, ares.ErrNotFound):
		return nil, notFound("subject in ARES")
	case errors.Is(err, ares.ErrInvalidICO):
		return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
			&huma.ErrorDetail{Location: "path.ico", Message: err.Error(), Value: in.ICO})
	case err != nil:
		return nil, huma.NewError(http.StatusBadGateway, "ARES is unavailable, try again later")
	}
	return &Out[AresSubject]{Body: AresSubject{
		RegistrationNo: r.RegistrationNo, VatNo: r.VatNo, Name: r.Name,
		Street: r.Street, City: r.City, Zip: r.Zip, Country: r.Country,
	}}, nil
}
