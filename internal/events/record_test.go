package events

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	return gdb
}

func TestRecordQueuesMatchingWebhooks(t *testing.T) {
	gdb := openDB(t)
	at := time.Date(2026, 3, 1, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	hooks := []model.Webhook{
		{AccountID: 1, URL: "https://a", Events: []string{"*"}, Secret: "s", Active: true},
		{AccountID: 1, URL: "https://b", Events: []string{"invoice.*"}, Secret: "s", Active: true},
		{AccountID: 1, URL: "https://c", Events: []string{"expense.created"}, Secret: "s", Active: true},
		{AccountID: 1, URL: "https://d", Events: []string{"*"}, Secret: "s", Active: false},
		{AccountID: 2, URL: "https://e", Events: []string{"*"}, Secret: "s", Active: true},
	}
	for i := range hooks {
		if err := gdb.Create(&hooks[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	// inactive hooks are stored with Active=false explicitly (gorm skips zero values on create)
	gdb.Model(&hooks[3]).Update("active", false)

	uid := uint(9)
	meta := Meta{AccountID: 1, AccountSlug: "firma", UserID: &uid, At: at}
	ev, err := Record(gdb, meta, Event{Name: InvoicePaid, SubjectType: SubjectInvoice, SubjectID: 5, Text: "Uhrazeno"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == 0 || ev.AccountID != 1 || *ev.UserID != 9 || !ev.CreatedAt.Equal(at) || ev.Data == nil {
		t.Fatalf("event %+v", ev)
	}
	var ds []model.WebhookDelivery
	gdb.Order("webhook_id").Find(&ds)
	if len(ds) != 2 || ds[0].WebhookID != hooks[0].ID || ds[1].WebhookID != hooks[1].ID {
		t.Fatalf("deliveries %+v", ds)
	}
	d := ds[0]
	if d.Status != model.DeliveryPending || d.EventName != InvoicePaid || *d.EventID != ev.ID || !d.NextAttemptAt.Equal(at) || d.AccountID != 1 {
		t.Fatalf("delivery %+v", d)
	}
	if ds[0].Payload != ds[1].Payload {
		t.Fatal("payload must be identical for every webhook")
	}
	var body PayloadBody
	if err := json.Unmarshal([]byte(d.Payload), &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != ev.ID || body.Event != InvoicePaid || body.Account != "firma" || body.Subject != (PayloadSubject{SubjectInvoice, 5}) ||
		body.Text != "Uhrazeno" || body.CreatedAt.Location() != time.UTC || !body.CreatedAt.Equal(at) || body.Data == nil {
		t.Fatalf("payload %+v", body)
	}
	if !strings.Contains(d.Payload, `"data":{}`) {
		t.Fatalf("empty data must be an object: %s", d.Payload)
	}
}

func TestRecordSkipsWebhookEventsAboutItself(t *testing.T) {
	gdb := openDB(t)
	h1 := model.Webhook{AccountID: 1, URL: "https://a", Events: []string{"webhook.*"}, Secret: "s", Active: true}
	h2 := model.Webhook{AccountID: 1, URL: "https://b", Events: []string{"*"}, Secret: "s", Active: true}
	gdb.Create(&h1)
	gdb.Create(&h2)
	if _, err := Record(gdb, Meta{AccountID: 1, At: time.Now()}, Event{Name: WebhookFailed, SubjectType: SubjectWebhook, SubjectID: h1.ID}); err != nil {
		t.Fatal(err)
	}
	var ds []model.WebhookDelivery
	gdb.Find(&ds)
	if len(ds) != 1 || ds[0].WebhookID != h2.ID {
		t.Fatalf("webhook must not get events about itself: %+v", ds)
	}
}

func TestRecordRollbackAndErrors(t *testing.T) {
	gdb := openDB(t)
	gdb.Create(&model.Webhook{AccountID: 1, URL: "https://a", Events: []string{"*"}, Secret: "s", Active: true})
	boom := errors.New("boom")
	err := gdb.Transaction(func(tx *gorm.DB) error {
		if _, err := Record(tx, Meta{AccountID: 1, At: time.Now()}, Event{Name: SubjectCreated, SubjectType: SubjectSubject, SubjectID: 1}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var n, nd int64
	gdb.Model(&model.Event{}).Count(&n)
	gdb.Model(&model.WebhookDelivery{}).Count(&nd)
	if n != 0 || nd != 0 {
		t.Fatalf("rollback left %d events, %d deliveries", n, nd)
	}

	// a failing insert is reported with the event name
	sqlDB, _ := gdb.DB()
	sqlDB.Close()
	if _, err := Record(gdb, Meta{AccountID: 1}, Event{Name: SubjectCreated}); err == nil || !strings.Contains(err.Error(), "record event subject.created") {
		t.Fatalf("closed DB: %v", err)
	}
}

func TestPayloadNilData(t *testing.T) {
	b, err := Payload(&model.Event{ID: 1, Name: InvoiceCreated}, "x")
	if err != nil || !strings.Contains(string(b), `"data":{}`) || !strings.Contains(string(b), `"account":"x"`) {
		t.Fatalf("%s %v", b, err)
	}
}

func TestCatalogUniqueAndValid(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Catalog {
		if seen[c.Name] || c.Description == "" || !strings.Contains(c.Name, ".") {
			t.Errorf("bad catalog entry %+v", c)
		}
		seen[c.Name] = true
		if !ValidPattern(c.Name) || !ValidPattern(strings.SplitN(c.Name, ".", 2)[0]+".*") {
			t.Errorf("catalog name %s not a valid pattern", c.Name)
		}
	}
}
