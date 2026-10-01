package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestPublicInvoice(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	acme := newSubject(a, api.SubjectCreate{Name: "ACME s.r.o.", City: "Praha", Email: "acme@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, PrivateNote: "TAJNA-POZNAMKA", Tags: []string{"TAJNY-STITEK"},
		Lines: []api.InvoiceLineInput{line("Práce", "2", 150000, nil)}})
	if inv.PublicViewedAt != nil {
		t.Fatal("new invoice already viewed")
	}
	anon := ts.anon()
	url := "/api/public/invoices/" + inv.PublicToken

	res, body := anon.do("GET", url, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("public get: %d %s", res.StatusCode, body)
	}
	pub := decodeJSON[api.PublicInvoice](t, body)
	if pub.Number != inv.Number || pub.Customer.Name != "ACME s.r.o." || pub.Supplier.Name != "Firma A" ||
		pub.Total != 300000 || pub.RemainingAmount != 300000 || len(pub.Lines) != 1 || pub.Lines[0].Quantity != "2" ||
		pub.Cancelled || pub.Status != "open" || pub.IBAN == "" {
		t.Fatalf("public invoice %+v", pub)
	}
	if !strings.HasPrefix(pub.Spayd, "SPD*1.0*ACC:CZ") || !strings.Contains(pub.Spayd, "AM:3000.00") {
		t.Fatalf("spayd %q", pub.Spayd)
	}
	// nothing private leaks
	for _, secret := range []string{"TAJNA-POZNAMKA", "TAJNY-STITEK", inv.PublicToken, `"id"`, `"subject_id"`,
		`"private_note"`, `"tags"`, `"account_id"`, `"bank_account_id"`, "acme@example.cz"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Errorf("public body leaks %s: %s", secret, body)
		}
	}
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	for _, k := range []string{"id", "public_token", "private_note", "tags", "subject_id", "related_id", "locked_at", "created_at"} {
		if _, ok := raw[k]; ok {
			t.Errorf("public output has %s", k)
		}
	}

	// the first view is recorded once
	viewed := getInv(a, inv.ID).PublicViewedAt
	if viewed == nil || !viewed.Equal(ts.now) {
		t.Fatalf("public_viewed_at %v", viewed)
	}
	ts.now = ts.now.Add(time.Hour)
	anon.mustDo(http.StatusOK, "GET", url, nil)
	if v := getInv(a, inv.ID).PublicViewedAt; !v.Equal(*viewed) {
		t.Fatalf("second view changed public_viewed_at: %v", v)
	}

	// PDF and ISDOC
	res, body = anon.do("GET", url+"/pdf", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("public pdf: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if text := pdfText(t, body); text != "" && !strings.Contains(text, inv.Number) {
		t.Errorf("public pdf lacks the number")
	}
	res, body = anon.do("GET", url+"/isdoc", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/xml" ||
		!bytes.Contains(body, []byte("<ID>"+inv.Number+"</ID>")) ||
		res.Header.Get("Content-Disposition") != `attachment; filename="faktura-2026-0001.isdoc"` {
		t.Fatalf("public isdoc: %d %v %s", res.StatusCode, res.Header, body)
	}

	// cancelled: still viewable, flagged, no QR payment
	action(a, inv.ID, "cancel")
	pub = doJSON[api.PublicInvoice](anon, http.StatusOK, "GET", url, nil)
	if !pub.Cancelled || pub.Status != "cancelled" || pub.Spayd != "" {
		t.Fatalf("cancelled %+v", pub)
	}

	// unknown / malformed / regenerated tokens → 404
	for _, bad := range []string{"x", strings.Repeat("a", 32), strings.Repeat("a", 80)} {
		res, body = anon.do("GET", "/api/public/invoices/"+bad, nil)
		assertError(t, res, body, http.StatusNotFound, "invoice not found")
		res, body = anon.do("GET", "/api/public/invoices/"+bad+"/pdf", nil)
		assertError(t, res, body, http.StatusNotFound, "invoice not found")
	}
	regen := doJSON[api.Invoice](a, http.StatusOK, "POST", invURL(a, inv.ID, "/regenerate-public-token"), nil)
	res, body = anon.do("GET", url+"/isdoc", nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
	anon.mustDo(http.StatusOK, "GET", "/api/public/invoices/"+regen.PublicToken, nil)
}

func TestInvoiceISDOC(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	setVatPayer(a)
	acme := newSubject(a, api.SubjectCreate{Name: "ACME", VatNo: "CZ27074358"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 100000, i32(2100))}})

	res, body := a.do("GET", invURL(a, inv.ID, "/isdoc"), nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/xml" {
		t.Fatalf("isdoc: %d %s", res.StatusCode, body)
	}
	for _, want := range []string{"<DocumentType>1</DocumentType>", "<TaxPointDate>2026-03-15</TaxPointDate>",
		"<CompanyID>CZ27074358</CompanyID>", "<TaxInclusiveAmount>1210.00</TaxInclusiveAmount>"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("isdoc lacks %s:\n%s", want, body)
		}
	}
	first := body
	_, again := a.do("GET", invURL(a, inv.ID, "/isdoc"), nil)
	if !bytes.Equal(first, again) {
		t.Error("isdoc is not deterministic (UUID must be stable)")
	}

	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	_, body = a.do("GET", invURL(a, corr.ID, "/isdoc"), nil)
	for _, want := range []string{"<DocumentType>2</DocumentType>", "<OriginalDocumentReference id=\"1\">",
		"<ID>" + inv.Number + "</ID>", "<TaxInclusiveAmount>1210.00</TaxInclusiveAmount>"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("correction isdoc lacks %s:\n%s", want, body)
		}
	}

	res, body = b.do("GET", invURL(b, inv.ID, "/isdoc"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
}

func TestSendInvoiceWithISDOC(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "klient@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 100000, nil)}})

	log := doJSON[api.EmailLog](a, http.StatusOK, "POST", invURL(a, inv.ID, "/send"), map[string]any{"attach_isdoc": true})
	if strings.Join(log.Attachments, ",") != "faktura-2026-0001.pdf,faktura-2026-0001.isdoc" {
		t.Fatalf("log attachments %v", log.Attachments)
	}
	m, ok := ts.mail.Last()
	if !ok || len(m.Attachments) != 2 {
		t.Fatalf("sent %+v", m)
	}
	x := m.Attachments[1]
	if x.Filename != "faktura-2026-0001.isdoc" || x.ContentType != "application/xml" ||
		!bytes.Contains(x.Data, []byte("<ID>2026-0001</ID>")) {
		t.Fatalf("isdoc attachment %s %s", x.Filename, x.ContentType)
	}

	// ISDOC only
	doJSON[api.EmailLog](a, http.StatusOK, "POST", invURL(a, inv.ID, "/send"), map[string]any{"attach_pdf": false, "attach_isdoc": true})
	if m, _ = ts.mail.Last(); len(m.Attachments) != 1 || m.Attachments[0].ContentType != "application/xml" {
		t.Fatalf("isdoc only %+v", m.Attachments)
	}
}

func TestPDFPreview(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Moje Firma")
	for _, q := range []string{"", "?template=modern&lang=en", "?template=minimal&document_type=correction"} {
		res, body := a.do("GET", a.acct("/pdf-preview"+q), nil)
		if res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("%PDF")) {
			t.Fatalf("preview %s: %d", q, res.StatusCode)
		}
		if text := pdfText(t, body); text != "" && !strings.Contains(text, "Moje Firma") {
			t.Errorf("preview %s does not show the account", q)
		}
	}
	res, body := a.do("GET", a.acct("/pdf-preview?template=nope"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "template")
	res, _ = ts.anon().do("GET", a.acct("/pdf-preview"), nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon preview %d", res.StatusCode)
	}
}
