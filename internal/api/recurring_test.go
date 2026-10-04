package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// ---- helpers ----

// runJob runs a scheduler job of the API (same DB, clock, mailer) at ts.now.
func runJob(ts *testServer, name string) error {
	ts.t.Helper()
	deps := api.Deps{Now: func() time.Time { return ts.now }, Mailer: ts.mail, Storage: storage.NewLocal(ts.dataDir), Secrets: ts.secrets}
	for _, j := range api.Jobs(ts.db, ts.cfg, deps) {
		if j.Name == name {
			return j.Run(context.Background(), ts.now)
		}
	}
	ts.t.Fatalf("no job %q", name)
	return nil
}

func mustRunJob(ts *testServer, name string) {
	ts.t.Helper()
	if err := runJob(ts, name); err != nil {
		ts.t.Fatalf("job %s: %v", name, err)
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 10, 0, 0, 0, time.UTC) }

func tplURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/templates"), id, suffix)
}

func recURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/recurring"), id, suffix)
}

func newTemplate(c *client, subjectID uint, lineName string) api.Template {
	c.ts.t.Helper()
	return doJSON[api.Template](c, http.StatusCreated, "POST", c.acct("/templates"), api.TemplateCreate{
		Name: "Hosting", SubjectID: subjectID,
		Lines: []api.TemplateLineInput{{Name: lineName, Quantity: "2", UnitPrice: 50000}},
	})
}

func newRecurring(c *client, body api.RecurringCreate) api.Recurring {
	c.ts.t.Helper()
	return doJSON[api.Recurring](c, http.StatusCreated, "POST", c.acct("/recurring"), body)
}

// invoicesByIssue lists invoices oldest first.
func invoicesByIssue(c *client) []api.InvoiceSummary {
	c.ts.t.Helper()
	return listInv(c, "?sort=issued_on&per_page=200").Items
}

// ---- templates ----

func TestTemplatesCRUD(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})

	tpl := newTemplate(a, subj.ID, "Hosting {MONTH_NAME} {YEAR}")
	if tpl.DocumentType != "invoice" || len(tpl.Lines) != 1 || tpl.Lines[0].Quantity != "2" || tpl.Tags == nil || tpl.Note != nil {
		t.Fatalf("created: %+v", tpl)
	}
	got := doJSON[api.Template](a, http.StatusOK, "GET", tplURL(a, tpl.ID, ""), nil)
	if got.Name != "Hosting" {
		t.Fatalf("get: %+v", got)
	}
	list := doJSON[api.ListResponse[api.Template]](a, http.StatusOK, "GET", a.acct("/templates?query=host"), nil)
	if list.Total != 1 {
		t.Fatalf("list: %+v", list)
	}
	if l := doJSON[api.ListResponse[api.Template]](a, http.StatusOK, "GET", a.acct("/templates?query=zzz"), nil); l.Total != 0 {
		t.Fatalf("query filter: %+v", l)
	}

	note := "Za období {MONTH}/{YEAR}"
	patched := doJSON[api.Template](a, http.StatusOK, "PATCH", tplURL(a, tpl.ID, ""), api.TemplatePatch{
		Name: ptr("Hosting měsíční"), Note: &note, DocumentType: ptr("proforma"),
		Lines: []api.TemplateLineInput{{Name: "A"}, {Name: "B", Quantity: "1,5", UnitPrice: 100, VatRateBps: i32(1200)}},
	})
	if patched.Name != "Hosting měsíční" || patched.DocumentType != "proforma" || len(patched.Lines) != 2 ||
		patched.Lines[1].Quantity != "1.5" || *patched.Lines[1].VatRateBps != 1200 || *patched.Note != note {
		t.Fatalf("patched: %+v", patched)
	}

	// validation
	other := newSubject(b, api.SubjectCreate{Name: "Cizí"})
	res, body := a.do("POST", a.acct("/templates"), api.TemplateCreate{Name: "X", SubjectID: other.ID, Lines: []api.TemplateLineInput{{Name: "a"}}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "subject not found")
	res, body = a.do("POST", a.acct("/templates"), api.TemplateCreate{Name: "X", SubjectID: subj.ID, Lines: []api.TemplateLineInput{{Name: "a", Quantity: "x"}}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "lines[0].quantity")
	res, body = a.do("POST", a.acct("/templates"), api.TemplateCreate{Name: "X", SubjectID: subj.ID, BankAccountID: uptr(999), Lines: []api.TemplateLineInput{{Name: "a"}}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "bank account not found")
	res, body = a.do("POST", a.acct("/templates"), api.TemplateCreate{Name: "   ", SubjectID: subj.ID, Lines: []api.TemplateLineInput{{Name: "a"}}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "name must not be empty")

	// tenant isolation
	res, body = b.do("GET", tplURL(b, tpl.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "template not found")
	res, body = b.do("POST", tplURL(b, tpl.ID, "/create-invoice"), nil)
	assertError(t, res, body, http.StatusNotFound, "template not found")
	if l := doJSON[api.ListResponse[api.Template]](b, http.StatusOK, "GET", b.acct("/templates"), nil); l.Total != 0 {
		t.Fatalf("b sees a's templates: %+v", l)
	}

	// delete blocked while used by a recurring invoice
	rec := newRecurring(a, api.RecurringCreate{Name: "R", TemplateID: tpl.ID})
	res, body = a.do("DELETE", tplURL(a, tpl.ID, ""), nil)
	assertError(t, res, body, http.StatusConflict, "used by a recurring invoice")
	a.mustDo(http.StatusNoContent, "DELETE", recURL(a, rec.ID, ""), nil)
	a.mustDo(http.StatusNoContent, "DELETE", tplURL(a, tpl.ID, ""), nil)
	res, body = a.do("GET", tplURL(a, tpl.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "template not found")
}

func ptr[T any](v T) *T { return &v }
func uptr(v uint) *uint { return &v }

func TestTemplateCreateInvoice(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"default_note": "Výchozí {MONTH_NAME}"})
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", DueDays: intPtr(7)})
	tpl := newTemplate(a, subj.ID, "Hosting {MONTH_NAME} {YEAR}, minulý {PREV_MONTH_NAME}, Q{QUARTER}")

	// no body: issued today (2026-03-15), placeholders from that date, defaults from subject/account
	inv := doJSON[api.Invoice](a, http.StatusCreated, "POST", tplURL(a, tpl.ID, "/create-invoice"), nil)
	if inv.IssuedOn != "2026-03-15" || inv.DocumentType != "invoice" || inv.DueDays != 7 || inv.Total != 100000 {
		t.Fatalf("invoice: %+v", inv.InvoiceSummary)
	}
	if inv.Lines[0].Name != "Hosting březen 2026, minulý únor, Q1" || inv.Note != "Výchozí březen" {
		t.Fatalf("placeholders: %q / %q", inv.Lines[0].Name, inv.Note)
	}

	// explicit date and type; English template
	a.mustDo(http.StatusOK, "PATCH", tplURL(a, tpl.ID, ""), api.TemplatePatch{Language: ptr("en")})
	pro := doJSON[api.Invoice](a, http.StatusCreated, "POST", tplURL(a, tpl.ID, "/create-invoice"),
		api.TemplateIssue{IssuedOn: "2026-01-10", DocumentType: "proforma"})
	if pro.DocumentType != "proforma" || pro.IssuedOn != "2026-01-10" || pro.Language != "en" ||
		pro.Lines[0].Name != "Hosting January 2026, minulý December, Q1" {
		t.Fatalf("proforma: %+v %q", pro.InvoiceSummary, pro.Lines[0].Name)
	}
}

func TestSaveAsTemplate(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{
		SubjectID: new(subj.ID), Tags: []string{"web"}, OrderNumber: "OBJ-1", DueDays: intPtr(30),
		Lines: []api.InvoiceLineInput{line("Vývoj", "3", 150000, nil), line("Sleva", "-1", 5000, i32(0))},
	})
	tpl := doJSON[api.Template](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/save-as-template"), nil)
	if tpl.Name != "ACME "+inv.Number || tpl.SubjectID != subj.ID || *tpl.DueDays != 30 || tpl.OrderNumber != "OBJ-1" ||
		len(tpl.Lines) != 2 || tpl.Lines[0].Quantity != "3" || tpl.Lines[1].Quantity != "-1" || tpl.Tags[0] != "web" {
		t.Fatalf("template: %+v", tpl)
	}
	named := doJSON[api.Template](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/save-as-template"), api.SaveAsTemplate{Name: "Moje"})
	if named.Name != "Moje" {
		t.Fatalf("named: %+v", named)
	}
	// round trip: an invoice from the template has the same total
	again := doJSON[api.Invoice](a, http.StatusCreated, "POST", tplURL(a, tpl.ID, "/create-invoice"), nil)
	if again.Total != inv.Total {
		t.Fatalf("total %d, want %d", again.Total, inv.Total)
	}

	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	res, body := a.do("POST", invURL(a, corr.ID, "/save-as-template"), nil)
	assertError(t, res, body, http.StatusConflict, "correction")
	res, body = a.do("POST", invURL(a, 99999, "/save-as-template"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

// ---- recurring ----

func TestRecurringCRUD(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := newTemplate(a, subj.ID, "Hosting")

	r := newRecurring(a, api.RecurringCreate{Name: "Měsíční hosting", TemplateID: tpl.ID})
	if r.StartOn != "2026-03-15" || r.NextOccurrenceOn != "2026-03-15" || r.MonthsPeriod != 1 || !r.Active || r.IssueAs != "invoice" {
		t.Fatalf("defaults: %+v", r)
	}
	r2 := newRecurring(a, api.RecurringCreate{Name: "Koncem měsíce", TemplateID: tpl.ID, StartOn: "2026-02-10", DayOfMonth: intPtr(31), MonthsPeriod: 3, Active: ptr(false)})
	if r2.NextOccurrenceOn != "2026-02-28" || r2.Active || *r2.DayOfMonth != 31 {
		t.Fatalf("day_of_month: %+v", r2)
	}
	if l := doJSON[api.ListResponse[api.Recurring]](a, http.StatusOK, "GET", a.acct("/recurring?active=true"), nil); l.Total != 1 || l.Items[0].ID != r.ID {
		t.Fatalf("active filter: %+v", l)
	}
	if l := doJSON[api.ListResponse[api.Recurring]](a, http.StatusOK, "GET", a.acct("/recurring"), nil); l.Total != 2 {
		t.Fatalf("list: %+v", l)
	}

	// patch: schedule change before any generation recomputes from start_on
	p := doJSON[api.Recurring](a, http.StatusOK, "PATCH", recURL(a, r.ID, ""), api.RecurringPatch{DayOfMonth: intPtr(1), SendEmail: ptr(true)})
	if p.NextOccurrenceOn != "2026-04-01" || !p.SendEmail {
		t.Fatalf("patch day: %+v", p)
	}
	p = doJSON[api.Recurring](a, http.StatusOK, "PATCH", recURL(a, r.ID, ""), api.RecurringPatch{NextOccurrenceOn: ptr("2026-05-05")})
	if p.NextOccurrenceOn != "2026-05-05" {
		t.Fatalf("patch next: %+v", p)
	}

	// validation
	res, body := a.do("POST", a.acct("/recurring"), api.RecurringCreate{Name: "X", TemplateID: tpl.ID, StartOn: "2026-05-01", EndOn: "2026-04-01"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "end_on must not be before start_on")
	bTpl := newTemplate(b, newSubject(b, api.SubjectCreate{Name: "B"}).ID, "B")
	res, body = a.do("POST", a.acct("/recurring"), api.RecurringCreate{Name: "X", TemplateID: bTpl.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "template not found")
	res, body = a.do("PATCH", recURL(a, r.ID, ""), api.RecurringPatch{EndOn: ptr("nope")})
	assertError(t, res, body, http.StatusUnprocessableEntity, "end_on")

	// tenant isolation
	res, body = b.do("GET", recURL(b, r.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "recurring not found")
	res, body = b.do("POST", recURL(b, r.ID, "/run-now"), nil)
	assertError(t, res, body, http.StatusNotFound, "recurring not found")

	a.mustDo(http.StatusNoContent, "DELETE", recURL(a, r2.ID, ""), nil)
	res, body = a.do("GET", recURL(a, r2.ID, ""), nil)
	assertError(t, res, body, http.StatusNotFound, "recurring not found")
}

// TestRecurringJobCatchUpAndIdempotency: missed periods are generated one
// by one (each dated its occurrence, with clamping), repeated runs create
// nothing new.
func TestRecurringJobCatchUpAndIdempotency(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := newTemplate(a, subj.ID, "Hosting za {MONTH_NAME} {YEAR}")
	r := newRecurring(a, api.RecurringCreate{Name: "R", TemplateID: tpl.ID, StartOn: "2026-01-31"})

	mustRunJob(ts, "recurring") // now = 2026-03-15 → Jan 31, Feb 28
	mustRunJob(ts, "recurring")
	mustRunJob(ts, "recurring")
	invs := invoicesByIssue(a)
	if len(invs) != 2 || invs[0].IssuedOn != "2026-01-31" || invs[1].IssuedOn != "2026-02-28" {
		t.Fatalf("catch-up: %+v", invs)
	}
	if n := getInv(a, invs[1].ID).Lines[0].Name; n != "Hosting za únor 2026" {
		t.Fatalf("line name %q", n)
	}
	r = doJSON[api.Recurring](a, http.StatusOK, "GET", recURL(a, r.ID, ""), nil)
	if r.NextOccurrenceOn != "2026-03-31" || r.LastInvoiceID == nil || *r.LastInvoiceID != invs[1].ID || r.LastRunAt == nil {
		t.Fatalf("advanced: %+v", r)
	}

	ts.now = day(2026, 3, 30)
	mustRunJob(ts, "recurring")
	if n := len(invoicesByIssue(a)); n != 2 {
		t.Fatalf("generated early: %d", n)
	}
	ts.now = day(2026, 3, 31)
	mustRunJob(ts, "recurring")
	mustRunJob(ts, "recurring")
	invs = invoicesByIssue(a)
	if len(invs) != 3 || invs[2].IssuedOn != "2026-03-31" {
		t.Fatalf("march: %+v", invs)
	}
	// anchor day 31 is kept after February (no drift)
	r = doJSON[api.Recurring](a, http.StatusOK, "GET", recURL(a, r.ID, ""), nil)
	if r.NextOccurrenceOn != "2026-04-30" {
		t.Fatalf("next %s", r.NextOccurrenceOn)
	}
	if len(ts.mail.Messages()) != 0 {
		t.Fatal("send_email=false sent mail")
	}
}

func TestRecurringEndOnAndInactive(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := newTemplate(a, subj.ID, "X")
	ended := newRecurring(a, api.RecurringCreate{Name: "End", TemplateID: tpl.ID, StartOn: "2026-01-01", EndOn: "2026-02-15"})
	paused := newRecurring(a, api.RecurringCreate{Name: "Paused", TemplateID: tpl.ID, StartOn: "2026-01-05", Active: ptr(false)})
	quarterly := newRecurring(a, api.RecurringCreate{Name: "Q", TemplateID: tpl.ID, StartOn: "2025-12-01", MonthsPeriod: 3, IssueAs: "proforma"})

	mustRunJob(ts, "recurring")
	invs := invoicesByIssue(a)
	var dates []string
	for _, i := range invs {
		dates = append(dates, i.IssuedOn+"/"+i.DocumentType)
	}
	want := "2025-12-01/proforma 2026-01-01/invoice 2026-02-01/invoice 2026-03-01/proforma"
	if strings.Join(dates, " ") != want {
		t.Fatalf("got %v, want %s", dates, want)
	}
	ended = doJSON[api.Recurring](a, http.StatusOK, "GET", recURL(a, ended.ID, ""), nil)
	if ended.Active || ended.NextOccurrenceOn != "2026-03-01" {
		t.Fatalf("ended: %+v", ended)
	}
	res, body := a.do("POST", recURL(a, ended.ID, "/activate"), nil)
	assertError(t, res, body, http.StatusConflict, "has ended")

	// activating skips the periods missed while paused
	act := doJSON[api.Recurring](a, http.StatusOK, "POST", recURL(a, paused.ID, "/activate"), nil)
	if !act.Active || act.NextOccurrenceOn != "2026-04-05" {
		t.Fatalf("activated: %+v", act)
	}
	mustRunJob(ts, "recurring")
	if n := len(invoicesByIssue(a)); n != 4 {
		t.Fatalf("activation generated backlog: %d", n)
	}
	deact := doJSON[api.Recurring](a, http.StatusOK, "POST", recURL(a, quarterly.ID, "/deactivate"), nil)
	if deact.Active {
		t.Fatalf("deactivate: %+v", deact)
	}
	ts.now = day(2026, 6, 2) // (the session would expire: check in the DB)
	mustRunJob(ts, "recurring")
	// the re-activated one catches up Apr 5 and May 5; the deactivated
	// quarterly one (next Jun 1) generates nothing
	var rows []model.Invoice
	if err := ts.db.Order("issued_on").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if got := rows[len(rows)-1].IssuedOn; got != "2026-05-05" || len(rows) != 6 {
		t.Fatalf("after deactivate: %d invoices, last %s", len(rows), got)
	}
}

func TestRecurringRunNow(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := newTemplate(a, subj.ID, "Služby {MONTH}/{YEAR}")
	r := newRecurring(a, api.RecurringCreate{Name: "R", TemplateID: tpl.ID, StartOn: "2026-04-01", Active: ptr(false)})

	inv := doJSON[api.Invoice](a, http.StatusCreated, "POST", recURL(a, r.ID, "/run-now"), nil)
	if inv.IssuedOn != "2026-03-15" || inv.Lines[0].Name != "Služby 03/2026" {
		t.Fatalf("run-now invoice: %+v", inv.InvoiceSummary)
	}
	r = doJSON[api.Recurring](a, http.StatusOK, "GET", recURL(a, r.ID, ""), nil)
	if r.NextOccurrenceOn != "2026-05-01" || *r.LastInvoiceID != inv.ID {
		t.Fatalf("after run-now: %+v", r)
	}
}

// TestRecurringJobFailure: a failing generation keeps next_occurrence_on,
// stores last_error and is retried on the next run.
func TestRecurringJobFailure(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	tpl := newTemplate(a, subj.ID, "X")
	r := newRecurring(a, api.RecurringCreate{Name: "R", TemplateID: tpl.ID, StartOn: "2026-03-01"})
	// break numbering: no default invoice number format
	if err := ts.db.Model(&model.NumberFormat{}).Where("document_type = ?", "invoice").Update("is_default", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := runJob(ts, "recurring"); err == nil {
		t.Fatal("job did not report the failure")
	}
	got := doJSON[api.Recurring](a, http.StatusOK, "GET", recURL(a, r.ID, ""), nil)
	if got.NextOccurrenceOn != "2026-03-01" || !strings.Contains(got.LastError, "number format") || got.LastInvoiceID != nil {
		t.Fatalf("after failure: %+v", got)
	}
	if err := ts.db.Model(&model.NumberFormat{}).Where("document_type = ?", "invoice").Update("is_default", true).Error; err != nil {
		t.Fatal(err)
	}
	mustRunJob(ts, "recurring")
	got = doJSON[api.Recurring](a, http.StatusOK, "GET", recURL(a, r.ID, ""), nil)
	if got.LastError != "" || got.NextOccurrenceOn != "2026-04-01" {
		t.Fatalf("after retry: %+v", got)
	}
}

func TestRecurringSendEmail(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	noMail := newSubject(a, api.SubjectCreate{Name: "Bez e-mailu"})
	tpl := newTemplate(a, subj.ID, "Hosting")
	tpl2 := newTemplate(a, noMail.ID, "Hosting")
	newRecurring(a, api.RecurringCreate{Name: "R", TemplateID: tpl.ID, StartOn: "2026-02-15", SendEmail: true})
	newRecurring(a, api.RecurringCreate{Name: "R2", TemplateID: tpl2.ID, StartOn: "2026-03-15", SendEmail: true})

	mustRunJob(ts, "recurring")
	msgs := ts.mail.Messages()
	if len(msgs) != 2 {
		t.Fatalf("messages: %d", len(msgs))
	}
	for _, m := range msgs {
		if m.To[0] != "klient@example.cz" || len(m.Attachments) != 1 || !strings.HasPrefix(string(m.Attachments[0].Data), "%PDF") {
			t.Fatalf("message: %+v", m.To)
		}
	}
	for _, inv := range invoicesByIssue(a) {
		logs := doJSON[api.ListResponse[api.EmailLog]](a, http.StatusOK, "GET", invURL(a, inv.ID, "/emails"), nil)
		if inv.SubjectID != nil && *inv.SubjectID == subj.ID {
			if inv.SentAt == nil || logs.Total != 1 || !logs.Items[0].Automatic || logs.Items[0].SentAt == nil {
				t.Fatalf("sent invoice: %+v %+v", inv.Status, logs)
			}
		} else if inv.SentAt != nil || logs.Total != 1 || !strings.Contains(logs.Items[0].Error, "no recipient") {
			t.Fatalf("no-recipient invoice: %+v %+v", inv.Status, logs)
		}
	}
}
