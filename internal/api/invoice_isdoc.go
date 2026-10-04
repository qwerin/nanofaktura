package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/isdoc"
	"github.com/qwerin/nanofaktura/internal/model"
)

// FileOutput is a raw file response (PDF, XML, …).
type FileOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Body               []byte
}

// fileResponses documents a binary/text response of the given media type.
func fileResponses(mediaType, desc string) map[string]*huma.Response {
	return map[string]*huma.Response{
		"200": {
			Description: desc,
			Content:     map[string]*huma.MediaType{mediaType: {Schema: &huma.Schema{Type: "string", Format: "binary"}}},
		},
	}
}

func (s *server) registerInvoiceISDOC(g huma.API) {
	huma.Register(g, huma.Operation{
		OperationID: "get-invoice-isdoc",
		Method:      http.MethodGet,
		Path:        "/invoices/{id}/isdoc",
		Summary:     "Invoice as ISDOC 6.0.2 XML",
		Tags:        []string{"Invoices"},
		Responses:   fileResponses("application/xml", "ISDOC document"),
	}, s.getInvoiceISDOC)
}

func (s *server) getInvoiceISDOC(ctx context.Context, in *invoiceID) (*FileOutput, error) {
	inv, err := loadInvoice(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	if err := notDraft(inv); err != nil {
		return nil, err
	}
	b, err := s.renderISDOC(ctx, inv)
	if err != nil {
		return nil, err
	}
	return &FileOutput{
		ContentType:        "application/xml",
		ContentDisposition: `attachment; filename="` + docFilename(inv.Number, ".isdoc") + `"`,
		Body:               b,
	}, nil
}

// renderISDOC generates the ISDOC of inv of the current account, with the
// related document (corrected invoice / paid proforma) when there is one.
func (s *server) renderISDOC(ctx context.Context, inv *model.Invoice) ([]byte, error) {
	acc := auth.AccountFrom(ctx)
	opt := isdoc.Options{}
	if inv.RelatedID != nil {
		var rel model.Invoice
		err := s.scoped(ctx).Select("id", "document_type", "number", "issued_on", "variable_symbol", "paid_amount").
			First(&rel, *inv.RelatedID).Error
		if err == nil {
			opt.Related = &isdoc.Related{
				DocumentType: rel.DocumentType, Number: rel.Number, IssuedOn: rel.IssuedOn,
				UUID: isdoc.DocumentUUID(acc.ID, rel.ID), VariableSymbol: rel.VariableSymbol, PaidAmount: rel.PaidAmount,
			}
		}
	}
	deps, err := invoiceDeposits(ctx, s.db.WithContext(ctx), inv)
	if err != nil {
		return nil, err
	}
	for i := range deps {
		opt.Deposits = append(opt.Deposits, &deps[i])
	}
	b, err := isdoc.Generate(inv, acc, opt)
	if err != nil {
		return nil, huma.Error500InternalServerError("isdoc generation failed", err)
	}
	return b, nil
}

// docFilename returns "faktura-<number><ext>" with only [A-Za-z0-9._-] kept.
func docFilename(number, ext string) string {
	return strings.TrimSuffix(pdfFilename(number), ".pdf") + ext
}
