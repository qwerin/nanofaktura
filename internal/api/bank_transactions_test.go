package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/bankimport"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/secret"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// fakeFio records Periods calls and returns st / err.
type fakeFio struct {
	mu    sync.Mutex
	st    *bankimport.Statement
	err   error
	calls []string
}

func (f *fakeFio) Periods(_ context.Context, token, from, to string) (*bankimport.Statement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, token+" "+from+" "+to)
	if f.err != nil {
		return nil, f.err
	}
	return f.st, nil
}
func (f *fakeFio) Last(context.Context, string) (*bankimport.Statement, error) { return f.st, f.err }
func (f *fakeFio) SetLastDate(context.Context, string, string) error           { return nil }

// withBank rebuilds ts's API with a fake Fio client and returns a function
// running the "bank-sync" scheduler job with the same dependencies.
func withBank(ts *testServer, fio bankimport.Fio) func() error {
	cfg := config.Config{AllowSignup: true}
	deps := api.Deps{
		Now: func() time.Time { return ts.now }, Mailer: ts.mail, Storage: storage.NewLocal(ts.dataDir),
		CNB: &fakeRates{}, VatRegistry: fakeVatReg{}, Fio: fio, Secrets: secret.NewRandom(),
	}
	ts.handler, _ = api.New(ts.db, cfg, deps)
	return func() error {
		for _, j := range api.Jobs(ts.db, cfg, deps) {
			if j.Name == "bank-sync" {
				if j.Every != 2*time.Hour {
					ts.t.Fatalf("bank-sync every %v", j.Every)
				}
				return j.Run(context.Background(), ts.now)
			}
		}
		ts.t.Fatal("no bank-sync job")
		return nil
	}
}

func fixtureFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../bankimport/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fioTx is a transaction for fioJSON.
type fioTx struct {
	id, date, vs, account, bank, name string
	amount                            string // decimal, e.g. "121.00"
}

// fioJSON builds a Fio transactions.json of account 2000000000/2010.
func fioJSON(txs ...fioTx) []byte {
	rows := []map[string]any{}
	for _, t := range txs {
		v := func(x any) map[string]any { return map[string]any{"value": x} }
		row := map[string]any{"column22": v(t.id), "column0": v(t.date + "+0200"), "column1": json.Number(t.amount),
			"column14": v("CZK"), "column5": v(t.vs), "column2": v(t.account), "column3": v(t.bank), "column10": v(t.name)}
		row["column1"] = v(json.Number(t.amount))
		rows = append(rows, row)
	}
	b, _ := json.Marshal(map[string]any{"accountStatement": map[string]any{
		"info":            map[string]any{"accountId": "2000000000", "bankId": "2010", "currency": "CZK", "iban": "CZ7920100000002000000000"},
		"transactionList": map[string]any{"transaction": rows},
	}})
	return b
}

func (c *client) importStatement(bankID uint, format string, data []byte) (*http.Response, []byte) {
	c.ts.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if format != "" {
		_ = mw.WriteField("format", format)
	}
	fw, _ := mw.CreateFormFile("file", "vypis")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", fmt.Sprintf("%s/%d/import", c.acct("/bank-accounts"), bankID), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func importOK(c *client, bankID uint, data []byte) api.BankImportResult {
	c.ts.t.Helper()
	res, body := c.importStatement(bankID, "", data)
	if res.StatusCode != http.StatusOK {
		c.ts.t.Fatalf("import: %d %s", res.StatusCode, body)
	}
	return decodeJSON[api.BankImportResult](c.ts.t, body)
}

func importSummary(r api.BankImportResult) string {
	return fmt.Sprintf("%s i%d d%d m%d s%d", r.Format, r.Imported, r.Duplicates, r.Matched, r.Suggestions)
}

func txURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/bank-transactions"), id, suffix)
}

func listTx(c *client, query string) api.ListResponse[api.BankTransaction] {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.BankTransaction]](c, http.StatusOK, "GET", c.acct("/bank-transactions")+query, nil)
}

func txByExternal(c *client, ext string) api.BankTransaction {
	c.ts.t.Helper()
	for _, tx := range listTx(c, "?per_page=200").Items {
		if tx.ExternalID == ext {
			return tx
		}
	}
	c.ts.t.Fatalf("transaction %s not found", ext)
	return api.BankTransaction{}
}

// fioBank creates a bank account of c and gives it the (synthetic, not
// mod-11 valid) number of the Fio fixtures directly in the DB.
func fioBank(c *client) api.BankAccount {
	c.ts.t.Helper()
	ba := doJSON[api.BankAccount](c, http.StatusCreated, "POST", c.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "19-2000145399/0800"})
	setFixtureAccount(c.ts, ba.ID)
	return ba
}

func setFixtureAccount(ts *testServer, id uint) {
	ts.t.Helper()
	if err := ts.db.Model(&model.BankAccount{}).Where("id = ?", id).
		Updates(map[string]any{"number": "2000000000/2010", "iban": "CZ7920100000002000000000"}).Error; err != nil {
		ts.t.Fatal(err)
	}
}

func TestBankImportAndMatching(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	bank := fioBank(a)

	subj := newSubject(a, api.SubjectCreate{Name: "Žluťoučký kůň s.r.o.", BankAccount: "123456789/0100"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("2026001"),
		Lines: []api.InvoiceLineInput{line("Web", "1", 1210000, nil)}})
	supplier := "Jan Novák"
	exp := createExp(a, api.ExpenseCreate{VariableSymbol: strPtr("1234"), ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &supplier},
		Lines: []api.InvoiceLineInput{line("Nájem", "1", 250050, i32(0))}})

	data := fixtureFile(t, "fio_transactions.json")
	res := importOK(a, bank.ID, data)
	if importSummary(res) != "fio_json i4 d0 m2 s0" {
		t.Fatalf("import: %+v", res)
	}
	if got := getInv(a, inv.ID); got.Status != "paid" || got.PaidOn != "2026-09-02" || len(got.Payments) != 1 || got.Payments[0].Amount != 1210000 {
		t.Fatalf("invoice: %+v", got)
	}
	if got := getExp(a, exp.ID); got.Status != "paid" || got.PaidOn != "2026-09-10" || got.Payments[0].Amount != 250050 {
		t.Fatalf("expense: %+v", got)
	}
	tx1 := txByExternal(a, "26543210001")
	if tx1.State != "matched" || !tx1.AutoMatched || tx1.MatchedInvoiceID == nil || *tx1.MatchedInvoiceID != inv.ID ||
		tx1.PaymentID == nil || tx1.Amount != 1210000 || tx1.CounterpartyAccount != "123456789/0100" || tx1.VariableSymbol != "2026001" {
		t.Fatalf("tx1: %+v", tx1)
	}
	if tx3 := txByExternal(a, "26543210003"); tx3.MatchedExpenseID == nil || *tx3.MatchedExpenseID != exp.ID {
		t.Fatalf("tx3: %+v", tx3)
	}

	// re-import: everything is a duplicate, nothing paid twice
	if res := importOK(a, bank.ID, data); importSummary(res) != "fio_json i0 d4 m0 s0" {
		t.Fatalf("reimport: %+v", res)
	}
	if got := getInv(a, inv.ID); len(got.Payments) != 1 {
		t.Fatalf("double payment: %+v", got.Payments)
	}

	// filters
	for q, want := range map[string]int64{
		"": 4, "?state=matched": 2, "?state=unmatched": 2, "?state=suggested": 0, "?state=ignored": 0,
		"?direction=in": 2, "?direction=out": 2, "?query=acme": 1, "?query=2026001": 1,
		"?since=2026-09-10": 2, "?until=2026-09-03": 2, fmt.Sprintf("?bank_account_id=%d", bank.ID): 4, "?bank_account_id=999": 0,
	} {
		if l := listTx(a, q); l.Total != want {
			t.Errorf("list %q: %d, want %d", q, l.Total, want)
		}
	}
	if l := listTx(a, ""); l.Items[0].BookedOn != "2026-09-25" || l.Items[0].Suggestions == nil {
		t.Fatalf("order: %+v", l.Items[0])
	}

	// unmatch → payment deleted, invoice open again, suggestion offered (no auto re-match)
	un := doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, tx1.ID, "/unmatch"), nil)
	if un.State != "suggested" || un.PaymentID != nil || un.MatchedInvoiceID != nil || len(un.Suggestions) != 1 ||
		*un.Suggestions[0].InvoiceID != inv.ID || strings.Join(un.Suggestions[0].Reasons, ",") != "VS sedí,Částka sedí,Účet protistrany sedí,Jméno protistrany sedí" {
		t.Fatalf("unmatch: %+v", un)
	}
	if got := getInv(a, inv.ID); got.Status != "open" || got.PaidOn != "" || len(got.Payments) != 0 {
		t.Fatalf("invoice after unmatch: %+v", got)
	}
	r, body := a.do("POST", txURL(a, tx1.ID, "/unmatch"), nil)
	assertError(t, r, body, http.StatusConflict, "not matched")
	if got := doJSON[api.BankTransaction](a, http.StatusOK, "GET", txURL(a, tx1.ID, ""), nil); got.State != "suggested" {
		t.Fatalf("detail: %+v", got)
	}

	// manual match
	m := doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, tx1.ID, "/match"), api.BankTransactionMatch{InvoiceID: &inv.ID})
	if m.State != "matched" || m.AutoMatched || len(m.Suggestions) != 0 {
		t.Fatalf("match: %+v", m)
	}
	if got := getInv(a, inv.ID); got.Status != "paid" {
		t.Fatalf("invoice after match: %+v", got)
	}
	r, body = a.do("POST", txURL(a, tx1.ID, "/match"), api.BankTransactionMatch{InvoiceID: &inv.ID})
	assertError(t, r, body, http.StatusConflict, "already matched")
	r, body = a.do("POST", txURL(a, tx1.ID, "/ignore"), nil)
	assertError(t, r, body, http.StatusConflict, "unmatch it first")

	// deleting the payment directly unlinks the transaction
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv.ID, fmt.Sprintf("/payments/%d", *m.PaymentID)), nil)
	if got := doJSON[api.BankTransaction](a, http.StatusOK, "GET", txURL(a, tx1.ID, ""), nil); got.State != "unmatched" || got.PaymentID != nil {
		t.Fatalf("after payment delete: %+v", got)
	}
	// rematch pays it automatically again
	rm := doJSON[api.RematchResult](a, http.StatusOK, "POST", a.acct("/bank-transactions/rematch"), nil)
	if rm.Processed != 3 || rm.Matched != 1 {
		t.Fatalf("rematch: %+v", rm)
	}
	if got := getInv(a, inv.ID); got.Status != "paid" {
		t.Fatalf("invoice after rematch: %+v", got)
	}

	// ignore / unignore
	card := txByExternal(a, "26543210002")
	ig := doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, card.ID, "/ignore"), nil)
	if ig.State != "ignored" || listTx(a, "?state=ignored").Total != 1 || listTx(a, "?state=unmatched").Total != 1 {
		t.Fatalf("ignore: %+v", ig)
	}
	if got := doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, card.ID, "/unignore"), nil); got.State != "unmatched" || got.Ignored {
		t.Fatalf("unignore: %+v", got)
	}

	// validation
	r, body = a.do("POST", txURL(a, card.ID, "/match"), api.BankTransactionMatch{})
	assertError(t, r, body, http.StatusUnprocessableEntity, "exactly one")
	r, body = a.do("POST", txURL(a, card.ID, "/match"), api.BankTransactionMatch{InvoiceID: &inv.ID, ExpenseID: &exp.ID})
	assertError(t, r, body, http.StatusUnprocessableEntity, "exactly one")
	missing := uint(99999)
	r, body = a.do("POST", txURL(a, card.ID, "/match"), api.BankTransactionMatch{ExpenseID: &missing})
	assertError(t, r, body, http.StatusUnprocessableEntity, "expense not found")
	r, body = a.importStatement(bank.ID, "", fixtureFile(t, "vypis.gpc"))
	assertError(t, r, body, http.StatusUnprocessableEntity, "belongs to another bank account")
	r, body = a.importStatement(bank.ID, "pdf", data)
	assertError(t, r, body, http.StatusUnprocessableEntity, "body.format")
	r, body = a.importStatement(bank.ID, "", []byte("hello world"))
	assertError(t, r, body, http.StatusUnprocessableEntity, "cannot read the statement")
	r, body = a.importStatement(bank.ID, "gpc", data)
	assertError(t, r, body, http.StatusUnprocessableEntity, "cannot read the statement")

	// tenant isolation
	r, body = b.do("GET", txURL(b, tx1.ID, ""), nil)
	assertError(t, r, body, http.StatusNotFound, "bank transaction not found")
	r, body = b.do("POST", txURL(b, tx1.ID, "/unmatch"), nil)
	assertError(t, r, body, http.StatusNotFound, "bank transaction not found")
	if l := listTx(b, ""); l.Total != 0 {
		t.Fatalf("b sees %d transactions", l.Total)
	}
	r, body = b.importStatement(bank.ID, "", data)
	assertError(t, r, body, http.StatusNotFound, "bank account not found")
	bBank := fioBank(b)
	importOK(b, bBank.ID, data) // same external IDs in another account are independent
	bTx := txByExternal(b, "26543210002")
	r, body = b.do("POST", txURL(b, bTx.ID, "/match"), api.BankTransactionMatch{ExpenseID: &exp.ID})
	assertError(t, r, body, http.StatusUnprocessableEntity, "expense not found")
	if got := getInv(a, inv.ID); len(got.Payments) != 1 {
		t.Fatalf("b's import touched a's invoice: %+v", got.Payments)
	}

	// deleting the bank account removes its transactions
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", a.acct("/bank-accounts"), bank.ID), nil)
	if l := listTx(a, ""); l.Total != 0 {
		t.Fatalf("after bank delete: %d", l.Total)
	}
}

func TestBankPartialPaymentsAndSuggestions(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	bank := fioBank(a)
	subj := newSubject(a, api.SubjectCreate{Name: "ACME s.r.o."})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("55"), Lines: []api.InvoiceLineInput{line("X", "1", 10000, nil)}})
	// two open invoices with the same amount and no VS match → suggestions only
	i2 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("61"), Lines: []api.InvoiceLineInput{line("Y", "1", 7700, nil)}})
	i3 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("62"), Lines: []api.InvoiceLineInput{line("Y", "1", 7700, nil)}})

	res := importOK(a, bank.ID, fioJSON(
		fioTx{id: "1", date: "2026-03-10", vs: "55", amount: "40.00", name: "ACME"}, // partial: suggestion
		fioTx{id: "2", date: "2026-03-11", vs: "", amount: "77.00", name: "Někdo"},  // two candidates by amount
	))
	if res.Imported != 2 || res.Matched != 0 || res.Suggestions != 2 {
		t.Fatalf("import: %+v", res)
	}
	partial := txByExternal(a, "1")
	if partial.State != "suggested" || len(partial.Suggestions) != 1 || *partial.Suggestions[0].InvoiceID != inv.ID ||
		strings.Join(partial.Suggestions[0].Reasons, ",") != "VS sedí,Částečná úhrada,Jméno protistrany sedí" {
		t.Fatalf("partial: %+v", partial)
	}
	amb := txByExternal(a, "2")
	if len(amb.Suggestions) != 2 || *amb.Suggestions[0].InvoiceID != i2.ID || *amb.Suggestions[1].InvoiceID != i3.ID {
		t.Fatalf("ambiguous: %+v", amb)
	}

	// confirm the partial payment: amount = transaction amount, not remaining
	doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, partial.ID, "/match"), api.BankTransactionMatch{InvoiceID: &inv.ID})
	got := getInv(a, inv.ID)
	if got.Status != "open" || got.PaidAmount != 4000 || got.RemainingAmount != 6000 || got.Payments[0].PaidOn != "2026-03-10" {
		t.Fatalf("after partial: %+v", got)
	}
	// the rest arrives with the VS → auto-matched (remaining matches)
	res = importOK(a, bank.ID, fioJSON(fioTx{id: "3", date: "2026-03-20", vs: "0055", amount: "60.00"}))
	if res.Matched != 1 {
		t.Fatalf("rest: %+v", res)
	}
	if got := getInv(a, inv.ID); got.Status != "paid" || got.PaidOn != "2026-03-20" || len(got.Payments) != 2 {
		t.Fatalf("after rest: %+v", got)
	}
	// same VS twice in one import: the second no longer matches the paid invoice
	i4 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("70"), Lines: []api.InvoiceLineInput{line("Z", "1", 500, nil)}})
	res = importOK(a, bank.ID, fioJSON(fioTx{id: "4", date: "2026-03-21", vs: "70", amount: "5.00"}, fioTx{id: "5", date: "2026-03-21", vs: "70", amount: "5.00"}))
	if res.Matched != 1 || res.Imported != 2 {
		t.Fatalf("twice: %+v", res)
	}
	if got := getInv(a, i4.ID); got.PaidAmount != 500 {
		t.Fatalf("i4: %+v", got)
	}

	// cancelled invoice cannot be matched
	i5 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Z", "1", 100, nil)}})
	action(a, i5.ID, "cancel")
	r, body := a.do("POST", txURL(a, amb.ID, "/match"), api.BankTransactionMatch{InvoiceID: &i5.ID})
	assertError(t, r, body, http.StatusConflict, "cancelled invoice")
	// other currency
	eur := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", Lines: []api.InvoiceLineInput{line("Z", "1", 100, nil)}})
	r, body = a.do("POST", txURL(a, amb.ID, "/match"), api.BankTransactionMatch{InvoiceID: &eur.ID})
	assertError(t, r, body, http.StatusUnprocessableEntity, "is in EUR")
}

func TestBankSync(t *testing.T) {
	ts := newTestServer(t)
	fio := &fakeFio{}
	withBank(ts, fio)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	token := strings.Repeat("a1B2", 16)

	// sync settings: token required for fio, validated, write-only
	r, body := a.do("POST", a.acct("/bank-accounts"), map[string]any{"name": "Fio", "number": "19-2000145399/0800", "sync_provider": "fio"})
	assertError(t, r, body, http.StatusUnprocessableEntity, "body.fio_token")
	r, body = a.do("POST", a.acct("/bank-accounts"), map[string]any{"name": "Fio", "number": "19-2000145399/0800", "sync_provider": "fio", "fio_token": "bad token!"})
	assertError(t, r, body, http.StatusUnprocessableEntity, "invalid Fio API token")
	bank := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		map[string]any{"name": "Fio", "number": "19-2000145399/0800", "sync_provider": "fio", "fio_token": token, "sync_from": "2026-03-01"})
	setFixtureAccount(ts, bank.ID)
	raw := a.mustDo(http.StatusOK, "GET", fmt.Sprintf("%s/%d", a.acct("/bank-accounts"), bank.ID), nil)
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), `"fio_token"`) ||
		!bank.HasFioToken || bank.SyncProvider != "fio" || bank.SyncFrom != "2026-03-01" || bank.LastSyncedAt != nil {
		t.Fatalf("bank account: %+v %s", bank, raw)
	}
	var stored model.BankAccount
	ts.db.First(&stored, bank.ID)
	if stored.FioToken == "" || strings.Contains(stored.FioToken, token) {
		t.Fatalf("token not encrypted: %q", stored.FioToken)
	}

	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("2026001"), Lines: []api.InvoiceLineInput{line("Web", "1", 1210000, nil)}})
	st, err := bankimport.ParseFioJSON(fixtureFile(t, "fio_transactions.json"))
	if err != nil {
		t.Fatal(err)
	}
	fio.st = st

	res := doJSON[api.BankImportResult](a, http.StatusOK, "POST", fmt.Sprintf("%s/%d/sync", a.acct("/bank-accounts"), bank.ID), nil)
	if res.Imported != 4 || res.Matched != 1 || res.LastSyncedAt == nil || !res.LastSyncedAt.Equal(ts.now) {
		t.Fatalf("sync: %+v", res)
	}
	if fio.calls[0] != token+" 2026-03-01 2026-03-15" {
		t.Fatalf("fio call: %v", fio.calls)
	}
	if got := getInv(a, inv.ID); got.Status != "paid" {
		t.Fatalf("invoice: %+v", got)
	}

	// next sync starts the day before the last one; duplicates skipped
	ts.now = ts.now.Add(48 * time.Hour)
	res = doJSON[api.BankImportResult](a, http.StatusOK, "POST", fmt.Sprintf("%s/%d/sync", a.acct("/bank-accounts"), bank.ID), nil)
	if res.Imported != 0 || res.Duplicates != 4 || fio.calls[1] != token+" 2026-03-14 2026-03-17" {
		t.Fatalf("second sync: %+v %v", res, fio.calls)
	}

	// Fio errors
	syncURL := fmt.Sprintf("%s/%d/sync", a.acct("/bank-accounts"), bank.ID)
	fio.err = fmt.Errorf("%w (retry in 12s)", bankimport.ErrFioRateLimited)
	r, body = a.do("POST", syncURL, nil)
	assertError(t, r, body, http.StatusTooManyRequests, "30 seconds")
	if r.Header.Get("Retry-After") != "30" {
		t.Fatalf("Retry-After: %q", r.Header.Get("Retry-After"))
	}
	fio.err = bankimport.ErrFioToken
	r, body = a.do("POST", syncURL, nil)
	assertError(t, r, body, http.StatusUnprocessableEntity, "Fio rejected the API token")
	fio.err = errors.New("dial tcp: timeout")
	r, body = a.do("POST", syncURL, nil)
	assertError(t, r, body, http.StatusBadGateway, "Fio API is unavailable")
	fio.err = nil

	// not configured → 409; other tenant → 404; accountant → 403
	plain := fioBank(b)
	r, body = b.do("POST", fmt.Sprintf("%s/%d/sync", b.acct("/bank-accounts"), plain.ID), nil)
	assertError(t, r, body, http.StatusConflict, "not configured")
	r, body = b.do("POST", fmt.Sprintf("%s/%d/sync", b.acct("/bank-accounts"), bank.ID), nil)
	assertError(t, r, body, http.StatusNotFound, "bank account not found")
	acct := ts.memberOf(a, "acc@example.cz", "accountant")
	r, body = acct.do("POST", syncURL, nil)
	assertError(t, r, body, http.StatusForbidden, "your role (accountant)")

	// PATCH: keep token when not sent, "" removes it (then fio is invalid)
	p := doJSON[api.BankAccount](a, http.StatusOK, "PATCH", fmt.Sprintf("%s/%d", a.acct("/bank-accounts"), bank.ID), map[string]any{"name": "Fio hlavní", "number": "19-2000145399/0800"})
	if !p.HasFioToken {
		t.Fatalf("patch lost token: %+v", p)
	}
	r, body = a.do("PATCH", fmt.Sprintf("%s/%d", a.acct("/bank-accounts"), bank.ID), map[string]any{"fio_token": ""})
	assertError(t, r, body, http.StatusUnprocessableEntity, "fio_token is required")
}

func TestSyncBankAccountsJob(t *testing.T) {
	ts := newTestServer(t)
	fio := &fakeFio{}
	syncJob := withBank(ts, fio)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	st, _ := bankimport.ParseFioJSON(fixtureFile(t, "fio_transactions.json"))
	fio.st = st
	for i, c := range []*client{a, b} {
		ba := doJSON[api.BankAccount](c, http.StatusCreated, "POST", c.acct("/bank-accounts"),
			map[string]any{"name": "Fio", "number": "19-2000145399/0800", "sync_provider": "fio", "fio_token": strings.Repeat(fmt.Sprint(i), 20)})
		setFixtureAccount(ts, ba.ID)
	}
	fioBank(a) // not synced (no provider)
	if err := syncJob(); err != nil {
		t.Fatal(err)
	}
	if len(fio.calls) != 2 || listTx(a, "").Total != 4 || listTx(b, "").Total != 4 {
		t.Fatalf("calls %v", fio.calls)
	}
	// rate limit is skipped silently, other errors are reported
	fio.err = bankimport.ErrFioRateLimited
	if err := syncJob(); err != nil {
		t.Fatalf("rate limited: %v", err)
	}
	fio.err = bankimport.ErrFioToken
	if err := syncJob(); err == nil || !strings.Contains(err.Error(), "bank account") {
		t.Fatalf("token error: %v", err)
	}
}

// TestBankMatchPaidThanks: invoices fully paid by bank matching get the
// paid-thanks e-mail (when enabled), partial payments don't.
func TestBankMatchPaidThanks(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"paid_thanks_enabled": true})
	bank := fioBank(a)
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("81"), Lines: []api.InvoiceLineInput{line("X", "1", 1000, nil)}})
	inv2 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, VariableSymbol: strPtr("82"), Lines: []api.InvoiceLineInput{line("X", "1", 1000, nil)}})

	importOK(a, bank.ID, fioJSON(fioTx{id: "p1", date: "2026-03-10", vs: "81", amount: "4.00"})) // suggestion only
	doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, txByExternal(a, "p1").ID, "/match"), api.BankTransactionMatch{InvoiceID: &inv.ID})
	if n := len(ts.mail.Messages()); n != 0 {
		t.Fatalf("sent on partial payment: %d", n)
	}
	// auto-matches: the rest of inv and the whole inv2
	importOK(a, bank.ID, fioJSON(fioTx{id: "p2", date: "2026-03-11", vs: "81", amount: "6.00"}, fioTx{id: "p3", date: "2026-03-11", vs: "82", amount: "10.00"}))
	if n := len(ts.mail.Messages()); n != 2 {
		t.Fatalf("auto-match thanks: %d", n)
	}
	if m, _ := ts.mail.Last(); !strings.Contains(m.Subject, inv2.Number) {
		t.Fatalf("thanks: %+v", m)
	}
	// manual match paying an invoice in full
	inv3 := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "1", 1000, nil)}})
	importOK(a, bank.ID, fioJSON(fioTx{id: "p4", date: "2026-03-12", amount: "10.00"}))
	doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, txByExternal(a, "p4").ID, "/match"), api.BankTransactionMatch{InvoiceID: &inv3.ID})
	if m, _ := ts.mail.Last(); len(ts.mail.Messages()) != 3 || !strings.Contains(m.Subject, inv3.Number) {
		t.Fatalf("manual match thanks: %d %+v", len(ts.mail.Messages()), m)
	}
}

func TestBankTransactionRoles(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	member := ts.memberOf(owner, "member@example.cz", "member")
	accountant := ts.memberOf(owner, "acc@example.cz", "accountant")
	bank := fioBank(owner)
	importOK(member, bank.ID, fixtureFile(t, "fio_transactions.json"))
	tx := listTx(accountant, "").Items[0]
	accountant.mustDo(http.StatusOK, "GET", txURL(accountant, tx.ID, ""), nil)
	for _, path := range []string{"/ignore", "/unignore", "/unmatch", "/match"} {
		r, body := accountant.do("POST", txURL(accountant, tx.ID, path), map[string]any{})
		assertError(t, r, body, http.StatusForbidden, "your role (accountant)")
	}
	r, body := accountant.importStatement(bank.ID, "", fixtureFile(t, "fio_transactions.json"))
	assertError(t, r, body, http.StatusForbidden, "your role (accountant)")
	member.mustDo(http.StatusOK, "POST", txURL(member, tx.ID, "/ignore"), nil)
}

func TestForeignCurrencyExchangeRate(t *testing.T) {
	ts := newTestServer(t)
	rates := &fakeRates{}
	withRegistries(ts, api.Deps{CNB: rates, Storage: storage.NewLocal(ts.dataDir)})
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	lines := []api.InvoiceLineInput{line("X", "1", 100, nil)}

	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", IssuedOn: "2026-03-10", Lines: lines})
	if inv.ExchangeRate != "24.350" || rates.lastDate != "2026-03-10" {
		t.Fatalf("invoice rate %q date %q", inv.ExchangeRate, rates.lastDate)
	}
	inv = createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", IssuedOn: "2026-03-10", TaxableFulfillmentDue: strPtr("2026-03-05"), Lines: lines})
	if rates.lastDate != "2026-03-05" {
		t.Fatalf("DUZP date: %q", rates.lastDate)
	}
	if inv = createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Currency: "EUR", ExchangeRate: "25", Lines: lines}); inv.ExchangeRate != "25" {
		t.Fatalf("manual rate: %q", inv.ExchangeRate)
	}
	if inv = createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: lines}); inv.ExchangeRate != "1" {
		t.Fatalf("CZK: %q", inv.ExchangeRate)
	}
	r, body := a.do("POST", a.acct("/invoices"), api.InvoiceCreate{SubjectID: subj.ID, Currency: "USD", Lines: lines})
	assertError(t, r, body, http.StatusUnprocessableEntity, "enter exchange_rate manually")

	name := "Dodavatel"
	exp := createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Currency: "EUR", IssuedOn: "2026-03-02", Lines: lines})
	if exp.ExchangeRate != "24.350" || rates.lastDate != "2026-03-02" {
		t.Fatalf("expense rate %q %q", exp.ExchangeRate, rates.lastDate)
	}
	r, body = a.do("POST", a.acct("/expenses"), api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name}, Currency: "USD", Lines: lines})
	assertError(t, r, body, http.StatusUnprocessableEntity, "body.exchange_rate")
}

func TestExpenseRegistryWarnings(t *testing.T) {
	ts := newTestServer(t)
	withRegistries(ts, api.Deps{VatRegistry: fakeVatReg{}, CNB: &fakeRates{}, Storage: storage.NewLocal(ts.dataDir)})
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	lines := []api.InvoiceLineInput{line("X", "1", 100, nil)}
	mk := func(vat, account string) api.Expense {
		name := "Dodavatel"
		return createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &name,
			SupplierVatNo: &vat, SupplierBankAccount: &account}, Lines: lines})
	}
	// unreliable payer, published account
	e := mk("CZ699001234", "19-2000145399/0800")
	got := getExp(a, e.ID)
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "nespolehlivý plátce DPH (od 2024-03-01)") {
		t.Fatalf("warnings: %v", got.Warnings)
	}
	// unreliable + unpublished account
	e = mk("CZ699001234", "123456789/0100")
	if got := getExp(a, e.ID); len(got.Warnings) != 2 || !strings.Contains(got.Warnings[1], "123456789/0100 není zveřejněný") {
		t.Fatalf("warnings: %v", got.Warnings)
	}
	// not registered, registry failure, foreign DIČ, no DIČ → no warnings
	for _, vat := range []string{"CZ12345678", "CZ11111111", "DE811907980", ""} {
		e = mk(vat, "123456789/0100")
		raw := a.mustDo(http.StatusOK, "GET", expURL(a, e.ID, ""), nil)
		if strings.Contains(string(raw), "warnings") {
			t.Fatalf("%s: %s", vat, raw)
		}
	}
	// list never computes warnings
	if strings.Contains(string(a.mustDo(http.StatusOK, "GET", a.acct("/expenses"), nil)), "warnings") {
		t.Fatal("list has warnings")
	}

	// GET /subjects/{id}/vat-status
	subj := newSubject(a, api.SubjectCreate{Name: "Novák", VatNo: "CZ699001234"})
	st := doJSON[api.VatRegistryResult](a, http.StatusOK, "GET", subjectURL(a, subj.ID)+"/vat-status", nil)
	if !st.Registered || st.Reliable == nil || *st.Reliable || len(st.PublishedAccounts) != 1 {
		t.Fatalf("vat-status: %+v", st)
	}
	noVat := newSubject(a, api.SubjectCreate{Name: "Bez DIČ"})
	r, body := a.do("GET", subjectURL(a, noVat.ID)+"/vat-status", nil)
	assertError(t, r, body, http.StatusUnprocessableEntity, "no Czech VAT number")
	down := newSubject(a, api.SubjectCreate{Name: "Down", VatNo: "11111111"})
	r, body = a.do("GET", subjectURL(a, down.ID)+"/vat-status", nil)
	assertError(t, r, body, http.StatusBadGateway, "unavailable")
	r, body = b.do("GET", subjectURL(b, subj.ID)+"/vat-status", nil)
	assertError(t, r, body, http.StatusNotFound, "subject not found")
}
