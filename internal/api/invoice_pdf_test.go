package api_test

import (
	"bytes"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestInvoicePDF(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	acme := newSubject(a, api.SubjectCreate{Name: "ACME s.r.o.", City: "Praha"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: acme.ID, Lines: []api.InvoiceLineInput{
		line("Práce", "2", 150000, nil),
	}})

	res, body := a.do("GET", invURL(a, inv.ID, "/pdf"), nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content-type %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `inline; filename="faktura-2026-0001.pdf"` {
		t.Fatalf("content-disposition %q", cd)
	}
	if !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", body[:min(len(body), 20)])
	}
	if text := pdfText(t, body); text != "" {
		for _, want := range []string{"2026-0001", "ACME s.r.o.", "QR Platba", "Faktura"} {
			if !strings.Contains(text, want) {
				t.Errorf("pdf text lacks %q", want)
			}
		}
	}

	// overrides
	res, body = a.do("GET", invURL(a, inv.ID, "/pdf?template=modern&lang=en"), nil)
	if res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("modern/en: %d", res.StatusCode)
	}
	if text := pdfText(t, body); text != "" && !strings.Contains(text, "Invoice") {
		t.Errorf("english pdf lacks title: %s", text)
	}
	res, body = a.do("GET", invURL(a, inv.ID, "/pdf?template=fancy"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "template")

	// correction refers to the original number
	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv.ID, "/correction"), nil)
	res, body = a.do("GET", invURL(a, corr.ID, "/pdf"), nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("correction pdf: %d %s", res.StatusCode, body)
	}
	if text := pdfText(t, body); text != "" && !strings.Contains(text, "2026-0001") {
		t.Errorf("correction pdf lacks the original number")
	}

	// tenant isolation, missing record, no auth
	res, body = b.do("GET", invURL(b, inv.ID, "/pdf"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
	res, body = a.do("GET", invURL(a, 9999, "/pdf"), nil)
	assertError(t, res, body, http.StatusNotFound, "invoice not found")
	res, _ = ts.anon().do("GET", invURL(a, inv.ID, "/pdf"), nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon: %d", res.StatusCode)
	}
}

// pdfText extracts text with poppler's pdftotext, or returns "" when it is
// not installed (the text checks are then skipped).
func pdfText(t *testing.T, pdf []byte) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return ""
	}
	cmd := exec.Command("pdftotext", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}
