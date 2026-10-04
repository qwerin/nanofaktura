package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/webhooks"
)

// hookReceiver is an httptest server recording webhook requests.
type hookReceiver struct {
	*httptest.Server
	mu     sync.Mutex
	status int
	reqs   []receivedHook
}

type receivedHook struct {
	header http.Header
	body   []byte
}

func newHookReceiver(t *testing.T) *hookReceiver {
	h := &hookReceiver{status: http.StatusOK}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		defer h.mu.Unlock()
		h.reqs = append(h.reqs, receivedHook{r.Header.Clone(), b})
		w.WriteHeader(h.status)
		_, _ = w.Write([]byte("ok from receiver"))
	}))
	t.Cleanup(h.Close)
	return h
}

func (h *hookReceiver) setStatus(s int) {
	h.mu.Lock()
	h.status = s
	h.mu.Unlock()
}

func (h *hookReceiver) received() []receivedHook {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]receivedHook(nil), h.reqs...)
}

func hookURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/webhooks"), id, suffix)
}

func deliveries(c *client, hookID uint) []api.WebhookDelivery {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.WebhookDelivery]](c, http.StatusOK, "GET", hookURL(c, hookID, "/deliveries"), nil).Items
}

func TestWebhookDelivery(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	rcv := newHookReceiver(t)

	hook := doJSON[api.Webhook](a, http.StatusCreated, "POST", a.acct("/webhooks"),
		api.WebhookCreate{URL: rcv.URL + "/hook", Events: []string{"invoice.*", "invoice.*"}})
	if hook.Secret == "" || !hook.HasSecret || !hook.Active || fmt.Sprint(hook.Events) != "[invoice.*]" {
		t.Fatalf("created %+v", hook)
	}
	got := doJSON[api.Webhook](a, http.StatusOK, "GET", hookURL(a, hook.ID, ""), nil)
	if got.Secret != "" || !got.HasSecret {
		t.Fatalf("secret leaked on read: %+v", got)
	}
	res, body := a.do("POST", a.acct("/webhooks"), api.WebhookCreate{URL: rcv.URL, Events: []string{"nonsense.*"}})
	assertError(t, res, body, http.StatusUnprocessableEntity, "unknown event")

	subj := newSubject(a, api.SubjectCreate{Name: "ACME"}) // subject.* does not match the filter
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("Práce", "1", 1000_00, nil)}})
	ds := deliveries(a, hook.ID)
	if len(ds) != 1 || ds[0].Event != "invoice.created" || ds[0].Status != "pending" {
		t.Fatalf("queued %+v", ds)
	}
	if len(rcv.received()) != 0 {
		t.Fatal("delivered synchronously")
	}

	mustRunJob(ts, "webhooks")
	reqs := rcv.received()
	if len(reqs) != 1 {
		t.Fatalf("received %d", len(reqs))
	}
	r := reqs[0]
	if r.header.Get(webhooks.HeaderEvent) != "invoice.created" || r.header.Get(webhooks.HeaderDelivery) != fmt.Sprint(ds[0].ID) ||
		!webhooks.Verify(hook.Secret, r.body, r.header.Get(webhooks.HeaderSignature)) {
		t.Fatalf("headers %v", r.header)
	}
	var payload events.PayloadBody
	if err := json.Unmarshal(r.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event != "invoice.created" || payload.Account != a.slug || payload.Subject.ID != inv.ID ||
		payload.Data["number"] != inv.Number || payload.ID == 0 {
		t.Fatalf("payload %+v", payload)
	}
	ds = deliveries(a, hook.ID)
	if ds[0].Status != "delivered" || ds[0].Attempts != 1 || ds[0].ResponseStatus != 200 || ds[0].ResponseBody != "ok from receiver" {
		t.Fatalf("delivery %+v", ds[0])
	}
	mustRunJob(ts, "webhooks") // nothing due
	if len(rcv.received()) != 1 {
		t.Fatal("delivered twice")
	}

	// redeliver: new delivery, attempted right away
	rd := doJSON[api.WebhookDelivery](a, http.StatusCreated, "POST", hookURL(a, hook.ID, fmt.Sprintf("/deliveries/%d/redeliver", ds[0].ID)), nil)
	if rd.ID == ds[0].ID || rd.Status != "delivered" || rd.Payload != ds[0].Payload || len(rcv.received()) != 2 {
		t.Fatalf("redelivered %+v", rd)
	}

	// test ping
	ping := doJSON[api.WebhookTestResult](a, http.StatusOK, "POST", hookURL(a, hook.ID, "/test"), nil)
	if !ping.OK || ping.StatusCode != 200 || len(rcv.received()) != 3 {
		t.Fatalf("ping %+v", ping)
	}
	rcv.setStatus(http.StatusInternalServerError)
	ping = doJSON[api.WebhookTestResult](a, http.StatusOK, "POST", hookURL(a, hook.ID, "/test"), nil)
	if ping.OK || ping.StatusCode != 500 || ping.Error != "HTTP 500" {
		t.Fatalf("failing ping %+v", ping)
	}

	// rotate secret
	rot := doJSON[api.Webhook](a, http.StatusOK, "PATCH", hookURL(a, hook.ID, ""), map[string]any{"rotate_secret": true})
	if rot.Secret == "" || rot.Secret == hook.Secret {
		t.Fatalf("rotated %+v", rot)
	}

	// tenant isolation
	res, body = b.do("GET", hookURL(b, hook.ID, "/deliveries"), nil)
	assertError(t, res, body, http.StatusNotFound, "webhook not found")
	if l := doJSON[api.ListResponse[api.Webhook]](b, http.StatusOK, "GET", b.acct("/webhooks"), nil); l.Total != 0 {
		t.Fatalf("b sees %+v", l.Items)
	}

	a.mustDo(http.StatusNoContent, "DELETE", hookURL(a, hook.ID, ""), nil)
	var n int64
	ts.db.Model(&model.WebhookDelivery{}).Count(&n)
	if n != 0 {
		t.Fatalf("%d deliveries left", n)
	}
}

// TestWebhookRetrySchedule: failures are retried after 1m/5m/30m/2h/12h,
// then the delivery fails (webhook.failed); 20 failed deliveries in a row
// disable the webhook.
func TestWebhookRetrySchedule(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	rcv := newHookReceiver(t)
	rcv.setStatus(http.StatusServiceUnavailable)
	hook := doJSON[api.Webhook](a, http.StatusCreated, "POST", a.acct("/webhooks"), api.WebhookCreate{URL: rcv.URL})
	other := doJSON[api.Webhook](a, http.StatusCreated, "POST", a.acct("/webhooks"),
		api.WebhookCreate{URL: rcv.URL + "/other", Events: []string{"webhook.*"}, Active: boolPtr(false)})
	newSubject(a, api.SubjectCreate{Name: "ACME"})

	start := ts.now
	mustRunJob(ts, "webhooks")
	for i, delay := range webhooks.RetryDelays {
		d := deliveries(a, hook.ID)[0]
		want := ts.now.Add(delay)
		if d.Status != "pending" || d.Attempts != i+1 || d.NextAttemptAt == nil || !d.NextAttemptAt.Equal(want) || d.Error != "HTTP 503" {
			t.Fatalf("after attempt %d: %+v (want next %v)", i+1, d, want)
		}
		ts.now = ts.now.Add(delay - time.Second)
		mustRunJob(ts, "webhooks") // not due yet
		if got := deliveries(a, hook.ID)[0].Attempts; got != i+1 {
			t.Fatalf("attempted early: %d", got)
		}
		ts.now = ts.now.Add(time.Second)
		mustRunJob(ts, "webhooks")
	}
	d := deliveries(a, hook.ID)[0]
	if d.Status != "failed" || d.Attempts != 6 || d.NextAttemptAt != nil {
		t.Fatalf("final %+v", d)
	}
	if ts.now.Sub(start) != 14*time.Hour+36*time.Minute {
		t.Fatalf("schedule took %v", ts.now.Sub(start))
	}
	var failed []model.Event
	ts.db.Where("name = ?", "webhook.failed").Find(&failed)
	if len(failed) != 1 || failed[0].SubjectID != hook.ID {
		t.Fatalf("webhook.failed events %+v", failed)
	}
	h := doJSON[api.Webhook](a, http.StatusOK, "GET", hookURL(a, hook.ID, ""), nil)
	if h.ConsecutiveFailures != 1 || !h.Active || h.LastStatus != 503 {
		t.Fatalf("hook %+v", h)
	}
	// the failing webhook is not notified about itself (no loop)
	for _, d := range deliveries(a, hook.ID) {
		if d.Event == "webhook.failed" {
			t.Fatalf("self notification %+v", d)
		}
	}
	_ = other

	// 20th consecutive failure disables it
	ts.db.Model(&model.Webhook{}).Where("id = ?", hook.ID).Update("consecutive_failures", webhooks.MaxConsecutiveFailures-1)
	newSubject(a, api.SubjectCreate{Name: "Druhý"})
	for i := 0; i <= len(webhooks.RetryDelays); i++ {
		mustRunJob(ts, "webhooks")
		ts.now = ts.now.Add(13 * time.Hour)
	}
	h = doJSON[api.Webhook](a, http.StatusOK, "GET", hookURL(a, hook.ID, ""), nil)
	if h.Active || h.DisabledAt == nil || h.ConsecutiveFailures != webhooks.MaxConsecutiveFailures {
		t.Fatalf("not disabled: %+v", h)
	}
	if n := listEvents(a, "?name=webhook.disabled").Total; n != 1 {
		t.Fatalf("webhook.disabled events %d", n)
	}
	// no new deliveries for a disabled webhook; re-enabling resets the counter
	newSubject(a, api.SubjectCreate{Name: "Třetí"})
	pending := 0
	for _, d := range deliveries(a, hook.ID) {
		if d.Status == "pending" {
			pending++
		}
	}
	if pending != 0 {
		t.Fatalf("%d pending deliveries for a disabled webhook", pending)
	}
	h = doJSON[api.Webhook](a, http.StatusOK, "PATCH", hookURL(a, hook.ID, ""), map[string]any{"active": true})
	if !h.Active || h.ConsecutiveFailures != 0 || h.DisabledAt != nil {
		t.Fatalf("re-enabled %+v", h)
	}
}

func TestWebhookSSRF(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.WebhooksAllowPrivate = false })
	a := ts.signup("a@example.cz", "Firma A")
	for _, u := range []string{"http://127.0.0.1:9/x", "http://10.0.0.1/", "http://169.254.169.254/latest/meta-data", "http://[::1]/", "ftp://example.com"} {
		res, body := a.do("POST", a.acct("/webhooks"), api.WebhookCreate{URL: u})
		assertError(t, res, body, http.StatusUnprocessableEntity, "url")
	}
	// a record created while private targets were allowed is still refused at connect time
	rcv := newHookReceiver(t)
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	enc, _ := ts.secrets.Encrypt("whsec_x")
	hook := model.Webhook{AccountID: acc.ID, URL: rcv.URL, Events: []string{"*"}, Secret: enc, Active: true}
	ts.db.Create(&hook)
	ping := doJSON[api.WebhookTestResult](a, http.StatusOK, "POST", hookURL(a, hook.ID, "/test"), nil)
	if ping.OK || len(rcv.received()) != 0 {
		t.Fatalf("private target called: %+v", ping)
	}
}

func boolPtr(v bool) *bool { return &v }
