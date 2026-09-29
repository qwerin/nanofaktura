package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

// ---- recording (used at every domain mutation point) ----

type clockKey struct{}

// withClock makes now the clock of record() and the todo sync for ctx.
// api.New installs it for every request and systemContext for jobs.
func withClock(ctx context.Context, now func() time.Time) context.Context {
	return context.WithValue(ctx, clockKey{}, now)
}

// nowFrom is the clock of ctx (time.Now when none was installed).
func nowFrom(ctx context.Context) time.Time {
	if f, ok := ctx.Value(clockKey{}).(func() time.Time); ok {
		return f()
	}
	return time.Now()
}

// record writes event e of the current account (user from ctx, nil in jobs
// and public endpoints) inside tx — the transaction of the change — and
// refreshes the automatic todos of e's subject. Call it at each mutation
// point, usually through the typed helpers below:
//
//	if err := recordInvoice(ctx, tx, events.InvoiceSent, m); err != nil {
//		return err
//	}
func record(ctx context.Context, tx *gorm.DB, e events.Event) error {
	acc := auth.AccountFrom(ctx)
	meta := events.Meta{AccountID: acc.ID, AccountSlug: acc.Slug, At: nowFrom(ctx)}
	if u := auth.UserFrom(ctx); u != nil {
		id := u.ID
		meta.UserID = &id
	}
	if _, err := events.Record(tx, meta, e); err != nil {
		return huma.Error500InternalServerError("cannot record event", err)
	}
	return syncTodos(ctx, tx, e.SubjectType, e.SubjectID)
}

// docNoun is the Czech name of a document type and whether it is feminine.
func docNoun(docType string) (string, bool) {
	switch docType {
	case model.DocProforma:
		return "Zálohová faktura", true
	case model.DocCorrection:
		return "Opravný daňový doklad", false
	default:
		return "Faktura", true
	}
}

// invoiceTexts are the event texts: [feminine, masculine] with {doc} {n}.
var invoiceTexts = map[string][2]string{
	events.InvoiceCreated:              {"{doc} {n} byla vystavena", "{doc} {n} byl vystaven"},
	events.InvoiceUpdated:              {"{doc} {n} byla upravena", "{doc} {n} byl upraven"},
	events.InvoiceDeleted:              {"{doc} {n} byla smazána", "{doc} {n} byl smazán"},
	events.InvoiceSent:                 {"{doc} {n} byla označena jako odeslaná", "{doc} {n} byl označen jako odeslaný"},
	events.InvoicePaid:                 {"{doc} {n} byla plně uhrazena", "{doc} {n} byl plně uhrazen"},
	events.InvoiceOverdue:              {"{doc} {n} je po splatnosti", "{doc} {n} je po splatnosti"},
	events.InvoiceCancelled:            {"{doc} {n} byla stornována", "{doc} {n} byl stornován"},
	events.InvoiceCancelUndone:         {"{doc} {n}: storno bylo zrušeno", "{doc} {n}: storno bylo zrušeno"},
	events.InvoiceUncollectible:        {"{doc} {n} byla označena jako nedobytná", "{doc} {n} byl označen jako nedobytný"},
	events.InvoiceUncollectibleUndone:  {"{doc} {n}: nedobytnost byla zrušena", "{doc} {n}: nedobytnost byla zrušena"},
	events.InvoiceLocked:               {"{doc} {n} byla zamčena", "{doc} {n} byl zamčen"},
	events.InvoiceUnlocked:             {"{doc} {n} byla odemčena", "{doc} {n} byl odemčen"},
	events.InvoicePublicLinkRegenerate: {"{doc} {n}: veřejný odkaz byl přegenerován", "{doc} {n}: veřejný odkaz byl přegenerován"},
	events.PublicViewed:                {"{doc} {n}: klient poprvé otevřel veřejný odkaz", "{doc} {n}: klient poprvé otevřel veřejný odkaz"},
}

func invoiceText(name string, m *model.Invoice) string {
	noun, fem := docNoun(m.DocumentType)
	t, ok := invoiceTexts[name]
	if !ok {
		return noun + " " + m.Number + ": " + name
	}
	s := t[1]
	if fem {
		s = t[0]
	}
	return strings.NewReplacer("{doc}", noun, "{n}", m.Number).Replace(s)
}

func money(amount int64, currency string) string { return pdf.FormatMoney(amount, currency, "cs") }

func invoiceData(m *model.Invoice) map[string]any {
	return map[string]any{
		"invoice_id": m.ID, "number": m.Number, "document_type": m.DocumentType, "status": m.Status,
		"subject_id": m.SubjectID, "client_name": m.ClientName, "total": m.Total, "paid_amount": m.PaidAmount,
		"currency": m.Currency, "issued_on": m.IssuedOn, "due_on": m.DueOn, "variable_symbol": m.VariableSymbol,
	}
}

// recordInvoice records an invoice.* (or public.viewed) event about m.
func recordInvoice(ctx context.Context, tx *gorm.DB, name string, m *model.Invoice) error {
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectInvoice, SubjectID: m.ID,
		Text: invoiceText(name, m), Data: invoiceData(m)})
}

// invoiceActionEvents maps POST /invoices/{id}/actions/{action} to events.
var invoiceActionEvents = map[string]string{
	"mark_as_sent": events.InvoiceSent, "cancel": events.InvoiceCancelled, "undo_cancel": events.InvoiceCancelUndone,
	"mark_as_uncollectible": events.InvoiceUncollectible, "undo_uncollectible": events.InvoiceUncollectibleUndone,
	"lock": events.InvoiceLocked, "unlock": events.InvoiceUnlocked,
}

// recordInvoicePayment records payment.created/deleted of m and, when the
// payment made m fully paid (status was not paid before), invoice.paid.
func recordInvoicePayment(ctx context.Context, tx *gorm.DB, name string, m *model.Invoice, p *model.Payment, prevStatus string) error {
	noun, _ := docNoun(m.DocumentType)
	verb := "přidána"
	if name == events.PaymentDeleted {
		verb = "smazána"
	}
	data := invoiceData(m)
	data["payment_id"], data["amount"], data["paid_on"] = p.ID, p.Amount, p.PaidOn
	err := record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectInvoice, SubjectID: m.ID,
		Text: fmt.Sprintf("%s %s: %s platba %s", noun, m.Number, verb, money(p.Amount, m.Currency)), Data: data})
	if err != nil || prevStatus == model.StatusPaid || m.Status != model.StatusPaid {
		return err
	}
	return recordInvoice(ctx, tx, events.InvoicePaid, m)
}

// recordEmail records email.sent / email.failed about the invoice of l.
func recordEmail(ctx context.Context, tx *gorm.DB, inv *model.Invoice, l *model.EmailLog) error {
	noun, _ := docNoun(inv.DocumentType)
	kind := map[string]string{model.EmailInvoice: "doklad", model.EmailReminder: "upomínka", model.EmailPaidThanks: "poděkování za úhradu"}[l.Kind]
	name, text := events.EmailSent, fmt.Sprintf("%s %s: odeslán e-mail (%s) na %s", noun, inv.Number, kind, strings.Join(append(append([]string{}, l.To...), l.Cc...), ", "))
	if l.SentAt == nil {
		name, text = events.EmailFailed, fmt.Sprintf("%s %s: e-mail (%s) se nepodařilo odeslat: %s", noun, inv.Number, kind, l.Error)
	}
	data := invoiceData(inv)
	data["email_log_id"], data["kind"], data["to"], data["cc"], data["automatic"] = l.ID, l.Kind, l.To, l.Cc, l.Automatic
	if l.ReminderStep > 0 {
		data["reminder_step"] = l.ReminderStep
	}
	if l.Error != "" {
		data["error"] = l.Error
	}
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectInvoice, SubjectID: inv.ID, Text: text, Data: data})
}

var expenseTexts = map[string]string{
	events.ExpenseCreated: "Náklad %s (%s) byl zapsán", events.ExpenseUpdated: "Náklad %s (%s) byl upraven",
	events.ExpenseDeleted: "Náklad %s (%s) byl smazán", events.ExpenseLocked: "Náklad %s (%s) byl zamčen",
	events.ExpenseUnlocked: "Náklad %s (%s) byl odemčen", events.ExpensePaid: "Náklad %s (%s) byl plně uhrazen",
}

func expenseData(m *model.Expense) map[string]any {
	return map[string]any{
		"expense_id": m.ID, "number": m.Number, "original_number": m.OriginalNumber, "status": m.Status,
		"subject_id": m.SubjectID, "supplier_name": m.SupplierName, "total": m.Total, "paid_amount": m.PaidAmount,
		"currency": m.Currency, "issued_on": m.IssuedOn, "due_on": m.DueOn,
	}
}

// recordExpense records an expense.* event about m.
func recordExpense(ctx context.Context, tx *gorm.DB, name string, m *model.Expense) error {
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectExpense, SubjectID: m.ID,
		Text: fmt.Sprintf(expenseTexts[name], m.Number, m.SupplierName), Data: expenseData(m)})
}

// recordExpensePayment records expense_payment.created/deleted and, when
// the payment made m fully paid, expense.paid.
func recordExpensePayment(ctx context.Context, tx *gorm.DB, name string, m *model.Expense, p *model.ExpensePayment, prevStatus string) error {
	verb := "přidána"
	if name == events.ExpensePaymentDeleted {
		verb = "smazána"
	}
	data := expenseData(m)
	data["payment_id"], data["amount"], data["paid_on"] = p.ID, p.Amount, p.PaidOn
	err := record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectExpense, SubjectID: m.ID,
		Text: fmt.Sprintf("Náklad %s: %s platba %s", m.Number, verb, money(p.Amount, m.Currency)), Data: data})
	if err != nil || prevStatus == model.StatusPaid || m.Status != model.StatusPaid {
		return err
	}
	return recordExpense(ctx, tx, events.ExpensePaid, m)
}

// recordSubject records subject.created/updated/deleted.
func recordSubject(ctx context.Context, tx *gorm.DB, name string, m *model.Subject) error {
	verb := map[string]string{events.SubjectCreated: "vytvořen", events.SubjectUpdated: "upraven", events.SubjectDeleted: "smazán"}[name]
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectSubject, SubjectID: m.ID,
		Text: fmt.Sprintf("Kontakt %s byl %s", m.Name, verb),
		Data: map[string]any{"subject_id": m.ID, "name": m.Name, "type": m.Type, "registration_no": m.RegistrationNo,
			"vat_no": m.VatNo, "email": m.Email}})
}

func priceItemData(m *model.PriceItem) map[string]any {
	d := map[string]any{"price_item_id": m.ID, "name": m.Name, "sku": m.SKU, "track_stock": m.TrackStock,
		"stock_quantity": billing.FormatQuantity(m.StockQuantityMilli)}
	if m.MinStockMilli != nil {
		d["min_stock"] = billing.FormatQuantity(*m.MinStockMilli)
	}
	return d
}

// recordPriceItem records price_item.created/updated/deleted.
func recordPriceItem(ctx context.Context, tx *gorm.DB, name string, m *model.PriceItem) error {
	verb := map[string]string{events.PriceItemCreated: "vytvořena", events.PriceItemUpdated: "upravena", events.PriceItemDeleted: "smazána"}[name]
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectPriceItem, SubjectID: m.ID,
		Text: fmt.Sprintf("Položka ceníku %s byla %s", m.Name, verb), Data: priceItemData(m)})
}

// recordStockMove records a manual stock move (created or deleted) of item.
func recordStockMove(ctx context.Context, tx *gorm.DB, item *model.PriceItem, mv *model.StockMove, deleted bool) error {
	what := map[string]string{model.StockIn: "příjem", model.StockOut: "výdej"}[mv.Direction]
	verb := "zapsán"
	if deleted {
		verb = "smazán"
	}
	data := priceItemData(item)
	data["stock_move_id"], data["direction"], data["quantity"], data["moved_on"], data["deleted"] =
		mv.ID, mv.Direction, billing.FormatQuantity(mv.QuantityMilli), mv.MovedOn, deleted
	return record(ctx, tx, events.Event{Name: events.StockMoved, SubjectType: events.SubjectPriceItem, SubjectID: item.ID,
		Text: fmt.Sprintf("Sklad %s: %s %s %s %s", item.Name, verb, what, billing.FormatQuantity(mv.QuantityMilli), item.UnitName), Data: data})
}

func bankTxData(m *model.BankTransaction) map[string]any {
	return map[string]any{"bank_transaction_id": m.ID, "bank_account_id": m.BankAccountID, "booked_on": m.BookedOn,
		"amount": m.Amount, "currency": m.Currency, "variable_symbol": m.VariableSymbol,
		"counterparty_name": m.CounterpartyName, "counterparty_account": m.CounterpartyAccount,
		"invoice_id": m.MatchedInvoiceID, "expense_id": m.MatchedExpenseID, "payment_id": m.PaymentID, "auto_matched": m.AutoMatched}
}

// recordBankMatch records bank.matched / bank.unmatched of m with the
// number of the document (docLabel, e.g. "faktura 2026-0001").
func recordBankMatch(ctx context.Context, tx *gorm.DB, name string, m *model.BankTransaction, docLabel string) error {
	text := fmt.Sprintf("Platba %s ze dne %s byla spárována: %s", money(m.Amount, m.Currency), czDate(m.BookedOn), docLabel)
	switch {
	case name == events.BankUnmatched:
		text = fmt.Sprintf("Párování platby %s ze dne %s bylo zrušeno (%s)", money(m.Amount, m.Currency), czDate(m.BookedOn), docLabel)
	case m.AutoMatched:
		text = fmt.Sprintf("Platba %s ze dne %s byla automaticky spárována: %s", money(m.Amount, m.Currency), czDate(m.BookedOn), docLabel)
	}
	return record(ctx, tx, events.Event{Name: name, SubjectType: events.SubjectBankTransaction, SubjectID: m.ID, Text: text, Data: bankTxData(m)})
}

// recordBankImport records bank.imported for a statement import / sync.
func recordBankImport(ctx context.Context, tx *gorm.DB, ba *model.BankAccount, res BankImportResult) error {
	return record(ctx, tx, events.Event{Name: events.BankImported, SubjectType: events.SubjectBankAccount, SubjectID: ba.ID,
		Text: fmt.Sprintf("Účet %s: naimportováno %d pohybů (spárováno %d, s návrhem %d, duplicit %d)",
			ba.Name, res.Imported, res.Matched, res.Suggestions, res.Duplicates),
		Data: map[string]any{"bank_account_id": ba.ID, "imported": res.Imported, "duplicates": res.Duplicates,
			"matched": res.Matched, "suggestions": res.Suggestions}})
}

// recordRecurring records recurring.generated (inv set) or recurring.failed (errMsg).
func recordRecurring(ctx context.Context, tx *gorm.DB, r *model.Recurring, inv *model.Invoice, errMsg string) error {
	data := map[string]any{"recurring_id": r.ID, "name": r.Name, "next_occurrence_on": r.NextOccurrenceOn}
	if inv == nil {
		data["error"] = errMsg
		return record(ctx, tx, events.Event{Name: events.RecurringFailed, SubjectType: events.SubjectRecurring, SubjectID: r.ID,
			Text: fmt.Sprintf("Pravidelnou fakturu %s se nepodařilo vystavit: %s", r.Name, errMsg), Data: data})
	}
	data["invoice_id"], data["number"], data["total"], data["currency"] = inv.ID, inv.Number, inv.Total, inv.Currency
	return record(ctx, tx, events.Event{Name: events.RecurringGenerated, SubjectType: events.SubjectRecurring, SubjectID: r.ID,
		Text: fmt.Sprintf("Z pravidelné faktury %s byl vystaven doklad %s", r.Name, inv.Number), Data: data})
}

// czDate formats "YYYY-MM-DD" as "15. 3. 2026".
func czDate(iso string) string { return pdf.FormatDate(iso, "cs") }

// ---- API ----

// Event is one activity log entry.
type Event struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name" doc:"e.g. invoice.paid (see GET /events/catalog)"`
	SubjectType string         `json:"subject_type" doc:"invoice, expense, subject, price_item, bank_transaction, bank_account, recurring, webhook"`
	SubjectID   uint           `json:"subject_id"`
	Text        string         `json:"text" doc:"Czech description"`
	Data        map[string]any `json:"data"`
	UserID      *uint          `json:"user_id,omitempty" doc:"Author; missing for the scheduler and public link"`
	UserName    string         `json:"user_name,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// EventCatalogEntry is a known event name.
type EventCatalogEntry = events.CatalogEntry

func toEvent(m *model.Event) Event {
	data := m.Data
	if data == nil {
		data = map[string]any{}
	}
	return Event{ID: m.ID, Name: m.Name, SubjectType: m.SubjectType, SubjectID: m.SubjectID, Text: m.Text,
		Data: data, UserID: m.UserID, CreatedAt: m.CreatedAt}
}

func (s *server) registerEvents(g huma.API) {
	huma.Get(g, "/events", s.listEvents)
	huma.Get(g, "/events/catalog", s.eventCatalog)
}

func (s *server) listEvents(ctx context.Context, in *struct {
	PageParams
	Name        string `query:"name" doc:"Exact name (invoice.paid) or prefix (invoice, invoice.*)"`
	SubjectType string `query:"subject_type"`
	SubjectID   uint   `query:"subject_id" doc:"With subject_type: the timeline of one record"`
	Since       string `query:"since" doc:"RFC 3339 instant or YYYY-MM-DD (inclusive)"`
}) (*Out[ListResponse[Event]], error) {
	q := s.scoped(ctx).Model(&model.Event{}).Order("created_at DESC, id DESC")
	if n := strings.TrimSuffix(strings.TrimSpace(in.Name), "*"); n != "" {
		if strings.Contains(n, ".") && !strings.HasSuffix(n, ".") {
			q = q.Where("name = ?", n)
		} else {
			q = q.Where("name LIKE ?", strings.TrimSuffix(n, ".")+".%")
		}
	}
	if in.SubjectType != "" {
		q = q.Where("subject_type = ?", in.SubjectType)
	}
	if in.SubjectID != 0 {
		if in.SubjectType == "" {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
				&huma.ErrorDetail{Location: "query.subject_type", Message: "subject_id requires subject_type"})
		}
		q = q.Where("subject_id = ?", in.SubjectID)
	}
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			if t, err = time.Parse("2006-01-02", in.Since); err != nil {
				return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
					&huma.ErrorDetail{Location: "query.since", Message: "use RFC 3339 or YYYY-MM-DD", Value: in.Since})
			}
		}
		q = q.Where("created_at >= ?", t)
	}
	out, err := paginate(q, in.PageParams, toEvent)
	if err != nil {
		return nil, err
	}
	s.fillEventUsers(ctx, out.Body.Items)
	return out, nil
}

// fillEventUsers adds user names (one query per page).
func (s *server) fillEventUsers(ctx context.Context, items []Event) {
	ids := []uint{}
	for _, e := range items {
		if e.UserID != nil {
			ids = append(ids, *e.UserID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var users []model.User
	if err := s.db.WithContext(ctx).Select("id", "name", "email").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return
	}
	names := map[uint]string{}
	for _, u := range users {
		names[u.ID] = defaultStr(u.Name, u.Email)
	}
	for i := range items {
		if items[i].UserID != nil {
			items[i].UserName = names[*items[i].UserID]
		}
	}
}

func (s *server) eventCatalog(ctx context.Context, _ *struct{}) (*Out[[]EventCatalogEntry], error) {
	return &Out[[]EventCatalogEntry]{Body: events.Catalog}, nil
}
