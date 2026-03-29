package handler_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/models"
)

func TestSettings_GetDefault(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "GET", "/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var s models.Settings
	decodeBody(t, w, &s)
	if s.CompanyCountry != "CZ" {
		t.Errorf("company_country = %q, want CZ", s.CompanyCountry)
	}
}

func TestSettings_Update(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "PUT", "/settings", `{"company_name": "Moje firma s.r.o."}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var s models.Settings
	decodeBody(t, w, &s)
	if s.CompanyName != "Moje firma s.r.o." {
		t.Errorf("company_name = %q, want Moje firma s.r.o.", s.CompanyName)
	}
}

func TestSettings_NumberFormat_List(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "GET", "/settings/number-formats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var formats []models.NumberFormat
	decodeBody(t, w, &formats)
	if len(formats) < 2 {
		t.Errorf("len(formats) = %d, want >= 2 (defaults)", len(formats))
	}
}

func TestSettings_NumberFormat_Next(t *testing.T) {
	api := newTestAPIChi(t)

	wl := do(t, api, "GET", "/settings/number-formats", "")
	if wl.Code != http.StatusOK {
		t.Fatalf("list status = %d; body: %s", wl.Code, wl.Body.String())
	}
	var formats []models.NumberFormat
	decodeBody(t, wl, &formats)
	if len(formats) == 0 {
		t.Fatal("no number formats found")
	}
	id := formats[0].ID

	wn := do(t, api, "POST", fmt.Sprintf("/settings/number-formats/%d/next", id), "")
	if wn.Code != http.StatusOK {
		t.Fatalf("next status = %d; body: %s", wn.Code, wn.Body.String())
	}
	var result struct {
		Number string `json:"number"`
	}
	decodeBody(t, wn, &result)
	if result.Number == "" {
		t.Error("expected non-empty number")
	}

	wn2 := do(t, api, "POST", fmt.Sprintf("/settings/number-formats/%d/next", id), "")
	if wn2.Code != http.StatusOK {
		t.Fatalf("next2 status = %d; body: %s", wn2.Code, wn2.Body.String())
	}
	var result2 struct {
		Number string `json:"number"`
	}
	decodeBody(t, wn2, &result2)
	if result2.Number == result.Number {
		t.Errorf("second number %q should differ from first %q", result2.Number, result.Number)
	}
}

func TestSettings_GetDefault_HasTwoFormats(t *testing.T) {
	api := newTestAPIChi(t)
	var s models.Settings
	decodeBody(t, do(t, api, "GET", "/settings", ""), &s)

	if len(s.NumberFormats) < 2 {
		t.Fatalf("number_formats = %d, want >= 2", len(s.NumberFormats))
	}
	types := make(map[string]bool)
	for _, f := range s.NumberFormats {
		types[f.DocumentType] = true
	}
	if !types["invoice"] {
		t.Error("missing default 'invoice' format")
	}
	if !types["proforma"] {
		t.Error("missing default 'proforma' format")
	}
}

func TestSettings_Update_MultipleFields(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "PUT", "/settings",
		`{"company_name":"OSVČ Jan Novák","registration_no":"12345678","bank_account":"123456789/0800"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var s models.Settings
	decodeBody(t, w, &s)
	if s.CompanyName != "OSVČ Jan Novák" {
		t.Errorf("company_name = %q", s.CompanyName)
	}
	if s.RegistrationNo != "12345678" {
		t.Errorf("registration_no = %q, want 12345678", s.RegistrationNo)
	}
	if s.BankAccount != "123456789/0800" {
		t.Errorf("bank_account = %q, want 123456789/0800", s.BankAccount)
	}
}

func TestSettings_Update_PartialDoesNotZero(t *testing.T) {
	api := newTestAPIChi(t)
	do(t, api, "PUT", "/settings", `{"company_name":"Firma A","registration_no":"11111111"}`)
	do(t, api, "PUT", "/settings", `{"company_name":"Firma B"}`)

	var s models.Settings
	decodeBody(t, do(t, api, "GET", "/settings", ""), &s)
	if s.RegistrationNo != "11111111" {
		t.Errorf("registration_no = %q, want 11111111 (nesmí být přepsáno)", s.RegistrationNo)
	}
	if s.CompanyName != "Firma B" {
		t.Errorf("company_name = %q, want Firma B", s.CompanyName)
	}
}

func TestSettings_NumberFormat_Next_Sequential(t *testing.T) {
	api := newTestAPIChi(t)

	var formats []models.NumberFormat
	decodeBody(t, do(t, api, "GET", "/settings/number-formats", ""), &formats)
	invoiceFormat := formats[0]
	for _, f := range formats {
		if f.DocumentType == "invoice" {
			invoiceFormat = f
			break
		}
	}

	year := fmt.Sprintf("%d", time.Now().Year())
	nums := make([]string, 3)
	for i := range nums {
		var r struct {
			Number string `json:"number"`
		}
		decodeBody(t, do(t, api, "POST", fmt.Sprintf("/settings/number-formats/%d/next", invoiceFormat.ID), ""), &r)
		nums[i] = r.Number
	}

	if nums[0] == nums[1] || nums[1] == nums[2] {
		t.Errorf("duplicitní čísla: %v", nums)
	}
	for _, n := range nums {
		if !strings.Contains(n, year) {
			t.Errorf("číslo %q neobsahuje rok %s", n, year)
		}
	}
}

func TestSettings_NumberFormat_Create(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/settings/number-formats",
		`{"document_type":"correction","label":"Dobropisy","pattern":"DD{YYYY}{NNN}","next_number":1,"padding_width":4}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var f models.NumberFormat
	decodeBody(t, w, &f)
	if f.Pattern != "DD{YYYY}{NNN}" {
		t.Errorf("pattern = %q, want DD{YYYY}{NNN}", f.Pattern)
	}
	if f.PaddingWidth != 4 {
		t.Errorf("padding_width = %d, want 4", f.PaddingWidth)
	}
	var r struct {
		Number string `json:"number"`
	}
	decodeBody(t, do(t, api, "POST", fmt.Sprintf("/settings/number-formats/%d/next", f.ID), ""), &r)
	year := fmt.Sprintf("%d", time.Now().Year())
	expected := "DD" + year + "0001"
	if r.Number != expected {
		t.Errorf("generated = %q, want %q", r.Number, expected)
	}
}
