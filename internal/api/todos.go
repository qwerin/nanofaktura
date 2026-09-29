package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
)

// ---- automatic todos ----
//
// An automatic todo exists for a condition of one record (Todo.Key =
// "<name>:<id>"): overdue invoice, unconfirmed bank match suggestion, low
// stock, failed recurring invoice. syncTodos re-evaluates the conditions of
// one record; record() calls it for the subject of every event, and the
// "todos" job for all candidates (conditions that start without an event,
// e.g. an invoice becoming overdue with time). A resolved condition
// completes the todo (auto_completed); a condition that returns reopens an
// auto-completed todo, never one completed by a person.

// todoWant is the desired state of an automatic todo (nil = resolved).
type todoWant struct {
	text, dueOn string
}

// syncTodos re-evaluates the automatic todos of one record in tx.
func syncTodos(ctx context.Context, tx *gorm.DB, subjectType string, id uint) error {
	switch subjectType {
	case events.SubjectInvoice:
		return syncInvoiceTodo(ctx, tx, id)
	case events.SubjectBankTransaction:
		return syncBankTxTodo(ctx, tx, id)
	case events.SubjectBankAccount:
		var ids []uint
		if err := tx.Model(&model.BankTransaction{}).Scopes(inAccount(ctx)).
			Where("bank_account_id = ? AND payment_id IS NULL AND ignored = ? AND suggestion_count > 0", id, false).
			Pluck("id", &ids).Error; err != nil {
			return dbErr(err, "bank transaction")
		}
		for _, t := range ids {
			if err := syncBankTxTodo(ctx, tx, t); err != nil {
				return err
			}
		}
	case events.SubjectPriceItem:
		_, err := syncStockTodo(ctx, tx, id)
		return err
	case events.SubjectRecurring:
		return syncRecurringTodo(ctx, tx, id)
	}
	return nil
}

func todoKey(name string, id uint) string { return fmt.Sprintf("%s:%d", name, id) }

// upsertTodo brings the automatic todo name:id to want (see the section
// comment); it reports whether the todo became open (created or reopened).
// A missing related record (deleted) removes its open todo.
func upsertTodo(ctx context.Context, tx *gorm.DB, name, relType string, relID uint, want *todoWant, gone bool) (bool, error) {
	key := todoKey(name, relID)
	var t model.Todo
	if err := tx.Scopes(inAccount(ctx)).Where("key = ?", key).Limit(1).Find(&t).Error; err != nil {
		return false, dbErr(err, "todo")
	}
	found := t.ID != 0
	now := nowFrom(ctx)
	switch {
	case gone:
		if found {
			return false, dbErrOrNil(tx.Delete(&t).Error, "todo")
		}
		return false, nil
	case want == nil:
		if found && t.CompletedAt == nil {
			return false, dbErrOrNil(tx.Model(&t).Updates(map[string]any{"completed_at": now, "auto_completed": true, "updated_at": now}).Error, "todo")
		}
		return false, nil
	case !found:
		id := relID
		t = model.Todo{AccountID: auth.AccountFrom(ctx).ID, Key: &key, Name: name, Text: want.text,
			RelatedType: relType, RelatedID: &id, DueOn: want.dueOn, CreatedAt: now, UpdatedAt: now}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&t)
		return r.RowsAffected > 0, dbErrOrNil(r.Error, "todo")
	}
	upd := map[string]any{}
	if t.Text != want.text {
		upd["text"] = want.text
	}
	if t.DueOn != want.dueOn {
		upd["due_on"] = want.dueOn
	}
	reopened := t.CompletedAt != nil && t.AutoCompleted
	if reopened {
		upd["completed_at"], upd["auto_completed"] = nil, false
	}
	if len(upd) == 0 {
		return false, nil
	}
	upd["updated_at"] = now
	return reopened, dbErrOrNil(tx.Model(&t).Updates(upd).Error, "todo")
}

// invoiceOverdue reports whether m is an unpaid invoice/proforma past due.
func invoiceOverdue(m *model.Invoice, today string) bool {
	return (m.Status == model.StatusOpen || m.Status == model.StatusSent) &&
		(m.DocumentType == model.DocInvoice || m.DocumentType == model.DocProforma) &&
		m.DueOn != "" && m.DueOn < today && m.Total > m.PaidAmount
}

func syncInvoiceTodo(ctx context.Context, tx *gorm.DB, id uint) error {
	var m model.Invoice
	if err := tx.Scopes(inAccount(ctx)).Limit(1).Find(&m, id).Error; err != nil {
		return dbErr(err, "invoice")
	}
	var want *todoWant
	if m.ID != 0 && invoiceOverdue(&m, billing.Today(nowFrom(ctx))) {
		noun, _ := docNoun(m.DocumentType)
		want = &todoWant{dueOn: m.DueOn, text: fmt.Sprintf("%s %s (%s) je po splatnosti od %s, zbývá uhradit %s",
			noun, m.Number, m.ClientName, czDate(m.DueOn), money(m.Total-m.PaidAmount, m.Currency))}
	}
	_, err := upsertTodo(ctx, tx, model.TodoInvoiceOverdue, events.SubjectInvoice, id, want, m.ID == 0)
	return err
}

func syncBankTxTodo(ctx context.Context, tx *gorm.DB, id uint) error {
	var m model.BankTransaction
	if err := tx.Scopes(inAccount(ctx)).Limit(1).Find(&m, id).Error; err != nil {
		return dbErr(err, "bank transaction")
	}
	var want *todoWant
	if m.ID != 0 && m.PaymentID == nil && !m.Ignored && m.SuggestionCount > 0 {
		from := m.CounterpartyName
		if from == "" {
			from = defaultStr(m.CounterpartyAccount, "neznámého protiúčtu")
		}
		dir := "od"
		if m.Amount < 0 {
			dir = "pro"
		}
		text := fmt.Sprintf("Potvrďte párování platby %s ze dne %s %s %s", money(m.Amount, m.Currency), czDate(m.BookedOn), dir, from)
		if len(m.Suggestions) > 0 {
			text += " (návrh: " + m.Suggestions[0].Number + ")"
		}
		want = &todoWant{text: text}
	}
	_, err := upsertTodo(ctx, tx, model.TodoBankSuggested, events.SubjectBankTransaction, id, want, m.ID == 0)
	return err
}

// syncStockTodo keeps the low stock todo of a price item; a newly low item
// also records stock.low (once per drop below the minimum).
func syncStockTodo(ctx context.Context, tx *gorm.DB, id uint) (bool, error) {
	var m model.PriceItem
	if err := tx.Scopes(inAccount(ctx)).Limit(1).Find(&m, id).Error; err != nil {
		return false, dbErr(err, "price item")
	}
	var want *todoWant
	low := m.ID != 0 && m.TrackStock && m.ArchivedAt == nil && m.MinStockMilli != nil && m.StockQuantityMilli < *m.MinStockMilli
	if low {
		want = &todoWant{text: fmt.Sprintf("Dochází zásoba: %s — skladem %s %s, minimum %s",
			m.Name, billing.FormatQuantity(m.StockQuantityMilli), m.UnitName, billing.FormatQuantity(*m.MinStockMilli))}
	}
	opened, err := upsertTodo(ctx, tx, model.TodoStockLow, events.SubjectPriceItem, id, want, m.ID == 0)
	if err != nil || !opened {
		return opened, err
	}
	return true, record(ctx, tx, events.Event{Name: events.StockLow, SubjectType: events.SubjectPriceItem, SubjectID: m.ID,
		Text: want.text, Data: priceItemData(&m)})
}

// syncStockTodos is syncStockTodo for the items touched by a stock change.
func syncStockTodos(ctx context.Context, tx *gorm.DB, ids []uint) error {
	seen := map[uint]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := syncStockTodo(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func syncRecurringTodo(ctx context.Context, tx *gorm.DB, id uint) error {
	var m model.Recurring
	if err := tx.Scopes(inAccount(ctx)).Limit(1).Find(&m, id).Error; err != nil {
		return dbErr(err, "recurring")
	}
	var want *todoWant
	if m.ID != 0 && m.Active && m.LastError != "" {
		want = &todoWant{text: fmt.Sprintf("Pravidelnou fakturu %s se nepodařilo vystavit: %s", m.Name, m.LastError)}
	}
	_, err := upsertTodo(ctx, tx, model.TodoRecurringFailed, events.SubjectRecurring, id, want, m.ID == 0)
	return err
}

// RunTodos is the scheduler job: records invoice.overdue once per invoice
// that became overdue and re-evaluates every automatic todo condition
// (candidates + open automatic todos), one record per transaction.
func (s *server) RunTodos(ctx context.Context, now time.Time) error {
	ctx = withClock(ctx, func() time.Time { return now })
	today := billing.Today(now)
	type unit struct {
		AccountID   uint
		SubjectType string
		ID          uint
	}
	var units []unit
	seen := map[unit]bool{}
	add := func(typ string, rows []model.Todo) {
		for _, r := range rows {
			u := unit{r.AccountID, typ, *r.RelatedID}
			if !seen[u] {
				seen[u] = true
				units = append(units, u)
			}
		}
	}
	db := s.db.WithContext(ctx)
	collect := func(typ string, q *gorm.DB) error {
		var rows []struct{ ID, AccountID uint }
		if err := q.Select("id", "account_id").Order("id").Find(&rows).Error; err != nil {
			return err
		}
		todos := make([]model.Todo, len(rows))
		for i, r := range rows {
			id := r.ID
			todos[i] = model.Todo{AccountID: r.AccountID, RelatedID: &id}
		}
		add(typ, todos)
		return nil
	}
	overdueQ := db.Model(&model.Invoice{}).Where("status IN ? AND document_type IN ? AND due_on <> '' AND due_on < ? AND total > paid_amount",
		[]string{model.StatusOpen, model.StatusSent}, []string{model.DocInvoice, model.DocProforma}, today)
	if err := collect(events.SubjectInvoice, overdueQ); err != nil {
		return err
	}
	overdue := len(units)
	if err := collect(events.SubjectBankTransaction, db.Model(&model.BankTransaction{}).
		Where("payment_id IS NULL AND ignored = ? AND suggestion_count > 0", false)); err != nil {
		return err
	}
	if err := collect(events.SubjectPriceItem, db.Model(&model.PriceItem{}).
		Where("track_stock = ? AND archived_at IS NULL AND min_stock_milli IS NOT NULL AND stock_quantity_milli < min_stock_milli", true)); err != nil {
		return err
	}
	if err := collect(events.SubjectRecurring, db.Model(&model.Recurring{}).Where("active = ? AND last_error <> ''", true)); err != nil {
		return err
	}
	var open []model.Todo
	if err := db.Where("key IS NOT NULL AND completed_at IS NULL AND related_id IS NOT NULL").Find(&open).Error; err != nil {
		return err
	}
	for _, t := range open {
		add(t.RelatedType, []model.Todo{t})
	}

	var errs []error
	ctxs := map[uint]context.Context{}
	for i, u := range units {
		actx, ok := ctxs[u.AccountID]
		if !ok {
			var err error
			if actx, err = s.systemContext(ctx, u.AccountID); err != nil {
				errs = append(errs, err)
				continue
			}
			ctxs[u.AccountID] = actx
		}
		isOverdue := i < overdue
		err := s.db.WithContext(actx).Transaction(func(tx *gorm.DB) error {
			if !isOverdue {
				return syncTodos(actx, tx, u.SubjectType, u.ID)
			}
			var m model.Invoice
			if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Scopes(inAccount(actx)).First(&m, u.ID).Error; err != nil {
				return err
			}
			if !invoiceOverdue(&m, today) {
				return syncTodos(actx, tx, u.SubjectType, u.ID)
			}
			var n int64
			if err := tx.Model(&model.Event{}).Where("account_id = ? AND subject_type = ? AND subject_id = ? AND name = ?",
				u.AccountID, events.SubjectInvoice, m.ID, events.InvoiceOverdue).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				return syncTodos(actx, tx, u.SubjectType, u.ID)
			}
			return recordInvoice(actx, tx, events.InvoiceOverdue, &m)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("todos %s %d: %w", u.SubjectType, u.ID, err))
		}
	}
	return errors.Join(errs...)
}

// ---- API ----

// Todo is a task (manual or automatic).
type Todo struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name" doc:"manual, invoice.overdue, bank.suggested, stock.low, recurring.failed"`
	Text        string     `json:"text"`
	Automatic   bool       `json:"automatic" doc:"Generated by the system; completed automatically when resolved"`
	RelatedType string     `json:"related_type,omitempty"`
	RelatedID   *uint      `json:"related_id,omitempty"`
	DueOn       string     `json:"due_on,omitempty" format:"date"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	UserID      *uint      `json:"user_id,omitempty" doc:"Author of a manual todo"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TodoCreate struct {
	Text        string `json:"text" minLength:"1" maxLength:"1000"`
	DueOn       string `json:"due_on,omitempty" format:"date"`
	RelatedType string `json:"related_type,omitempty" enum:"invoice,expense,subject,price_item,bank_transaction,recurring"`
	RelatedID   *uint  `json:"related_id,omitempty"`
}

type TodoPatch struct {
	Text      *string `json:"text,omitempty" minLength:"1" maxLength:"1000" doc:"Manual todos only"`
	DueOn     *string `json:"due_on,omitempty" doc:"Manual todos only; \"\" clears"`
	Completed *bool   `json:"completed,omitempty"`
}

func toTodo(m *model.Todo) Todo {
	return Todo{ID: m.ID, Name: m.Name, Text: m.Text, Automatic: m.Key != nil, RelatedType: m.RelatedType,
		RelatedID: m.RelatedID, DueOn: m.DueOn, Completed: m.CompletedAt != nil, CompletedAt: m.CompletedAt,
		UserID: m.UserID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func (s *server) registerTodos(g huma.API) {
	huma.Get(g, "/todos", s.listTodos)
	huma.Post(g, "/todos", s.createTodo, status(http.StatusCreated), auth.ForEditors)
	huma.Patch(g, "/todos/{id}", s.patchTodo, auth.ForEditors)
	huma.Delete(g, "/todos/{id}", s.deleteTodo, status(http.StatusNoContent), auth.ForEditors)
	huma.Post(g, "/todos/{id}/toggle", s.toggleTodo, auth.ForEditors)
}

type todoID struct {
	ID uint `path:"id"`
}

func (s *server) listTodos(ctx context.Context, in *struct {
	PageParams
	Completed   string `query:"completed" enum:"true,false," doc:"Empty = all"`
	RelatedType string `query:"related_type"`
	RelatedID   uint   `query:"related_id"`
	Name        string `query:"name"`
}) (*Out[ListResponse[Todo]], error) {
	q := s.scoped(ctx).Model(&model.Todo{})
	switch in.Completed {
	case "true":
		q = q.Where("completed_at IS NOT NULL").Order("completed_at DESC")
	case "false":
		q = q.Where("completed_at IS NULL").Order("CASE WHEN due_on = '' OR due_on IS NULL THEN 1 ELSE 0 END, due_on")
	default:
		q = q.Order("CASE WHEN completed_at IS NULL THEN 0 ELSE 1 END")
	}
	q = q.Order("created_at DESC, id DESC")
	if in.RelatedType != "" {
		q = q.Where("related_type = ?", in.RelatedType)
	}
	if in.RelatedID != 0 {
		q = q.Where("related_id = ?", in.RelatedID)
	}
	if in.Name != "" {
		q = q.Where("name = ?", in.Name)
	}
	return paginate(q, in.PageParams, toTodo)
}

// relatedModels are the record types a manual todo may point to.
var relatedModels = map[string]func() any{
	"invoice": func() any { return &model.Invoice{} }, "expense": func() any { return &model.Expense{} },
	"subject": func() any { return &model.Subject{} }, "price_item": func() any { return &model.PriceItem{} },
	"bank_transaction": func() any { return &model.BankTransaction{} }, "recurring": func() any { return &model.Recurring{} },
}

func (s *server) createTodo(ctx context.Context, in *struct{ Body TodoCreate }) (*Out[Todo], error) {
	b := in.Body
	m := model.Todo{AccountID: auth.AccountFrom(ctx).ID, Name: model.TodoManual, Text: strings.TrimSpace(b.Text), DueOn: b.DueOn}
	if m.Text == "" {
		return nil, invalid("text", "text must not be empty")
	}
	if m.DueOn != "" && !billing.ValidDate(m.DueOn) {
		return nil, invalid("due_on", "invalid date")
	}
	if (b.RelatedType == "") != (b.RelatedID == nil) {
		return nil, invalid("related_id", "related_type and related_id go together")
	}
	if b.RelatedType != "" {
		var n int64
		if err := s.scoped(ctx).Model(relatedModels[b.RelatedType]()).Where("id = ?", *b.RelatedID).Count(&n).Error; err != nil {
			return nil, dbErr(err, "todo")
		}
		if n == 0 {
			return nil, invalid("related_id", b.RelatedType+" not found")
		}
		m.RelatedType, m.RelatedID = b.RelatedType, b.RelatedID
	}
	if u := auth.UserFrom(ctx); u != nil {
		id := u.ID
		m.UserID = &id
	}
	now := s.deps.Now()
	m.CreatedAt, m.UpdatedAt = now, now
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, dbErr(err, "todo")
	}
	return &Out[Todo]{Body: toTodo(&m)}, nil
}

func (s *server) loadTodo(ctx context.Context, id uint) (*model.Todo, error) {
	var m model.Todo
	if err := s.scoped(ctx).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "todo")
	}
	return &m, nil
}

// setCompleted marks m (not) completed by a person.
func (s *server) setCompleted(m *model.Todo, done bool) {
	switch {
	case done && m.CompletedAt == nil:
		now := s.deps.Now()
		m.CompletedAt = &now
	case !done:
		m.CompletedAt = nil
	}
	m.AutoCompleted = false
}

func (s *server) patchTodo(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body TodoPatch
}) (*Out[Todo], error) {
	m, err := s.loadTodo(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	p := in.Body
	if m.Key != nil && (p.Text != nil || p.DueOn != nil) {
		return nil, conflict(CodeAutomaticTodo, "an automatic todo can only be completed or reopened")
	}
	if p.Text != nil {
		if m.Text = strings.TrimSpace(*p.Text); m.Text == "" {
			return nil, invalid("text", "text must not be empty")
		}
	}
	if p.DueOn != nil {
		if *p.DueOn != "" && !billing.ValidDate(*p.DueOn) {
			return nil, invalid("due_on", "invalid date")
		}
		m.DueOn = *p.DueOn
	}
	if p.Completed != nil {
		s.setCompleted(m, *p.Completed)
	}
	m.UpdatedAt = s.deps.Now()
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		return nil, dbErr(err, "todo")
	}
	return &Out[Todo]{Body: toTodo(m)}, nil
}

func (s *server) toggleTodo(ctx context.Context, in *todoID) (*Out[Todo], error) {
	m, err := s.loadTodo(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	s.setCompleted(m, m.CompletedAt == nil)
	m.UpdatedAt = s.deps.Now()
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		return nil, dbErr(err, "todo")
	}
	return &Out[Todo]{Body: toTodo(m)}, nil
}

func (s *server) deleteTodo(ctx context.Context, in *todoID) (*NoContent, error) {
	m, err := s.loadTodo(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if m.Key != nil {
		return nil, conflict(CodeAutomaticTodo, "an automatic todo cannot be deleted; complete it instead")
	}
	if err := s.db.WithContext(ctx).Delete(m).Error; err != nil {
		return nil, dbErr(err, "todo")
	}
	return &NoContent{}, nil
}
