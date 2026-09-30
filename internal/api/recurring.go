package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/scheduler"
)

// ---- DTOs ----

// Recurring issues an invoice from a template every months_period months (SPEC §7.3).
type Recurring struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	TemplateID       uint       `json:"template_id"`
	StartOn          string     `json:"start_on"`
	NextOccurrenceOn string     `json:"next_occurrence_on" doc:"Issue date of the next invoice"`
	EndOn            string     `json:"end_on" doc:"Last possible issue date; empty = no end"`
	MonthsPeriod     int        `json:"months_period"`
	DayOfMonth       *int       `json:"day_of_month,omitempty" doc:"Day of every occurrence (clamped to the month length); omitted = day of start_on"`
	IssueAs          string     `json:"issue_as" enum:"invoice,proforma"`
	SendEmail        bool       `json:"send_email"`
	Active           bool       `json:"active"`
	LastInvoiceID    *uint      `json:"last_invoice_id,omitempty"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	LastError        string     `json:"last_error" doc:"Error of the last failed generation (cleared by the next success)"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type RecurringCreate struct {
	Name         string `json:"name" minLength:"1" maxLength:"200"`
	TemplateID   uint   `json:"template_id" minimum:"1"`
	StartOn      string `json:"start_on,omitempty" format:"date" doc:"Default today"`
	EndOn        string `json:"end_on,omitempty" format:"date"`
	MonthsPeriod int    `json:"months_period,omitempty" minimum:"1" maximum:"120" doc:"Default 1 (monthly)"`
	DayOfMonth   *int   `json:"day_of_month,omitempty" minimum:"1" maximum:"31" doc:"31 = last day of every month"`
	IssueAs      string `json:"issue_as,omitempty" enum:"invoice,proforma" doc:"Default invoice"`
	SendEmail    bool   `json:"send_email,omitempty" doc:"E-mail every generated invoice to the client"`
	Active       *bool  `json:"active,omitempty" doc:"Default true"`
}

// RecurringPatch: nil = unchanged. Changing start_on/day_of_month recomputes
// next_occurrence_on (unless sent): from start_on when nothing was generated
// yet, otherwise the day is moved within the month of the next occurrence.
type RecurringPatch struct {
	Name             *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	TemplateID       *uint   `json:"template_id,omitempty" minimum:"1"`
	StartOn          *string `json:"start_on,omitempty" format:"date"`
	NextOccurrenceOn *string `json:"next_occurrence_on,omitempty" format:"date"`
	EndOn            *string `json:"end_on,omitempty" doc:"YYYY-MM-DD or \"\" (no end)"`
	MonthsPeriod     *int    `json:"months_period,omitempty" minimum:"1" maximum:"120"`
	DayOfMonth       *int    `json:"day_of_month,omitempty" minimum:"0" maximum:"31" doc:"0 = day of start_on"`
	IssueAs          *string `json:"issue_as,omitempty" enum:"invoice,proforma"`
	SendEmail        *bool   `json:"send_email,omitempty"`
}

func toRecurring(m *model.Recurring) Recurring {
	return Recurring{
		ID: m.ID, Name: m.Name, TemplateID: m.TemplateID, StartOn: m.StartOn, NextOccurrenceOn: m.NextOccurrenceOn,
		EndOn: m.EndOn, MonthsPeriod: m.MonthsPeriod, DayOfMonth: m.DayOfMonth, IssueAs: m.IssueAs,
		SendEmail: m.SendEmail, Active: m.Active, LastInvoiceID: m.LastInvoiceID, LastRunAt: m.LastRunAt,
		LastError: m.LastError, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ---- routes ----

func (s *server) registerRecurring(g huma.API) {
	huma.Get(g, "/recurring", s.listRecurring)
	huma.Post(g, "/recurring", s.createRecurring, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/recurring/{id}", s.getRecurring)
	huma.Patch(g, "/recurring/{id}", s.patchRecurring, auth.ForEditors)
	huma.Delete(g, "/recurring/{id}", s.deleteRecurring, status(http.StatusNoContent), auth.ForEditors)
	huma.Post(g, "/recurring/{id}/activate", s.activateRecurring, auth.ForEditors)
	huma.Post(g, "/recurring/{id}/deactivate", s.deactivateRecurring, auth.ForEditors)
	huma.Post(g, "/recurring/{id}/run-now", s.runRecurringNow, status(http.StatusCreated), auth.ForEditors)
}

type recurringID struct {
	ID uint `path:"id"`
}

func (s *server) listRecurring(ctx context.Context, in *struct {
	PageParams
	Active     string `query:"active" enum:"true,false" doc:"Filter by active flag"`
	TemplateID uint   `query:"template_id"`
}) (*Out[ListResponse[Recurring]], error) {
	q := s.scoped(ctx).Model(&model.Recurring{}).Order("next_occurrence_on, id")
	if in.Active != "" {
		q = q.Where("active = ?", in.Active == "true")
	}
	if in.TemplateID != 0 {
		q = q.Where("template_id = ?", in.TemplateID)
	}
	return paginate(q, in.PageParams, toRecurring)
}

func (s *server) loadRecurring(ctx context.Context, db *gorm.DB, id uint) (*model.Recurring, error) {
	var m model.Recurring
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "recurring")
	}
	return &m, nil
}

func (s *server) getRecurring(ctx context.Context, in *recurringID) (*Out[Recurring], error) {
	m, err := s.loadRecurring(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	return &Out[Recurring]{Body: toRecurring(m)}, nil
}

func (s *server) createRecurring(ctx context.Context, in *struct{ Body RecurringCreate }) (*Out[Recurring], error) {
	b := &in.Body
	m := &model.Recurring{
		AccountID: auth.AccountFrom(ctx).ID, Name: strings.TrimSpace(b.Name), TemplateID: b.TemplateID,
		StartOn: defaultStr(b.StartOn, s.today()), EndOn: b.EndOn, MonthsPeriod: b.MonthsPeriod,
		DayOfMonth: b.DayOfMonth, IssueAs: defaultStr(b.IssueAs, model.DocInvoice), SendEmail: b.SendEmail, Active: true,
	}
	if m.MonthsPeriod == 0 {
		m.MonthsPeriod = 1
	}
	apply(&m.Active, b.Active)
	var err error
	if m.NextOccurrenceOn, err = billing.FirstOccurrence(m.StartOn, m.AnchorDayIfSet()); err != nil {
		return nil, invalid("start_on", "invalid date")
	}
	db := s.db.WithContext(ctx)
	if err := s.checkRecurring(ctx, db, m); err != nil {
		return nil, err
	}
	if err := db.Create(m).Error; err != nil {
		return nil, dbErr(err, "recurring")
	}
	return &Out[Recurring]{Body: toRecurring(m)}, nil
}

func (s *server) patchRecurring(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body RecurringPatch
}) (*Out[Recurring], error) {
	db := s.db.WithContext(ctx)
	m, err := s.loadRecurring(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	p := &in.Body
	if p.Name != nil {
		m.Name = strings.TrimSpace(*p.Name)
	}
	apply(&m.TemplateID, p.TemplateID)
	apply(&m.EndOn, p.EndOn)
	apply(&m.MonthsPeriod, p.MonthsPeriod)
	apply(&m.IssueAs, p.IssueAs)
	apply(&m.SendEmail, p.SendEmail)
	scheduleChanged := false
	if p.StartOn != nil && *p.StartOn != m.StartOn {
		m.StartOn, scheduleChanged = *p.StartOn, true
	}
	if p.DayOfMonth != nil {
		if *p.DayOfMonth == 0 {
			m.DayOfMonth = nil
		} else {
			m.DayOfMonth = p.DayOfMonth
		}
		scheduleChanged = true
	}
	switch {
	case p.NextOccurrenceOn != nil:
		m.NextOccurrenceOn = *p.NextOccurrenceOn
	case scheduleChanged && m.LastInvoiceID == nil:
		if m.NextOccurrenceOn, err = billing.FirstOccurrence(m.StartOn, m.AnchorDayIfSet()); err != nil {
			return nil, invalid("start_on", "invalid date")
		}
	case scheduleChanged: // keep the month of the next occurrence, move the day
		if m.NextOccurrenceOn, err = billing.AddMonths(m.NextOccurrenceOn, 0, m.AnchorDay()); err != nil {
			return nil, invalid("next_occurrence_on", "invalid date")
		}
	}
	if err := s.checkRecurring(ctx, db, m); err != nil {
		return nil, err
	}
	if err := db.Save(m).Error; err != nil {
		return nil, dbErr(err, "recurring")
	}
	return &Out[Recurring]{Body: toRecurring(m)}, nil
}

func (s *server) deleteRecurring(ctx context.Context, in *recurringID) (*NoContent, error) {
	db := s.db.WithContext(ctx)
	m, err := s.loadRecurring(ctx, db, in.ID)
	if err != nil {
		return nil, err
	}
	if err := db.Delete(m).Error; err != nil {
		return nil, dbErr(err, "recurring")
	}
	return &NoContent{}, nil
}

// activateRecurring re-activates a recurring invoice. Occurrences missed
// while it was inactive are skipped: a past next_occurrence_on is moved to
// the first occurrence on or after today.
func (s *server) activateRecurring(ctx context.Context, in *recurringID) (*Out[Recurring], error) {
	return s.setRecurringActive(ctx, in.ID, true)
}

func (s *server) deactivateRecurring(ctx context.Context, in *recurringID) (*Out[Recurring], error) {
	return s.setRecurringActive(ctx, in.ID, false)
}

func (s *server) setRecurringActive(ctx context.Context, id uint, active bool) (*Out[Recurring], error) {
	db := s.db.WithContext(ctx)
	m, err := s.loadRecurring(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if active && !m.Active {
		today := s.today()
		for i := 0; m.NextOccurrenceOn < today && i < maxCatchUp; i++ {
			if m.NextOccurrenceOn, err = billing.AddMonths(m.NextOccurrenceOn, m.MonthsPeriod, m.AnchorDay()); err != nil {
				return nil, huma.Error500InternalServerError("invalid next_occurrence_on", err)
			}
		}
		if m.EndOn != "" && m.NextOccurrenceOn > m.EndOn {
			return nil, conflict(CodeRecurringEnded, "the recurring invoice has ended (end_on "+m.EndOn+"); change end_on first")
		}
	}
	m.Active = active
	if err := db.Save(m).Error; err != nil {
		return nil, dbErr(err, "recurring")
	}
	return &Out[Recurring]{Body: toRecurring(m)}, nil
}

// runRecurringNow issues an invoice right away (dated today, whatever the
// schedule or active flag) and advances next_occurrence_on by one period.
func (s *server) runRecurringNow(ctx context.Context, in *recurringID) (*Out[Invoice], error) {
	inv, r, err := s.issueRecurring(ctx, in.ID, s.today(), true)
	if err != nil {
		return nil, err
	}
	if r.SendEmail {
		s.sendGeneratedInvoice(ctx, inv.ID)
	}
	return s.invoiceOut(ctx, s.db.WithContext(ctx), inv.ID)
}

// ---- generation ----

// maxCatchUp bounds the invoices generated for one recurring in one run.
const maxCatchUp = 1000

// checkRecurring validates name, template, dates → 422.
func (s *server) checkRecurring(ctx context.Context, db *gorm.DB, m *model.Recurring) error {
	if m.Name == "" {
		return invalid("name", "name must not be empty")
	}
	if !billing.ValidDate(m.StartOn) {
		return invalid("start_on", "invalid date")
	}
	if !billing.ValidDate(m.NextOccurrenceOn) {
		return invalid("next_occurrence_on", "invalid date")
	}
	if m.EndOn != "" {
		if !billing.ValidDate(m.EndOn) {
			return invalid("end_on", "invalid date")
		}
		if m.EndOn < m.StartOn {
			return invalid("end_on", "end_on must not be before start_on")
		}
	}
	var t model.InvoiceTemplate
	if err := db.Scopes(inAccount(ctx)).Select("id").First(&t, m.TemplateID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid("template_id", "template not found")
		}
		return dbErr(err, "template")
	}
	return nil
}

// issueRecurring issues the next invoice of recurring id in one transaction:
// the recurring row is locked and re-checked, the invoice created and
// next_occurrence_on advanced by one period (the recurring deactivates itself
// after end_on). Scheduled runs (force=false) issue only an active, due
// (next_occurrence_on ≤ today, ≤ end_on) recurring, dated next_occurrence_on,
// and return a nil invoice when nothing is due; force issues it dated today.
func (s *server) issueRecurring(ctx context.Context, id uint, today string, force bool) (*model.Invoice, *model.Recurring, error) {
	var inv *model.Invoice
	var r model.Recurring
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Scopes(inAccount(ctx)).First(&r, id).Error; err != nil {
			return dbErr(err, "recurring")
		}
		issuedOn := today
		if !force {
			if !r.Active || r.NextOccurrenceOn > today || (r.EndOn != "" && r.NextOccurrenceOn > r.EndOn) {
				return nil
			}
			issuedOn = r.NextOccurrenceOn
		}
		var t model.InvoiceTemplate
		if err := tx.Scopes(inAccount(ctx)).First(&t, r.TemplateID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return conflict(CodeTemplateMissing, "the template of the recurring invoice no longer exists")
			}
			return dbErr(err, "template")
		}
		body := templateInvoice(ctx, &t, r.IssueAs, issuedOn)
		m, err := s.createInvoiceTx(ctx, tx, &body)
		if err != nil {
			return err
		}
		m.RecurringID = &r.ID
		if err := tx.Model(m).Update("recurring_id", r.ID).Error; err != nil {
			return dbErr(err, "invoice")
		}
		next, err := billing.AddMonths(r.NextOccurrenceOn, r.MonthsPeriod, r.AnchorDay())
		if err != nil {
			return huma.Error500InternalServerError("invalid next_occurrence_on", err)
		}
		now := s.deps.Now()
		r.NextOccurrenceOn, r.LastInvoiceID, r.LastRunAt, r.LastError = next, &m.ID, &now, ""
		if r.EndOn != "" && next > r.EndOn {
			r.Active = false
		}
		if err := tx.Save(&r).Error; err != nil {
			return dbErr(err, "recurring")
		}
		inv = m
		return recordRecurring(ctx, tx, &r, m, "")
	})
	if err != nil {
		return nil, nil, err
	}
	return inv, &r, nil
}

// RunRecurring is the scheduler job: for every active recurring whose
// next_occurrence_on ≤ today it issues the due invoices one by one (catching
// up missed periods, each dated its own occurrence) and e-mails them when
// send_email is set. A failure is stored in last_error and retried next run.
func (s *server) RunRecurring(ctx context.Context, now time.Time) error {
	today := billing.Today(now)
	var due []model.Recurring
	if err := s.db.WithContext(ctx).Where("active = ? AND next_occurrence_on <= ?", true, today).
		Order("id").Find(&due).Error; err != nil {
		return err
	}
	var errs []error
	for _, r := range due {
		actx, err := s.systemContext(ctx, r.AccountID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for i := 0; i < maxCatchUp; i++ {
			inv, rec, err := s.issueRecurring(actx, r.ID, today, false)
			if err != nil {
				errs = append(errs, fmt.Errorf("recurring %d: %w", r.ID, err))
				if ferr := s.recurringFailed(actx, r.ID, err); ferr != nil {
					errs = append(errs, ferr)
				}
				break
			}
			if inv == nil {
				break
			}
			if rec.SendEmail {
				s.sendGeneratedInvoice(actx, inv.ID)
			}
		}
	}
	return errors.Join(errs...)
}

// recurringFailed stores a failed scheduled generation in last_error and
// records recurring.failed (+ todo) in its own transaction.
func (s *server) recurringFailed(ctx context.Context, id uint, cause error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var r model.Recurring
		if err := tx.Scopes(inAccount(ctx)).First(&r, id).Error; err != nil {
			return err
		}
		now := s.deps.Now()
		r.LastError, r.LastRunAt = cause.Error(), &now
		if err := tx.Model(&r).Updates(map[string]any{"last_error": r.LastError, "last_run_at": now}).Error; err != nil {
			return err
		}
		return recordRecurring(ctx, tx, &r, nil, r.LastError)
	})
}

// systemContext acts in account id outside a request (scheduler jobs).
func (s *server) systemContext(ctx context.Context, accountID uint) (context.Context, error) {
	var acc model.Account
	if err := s.db.WithContext(ctx).First(&acc, accountID).Error; err != nil {
		return nil, fmt.Errorf("account %d: %w", accountID, err)
	}
	actx := auth.WithAccount(ctx, &acc, "system")
	if _, ok := ctx.Value(clockKey{}).(func() time.Time); !ok {
		actx = withClock(actx, s.deps.Now)
	}
	return actx, nil
}

// Jobs returns the API's scheduler jobs (recurring invoices, reminders) using
// the same dependencies (clock, mailer, storage) as New.
func Jobs(db *gorm.DB, cfg config.Config, deps Deps) []scheduler.Job {
	s := newServer(db, cfg, deps)
	return []scheduler.Job{
		{Name: "recurring", Run: s.RunRecurring, Every: time.Hour},
		{Name: "reminders", Run: s.RunReminders, Every: time.Hour},
		{Name: "bank-sync", Run: s.RunBankSync, Every: 2 * time.Hour},
		{Name: "todos", Run: s.RunTodos, Every: time.Hour},
		{Name: "webhooks", Run: s.RunWebhooks}, // every tick (cmd/server ticks every minute)
	}
}

// logJobErr logs a background failure that has no caller to report to.
func logJobErr(msg string, args ...any) {
	slog.Warn(msg, args...)
}
