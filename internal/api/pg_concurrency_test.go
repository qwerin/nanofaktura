package api_test

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/mail/mailtest"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/secret"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// newPGTestServer is newTestServer on PostgreSQL (several connections, so
// transactions really run concurrently — SQLite's single connection hides
// missing row locks). It needs NANOFAKTURA_TEST_PG_DSN pointing at a
// throw-away database (its public schema is dropped), otherwise it skips:
//
//	docker run -d --rm --name nf-pg --tmpfs /var/lib/postgresql/data -e POSTGRES_PASSWORD=pg -p 127.0.0.1:55432:5432 postgres:16-alpine
//	NANOFAKTURA_TEST_PG_DSN='postgres://postgres:pg@127.0.0.1:55432/postgres?sslmode=disable' go test ./internal/api -run PG
func newPGTestServer(t *testing.T) *testServer {
	t.Helper()
	dsn := os.Getenv("NANOFAKTURA_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("NANOFAKTURA_TEST_PG_DSN not set")
	}
	gdb, err := db.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	cfg := config.Config{AllowSignup: true, WebhooksAllowPrivate: true, DisableRateLimit: true}
	ts := &testServer{t: t, db: gdb, now: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC), mail: mailtest.New(), dataDir: t.TempDir(),
		cfg: cfg, secrets: secret.NewRandom()}
	deps := api.Deps{Now: func() time.Time { return ts.now }, Mailer: ts.mail, Storage: storage.NewLocal(ts.dataDir),
		Secrets: ts.secrets, CNB: &fakeRates{}}
	ts.handler, _ = api.New(gdb, cfg, deps)
	return ts
}

// parallel runs fn(i) n times concurrently and waits.
func parallel(n int, fn func(i int)) {
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); fn(i) }()
	}
	wg.Wait()
}

// assertPaymentsConsistent checks stored paid_amount = Σ payments and total = Σ lines.
func assertPaymentsConsistent(t *testing.T, ts *testServer, id uint) model.Invoice {
	t.Helper()
	var m model.Invoice
	ts.db.First(&m, id)
	var sumLines, sumPay int64
	ts.db.Model(&model.InvoiceLine{}).Where("invoice_id = ?", id).Select("COALESCE(SUM(total),0)").Scan(&sumLines)
	ts.db.Model(&model.Payment{}).Where("invoice_id = ?", id).Select("COALESCE(SUM(amount),0)").Scan(&sumPay)
	if m.PaidAmount != sumPay || (m.Rounding == 0 && m.Total != sumLines) {
		t.Fatalf("invoice %d inconsistent: total=%d Σlines=%d paid_amount=%d Σpayments=%d status=%s",
			id, m.Total, sumLines, m.PaidAmount, sumPay, m.Status)
	}
	return m
}

// Money C-01: 8 concurrent "pay the rest" requests must create exactly one payment.
func TestPGConcurrentPayments(t *testing.T) {
	ts := newPGTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100000, i32(0))}})
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100000, i32(0))}})
	exp := createExp(a, api.ExpenseCreate{SubjectID: &acme.ID, Lines: []api.InvoiceLineInput{line("X", "1", 50000, i32(0))}})
	codes := make([]int, 8)
	proCodes := make([]int, 8)
	expCodes := make([]int, 8)
	parallel(8, func(i int) {
		r, _ := a.do("POST", invURL(a, inv.ID, "/payments"), map[string]any{})
		codes[i] = r.StatusCode
		r, _ = a.do("POST", invURL(a, pro.ID, "/payments"), map[string]any{"create_final_invoice": true})
		proCodes[i] = r.StatusCode
		r, _ = a.do("POST", fmt.Sprintf("%s/%d/payments", a.acct("/expenses"), exp.ID), map[string]any{})
		expCodes[i] = r.StatusCode
	})
	count := func(codes []int, want int) (n int) {
		for _, c := range codes {
			if c == want {
				n++
			}
		}
		return n
	}
	if count(codes, 201) != 1 || count(codes, 409) != 7 {
		t.Errorf("invoice payment codes %v, want one 201 and 409s", codes)
	}
	if count(proCodes, 201) != 1 || count(expCodes, 201) != 1 {
		t.Errorf("proforma codes %v, expense codes %v", proCodes, expCodes)
	}
	if m := assertPaymentsConsistent(t, ts, inv.ID); m.PaidAmount != 100000 || m.Status != model.StatusPaid {
		t.Errorf("invoice: %+v", m)
	}
	var finals int64
	ts.db.Model(&model.Invoice{}).Where("related_id = ? AND document_type = ?", pro.ID, model.DocInvoice).Count(&finals)
	if finals != 1 {
		t.Errorf("final invoices: %d", finals)
	}
	var e model.Expense
	ts.db.First(&e, exp.ID)
	var sum int64
	ts.db.Model(&model.ExpensePayment{}).Where("expense_id = ?", exp.ID).Select("COALESCE(SUM(amount),0)").Scan(&sum)
	if e.PaidAmount != 50000 || sum != 50000 {
		t.Errorf("expense paid_amount=%d Σ=%d", e.PaidAmount, sum)
	}
}

// Money H-01: a concurrent PATCH of the lines and a payment must not lose
// either update (total = Σ lines, paid_amount = Σ payments, status consistent).
func TestPGConcurrentPatchVsPayment(t *testing.T) {
	ts := newPGTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	for k := 0; k < 10; k++ {
		inv := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100000, i32(0))}})
		parallel(3, func(i int) {
			switch i {
			case 0:
				a.do("POST", invURL(a, inv.ID, "/payments"), map[string]any{"amount": 100000})
			case 1:
				a.do("PATCH", invURL(a, inv.ID, ""), map[string]any{"lines": []map[string]any{{"name": "X", "quantity": "2", "unit_price": 100000, "vat_rate_bps": 0}}})
			default:
				a.do("POST", a.acct("/invoices/mark-paid"), map[string]any{})
			}
		})
		m := assertPaymentsConsistent(t, ts, inv.ID)
		if m.Total != 200000 {
			t.Fatalf("iter %d: total %d", k, m.Total)
		}
		if (m.PaidAmount >= m.Total) != (m.Status == model.StatusPaid) {
			t.Fatalf("iter %d: status %s with paid %d of %d", k, m.Status, m.PaidAmount, m.Total)
		}
	}
}

// Money C-01 (bank): concurrent rematches must create one payment from one transaction.
func TestPGConcurrentRematch(t *testing.T) {
	ts := newPGTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	bank := fioBank(a)
	for k := 0; k < 5; k++ {
		vs := fmt.Sprintf("77%02d", k)
		importOK(a, bank.ID, fioJSON(fioTx{id: fmt.Sprintf("9%03d", k), date: "2026-03-10", vs: vs, amount: "1000.00", name: "ACME"}))
		inv := createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), VariableSymbol: &vs, Lines: []api.InvoiceLineInput{line("X", "1", 100000, i32(0))}})
		parallel(3, func(int) { a.do("POST", a.acct("/bank-transactions/rematch"), nil) })
		var n int64
		ts.db.Model(&model.Payment{}).Where("invoice_id = ?", inv.ID).Count(&n)
		if n != 1 {
			t.Fatalf("iter %d: %d payments from one bank transaction", k, n)
		}
		assertPaymentsConsistent(t, ts, inv.ID)
	}
	d := doJSON[map[string]any](a, http.StatusOK, "GET", a.acct("/dashboard?year=2026"), nil)
	if d["unpaid_total"].(float64) != 0 {
		t.Errorf("dashboard: %v", d)
	}
}

// Money C-01 + tax H-03: concurrent payments / bulk mark-paid of a VAT
// payer's proforma create one payment and one tax document.
func TestPGConcurrentProformaPayments(t *testing.T) {
	ts := newPGTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME"})
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100000, i32(2100))}})
	parallel(8, func(i int) {
		if i%2 == 0 {
			a.do("POST", invURL(a, pro.ID, "/payments"), map[string]any{})
		} else {
			a.do("POST", a.acct("/invoices/mark-paid"), map[string]any{})
		}
	})
	m := assertPaymentsConsistent(t, ts, pro.ID)
	var tds int64
	ts.db.Model(&model.Invoice{}).Where("related_id = ? AND document_type = ?", pro.ID, model.DocTaxDocument).Count(&tds)
	if m.PaidAmount != 121000 || tds != 1 {
		t.Fatalf("proforma paid %d, tax documents %d", m.PaidAmount, tds)
	}
	// the reports' SQL runs on PostgreSQL too
	if o := overview(a); o.IncomeTax.Income != 100000 {
		t.Fatalf("income %+v", o.IncomeTax)
	}
	if v := vatReport(a, "2026-03"); v.Return.R1.Vat != 21000 {
		t.Fatalf("vat %+v", v.Return.R1)
	}
}
