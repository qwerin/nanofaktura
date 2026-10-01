package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/bankimport"
	"github.com/qwerin/nanofaktura/internal/cnb"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/matching"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// Bank transactions (SPEC §7.6): statements are imported from files or the
// Fio API, stored once per (bank account, external_id) and matched to open
// invoices (incoming) or unpaid expenses (outgoing) by internal/matching.
// A match creates a Payment / ExpensePayment dated booked_on with the
// transaction amount; unmatch deletes it again.

// MaxStatementSize is the upload limit of one bank statement file.
const MaxStatementSize = 10 << 20

// Transaction states (output field state and ?state= filter).
const (
	TxUnmatched = "unmatched" // not matched, not ignored (includes suggested)
	TxSuggested = "suggested" // unmatched with at least one suggestion
	TxMatched   = "matched"
	TxIgnored   = "ignored"
)

// ---- DTOs ----

// BankTransaction is one imported bank movement.
type BankTransaction struct {
	ID                  uint   `json:"id"`
	BankAccountID       uint   `json:"bank_account_id"`
	ExternalID          string `json:"external_id" doc:"Bank's movement ID (\"h:…\" hash when the source has none)"`
	BookedOn            string `json:"booked_on"`
	Amount              int64  `json:"amount" doc:"Minor units, + incoming / − outgoing"`
	Currency            string `json:"currency"`
	CounterpartyAccount string `json:"counterparty_account"`
	CounterpartyName    string `json:"counterparty_name"`
	VariableSymbol      string `json:"variable_symbol"`
	ConstantSymbol      string `json:"constant_symbol"`
	SpecificSymbol      string `json:"specific_symbol"`
	Message             string `json:"message"`

	State            string            `json:"state" enum:"unmatched,suggested,matched,ignored"`
	MatchedInvoiceID *uint             `json:"matched_invoice_id,omitempty"`
	MatchedExpenseID *uint             `json:"matched_expense_id,omitempty"`
	MatchedNumber    string            `json:"matched_number,omitempty" doc:"Number of the matched document (expense: the supplier's original number when set)"`
	MatchedName      string            `json:"matched_name,omitempty" doc:"Client / supplier of the matched document"`
	PaymentID        *uint             `json:"payment_id,omitempty" doc:"Payment (invoice) or expense payment created by the match"`
	AutoMatched      bool              `json:"auto_matched"`
	Ignored          bool              `json:"ignored"`
	Suggestions      []MatchSuggestion `json:"suggestions" nullable:"false" doc:"Best candidates (max 3) of an unmatched transaction"`

	CreatedAt time.Time `json:"created_at"`
}

// MatchSuggestion is a candidate document with the reasons it was suggested.
type MatchSuggestion struct {
	InvoiceID *uint    `json:"invoice_id,omitempty"`
	ExpenseID *uint    `json:"expense_id,omitempty"`
	Number    string   `json:"number"`
	Name      string   `json:"name" doc:"Client / supplier name"`
	Remaining int64    `json:"remaining" doc:"Remaining amount when suggested"`
	Score     int      `json:"score"`
	Reasons   []string `json:"reasons" nullable:"false" example:"VS sedí"`
}

// BankImportResult summarizes an import or sync.
type BankImportResult struct {
	Format       string     `json:"format" doc:"Detected or given statement format"`
	Imported     int        `json:"imported" doc:"New transactions"`
	Duplicates   int        `json:"duplicates" doc:"Transactions already imported before (skipped)"`
	Matched      int        `json:"matched" doc:"New transactions paired automatically"`
	Suggestions  int        `json:"suggestions" doc:"New transactions with suggestions to confirm"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty" doc:"Sync only"`

	paid []uint // invoices fully paid by the import (paid-thanks e-mails after commit)
}

// BankImportUpload is the multipart body of POST /bank-accounts/{id}/import.
type BankImportUpload struct {
	File   huma.FormFile `form:"file" required:"true" doc:"Statement file, max 10 MB"`
	Format string        `form:"format" required:"false" doc:"fio_json, gpc, fio_csv, csob_csv, kb_csv, airbank_csv; empty = detect"`
}

// BankTransactionMatch pairs a transaction with exactly one document.
type BankTransactionMatch struct {
	InvoiceID *uint `json:"invoice_id,omitempty" doc:"Invoice or proforma (typically for incoming payments)"`
	ExpenseID *uint `json:"expense_id,omitempty" doc:"Expense (typically for outgoing payments)"`
}

// RematchResult summarizes POST /bank-transactions/rematch.
type RematchResult struct {
	Processed   int `json:"processed" doc:"Unmatched, not ignored transactions examined"`
	Matched     int `json:"matched"`
	Suggestions int `json:"suggestions"`
}

func txState(m *model.BankTransaction) string {
	switch {
	case m.PaymentID != nil:
		return TxMatched
	case m.Ignored:
		return TxIgnored
	case m.SuggestionCount > 0:
		return TxSuggested
	default:
		return TxUnmatched
	}
}

func toBankTransaction(m *model.BankTransaction) BankTransaction {
	out := BankTransaction{
		ID: m.ID, BankAccountID: m.BankAccountID, ExternalID: m.ExternalID, BookedOn: m.BookedOn, Amount: m.Amount,
		Currency: m.Currency, CounterpartyAccount: m.CounterpartyAccount, CounterpartyName: m.CounterpartyName,
		VariableSymbol: m.VariableSymbol, ConstantSymbol: m.ConstantSymbol, SpecificSymbol: m.SpecificSymbol, Message: m.Message,
		State: txState(m), MatchedInvoiceID: m.MatchedInvoiceID, MatchedExpenseID: m.MatchedExpenseID, PaymentID: m.PaymentID,
		AutoMatched: m.AutoMatched, Ignored: m.Ignored, Suggestions: make([]MatchSuggestion, 0, len(m.Suggestions)),
		CreatedAt: m.CreatedAt,
	}
	for _, s := range m.Suggestions {
		reasons := s.Reasons
		if reasons == nil {
			reasons = []string{}
		}
		out.Suggestions = append(out.Suggestions, MatchSuggestion{InvoiceID: s.InvoiceID, ExpenseID: s.ExpenseID,
			Number: s.Number, Name: s.Name, Remaining: s.Remaining, Score: s.Score, Reasons: reasons})
	}
	return out
}

// ---- routes ----

func (s *server) registerBankTransactions(g huma.API) {
	huma.Post(g, "/bank-accounts/{id}/import", s.importBankStatement, auth.ForEditors, func(o *huma.Operation) {
		o.Summary = "Import a bank statement file (Fio JSON/CSV, ČSOB, KB, Air Bank CSV, ABO/GPC)"
		o.MaxBodyBytes = MaxStatementSize + uploadOverhead
		o.Middlewares = append(o.Middlewares, limitBody(g, MaxStatementSize+uploadOverhead))
	})
	huma.Post(g, "/bank-accounts/{id}/sync", s.syncBankAccountOp, auth.ForEditors, func(o *huma.Operation) {
		o.Summary = "Download new transactions from the bank API (Fio)"
		o.Errors = []int{http.StatusConflict, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusBadGateway}
	})
	huma.Get(g, "/bank-transactions", s.listBankTransactions)
	huma.Post(g, "/bank-transactions/rematch", s.rematchBankTransactions, auth.ForEditors)
	huma.Get(g, "/bank-transactions/{id}", s.getBankTransaction)
	huma.Post(g, "/bank-transactions/{id}/match", s.matchBankTransaction, auth.ForEditors)
	huma.Post(g, "/bank-transactions/{id}/unmatch", s.unmatchBankTransaction, auth.ForEditors)
	huma.Post(g, "/bank-transactions/{id}/ignore", s.ignoreBankTransaction, auth.ForEditors)
	huma.Post(g, "/bank-transactions/{id}/unignore", s.unignoreBankTransaction, auth.ForEditors)
}

type bankTxID struct {
	ID uint `path:"id"`
}

func (s *server) listBankTransactions(ctx context.Context, in *struct {
	PageParams
	BankAccountID uint   `query:"bank_account_id"`
	State         string `query:"state" enum:"unmatched,suggested,matched,ignored" doc:"unmatched includes suggested"`
	Direction     string `query:"direction" enum:"in,out"`
	Since         string `query:"since" format:"date" doc:"booked_on ≥ since"`
	Until         string `query:"until" format:"date" doc:"booked_on ≤ until"`
	Query         string `query:"query" doc:"Counterparty name/account, message or variable symbol (case-insensitive substring)"`
}) (*Out[ListResponse[BankTransaction]], error) {
	q := s.scoped(ctx).Model(&model.BankTransaction{}).Order("booked_on DESC, id DESC")
	if in.BankAccountID != 0 {
		q = q.Where("bank_account_id = ?", in.BankAccountID)
	}
	switch in.State {
	case TxMatched:
		q = q.Where("payment_id IS NOT NULL")
	case TxIgnored:
		q = q.Where("payment_id IS NULL AND ignored = ?", true)
	case TxUnmatched:
		q = q.Where("payment_id IS NULL AND ignored = ?", false)
	case TxSuggested:
		q = q.Where("payment_id IS NULL AND ignored = ? AND suggestion_count > 0", false)
	}
	switch in.Direction {
	case "in":
		q = q.Where("amount > 0")
	case "out":
		q = q.Where("amount < 0")
	}
	if in.Since != "" {
		q = q.Where("booked_on >= ?", in.Since)
	}
	if in.Until != "" {
		q = q.Where("booked_on <= ?", in.Until)
	}
	if qs := strings.TrimSpace(in.Query); qs != "" {
		like := likePattern(qs)
		q = q.Where(`(LOWER(counterparty_name) LIKE ? ESCAPE '\' OR LOWER(counterparty_account) LIKE ? ESCAPE '\'`+
			` OR LOWER(message) LIKE ? ESCAPE '\' OR variable_symbol LIKE ? ESCAPE '\')`, like, like, like, like)
	}
	return listOut(q, in.PageParams, func(ms []model.BankTransaction) ([]BankTransaction, error) {
		return bankTxsOut(ctx, s.db.WithContext(ctx), ms)
	})
}

func (s *server) getBankTransaction(ctx context.Context, in *bankTxID) (*Out[BankTransaction], error) {
	var m model.BankTransaction
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "bank transaction")
	}
	return bankTxOut(ctx, s.db.WithContext(ctx), &m)
}

// ---- import & sync ----

func (s *server) importBankStatement(ctx context.Context, in *struct {
	ID      uint `path:"id"`
	RawBody huma.MultipartFormFiles[BankImportUpload]
}) (*Out[BankImportResult], error) {
	var ba model.BankAccount
	if err := s.scoped(ctx).First(&ba, in.ID).Error; err != nil {
		return nil, dbErr(err, "bank account")
	}
	data := in.RawBody.Data()
	f := data.File
	if !f.IsSet || f.File == nil {
		return nil, invalid("file", "file is required")
	}
	defer f.Close()
	if f.Size > MaxStatementSize {
		return nil, huma.NewError(http.StatusRequestEntityTooLarge, "file is larger than 10 MB")
	}
	content, err := io.ReadAll(io.LimitReader(f, MaxStatementSize+1))
	if err != nil {
		return nil, huma.Error400BadRequest("cannot read file", err)
	}
	if len(content) == 0 {
		return nil, invalid("file", "file is empty")
	}
	format := bankimport.Format(strings.TrimSpace(data.Format))
	var st *bankimport.Statement
	switch format {
	case bankimport.FormatUnknown:
		format, st, err = bankimport.ParseAuto(content)
	case bankimport.FormatFioJSON, bankimport.FormatGPC, bankimport.FormatFioCSV, bankimport.FormatCSOBCSV,
		bankimport.FormatKBCSV, bankimport.FormatAirBankCSV:
		st, err = bankimport.Parse(format, content)
	default:
		return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
			&huma.ErrorDetail{Location: "body.format", Message: "unknown format (fio_json, gpc, fio_csv, csob_csv, kb_csv, airbank_csv)", Value: data.Format})
	}
	if err != nil {
		return nil, invalid("file", "cannot read the statement: "+err.Error())
	}
	var res BankImportResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res, err = s.storeStatement(ctx, tx, &ba, st)
		return err
	})
	if err != nil {
		return nil, err
	}
	res.Format = string(format)
	s.notifyPaid(ctx, res.paid)
	return &Out[BankImportResult]{Body: res}, nil
}

func (s *server) syncBankAccountOp(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[BankImportResult], error) {
	var ba model.BankAccount
	if err := s.scoped(ctx).First(&ba, in.ID).Error; err != nil {
		return nil, dbErr(err, "bank account")
	}
	res, err := s.syncBankAccount(ctx, &ba, s.deps.Now())
	if err != nil {
		return nil, err
	}
	return &Out[BankImportResult]{Body: res}, nil
}

// syncBankAccount downloads transactions of ba from Fio since the day before
// last_synced_at (or sync_from, or 30 days back), stores and matches them.
// The network call runs outside the DB transaction.
func (s *server) syncBankAccount(ctx context.Context, ba *model.BankAccount, now time.Time) (BankImportResult, error) {
	if ba.SyncProvider != model.SyncFio || ba.FioToken == "" {
		return BankImportResult{}, conflict(CodeSyncNotConfigured, "automatic sync is not configured for this bank account (sync_provider=fio with fio_token)")
	}
	token, err := s.deps.Secrets.Decrypt(ba.FioToken)
	if err != nil {
		return BankImportResult{}, huma.NewError(http.StatusUnprocessableEntity,
			"the stored Fio token cannot be decrypted (was NANOFAKTURA_SECRET_KEY changed?); enter the token again")
	}
	to := cnb.Today(now)
	from := ""
	switch {
	case ba.LastSyncedAt != nil:
		from = cnb.Today(ba.LastSyncedAt.AddDate(0, 0, -1)) // overlap: late bookings; duplicates are skipped
	case ba.SyncFrom != "":
		from = ba.SyncFrom
	default:
		from = cnb.Today(now.AddDate(0, 0, -30))
	}
	if from > to {
		from = to
	}
	st, err := s.deps.Fio.Periods(ctx, token, from, to)
	if err != nil {
		return BankImportResult{}, fioErr(err)
	}
	var res BankImportResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if res, err = s.storeStatement(ctx, tx, ba, st); err != nil {
			return err
		}
		ba.LastSyncedAt = &now
		return dbErrOrNil(tx.Model(ba).Update("last_synced_at", now).Error, "bank account")
	})
	if err != nil {
		return BankImportResult{}, err
	}
	res.Format = string(bankimport.FormatFioJSON)
	res.LastSyncedAt = ba.LastSyncedAt
	s.notifyPaid(ctx, res.paid)
	return res, nil
}

// fioErr maps Fio client errors to HTTP errors.
func fioErr(err error) error {
	switch {
	case errors.Is(err, bankimport.ErrFioRateLimited):
		return huma.ErrorWithHeaders(
			huma.NewError(http.StatusTooManyRequests, "Fio allows one request per 30 seconds per token; try again in 30 s"),
			http.Header{"Retry-After": {"30"}})
	case errors.Is(err, bankimport.ErrFioToken):
		return apiError(http.StatusUnprocessableEntity, CodeFioToken, "Fio rejected the API token (invalid, expired or without permission); set a new fio_token")
	case errors.Is(err, bankimport.ErrFioTooMany):
		return apiError(http.StatusUnprocessableEntity, CodeFioTooMany, "too many transactions in the period; set a later sync_from")
	default:
		return huma.NewError(http.StatusBadGateway, "Fio API is unavailable, try again later")
	}
}

// RunBankSync is the "bank-sync" scheduler job (every 2 h): it syncs every
// bank account with sync_provider=fio (all accounts of the instance), one
// after another. Fio's 30 s per-token limit is enforced by the Fio client; an
// account hitting it is skipped until the next run. Re-running is harmless
// (known external_ids are skipped). Errors of individual bank accounts are
// logged and returned joined; they do not stop the others.
func (s *server) RunBankSync(ctx context.Context, now time.Time) error {
	var bas []model.BankAccount
	if err := s.db.WithContext(ctx).Where("sync_provider = ? AND fio_token <> ''", model.SyncFio).Order("id").Find(&bas).Error; err != nil {
		return err
	}
	var errs []error
	for i := range bas {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ba := &bas[i]
		actx, err := s.systemContext(ctx, ba.AccountID)
		if err != nil {
			errs = append(errs, fmt.Errorf("bank account %d: %w", ba.ID, err))
			continue
		}
		res, err := s.syncBankAccount(actx, ba, now)
		var se huma.StatusError
		switch {
		case err == nil:
			slog.Info("bank sync", "bank_account_id", ba.ID, "imported", res.Imported, "matched", res.Matched)
		case errors.As(err, &se) && se.GetStatus() == http.StatusTooManyRequests:
			slog.Info("bank sync skipped: Fio rate limit", "bank_account_id", ba.ID)
		default:
			slog.Warn("bank sync failed", "bank_account_id", ba.ID, "err", err)
			errs = append(errs, fmt.Errorf("bank account %d: %w", ba.ID, err))
		}
	}
	return errors.Join(errs...)
}

// notifyPaid sends the (optional) paid-thanks e-mails of invoices fully paid
// by bank matching; call after the transaction committed.
func (s *server) notifyPaid(ctx context.Context, invoiceIDs []uint) {
	for _, id := range invoiceIDs {
		s.sendPaidThanks(ctx, id)
	}
}

// sameBankAccount reports whether a statement's own account (when present)
// is ba. Numbers are compared without leading zeros; the bank code only when
// both sides have one (GPC statements carry just the number).
func sameBankAccount(st *bankimport.Statement, ba *model.BankAccount) bool {
	if st.IBAN != "" && ba.IBAN != "" {
		return spayd.NormalizeIBAN(st.IBAN) == ba.IBAN
	}
	if st.Account == "" || ba.Number == "" {
		return true
	}
	split := func(a string) (string, string) {
		num, bank, _ := strings.Cut(strings.ReplaceAll(a, " ", ""), "/")
		p, n, ok := strings.Cut(num, "-")
		if !ok {
			p, n = "", num
		}
		return strings.TrimLeft(p, "0") + "-" + strings.TrimLeft(n, "0"), bank
	}
	sn, sb := split(st.Account)
	bn, bb := split(ba.Number)
	return sn == bn && (sb == "" || bb == "" || sb == bb)
}

// storeStatement inserts the new transactions of st into ba (known
// external_ids are skipped) and matches them.
func (s *server) storeStatement(ctx context.Context, tx *gorm.DB, ba *model.BankAccount, st *bankimport.Statement) (BankImportResult, error) {
	var res BankImportResult
	if !sameBankAccount(st, ba) {
		acc := st.IBAN
		if acc == "" {
			acc = st.Account
		}
		return res, invalid("file", "the statement belongs to another bank account ("+acc+")")
	}
	if err := s.storeBalance(ctx, tx, ba, st); err != nil {
		return res, err
	}
	var fresh []*model.BankTransaction
	for _, t := range st.Transactions {
		m := &model.BankTransaction{
			AccountID: ba.AccountID, BankAccountID: ba.ID, ExternalID: t.ExternalID, BookedOn: t.BookedOn,
			Amount: t.Amount, Currency: defaultStr(t.Currency, defaultStr(st.Currency, ba.Currency)),
			CounterpartyAccount: t.CounterpartyAccount, CounterpartyName: t.CounterpartyName,
			VariableSymbol: t.VS, ConstantSymbol: t.KS, SpecificSymbol: t.SS, Message: t.Message,
		}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(m)
		if r.Error != nil {
			return res, dbErr(r.Error, "bank transaction")
		}
		if r.RowsAffected == 0 {
			res.Duplicates++
			continue
		}
		res.Imported++
		fresh = append(fresh, m)
	}
	if len(fresh) == 0 {
		return res, recordBankImport(ctx, tx, ba, res)
	}
	mt, err := loadMatchCandidates(ctx, tx)
	if err != nil {
		return res, err
	}
	for _, m := range fresh {
		matched, err := s.matchTransaction(ctx, tx, mt, m, true)
		if err != nil {
			return res, err
		}
		switch {
		case matched:
			res.Matched++
		case m.SuggestionCount > 0:
			res.Suggestions++
		}
	}
	res.paid = mt.paid
	return res, recordBankImport(ctx, tx, ba, res)
}

// storeBalance keeps the closing balance of st on the bank account unless a
// newer one is already stored (statements may be imported out of order).
func (s *server) storeBalance(ctx context.Context, tx *gorm.DB, ba *model.BankAccount, st *bankimport.Statement) error {
	if st.ClosingBalance == nil {
		return nil
	}
	on := ""
	for _, t := range st.Transactions {
		on = max(on, t.BookedOn)
	}
	if on == "" {
		on = s.today()
	}
	if ba.Balance != nil && ba.BalanceOn > on {
		return nil
	}
	ba.Balance, ba.BalanceOn = st.ClosingBalance, on
	return dbErrOrNil(tx.Model(ba).Select("balance", "balance_on").Updates(ba).Error, "bank account")
}

// bankTxsOut converts transactions with the matched document's number and name.
func bankTxsOut(ctx context.Context, db *gorm.DB, ms []model.BankTransaction) ([]BankTransaction, error) {
	var invIDs, expIDs []uint
	for _, m := range ms {
		if m.MatchedInvoiceID != nil {
			invIDs = append(invIDs, *m.MatchedInvoiceID)
		}
		if m.MatchedExpenseID != nil {
			expIDs = append(expIDs, *m.MatchedExpenseID)
		}
	}
	type doc struct{ number, name string }
	invs, exps := map[uint]doc{}, map[uint]doc{}
	if len(invIDs) > 0 {
		var rows []model.Invoice
		if err := db.Scopes(inAccount(ctx)).Select("id", "number", "client_name").Where("id IN ?", invIDs).Find(&rows).Error; err != nil {
			return nil, dbErr(err, "invoices")
		}
		for _, r := range rows {
			invs[r.ID] = doc{r.Number, r.ClientName}
		}
	}
	if len(expIDs) > 0 {
		var rows []model.Expense
		if err := db.Scopes(inAccount(ctx)).Select("id", "number", "original_number", "supplier_name").Where("id IN ?", expIDs).Find(&rows).Error; err != nil {
			return nil, dbErr(err, "expenses")
		}
		for _, r := range rows {
			exps[r.ID] = doc{defaultStr(r.OriginalNumber, r.Number), r.SupplierName}
		}
	}
	out := make([]BankTransaction, len(ms))
	for i := range ms {
		out[i] = toBankTransaction(&ms[i])
		var d doc
		switch {
		case ms[i].MatchedInvoiceID != nil:
			d = invs[*ms[i].MatchedInvoiceID]
		case ms[i].MatchedExpenseID != nil:
			d = exps[*ms[i].MatchedExpenseID]
		}
		out[i].MatchedNumber, out[i].MatchedName = d.number, d.name
	}
	return out, nil
}

func bankTxOut(ctx context.Context, db *gorm.DB, m *model.BankTransaction) (*Out[BankTransaction], error) {
	out, err := bankTxsOut(ctx, db, []model.BankTransaction{*m})
	if err != nil {
		return nil, err
	}
	return &Out[BankTransaction]{Body: out[0]}, nil
}

// ---- matching ----

// matchCandidates are the unpaid documents of the account, loaded once per
// import; auto-matches reduce their remaining amounts in memory.
type matchCandidates struct {
	invoices []matching.Candidate
	expenses []matching.Candidate
	paid     []uint // invoices fully paid by auto-matches
}

func loadMatchCandidates(ctx context.Context, tx *gorm.DB) (*matchCandidates, error) {
	mt := &matchCandidates{}
	var invs []model.Invoice
	if err := tx.Scopes(inAccount(ctx)).
		Where("status IN ? AND document_type IN ? AND total > paid_amount",
			[]string{model.StatusOpen, model.StatusSent}, []string{model.DocInvoice, model.DocProforma}).
		Order("id").Find(&invs).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	subjectIDs := make([]uint, 0, len(invs))
	for _, inv := range invs {
		subjectIDs = append(subjectIDs, inv.SubjectID)
	}
	accounts := map[uint][]string{}
	if len(subjectIDs) > 0 {
		var subs []model.Subject
		if err := tx.Scopes(inAccount(ctx)).Where("id IN ?", subjectIDs).Find(&subs).Error; err != nil {
			return nil, dbErr(err, "subject")
		}
		for _, s := range subs {
			accounts[s.ID] = nonEmpty(s.BankAccount, s.IBAN)
		}
	}
	for _, inv := range invs {
		mt.invoices = append(mt.invoices, matching.Candidate{ID: inv.ID, Number: inv.Number, Currency: inv.Currency,
			VS: inv.VariableSymbol, Total: inv.Total, Remaining: inv.Total - inv.PaidAmount, Name: inv.ClientName,
			Accounts: accounts[inv.SubjectID]})
	}
	var exps []model.Expense
	if err := tx.Scopes(inAccount(ctx)).Where("status = ? AND total > paid_amount", model.StatusOpen).
		Order("id").Find(&exps).Error; err != nil {
		return nil, dbErr(err, "expense")
	}
	for _, e := range exps {
		mt.expenses = append(mt.expenses, matching.Candidate{ID: e.ID, Number: e.Number, Currency: e.Currency,
			VS: e.VariableSymbol, Total: e.Total, Remaining: e.Total - e.PaidAmount, Name: e.SupplierName,
			Accounts: nonEmpty(e.SupplierBankAccount, e.SupplierIBAN)})
	}
	return mt, nil
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// matchTransaction runs the matching engine for m (incoming → invoices,
// outgoing → expenses). With allowAuto a unique exact candidate is paid
// right away; otherwise the suggestions are stored on m.
func (s *server) matchTransaction(ctx context.Context, tx *gorm.DB, mt *matchCandidates, m *model.BankTransaction, allowAuto bool) (bool, error) {
	cands, isInvoice := mt.expenses, false
	if m.Amount > 0 {
		cands, isInvoice = mt.invoices, true
	}
	r := matching.Match(matching.Tx{Amount: m.Amount, Currency: m.Currency, VS: m.VariableSymbol,
		CounterpartyAccount: m.CounterpartyAccount, CounterpartyName: m.CounterpartyName}, cands)
	if allowAuto && r.Auto != nil {
		// re-read the transaction under lock: a concurrent rematch / sync may
		// have matched (or the user ignored) it since the candidates were loaded
		var cur model.BankTransaction
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Scopes(inAccount(ctx)).
			First(&cur, m.ID).Error; err != nil {
			return false, dbErr(err, "bank transaction")
		}
		if cur.PaymentID != nil || cur.Ignored {
			*m = cur
			return false, nil
		}
		id := r.Auto.ID
		var err error
		if isInvoice {
			err = s.linkTransaction(ctx, tx, m, &id, nil, true, &mt.paid)
		} else {
			err = s.linkTransaction(ctx, tx, m, nil, &id, true, nil)
		}
		switch {
		case errors.Is(err, errAutoStale):
			r.Auto = nil // the document changed meanwhile: keep it as a suggestion only
		case err != nil:
			return false, err
		default:
			for i := range cands {
				if cands[i].ID == id {
					cands[i].Remaining -= abs64(m.Amount)
				}
			}
			return true, nil
		}
	}
	m.Suggestions = make([]model.MatchSuggestion, 0, len(r.Suggestions))
	for _, sg := range r.Suggestions {
		id := sg.Candidate.ID
		ms := model.MatchSuggestion{Number: sg.Candidate.Number, Name: sg.Candidate.Name,
			Remaining: sg.Candidate.Remaining, Score: sg.Score, Reasons: sg.Reasons}
		if isInvoice {
			ms.InvoiceID = &id
		} else {
			ms.ExpenseID = &id
		}
		m.Suggestions = append(m.Suggestions, ms)
	}
	m.SuggestionCount = len(m.Suggestions)
	if err := tx.Save(m).Error; err != nil {
		return false, dbErr(err, "bank transaction")
	}
	return false, syncTodos(ctx, tx, events.SubjectBankTransaction, m.ID)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// errAutoStale: an auto-match candidate no longer has exactly the
// transaction's amount left to pay (it was paid concurrently).
var errAutoStale = errors.New("auto-match candidate changed")

// linkTransaction creates the payment of m on the invoice or expense and
// marks m matched. Invoice payments get the signed amount (refunds of
// corrections are negative), expense payments the opposite sign. An invoice
// fully paid by it is appended to *paid (when non-nil).
func (s *server) linkTransaction(ctx context.Context, tx *gorm.DB, m *model.BankTransaction, invoiceID, expenseID *uint, auto bool, paid *[]uint) error {
	if m.Amount == 0 {
		return conflict(CodeZeroAmount, "a zero-amount transaction cannot be matched")
	}
	note := "Platba z banky"
	if m.CounterpartyName != "" {
		note += ": " + m.CounterpartyName
	}
	if len(note) > 500 {
		note = note[:500]
	}
	switch {
	case invoiceID != nil:
		inv, err := loadInvoiceForUpdate(ctx, tx, *invoiceID)
		if err != nil {
			return refErr(err, "invoice_id", "invoice")
		}
		if inv.Status == model.StatusCancelled || inv.Status == model.StatusUncollectible {
			return conflict(CodeNotPayable, "cannot add a payment to a "+inv.Status+" invoice")
		}
		if inv.Currency != m.Currency {
			return invalid("invoice_id", "the invoice is in "+inv.Currency+", the transaction in "+m.Currency)
		}
		if auto && (inv.Total-inv.PaidAmount != m.Amount || (inv.Status != model.StatusOpen && inv.Status != model.StatusSent)) {
			return errAutoStale
		}
		p, err := addPayment(ctx, tx, inv, m.BookedOn, m.Amount, note)
		if err != nil {
			return err
		}
		m.MatchedInvoiceID, m.PaymentID = &inv.ID, &p.ID
		if paid != nil && inv.Status == model.StatusPaid {
			*paid = append(*paid, inv.ID)
		}
	case expenseID != nil:
		exp, err := loadExpenseForUpdate(ctx, tx, *expenseID)
		if err != nil {
			return refErr(err, "expense_id", "expense")
		}
		if exp.Currency != m.Currency {
			return invalid("expense_id", "the expense is in "+exp.Currency+", the transaction in "+m.Currency)
		}
		if auto && (exp.Total-exp.PaidAmount != -m.Amount || exp.Status != model.StatusOpen) {
			return errAutoStale
		}
		p, err := addExpensePayment(ctx, tx, exp, m.BookedOn, -m.Amount, note)
		if err != nil {
			return err
		}
		m.MatchedExpenseID, m.PaymentID = &exp.ID, &p.ID
	}
	m.AutoMatched, m.Ignored = auto, false
	m.Suggestions, m.SuggestionCount = nil, 0
	if err := tx.Save(m).Error; err != nil {
		return dbErr(err, "bank transaction")
	}
	return recordBankMatch(ctx, tx, events.BankMatched, m, docLabel(ctx, tx, m.MatchedInvoiceID, m.MatchedExpenseID))
}

// docLabel names the matched document for event texts ("doklad 2026-0001").
func docLabel(ctx context.Context, tx *gorm.DB, invoiceID, expenseID *uint) string {
	var number []string
	switch {
	case invoiceID != nil:
		tx.Model(&model.Invoice{}).Scopes(inAccount(ctx)).Where("id = ?", *invoiceID).Limit(1).Pluck("number", &number)
		return "doklad " + strings.Join(number, "")
	case expenseID != nil:
		tx.Model(&model.Expense{}).Scopes(inAccount(ctx)).Where("id = ?", *expenseID).Limit(1).Pluck("number", &number)
		return "náklad " + strings.Join(number, "")
	}
	return ""
}

// refErr turns a 404 of a document referenced in the body into a 422.
func refErr(err error, field, what string) error {
	var se huma.StatusError
	if errors.As(err, &se) && se.GetStatus() == http.StatusNotFound {
		return invalid(field, what+" not found")
	}
	return err
}

// lockBankTxsOfPayment locks the bank transactions matched to a payment
// before the payment's document is locked — the same order as unmatch
// (transaction, then document), so the two cannot deadlock.
func lockBankTxsOfPayment(ctx context.Context, tx *gorm.DB, paymentID uint) error {
	var ids []uint
	return dbErrOrNil(tx.Model(&model.BankTransaction{}).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Scopes(inAccount(ctx)).Where("payment_id = ?", paymentID).Pluck("id", &ids).Error, "bank transaction")
}

// unlinkBankPayment clears the match of transactions whose payment paymentID
// (of the document in column matched_invoice_id / matched_expense_id) is
// being deleted.
func unlinkBankPayment(tx *gorm.DB, column string, docID, paymentID uint) error {
	return dbErrOrNil(tx.Model(&model.BankTransaction{}).
		Where(column+" = ? AND payment_id = ?", docID, paymentID).
		Updates(map[string]any{column: nil, "payment_id": nil, "auto_matched": false}).Error, "bank transaction")
}

// ---- actions ----

// mutateBankTx loads transaction id in a transaction, runs fn and returns it.
func (s *server) mutateBankTx(ctx context.Context, id uint, fn func(tx *gorm.DB, m *model.BankTransaction) error) (*Out[BankTransaction], error) {
	var m model.BankTransaction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// lock the transaction first (then its document): concurrent match /
		// unmatch / rematch of one transaction must not create two payments
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Scopes(inAccount(ctx)).
			First(&m, id).Error; err != nil {
			return dbErr(err, "bank transaction")
		}
		return fn(tx, &m)
	})
	if err != nil {
		return nil, err
	}
	return bankTxOut(ctx, s.db.WithContext(ctx), &m)
}

func (s *server) matchBankTransaction(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body BankTransactionMatch
}) (*Out[BankTransaction], error) {
	b := in.Body
	if (b.InvoiceID == nil) == (b.ExpenseID == nil) {
		return nil, invalid("invoice_id", "send exactly one of invoice_id and expense_id")
	}
	var paid []uint
	out, err := s.mutateBankTx(ctx, in.ID, func(tx *gorm.DB, m *model.BankTransaction) error {
		if m.PaymentID != nil {
			return conflict(CodeAlreadyMatched, "the transaction is already matched; unmatch it first")
		}
		return s.linkTransaction(ctx, tx, m, b.InvoiceID, b.ExpenseID, false, &paid)
	})
	if err == nil {
		s.notifyPaid(ctx, paid)
	}
	return out, err
}

func (s *server) unmatchBankTransaction(ctx context.Context, in *bankTxID) (*Out[BankTransaction], error) {
	return s.mutateBankTx(ctx, in.ID, func(tx *gorm.DB, m *model.BankTransaction) error {
		if m.PaymentID == nil {
			return conflict(CodeNotMatched, "the transaction is not matched")
		}
		label := docLabel(ctx, tx, m.MatchedInvoiceID, m.MatchedExpenseID)
		switch {
		case m.MatchedInvoiceID != nil:
			if err := removeInvoicePayment(ctx, tx, *m.MatchedInvoiceID, *m.PaymentID); err != nil {
				return err
			}
		case m.MatchedExpenseID != nil:
			if err := removeExpensePayment(ctx, tx, *m.MatchedExpenseID, *m.PaymentID); err != nil {
				return err
			}
		}
		m.MatchedInvoiceID, m.MatchedExpenseID, m.PaymentID, m.AutoMatched = nil, nil, nil, false
		mt, err := loadMatchCandidates(ctx, tx)
		if err != nil {
			return err
		}
		if _, err = s.matchTransaction(ctx, tx, mt, m, false); err != nil { // fresh suggestions, never auto-match again
			return err
		}
		return recordBankMatch(ctx, tx, events.BankUnmatched, m, label)
	})
}

// removeInvoicePayment deletes a payment of an invoice (if it still exists)
// and recomputes the invoice's paid amount and status.
func removeInvoicePayment(ctx context.Context, tx *gorm.DB, invoiceID, paymentID uint) error {
	inv, err := loadInvoiceForUpdate(ctx, tx, invoiceID)
	if err != nil {
		return err
	}
	for _, p := range inv.Payments {
		if p.ID == paymentID {
			if err := tx.Delete(&model.Payment{}, paymentID).Error; err != nil {
				return dbErr(err, "payment")
			}
			if err := refreshInvoicePayments(tx, inv); err != nil {
				return err
			}
			return recordInvoicePayment(ctx, tx, events.PaymentDeleted, inv, &p, model.StatusPaid)
		}
	}
	return nil
}

// removeExpensePayment is removeInvoicePayment for expenses.
func removeExpensePayment(ctx context.Context, tx *gorm.DB, expenseID, paymentID uint) error {
	exp, err := loadExpenseForUpdate(ctx, tx, expenseID)
	if err != nil {
		return err
	}
	for _, p := range exp.Payments {
		if p.ID == paymentID {
			if err := tx.Delete(&model.ExpensePayment{}, paymentID).Error; err != nil {
				return dbErr(err, "payment")
			}
			if err := refreshExpensePayments(tx, exp); err != nil {
				return err
			}
			return recordExpensePayment(ctx, tx, events.ExpensePaymentDeleted, exp, &p, model.StatusPaid)
		}
	}
	return nil
}

func (s *server) ignoreBankTransaction(ctx context.Context, in *bankTxID) (*Out[BankTransaction], error) {
	return s.mutateBankTx(ctx, in.ID, func(tx *gorm.DB, m *model.BankTransaction) error {
		if m.PaymentID != nil {
			return conflict(CodeAlreadyMatched, "the transaction is matched; unmatch it first")
		}
		m.Ignored = true
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "bank transaction")
		}
		return syncTodos(ctx, tx, events.SubjectBankTransaction, m.ID)
	})
}

func (s *server) unignoreBankTransaction(ctx context.Context, in *bankTxID) (*Out[BankTransaction], error) {
	return s.mutateBankTx(ctx, in.ID, func(tx *gorm.DB, m *model.BankTransaction) error {
		m.Ignored = false
		if err := tx.Save(m).Error; err != nil {
			return dbErr(err, "bank transaction")
		}
		return syncTodos(ctx, tx, events.SubjectBankTransaction, m.ID)
	})
}

// rematchBankTransactions re-runs matching (with auto-match) for all
// unmatched, not ignored transactions of the account, oldest first.
func (s *server) rematchBankTransactions(ctx context.Context, _ *struct{}) (*Out[RematchResult], error) {
	var res RematchResult
	var paid []uint
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var txs []model.BankTransaction
		if err := tx.Scopes(inAccount(ctx)).Where("payment_id IS NULL AND ignored = ?", false).
			Order("booked_on, id").Find(&txs).Error; err != nil {
			return dbErr(err, "bank transaction")
		}
		if len(txs) == 0 {
			return nil
		}
		mt, err := loadMatchCandidates(ctx, tx)
		if err != nil {
			return err
		}
		defer func() { paid = mt.paid }()
		for i := range txs {
			res.Processed++
			matched, err := s.matchTransaction(ctx, tx, mt, &txs[i], true)
			if err != nil {
				return err
			}
			switch {
			case matched:
				res.Matched++
			case txs[i].SuggestionCount > 0:
				res.Suggestions++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notifyPaid(ctx, paid)
	return &Out[RematchResult]{Body: res}, nil
}
