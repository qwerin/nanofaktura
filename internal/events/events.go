// Package events writes the activity log (SPEC §7.9) and queues webhook
// deliveries (SPEC §7.10).
//
// Record must be called with the transaction of the change it describes:
// the Event row and the WebhookDelivery rows of matching webhooks are
// inserted in that transaction, so a rollback removes them too and a
// webhook never announces a change that did not happen. The deliveries are
// POSTed later by the "webhooks" scheduler job (internal/api/webhooks.go).
//
// Handlers do not call Record directly but the api helper
// record(ctx, tx, events.Event{…}), which fills Meta from the request
// context (account, user, clock) and refreshes automatic todos.
package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/model"
)

// Event is what happened; Name is one of the constants below (Catalog).
type Event struct {
	Name        string
	SubjectType string // invoice, expense, subject, price_item, bank_transaction, bank_account, recurring, webhook, account
	SubjectID   uint
	Text        string         // Czech sentence for people
	Data        map[string]any // key fields (JSON), also sent to webhooks
}

// Meta is who/where/when (filled by the caller from its context).
type Meta struct {
	AccountID   uint
	AccountSlug string // "account" of the webhook payload
	UserID      *uint  // nil = scheduler, public link, system
	At          time.Time
}

// Subject types.
const (
	SubjectInvoice         = "invoice"
	SubjectExpense         = "expense"
	SubjectSubject         = "subject"
	SubjectPriceItem       = "price_item"
	SubjectBankTransaction = "bank_transaction"
	SubjectBankAccount     = "bank_account"
	SubjectRecurring       = "recurring"
	SubjectWebhook         = "webhook"
	SubjectAccount         = "account"
)

// Event names.
const (
	InvoiceCreated              = "invoice.created"
	InvoiceUpdated              = "invoice.updated"
	InvoiceDeleted              = "invoice.deleted"
	InvoiceSent                 = "invoice.sent"
	InvoicePaid                 = "invoice.paid"
	InvoiceOverdue              = "invoice.overdue"
	InvoiceCancelled            = "invoice.cancelled"
	InvoiceCancelUndone         = "invoice.cancel_undone"
	InvoiceUncollectible        = "invoice.uncollectible"
	InvoiceUncollectibleUndone  = "invoice.uncollectible_undone"
	InvoiceLocked               = "invoice.locked"
	InvoiceUnlocked             = "invoice.unlocked"
	InvoicePublicLinkRegenerate = "invoice.public_link_regenerated"
	PaymentCreated              = "payment.created"
	PaymentDeleted              = "payment.deleted"
	PublicViewed                = "public.viewed"
	EmailSent                   = "email.sent"
	EmailFailed                 = "email.failed"
	ExpenseCreated              = "expense.created"
	ExpenseUpdated              = "expense.updated"
	ExpenseDeleted              = "expense.deleted"
	ExpenseLocked               = "expense.locked"
	ExpenseUnlocked             = "expense.unlocked"
	ExpensePaid                 = "expense.paid"
	ExpensePaymentCreated       = "expense_payment.created"
	ExpensePaymentDeleted       = "expense_payment.deleted"
	SubjectCreated              = "subject.created"
	SubjectUpdated              = "subject.updated"
	SubjectDeleted              = "subject.deleted"
	PriceItemCreated            = "price_item.created"
	PriceItemUpdated            = "price_item.updated"
	PriceItemDeleted            = "price_item.deleted"
	StockMoved                  = "stock.moved"
	StockLow                    = "stock.low"
	RecurringGenerated          = "recurring.generated"
	RecurringFailed             = "recurring.failed"
	BankImported                = "bank.imported"
	BankMatched                 = "bank.matched"
	BankUnmatched               = "bank.unmatched"
	WebhookFailed               = "webhook.failed"
	WebhookDisabled             = "webhook.disabled"
	AccountExported             = "account.exported"
	AccountImported             = "account.imported"
)

// CatalogEntry describes one event name (for UIs choosing webhook filters).
type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Catalog lists every event name with a Czech description.
var Catalog = []CatalogEntry{
	{InvoiceCreated, "Doklad vystaven"},
	{InvoiceUpdated, "Doklad upraven"},
	{InvoiceDeleted, "Doklad smazán"},
	{InvoiceSent, "Doklad označen jako odeslaný"},
	{InvoicePaid, "Doklad plně uhrazen"},
	{InvoiceOverdue, "Doklad po splatnosti"},
	{InvoiceCancelled, "Doklad stornován"},
	{InvoiceCancelUndone, "Storno dokladu zrušeno"},
	{InvoiceUncollectible, "Doklad označen jako nedobytný"},
	{InvoiceUncollectibleUndone, "Nedobytnost dokladu zrušena"},
	{InvoiceLocked, "Doklad zamčen"},
	{InvoiceUnlocked, "Doklad odemčen"},
	{InvoicePublicLinkRegenerate, "Veřejný odkaz dokladu přegenerován"},
	{PaymentCreated, "Platba k dokladu přidána"},
	{PaymentDeleted, "Platba k dokladu smazána"},
	{PublicViewed, "Klient poprvé otevřel veřejný odkaz"},
	{EmailSent, "E-mail odeslán"},
	{EmailFailed, "E-mail se nepodařilo odeslat"},
	{ExpenseCreated, "Náklad zapsán"},
	{ExpenseUpdated, "Náklad upraven"},
	{ExpenseDeleted, "Náklad smazán"},
	{ExpenseLocked, "Náklad zamčen"},
	{ExpenseUnlocked, "Náklad odemčen"},
	{ExpensePaid, "Náklad plně uhrazen"},
	{ExpensePaymentCreated, "Platba nákladu přidána"},
	{ExpensePaymentDeleted, "Platba nákladu smazána"},
	{SubjectCreated, "Kontakt vytvořen"},
	{SubjectUpdated, "Kontakt upraven"},
	{SubjectDeleted, "Kontakt smazán"},
	{PriceItemCreated, "Položka ceníku vytvořena"},
	{PriceItemUpdated, "Položka ceníku upravena"},
	{PriceItemDeleted, "Položka ceníku smazána"},
	{StockMoved, "Ruční pohyb skladu"},
	{StockLow, "Stav skladu klesl pod minimum"},
	{RecurringGenerated, "Pravidelná faktura vystavena"},
	{RecurringFailed, "Pravidelnou fakturu se nepodařilo vystavit"},
	{BankImported, "Bankovní pohyby naimportovány"},
	{BankMatched, "Bankovní pohyb spárován"},
	{BankUnmatched, "Párování bankovního pohybu zrušeno"},
	{WebhookFailed, "Webhook se nepodařilo doručit"},
	{WebhookDisabled, "Webhook automaticky vypnut"},
	{AccountExported, "Záloha účtu stažena"},
	{AccountImported, "Účet obnoven ze zálohy"},
}

// Record stores e and queues a delivery for every active webhook of the
// account whose filter matches e.Name — inside tx (see the package doc).
// A webhook never receives the webhook.* events about itself (no loops).
func Record(tx *gorm.DB, meta Meta, e Event) (*model.Event, error) {
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	ev := &model.Event{
		AccountID: meta.AccountID, UserID: meta.UserID, Name: e.Name,
		SubjectType: e.SubjectType, SubjectID: e.SubjectID, Text: e.Text, Data: e.Data, CreatedAt: meta.At,
	}
	if err := tx.Create(ev).Error; err != nil {
		return nil, fmt.Errorf("record event %s: %w", e.Name, err)
	}
	var hooks []model.Webhook
	if err := tx.Where("account_id = ? AND active = ?", meta.AccountID, true).Find(&hooks).Error; err != nil {
		return nil, fmt.Errorf("record event %s: webhooks: %w", e.Name, err)
	}
	var payload []byte
	for _, h := range hooks {
		if !Matches(h.Events, e.Name) || (e.SubjectType == SubjectWebhook && e.SubjectID == h.ID) {
			continue
		}
		if payload == nil {
			var err error
			if payload, err = Payload(ev, meta.AccountSlug); err != nil {
				return nil, err
			}
		}
		at := meta.At
		d := &model.WebhookDelivery{
			AccountID: meta.AccountID, WebhookID: h.ID, EventID: &ev.ID, EventName: e.Name,
			Payload: string(payload), Status: model.DeliveryPending, NextAttemptAt: &at, CreatedAt: meta.At,
		}
		if err := tx.Create(d).Error; err != nil {
			return nil, fmt.Errorf("record event %s: queue delivery: %w", e.Name, err)
		}
	}
	return ev, nil
}

// Matches reports whether name passes a webhook filter: "*" = everything,
// "invoice.*" = every name starting with "invoice.", otherwise exact match.
func Matches(patterns []string, name string) bool {
	for _, p := range patterns {
		switch {
		case p == "*" || p == name:
			return true
		case strings.HasSuffix(p, ".*") && strings.HasPrefix(name, strings.TrimSuffix(p, "*")):
			return true
		}
	}
	return false
}

// ValidPattern reports whether p is "*", a known event name or "<prefix>.*"
// with a known prefix.
func ValidPattern(p string) bool {
	if p == "*" {
		return true
	}
	for _, c := range Catalog {
		if c.Name == p || (strings.HasSuffix(p, ".*") && strings.HasPrefix(c.Name, strings.TrimSuffix(p, "*"))) {
			return true
		}
	}
	return false
}

// PayloadBody is the JSON body POSTed to webhooks.
type PayloadBody struct {
	ID        uint           `json:"id"` // event id
	Event     string         `json:"event"`
	CreatedAt time.Time      `json:"created_at"`
	Account   string         `json:"account"` // account slug
	Subject   PayloadSubject `json:"subject"`
	Text      string         `json:"text"`
	Data      map[string]any `json:"data"`
}

// PayloadSubject is the record the event is about.
type PayloadSubject struct {
	Type string `json:"type"`
	ID   uint   `json:"id"`
}

// Payload is the webhook body of ev.
func Payload(ev *model.Event, accountSlug string) ([]byte, error) {
	data := ev.Data
	if data == nil {
		data = map[string]any{}
	}
	return json.Marshal(PayloadBody{
		ID: ev.ID, Event: ev.Name, CreatedAt: ev.CreatedAt.UTC(), Account: accountSlug,
		Subject: PayloadSubject{Type: ev.SubjectType, ID: ev.SubjectID}, Text: ev.Text, Data: data,
	})
}
