package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

// MarkPaidRequest marks every unpaid document matching the list filter as
// fully paid (typically old documents entered after the fact).
type MarkPaidRequest struct {
	PaidOn string `json:"paid_on,omitempty" format:"date" doc:"Payment date of all documents; default: each document's due date (issue date without one), at most today"`
	DryRun bool   `json:"dry_run,omitempty" doc:"Only count the documents that would be marked"`
}

// MarkPaidResult counts the documents marked (or, with dry_run, to be
// marked); sum_remaining is the amount paid per currency.
type MarkPaidResult struct {
	Count int64         `json:"count"`
	Sums  []CurrencySum `json:"sums" nullable:"false"`
}

func (s *server) registerMarkPaid(g huma.API) {
	huma.Post(g, "/invoices/mark-paid", s.markInvoicesPaid, auth.ForEditors)
	huma.Post(g, "/expenses/mark-paid", s.markExpensesPaid, auth.ForEditors)
}

// bulkPaidOn is the payment date of one document: the requested date, else
// the due date (issue date without one), never in the future.
func bulkPaidOn(requested, dueOn, issuedOn, today string) string {
	if requested != "" {
		return requested
	}
	d := defaultStr(dueOn, issuedOn)
	if d == "" || d > today {
		return today
	}
	return d
}

// markPaid runs the shared part: eligible(tx) is the filtered query of the
// documents to pay, pay(tx, id) adds the remaining amount to one of them.
// The paid-thanks e-mail is deliberately not sent (old documents).
func (s *server) markPaid(ctx context.Context, body MarkPaidRequest, eligible func(tx *gorm.DB) *gorm.DB,
	pay func(tx *gorm.DB, id uint) error) (*Out[MarkPaidResult], error) {
	if body.PaidOn != "" && !billing.ValidDate(body.PaidOn) {
		return nil, invalid("paid_on", "invalid date")
	}
	res := MarkPaidResult{Sums: []CurrencySum{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sums, err := currencySums(eligible(tx))
		if err != nil {
			return err
		}
		res.Sums = sums
		for _, c := range sums {
			res.Count += c.Count
		}
		if body.DryRun {
			return nil
		}
		var ids []uint
		if err := eligible(tx).Order("id").Pluck("id", &ids).Error; err != nil {
			return dbErr(err, "document")
		}
		for _, id := range ids {
			if err := pay(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[MarkPaidResult]{Body: res}, nil
}

func (s *server) markInvoicesPaid(ctx context.Context, in *struct {
	InvoiceFilter
	Body MarkPaidRequest
}) (*Out[MarkPaidResult], error) {
	today := s.today()
	return s.markPaid(ctx, in.Body,
		func(tx *gorm.DB) *gorm.DB {
			return in.InvoiceFilter.where(tx.Scopes(inAccount(ctx)), today).
				Where("status IN ? AND total <> paid_amount", []string{model.StatusOpen, model.StatusSent})
		},
		func(tx *gorm.DB, id uint) error {
			m, err := loadInvoiceForUpdate(ctx, tx, id)
			if err != nil {
				return err
			}
			if (m.Status != model.StatusOpen && m.Status != model.StatusSent) || m.Total == m.PaidAmount {
				return nil // paid or closed concurrently since the ids were selected
			}
			_, err = addPayment(ctx, tx, m, bulkPaidOn(in.Body.PaidOn, m.DueOn, m.IssuedOn, today), m.Total-m.PaidAmount, "")
			return err
		})
}

func (s *server) markExpensesPaid(ctx context.Context, in *struct {
	ExpenseFilter
	Body MarkPaidRequest
}) (*Out[MarkPaidResult], error) {
	today := s.today()
	return s.markPaid(ctx, in.Body,
		func(tx *gorm.DB) *gorm.DB {
			return in.ExpenseFilter.where(tx.Scopes(inAccount(ctx)), today).
				Where("status = ? AND total <> paid_amount", model.StatusOpen)
		},
		func(tx *gorm.DB, id uint) error {
			m, err := loadExpenseForUpdate(ctx, tx, id)
			if err != nil {
				return err
			}
			if m.Status != model.StatusOpen || m.Total == m.PaidAmount {
				return nil
			}
			_, err = addExpensePayment(ctx, tx, m, bulkPaidOn(in.Body.PaidOn, m.DueOn, m.IssuedOn, today), m.Total-m.PaidAmount, "")
			return err
		})
}
