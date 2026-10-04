package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
)

func emailsOf(c *client, invID uint) api.ListResponse[api.EmailLog] {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.EmailLog]](c, http.StatusOK, "GET", invURL(c, invID, "/emails"), nil)
}

func TestSendInvoice(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.PublicURL = "https://faktury.example.cz" })
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"email": "info@firma-a.cz", "email_signature": "Jan Novák\n+420 123 456 789"})
	a.mustDo(http.StatusCreated, "POST", a.acct("/bank-accounts"), map[string]any{"name": "Hlavní", "number": "19-2000145399/0800"})
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz", EmailCopy: "ucetni@example.cz; klient@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("Práce", "1", 123456, nil)}})

	// defaults: recipients, template, PDF attachment, mark_as_sent
	log := doJSON[api.EmailLog](a, http.StatusOK, "POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{})
	m, ok := ts.mail.Last()
	if !ok {
		t.Fatal("nothing sent")
	}
	if strings.Join(m.To, ",") != "klient@example.cz" || strings.Join(m.Cc, ",") != "ucetni@example.cz" || m.ReplyTo != "info@firma-a.cz" {
		t.Fatalf("recipients: to %v cc %v reply %q", m.To, m.Cc, m.ReplyTo)
	}
	if m.Subject != "Faktura "+inv.Number+" – Firma A" {
		t.Fatalf("subject %q", m.Subject)
	}
	for _, want := range []string{
		"doklad č. " + inv.Number, "1\u00a0234,56\u00a0Kč", "29.\u00a03.\u00a02026",
		"https://faktury.example.cz/p/" + inv.PublicToken, "Číslo účtu: 19-2000145399/0800",
		"IBAN: CZ", "Variabilní symbol: " + inv.VariableSymbol, "Jan Novák\n+420 123 456 789",
	} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("body lacks %q:\n%s", want, m.Text)
		}
	}
	if strings.Contains(m.Text, "{") {
		t.Errorf("unrendered placeholder:\n%s", m.Text)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Filename != "faktura-"+inv.Number+".pdf" ||
		m.Attachments[0].ContentType != "application/pdf" || !strings.HasPrefix(string(m.Attachments[0].Data), "%PDF") {
		t.Fatalf("attachment: %+v", m.Attachments)
	}
	if log.SentAt == nil || log.Error != "" || log.Kind != "invoice" || log.Attachments[0] != m.Attachments[0].Filename || log.Automatic {
		t.Fatalf("log: %+v", log)
	}
	if got := getInv(a, inv.ID); got.Status != "sent" || got.SentAt == nil {
		t.Fatalf("not marked as sent: %s", got.Status)
	}

	// custom recipients/subject/body with placeholders, no PDF, reminder kind
	log = doJSON[api.EmailLog](a, http.StatusOK, "POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{
		To: []string{"Jiný <jiny@example.cz>"}, Subject: ptr("Připomínka {number}"), Body: ptr("Zbývá {remaining}, VS {vs}"),
		AttachPDF: ptr(false), Kind: "reminder",
	})
	m, _ = ts.mail.Last()
	if m.To[0] != "Jiný <jiny@example.cz>" || len(m.Cc) != 0 || m.Subject != "Připomínka "+inv.Number ||
		m.Text != "Zbývá 1\u00a0234,56\u00a0Kč, VS "+inv.VariableSymbol || len(m.Attachments) != 0 || log.Kind != "reminder" {
		t.Fatalf("custom: %+v / %+v", m, log)
	}

	// history, newest first
	if l := emailsOf(a, inv.ID); l.Total != 2 || l.Items[0].Kind != "reminder" || l.Items[1].Kind != "invoice" {
		t.Fatalf("history: %+v", l)
	}

	// validation
	res, body := a.do("POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{To: []string{"nope"}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "to[0]")
	noMail := newSubject(a, api.SubjectCreate{Name: "Bez"})
	inv2 := createInv(a, api.InvoiceCreate{SubjectID: new(noMail.ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	res, body = a.do("POST", invURL(a, inv2.ID, "/send"), api.InvoiceSend{})
	assertError(t, res, body, http.StatusUnprocessableEntity, "no recipient")

	// tenant isolation
	res, body = b.do("POST", invURL(b, inv.ID, "/send"), api.InvoiceSend{})
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
	res, body = b.do("GET", invURL(b, inv.ID, "/emails"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
	res, body = b.do("GET", b.acct("/email-templates/preview?invoice_id=1"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

func TestSendInvoiceMailerFailure(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	ts.mail.Err = errors.New("smtp down")
	res, body := a.do("POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{})
	assertError(t, res, body, http.StatusBadGateway, "smtp down")
	l := emailsOf(a, inv.ID)
	if l.Total != 1 || l.Items[0].Error != "smtp down" || l.Items[0].SentAt != nil {
		t.Fatalf("log: %+v", l)
	}
	if got := getInv(a, inv.ID); got.Status != "open" {
		t.Fatalf("status %s", got.Status)
	}
}

func TestSendRoles(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	member := ts.memberOf(owner, "member@example.cz", "member")
	accountant := ts.memberOf(owner, "acc@example.cz", "accountant")
	subj := newSubject(owner, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	inv := createInv(owner, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	member.mustDo(http.StatusOK, "POST", invURL(member, inv.ID, "/send"), api.InvoiceSend{})
	res, body := accountant.do("POST", invURL(accountant, inv.ID, "/send"), api.InvoiceSend{})
	assertError(t, res, body, http.StatusForbidden, "your role (accountant)")
	if l := emailsOf(accountant, inv.ID); l.Total != 1 {
		t.Fatalf("accountant history: %+v", l)
	}
	// e-mail settings are account settings: managers only
	res, body = member.do("PATCH", member.acct(""), map[string]any{"reminders_enabled": true})
	assertError(t, res, body, http.StatusForbidden, "your role (member)")
}

func TestEmailSettingsAndPreview(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	acc := doJSON[api.Account](a, http.StatusOK, "GET", a.acct(""), nil)
	if acc.RemindersEnabled || acc.PaidThanksEnabled || len(acc.ReminderDaysAfterDue) != 3 || len(acc.EmailTemplates) != 12 {
		t.Fatalf("defaults: %+v", acc.AccountEmailSettings)
	}
	for _, tpl := range acc.EmailTemplates {
		if tpl.Custom || tpl.Subject == "" || tpl.Body == "" {
			t.Fatalf("default template: %+v", tpl)
		}
	}

	acc = doJSON[api.Account](a, http.StatusOK, "PATCH", a.acct(""), map[string]any{
		"reminders_enabled": true, "reminder_days_after_due": []int{30, 7, 7}, "paid_thanks_enabled": true,
		"email_reply_to": "faktury@firma-a.cz",
		"email_templates": []map[string]string{
			{"kind": "invoice", "lang": "cs", "subject": "Vaše faktura {number} od {account_name}"},
			{"kind": "reminder", "lang": "en", "subject": "", "body": ""}, // empty = default, not stored
		},
	})
	if !acc.RemindersEnabled || !acc.PaidThanksEnabled || acc.EmailReplyTo != "faktury@firma-a.cz" ||
		len(acc.ReminderDaysAfterDue) != 2 || acc.ReminderDaysAfterDue[0] != 7 || acc.ReminderDaysAfterDue[1] != 30 {
		t.Fatalf("patched: %+v", acc.AccountEmailSettings)
	}
	custom := 0
	for _, tpl := range acc.EmailTemplates {
		if tpl.Custom {
			custom++
			if tpl.Kind != "invoice" || tpl.Lang != "cs" || !strings.HasPrefix(tpl.Subject, "Vaše faktura") || !strings.Contains(tpl.Body, "Dobrý den") {
				t.Fatalf("override: %+v", tpl)
			}
		}
	}
	if custom != 1 {
		t.Fatalf("custom templates: %d", custom)
	}

	for _, bad := range []map[string]any{
		{"reminder_days_after_due": []int{0}},
		{"reminder_days_after_due": []int{}},
		{"email_reply_to": "not an address"},
		{"email_templates": []map[string]string{{"kind": "bogus", "lang": "cs"}}},
	} {
		res, body := a.do("PATCH", a.acct(""), bad)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%v: %d %s", bad, res.StatusCode, body)
		}
	}

	// preview with sample data and with an invoice
	p := doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview"), nil)
	if p.Subject != "Vaše faktura 2026-0001 od Firma A" || p.Lang != "cs" || len(p.To) != 0 {
		t.Fatalf("sample preview: %+v", p)
	}
	p = doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?kind=reminder&lang=en"), nil)
	if !strings.HasPrefix(p.Subject, "Reminder – invoice 2026-0001") || !strings.Contains(p.Body, "5 days overdue") {
		t.Fatalf("reminder preview: %+v", p)
	}
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz", EmailCopy: "kopie@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), DocumentType: "proforma", Language: "en", Lines: []api.InvoiceLineInput{line("Work", "1", 1000, nil)}})
	p = doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?kind=paid_thanks&invoice_id="+uintStr(inv.ID)), nil)
	if p.Lang != "en" || p.Subject != "Thank you for paying proforma invoice "+inv.Number || p.To[0] != "klient@example.cz" || p.Cc[0] != "kopie@example.cz" {
		t.Fatalf("invoice preview: %+v", p)
	}
	// e-mail added to the contact after the invoice was created → used as the recipient
	late := newSubject(a, api.SubjectCreate{Name: "Bez e-mailu"})
	lateInv := createInv(a, api.InvoiceCreate{SubjectID: new(late.ID), Lines: []api.InvoiceLineInput{line("Work", "1", 1000, nil)}})
	email := "pozdeji@example.cz"
	a.mustDo(http.StatusOK, "PATCH", fmt.Sprintf("%s/%d", a.acct("/subjects"), late.ID), api.SubjectPatch{Email: &email})
	p = doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?invoice_id="+uintStr(lateInv.ID)), nil)
	if len(p.To) != 1 || p.To[0] != email {
		t.Fatalf("late contact e-mail: %+v", p.To)
	}
	res, body := a.do("GET", a.acct("/email-templates/preview?kind=nope"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "kind")

	// Slovak and German documents get their own texts
	sk := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Language: "sk", Lines: []api.InvoiceLineInput{line("Práca", "1", 1000, nil)}})
	p = doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?invoice_id="+uintStr(sk.ID)), nil)
	if p.Lang != "sk" || !strings.HasPrefix(p.Subject, "Faktúra "+sk.Number) || !strings.HasPrefix(p.Body, "Dobrý deň") || !strings.Contains(p.Body, "Suma na úhradu") {
		t.Fatalf("sk preview: %+v", p)
	}
	p = doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?kind=reminder&lang=de"), nil)
	if p.Lang != "de" || p.Subject != "Zahlungserinnerung – Rechnung 2026-0001" || !strings.Contains(p.Body, "5 Tage überfällig") ||
		!strings.Contains(p.Body, "Kontonummer: 123456789/0800") {
		t.Fatalf("de preview: %+v", p)
	}

	// unsaved texts override the stored template
	p = doJSON[api.EmailPreview](a, http.StatusOK, "GET", a.acct("/email-templates/preview?subject=Test+%7Bnumber%7D&body=Ahoj+%7Baccount_name%7D"), nil)
	if p.Subject != "Test 2026-0001" || !strings.HasPrefix(p.Body, "Ahoj Firma A") {
		t.Fatalf("override preview: %+v", p)
	}
	defs := doJSON[api.EmailTemplateDefaults](a, http.StatusOK, "GET", a.acct("/email-templates/defaults"), nil)
	if len(defs.Items) != 12 || defs.Items[0].Kind != "invoice" || defs.Items[0].Lang != "cs" || !strings.Contains(defs.Items[0].Subject, "{number}") {
		t.Fatalf("defaults: %+v", defs)
	}
}

func uintStr(v uint) string { return strconv.FormatUint(uint64(v), 10) }

// TestReminderJob: reminders go out once per reached step, only for
// overdue unpaid invoices with a client e-mail in accounts that enabled them.
func TestReminderJob(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B") // reminders disabled
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"reminders_enabled": true})
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	noMail := newSubject(a, api.SubjectCreate{Name: "Bez"})
	mk := func(c *client, subjectID uint, issued string) api.Invoice {
		return createInv(c, api.InvoiceCreate{SubjectID: new(subjectID), IssuedOn: issued, DueDays: intPtr(0),
			Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	}
	overdue := mk(a, subj.ID, "2026-03-01")   // 14 days overdue on 2026-03-15
	fresh := mk(a, subj.ID, "2026-03-14")     // 1 day: below the first step
	paid := mk(a, subj.ID, "2026-03-01")      // paid
	cancelled := mk(a, subj.ID, "2026-03-01") // cancelled
	mk(a, noMail.ID, "2026-03-01")            // no e-mail
	mk(b, newSubject(b, api.SubjectCreate{Name: "B", Email: "b-klient@example.cz"}).ID, "2026-03-01")
	pay(a, paid.ID, api.PaymentCreate{})
	action(a, cancelled.ID, "cancel")

	mustRunJob(ts, "reminders")
	mustRunJob(ts, "reminders") // idempotent
	msgs := ts.mail.Messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Subject, overdue.Number) || len(msgs[0].Attachments) != 1 {
		t.Fatalf("messages: %+v", msgs)
	}
	l := emailsOf(a, overdue.ID)
	if l.Total != 1 || l.Items[0].ReminderStep != 14 || !l.Items[0].Automatic || l.Items[0].Kind != "reminder" {
		t.Fatalf("log: %+v", l)
	}
	if got := getInv(a, overdue.ID); got.SentAt != nil {
		t.Fatal("a reminder must not mark as sent")
	}

	ts.now = day(2026, 3, 18) // fresh: 4 days → step 3
	mustRunJob(ts, "reminders")
	if n := len(ts.mail.Messages()); n != 2 {
		t.Fatalf("after step 3: %d", n)
	}
	if l := emailsOf(a, fresh.ID); l.Total != 1 || l.Items[0].ReminderStep != 3 {
		t.Fatalf("fresh: %+v", l)
	}

	ts.now = day(2026, 3, 31) // overdue: 30 days → step 30; fresh: 17 days → step 14
	mustRunJob(ts, "reminders")
	mustRunJob(ts, "reminders")
	if n := len(ts.mail.Messages()); n != 4 {
		t.Fatalf("after day 31: %d", n)
	}

	// a failed attempt is retried after 24 h, not on every run
	ts.mail.Err = errors.New("smtp down")
	ts.now = day(2026, 4, 17) // fresh: 34 days → step 30
	if err := runJob(ts, "reminders"); err == nil {
		t.Fatal("failure not reported")
	}
	if err := runJob(ts, "reminders"); err != nil {
		t.Fatalf("retried within 24 h: %v", err)
	}
	ts.mail.Err = nil
	ts.now = ts.now.AddDate(0, 0, 1)
	mustRunJob(ts, "reminders")
	if n := len(ts.mail.Messages()); n != 5 {
		t.Fatalf("after retry: %d", n)
	}
}

func TestPaidThanks(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	mk := func() api.Invoice {
		return createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	}
	off := mk()
	pay(a, off.ID, api.PaymentCreate{})
	if n := len(ts.mail.Messages()); n != 0 {
		t.Fatalf("sent while disabled: %d", n)
	}

	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"paid_thanks_enabled": true})
	inv := mk()
	pay(a, inv.ID, api.PaymentCreate{Amount: i64(400)}) // partial
	if n := len(ts.mail.Messages()); n != 0 {
		t.Fatalf("sent on partial payment: %d", n)
	}
	pay(a, inv.ID, api.PaymentCreate{}) // rest
	m, ok := ts.mail.Last()
	if !ok || m.Subject != "Děkujeme za úhradu dokladu "+inv.Number || len(m.Attachments) != 0 || !strings.Contains(m.Text, "10,00\u00a0Kč") {
		t.Fatalf("thanks: %+v", m)
	}
	if l := emailsOf(a, inv.ID); l.Total != 1 || l.Items[0].Kind != "paid_thanks" || !l.Items[0].Automatic {
		t.Fatalf("log: %+v", l)
	}
	// a failing mailer does not break the payment
	ts.mail.Err = errors.New("smtp down")
	pay(a, mk().ID, api.PaymentCreate{})
}
