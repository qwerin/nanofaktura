package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

// InvoicePDFInput selects the invoice and optional look overrides.
type InvoicePDFInput struct {
	ID       uint   `path:"id"`
	Template string `query:"template" enum:"classic,modern,minimal" doc:"PDF template (default classic)"`
	Lang     string `query:"lang" enum:"cs,en,sk,de" doc:"Document language (default: the invoice language)"`
}

// InvoicePDFOutput is the raw PDF document.
type InvoicePDFOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Body               []byte
}

func (s *server) registerInvoicePDF(g huma.API) {
	huma.Register(g, huma.Operation{
		OperationID: "get-invoice-pdf",
		Method:      http.MethodGet,
		Path:        "/invoices/{id}/pdf",
		Summary:     "Invoice as PDF",
		Tags:        []string{"Invoices"},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "PDF document",
				Content: map[string]*huma.MediaType{
					"application/pdf": {Schema: &huma.Schema{Type: "string", Format: "binary"}},
				},
			},
		},
	}, s.getInvoicePDF)
}

func (s *server) getInvoicePDF(ctx context.Context, in *InvoicePDFInput) (*InvoicePDFOutput, error) {
	inv, err := loadInvoice(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	opt := pdf.Options{Template: in.Template, Language: in.Lang, ShowQR: true}
	if inv.RelatedID != nil {
		var rel model.Invoice
		if err := s.scoped(ctx).Select("number").First(&rel, *inv.RelatedID).Error; err == nil {
			opt.RelatedNumber = rel.Number
		}
	}
	b, err := pdf.Render(inv, auth.AccountFrom(ctx), opt)
	if err != nil {
		return nil, huma.Error500InternalServerError("pdf rendering failed", err)
	}
	return &InvoicePDFOutput{
		ContentType:        "application/pdf",
		ContentDisposition: `inline; filename="` + pdfFilename(inv.Number) + `"`,
		Body:               b,
	}, nil
}

// pdfFilename returns "faktura-<number>.pdf" with only [A-Za-z0-9._-] kept.
func pdfFilename(number string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		}
		return '-'
	}, number)
	safe = strings.Trim(safe, "-.")
	if safe == "" {
		safe = "doklad"
	}
	return "faktura-" + safe + ".pdf"
}
