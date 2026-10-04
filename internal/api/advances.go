package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/bankimport"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/numbering"
)

// Advances (zálohy, SPEC §4.5 "Zálohy"):
//
//   - a payment of a proforma is the money received; for a VAT payer every
//     proforma payment (not reverse charge) gets a tax document for the
//     received payment (document_type tax_document, § 28 ZDPH): own number
//     series, issued and DUZP = payment date, VAT computed from the received
//     amount split over the proforma's rates; it is "paid" by a payment
//     mirrored from the proforma payment (source_payment_id) and is never a
//     receivable;
//   - the final invoice (vyúčtování) has the proforma's lines, takes over all
//     proforma payments as mirrored payments (so it shows the remaining
//     amount) and settles the proforma (status paid, no further payments or
//     changes); its VAT is reduced by the proforma's tax documents (deposits);
//   - deleting a proforma payment deletes its tax document; mirrored payments
//     cannot be deleted on their own; deleting a final invoice that has only
//     mirrored payments reopens the proforma.

// issuesTaxDocuments reports whether payments of m get a tax document.
func issuesTaxDocuments(m *model.Invoice) bool {
	return m.DocumentType == model.DocProforma && m.YourVatMode == model.VatModePayer && !m.ReverseCharge
}

// finalInvoiceOf returns the final invoice of proforma id, nil when none.
func finalInvoiceOf(ctx context.Context, tx *gorm.DB, id uint) (*model.Invoice, error) {
	var f model.Invoice
	res := tx.Scopes(inAccount(ctx)).Select("id", "number", "issued_on").
		Where("related_id = ? AND document_type = ?", id, model.DocInvoice).Order("id").Limit(1).Find(&f)
	if res.Error != nil {
		return nil, dbErr(res.Error, "invoice")
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return &f, nil
}

// checkNotSettled answers 409 when proforma m already has its final invoice.
func checkNotSettled(ctx context.Context, tx *gorm.DB, m *model.Invoice) error {
	if m.DocumentType != model.DocProforma {
		return nil
	}
	f, err := finalInvoiceOf(ctx, tx, m.ID)
	if err != nil {
		return err
	}
	if f != nil {
		return conflict(CodeProformaSettled, "the proforma is settled by the final invoice "+f.Number+"; record payments and changes there")
	}
	return nil
}

var advanceLineNames = map[string]string{
	"cs": "Přijatá platba k zálohové faktuře č. %s",
	"sk": "Prijatá platba k zálohovej faktúre č. %s",
	"en": "Payment received for proforma invoice no. %s",
	"de": "Erhaltene Zahlung zur Vorauszahlungsrechnung Nr. %s",
}

// advanceLines splits a received amount over the proforma's VAT rates in
// the proportion of their totals. The lines are VAT-inclusive, so the tax
// document computes base and VAT from the received amount (§ 37 odst. 2).
func advanceLines(pro *model.Invoice, amount int64) ([]InvoiceLineInput, error) {
	opts := billingOptions(pro)
	opts.RoundTotal = false
	t, err := billing.Calculate(billingLines(pro.Lines), opts)
	if err != nil {
		return nil, invalid("lines", "amounts are out of range")
	}
	name := strings.Replace(defaultStr(advanceLineNames[pro.Language], advanceLineNames["cs"]), "%s", pro.Number, 1)
	var sum int64
	for _, r := range t.VatRecap {
		sum += r.Total
	}
	if sum == 0 || len(t.VatRecap) == 0 {
		rate := int32(0)
		if len(pro.Lines) > 0 {
			rate = pro.Lines[0].VatRateBps
		}
		return []InvoiceLineInput{{Name: name, Quantity: "1", UnitPrice: amount, VatRateBps: &rate}}, nil
	}
	out := []InvoiceLineInput{}
	rest := amount
	for i, r := range t.VatRecap {
		share := rest
		if i < len(t.VatRecap)-1 {
			if share, _ = billing.MulDivRound(amount, r.Total, sum); share == 0 {
				continue
			}
		}
		rest -= share
		rate := r.VatRateBps
		out = append(out, InvoiceLineInput{Name: name, Quantity: "1", UnitPrice: share, VatRateBps: &rate})
	}
	return out, nil
}

// issueTaxDocument creates the tax document for payment p of proforma pro
// (locked), pays it by a payment mirrored from p and links p to it.
func (s *server) issueTaxDocument(ctx context.Context, tx *gorm.DB, pro *model.Invoice, p *model.Payment) error {
	lines, err := advanceLines(pro, p.Amount)
	if err != nil {
		return err
	}
	if err := ensureNumberFormat(ctx, tx, model.DocTaxDocument); err != nil {
		return err
	}
	body := copyInvoice(pro, model.DocTaxDocument, true, false)
	zero, empty, noRound, paidOn := 0, "", false, p.PaidOn
	body.Lines, body.PricesIncludeVat, body.RoundTotal, body.DueDays = lines, true, &noRound, &zero
	body.RelatedID, body.IssuedOn, body.TaxableFulfillmentDue, body.Note = &pro.ID, p.PaidOn, &paidOn, &empty
	body.ExchangeRate = s.advanceRate(ctx, pro, p.PaidOn)
	td, err := s.createInvoiceTx(ctx, tx, &body)
	if err != nil {
		return err
	}
	mirror := model.Payment{AccountID: pro.AccountID, InvoiceID: td.ID, PaidOn: p.PaidOn, Amount: td.Total,
		Note: p.Note, SourcePaymentID: &p.ID}
	if err := tx.Create(&mirror).Error; err != nil {
		return dbErr(err, "payment")
	}
	if err := refreshInvoicePayments(tx, td); err != nil {
		return err
	}
	p.TaxDocumentID = &td.ID
	return dbErrOrNil(tx.Model(&model.Payment{}).Where("id = ?", p.ID).UpdateColumn("tax_document_id", td.ID).Error, "payment")
}

// ensureNumberFormat creates the default number format of docType when the
// account has none (accounts created before the type existed).
func ensureNumberFormat(ctx context.Context, tx *gorm.DB, docType string) error {
	acc := auth.AccountFrom(ctx)
	_, err := numbering.DefaultFormat(tx, acc.ID, docType)
	if !errors.Is(err, numbering.ErrNoFormat) {
		return dbErrOrNil(err, "number format")
	}
	for _, f := range defaultNumberFormats {
		if f.DocumentType == docType {
			f.AccountID = acc.ID
			return dbErrOrNil(tx.Create(&f).Error, "number format")
		}
	}
	return nil
}

// createFinalInvoice issues the final invoice of proforma pro (locked, with
// payments): the proforma's lines, issued on issuedOn with DUZP duzp (VAT
// payers) at exchange rate rate, all proforma payments taken over, and the
// proforma settled.
func (s *server) createFinalInvoice(ctx context.Context, tx *gorm.DB, pro *model.Invoice, issuedOn, duzp, rate string) (*model.Invoice, error) {
	if pro.DocumentType != model.DocProforma {
		return nil, invalid("create_final_invoice", "a final invoice can only be created for a proforma")
	}
	if err := notDraft(pro); err != nil {
		return nil, err
	}
	if pro.Status == model.StatusCancelled || pro.Status == model.StatusUncollectible {
		return nil, conflict(CodeInvalidTransition, "a "+pro.Status+" proforma cannot be settled")
	}
	f, err := finalInvoiceOf(ctx, tx, pro.ID)
	if err != nil {
		return nil, err
	}
	if f != nil {
		return nil, conflict(CodeFinalExists, "a final invoice for this proforma already exists")
	}
	body := copyInvoice(pro, model.DocInvoice, true, false)
	body.RelatedID, body.IssuedOn, body.ExchangeRate = &pro.ID, issuedOn, rate
	if duzp != "" {
		body.TaxableFulfillmentDue = &duzp
	}
	body.InvoiceSnapshotFields.YourVatMode = nil // the final invoice follows the current VAT mode
	fin, err := s.createInvoiceTx(ctx, tx, &body)
	if err != nil {
		return nil, err
	}
	for _, p := range pro.Payments {
		if p.SourcePaymentID != nil {
			continue
		}
		mirror := model.Payment{AccountID: pro.AccountID, InvoiceID: fin.ID, PaidOn: p.PaidOn, Amount: p.Amount,
			Note: p.Note, SourcePaymentID: &p.ID}
		if err := tx.Create(&mirror).Error; err != nil {
			return nil, dbErr(err, "payment")
		}
	}
	prev := fin.Status
	if err := refreshInvoicePayments(tx, fin); err != nil {
		return nil, err
	}
	if prev != model.StatusPaid && fin.Status == model.StatusPaid {
		if err := recordInvoice(ctx, tx, events.InvoicePaid, fin); err != nil {
			return nil, err
		}
	}
	// settled: the receivable moved to the final invoice
	paidOn := pro.PaidOn
	if pro.Status != model.StatusPaid {
		paidOn = fin.IssuedOn
	}
	if err := tx.Model(&model.Invoice{}).Where("id = ?", pro.ID).
		UpdateColumns(map[string]any{"status": model.StatusPaid, "paid_on": paidOn}).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	pro.Status, pro.PaidOn = model.StatusPaid, paidOn
	return fin, recordInvoice(ctx, tx, events.InvoiceUpdated, pro)
}

// removePayment deletes payment paymentID of invoice inv (locked) with the
// rules above (mirrored payments and settled proformas → 409; a proforma
// payment takes its tax document along) and recomputes inv. It returns
// false when the payment does not belong to inv.
func (s *server) removePayment(ctx context.Context, tx *gorm.DB, inv *model.Invoice, paymentID uint) (bool, error) {
	var p *model.Payment
	for i := range inv.Payments {
		if inv.Payments[i].ID == paymentID {
			p = &inv.Payments[i]
		}
	}
	if p == nil {
		return false, nil
	}
	if p.SourcePaymentID != nil {
		return true, conflict(CodeAdvancePayment, "the payment is taken over from the proforma; delete it on the proforma")
	}
	if err := checkNotSettled(ctx, tx, inv); err != nil {
		return true, err
	}
	pay := *p
	if err := tx.Delete(&model.Payment{}, paymentID).Error; err != nil {
		return true, dbErr(err, "payment")
	}
	if err := unlinkBankPayment(tx, "matched_invoice_id", inv.ID, paymentID); err != nil {
		return true, err
	}
	if pay.TaxDocumentID != nil {
		if err := deleteTaxDocument(ctx, tx, *pay.TaxDocumentID); err != nil {
			return true, err
		}
	}
	if err := refreshInvoicePayments(tx, inv); err != nil {
		return true, err
	}
	return true, recordInvoicePayment(ctx, tx, events.PaymentDeleted, inv, &pay, model.StatusPaid)
}

// deleteTaxDocument deletes the tax document of a deleted proforma payment.
func deleteTaxDocument(ctx context.Context, tx *gorm.DB, id uint) error {
	td, err := loadInvoiceForUpdate(ctx, tx, id)
	if err != nil {
		var se huma.StatusError
		if errors.As(err, &se) && se.GetStatus() == http.StatusNotFound {
			return nil // deleted already
		}
		return err
	}
	if td.LockedAt != nil {
		return conflict(CodeLocked, "the tax document "+td.Number+" of this payment is locked; unlock it first")
	}
	var ref model.Invoice
	res := tx.Scopes(inAccount(ctx)).Select("id", "number").Where("related_id = ?", td.ID).Limit(1).Find(&ref)
	if res.Error != nil {
		return dbErr(res.Error, "invoice")
	}
	if res.RowsAffected > 0 {
		return conflict(CodeReferenced, "the tax document "+td.Number+" is referenced by "+ref.Number+"; delete that document first")
	}
	for _, q := range []*gorm.DB{
		tx.Where("invoice_id = ?", td.ID).Delete(&model.Payment{}),
		tx.Where("invoice_id = ?", td.ID).Delete(&model.InvoiceLine{}),
		tx.Delete(&model.Invoice{}, td.ID),
	} {
		if q.Error != nil {
			return dbErr(q.Error, "invoice")
		}
	}
	return recordInvoice(ctx, tx, events.InvoiceDeleted, td)
}

// ---- deposits (tax documents deducted on a final invoice) ----

// depositsOf returns the tax documents of proforma proformaID (not
// cancelled), loaded with lines, ordered by DUZP.
func depositsOf(ctx context.Context, db *gorm.DB, proformaIDs []uint) (map[uint][]model.Invoice, error) {
	out := map[uint][]model.Invoice{}
	if len(proformaIDs) == 0 {
		return out, nil
	}
	var tds []model.Invoice
	if err := db.Scopes(inAccount(ctx)).Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("position, id") }).
		Where("document_type = ? AND related_id IN ? AND status <> ?", model.DocTaxDocument, proformaIDs, model.StatusCancelled).
		Order("taxable_fulfillment_due, id").Find(&tds).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	for _, td := range tds {
		out[*td.RelatedID] = append(out[*td.RelatedID], td)
	}
	return out, nil
}

// invoiceDeposits are the deposits of final invoice m (empty for other documents).
func invoiceDeposits(ctx context.Context, db *gorm.DB, m *model.Invoice) ([]model.Invoice, error) {
	if m.DocumentType != model.DocInvoice || m.RelatedID == nil {
		return nil, nil
	}
	var rel model.Invoice
	res := db.Scopes(inAccount(ctx)).Select("id", "document_type").Where("id = ?", *m.RelatedID).Limit(1).Find(&rel)
	if res.Error != nil {
		return nil, dbErr(res.Error, "invoice")
	}
	if res.RowsAffected == 0 || rel.DocumentType != model.DocProforma {
		return nil, nil
	}
	d, err := depositsOf(ctx, db, []uint{rel.ID})
	if err != nil {
		return nil, err
	}
	return d[rel.ID], nil
}

func toDeposits(tds []model.Invoice) []Deposit {
	out := []Deposit{}
	for i := range tds {
		td := &tds[i]
		d := Deposit{TaxDocumentID: td.ID, Number: td.Number, TaxPointDate: td.TaxableFulfillmentDue, Total: td.Total,
			VatRecap: []VatRecapItem{}}
		if t, err := billing.Calculate(billingLines(td.Lines), billingOptions(td)); err == nil {
			for _, r := range t.VatRecap {
				d.VatRecap = append(d.VatRecap, VatRecapItem{VatRateBps: r.VatRateBps, Base: r.Base, Vat: r.Vat, Total: r.Total})
			}
		}
		out = append(out, d)
	}
	return out
}

// relatedDocuments lists the documents whose related_id is id.
func relatedDocuments(ctx context.Context, db *gorm.DB, id uint) ([]RelatedDocument, error) {
	var rows []model.Invoice
	if err := db.Scopes(inAccount(ctx)).Select("id", "document_type", "number", "status", "total").
		Where("related_id = ?", id).Order("issued_on, id").Find(&rows).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	out := make([]RelatedDocument, len(rows))
	for i, r := range rows {
		out[i] = RelatedDocument{ID: r.ID, DocumentType: r.DocumentType, Number: r.Number, Status: r.Status, Total: r.Total}
	}
	return out, nil
}

// ---- POST /invoices/{id}/final-invoice ----

// FinalInvoiceCreate are the dates of a final invoice issued separately
// from a payment (typically at the delivery, after advance payments).
type FinalInvoiceCreate struct {
	IssuedOn              string `json:"issued_on,omitempty" format:"date" doc:"Default today"`
	TaxableFulfillmentDue string `json:"taxable_fulfillment_due,omitempty" format:"date" doc:"DUZP (date of the delivery) for VAT payers; default issued_on"`
}

func (s *server) registerAdvances(g huma.API) {
	huma.Post(g, "/invoices/{id}/final-invoice", s.createFinalInvoiceOp, status(http.StatusCreated), auth.ForEditors)
}

func (s *server) createFinalInvoiceOp(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body *FinalInvoiceCreate
}) (*Out[Invoice], error) {
	b := FinalInvoiceCreate{}
	if in.Body != nil {
		b = *in.Body
	}
	issuedOn := defaultStr(b.IssuedOn, s.today())
	duzp := defaultStr(b.TaxableFulfillmentDue, issuedOn)
	var pro model.Invoice
	if err := s.scoped(ctx).Select("id", "currency").First(&pro, in.ID).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	rate, err := s.defaultExchangeRate(ctx, pro.Currency, "", duzp)
	if err != nil {
		return nil, err
	}
	return s.mutateInvoice(ctx, func(tx *gorm.DB) (uint, error) {
		m, err := loadInvoiceForUpdate(ctx, tx, in.ID)
		if err != nil {
			return 0, err
		}
		if m.DocumentType != model.DocProforma {
			return 0, conflict(CodeInvalidTransition, "a final invoice can only be created for a proforma")
		}
		fin, err := s.createFinalInvoice(ctx, tx, m, issuedOn, duzp, defaultStr(rate, m.ExchangeRate))
		if err != nil {
			return 0, err
		}
		return fin.ID, nil
	})
}

// ---- exchange rates for documents created inside a transaction ----

// The ČNB client caches rates through its own DB handle, which must not be
// used inside a transaction (SQLite has one connection). Rates needed by
// documents created inside a transaction are fetched before it and handed
// over in the context.

type fxKey struct{ currency, date string }

type fxCtxKey struct{}

// withRates fetches the ČNB rates of the (currency, date) pairs that need
// one and returns a context carrying them. Unavailable rates are skipped
// (the caller falls back to the document's rate).
func (s *server) withRates(ctx context.Context, pairs ...fxKey) context.Context {
	acc := auth.AccountFrom(ctx)
	if acc == nil || acc.DefaultCurrency != "CZK" {
		return ctx
	}
	m := map[fxKey]string{}
	if prev, ok := ctx.Value(fxCtxKey{}).(map[fxKey]string); ok {
		for k, v := range prev {
			m[k] = v
		}
	}
	for _, p := range pairs {
		if p.currency == "" || p.currency == "CZK" || !billing.ValidDate(p.date) {
			continue
		}
		if _, ok := m[p]; ok {
			continue
		}
		if r, _, err := s.deps.CNB.Rate(ctx, p.currency, p.date); err == nil {
			m[p] = r
		}
	}
	return context.WithValue(ctx, fxCtxKey{}, m)
}

// advanceRate is the exchange rate of a tax document / final invoice of pro
// dated date: the prefetched ČNB rate, else the proforma's rate.
func (s *server) advanceRate(ctx context.Context, pro *model.Invoice, date string) string {
	if pro.Currency == "" || pro.Currency == "CZK" {
		return pro.ExchangeRate
	}
	if m, ok := ctx.Value(fxCtxKey{}).(map[fxKey]string); ok {
		if r, ok := m[fxKey{pro.Currency, date}]; ok {
			return r
		}
	}
	return pro.ExchangeRate
}

// withStatementRates prefetches the rates of the incoming foreign-currency
// transactions of a statement of bank account ba (matched proforma payments
// issue tax documents dated the booking day).
func (s *server) withStatementRates(ctx context.Context, ba *model.BankAccount, st *bankimport.Statement) context.Context {
	var pairs []fxKey
	for _, t := range st.Transactions {
		if t.Amount > 0 {
			pairs = append(pairs, fxKey{defaultStr(t.Currency, defaultStr(st.Currency, ba.Currency)), t.BookedOn})
		}
	}
	return s.withRates(ctx, pairs...)
}

// withUnmatchedRates prefetches the rates of the unmatched incoming
// foreign-currency transactions of the account (rematch).
func (s *server) withUnmatchedRates(ctx context.Context) context.Context {
	var rows []model.BankTransaction
	if err := s.scoped(ctx).Select("currency", "booked_on").Distinct("currency", "booked_on").
		Where("payment_id IS NULL AND ignored = ? AND amount > 0 AND currency <> ?", false, "CZK").Find(&rows).Error; err != nil {
		return ctx
	}
	pairs := make([]fxKey, len(rows))
	for i, r := range rows {
		pairs[i] = fxKey{r.Currency, r.BookedOn}
	}
	return s.withRates(ctx, pairs...)
}

// withInvoiceRate prefetches the rate of invoice id's currency on date when
// the invoice is a proforma (its payments may issue tax documents).
func (s *server) withInvoiceRate(ctx context.Context, id uint, date string) context.Context {
	var m model.Invoice
	if err := s.scoped(ctx).Select("id", "document_type", "currency").Where("id = ?", id).Limit(1).Find(&m).Error; err != nil ||
		m.DocumentType != model.DocProforma {
		return ctx
	}
	return s.withRates(ctx, fxKey{m.Currency, date})
}
