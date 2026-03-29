package handler_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/models"
)

const minimalSubject = `{"name": "ACME s.r.o.", "registration_no": "12345678"}`

func TestSubject_Create(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/subjects", minimalSubject)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var s models.Subject
	decodeBody(t, w, &s)
	if s.Name != "ACME s.r.o." {
		t.Errorf("name = %q, want ACME s.r.o.", s.Name)
	}
}

func TestSubject_List(t *testing.T) {
	api := newTestAPIChi(t)
	do(t, api, "POST", "/subjects", minimalSubject)
	do(t, api, "POST", "/subjects", `{"name": "Beta Corp"}`)

	w := do(t, api, "GET", "/subjects", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var subjects []models.Subject
	decodeBody(t, w, &subjects)
	if len(subjects) != 2 {
		t.Errorf("len = %d, want 2", len(subjects))
	}
}

func TestSubject_InvoiceLinked(t *testing.T) {
	api := newTestAPIChi(t)

	ws := do(t, api, "POST", "/subjects", minimalSubject)
	var subj models.Subject
	decodeBody(t, ws, &subj)

	invBody := fmt.Sprintf(`{
		"number": "LINK-001",
		"issued_on": "2024-03-28",
		"taxable_fulfillment_due": "2024-03-28",
		"client_name": "Testovací s.r.o.",
		"subject_id": %d,
		"lines": [{"name": "Vývoj", "quantity": "1", "unit_name": "hod", "unit_price_hal": 100000}]
	}`, subj.ID)

	wi := do(t, api, "POST", "/invoices", invBody)
	if wi.Code != http.StatusCreated {
		t.Fatalf("invoice create status = %d; body: %s", wi.Code, wi.Body.String())
	}
	var inv models.Invoice
	decodeBody(t, wi, &inv)
	if inv.SubjectID != subj.ID {
		t.Errorf("subject_id = %d, want %d", inv.SubjectID, subj.ID)
	}
}
