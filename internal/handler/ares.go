package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/ares"
)

type AresOutput struct {
	Body ares.Lookup
}

// RegisterAres registruje GET /ares/{ic} endpoint.
// Frontend ho volá pro automatické doplnění dat firmy z ARES.
func RegisterAres(api huma.API) {
	type Input struct {
		IC string `path:"ic" doc:"IČO (8 číslic)" minLength:"8" maxLength:"8"`
	}

	client := ares.New(nil)

	huma.Register(api, huma.Operation{
		OperationID: "get-ares-subject",
		Method:      "GET",
		Path:        "/ares/{ic}",
		Summary:     "Načti data subjektu z ARES podle IČO",
		Tags:        []string{"ares"},
	}, func(ctx context.Context, in *Input) (*AresOutput, error) {
		subj, err := client.FetchByIC(ctx, in.IC)
		if err != nil {
			return nil, huma.Error404NotFound("subjekt nenalezen", err)
		}
		return &AresOutput{Body: *subj}, nil
	})
}
