package api_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/eet/eettest"
	"github.com/qwerin/nanofaktura/internal/model"
)

const eetEIC = "CZ12345678"

func eetCert(ts *testServer, eic string, notAfter time.Time) api.EETCertificateUpload {
	return api.EETCertificateUpload{PKCS12: base64.StdEncoding.EncodeToString(eettest.PKCS12(ts.t, eic, "heslo", notAfter)), Password: "heslo"}
}

// setupEET uploads a certificate and turns EET on (production environment).
func setupEET(c *client) api.EETSettings {
	c.ts.t.Helper()
	doJSON[api.EETSettings](c, http.StatusOK, "PUT", c.acct("/eet/certificate"), eetCert(c.ts, eetEIC, c.ts.now.AddDate(1, 0, 0)))
	return doJSON[api.EETSettings](c, http.StatusOK, "PATCH", c.acct("/eet"), map[string]any{"unit_id": "11", "enabled": true})
}

func eetSales(c *client, query string) []api.EETSale {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.EETSale]](c, http.StatusOK, "GET", c.acct("/eet-sales")+query, nil).Items
}

func invoiceWith(c *client, method string, price int64) api.Invoice {
	c.ts.t.Helper()
	name := "Zákazník"
	return createInv(c, api.InvoiceCreate{InvoiceSnapshotFields: api.InvoiceSnapshotFields{ClientName: &name}, PaymentMethod: method,
		Lines: []api.InvoiceLineInput{line("Služba", "", price, nil)}})
}

func TestEETSettings(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	url := a.acct("/eet")

	got := doJSON[api.EETSettings](a, http.StatusOK, "GET", url, nil)
	if got.Enabled || got.HasCertificate || got.Environment != "production" || got.RegisterID != "1" {
		t.Fatalf("defaults: %+v", got)
	}
	res, body := a.do("PATCH", url, map[string]any{"enabled": true})
	assertCode(t, res, body, http.StatusConflict, "eet_not_configured")
	res, body = a.do("POST", a.acct("/eet/actions/test"), nil)
	assertCode(t, res, body, http.StatusConflict, "eet_not_configured")
	for field, bad := range map[string]string{"eic": "12345678", "unit_id": "011", "register_id": "pokladna č. 1"} {
		res, body = a.do("PATCH", url, map[string]any{field: bad})
		assertError(t, res, body, http.StatusUnprocessableEntity, field)
	}

	// certificate: garbage, wrong password, expired, valid
	res, body = a.do("PUT", url+"/certificate", api.EETCertificateUpload{PKCS12: base64.StdEncoding.EncodeToString([]byte("x")), Password: "p"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "eet_bad_certificate")
	wrong := eetCert(ts, eetEIC, ts.now.AddDate(1, 0, 0))
	wrong.Password = "jine"
	res, body = a.do("PUT", url+"/certificate", wrong)
	assertCode(t, res, body, http.StatusUnprocessableEntity, "eet_bad_certificate")
	if strings.Contains(string(body), "jine") {
		t.Fatalf("password echoed: %s", body)
	}
	res, body = a.do("PUT", url+"/certificate", eetCert(ts, eetEIC, ts.now.AddDate(0, 0, -1)))
	assertCode(t, res, body, http.StatusUnprocessableEntity, "eet_certificate_expired")

	got = doJSON[api.EETSettings](a, http.StatusOK, "PUT", url+"/certificate", eetCert(ts, eetEIC, ts.now.AddDate(1, 0, 0)))
	if !got.HasCertificate || got.CertificateEIC != eetEIC || got.EIC != eetEIC || got.CertificateValidTo == nil || got.Enabled {
		t.Fatalf("after upload: %+v", got)
	}
	var stored model.EETConfig
	ts.db.First(&stored)
	if stored.Certificate == "" || strings.Contains(stored.Certificate, "PRIVATE KEY") {
		t.Fatalf("certificate must be stored encrypted")
	}
	res, body = a.do("PATCH", url, map[string]any{"enabled": true}) // still no unit
	assertCode(t, res, body, http.StatusConflict, "eet_not_configured")
	got = doJSON[api.EETSettings](a, http.StatusOK, "PATCH", url, map[string]any{"unit_id": "11", "register_id": "kasa-1", "environment": "playground", "enabled": true})
	if !got.Enabled || got.UnitID != "11" || got.RegisterID != "kasa-1" || got.Environment != "playground" {
		t.Fatalf("enabled: %+v", got)
	}

	// connection test = verification mode, registers nothing
	test := doJSON[api.EETTestResult](a, http.StatusOK, "POST", url+"/actions/test", nil)
	reqs := ts.eet.Requests()
	if !test.OK || test.Environment != "playground" || len(reqs) != 1 || !reqs[0].Verify || reqs[0].Production || reqs[0].EIC != eetEIC || reqs[0].RegisterID != "kasa-1" {
		t.Fatalf("test: %+v %+v", test, reqs)
	}
	ts.eet.SetMode(eettest.ModeDown)
	if test = doJSON[api.EETTestResult](a, http.StatusOK, "POST", url+"/actions/test", nil); test.OK || test.Message == "" {
		t.Fatalf("test while down: %+v", test)
	}
	ts.eet.SetMode(eettest.ModeReject)
	if test = doJSON[api.EETTestResult](a, http.StatusOK, "POST", url+"/actions/test", nil); test.OK || test.Code != 4 {
		t.Fatalf("test rejected: %+v", test)
	}
	ts.eet.SetMode(eettest.ModeOK)

	// clearing a required value turns EET off
	if got = doJSON[api.EETSettings](a, http.StatusOK, "PATCH", url, map[string]any{"unit_id": ""}); got.Enabled {
		t.Fatalf("enabled without a unit: %+v", got)
	}
	doJSON[api.EETSettings](a, http.StatusOK, "PATCH", url, map[string]any{"unit_id": "11", "enabled": true})

	// tenant isolation and roles
	if other := doJSON[api.EETSettings](b, http.StatusOK, "GET", b.acct("/eet"), nil); other.HasCertificate || other.EIC != "" {
		t.Fatalf("b sees a's settings: %+v", other)
	}
	member := ts.memberOf(a, "m@example.cz", "member")
	res, body = member.do("PATCH", url, map[string]any{"enabled": false})
	assertError(t, res, body, http.StatusForbidden, "cannot do this")

	a.mustDo(http.StatusNoContent, "DELETE", url+"/certificate", nil)
	if got = doJSON[api.EETSettings](a, http.StatusOK, "GET", url, nil); got.Enabled || got.HasCertificate || got.CertificateEIC != "" {
		t.Fatalf("after certificate removal: %+v", got)
	}
	events := doJSON[api.ListResponse[api.Event]](a, http.StatusOK, "GET", a.acct("/events?name=eet.settings_updated"), nil)
	if events.Total < 3 {
		t.Fatalf("settings events: %d", events.Total)
	}
}

func TestEETPayments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	yes, no := true, false

	// without EET: nothing is registered, an explicit request is refused
	plain := invoiceWith(a, "cash", 10000)
	if r := pay(a, plain.ID, api.PaymentCreate{Amount: ptr64(1000)}); len(r.Invoice.EETSales) != 0 {
		t.Fatalf("sale without EET: %+v", r.Invoice.EETSales)
	}
	res, body := a.do("POST", invURL(a, plain.ID, "/payments"), api.PaymentCreate{Amount: ptr64(1000), EET: &yes})
	assertCode(t, res, body, http.StatusConflict, "eet_not_enabled")

	setupEET(a)

	// cash invoice: registered by default, right after the commit
	cash := invoiceWith(a, "cash", 123450)
	r := pay(a, cash.ID, api.PaymentCreate{})
	if len(r.Invoice.EETSales) != 1 {
		t.Fatalf("cash payment: %+v", r.Invoice.EETSales)
	}
	sale := r.Invoice.EETSales[0]
	if sale.Status != "confirmed" || sale.POK == "" || sale.PaymentID == nil || *sale.PaymentID != r.Payment.ID || sale.Amount != 123450 ||
		sale.Environment != "production" || sale.Attempts != 1 || sale.InvoiceNumber != cash.Number || sale.SerialNo != fmt.Sprint(sale.ID) ||
		!sale.SoldAt.Equal(ts.now) || !sale.DeadlineAt.Equal(ts.now.Add(48*time.Hour)) {
		t.Fatalf("sale: %+v", sale)
	}
	reqs := ts.eet.Requests()
	last := reqs[len(reqs)-1]
	if last.Amount != "1234.50" || !last.First || last.Verify || !last.Production || last.EIC != eetEIC || last.UnitID != "11" ||
		last.RegisterID != "1" || last.SerialNo != sale.SerialNo || last.SoldAt != "2026-03-15T11:00:00+01:00" || last.CertEIC != eetEIC {
		t.Fatalf("request: %+v", last)
	}
	if got := getInv(a, cash.ID); len(got.EETSales) != 1 || got.EETSales[0].POK != sale.POK {
		t.Fatalf("detail: %+v", got.EETSales)
	}
	ev := doJSON[api.ListResponse[api.Event]](a, http.StatusOK, "GET", a.acct("/events?name=eet.registered"), nil)
	if ev.Total != 1 || !strings.Contains(ev.Items[0].Text, cash.Number) {
		t.Fatalf("events: %+v", ev)
	}

	// bank transfer: not a contact payment unless said so; cash can opt out
	bank := invoiceWith(a, "bank", 50000)
	if r = pay(a, bank.ID, api.PaymentCreate{Amount: ptr64(20000)}); len(r.Invoice.EETSales) != 0 {
		t.Fatalf("bank payment registered: %+v", r.Invoice.EETSales)
	}
	if r = pay(a, bank.ID, api.PaymentCreate{Amount: ptr64(30000), EET: &yes}); len(r.Invoice.EETSales) != 1 || r.Invoice.EETSales[0].Amount != 30000 {
		t.Fatalf("explicit contact payment: %+v", r.Invoice.EETSales)
	}
	cash2 := invoiceWith(a, "cash", 7000)
	if r = pay(a, cash2.ID, api.PaymentCreate{EET: &no}); len(r.Invoice.EETSales) != 0 {
		t.Fatalf("opt-out registered: %+v", r.Invoice.EETSales)
	}

	// an older payment is registered at noon of its day; a future one is refused
	old := invoiceWith(a, "cash", 9900)
	r = pay(a, old.ID, api.PaymentCreate{PaidOn: "2026-03-10"})
	if got := r.Invoice.EETSales[0].SoldAt.UTC().Format(time.RFC3339); got != "2026-03-10T11:00:00Z" {
		t.Fatalf("sold_at of an older payment: %s", got)
	}
	future := invoiceWith(a, "cash", 9900)
	res, body = a.do("POST", invURL(a, future.ID, "/payments"), api.PaymentCreate{PaidOn: "2026-03-16"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "paid_on")
	if got := getInv(a, future.ID); len(got.Payments) != 0 || len(got.EETSales) != 0 {
		t.Fatalf("refused payment left traces: %+v", got)
	}

	// foreign currency: the amount is registered in CZK
	name := "Kunde"
	eur := createInv(a, api.InvoiceCreate{InvoiceSnapshotFields: api.InvoiceSnapshotFields{ClientName: &name}, PaymentMethod: "cash",
		Currency: "EUR", ExchangeRate: "25", Lines: []api.InvoiceLineInput{line("Service", "", 10000, nil)}})
	if r = pay(a, eur.ID, api.PaymentCreate{}); r.Invoice.EETSales[0].Amount != 250000 {
		t.Fatalf("EUR sale: %+v", r.Invoice.EETSales[0])
	}

	// deleting a registered payment registers a cancellation
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, cash.ID, fmt.Sprintf("/payments/%d", *sale.PaymentID)), nil)
	got := getInv(a, cash.ID)
	if len(got.EETSales) != 2 {
		t.Fatalf("after delete: %+v", got.EETSales)
	}
	orig, storno := got.EETSales[0], got.EETSales[1]
	if orig.PaymentID != nil || orig.Status != "confirmed" || storno.CancelsID == nil || *storno.CancelsID != orig.ID ||
		storno.Amount != -123450 || storno.Status != "confirmed" || storno.POK == "" || storno.POK == orig.POK {
		t.Fatalf("cancellation: %+v / %+v", orig, storno)
	}
	reqs = ts.eet.Requests()
	if last = reqs[len(reqs)-1]; last.Amount != "-1234.50" || last.SerialNo != storno.SerialNo {
		t.Fatalf("cancellation request: %+v", last)
	}

	// lists, filters, tenant isolation
	if all := eetSales(a, ""); len(all) != 5 || all[0].ID != storno.ID {
		t.Fatalf("list: %d", len(all))
	}
	if one := eetSales(a, fmt.Sprintf("?invoice_id=%d&status=confirmed", bank.ID)); len(one) != 1 || one[0].InvoiceID != bank.ID {
		t.Fatalf("filtered: %+v", one)
	}
	if len(eetSales(b, "")) != 0 {
		t.Fatal("b sees a's sales")
	}
	res, body = b.do("POST", b.acct(fmt.Sprintf("/eet-sales/%d/actions/retry", storno.ID)), nil)
	assertError(t, res, body, http.StatusNotFound, "eet sale not found")
	res, body = a.do("POST", a.acct(fmt.Sprintf("/eet-sales/%d/actions/retry", storno.ID)), nil)
	assertCode(t, res, body, http.StatusConflict, "eet_not_retryable")
	if st := doJSON[api.EETSettings](a, http.StatusOK, "GET", a.acct("/eet"), nil); st.PendingCount != 0 || st.FailedCount != 0 {
		t.Fatalf("counts: %+v", st)
	}
}

func TestEETRetries(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	setupEET(a)
	saleOf := func(inv uint) api.EETSale {
		t.Helper()
		return eetSales(a, fmt.Sprintf("?invoice_id=%d", inv))[0]
	}

	// the tax authority is unreachable: the payment succeeds, the sale waits
	ts.eet.SetMode(eettest.ModeDown)
	inv := invoiceWith(a, "cash", 5000)
	r := pay(a, inv.ID, api.PaymentCreate{})
	s := r.Invoice.EETSales[0]
	if r.Invoice.Status != "paid" || s.Status != "pending" || s.Attempts != 1 || s.Error == "" || s.POK != "" {
		t.Fatalf("while down: %+v", s)
	}
	if st := doJSON[api.EETSettings](a, http.StatusOK, "GET", a.acct("/eet"), nil); st.PendingCount != 1 {
		t.Fatalf("pending count: %+v", st)
	}
	ts.eet.SetMode(eettest.ModeOK)
	mustRunJob(ts, "eet") // not due yet (1 minute)
	if s = saleOf(inv.ID); s.Status != "pending" || s.Attempts != 1 {
		t.Fatalf("sent too early: %+v", s)
	}
	ts.eet.SetMode(eettest.ModeTemporary)
	ts.now = ts.now.Add(time.Minute)
	mustRunJob(ts, "eet")
	if s = saleOf(inv.ID); s.Status != "pending" || s.Attempts != 2 || s.ErrorCode != -1 {
		t.Fatalf("temporary error: %+v", s)
	}
	ts.eet.SetMode(eettest.ModeOK)
	ts.now = ts.now.Add(2 * time.Minute)
	mustRunJob(ts, "eet")
	mustRunJob(ts, "eet") // idempotent
	if s = saleOf(inv.ID); s.Status != "confirmed" || s.Attempts != 3 || s.POK == "" || s.Error != "" || s.ErrorCode != 0 {
		t.Fatalf("after the job: %+v", s)
	}
	reqs := ts.eet.Requests()
	first, last := reqs[len(reqs)-2], reqs[len(reqs)-1]
	if last.First || last.SerialNo != first.SerialNo || last.SoldAt != first.SoldAt || last.Amount != first.Amount || last.UUID == first.UUID || last.SentAt == first.SentAt {
		t.Fatalf("resend must keep the data and change the header: %+v / %+v", first, last)
	}

	// refused: failed + event + todo; fixed by a retry
	ts.eet.SetMode(eettest.ModeReject)
	inv2 := invoiceWith(a, "cash", 6000)
	r = pay(a, inv2.ID, api.PaymentCreate{})
	if s = r.Invoice.EETSales[0]; s.Status != "failed" || s.ErrorCode != 4 || s.Error == "" {
		t.Fatalf("rejected: %+v", s)
	}
	ts.now = ts.now.Add(time.Hour)
	mustRunJob(ts, "eet")
	if s = saleOf(inv2.ID); s.Attempts != 1 {
		t.Fatalf("a failed sale must not be resent by the job: %+v", s)
	}
	if ev := doJSON[api.ListResponse[api.Event]](a, http.StatusOK, "GET", a.acct("/events?name=eet.failed"), nil); ev.Total != 1 {
		t.Fatalf("eet.failed events: %d", ev.Total)
	}
	openTodos := func() int {
		n := 0
		for _, td := range doJSON[api.ListResponse[api.Todo]](a, http.StatusOK, "GET", a.acct("/todos"), nil).Items {
			if td.Name == "eet.failed" && !td.Completed {
				n++
			}
		}
		return n
	}
	if openTodos() != 1 {
		t.Fatal("missing the todo of the failed sale")
	}
	ts.eet.SetMode(eettest.ModeOK)
	retried := doJSON[api.EETSale](a, http.StatusOK, "POST", a.acct(fmt.Sprintf("/eet-sales/%d/actions/retry", s.ID)), nil)
	if retried.Status != "confirmed" || retried.Attempts != 2 || openTodos() != 0 {
		t.Fatalf("retry: %+v, open todos %d", retried, openTodos())
	}

	// a new certificate puts failed sales back into the queue
	ts.eet.SetMode(eettest.ModeReject)
	inv3 := invoiceWith(a, "cash", 7000)
	pay(a, inv3.ID, api.PaymentCreate{})
	ts.eet.SetMode(eettest.ModeOK)
	doJSON[api.EETSettings](a, http.StatusOK, "PUT", a.acct("/eet/certificate"), eetCert(ts, eetEIC, ts.now.AddDate(1, 0, 0)))
	mustRunJob(ts, "eet")
	if s = saleOf(inv3.ID); s.Status != "confirmed" {
		t.Fatalf("after a new certificate: %+v", s)
	}

	// deleting the payment of a refused sale drops it without a cancellation
	ts.eet.SetMode(eettest.ModeReject)
	inv4 := invoiceWith(a, "cash", 8000)
	r = pay(a, inv4.ID, api.PaymentCreate{})
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv4.ID, fmt.Sprintf("/payments/%d", r.Payment.ID)), nil)
	if sales := eetSales(a, fmt.Sprintf("?invoice_id=%d", inv4.ID)); len(sales) != 1 || sales[0].Status != "cancelled" || sales[0].PaymentID != nil {
		t.Fatalf("dropped sale: %+v", sales)
	}
	if openTodos() != 0 {
		t.Fatal("the todo of a dropped sale must be closed")
	}

	// … but a sale that may have reached the tax authority is cancelled properly
	ts.eet.SetMode(eettest.ModeDown)
	inv5 := invoiceWith(a, "cash", 9000)
	r = pay(a, inv5.ID, api.PaymentCreate{})
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, inv5.ID, fmt.Sprintf("/payments/%d", r.Payment.ID)), nil)
	ts.eet.SetMode(eettest.ModeOK)
	ts.now = ts.now.Add(time.Hour)
	mustRunJob(ts, "eet")
	sales := eetSales(a, fmt.Sprintf("?invoice_id=%d", inv5.ID))
	if len(sales) != 2 || sales[0].Status != "confirmed" || sales[1].Status != "confirmed" || sales[0].Amount+sales[1].Amount != 0 {
		t.Fatalf("sale + cancellation: %+v", sales)
	}

	// without a certificate the waiting sale fails with a clear reason
	ts.eet.SetMode(eettest.ModeDown)
	inv6 := invoiceWith(a, "cash", 9100)
	pay(a, inv6.ID, api.PaymentCreate{})
	a.mustDo(http.StatusNoContent, "DELETE", a.acct("/eet/certificate"), nil)
	ts.now = ts.now.Add(time.Hour)
	mustRunJob(ts, "eet")
	if s = saleOf(inv6.ID); s.Status != "failed" || !strings.Contains(s.Error, "certificate") || s.Attempts != 1 {
		t.Fatalf("without a certificate: %+v", s)
	}
}
