package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/eet"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Registration of sales — EET 2.0 (SPEC §7.19). A contact payment of an
// invoice (cash, card at the counter …) becomes an EETSale in the payment's
// transaction; it is sent to the tax authority right after the commit and,
// until confirmed, again by the "eet" job. Deleting such a payment registers
// a cancellation (negative amount).

// EETSettings is the EET setup of the account; the certificate itself is
// write-only.
type EETSettings struct {
	Enabled            bool       `json:"enabled" doc:"Contact payments are registered"`
	Environment        string     `json:"environment" enum:"production,playground" doc:"playground = the tax authority's test system (its confirmations are not valid)"`
	EIC                string     `json:"eic" doc:"Evidenční identifikační číslo: CZ + 8–10 digits (usually the DIČ)"`
	UnitID             string     `json:"unit_id" doc:"Number of the evidenční jednotka assigned in DIS+"`
	RegisterID         string     `json:"register_id" doc:"The account's own name of the cash register (id_pokl)"`
	HasCertificate     bool       `json:"has_certificate"`
	CertificateEIC     string     `json:"certificate_eic,omitempty" doc:"EIC the stored certificate was issued to"`
	CertificateValidTo *time.Time `json:"certificate_valid_to,omitempty"`
	PendingCount       int64      `json:"pending_count" doc:"Sales waiting to be sent"`
	FailedCount        int64      `json:"failed_count" doc:"Sales refused by the tax authority (fix the cause and retry)"`
}

type EETSettingsPatch struct {
	Enabled     *bool   `json:"enabled,omitempty" doc:"true needs a certificate, EIC, unit and register"`
	Environment *string `json:"environment,omitempty" enum:"production,playground"`
	EIC         *string `json:"eic,omitempty" maxLength:"12"`
	UnitID      *string `json:"unit_id,omitempty" maxLength:"9"`
	RegisterID  *string `json:"register_id,omitempty" maxLength:"20"`
}

// EETCertificateUpload carries the .p12 file from the certification
// authority (MOJE daně → DIS+ → Evidence tržeb).
type EETCertificateUpload struct {
	PKCS12   string `json:"pkcs12" maxLength:"100000" doc:"The .p12 file, base64"`
	Password string `json:"password" maxLength:"200" doc:"Password of the .p12 file"`
}

type EETWarning struct {
	Code int    `json:"code"`
	Text string `json:"text"`
}

// EETSale is one sale registered (or waiting to be registered) in EET.
type EETSale struct {
	ID            uint       `json:"id"`
	InvoiceID     uint       `json:"invoice_id"`
	InvoiceNumber string     `json:"invoice_number"`
	PaymentID     *uint      `json:"payment_id,omitempty" doc:"Missing for cancellations and once the payment was deleted"`
	CancelsID     *uint      `json:"cancels_id,omitempty" doc:"Cancellation (storno) of this sale"`
	Environment   string     `json:"environment" enum:"production,playground"`
	EIC           string     `json:"eic"`
	UnitID        string     `json:"unit_id"`
	RegisterID    string     `json:"register_id"`
	SerialNo      string     `json:"serial_no"`
	SoldAt        time.Time  `json:"sold_at"`
	Amount        int64      `json:"amount" doc:"CZK minor units; negative = cancellation"`
	Status        string     `json:"status" enum:"pending,confirmed,failed,cancelled"`
	POK           string     `json:"pok,omitempty" doc:"Confirmation code of the tax authority"`
	Attempts      int        `json:"attempts"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
	DeadlineAt    time.Time  `json:"deadline_at" doc:"Legal limit for sending: 48 hours after the sale"`
	ErrorCode     int        `json:"error_code,omitempty"`
	Error         string     `json:"error,omitempty"`
	Warnings      []string   `json:"warnings" nullable:"false"`
	TransactionID string     `json:"transaction_id,omitempty" doc:"For support requests to the tax authority"`
	CreatedAt     time.Time  `json:"created_at"`
}

// EETTestResult is the outcome of a message sent in the verification mode.
type EETTestResult struct {
	OK            bool         `json:"ok"`
	Environment   string       `json:"environment"`
	Code          int          `json:"code,omitempty" doc:"Error code of the tax authority (0 = processed)"`
	Message       string       `json:"message"`
	Warnings      []EETWarning `json:"warnings" nullable:"false"`
	TransactionID string       `json:"transaction_id,omitempty"`
}

func toEETSale(m *model.EETSale) EETSale {
	warnings := []string{}
	if m.Warnings != "" {
		warnings = strings.Split(m.Warnings, "\n")
	}
	return EETSale{ID: m.ID, InvoiceID: m.InvoiceID, InvoiceNumber: m.InvoiceNumber, PaymentID: m.PaymentID, CancelsID: m.CancelsID,
		Environment: m.Env, EIC: m.EIC, UnitID: m.UnitID, RegisterID: m.RegisterID, SerialNo: m.SerialNo, SoldAt: m.SoldAt, Amount: m.Amount,
		Status: m.Status, POK: m.POK, Attempts: m.Attempts, LastAttemptAt: m.LastAttemptAt, ConfirmedAt: m.ConfirmedAt,
		DeadlineAt: m.SoldAt.Add(eet.Deadline), ErrorCode: m.ErrorCode, Error: m.Error, Warnings: warnings,
		TransactionID: m.TransactionID, CreatedAt: m.CreatedAt}
}

func (s *server) registerEET(g huma.API) {
	huma.Get(g, "/eet", s.getEET)
	huma.Patch(g, "/eet", s.patchEET, auth.ForManagers)
	huma.Put(g, "/eet/certificate", s.putEETCertificate, auth.ForManagers)
	huma.Delete(g, "/eet/certificate", s.deleteEETCertificate, status(http.StatusNoContent), auth.ForManagers)
	huma.Post(g, "/eet/actions/test", s.testEET, auth.ForManagers)
	huma.Get(g, "/eet-sales", s.listEETSales)
	huma.Post(g, "/eet-sales/{id}/actions/retry", s.retryEETSale, auth.ForEditors)
}

// eetConfig loads the account's setup (a zero config with the defaults when
// none is stored yet).
func eetConfig(ctx context.Context, db *gorm.DB) (*model.EETConfig, error) {
	var c model.EETConfig
	if err := db.Scopes(inAccount(ctx)).Limit(1).Find(&c).Error; err != nil {
		return nil, dbErr(err, "eet settings")
	}
	if c.ID == 0 {
		c = model.EETConfig{AccountID: auth.AccountFrom(ctx).ID, Env: eet.EnvProduction, RegisterID: "1"}
	}
	return &c, nil
}

func eetReady(c *model.EETConfig) bool {
	return c.Certificate != "" && eet.ValidEnv(c.Env) && eet.ValidEIC(c.EIC) && eet.ValidUnitID(c.UnitID) && eet.ValidRegisterID(c.RegisterID)
}

func (s *server) eetOut(ctx context.Context, db *gorm.DB, c *model.EETConfig) (*Out[EETSettings], error) {
	out := EETSettings{Enabled: c.Enabled, Environment: c.Env, EIC: c.EIC, UnitID: c.UnitID, RegisterID: c.RegisterID,
		HasCertificate: c.Certificate != "", CertificateEIC: c.CertEIC, CertificateValidTo: c.CertNotAfter}
	count := func(status string, n *int64) error {
		return dbErrOrNil(db.Model(&model.EETSale{}).Scopes(inAccount(ctx)).Where("status = ?", status).Count(n).Error, "eet sale")
	}
	if err := errors.Join(count(model.EETPending, &out.PendingCount), count(model.EETFailed, &out.FailedCount)); err != nil {
		return nil, err
	}
	return &Out[EETSettings]{Body: out}, nil
}

func (s *server) getEET(ctx context.Context, _ *struct{}) (*Out[EETSettings], error) {
	db := s.db.WithContext(ctx)
	c, err := eetConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	return s.eetOut(ctx, db, c)
}

// recordEETSettings records eet.settings_updated (never the certificate).
func recordEETSettings(ctx context.Context, tx *gorm.DB, c *model.EETConfig, text string) error {
	return record(ctx, tx, events.Event{Name: events.EETSettingsUpdated, SubjectType: events.SubjectAccount, SubjectID: c.AccountID,
		Text: text, Data: map[string]any{"enabled": c.Enabled, "environment": c.Env, "eic": c.EIC, "unit_id": c.UnitID,
			"register_id": c.RegisterID, "has_certificate": c.Certificate != ""}})
}

func (s *server) patchEET(ctx context.Context, in *struct{ Body EETSettingsPatch }) (*Out[EETSettings], error) {
	b := &in.Body
	var out *Out[EETSettings]
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := eetConfig(ctx, tx)
		if err != nil {
			return err
		}
		if b.Environment != nil {
			c.Env = *b.Environment
		}
		if b.EIC != nil {
			if c.EIC = strings.ToUpper(strings.TrimSpace(*b.EIC)); c.EIC != "" && !eet.ValidEIC(c.EIC) {
				return invalid("eic", "the EIC is CZ followed by 8 to 10 digits")
			}
		}
		if b.UnitID != nil {
			if c.UnitID = strings.TrimSpace(*b.UnitID); c.UnitID != "" && !eet.ValidUnitID(c.UnitID) {
				return invalid("unit_id", "the unit number has 1 to 9 digits and does not start with 0")
			}
		}
		if b.RegisterID != nil {
			if c.RegisterID = strings.TrimSpace(*b.RegisterID); c.RegisterID != "" && !eet.ValidRegisterID(c.RegisterID) {
				return invalid("register_id", "at most 20 characters: letters without diacritics, digits, space and . , : ; / # - _")
			}
		}
		if b.Enabled != nil {
			c.Enabled = *b.Enabled
		}
		if c.Enabled && !eetReady(c) {
			if b.Enabled != nil && *b.Enabled {
				return conflict(CodeEETNotConfigured, "EET needs a certificate, an EIC, a unit number and a register name before it can be turned on")
			}
			c.Enabled = false // a required value was cleared
		}
		if err := tx.Save(c).Error; err != nil {
			return dbErr(err, "eet settings")
		}
		state := "vypnuta"
		if c.Enabled {
			state = "zapnuta"
		}
		if err := recordEETSettings(ctx, tx, c, "Nastavení evidence tržeb (EET) změněno – evidence je "+state); err != nil {
			return err
		}
		out, err = s.eetOut(ctx, tx, c)
		return err
	})
	return out, err
}

func (s *server) putEETCertificate(ctx context.Context, in *struct{ Body EETCertificateUpload }) (*Out[EETSettings], error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.Body.PKCS12))
	if err != nil || len(raw) == 0 {
		return nil, invalid("pkcs12", "expected a base64 encoded .p12 file")
	}
	cert, err := eet.ParsePKCS12(raw, in.Body.Password)
	if err != nil {
		return nil, apiError(http.StatusUnprocessableEntity, CodeEETBadCertificate, "the file is not a .p12 certificate with an RSA key, or the password is wrong")
	}
	now := s.deps.Now()
	if !cert.NotAfter().After(now) {
		return nil, apiError(http.StatusUnprocessableEntity, CodeEETCertificateExpired, "the certificate expired on "+billing.Today(cert.NotAfter()))
	}
	pemText, err := cert.MarshalPEM()
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot store the certificate", err)
	}
	enc, err := s.deps.Secrets.Encrypt(pemText)
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot store the certificate", err)
	}
	var out *Out[EETSettings]
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := eetConfig(ctx, tx)
		if err != nil {
			return err
		}
		notAfter := cert.NotAfter()
		c.Certificate, c.CertEIC, c.CertNotAfter = enc, cert.EIC(), &notAfter
		if c.EIC == "" && eet.ValidEIC(cert.EIC()) {
			c.EIC = cert.EIC()
		}
		if err := tx.Save(c).Error; err != nil {
			return dbErr(err, "eet settings")
		}
		// sales refused because of the old certificate get another chance
		if err := tx.Model(&model.EETSale{}).Scopes(inAccount(ctx)).Where("status = ?", model.EETFailed).
			Updates(map[string]any{"status": model.EETPending, "next_attempt_at": now}).Error; err != nil {
			return dbErr(err, "eet sale")
		}
		if err := recordEETSettings(ctx, tx, c, "Nahrán certifikát pro evidenci tržeb (EET), platný do "+czDate(billing.Today(notAfter))); err != nil {
			return err
		}
		out, err = s.eetOut(ctx, tx, c)
		return err
	})
	return out, err
}

func (s *server) deleteEETCertificate(ctx context.Context, _ *struct{}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := eetConfig(ctx, tx)
		if err != nil || c.ID == 0 {
			return err
		}
		c.Certificate, c.CertEIC, c.CertNotAfter, c.Enabled = "", "", nil, false
		if err := tx.Select("*").Save(c).Error; err != nil {
			return dbErr(err, "eet settings")
		}
		return recordEETSettings(ctx, tx, c, "Certifikát pro evidenci tržeb (EET) odstraněn – evidence je vypnuta")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// eetCertificate decrypts the stored certificate of c.
func (s *server) eetCertificate(c *model.EETConfig) (*eet.Certificate, error) {
	if c.Certificate == "" {
		return nil, errors.New("no certificate is stored")
	}
	pemText, err := s.deps.Secrets.Decrypt(c.Certificate)
	if err != nil {
		return nil, errors.New("the stored certificate cannot be decrypted (was NANOFAKTURA_SECRET_KEY changed?); upload it again")
	}
	return eet.ParsePEM(pemText)
}

// testEET sends a message in the verification mode: the tax authority
// checks it (format, signature, certificate) and registers nothing.
func (s *server) testEET(ctx context.Context, _ *struct{}) (*Out[EETTestResult], error) {
	if err := s.rateLimit(s.limits.eetTest, accountKey(ctx)); err != nil {
		return nil, err
	}
	c, err := eetConfig(ctx, s.db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if !eetReady(c) {
		return nil, conflict(CodeEETNotConfigured, "upload a certificate and fill in the EIC, the unit number and the register name first")
	}
	out := EETTestResult{Environment: c.Env, Warnings: []EETWarning{}}
	cert, err := s.eetCertificate(c)
	if err != nil {
		out.Message = err.Error()
		return &Out[EETTestResult]{Body: out}, nil
	}
	now := s.deps.Now()
	sctx, cancel := context.WithTimeout(ctx, eet.Timeout)
	defer cancel()
	res, err := s.deps.EET.Send(sctx, c.Env, cert, eet.Message{UUID: uuid.NewString(), SentAt: now, First: true, Verify: true,
		Sale: eet.Sale{EIC: c.EIC, UnitID: c.UnitID, RegisterID: c.RegisterID, SerialNo: "TEST", SoldAt: now, Amount: 100}})
	if err != nil {
		out.Message = clipText(err.Error(), 300)
		return &Out[EETTestResult]{Body: out}, nil
	}
	out.OK, out.Code, out.Message, out.TransactionID = res.Verified, res.ErrCode, res.ErrText, res.TransactionID
	for _, w := range res.Warnings {
		out.Warnings = append(out.Warnings, EETWarning{Code: w.Code, Text: w.Text})
	}
	return &Out[EETTestResult]{Body: out}, nil
}

func (s *server) listEETSales(ctx context.Context, in *struct {
	PageParams
	Status    string `query:"status" enum:"pending,confirmed,failed,cancelled"`
	InvoiceID uint   `query:"invoice_id"`
}) (*Out[ListResponse[EETSale]], error) {
	q := s.scoped(ctx).Order("id DESC")
	if in.Status != "" {
		q = q.Where("status = ?", in.Status)
	}
	if in.InvoiceID != 0 {
		q = q.Where("invoice_id = ?", in.InvoiceID)
	}
	return paginate(q, in.PageParams, toEETSale)
}

// retryEETSale puts a refused sale back into the queue and sends it at once.
func (s *server) retryEETSale(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[EETSale], error) {
	now := s.deps.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model.EETSale
		if err := tx.Scopes(inAccount(ctx)).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&m, in.ID).Error; err != nil {
			return dbErr(err, "eet sale")
		}
		if m.Status != model.EETFailed && m.Status != model.EETPending {
			return conflict(CodeEETNotRetryable, "only a waiting or refused sale can be sent again")
		}
		return dbErrOrNil(tx.Model(&m).Updates(map[string]any{"status": model.EETPending, "next_attempt_at": now}).Error, "eet sale")
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.attemptEETSale(ctx, in.ID, now, true, eetInlineTimeout); err != nil {
		logJobErr("eet: retry failed", "sale_id", in.ID, "err", err)
	}
	var m model.EETSale
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "eet sale")
	}
	return &Out[EETSale]{Body: toEETSale(&m)}, nil
}

// invoiceEETSales lists the sales of an invoice for its detail.
func invoiceEETSales(ctx context.Context, db *gorm.DB, invoiceID uint) ([]EETSale, error) {
	var rows []model.EETSale
	if err := db.Scopes(inAccount(ctx)).Where("invoice_id = ?", invoiceID).Order("id").Find(&rows).Error; err != nil {
		return nil, dbErr(err, "eet sale")
	}
	out := make([]EETSale, len(rows))
	for i := range rows {
		out[i] = toEETSale(&rows[i])
	}
	return out, nil
}

// eetSoldAt is the moment of a sale paid on paidOn: now for today's payment,
// noon (Czech time) of that day for a payment entered later.
func eetSoldAt(paidOn string, now time.Time) time.Time {
	if paidOn == billing.Today(now) {
		return now
	}
	d, err := time.ParseInLocation("2006-01-02", paidOn, eet.Location())
	if err != nil {
		return now
	}
	return d.Add(12 * time.Hour)
}

// registerEETSale queues the sale of payment p of m (locked) when it is a
// contact payment: want says so explicitly; nil = the account registers and
// the invoice is paid in cash. Returns nil when nothing is to be registered.
func (s *server) registerEETSale(ctx context.Context, tx *gorm.DB, m *model.Invoice, p *model.Payment, want *bool) (*model.EETSale, error) {
	if want != nil && !*want {
		return nil, nil
	}
	c, err := eetConfig(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !c.Enabled || !eetReady(c) {
		if want != nil {
			return nil, conflict(CodeEETNotEnabled, "registration of sales (EET) is not turned on for this account")
		}
		return nil, nil
	}
	if want == nil && m.PaymentMethod != "cash" {
		return nil, nil
	}
	now := nowFrom(ctx)
	if p.PaidOn > billing.Today(now) {
		return nil, invalid("paid_on", "a sale registered in EET cannot be dated in the future")
	}
	amount, ok := toCZK(p.Amount, m.Currency, m.ExchangeRate)
	if !ok || amount > eet.MaxAmount || amount < -eet.MaxAmount {
		return nil, invalid("amount", "the amount cannot be registered in EET (invalid exchange rate or 100 million CZK and more)")
	}
	sale := model.EETSale{AccountID: m.AccountID, InvoiceID: m.ID, InvoiceNumber: m.Number, PaymentID: &p.ID, Env: c.Env, EIC: c.EIC,
		UnitID: c.UnitID, RegisterID: c.RegisterID, SoldAt: eetSoldAt(p.PaidOn, now), Amount: amount,
		Status: model.EETPending, NextAttemptAt: &now, CreatedAt: now}
	return &sale, createEETSale(tx, &sale)
}

// createEETSale stores a new sale; its serial number is its ID.
func createEETSale(tx *gorm.DB, sale *model.EETSale) error {
	if err := tx.Create(sale).Error; err != nil {
		return dbErr(err, "eet sale")
	}
	sale.SerialNo = strconv.FormatUint(uint64(sale.ID), 10)
	return dbErrOrNil(tx.Model(sale).UpdateColumn("serial_no", sale.SerialNo).Error, "eet sale")
}

// cancelEETSale handles the deletion of payment paymentID in tx: a sale
// that reached (or may have reached) the tax authority gets a cancellation
// with the opposite amount; a sale never sent or refused is just dropped.
func cancelEETSale(ctx context.Context, tx *gorm.DB, paymentID uint) error {
	var sale model.EETSale
	if err := tx.Scopes(inAccount(ctx)).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("payment_id = ?", paymentID).Limit(1).Find(&sale).Error; err != nil {
		return dbErr(err, "eet sale")
	}
	if sale.ID == 0 {
		return nil
	}
	if sale.Status == model.EETFailed || (sale.Status == model.EETPending && sale.Attempts == 0) {
		return dbErrOrNil(tx.Model(&sale).Select("payment_id", "status", "next_attempt_at").
			Updates(map[string]any{"payment_id": nil, "status": model.EETCancelled, "next_attempt_at": nil}).Error, "eet sale")
	}
	if err := tx.Model(&sale).Select("payment_id").Updates(map[string]any{"payment_id": nil}).Error; err != nil {
		return dbErr(err, "eet sale")
	}
	return createEETStorno(ctx, tx, &sale)
}

// createEETStorno queues the cancellation of sale: the opposite amount at
// the current time, not linked to the original on the tax authority's side.
func createEETStorno(ctx context.Context, tx *gorm.DB, sale *model.EETSale) error {
	now := nowFrom(ctx)
	storno := model.EETSale{AccountID: sale.AccountID, InvoiceID: sale.InvoiceID, InvoiceNumber: sale.InvoiceNumber, CancelsID: &sale.ID,
		Env: sale.Env, EIC: sale.EIC, UnitID: sale.UnitID, RegisterID: sale.RegisterID, SoldAt: now, Amount: -sale.Amount,
		Status: model.EETPending, NextAttemptAt: &now, CreatedAt: now}
	return createEETSale(tx, &storno)
}

// eetInlineTimeout bounds the attempt made inside a request (the user is
// waiting; the job retries with the full eet.Timeout).
const eetInlineTimeout = 5 * time.Second

// eetLease keeps a claimed sale away from the job while its request is in flight.
const eetLease = 3 * eet.Timeout

// eetRetryDelays: waits before the 2nd, 3rd … attempt; the last one repeats
// (the duty to send a sale never lapses).
var eetRetryDelays = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour}

const eetBatch = 50

// RunEET is the scheduler job (every tick): sends the waiting sales whose
// next attempt has come, oldest first.
func (s *server) RunEET(ctx context.Context, now time.Time) error {
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&model.EETSale{}).Where("status = ? AND next_attempt_at <= ?", model.EETPending, now).
		Order("id").Limit(eetBatch).Pluck("id", &ids).Error; err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if _, err := s.attemptEETSale(ctx, id, now, false, eet.Timeout); err != nil {
			errs = append(errs, fmt.Errorf("eet sale %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// sendEETSales sends the waiting sales of an invoice right after the commit
// of the request that queued them (best effort; the job takes over).
func (s *server) sendEETSales(ctx context.Context, invoiceID uint) {
	now := s.deps.Now()
	var ids []uint
	if err := s.scoped(ctx).Model(&model.EETSale{}).Where("invoice_id = ? AND status = ? AND next_attempt_at <= ?", invoiceID, model.EETPending, now).
		Order("id").Pluck("id", &ids).Error; err != nil {
		logJobErr("eet: cannot list the sales to send", "invoice_id", invoiceID, "err", err)
		return
	}
	for _, id := range ids {
		if _, err := s.attemptEETSale(ctx, id, now, false, eetInlineTimeout); err != nil {
			logJobErr("eet: sending failed", "sale_id", id, "err", err)
		}
	}
}

// attemptEETSale claims the waiting sale id (due, or any waiting one with
// force), sends it outside a transaction and stores the outcome: confirmed
// (POK); refused → failed (eet.failed, a todo); no answer or a temporary
// error → the next attempt. A resend keeps the data and gets a new UUID,
// sending time and first=false. Returns the updated sale (nil when it was
// not claimable).
func (s *server) attemptEETSale(ctx context.Context, id uint, now time.Time, force bool, timeout time.Duration) (*model.EETSale, error) {
	var sale model.EETSale
	var cfg model.EETConfig
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&sale, id).Error; err != nil {
			return err
		}
		if sale.Status != model.EETPending || (!force && (sale.NextAttemptAt == nil || sale.NextAttemptAt.After(now))) {
			return nil
		}
		if err := tx.Where("account_id = ?", sale.AccountID).Limit(1).Find(&cfg).Error; err != nil {
			return err
		}
		claimed = true
		return tx.Model(&sale).Update("next_attempt_at", now.Add(eetLease)).Error
	})
	if err != nil || !claimed {
		return nil, err
	}

	var res eet.Result
	cert, sendErr := s.eetCertificate(&cfg)
	noCert := sendErr != nil
	if !noCert {
		sctx, cancel := context.WithTimeout(ctx, timeout)
		res, sendErr = s.deps.EET.Send(sctx, sale.Env, cert, eet.Message{UUID: uuid.NewString(), SentAt: now, First: sale.Attempts == 0,
			Sale: eet.Sale{EIC: sale.EIC, UnitID: sale.UnitID, RegisterID: sale.RegisterID, SerialNo: sale.SerialNo, SoldAt: sale.SoldAt, Amount: sale.Amount}})
		cancel()
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&sale, id).Error; err != nil {
			return err
		}
		// the payment may have been deleted while the request was in flight
		dropped := sale.Status == model.EETCancelled
		if sale.Status != model.EETPending && !(dropped && res.Confirmed()) {
			return nil
		}
		actx := ctx
		if acc := auth.AccountFrom(ctx); acc == nil || acc.ID != sale.AccountID {
			var err error
			if actx, err = s.systemContextTx(ctx, tx, sale.AccountID, now); err != nil {
				return err
			}
		}
		if !noCert {
			sale.Attempts++
			sale.LastAttemptAt, sale.TransactionID = &now, res.TransactionID
		}
		var event string
		switch {
		case res.Confirmed():
			var warnings []string
			for _, w := range res.Warnings {
				warnings = append(warnings, fmt.Sprintf("%d: %s", w.Code, w.Text))
			}
			sale.Status, sale.POK, sale.ConfirmedAt, sale.NextAttemptAt = model.EETConfirmed, res.POK, &now, nil
			sale.ErrorCode, sale.Error, sale.Warnings = 0, "", strings.Join(warnings, "\n")
			event = events.EETRegistered
		case noCert:
			sale.Status, sale.NextAttemptAt, sale.ErrorCode, sale.Error = model.EETFailed, nil, 0, sendErr.Error()
			event = events.EETFailed
		case sendErr == nil && res.Rejected():
			sale.Status, sale.NextAttemptAt, sale.ErrorCode, sale.Error = model.EETFailed, nil, res.ErrCode, res.ErrText
			event = events.EETFailed
		default: // no answer, or "send it again later"
			delay := eetRetryDelays[min(sale.Attempts, len(eetRetryDelays))-1]
			next := now.Add(delay)
			sale.NextAttemptAt, sale.ErrorCode, sale.Error = &next, res.ErrCode, res.ErrText
			if sendErr != nil {
				sale.Error = clipText(sendErr.Error(), 300)
			}
		}
		if err := tx.Select("*").Save(&sale).Error; err != nil {
			return err
		}
		if dropped {
			if err := createEETStorno(actx, tx, &sale); err != nil {
				return err
			}
		}
		if event == "" {
			return nil
		}
		return recordEETSale(actx, tx, event, &sale)
	})
	return &sale, err
}

// recordEETSale records eet.registered / eet.failed on the sale's invoice
// (which also re-evaluates the invoice's todos).
func recordEETSale(ctx context.Context, tx *gorm.DB, name string, sale *model.EETSale) error {
	what := "Tržba"
	if sale.CancelsID != nil {
		what = "Storno tržby"
	}
	text := fmt.Sprintf("%s %s k dokladu %s zaevidována v EET", what, money(sale.Amount, "CZK"), sale.InvoiceNumber)
	if name == events.EETFailed {
		text = fmt.Sprintf("%s %s k dokladu %s se nepodařilo zaevidovat v EET: %s", what, money(sale.Amount, "CZK"), sale.InvoiceNumber, sale.Error)
	}
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectInvoice, SubjectID: sale.InvoiceID, Text: text,
		Data: map[string]any{"sale_id": sale.ID, "invoice_id": sale.InvoiceID, "invoice_number": sale.InvoiceNumber, "amount": sale.Amount,
			"environment": sale.Env, "status": sale.Status, "pok": sale.POK, "error_code": sale.ErrorCode, "error": sale.Error,
			"cancellation": sale.CancelsID != nil}})
}

// syncEETTodo keeps the automatic todo "a sale of this invoice was refused".
func syncEETTodo(ctx context.Context, tx *gorm.DB, invoiceID uint) error {
	var failed model.EETSale
	if err := tx.Scopes(inAccount(ctx)).Where("invoice_id = ? AND status = ?", invoiceID, model.EETFailed).
		Order("id").Limit(1).Find(&failed).Error; err != nil {
		return dbErr(err, "eet sale")
	}
	var want *todoWant
	if failed.ID != 0 {
		want = &todoWant{text: fmt.Sprintf("EET: tržbu %s k dokladu %s se nepodařilo zaevidovat (%s). Opravte příčinu a odešlete ji znovu.",
			money(failed.Amount, "CZK"), failed.InvoiceNumber, failed.Error)}
	}
	_, err := upsertTodo(ctx, tx, model.TodoEETFailed, events.SubjectInvoice, invoiceID, want, false)
	return err
}

func clipText(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
