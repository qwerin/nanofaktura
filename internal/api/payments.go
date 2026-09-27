package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

type PaymentCreate struct {
	PaidOn             string `json:"paid_on,omitempty" format:"date" doc:"Default today"`
	Amount             *int64 `json:"amount,omitempty" doc:"Minor units, non-zero (negative = refund); default: remaining_amount"`
	Note               string `json:"note,omitempty" maxLength:"500"`
	CreateFinalInvoice bool   `json:"create_final_invoice,omitempty" doc:"Proforma only: also issue the final invoice, paid by the same payment"`
}

// PaymentResult is the created payment, the updated invoice and, with
// create_final_invoice, the id of the new final invoice.
type PaymentResult struct {
	Payment        Payment `json:"payment"`
	Invoice        Invoice `json:"invoice"`
	FinalInvoiceID *uint   `json:"final_invoice_id,omitempty"`
}

func (s *server) registerPayments(g huma.API) {
	huma.Post(g, "/invoices/{id}/payments", s.createPayment, status(http.StatusCreated), auth.ForEditors)
	huma.Delete(g, "/invoices/{id}/payments/{payment_id}", s.deletePayment, status(http.StatusNoContent), auth.ForEditors)
}

func (s *server) createPayment(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body PaymentCreate
}) (*Out[PaymentResult], error) {
	var res PaymentResult
	wasPaid := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadInvoice(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		wasPaid = m.Status == model.StatusPaid
		if m.Status == model.StatusCancelled || m.Status == model.StatusUncollectible {
			return conflict("cannot add a payment to a " + m.Status + " invoice")
		}
		if in.Body.CreateFinalInvoice && m.DocumentType != model.DocProforma {
			return invalid("create_final_invoice", "a final invoice can only be created for a proforma")
		}
		paidOn := defaultStr(in.Body.PaidOn, s.today())
		if !billing.ValidDate(paidOn) {
			return invalid("paid_on", "invalid date")
		}
		amount := m.Total - m.PaidAmount
		if in.Body.Amount != nil {
			if amount = *in.Body.Amount; amount == 0 {
				return invalid("amount", "amount must not be zero")
			}
		} else if amount == 0 {
			return conflict("nothing to pay: the remaining amount is 0")
		}

		p, err := addPayment(tx, m, paidOn, amount, in.Body.Note)
		if err != nil {
			return err
		}
		res.Payment = toPayment(p)

		if in.Body.CreateFinalInvoice {
			var n int64
			if err := tx.Model(&model.Invoice{}).Scopes(inAccount(ctx)).
				Where("related_id = ? AND document_type = ?", m.ID, model.DocInvoice).Count(&n).Error; err != nil {
				return dbErr(err, "invoice")
			}
			if n > 0 {
				return conflict("a final invoice for this proforma already exists")
			}
			body := copyInvoice(m, model.DocInvoice, true, false)
			body.RelatedID, body.IssuedOn = &m.ID, paidOn
			body.InvoiceSnapshotFields.YourVatMode = nil // the final invoice follows the current VAT mode
			fin, err := s.createInvoiceTx(ctx, tx, &body)
			if err != nil {
				return err
			}
			if _, err := addPayment(tx, fin, paidOn, amount, in.Body.Note); err != nil {
				return err
			}
			res.FinalInvoiceID = &fin.ID
		}

		out, err := s.invoiceOut(ctx, tx, m.ID)
		if err != nil {
			return err
		}
		res.Invoice = out.Body
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !wasPaid && res.Invoice.Status == model.StatusPaid {
		s.sendPaidThanks(ctx, res.Invoice.ID) // best effort, after commit
	}
	return &Out[PaymentResult]{Body: res}, nil
}

func (s *server) deletePayment(ctx context.Context, in *struct {
	ID        uint `path:"id"`
	PaymentID uint `path:"payment_id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadInvoice(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		idx := -1
		for i, p := range m.Payments {
			if p.ID == in.PaymentID {
				idx = i
			}
		}
		if idx < 0 {
			return notFound("payment")
		}
		if err := tx.Delete(&model.Payment{}, in.PaymentID).Error; err != nil {
			return dbErr(err, "payment")
		}
		if err := unlinkBankPayment(tx, "matched_invoice_id", m.ID, in.PaymentID); err != nil {
			return err
		}
		m.Payments = append(m.Payments[:idx], m.Payments[idx+1:]...)
		applyPayments(m)
		return dbErrOrNil(tx.Omit(clause.Associations).Save(m).Error, "invoice")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// addPayment stores a payment of m and recomputes m's paid amount and status.
func addPayment(tx *gorm.DB, m *model.Invoice, paidOn string, amount int64, note string) (*model.Payment, error) {
	p := model.Payment{AccountID: m.AccountID, InvoiceID: m.ID, PaidOn: paidOn, Amount: amount, Note: note}
	if err := tx.Create(&p).Error; err != nil {
		return nil, dbErr(err, "payment")
	}
	m.Payments = append(m.Payments, p)
	applyPayments(m)
	if err := tx.Omit(clause.Associations).Save(m).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	return &p, nil
}
