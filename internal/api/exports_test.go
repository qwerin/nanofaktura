package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/qwerin/nanofaktura/internal/api"
)

// parseCSV checks the Excel-friendly format (BOM, ";") and returns the records.
func parseCSV(t *testing.T, res *http.Response, body []byte) [][]string {
	t.Helper()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("csv: %d %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content-type %q", ct)
	}
	if !bytes.HasPrefix(body, []byte("\xEF\xBB\xBF")) {
		t.Fatal("csv without BOM")
	}
	r := csv.NewReader(bytes.NewReader(body[3:]))
	r.Comma = ';'
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v\n%s", err, body)
	}
	return recs
}

func csvOf(t *testing.T, c *client, path string) [][]string {
	t.Helper()
	res, body := c.do("GET", path, nil)
	return parseCSV(t, res, body)
}

func xlsxOf(t *testing.T, c *client, path string) *excelize.File {
	t.Helper()
	res, body := c.do("GET", path, nil)
	return openXLSX(t, res, body)
}

func zipOf(t *testing.T, c *client, path string) []string {
	t.Helper()
	res, body := c.do("GET", path, nil)
	return zipNames(t, res, body)
}

func col(t *testing.T, header []string, name string) int {
	t.Helper()
	for i, h := range header {
		if h == name {
			return i
		}
	}
	t.Fatalf("no column %q in %v", name, header)
	return -1
}

func openXLSX(t *testing.T, res *http.Response, body []byte) *excelize.File {
	t.Helper()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/vnd.openxmlformats") {
		t.Fatalf("xlsx: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestExportInvoices(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	acme := newSubject(a, api.SubjectCreate{Name: "ACME; s.r.o.", RegistrationNo: "27074358"}) // ";" must be quoted
	beta := newSubject(a, api.SubjectCreate{Name: "Beta"})
	createInv(a, api.InvoiceCreate{SubjectID: new(acme.ID), Tags: []string{"web", "2026"}, Lines: []api.InvoiceLineInput{line("Práce", "1.5", 121050, nil)}})
	createInv(a, api.InvoiceCreate{SubjectID: new(beta.ID), IssuedOn: "2026-02-01", Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})
	createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: new(acme.ID), Lines: []api.InvoiceLineInput{line("Záloha", "1", 50000, nil)}})
	createInv(b, api.InvoiceCreate{SubjectID: new(newSubject(b, api.SubjectCreate{Name: "Cizí"}).ID), Lines: []api.InvoiceLineInput{line("X", "1", 100, nil)}})

	res, body := a.do("GET", a.acct("/exports/invoices.csv"), nil)
	recs := parseCSV(t, res, body)
	if res.Header.Get("Content-Disposition") != `attachment; filename="invoices.csv"` {
		t.Errorf("disposition %q", res.Header.Get("Content-Disposition"))
	}
	if len(recs) != 4 {
		t.Fatalf("rows %v", recs)
	}
	h := recs[0]
	first := recs[1] // default order -issued_on: 2026-0001 (15. 3.) before Z2026-0001? both 15. 3. → id DESC
	if first[col(t, h, "Číslo")] != "Z2026-0001" || first[col(t, h, "Typ dokladu")] != "zálohová faktura" {
		t.Errorf("first row %v", first)
	}
	row := recs[2]
	if row[col(t, h, "Odběratel")] != "ACME; s.r.o." || row[col(t, h, "Celkem")] != "1815,75" ||
		row[col(t, h, "Vystaveno")] != "2026-03-15" || row[col(t, h, "Stav")] != "vystavená" ||
		row[col(t, h, "Štítky")] != "web, 2026" || row[col(t, h, "Kurz")] != "1" {
		t.Errorf("row %v", row)
	}

	// same filters as the list
	recs = csvOf(t, a, a.acct("/exports/invoices.csv?document_type=invoice&since=2026-03-01"))
	if len(recs) != 2 || recs[1][0] != "2026-0001" {
		t.Errorf("filtered %v", recs)
	}
	res, body = a.do("GET", a.acct("/exports/invoices.csv?status=bogus"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "status")

	// tenant isolation
	recs = csvOf(t, b, b.acct("/exports/invoices.csv"))
	if len(recs) != 2 || recs[1][col(t, recs[0], "Odběratel")] != "Cizí" {
		t.Errorf("b export %v", recs)
	}

	// XLSX: typed cells
	f := xlsxOf(t, a, a.acct("/exports/invoices.xlsx?sort=issued_on"))
	rows, err := f.GetRows("invoices")
	if err != nil || len(rows) != 4 || rows[0][0] != "Číslo" || rows[1][0] != "2026-0002" {
		t.Fatalf("xlsx rows %v %v", rows, err)
	}
	totalCol := col(t, rows[0], "Celkem") + 1
	cell, _ := excelize.CoordinatesToCellName(totalCol, 3)
	if v, _ := f.GetCellValue("invoices", cell, excelize.Options{RawCellValue: true}); v != "1815.75" {
		t.Errorf("total cell %s = %q", cell, v)
	}
	if ty, _ := f.GetCellType("invoices", cell); ty != excelize.CellTypeUnset && ty != excelize.CellTypeNumber {
		t.Errorf("total cell type %v", ty)
	}
	dateCell, _ := excelize.CoordinatesToCellName(col(t, rows[0], "Vystaveno")+1, 2)
	if v, _ := f.GetCellValue("invoices", dateCell); v != "1.2.2026" {
		t.Errorf("date cell %q", v)
	}
	if v, _ := f.GetCellValue("invoices", dateCell, excelize.Options{RawCellValue: true}); v != "46054" {
		t.Errorf("raw date serial %q", v) // 2026-02-01
	}
	style, _ := f.GetCellStyle("invoices", "A1")
	if s, _ := f.GetStyle(style); s == nil || s.Font == nil || !s.Font.Bold {
		t.Errorf("header not bold")
	}
}

func TestExportSubjectsExpenses(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	newSubject(a, api.SubjectCreate{Name: "Zeta", Type: "supplier", DueDays: intPtr(30), CustomID: strPtr("Z1")})
	newSubject(a, api.SubjectCreate{Name: "Alfa", Email: "alfa@example.cz"})
	newSubject(b, api.SubjectCreate{Name: "Cizí"})

	recs := csvOf(t, a, a.acct("/exports/subjects.csv"))
	if len(recs) != 3 || recs[1][col(t, recs[0], "Název")] != "Alfa" || recs[2][col(t, recs[0], "Splatnost (dny)")] != "30" ||
		recs[2][col(t, recs[0], "Vlastní ID")] != "Z1" || recs[2][col(t, recs[0], "Typ")] != "dodavatel" {
		t.Fatalf("subjects %v", recs)
	}
	recs = csvOf(t, a, a.acct("/exports/subjects.csv?type=supplier"))
	if len(recs) != 2 {
		t.Errorf("supplier filter %v", recs)
	}
	f := xlsxOf(t, a, a.acct("/exports/subjects.xlsx?query=alf"))
	if rows, _ := f.GetRows("subjects"); len(rows) != 2 || rows[1][col(t, rows[0], "E-mail")] != "alfa@example.cz" {
		t.Errorf("subjects xlsx %v", rows)
	}

	createExp(a, api.ExpenseCreate{OriginalNumber: "FV-7", ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: strPtr("Dodavatel")},
		Category: "software", Lines: []api.InvoiceLineInput{line("Licence", "1", 99900, nil)}})
	createExp(a, api.ExpenseCreate{ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: strPtr("Jiný")},
		Category: "cesty", Lines: []api.InvoiceLineInput{line("Vlak", "1", 30000, nil)}})
	recs = csvOf(t, a, a.acct("/exports/expenses.csv?category=software"))
	h := recs[0]
	if len(recs) != 2 || recs[1][col(t, h, "Číslo dokladu dodavatele")] != "FV-7" || recs[1][col(t, h, "Celkem")] != "1208,79" ||
		recs[1][col(t, h, "Daňově uznatelný")] != "ano" || recs[1][col(t, h, "Stav")] != "neuhrazený" {
		t.Errorf("expenses %v", recs)
	}
	f = xlsxOf(t, a, a.acct("/exports/expenses.xlsx"))
	if rows, _ := f.GetRows("expenses"); len(rows) != 3 {
		t.Errorf("expenses xlsx %v", rows)
	}
	recs = csvOf(t, b, b.acct("/exports/expenses.csv"))
	if len(recs) != 1 {
		t.Errorf("b expenses %v", recs)
	}
}

func zipNames(t *testing.T, res *http.Response, body []byte) []string {
	t.Helper()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("zip: %d %s", res.StatusCode, body)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	names := []string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		head := make([]byte, 5)
		_, _ = rc.Read(head)
		_ = rc.Close()
		if strings.HasSuffix(f.Name, ".pdf") && string(head[:4]) != "%PDF" || strings.HasSuffix(f.Name, ".isdoc") && string(head) != "<?xml" {
			t.Errorf("%s content %q", f.Name, head)
		}
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

func TestExportPDFZip(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), IssuedOn: "2026-01-10", Lines: []api.InvoiceLineInput{line("A", "1", 1000, nil)}})
	createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), IssuedOn: "2026-02-10", Lines: []api.InvoiceLineInput{line("B", "1", 1000, nil)}})
	createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: new(subj.ID), IssuedOn: "2026-02-11", Number: "2026-0002",
		Lines: []api.InvoiceLineInput{line("C", "1", 1000, nil)}})

	names := zipOf(t, a, a.acct("/exports/pdf.zip"))
	if strings.Join(names, ",") != "faktura-2026-0001.pdf,faktura-2026-0002-invoice.pdf,faktura-2026-0002.pdf" &&
		strings.Join(names, ",") != "faktura-2026-0001.pdf,faktura-2026-0002-proforma.pdf,faktura-2026-0002.pdf" {
		t.Errorf("names %v", names)
	}
	res, body := a.do("GET", a.acct("/exports/pdf.zip?since=2026-02-01&until=2026-02-28&document_type=invoice&isdoc=true"), nil)
	names = zipNames(t, res, body)
	if strings.Join(names, ",") != "faktura-2026-0002.isdoc,faktura-2026-0002.pdf" {
		t.Errorf("filtered names %v", names)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="doklady-2026-02-01_2026-02-28.zip"` {
		t.Errorf("disposition %q", cd)
	}
	if names := zipOf(t, b, b.acct("/exports/pdf.zip")); len(names) != 0 {
		t.Errorf("b zip %v", names)
	}
}
