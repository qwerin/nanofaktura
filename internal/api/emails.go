package api

import (
	"context"
	"errors"
	"fmt"
	"maps"
	netmail "net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

// ---- account settings (embedded in Account / AccountPatch) ----

// EmailTemplate is the effective text of one e-mail kind in one language.
// Placeholders: {number} {document} {document_title} {total} {remaining}
// {issued_on} {due_on} {days_overdue} {public_url} {account_name}
// {client_name} {vs} {iban} {bank_account} {payment_info}.
type EmailTemplate struct {
	Kind    string `json:"kind" enum:"invoice,reminder,paid_thanks"`
	Lang    string `json:"lang" enum:"cs,en,sk,de"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Custom  bool   `json:"custom" doc:"false = built-in default text"`
}

// EmailTemplateInput overrides one template (PATCH account).
type EmailTemplateInput struct {
	Kind    string `json:"kind" enum:"invoice,reminder,paid_thanks"`
	Lang    string `json:"lang" enum:"cs,en,sk,de"`
	Subject string `json:"subject,omitempty" maxLength:"500" doc:"Empty = default subject"`
	Body    string `json:"body,omitempty" maxLength:"20000" doc:"Empty = default body"`
}

// AccountEmailSettings are the e-mail and reminder settings of the account (SPEC §7.4).
type AccountEmailSettings struct {
	EmailReplyTo         string          `json:"email_reply_to" doc:"Reply-To of invoice e-mails; empty = account email"`
	EmailSignature       string          `json:"email_signature" doc:"Appended to template texts"`
	EmailTemplates       []EmailTemplate `json:"email_templates" nullable:"false" doc:"Effective texts (kind × language)"`
	RemindersEnabled     bool            `json:"reminders_enabled"`
	ReminderDaysAfterDue []int           `json:"reminder_days_after_due" nullable:"false" doc:"Automatic reminder steps (days after due_on)"`
	PaidThanksEnabled    bool            `json:"paid_thanks_enabled" doc:"Send a thank-you e-mail when an invoice gets fully paid"`
}

// AccountEmailSettingsPatch: nil = unchanged.
type AccountEmailSettingsPatch struct {
	EmailReplyTo         *string              `json:"email_reply_to,omitempty" maxLength:"254"`
	EmailSignature       *string              `json:"email_signature,omitempty" maxLength:"2000"`
	EmailTemplates       []EmailTemplateInput `json:"email_templates,omitempty" maxItems:"20" doc:"Replaces all overrides; entries equal to the default (or empty) are not stored"`
	RemindersEnabled     *bool                `json:"reminders_enabled,omitempty"`
	ReminderDaysAfterDue []int                `json:"reminder_days_after_due,omitempty" minItems:"1" maxItems:"10" doc:"Days after due_on, 1–365"`
	PaidThanksEnabled    *bool                `json:"paid_thanks_enabled,omitempty"`
}

var (
	emailKinds = []string{model.EmailInvoice, model.EmailReminder, model.EmailPaidThanks}
	emailLangs = []string{"cs", "en", "sk", "de"}
	// defaultReminderDays are the reminder steps when the account has none set.
	defaultReminderDays = []int{3, 14, 30}
)

func toAccountEmailSettings(a *model.Account) AccountEmailSettings {
	out := AccountEmailSettings{
		EmailReplyTo: a.EmailReplyTo, EmailSignature: a.EmailSignature, RemindersEnabled: a.RemindersEnabled,
		ReminderDaysAfterDue: reminderSteps(a), PaidThanksEnabled: a.PaidThanksEnabled,
	}
	for _, k := range emailKinds {
		for _, l := range emailLangs {
			t, custom := emailTemplate(a, k, l)
			out.EmailTemplates = append(out.EmailTemplates, EmailTemplate{Kind: k, Lang: l, Subject: t.Subject, Body: t.Body, Custom: custom})
		}
	}
	return out
}

// applyEmailSettings applies the e-mail part of an account PATCH.
func applyEmailSettings(a *model.Account, p *AccountEmailSettingsPatch) error {
	if p.EmailReplyTo != nil {
		v := strings.TrimSpace(*p.EmailReplyTo)
		if v != "" {
			if _, err := netmail.ParseAddress(v); err != nil {
				return invalid("email_reply_to", "invalid e-mail address")
			}
		}
		a.EmailReplyTo = v
	}
	apply(&a.EmailSignature, p.EmailSignature)
	apply(&a.RemindersEnabled, p.RemindersEnabled)
	apply(&a.PaidThanksEnabled, p.PaidThanksEnabled)
	if p.ReminderDaysAfterDue != nil {
		days := slices.Clone(p.ReminderDaysAfterDue)
		for i, d := range days {
			if d < 1 || d > 365 {
				return invalid(fmt.Sprintf("reminder_days_after_due[%d]", i), "must be 1–365")
			}
		}
		slices.Sort(days)
		a.ReminderDaysAfterDue = slices.Compact(days)
	}
	if p.EmailTemplates != nil {
		over := map[string]model.MailTemplate{}
		for _, t := range p.EmailTemplates {
			def := defaultEmailTemplates[t.Kind+":"+t.Lang]
			if t.Subject == def.Subject {
				t.Subject = ""
			}
			if t.Body == def.Body {
				t.Body = ""
			}
			if strings.TrimSpace(t.Subject) == "" && strings.TrimSpace(t.Body) == "" {
				continue
			}
			over[t.Kind+":"+t.Lang] = model.MailTemplate{Subject: t.Subject, Body: t.Body}
		}
		a.EmailTemplates = over
	}
	return nil
}

// reminderSteps returns the account's reminder steps (sorted), default [3, 14, 30].
func reminderSteps(a *model.Account) []int {
	if len(a.ReminderDaysAfterDue) == 0 {
		return slices.Clone(defaultReminderDays)
	}
	return slices.Sorted(slices.Values(a.ReminderDaysAfterDue))
}

// emailTemplate returns the effective template (override parts win over the
// default) and whether an override exists.
func emailTemplate(a *model.Account, kind, lang string) (model.MailTemplate, bool) {
	if !slices.Contains(emailLangs, lang) {
		lang = "cs"
	}
	t := defaultEmailTemplates[kind+":"+lang]
	o, ok := a.EmailTemplates[kind+":"+lang]
	if !ok {
		return t, false
	}
	if strings.TrimSpace(o.Subject) != "" {
		t.Subject = o.Subject
	}
	if strings.TrimSpace(o.Body) != "" {
		t.Body = o.Body
	}
	return t, true
}

// defaultEmailTemplates are the built-in texts ("kind:lang"). Czech texts
// avoid inflecting placeholders (the document name stays in nominative).
var defaultEmailTemplates = map[string]model.MailTemplate{
	"invoice:cs": {
		Subject: "{document_title} {number} – {account_name}",
		Body: "Dobrý den,\n\nv příloze posíláme doklad č. {number} ({document}).\n\n" +
			"Částka k úhradě: {remaining}\nDatum splatnosti: {due_on}\n{payment_info}\n\n" +
			"Doklad si můžete zobrazit i online: {public_url}\n\nDěkujeme a přejeme hezký den.\n{account_name}",
	},
	"invoice:en": {
		Subject: "{document_title} {number} – {account_name}",
		Body: "Hello,\n\nplease find attached {document} {number}.\n\n" +
			"Amount due: {remaining}\nDue date: {due_on}\n{payment_info}\n\n" +
			"You can also view it online: {public_url}\n\nThank you,\n{account_name}",
	},
	"reminder:cs": {
		Subject: "Upomínka – doklad {number} po splatnosti",
		Body: "Dobrý den,\n\ndovolujeme si připomenout, že doklad č. {number} se splatností {due_on} " +
			"dosud není uhrazen (po splatnosti {days_overdue} dní).\n\nZbývá uhradit: {remaining}\n{payment_info}\n\n" +
			"Doklad najdete v příloze i online: {public_url}\n\n" +
			"Pokud jste již platbu odeslali, považujte prosím tuto zprávu za bezpředmětnou.\n\nS pozdravem\n{account_name}",
	},
	"reminder:en": {
		Subject: "Reminder – {document} {number} is overdue",
		Body: "Hello,\n\nthis is a friendly reminder that {document} {number} was due on {due_on} " +
			"and has not been paid yet ({days_overdue} days overdue).\n\nAmount due: {remaining}\n{payment_info}\n\n" +
			"The document is attached and available online: {public_url}\n\n" +
			"If you have already paid, please disregard this message.\n\nKind regards,\n{account_name}",
	},
	"paid_thanks:cs": {
		Subject: "Děkujeme za úhradu dokladu {number}",
		Body:    "Dobrý den,\n\nděkujeme, platbu za doklad č. {number} ve výši {total} jsme přijali.\n\nS pozdravem\n{account_name}",
	},
	"paid_thanks:en": {
		Subject: "Thank you for paying {document} {number}",
		Body:    "Hello,\n\nthank you, we have received your payment of {total} for {document} {number}.\n\nKind regards,\n{account_name}",
	},
	"invoice:sk": {
		Subject: "{document_title} {number} – {account_name}",
		Body: "Dobrý deň,\n\nv prílohe vám posielame doklad č. {number} ({document}).\n\n" +
			"Suma na úhradu: {remaining}\nDátum splatnosti: {due_on}\n{payment_info}\n\n" +
			"Doklad si môžete pozrieť aj online: {public_url}\n\nĎakujeme a prajeme pekný deň.\n{account_name}",
	},
	"reminder:sk": {
		Subject: "Upomienka – doklad {number} po splatnosti",
		Body: "Dobrý deň,\n\ndovoľujeme si pripomenúť, že doklad č. {number} so splatnosťou {due_on} " +
			"zatiaľ nie je uhradený (po splatnosti {days_overdue} dní).\n\nZostáva uhradiť: {remaining}\n{payment_info}\n\n" +
			"Doklad nájdete v prílohe aj online: {public_url}\n\n" +
			"Ak ste už platbu odoslali, považujte, prosím, túto správu za bezpredmetnú.\n\nS pozdravom\n{account_name}",
	},
	"paid_thanks:sk": {
		Subject: "Ďakujeme za úhradu dokladu {number}",
		Body:    "Dobrý deň,\n\nďakujeme, platbu za doklad č. {number} vo výške {total} sme prijali.\n\nS pozdravom\n{account_name}",
	},
	// German document names are all feminine (die Rechnung/Proformarechnung/Rechnungskorrektur).
	"invoice:de": {
		Subject: "{document_title} {number} – {account_name}",
		Body: "Guten Tag,\n\nanbei erhalten Sie die {document_title} Nr. {number}.\n\n" +
			"Offener Betrag: {remaining}\nFällig am: {due_on}\n{payment_info}\n\n" +
			"Sie können das Dokument auch online ansehen: {public_url}\n\nVielen Dank und freundliche Grüße\n{account_name}",
	},
	"reminder:de": {
		Subject: "Zahlungserinnerung – {document_title} {number}",
		Body: "Guten Tag,\n\nwir möchten Sie freundlich daran erinnern, dass die {document_title} Nr. {number} am {due_on} " +
			"fällig war und noch nicht beglichen ist ({days_overdue} Tage überfällig).\n\nOffener Betrag: {remaining}\n{payment_info}\n\n" +
			"Das Dokument finden Sie im Anhang und online: {public_url}\n\n" +
			"Sollten Sie die Zahlung bereits veranlasst haben, betrachten Sie diese Nachricht bitte als gegenstandslos.\n\n" +
			"Mit freundlichen Grüßen\n{account_name}",
	},
	"paid_thanks:de": {
		Subject: "Vielen Dank für Ihre Zahlung – {document_title} {number}",
		Body: "Guten Tag,\n\nvielen Dank, wir haben Ihre Zahlung über {total} für die {document_title} Nr. {number} erhalten.\n\n" +
			"Mit freundlichen Grüßen\n{account_name}",
	},
}

var documentNames = map[string]map[string][2]string{ // type → lang → {lowercase, title}
	model.DocInvoice: {"cs": {"faktura", "Faktura"}, "en": {"invoice", "Invoice"},
		"sk": {"faktúra", "Faktúra"}, "de": {"Rechnung", "Rechnung"}},
	model.DocProforma: {"cs": {"zálohová faktura", "Zálohová faktura"}, "en": {"proforma invoice", "Proforma invoice"},
		"sk": {"zálohová faktúra", "Zálohová faktúra"}, "de": {"Proformarechnung", "Proformarechnung"}},
	model.DocCorrection: {"cs": {"opravný daňový doklad", "Opravný daňový doklad"}, "en": {"credit note", "Credit note"},
		"sk": {"opravný daňový doklad", "Opravný daňový doklad"}, "de": {"Rechnungskorrektur", "Rechnungskorrektur"}},
	model.DocTaxDocument: {"cs": {"daňový doklad k přijaté platbě", "Daňový doklad k přijaté platbě"},
		"en": {"tax document for a received payment", "Tax document for a received payment"},
		"sk": {"daňový doklad k prijatej platbe", "Daňový doklad k prijatej platbe"},
		"de": {"Steuerbeleg über erhaltene Anzahlung", "Steuerbeleg über erhaltene Anzahlung"}},
}

// nonPayerCorrection: a non-payer's correction is an "opravná faktura" (no tax document).
var nonPayerCorrection = map[string][2]string{
	"cs": {"opravná faktura", "Opravná faktura"}, "sk": {"opravná faktúra", "Opravná faktúra"},
	"en": {"credit note", "Credit note"}, "de": {"Gutschrift", "Gutschrift"},
}

// emailLang is the e-mail language of an invoice: the document language
// (cs|en|sk|de), Czech for anything else.
func emailLang(inv *model.Invoice) string {
	if slices.Contains(emailLangs, inv.Language) {
		return inv.Language
	}
	return "cs"
}

// emailVars are the placeholder values of an invoice e-mail.
func (s *server) emailVars(acc *model.Account, inv *model.Invoice, today string) map[string]string {
	lang := emailLang(inv)
	names := documentNames[inv.DocumentType][lang]
	if names[0] == "" {
		names = documentNames[model.DocInvoice][lang]
	}
	if inv.DocumentType == model.DocCorrection && inv.YourVatMode != model.VatModePayer {
		names = nonPayerCorrection[lang]
	}
	overdue := 0
	if inv.DueOn != "" && inv.DueOn < today {
		due, _ := time.Parse(billing.DateLayout, inv.DueOn)
		t, _ := time.Parse(billing.DateLayout, today)
		overdue = int(t.Sub(due).Hours() / 24)
	}
	remaining := inv.Total - inv.PaidAmount
	var pay []string
	if inv.PaymentMethod == "bank" {
		labels := map[string][3]string{"cs": {"Číslo účtu", "IBAN", "Variabilní symbol"}, "en": {"Bank account", "IBAN", "Payment reference"},
			"sk": {"Číslo účtu", "IBAN", "Variabilný symbol"}, "de": {"Kontonummer", "IBAN", "Zahlungsreferenz"}}[lang]
		for i, v := range []string{inv.BankAccount, inv.IBAN, inv.VariableSymbol} {
			if v != "" {
				pay = append(pay, labels[i]+": "+v)
			}
		}
	}
	return map[string]string{
		"number": inv.Number, "document": names[0], "document_title": names[1],
		"total":     pdf.FormatMoney(inv.Total, inv.Currency, lang),
		"remaining": pdf.FormatMoney(remaining, inv.Currency, lang),
		"issued_on": pdf.FormatDate(inv.IssuedOn, lang), "due_on": pdf.FormatDate(inv.DueOn, lang),
		"days_overdue": strconv.Itoa(overdue),
		"public_url":   s.publicURL() + "/p/" + inv.PublicToken,
		"account_name": acc.Name, "client_name": inv.ClientName,
		"vs": inv.VariableSymbol, "iban": inv.IBAN, "bank_account": inv.BankAccount,
		"payment_info": strings.Join(pay, "\n"),
	}
}

// renderEmail renders the subject and body of kind for inv; custom
// subject/body (from the send request) replace the template and get no signature.
func (s *server) renderEmail(acc *model.Account, inv *model.Invoice, kind string, subject, body *string, today string) (string, string) {
	vars := s.emailVars(acc, inv, today)
	t, _ := emailTemplate(acc, kind, emailLang(inv))
	subj := t.Subject
	apply(&subj, subject)
	var text string
	if body != nil {
		text = mail.Render(*body, vars)
	} else {
		text = mail.Render(t.Body, vars)
		if sig := strings.TrimSpace(acc.EmailSignature); sig != "" {
			text = strings.TrimRight(text, "\n ") + "\n\n" + sig
		}
	}
	return strings.TrimSpace(mail.Render(subj, vars)), cleanBlankLines(text)
}

// cleanBlankLines collapses runs of blank lines (left by empty placeholders).
func cleanBlankLines(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := lines[:0]
	blank := false
	for _, l := range lines {
		b := strings.TrimSpace(l) == ""
		if b && blank {
			continue
		}
		blank = b
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ---- e-mail log DTO ----

// EmailLog is one e-mail sent (or attempted) about an invoice.
type EmailLog struct {
	ID           uint       `json:"id"`
	InvoiceID    uint       `json:"invoice_id"`
	Kind         string     `json:"kind" enum:"invoice,reminder,paid_thanks"`
	To           []string   `json:"to" nullable:"false"`
	Cc           []string   `json:"cc" nullable:"false"`
	Subject      string     `json:"subject"`
	Body         string     `json:"body"`
	Attachments  []string   `json:"attachments" nullable:"false"`
	ReminderStep int        `json:"reminder_step" doc:"Automatic reminder: days after due; 0 otherwise"`
	Automatic    bool       `json:"automatic" doc:"Sent by the scheduler or an automatic rule"`
	SentAt       *time.Time `json:"sent_at,omitempty" doc:"Omitted when sending failed"`
	Error        string     `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func toEmailLog(m *model.EmailLog) EmailLog {
	return EmailLog{
		ID: m.ID, InvoiceID: m.InvoiceID, Kind: m.Kind, To: nonNil(m.To), Cc: nonNil(m.Cc), Subject: m.Subject,
		Body: m.Body, Attachments: nonNil(m.Attachments), ReminderStep: m.ReminderStep, Automatic: m.Automatic,
		SentAt: m.SentAt, Error: m.Error, CreatedAt: m.CreatedAt,
	}
}

// InvoiceSend is the body of POST /invoices/{id}/send.
type InvoiceSend struct {
	To        []string `json:"to,omitempty" maxItems:"10" doc:"Default: client_email of the invoice (+ the subject's email_copy as cc); at most 10 recipients in to + cc"`
	Cc        []string `json:"cc,omitempty" maxItems:"10"`
	Subject   *string  `json:"subject,omitempty" maxLength:"500" doc:"Default: the account template of kind in the invoice language"`
	Body      *string  `json:"body,omitempty" maxLength:"20000" doc:"Default: the template + signature; placeholders are rendered here too"`
	AttachPDF *bool    `json:"attach_pdf,omitempty" doc:"Default true"`
	// ISDOC 6.0.2 XML of the invoice as a second attachment.
	AttachISDOC bool   `json:"attach_isdoc,omitempty" doc:"Also attach the ISDOC XML (default false)"`
	Kind        string `json:"kind,omitempty" enum:"invoice,reminder,paid_thanks" doc:"Default invoice; invoice marks an open invoice as sent"`
}

// EmailPreview is a rendered e-mail (GET /email-templates/preview).
type EmailPreview struct {
	Kind    string   `json:"kind"`
	Lang    string   `json:"lang"`
	To      []string `json:"to" nullable:"false"`
	Cc      []string `json:"cc" nullable:"false"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

// ---- routes ----

func (s *server) registerEmails(g huma.API) {
	huma.Post(g, "/invoices/{id}/send", s.sendInvoice, auth.ForEditors)
	huma.Get(g, "/invoices/{id}/emails", s.listInvoiceEmails)
	huma.Get(g, "/email-templates/preview", s.previewEmail)
	huma.Get(g, "/email-templates/defaults", s.defaultEmailTemplatesList)
}

// EmailTemplateDefault is a built-in e-mail text ("Obnovit výchozí").
type EmailTemplateDefault struct {
	Kind    string `json:"kind" enum:"invoice,reminder,paid_thanks"`
	Lang    string `json:"lang" enum:"cs,en,sk,de"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type EmailTemplateDefaults struct {
	Items []EmailTemplateDefault `json:"items" nullable:"false"`
}

func (s *server) defaultEmailTemplatesList(_ context.Context, _ *struct{}) (*Out[EmailTemplateDefaults], error) {
	out := EmailTemplateDefaults{Items: []EmailTemplateDefault{}}
	for _, kind := range emailKinds {
		for _, lang := range emailLangs {
			t := defaultEmailTemplates[kind+":"+lang]
			out.Items = append(out.Items, EmailTemplateDefault{Kind: kind, Lang: lang, Subject: t.Subject, Body: t.Body})
		}
	}
	return &Out[EmailTemplateDefaults]{Body: out}, nil
}

func (s *server) sendInvoice(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body InvoiceSend
}) (*Out[EmailLog], error) {
	db := s.db.WithContext(ctx)
	inv, err := loadInvoice(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	if err := notDraft(inv); err != nil {
		return nil, err
	}
	b := &in.Body
	to, cc := b.To, b.Cc
	if len(to) == 0 && len(cc) == 0 {
		to, cc = s.defaultRecipients(ctx, db, inv)
	}
	if to, err = normalizeAddresses("to", to); err != nil {
		return nil, err
	}
	if cc, err = normalizeAddresses("cc", cc); err != nil {
		return nil, err
	}
	if len(to) == 0 {
		return nil, invalid("to", "no recipient: the invoice has no client e-mail; set to")
	}
	if len(to)+len(cc) > maxRecipients {
		return nil, invalid("to", fmt.Sprintf("at most %d recipients (to + cc)", maxRecipients))
	}
	if err := s.rateLimit(s.limits.mail, accountKey(ctx)); err != nil {
		return nil, err
	}
	attach := true
	apply(&attach, b.AttachPDF)
	log, err := s.sendInvoiceEmail(ctx, inv, emailRequest{
		kind: defaultStr(b.Kind, model.EmailInvoice), to: to, cc: cc, subject: b.Subject, body: b.Body, attachPDF: attach,
		attachISDOC: b.AttachISDOC,
	})
	if err != nil {
		var se huma.StatusError
		if errors.As(err, &se) {
			return nil, err
		}
		return nil, huma.Error502BadGateway("failed to send the e-mail: "+err.Error(), err)
	}
	return &Out[EmailLog]{Body: toEmailLog(log)}, nil
}

func (s *server) listInvoiceEmails(ctx context.Context, in *struct {
	ID uint `path:"id"`
	PageParams
}) (*Out[ListResponse[EmailLog]], error) {
	var inv model.Invoice
	if err := s.scoped(ctx).Select("id").First(&inv, in.ID).Error; err != nil {
		return nil, dbErr(err, "invoice")
	}
	q := s.scoped(ctx).Model(&model.EmailLog{}).Where("invoice_id = ?", inv.ID).Order("id DESC")
	return paginate(q, in.PageParams, toEmailLog)
}

func (s *server) previewEmail(ctx context.Context, in *struct {
	Kind      string `query:"kind" enum:"invoice,reminder,paid_thanks" default:"invoice"`
	Lang      string `query:"lang" enum:"cs,en,sk,de" doc:"Default: the invoice language (or the account default)"`
	InvoiceID uint   `query:"invoice_id" doc:"Render with this invoice; omitted = sample data"`
	Subject   string `query:"subject" maxLength:"500" doc:"Unsaved subject template to render instead of the stored one"`
	BodyText  string `query:"body" maxLength:"10000" doc:"Unsaved body template to render instead of the stored one"`
}) (*Out[EmailPreview], error) {
	acc := auth.AccountFrom(ctx)
	db := s.db.WithContext(ctx)
	var inv *model.Invoice
	to, cc := []string{}, []string{}
	if in.InvoiceID != 0 {
		var err error
		if inv, err = loadInvoice(ctx, db, in.InvoiceID); err != nil {
			return nil, err
		}
		to, cc = s.defaultRecipients(ctx, db, inv)
	} else {
		inv = sampleEmailInvoice(acc, s.today())
	}
	if in.Lang != "" {
		inv.Language = in.Lang
	}
	if in.Subject != "" || in.BodyText != "" {
		// render unsaved texts as if they were the stored template (incl. signature)
		cp := *acc
		cp.EmailTemplates = maps.Clone(acc.EmailTemplates)
		if cp.EmailTemplates == nil {
			cp.EmailTemplates = map[string]model.MailTemplate{}
		}
		key := in.Kind + ":" + emailLang(inv)
		t := cp.EmailTemplates[key]
		if in.Subject != "" {
			t.Subject = in.Subject
		}
		if in.BodyText != "" {
			t.Body = in.BodyText
		}
		cp.EmailTemplates[key] = t
		acc = &cp
	}
	subj, body := s.renderEmail(acc, inv, in.Kind, nil, nil, s.today())
	return &Out[EmailPreview]{Body: EmailPreview{
		Kind: in.Kind, Lang: emailLang(inv), To: nonNil(to), Cc: nonNil(cc), Subject: subj, Body: body,
	}}, nil
}

// sampleEmailInvoice is placeholder data for previews without an invoice.
func sampleEmailInvoice(acc *model.Account, today string) *model.Invoice {
	due, _ := billing.AddDays(today, -5)
	issued, _ := billing.AddDays(today, -19)
	return &model.Invoice{
		DocumentType: model.DocInvoice, Number: "2026-0001", VariableSymbol: "20260001",
		ClientName: "Vzorový klient s.r.o.", IssuedOn: issued, DueOn: due, Total: 1210000, PaidAmount: 0,
		Currency: defaultStr(acc.DefaultCurrency, "CZK"), Language: defaultStr(acc.DefaultLanguage, "cs"),
		PaymentMethod: "bank", BankAccount: "123456789/0800", IBAN: "CZ6508000000000123456789",
		PublicToken: "ukazka",
	}
}

// ---- sending ----

// emailRequest describes one invoice e-mail to send.
type emailRequest struct {
	kind          string
	to, cc        []string
	subject, body *string // nil = template
	attachPDF     bool
	attachISDOC   bool
	reminderStep  int  // automatic reminders
	automatic     bool // scheduler / automatic rule
}

// errNoRecipients: automatic e-mail for an invoice without a client e-mail.
var errNoRecipients = errors.New("no recipient: the invoice has no client e-mail")

// defaultRecipients: to = invoice client_email, cc = the subject's email_copy
// (comma/semicolon separated, duplicates dropped).
func (s *server) defaultRecipients(ctx context.Context, db *gorm.DB, inv *model.Invoice) (to, cc []string) {
	to = []string{}
	cc = []string{}
	if e := strings.TrimSpace(inv.ClientEmail); e != "" {
		to = append(to, e)
	}
	var subj model.Subject
	if err := db.Scopes(inAccount(ctx)).Select("id", "email_copy").First(&subj, inv.SubjectID).Error; err == nil {
		for _, e := range strings.FieldsFunc(subj.EmailCopy, func(r rune) bool { return r == ',' || r == ';' }) {
			if e = strings.TrimSpace(e); e != "" && !slices.Contains(to, e) && !slices.Contains(cc, e) {
				cc = append(cc, e)
			}
		}
	}
	return to, cc
}

// maxRecipients bounds to + cc of one user-sent e-mail (the server must not
// become a bulk mailer); user-sent e-mails are also rate limited per account.
const maxRecipients = 10

// accountKey is the rate limit key of the current account.
func accountKey(ctx context.Context) string {
	return "a" + strconv.FormatUint(uint64(auth.AccountFrom(ctx).ID), 10)
}

// normalizeAddresses validates e-mail addresses → 422 on field[i].
func normalizeAddresses(field string, list []string) ([]string, error) {
	out := []string{}
	for i, a := range list {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, err := netmail.ParseAddress(a); err != nil {
			return nil, invalid(fmt.Sprintf("%s[%d]", field, i), "invalid e-mail address")
		}
		out = append(out, a)
	}
	return out, nil
}

// sendInvoiceEmail renders, sends and logs one e-mail about inv (loaded with
// lines and payments) in the account of ctx. Every attempt is written to
// EmailLog (with the error when the mailer fails, which is also returned).
// A successful kind=invoice e-mail marks an open invoice as sent.
func (s *server) sendInvoiceEmail(ctx context.Context, inv *model.Invoice, req emailRequest) (*model.EmailLog, error) {
	acc := auth.AccountFrom(ctx)
	now := s.deps.Now()
	subject, body := s.renderEmail(acc, inv, req.kind, req.subject, req.body, billing.Today(now))
	log := &model.EmailLog{
		AccountID: acc.ID, InvoiceID: inv.ID, Kind: req.kind, To: nonNil(req.to), Cc: nonNil(req.cc),
		Subject: subject, Body: body, Attachments: []string{}, ReminderStep: req.reminderStep,
		Automatic: req.automatic, CreatedAt: now,
	}
	msg := mail.Message{To: req.to, Cc: req.cc, Subject: subject, Text: body}
	if r := defaultStr(acc.EmailReplyTo, acc.Email); r != "" {
		if _, err := netmail.ParseAddress(r); err == nil {
			msg.ReplyTo = r
		}
	}
	var sendErr error
	if len(req.to)+len(req.cc) == 0 {
		sendErr = errNoRecipients
	}
	if sendErr == nil && req.attachPDF {
		b, err := s.renderInvoicePDF(ctx, inv, pdf.Options{})
		if err != nil {
			return nil, huma.Error500InternalServerError("pdf rendering failed", err)
		}
		name := pdfFilename(inv.Number)
		msg.Attachments = append(msg.Attachments, mail.Attachment{Filename: name, ContentType: "application/pdf", Data: b})
		log.Attachments = append(log.Attachments, name)
	}
	if sendErr == nil && req.attachISDOC {
		b, err := s.renderISDOC(ctx, inv)
		if err != nil {
			return nil, err
		}
		name := docFilename(inv.Number, ".isdoc")
		msg.Attachments = append(msg.Attachments, mail.Attachment{Filename: name, ContentType: "application/xml", Data: b})
		log.Attachments = append(log.Attachments, name)
	}
	if sendErr == nil {
		sendErr = s.deps.Mailer.Send(ctx, msg)
	}
	if sendErr != nil {
		log.Error = sendErr.Error()
	} else {
		log.SentAt = &now
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(log).Error; err != nil {
			return dbErr(err, "e-mail log")
		}
		if err := recordEmail(ctx, tx, inv, log); err != nil {
			return err
		}
		if sendErr != nil || req.kind != model.EmailInvoice {
			return nil
		}
		// mark_as_sent when still open (re-read inside the transaction)
		var cur model.Invoice
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Scopes(inAccount(ctx)).First(&cur, inv.ID).Error; err != nil {
			return dbErr(err, "invoice")
		}
		if cur.Status != model.StatusOpen {
			return nil
		}
		if err := tx.Model(&cur).Updates(map[string]any{"status": model.StatusSent, "sent_at": now}).Error; err != nil {
			return dbErr(err, "invoice")
		}
		cur.Status, cur.SentAt = model.StatusSent, &now
		return recordInvoice(ctx, tx, events.InvoiceSent, &cur)
	})
	if err != nil {
		return nil, err
	}
	return log, sendErr
}

// sendGeneratedInvoice e-mails a freshly generated recurring invoice to the
// default recipients (best effort: failures end up in EmailLog).
func (s *server) sendGeneratedInvoice(ctx context.Context, invoiceID uint) {
	db := s.db.WithContext(ctx)
	inv, err := loadInvoice(ctx, db, invoiceID)
	if err != nil {
		logJobErr("recurring: load invoice for e-mail", "invoice", invoiceID, "err", err)
		return
	}
	to, cc := s.defaultRecipients(ctx, db, inv)
	if len(to) == 0 {
		cc = nil // the copy address alone is not the client
	}
	if _, err := s.sendInvoiceEmail(ctx, inv, emailRequest{kind: model.EmailInvoice, to: to, cc: cc, attachPDF: true, automatic: true}); err != nil {
		logJobErr("recurring: e-mail not sent", "invoice", invoiceID, "err", err)
	}
}

// sendPaidThanks sends the paid_thanks e-mail after an invoice got fully paid
// when the account has it enabled and the invoice has a client e-mail (best
// effort: failures end up in EmailLog). Call after the payment is committed.
func (s *server) sendPaidThanks(ctx context.Context, invoiceID uint) {
	if !auth.AccountFrom(ctx).PaidThanksEnabled {
		return
	}
	db := s.db.WithContext(ctx)
	inv, err := loadInvoice(ctx, db, invoiceID)
	if err != nil || inv.ClientEmail == "" || inv.Status != model.StatusPaid {
		return
	}
	to, cc := s.defaultRecipients(ctx, db, inv)
	if _, err := s.sendInvoiceEmail(ctx, inv, emailRequest{kind: model.EmailPaidThanks, to: to, cc: cc, automatic: true}); err != nil {
		logJobErr("paid_thanks: e-mail not sent", "invoice", invoiceID, "err", err)
	}
}

// ---- reminders ----

// reminderRetryAfter: a failed automatic reminder is retried after this long.
const reminderRetryAfter = 24 * time.Hour

// RunReminders is the scheduler job sending automatic reminders: for every
// account with reminders_enabled, each open/sent invoice or proforma past
// due with a client e-mail and a remaining amount > 0 gets a reminder for the
// highest step (days after due) it has reached — once per step: a step is
// done when an EmailLog with that reminder_step was sent successfully. Missed
// lower steps are not sent (at most one reminder per run and invoice).
func (s *server) RunReminders(ctx context.Context, now time.Time) error {
	today := billing.Today(now)
	var accs []model.Account
	if err := s.db.WithContext(ctx).Where("reminders_enabled = ?", true).Order("id").Find(&accs).Error; err != nil {
		return err
	}
	var errs []error
	for i := range accs {
		acc := &accs[i]
		actx := withClock(auth.WithAccount(ctx, acc, "system"), func() time.Time { return now }) // events/todos use the job clock
		steps := reminderSteps(acc)
		var invs []model.Invoice
		err := s.scoped(actx).Select("id", "due_on").
			Where("status IN ? AND due_on <> '' AND due_on < ? AND client_email <> '' AND total > paid_amount AND document_type IN ?",
				[]string{model.StatusOpen, model.StatusSent}, today, []string{model.DocInvoice, model.DocProforma}).
			Order("id").Find(&invs).Error
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, inv := range invs {
			due, err := time.Parse(billing.DateLayout, inv.DueOn)
			if err != nil {
				continue
			}
			t, _ := time.Parse(billing.DateLayout, today)
			days := int(t.Sub(due).Hours() / 24)
			step := 0
			for _, st := range steps {
				if st <= days {
					step = st
				}
			}
			if step == 0 {
				continue
			}
			var logs []model.EmailLog
			if err := s.scoped(actx).Select("id", "sent_at", "created_at").
				Where("invoice_id = ? AND kind = ? AND reminder_step = ?", inv.ID, model.EmailReminder, step).
				Find(&logs).Error; err != nil {
				errs = append(errs, err)
				continue
			}
			if slices.ContainsFunc(logs, func(l model.EmailLog) bool {
				return l.SentAt != nil || now.Sub(l.CreatedAt) < reminderRetryAfter
			}) {
				continue
			}
			full, err := loadInvoice(actx, s.db.WithContext(ctx), inv.ID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			to, cc := s.defaultRecipients(actx, s.db.WithContext(ctx), full)
			if _, err := s.sendInvoiceEmail(actx, full, emailRequest{
				kind: model.EmailReminder, to: to, cc: cc, attachPDF: true, reminderStep: step, automatic: true,
			}); err != nil {
				errs = append(errs, fmt.Errorf("reminder for invoice %d: %w", inv.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}
