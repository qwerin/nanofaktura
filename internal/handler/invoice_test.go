package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/models"
)

const minimalInvoice = `{
  "number": "2024-001",
  "issued_on": "2024-03-28",
  "taxable_fulfillment_due": "2024-03-28",
  "client_name": "Testovací s.r.o.",
  "lines": [
    {"name": "Vývoj", "quantity": "8", "unit_name": "hod", "unit_price_hal": 150000}
  ]
}`

func TestInvoice_Create(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/invoices", minimalInvoice)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var inv models.Invoice
	decodeBody(t, w, &inv)

	if inv.Number != "2024-001" {
		t.Errorf("number = %q, want 2024-001", inv.Number)
	}
	// Recalculate: 8 hod × 1 500 Kč = 12 000 Kč = 1 200 000 haléřů
	if inv.Total != 1_200_000 {
		t.Errorf("total = %d, want 1_200_000", inv.Total)
	}
	if inv.Status != models.StatusOpen {
		t.Errorf("status = %q, want open", inv.Status)
	}
}

func TestInvoice_Create_MissingRequired(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/invoices", `{"issued_on":"2024-03-28","taxable_fulfillment_due":"2024-03-28","lines":[]}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
}

func TestInvoice_Create_DuplicateNumber(t *testing.T) {
	api := newTestAPIChi(t)
	do(t, api, "POST", "/invoices", minimalInvoice)

	w := do(t, api, "POST", "/invoices", minimalInvoice)
	if w.Code == http.StatusCreated {
		t.Error("expected error for duplicate number, got 201")
	}
}

func TestInvoice_List(t *testing.T) {
	api := newTestAPIChi(t)
	do(t, api, "POST", "/invoices", minimalInvoice)
	do(t, api, "POST", "/invoices", strings.Replace(minimalInvoice, "2024-001", "2024-002", 1))

	w := do(t, api, "GET", "/invoices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var invoices []models.Invoice
	decodeBody(t, w, &invoices)
	if len(invoices) != 2 {
		t.Errorf("len = %d, want 2", len(invoices))
	}
}

func TestInvoice_Get(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/invoices", minimalInvoice)
	var created models.Invoice
	decodeBody(t, w, &created)

	w2 := do(t, api, "GET", "/invoices/"+idStr(created.ID), "")
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w2.Code, w2.Body.String())
	}
	var fetched models.Invoice
	decodeBody(t, w2, &fetched)
	if fetched.Number != "2024-001" {
		t.Errorf("number = %q, want 2024-001", fetched.Number)
	}
	if len(fetched.Lines) != 1 {
		t.Errorf("lines len = %d, want 1", len(fetched.Lines))
	}
}

func TestInvoice_Get_NotFound(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "GET", "/invoices/9999", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestInvoice_PatchStatus(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/invoices", minimalInvoice)
	var created models.Invoice
	decodeBody(t, w, &created)

	w2 := do(t, api, "PATCH", "/invoices/"+idStr(created.ID)+"/status", `{"status":"paid"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("patch status = %d; body: %s", w2.Code, w2.Body.String())
	}
	var patched models.Invoice
	decodeBody(t, w2, &patched)
	if patched.Status != models.StatusPaid {
		t.Errorf("status = %q, want paid", patched.Status)
	}
}

func TestInvoice_Recalculate_VatLines(t *testing.T) {
	api := newTestAPIChi(t)
	body := `{
		"number": "VAT-001",
		"issued_on": "2024-03-28",
		"taxable_fulfillment_due": "2024-03-28",
		"client_name": "Plátce DPH s.r.o.",
		"vat_exempt": false,
		"lines": [
			{"name": "Produkt", "quantity": "1", "unit_name": "ks", "unit_price_hal": 100000, "vat_rate_bps": 2100}
		]
	}`
	w := do(t, api, "POST", "/invoices", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var inv models.Invoice
	decodeBody(t, w, &inv)

	// základ 1 000 Kč, DPH 21 % = 210 Kč, celkem 1 210 Kč
	if inv.Subtotal != 100_000 {
		t.Errorf("subtotal = %d, want 100_000", inv.Subtotal)
	}
	if inv.TotalVatHal != 21_000 {
		t.Errorf("total_vat_hal = %d, want 21_000", inv.TotalVatHal)
	}
	if inv.Total != 121_000 {
		t.Errorf("total = %d, want 121_000", inv.Total)
	}
}

func TestInvoice_PDF(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/invoices", minimalInvoice)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", w.Code, w.Body.String())
	}
	var created models.Invoice
	decodeBody(t, w, &created)

	wp := do(t, api, "GET", "/invoices/"+idStr(created.ID)+"/pdf", "")
	if wp.Code != http.StatusOK {
		t.Fatalf("pdf status = %d; body: %s", wp.Code, wp.Body.String())
	}
	if ct := wp.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	if wp.Body.Len() == 0 {
		t.Error("pdf body is empty")
	}
}

func TestInvoice_PDF_NotFound(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "GET", "/invoices/9999/pdf", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestInvoice_Update(t *testing.T) {
	api := newTestAPIChi(t)

	// Vytvoř fakturu
	w := do(t, api, "POST", "/invoices", minimalInvoice)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", w.Code, w.Body.String())
	}
	var created models.Invoice
	decodeBody(t, w, &created)

	// Uprav: změň klienta, přidej řádek
	updated := `{
		"number": "2024-001",
		"issued_on": "2024-03-28",
		"taxable_fulfillment_due": "2024-03-28",
		"client_name": "Nový klient s.r.o.",
		"lines": [
			{"name": "Vývoj", "quantity": "10", "unit_name": "hod", "unit_price_hal": 150000},
			{"name": "Konzultace", "quantity": "2", "unit_name": "hod", "unit_price_hal": 200000}
		]
	}`
	w2 := do(t, api, "PUT", "/invoices/"+idStr(created.ID), updated)
	if w2.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", w2.Code, w2.Body.String())
	}

	var inv models.Invoice
	decodeBody(t, w2, &inv)

	if inv.ClientName != "Nový klient s.r.o." {
		t.Errorf("client_name = %q, want 'Nový klient s.r.o.'", inv.ClientName)
	}
	if len(inv.Lines) != 2 {
		t.Errorf("lines len = %d, want 2", len(inv.Lines))
	}
	// 10 × 1500 + 2 × 2000 = 15000 + 4000 = 19000 Kč = 1 900 000 hal
	if inv.Total != 1_900_000 {
		t.Errorf("total = %d, want 1_900_000", inv.Total)
	}
	if inv.Status != models.StatusOpen {
		t.Errorf("status = %q, want open (preserved)", inv.Status)
	}
}

func TestInvoice_Update_ReduceLines(t *testing.T) {
	api := newTestAPIChi(t)

	w := do(t, api, "POST", "/invoices", minimalInvoice)
	var created models.Invoice
	decodeBody(t, w, &created)

	// Uprav na 0 řádků
	w2 := do(t, api, "PUT", "/invoices/"+idStr(created.ID), `{
		"number": "2024-001",
		"issued_on": "2024-03-28",
		"taxable_fulfillment_due": "2024-03-28",
		"client_name": "Testovací s.r.o.",
		"lines": []
	}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", w2.Code, w2.Body.String())
	}
	var inv models.Invoice
	decodeBody(t, w2, &inv)
	if len(inv.Lines) != 0 {
		t.Errorf("lines len = %d, want 0", len(inv.Lines))
	}
	if inv.Total != 0 {
		t.Errorf("total = %d, want 0", inv.Total)
	}
}

func TestInvoice_Update_NotFound(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "PUT", "/invoices/9999", minimalInvoice)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
