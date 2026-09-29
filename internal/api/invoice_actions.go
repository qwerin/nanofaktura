package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
)

func (s *server) registerInvoiceActions(g huma.API) {
	huma.Post(g, "/invoices/{id}/actions/{action}", s.invoiceAction, auth.ForEditors)
	huma.Post(g, "/invoices/{id}/correction", s.createCorrection, status(http.StatusCreated), auth.ForEditors)
	huma.Post(g, "/invoices/{id}/duplicate", s.duplicateInvoice, status(http.StatusCreated), auth.ForEditors)
	huma.Post(g, "/invoices/{id}/regenerate-public-token", s.regeneratePublicToken, auth.ForEditors)
}

func (s *server) invoiceAction(ctx context.Context, in *struct {
	ID     uint   `path:"id"`
	Action string `path:"action" enum:"mark_as_sent,cancel,undo_cancel,mark_as_uncollectible,undo_uncollectible,lock,unlock"`
}) (*Out[Invoice], error) {
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := loadInvoice(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		st, err := billing.ApplyAction(billing.State{
			Status: m.Status, SentAt: m.SentAt, CancelledAt: m.CancelledAt,
			UncollectibleAt: m.UncollectibleAt, LockedAt: m.LockedAt, HasPayments: len(m.Payments) > 0,
		}, in.Action, s.deps.Now())
		if err != nil {
			var te *billing.TransitionError
			if errors.As(err, &te) {
				return 0, conflict(CodeInvalidTransition, te.Msg)
			}
			return 0, err
		}
		m.Status, m.SentAt, m.CancelledAt, m.UncollectibleAt, m.LockedAt =
			st.Status, st.SentAt, st.CancelledAt, st.UncollectibleAt, st.LockedAt
		if err := tx.Omit(clause.Associations).Save(m).Error; err != nil {
			return 0, dbErr(err, "invoice")
		}
		if err := recordInvoice(ctx, tx, invoiceActionEvents[in.Action], m); err != nil {
			return 0, err
		}
		if in.Action == billing.ActionCancel || in.Action == billing.ActionUndoCancel {
			return m.ID, syncInvoiceStock(ctx, tx, m) // cancelled invoices return their goods
		}
		return m.ID, nil
	})
}

// createCorrection creates a correction of an invoice with the lines copied
// and quantities negated (SPEC §4.5 "Dobropis").
func (s *server) createCorrection(ctx context.Context, in *invoiceID) (*Out[Invoice], error) {
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		src, err := loadInvoice(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		if src.DocumentType != model.DocInvoice {
			return 0, conflict(CodeCorrectionOnly, "a correction can only be issued for an invoice, not a " + src.DocumentType)
		}
		body := copyInvoice(src, model.DocCorrection, true, true)
		body.RelatedID = &src.ID
		m, err := s.createInvoiceTx(ctx, tx, &body)
		if err != nil {
			return 0, err
		}
		return m.ID, nil
	})
}

// duplicateInvoice creates a new open document of the same type with the same
// subject and lines, a new number and today's date; snapshots are taken anew.
func (s *server) duplicateInvoice(ctx context.Context, in *invoiceID) (*Out[Invoice], error) {
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		src, err := loadInvoice(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		body := copyInvoice(src, src.DocumentType, false, false)
		if src.DocumentType == model.DocCorrection {
			body.RelatedID = src.RelatedID
		}
		m, err := s.createInvoiceTx(ctx, tx, &body)
		if err != nil {
			return 0, err
		}
		return m.ID, nil
	})
}

func (s *server) regeneratePublicToken(ctx context.Context, in *invoiceID) (*Out[Invoice], error) {
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := loadInvoice(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		if err := tx.Model(m).Update("public_token", newPublicToken()).Error; err != nil {
			return 0, dbErr(err, "invoice")
		}
		return m.ID, recordInvoice(ctx, tx, events.InvoicePublicLinkRegenerate, m)
	})
}

// copyInvoice builds a create body with the content of src (subject, lines,
// currency, payment, notes, flags); dates, number and related_id are left
// to the caller / defaults. copySnapshots keeps src's client_* and your_*
// data instead of snapshotting the subject and account anew; negate flips
// the sign of every quantity.
func copyInvoice(src *model.Invoice, docType string, copySnapshots, negate bool) InvoiceCreate {
	dueDays, note, footer, round := src.DueDays, src.Note, src.FooterNote, src.RoundTotal
	body := InvoiceCreate{
		DocumentType: docType, SubjectID: src.SubjectID, DueDays: &dueDays,
		Currency: src.Currency, ExchangeRate: src.ExchangeRate, Language: src.Language,
		PaymentMethod: src.PaymentMethod, CustomPaymentMethod: src.CustomPaymentMethod,
		OrderNumber: src.OrderNumber, Note: &note, FooterNote: &footer, PrivateNote: src.PrivateNote,
		Tags: src.Tags, PricesIncludeVat: src.PricesIncludeVat, RoundTotal: &round, ReverseCharge: src.ReverseCharge,
		Lines: make([]InvoiceLineInput, len(src.Lines)),
	}
	for i, l := range src.Lines {
		q, rate := l.QuantityMilli, l.VatRateBps
		if negate {
			q = -q
		}
		body.Lines[i] = InvoiceLineInput{
			PriceItemID: l.PriceItemID, Name: l.Name, Quantity: billing.FormatQuantity(q),
			UnitName: l.UnitName, UnitPrice: l.UnitPrice, VatRateBps: &rate,
		}
	}
	if copySnapshots {
		c := *src // copies, so the pointers below do not alias src
		body.InvoiceSnapshotFields = InvoiceSnapshotFields{
			ClientName: &c.ClientName, ClientFullName: &c.ClientFullName, ClientRegistrationNo: &c.ClientRegistrationNo,
			ClientVatNo: &c.ClientVatNo, ClientStreet: &c.ClientStreet, ClientCity: &c.ClientCity,
			ClientZip: &c.ClientZip, ClientCountry: &c.ClientCountry, ClientEmail: &c.ClientEmail,
			YourName: &c.YourName, YourRegistrationNo: &c.YourRegistrationNo, YourVatNo: &c.YourVatNo,
			YourStreet: &c.YourStreet, YourCity: &c.YourCity, YourZip: &c.YourZip, YourCountry: &c.YourCountry,
			YourRegisteredBy: &c.YourRegisteredBy, YourVatMode: &c.YourVatMode,
		}
	}
	return body
}
