package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

func doSearch(c *client, q string) api.SearchResults {
	c.ts.t.Helper()
	return doJSON[api.SearchResults](c, http.StatusOK, "GET", c.acct("/search?q="+url.QueryEscape(q)), nil)
}

func TestSearch(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "Žluťoučký kůň s.r.o.", RegistrationNo: "25596641", Email: "kun@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 1000_00, nil)}})
	newPriceItem(a, api.PriceItemCreate{Name: "Šroubek", SKU: "SRB-10"})
	newSubject(b, api.SubjectCreate{Name: "Žluťoučký kůň B"})

	r := doSearch(a, "ZLUTOUCKY")
	if len(r.Subjects) != 1 || r.Subjects[0].ID != subj.ID || r.Subjects[0].URLHint == "" || len(r.Invoices) != 1 {
		t.Fatalf("diacritics/case: %+v", r)
	}
	if r := doSearch(a, inv.Number); len(r.Invoices) != 1 || r.Invoices[0].ID != inv.ID || r.Invoices[0].Status != "open" {
		t.Fatalf("by number: %+v", r)
	}
	if r := doSearch(a, "25596641"); len(r.Subjects) != 1 || len(r.Invoices) != 1 {
		t.Fatalf("by IČO: %+v", r)
	}
	if r := doSearch(a, "srb-10"); len(r.PriceItems) != 1 {
		t.Fatalf("by SKU: %+v", r)
	}
	if r := doSearch(a, "%"); len(r.Subjects)+len(r.Invoices)+len(r.PriceItems) != 0 {
		t.Fatalf("wildcard not escaped: %+v", r)
	}
	// tenant isolation
	if r := doSearch(b, "zlutoucky"); len(r.Subjects) != 1 || r.Subjects[0].ID == subj.ID || len(r.Invoices) != 0 {
		t.Fatalf("b: %+v", r)
	}
	res, body := a.do("GET", a.acct("/search"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "q")

	// Migrate backfills rows without search_text
	ts.db.Model(&model.Subject{}).Where("id = ?", subj.ID).UpdateColumn("search_text", "")
	if err := db.Migrate(ts.db); err != nil {
		t.Fatal(err)
	}
	if r := doSearch(a, "kun@example"); len(r.Subjects) != 1 {
		t.Fatalf("after backfill: %+v", r)
	}
}
