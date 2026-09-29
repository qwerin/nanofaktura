package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

func listEvents(c *client, query string) api.ListResponse[api.Event] {
	c.ts.t.Helper()
	return doJSON[api.ListResponse[api.Event]](c, http.StatusOK, "GET", c.acct("/events")+query, nil)
}

func eventNames(l api.ListResponse[api.Event]) []string {
	out := make([]string, len(l.Items))
	for i, e := range l.Items {
		out[i] = e.Name
	}
	return out
}

func TestEvents(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 1000_00, nil)}})
	action(a, inv.ID, "mark_as_sent")
	pay(a, inv.ID, api.PaymentCreate{})

	all := listEvents(a, "")
	want := []string{"invoice.paid", "payment.created", "invoice.sent", "invoice.created", "subject.created"}
	if got := eventNames(all); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events %v, want %v (newest first)", got, want)
	}
	sent := all.Items[2]
	if sent.Text != "Faktura "+inv.Number+" byla označena jako odeslaná" || sent.SubjectType != "invoice" ||
		sent.SubjectID != inv.ID || sent.UserID == nil || sent.UserName == "" || sent.Data["number"] != inv.Number {
		t.Fatalf("sent event %+v", sent)
	}

	// filters
	if l := listEvents(a, "?name=invoice"); l.Total != 3 {
		t.Fatalf("prefix filter: %v", eventNames(l))
	}
	if l := listEvents(a, "?name=invoice.paid"); l.Total != 1 {
		t.Fatalf("exact filter: %v", eventNames(l))
	}
	if l := listEvents(a, fmt.Sprintf("?subject_type=invoice&subject_id=%d", inv.ID)); l.Total != 4 {
		t.Fatalf("subject filter: %v", eventNames(l))
	}
	if l := listEvents(a, "?since=2026-03-16"); l.Total != 0 {
		t.Fatalf("since filter: %v", eventNames(l))
	}
	res, body := a.do("GET", a.acct("/events?subject_id=1"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "subject_type")
	if cat := doJSON[[]api.EventCatalogEntry](a, http.StatusOK, "GET", a.acct("/events/catalog"), nil); len(cat) < 30 {
		t.Fatalf("catalog %d", len(cat))
	}

	// tenant isolation
	if l := listEvents(b, ""); l.Total != 0 {
		t.Fatalf("b sees %v", eventNames(l))
	}
}

// TestEventRolledBackWithTransaction: an event recorded inside a transaction
// that fails later disappears with it.
func TestEventRolledBackWithTransaction(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	pro := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: subj.ID,
		Lines: []api.InvoiceLineInput{line("Záloha", "1", 500_00, nil)}})
	pay(a, pro.ID, api.PaymentCreate{Amount: ptr64(100_00), CreateFinalInvoice: true})
	before := listEvents(a, "?name=payment.created").Total

	// addPayment records payment.created, then the duplicate final invoice fails → rollback
	res, body := a.do("POST", invURL(a, pro.ID, "/payments"), api.PaymentCreate{Amount: ptr64(100_00), CreateFinalInvoice: true})
	assertError(t, res, body, http.StatusConflict, "final invoice")
	if after := listEvents(a, "?name=payment.created").Total; after != before {
		t.Fatalf("payment.created events %d → %d after rollback", before, after)
	}
	var n int64
	ts.db.Model(&model.Event{}).Where("name = ?", "invoice.created").Count(&n)
	if n != 2 { // proforma + final invoice
		t.Fatalf("invoice.created events %d", n)
	}
}

func TestEventsAtMutationPoints(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "acme@example.cz"})
	a.mustDo(http.StatusOK, "PATCH", fmt.Sprintf("%s/%d", a.acct("/subjects"), subj.ID), map[string]any{"city": "Brno"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 1000_00, nil)}})
	a.mustDo(http.StatusOK, "PATCH", invURL(a, inv.ID, ""), map[string]any{"note": "x"})
	a.mustDo(http.StatusOK, "POST", invURL(a, inv.ID, "/send"), map[string]any{})
	a.mustDo(http.StatusOK, "POST", invURL(a, inv.ID, "/regenerate-public-token"), nil)
	cur := getInv(a, inv.ID)
	ts.anon().mustDo(http.StatusOK, "GET", "/api/public/invoices/"+cur.PublicToken, nil)
	ts.anon().mustDo(http.StatusOK, "GET", "/api/public/invoices/"+cur.PublicToken, nil) // second view: no event
	action(a, inv.ID, "cancel")
	dup := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/duplicate"), nil)
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, dup.ID, ""), nil)

	counts := map[string]int{}
	for _, e := range listEvents(a, "?per_page=200").Items {
		counts[e.Name]++
		if e.Name == "public.viewed" && e.UserID != nil {
			t.Fatalf("public.viewed has a user: %+v", e)
		}
	}
	for name, want := range map[string]int{
		"subject.created": 1, "subject.updated": 1, "invoice.created": 2, "invoice.updated": 1, "email.sent": 1,
		"invoice.sent": 1, "invoice.public_link_regenerated": 1, "public.viewed": 1, "invoice.cancelled": 1, "invoice.deleted": 1,
	} {
		if counts[name] != want {
			t.Errorf("%s: %d events, want %d (all: %v)", name, counts[name], want, counts)
		}
	}
}

func ptr64(v int64) *int64 { return &v }
