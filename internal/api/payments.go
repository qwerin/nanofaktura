package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
)

type PaymentCreate struct {
	PaidOn             string `json:"paid_on,omitempty" format:"date" doc:"Default today"`
	Amount             *int64 `json:"amount,omitempty" doc:"Minor units, non-zero (negative = refund), same sign as remaining_amount; default: remaining_amount"`
	Note               string `json:"note,omitempty" maxLength:"500"`
	CreateFinalInvoice bool   `json:"create_final_invoice,omitempty" doc:"Proforma only: also issue the final invoice dated paid_on; it takes over all proforma payments (a partial payment leaves the rest due on the final invoice) and settles the proforma"`
}

// PaymentResult is the created payment, the updated invoice and, with
// create_final_invoice, the id of the new final invoice.
type PaymentResult struct {
	Payment        Payment `json:"payment"`
	Invoice        Invoice `json:"invoice"`
	FinalInvoiceID *uint   `json:"final_invoice_id,omitempty"`
	TaxDocumentID  *uint   `json:"tax_document_id,omitempty" doc:"Tax document issued for the received proforma payment (VAT payers)"`
}

func (s *server) registerPayments(g huma.API) {
	huma.Post(g, "/invoices/{id}/payments", s.createPayment, status(http.StatusCreated), auth.ForEditors)
	huma.Delete(g, "/invoices/{id}/payments/{payment_id}", s.deletePayment, status(http.StatusNoContent), auth.ForEditors)
}

func (s *server) createPayment(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body PaymentCreate
}) (*Out[PaymentResult], error) {
	paidOn := defaultStr(in.Body.PaidOn, s.today())
	if !billing.ValidDate(paidOn) {
		return nil, invalid("paid_on", "invalid date")
	}
	ctx = s.withInvoiceRate(ctx, in.ID, paidOn) // tax document / final invoice of a foreign-currency proforma
	var res PaymentResult
	wasPaid := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadInvoiceForUpdate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		wasPaid = m.Status == model.StatusPaid
		if m.Status == model.StatusCancelled || m.Status == model.StatusUncollectible {
			return conflict(CodeNotPayable, "cannot add a payment to a "+m.Status+" invoice")
		}
		if in.Body.CreateFinalInvoice && m.DocumentType != model.DocProforma {
			return invalid("create_final_invoice", "a final invoice can only be created for a proforma")
		}
		remaining := m.Total - m.PaidAmount
		amount := remaining
		if in.Body.Amount != nil {
			if amount = *in.Body.Amount; amount == 0 {
				return invalid("amount", "amount must not be zero")
			}
			if remaining != 0 && (amount > 0) != (remaining > 0) {
				return invalid("amount", "the amount must have the same sign as the remaining amount (a refund of a credit note is negative)")
			}
		} else if amount == 0 {
			return conflict(CodeNothingToPay, "nothing to pay: the remaining amount is 0")
		}

		p, err := s.addPayment(ctx, tx, m, paidOn, amount, in.Body.Note)
		if err != nil {
			return err
		}
		res.Payment, res.TaxDocumentID = toPayment(p), p.TaxDocumentID

		if in.Body.CreateFinalInvoice {
			fin, err := s.createFinalInvoice(ctx, tx, m, paidOn, paidOn, s.advanceRate(ctx, m, paidOn))
			if err != nil {
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
	// a proforma settled by a partial payment is "paid" without being paid in full
	if !wasPaid && res.Invoice.Status == model.StatusPaid && (res.FinalInvoiceID == nil || res.Invoice.RemainingAmount == 0) {
		s.sendPaidThanks(ctx, res.Invoice.ID) // best effort, after commit
	}
	return &Out[PaymentResult]{Body: res}, nil
}

func (s *server) deletePayment(ctx context.Context, in *struct {
	ID        uint `path:"id"`
	PaymentID uint `path:"payment_id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockBankTxsOfPayment(ctx, tx, in.PaymentID); err != nil {
			return err
		}
		m, err := loadInvoiceForUpdate(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		found, err := s.removePayment(ctx, tx, m, in.PaymentID)
		if err == nil && !found {
			return notFound("payment")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// addPayment stores a payment of m (locked), recomputes m's paid amount and
// status and records payment.created (+ invoice.paid when it got fully
// paid). A proforma payment of a VAT payer also issues its tax document;
// a settled proforma (with a final invoice) takes no more payments.
func (s *server) addPayment(ctx context.Context, tx *gorm.DB, m *model.Invoice, paidOn string, amount int64, note string) (*model.Payment, error) {
	if err := checkNotSettled(ctx, tx, m); err != nil {
		return nil, err
	}
	p := model.Payment{AccountID: m.AccountID, InvoiceID: m.ID, PaidOn: paidOn, Amount: amount, Note: note}
	if err := tx.Create(&p).Error; err != nil {
		return nil, dbErr(err, "payment")
	}
	if issuesTaxDocuments(m) {
		if err := s.issueTaxDocument(ctx, tx, m, &p); err != nil {
			return nil, err
		}
	}
	prev := m.Status
	if err := refreshInvoicePayments(tx, m); err != nil {
		return nil, err
	}
	return &p, recordInvoicePayment(ctx, tx, events.PaymentCreated, m, &p, prev)
}

// refreshInvoicePayments reloads m's payments (the source of truth),
// recomputes paid_amount, status and paid_on and stores only these columns,
// so a concurrent change of other fields is never overwritten. The caller
// holds the row lock (loadInvoiceForUpdate).
func refreshInvoicePayments(tx *gorm.DB, m *model.Invoice) error {
	if err := tx.Where("invoice_id = ?", m.ID).Order("paid_on, id").Find(&m.Payments).Error; err != nil {
		return dbErr(err, "payment")
	}
	applyPayments(m)
	return dbErrOrNil(tx.Model(&model.Invoice{}).Where("id = ?", m.ID).UpdateColumns(map[string]any{
		"paid_amount": m.PaidAmount, "status": m.Status, "paid_on": m.PaidOn, "updated_at": time.Now(),
	}).Error, "invoice")
}
