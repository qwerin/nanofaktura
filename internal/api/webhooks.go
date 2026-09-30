package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/webhooks"
)

// Webhook is a webhook subscription; the secret is only returned on create
// and on rotate_secret.
type Webhook struct {
	ID                  uint       `json:"id"`
	URL                 string     `json:"url"`
	Description         string     `json:"description"`
	Events              []string   `json:"events" nullable:"false"`
	Active              bool       `json:"active"`
	HasSecret           bool       `json:"has_secret"`
	Secret              string     `json:"secret,omitempty" doc:"Signing secret; only in the response of create and rotate_secret"`
	LastStatus          int        `json:"last_status" doc:"HTTP status of the last attempt, 0 = none / network error"`
	LastDeliveredAt     *time.Time `json:"last_delivered_at,omitempty"`
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures" doc:"Finally failed deliveries in a row; 20 disable the webhook"`
	DisabledAt          *time.Time `json:"disabled_at,omitempty" doc:"Set when disabled automatically"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type WebhookCreate struct {
	URL         string   `json:"url" minLength:"1" maxLength:"2000"`
	Description string   `json:"description,omitempty" maxLength:"200"`
	Events      []string `json:"events,omitempty" doc:"Event names, \"prefix.*\" or \"*\"; default [\"*\"]"`
	Active      *bool    `json:"active,omitempty" doc:"Default true"`
}

type WebhookPatch struct {
	URL          *string  `json:"url,omitempty" minLength:"1" maxLength:"2000"`
	Description  *string  `json:"description,omitempty" maxLength:"200"`
	Events       []string `json:"events,omitempty"`
	Active       *bool    `json:"active,omitempty" doc:"true also resets the failure counter"`
	RotateSecret bool     `json:"rotate_secret,omitempty" doc:"Generate a new secret (returned once)"`
}

// WebhookDelivery is one queued or attempted delivery.
type WebhookDelivery struct {
	ID             uint       `json:"id"`
	WebhookID      uint       `json:"webhook_id"`
	EventID        *uint      `json:"event_id,omitempty" doc:"Missing for pings"`
	Event          string     `json:"event"`
	Status         string     `json:"status" enum:"pending,delivered,failed"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  *time.Time `json:"next_attempt_at,omitempty"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	ResponseStatus int        `json:"response_status"`
	ResponseBody   string     `json:"response_body" doc:"First 1 KB"`
	Error          string     `json:"error"`
	DurationMs     int64      `json:"duration_ms"`
	Payload        string     `json:"payload" doc:"The exact JSON body sent"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toWebhook(m *model.Webhook) Webhook {
	ev := m.Events
	if ev == nil {
		ev = []string{}
	}
	return Webhook{ID: m.ID, URL: m.URL, Description: m.Description, Events: ev, Active: m.Active, HasSecret: m.Secret != "",
		LastStatus: m.LastStatus, LastDeliveredAt: m.LastDeliveredAt, LastError: m.LastError,
		ConsecutiveFailures: m.ConsecutiveFailures, DisabledAt: m.DisabledAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func toWebhookDelivery(m *model.WebhookDelivery) WebhookDelivery {
	return WebhookDelivery{ID: m.ID, WebhookID: m.WebhookID, EventID: m.EventID, Event: m.EventName, Status: m.Status,
		Attempts: m.Attempts, NextAttemptAt: m.NextAttemptAt, LastAttemptAt: m.LastAttemptAt, ResponseStatus: m.ResponseStatus,
		ResponseBody: m.ResponseBody, Error: m.Error, DurationMs: m.DurationMs, Payload: m.Payload, CreatedAt: m.CreatedAt}
}

func (s *server) registerWebhooks(g huma.API) {
	huma.Get(g, "/webhooks", s.listWebhooks, auth.ForManagers)
	huma.Post(g, "/webhooks", s.createWebhook, status(http.StatusCreated), auth.ForManagers)
	huma.Get(g, "/webhooks/{id}", s.getWebhook, auth.ForManagers)
	huma.Patch(g, "/webhooks/{id}", s.patchWebhook, auth.ForManagers)
	huma.Delete(g, "/webhooks/{id}", s.deleteWebhook, status(http.StatusNoContent), auth.ForManagers)
	huma.Post(g, "/webhooks/{id}/test", s.testWebhook, auth.ForManagers)
	huma.Get(g, "/webhooks/{id}/deliveries", s.listWebhookDeliveries, auth.ForManagers)
	huma.Post(g, "/webhooks/{id}/deliveries/{delivery_id}/redeliver", s.redeliverWebhook, status(http.StatusCreated), auth.ForManagers)
}

type webhookID struct {
	ID uint `path:"id"`
}

func (s *server) listWebhooks(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[Webhook]], error) {
	return paginate(s.scoped(ctx).Model(&model.Webhook{}).Order("id"), in.PageParams, toWebhook)
}

func (s *server) loadWebhook(ctx context.Context, db *gorm.DB, id uint) (*model.Webhook, error) {
	var m model.Webhook
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "webhook")
	}
	return &m, nil
}

func (s *server) getWebhook(ctx context.Context, in *webhookID) (*Out[Webhook], error) {
	m, err := s.loadWebhook(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	return &Out[Webhook]{Body: toWebhook(m)}, nil
}

// checkWebhookURL validates the URL incl. SSRF protection (422 on url).
func (s *server) checkWebhookURL(ctx context.Context, raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if err := webhooks.CheckURL(ctx, u, s.cfg.WebhooksAllowPrivate); err != nil {
		return "", invalid("url", err.Error())
	}
	return u, nil
}

func normalizeWebhookEvents(list []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for i, e := range list {
		e = strings.TrimSpace(e)
		if !events.ValidPattern(e) {
			return nil, invalid(fmt.Sprintf("events[%d]", i), "unknown event "+e+` (use a name from GET /events/catalog, "prefix.*" or "*")`)
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		out = []string{"*"}
	}
	return out, nil
}

// newWebhookSecret returns a fresh secret and its encrypted form.
func (s *server) newWebhookSecret() (plain, enc string, err error) {
	plain = webhooks.NewSecret()
	if enc, err = s.deps.Secrets.Encrypt(plain); err != nil {
		return "", "", huma.Error500InternalServerError("cannot encrypt secret", err)
	}
	return plain, enc, nil
}

func (s *server) createWebhook(ctx context.Context, in *struct{ Body WebhookCreate }) (*Out[Webhook], error) {
	b := in.Body
	u, err := s.checkWebhookURL(ctx, b.URL)
	if err != nil {
		return nil, err
	}
	evs, err := normalizeWebhookEvents(b.Events)
	if err != nil {
		return nil, err
	}
	plain, enc, err := s.newWebhookSecret()
	if err != nil {
		return nil, err
	}
	m := model.Webhook{AccountID: auth.AccountFrom(ctx).ID, URL: u, Description: strings.TrimSpace(b.Description),
		Events: evs, Secret: enc, Active: true}
	apply(&m.Active, b.Active)
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, dbErr(err, "webhook")
	}
	out := toWebhook(&m)
	out.Secret = plain
	return &Out[Webhook]{Body: out}, nil
}

func (s *server) patchWebhook(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body WebhookPatch
}) (*Out[Webhook], error) {
	p := in.Body
	m, err := s.loadWebhook(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	if p.URL != nil {
		if m.URL, err = s.checkWebhookURL(ctx, *p.URL); err != nil {
			return nil, err
		}
	}
	if p.Description != nil {
		m.Description = strings.TrimSpace(*p.Description)
	}
	if p.Events != nil {
		if m.Events, err = normalizeWebhookEvents(p.Events); err != nil {
			return nil, err
		}
	}
	if p.Active != nil {
		if *p.Active && !m.Active {
			m.ConsecutiveFailures, m.DisabledAt = 0, nil
		}
		m.Active = *p.Active
	}
	plain := ""
	// a webhook without secret (imported from a backup) gets one when activated
	if p.RotateSecret || (m.Active && m.Secret == "") {
		if plain, m.Secret, err = s.newWebhookSecret(); err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Select("*").Save(m).Error; err != nil {
		return nil, dbErr(err, "webhook")
	}
	out := toWebhook(m)
	out.Secret = plain
	return &Out[Webhook]{Body: out}, nil
}

func (s *server) deleteWebhook(ctx context.Context, in *webhookID) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.loadWebhook(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if err := tx.Where("webhook_id = ?", m.ID).Delete(&model.WebhookDelivery{}).Error; err != nil {
			return dbErr(err, "webhook delivery")
		}
		return dbErrOrNil(tx.Delete(m).Error, "webhook")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) listWebhookDeliveries(ctx context.Context, in *struct {
	ID uint `path:"id"`
	PageParams
	Status string `query:"status" enum:"pending,delivered,failed,"`
}) (*Out[ListResponse[WebhookDelivery]], error) {
	if _, err := s.loadWebhook(ctx, s.db.WithContext(ctx), in.ID); err != nil {
		return nil, err
	}
	q := s.scoped(ctx).Model(&model.WebhookDelivery{}).Where("webhook_id = ?", in.ID).Order("created_at DESC, id DESC")
	if in.Status != "" {
		q = q.Where("status = ?", in.Status)
	}
	return paginate(q, in.PageParams, toWebhookDelivery)
}

// WebhookTestResult is the outcome of a synchronous ping.
type WebhookTestResult struct {
	DeliveryID   uint   `json:"delivery_id"`
	OK           bool   `json:"ok"`
	StatusCode   int    `json:"status_code" doc:"0 = no response"`
	ResponseBody string `json:"response_body" doc:"First 1 KB"`
	Error        string `json:"error"`
	DurationMs   int64  `json:"duration_ms"`
}

// testWebhook POSTs a "ping" event synchronously (also to an inactive
// webhook); the attempt is logged as a delivery without retries.
func (s *server) testWebhook(ctx context.Context, in *webhookID) (*Out[WebhookTestResult], error) {
	m, err := s.loadWebhook(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	now := s.deps.Now()
	acc := auth.AccountFrom(ctx)
	payload, _ := json.Marshal(events.PayloadBody{Event: "ping", CreatedAt: now.UTC(), Account: acc.Slug,
		Text: "Testovací zpráva z NanoFaktury", Data: map[string]any{"webhook_id": m.ID}})
	d := &model.WebhookDelivery{AccountID: acc.ID, WebhookID: m.ID, EventName: "ping", Payload: string(payload),
		Status: model.DeliveryPending, NextAttemptAt: &now, CreatedAt: now}
	if err := s.db.WithContext(ctx).Create(d).Error; err != nil {
		return nil, dbErr(err, "webhook delivery")
	}
	d, err = s.attemptDelivery(ctx, d.ID, now, true)
	if err != nil {
		return nil, err
	}
	return &Out[WebhookTestResult]{Body: WebhookTestResult{DeliveryID: d.ID, OK: d.Status == model.DeliveryDelivered,
		StatusCode: d.ResponseStatus, ResponseBody: d.ResponseBody, Error: d.Error, DurationMs: d.DurationMs}}, nil
}

// redeliverWebhook queues a copy of a delivery (same payload) and attempts
// it right away; when that fails it continues with the retry schedule.
func (s *server) redeliverWebhook(ctx context.Context, in *struct {
	ID         uint `path:"id"`
	DeliveryID uint `path:"delivery_id"`
}) (*Out[WebhookDelivery], error) {
	if _, err := s.loadWebhook(ctx, s.db.WithContext(ctx), in.ID); err != nil {
		return nil, err
	}
	var src model.WebhookDelivery
	if err := s.scoped(ctx).Where("webhook_id = ?", in.ID).First(&src, in.DeliveryID).Error; err != nil {
		return nil, dbErr(err, "webhook delivery")
	}
	now := s.deps.Now()
	d := &model.WebhookDelivery{AccountID: src.AccountID, WebhookID: src.WebhookID, EventID: src.EventID, EventName: src.EventName,
		Payload: src.Payload, Status: model.DeliveryPending, NextAttemptAt: &now, CreatedAt: now}
	if err := s.db.WithContext(ctx).Create(d).Error; err != nil {
		return nil, dbErr(err, "webhook delivery")
	}
	d, err := s.attemptDelivery(ctx, d.ID, now, true)
	if err != nil {
		return nil, err
	}
	return &Out[WebhookDelivery]{Body: toWebhookDelivery(d)}, nil
}

// webhookBatch is the most deliveries one job run attempts.
const webhookBatch = 100

// webhookLease is how long a claimed delivery is hidden from other runs
// while its HTTP request is in flight.
const webhookLease = 3 * webhooks.Timeout

// RunWebhooks is the scheduler job (every tick = every minute): attempts
// pending deliveries whose next_attempt_at has come, oldest first.
func (s *server) RunWebhooks(ctx context.Context, now time.Time) error {
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&model.WebhookDelivery{}).
		Where("status = ? AND next_attempt_at <= ?", model.DeliveryPending, now).
		Order("next_attempt_at, id").Limit(webhookBatch).Pluck("id", &ids).Error; err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if _, err := s.attemptDelivery(ctx, id, now, false); err != nil {
			errs = append(errs, fmt.Errorf("webhook delivery %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// attemptDelivery claims delivery id (pending and due, or any pending with
// force), POSTs it outside a transaction and stores the outcome: delivered;
// or the next retry (webhooks.RetryDelays); or failed — then the webhook's
// failure counter grows, webhook.failed is recorded and after
// webhooks.MaxConsecutiveFailures the webhook is disabled (webhook.disabled).
// Pings are attempted once. Returns the updated delivery (nil when it was
// not claimable).
func (s *server) attemptDelivery(ctx context.Context, id uint, now time.Time, force bool) (*model.WebhookDelivery, error) {
	var d model.WebhookDelivery
	var hook model.Webhook
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&d, id).Error; err != nil {
			return err
		}
		if d.Status != model.DeliveryPending || (!force && (d.NextAttemptAt == nil || d.NextAttemptAt.After(now))) {
			return nil
		}
		err := tx.First(&hook, d.WebhookID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !hook.Active && d.EventID != nil && !force) {
			return tx.Model(&d).Updates(map[string]any{"status": model.DeliveryFailed, "next_attempt_at": nil,
				"error": "webhook is disabled or deleted"}).Error
		}
		if err != nil {
			return err
		}
		lease := now.Add(webhookLease)
		claimed = true
		return tx.Model(&d).Update("next_attempt_at", lease).Error
	})
	if err != nil {
		return nil, err
	}
	if !claimed {
		return &d, nil
	}

	var res webhooks.Result
	secret, err := s.deps.Secrets.Decrypt(hook.Secret)
	if err != nil {
		res = webhooks.Result{Err: "the stored secret cannot be decrypted (was NANOFAKTURA_SECRET_KEY changed?); rotate the secret"}
	} else {
		res = webhooks.Post(ctx, s.webhookClient, hook.URL, secret, d.EventName, d.ID, []byte(d.Payload))
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&d, id).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&hook, d.WebhookID).Error; err != nil {
			return err
		}
		d.Attempts++
		d.LastAttemptAt, d.ResponseStatus, d.ResponseBody, d.Error, d.DurationMs =
			&now, res.Status, res.Body, res.Err, res.Duration.Milliseconds()
		hook.LastStatus, hook.LastError = res.Status, res.Err
		finalFail := false
		switch {
		case res.OK:
			d.Status, d.NextAttemptAt = model.DeliveryDelivered, nil
			hook.LastDeliveredAt = &now
			if d.EventID != nil {
				hook.ConsecutiveFailures = 0
			}
		case d.EventID == nil: // ping: no retries
			d.Status, d.NextAttemptAt = model.DeliveryFailed, nil
		default:
			if next, ok := webhooks.NextAttempt(d.Attempts, now); ok {
				d.NextAttemptAt = &next
			} else {
				d.Status, d.NextAttemptAt, finalFail = model.DeliveryFailed, nil, true
			}
		}
		if err := tx.Select("*").Save(&d).Error; err != nil {
			return err
		}
		disable := false
		if finalFail {
			hook.ConsecutiveFailures++
			if hook.Active && hook.ConsecutiveFailures >= webhooks.MaxConsecutiveFailures {
				hook.Active, hook.DisabledAt, disable = false, &now, true
			}
		}
		if err := tx.Select("*").Save(&hook).Error; err != nil {
			return err
		}
		if !finalFail {
			return nil
		}
		actx, err := s.systemContextTx(ctx, tx, hook.AccountID, now)
		if err != nil {
			return err
		}
		// only the host: webhook URLs often carry a secret (path/query token)
		host := webhooks.Host(hook.URL)
		data := map[string]any{"webhook_id": hook.ID, "host": host, "delivery_id": d.ID, "event": d.EventName,
			"attempts": d.Attempts, "error": d.Error, "consecutive_failures": hook.ConsecutiveFailures}
		if err := record(actx, tx, events.Event{Name: events.WebhookFailed, SubjectType: events.SubjectWebhook, SubjectID: hook.ID,
			Text: fmt.Sprintf("Webhook %s: událost %s se nepodařilo doručit ani po %d pokusech (%s)", host, d.EventName, d.Attempts, d.Error),
			Data: data}); err != nil {
			return err
		}
		if !disable {
			return nil
		}
		return record(actx, tx, events.Event{Name: events.WebhookDisabled, SubjectType: events.SubjectWebhook, SubjectID: hook.ID,
			Text: fmt.Sprintf("Webhook %s byl vypnut po %d neúspěšných doručeních za sebou", host, hook.ConsecutiveFailures), Data: data})
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// systemContextTx is systemContext inside a transaction (reads with tx).
func (s *server) systemContextTx(ctx context.Context, tx *gorm.DB, accountID uint, now time.Time) (context.Context, error) {
	var acc model.Account
	if err := tx.First(&acc, accountID).Error; err != nil {
		return nil, fmt.Errorf("account %d: %w", accountID, err)
	}
	return withClock(auth.WithAccount(ctx, &acc, "system"), func() time.Time { return now }), nil
}
