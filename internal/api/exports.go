package api

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

// Exports (SPEC §7.11): lists as CSV (UTF-8 with BOM, ";" separated,
// decimal comma — opens directly in a Czech Excel) or XLSX (typed number and
// date cells), with the same filters as the list endpoints, and a ZIP of
// PDFs (+ ISDOC) for the accountant. Everything is streamed page by page.

// exportRoles: exports contain the same data as the lists, so every role
// that reads documents may export them (accountants in particular).
var exportRoles = auth.Allow(model.RoleOwner, model.RoleAdmin, model.RoleAccountant, model.RoleMember)

// exportPage is the number of rows loaded per query while streaming.
const exportPage = 500

type colKind int

const (
	colText    colKind = iota
	colMoney           // int64 minor units
	colDate            // "YYYY-MM-DD" string
	colInt             // integer
	colDecimal         // decimal string ("24.355")
)

type column[T any] struct {
	name string
	kind colKind
	get  func(*T) any
}

type exportFormat struct {
	ext, contentType string
}

var (
	formatCSV  = exportFormat{".csv", "text/csv; charset=utf-8"}
	formatXLSX = exportFormat{".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}
)

// registerExport registers GET /exports/<name>.csv and .xlsx for a table
// built by rows from the (filtered, ordered) query returned by query.
func registerExport[F, T any](g huma.API, name, summary string, query func(ctx context.Context, f *F) *gorm.DB, cols []column[T]) {
	for _, fm := range []exportFormat{formatCSV, formatXLSX} {
		op := huma.Operation{
			OperationID: "export-" + name + strings.ReplaceAll(fm.ext, ".", "-"),
			Method:      http.MethodGet,
			Path:        "/exports/" + name + fm.ext,
			Summary:     summary + " (" + strings.TrimPrefix(fm.ext, ".") + ")",
			Tags:        []string{"Exports"},
			Responses:   fileResponses(strings.Split(fm.contentType, ";")[0], summary),
		}
		exportRoles(&op)
		huma.Register(g, op, func(ctx context.Context, in *F) (*huma.StreamResponse, error) {
			q := query(ctx, in)
			return &huma.StreamResponse{Body: func(hc huma.Context) {
				hc.SetHeader("Content-Type", fm.contentType)
				hc.SetHeader("Content-Disposition", `attachment; filename="`+name+fm.ext+`"`)
				w := hc.BodyWriter()
				var err error
				if fm == formatCSV {
					err = writeCSV(w, q, cols)
				} else {
					err = writeXLSX(w, q, cols, name)
				}
				if err != nil {
					// Headers are gone already; the truncated body is the signal.
					_, _ = io.WriteString(w, "\n# export failed")
				}
			}}, nil
		})
	}
}

// eachPage loads q page by page (q must be ordered deterministically) and
// calls fn with every row. No other query runs while a page is loaded, so
// this is safe on single-connection SQLite.
func eachPage[T any](q *gorm.DB, fn func(*T) error) error {
	for offset := 0; ; offset += exportPage {
		var rows []T
		if err := q.Session(&gorm.Session{}).Offset(offset).Limit(exportPage).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := fn(&rows[i]); err != nil {
				return err
			}
		}
		if len(rows) < exportPage {
			return nil
		}
	}
}

func writeCSV[T any](w io.Writer, q *gorm.DB, cols []column[T]) error {
	if _, err := io.WriteString(w, "\xEF\xBB\xBF"); err != nil { // BOM: Excel detects UTF-8
		return err
	}
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.name
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	rec := make([]string, len(cols))
	err := eachPage(q, func(row *T) error {
		for i, c := range cols {
			rec[i] = csvValue(c.kind, c.get(row))
		}
		return cw.Write(rec)
	})
	cw.Flush()
	if err != nil {
		return err
	}
	return cw.Error()
}

func csvValue(kind colKind, v any) string {
	switch kind {
	case colMoney:
		return decimalComma(v.(int64))
	case colDecimal:
		return strings.ReplaceAll(v.(string), ".", ",")
	case colInt:
		if p, ok := v.(*int); ok {
			if p == nil {
				return ""
			}
			return strconv.Itoa(*p)
		}
		return fmt.Sprint(v)
	default:
		return csvText(fmt.Sprint(v))
	}
}

// csvText neutralizes spreadsheet formulas in text cells (CSV injection):
// a value starting with = + - @ tab or CR gets a leading apostrophe, so
// Excel/LibreOffice show it as text instead of evaluating it.
func csvText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// decimalComma formats minor units as "-1234,50".
func decimalComma(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d,%02d", sign, v/100, v%100)
}

func writeXLSX[T any](w io.Writer, q *gorm.DB, cols []column[T], sheet string) error {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	header, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"4C1D95"}},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return err
	}
	money, err := f.NewStyle(&excelize.Style{NumFmt: 4}) // #,##0.00
	if err != nil {
		return err
	}
	dateFmt := "d.m.yyyy"
	date, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt})
	if err != nil {
		return err
	}
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return err
	}
	for i, c := range cols {
		width := 14.0
		switch c.kind {
		case colText:
			width = 22
		case colMoney:
			width = 14
		case colDate:
			width = 11
		}
		if err := sw.SetColWidth(i+1, i+1, width); err != nil {
			return err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	cells := make([]any, len(cols))
	for i, c := range cols {
		cells[i] = excelize.Cell{StyleID: header, Value: c.name}
	}
	if err := sw.SetRow("A1", cells); err != nil {
		return err
	}
	rowNo := 1
	err = eachPage(q, func(row *T) error {
		rowNo++
		for i, c := range cols {
			cells[i] = xlsxCell(c.kind, c.get(row), money, date)
		}
		cell, _ := excelize.CoordinatesToCellName(1, rowNo)
		return sw.SetRow(cell, cells)
	})
	if err != nil {
		return err
	}
	if err := sw.Flush(); err != nil {
		return err
	}
	return f.Write(w)
}

func xlsxCell(kind colKind, v any, money, date int) any {
	switch kind {
	case colMoney:
		// a spreadsheet stores numbers as doubles anyway; exact to the haléř
		return excelize.Cell{StyleID: money, Value: float64(v.(int64)) / 100}
	case colDate:
		t, err := time.Parse(time.DateOnly, v.(string))
		if err != nil {
			return ""
		}
		return excelize.Cell{StyleID: date, Value: t}
	case colDecimal:
		if f, err := strconv.ParseFloat(v.(string), 64); err == nil {
			return f
		}
		return v
	case colInt:
		if p, ok := v.(*int); ok {
			if p == nil {
				return ""
			}
			return *p
		}
		return v
	default:
		return v
	}
}

// ---- tables ----

var docTypeNames = map[string]string{
	model.DocInvoice: "faktura", model.DocProforma: "zálohová faktura", model.DocCorrection: "opravný doklad",
}

var statusNames = map[string]string{
	model.StatusOpen: "vystavená", model.StatusSent: "odeslaná", billing.StatusOverdue: "po splatnosti",
	model.StatusPaid: "zaplacená", model.StatusCancelled: "stornovaná", model.StatusUncollectible: "nedobytná",
}

var expenseStatusNames = map[string]string{
	model.StatusOpen: "neuhrazený", billing.StatusOverdue: "po splatnosti", model.StatusPaid: "uhrazený",
}

func yesNo(b bool) string {
	if b {
		return "ano"
	}
	return "ne"
}

func (s *server) invoiceColumns() []column[model.Invoice] {
	type c = column[model.Invoice]
	return []c{
		{"Číslo", colText, func(m *model.Invoice) any { return m.Number }},
		{"Typ dokladu", colText, func(m *model.Invoice) any { return docTypeNames[m.DocumentType] }},
		{"Stav", colText, func(m *model.Invoice) any {
			return statusNames[billing.EffectiveStatus(m.Status, m.DueOn, s.today())]
		}},
		{"Odběratel", colText, func(m *model.Invoice) any { return m.ClientName }},
		{"IČO", colText, func(m *model.Invoice) any { return m.ClientRegistrationNo }},
		{"DIČ", colText, func(m *model.Invoice) any { return m.ClientVatNo }},
		{"Země", colText, func(m *model.Invoice) any { return m.ClientCountry }},
		{"Vystaveno", colDate, func(m *model.Invoice) any { return m.IssuedOn }},
		{"DUZP", colDate, func(m *model.Invoice) any { return m.TaxableFulfillmentDue }},
		{"Splatnost", colDate, func(m *model.Invoice) any { return m.DueOn }},
		{"Zaplaceno dne", colDate, func(m *model.Invoice) any { return m.PaidOn }},
		{"Měna", colText, func(m *model.Invoice) any { return m.Currency }},
		{"Kurz", colDecimal, func(m *model.Invoice) any { return m.ExchangeRate }},
		{"Základ", colMoney, func(m *model.Invoice) any { return m.Subtotal }},
		{"DPH", colMoney, func(m *model.Invoice) any { return m.VatTotal }},
		{"Zaokrouhlení", colMoney, func(m *model.Invoice) any { return m.Rounding }},
		{"Celkem", colMoney, func(m *model.Invoice) any { return m.Total }},
		{"Uhrazeno", colMoney, func(m *model.Invoice) any { return m.PaidAmount }},
		{"Zbývá uhradit", colMoney, func(m *model.Invoice) any { return m.Total - m.PaidAmount }},
		{"Variabilní symbol", colText, func(m *model.Invoice) any { return m.VariableSymbol }},
		{"Číslo objednávky", colText, func(m *model.Invoice) any { return m.OrderNumber }},
		{"Štítky", colText, func(m *model.Invoice) any { return strings.Join(m.Tags, ", ") }},
	}
}

var subjectTypeNames = map[string]string{
	model.SubjectCustomer: "odběratel", model.SubjectSupplier: "dodavatel", model.SubjectBoth: "odběratel i dodavatel",
}

var subjectColumns = []column[model.Subject]{
	{"Vlastní ID", colText, func(m *model.Subject) any {
		if m.CustomID == nil {
			return ""
		}
		return *m.CustomID
	}},
	{"Typ", colText, func(m *model.Subject) any { return subjectTypeNames[m.Type] }},
	{"Název", colText, func(m *model.Subject) any { return m.Name }},
	{"Kontaktní osoba", colText, func(m *model.Subject) any { return m.FullName }},
	{"IČO", colText, func(m *model.Subject) any { return m.RegistrationNo }},
	{"DIČ", colText, func(m *model.Subject) any { return m.VatNo }},
	{"IČ DPH", colText, func(m *model.Subject) any { return m.LocalVatNo }},
	{"Ulice", colText, func(m *model.Subject) any { return m.Street }},
	{"Město", colText, func(m *model.Subject) any { return m.City }},
	{"PSČ", colText, func(m *model.Subject) any { return m.Zip }},
	{"Země", colText, func(m *model.Subject) any { return m.Country }},
	{"E-mail", colText, func(m *model.Subject) any { return m.Email }},
	{"E-mail (kopie)", colText, func(m *model.Subject) any { return m.EmailCopy }},
	{"Telefon", colText, func(m *model.Subject) any { return m.Phone }},
	{"Web", colText, func(m *model.Subject) any { return m.Web }},
	{"Bankovní účet", colText, func(m *model.Subject) any { return m.BankAccount }},
	{"IBAN", colText, func(m *model.Subject) any { return m.IBAN }},
	{"SWIFT", colText, func(m *model.Subject) any { return m.SwiftBIC }},
	{"Splatnost (dny)", colInt, func(m *model.Subject) any { return m.DueDays }},
	{"Poznámka", colText, func(m *model.Subject) any { return m.Note }},
}

func (s *server) expenseColumns() []column[model.Expense] {
	type c = column[model.Expense]
	return []c{
		{"Číslo", colText, func(m *model.Expense) any { return m.Number }},
		{"Číslo dokladu dodavatele", colText, func(m *model.Expense) any { return m.OriginalNumber }},
		{"Stav", colText, func(m *model.Expense) any {
			return expenseStatusNames[billing.EffectiveStatus(m.Status, m.DueOn, s.today())]
		}},
		{"Dodavatel", colText, func(m *model.Expense) any { return m.SupplierName }},
		{"IČO", colText, func(m *model.Expense) any { return m.SupplierRegistrationNo }},
		{"DIČ", colText, func(m *model.Expense) any { return m.SupplierVatNo }},
		{"Vystaveno", colDate, func(m *model.Expense) any { return m.IssuedOn }},
		{"DUZP", colDate, func(m *model.Expense) any { return m.TaxableFulfillmentDue }},
		{"Splatnost", colDate, func(m *model.Expense) any { return m.DueOn }},
		{"Zaplaceno dne", colDate, func(m *model.Expense) any { return m.PaidOn }},
		{"Kategorie", colText, func(m *model.Expense) any { return m.Category }},
		{"Popis", colText, func(m *model.Expense) any { return m.Description }},
		{"Měna", colText, func(m *model.Expense) any { return m.Currency }},
		{"Kurz", colDecimal, func(m *model.Expense) any { return m.ExchangeRate }},
		{"Základ", colMoney, func(m *model.Expense) any { return m.Subtotal }},
		{"DPH", colMoney, func(m *model.Expense) any { return m.VatTotal }},
		{"Celkem", colMoney, func(m *model.Expense) any { return m.Total }},
		{"Uhrazeno", colMoney, func(m *model.Expense) any { return m.PaidAmount }},
		{"Zbývá uhradit", colMoney, func(m *model.Expense) any { return m.Total - m.PaidAmount }},
		{"Daňově uznatelný", colText, func(m *model.Expense) any { return yesNo(m.TaxDeductible) }},
		{"Variabilní symbol", colText, func(m *model.Expense) any { return m.VariableSymbol }},
		{"Štítky", colText, func(m *model.Expense) any { return strings.Join(m.Tags, ", ") }},
	}
}

func (s *server) registerExports(g huma.API) {
	registerExport(g, "invoices", "Invoices", func(ctx context.Context, f *InvoiceFilter) *gorm.DB {
		return f.query(s.scoped(ctx), s.today())
	}, s.invoiceColumns())
	registerExport(g, "subjects", "Subjects", func(ctx context.Context, f *SubjectFilter) *gorm.DB {
		return f.query(s.scoped(ctx))
	}, subjectColumns)
	registerExport(g, "expenses", "Expenses", func(ctx context.Context, f *ExpenseFilter) *gorm.DB {
		return f.query(s.scoped(ctx), s.today())
	}, s.expenseColumns())

	op := huma.Operation{
		OperationID: "export-pdf-zip",
		Method:      http.MethodGet,
		Path:        "/exports/pdf.zip",
		Summary:     "ZIP of invoice PDFs (optionally with ISDOC)",
		Tags:        []string{"Exports"},
		Responses:   fileResponses("application/zip", "ZIP archive"),
	}
	exportRoles(&op)
	huma.Register(g, op, s.exportPDFZip)
}

// maxZipDocuments bounds one ZIP export (each document is rendered).
const maxZipDocuments = 5000

func (s *server) exportPDFZip(ctx context.Context, in *struct {
	InvoiceFilter
	ISDOC bool `query:"isdoc" doc:"Also include the ISDOC XML of every document"`
}) (*huma.StreamResponse, error) {
	// Only ids are collected up front; documents are loaded and rendered one
	// by one while the archive is streamed.
	var ids []uint
	if err := in.InvoiceFilter.query(s.scoped(ctx), s.today()).Limit(maxZipDocuments+1).Pluck("id", &ids).Error; err != nil {
		return nil, dbErr(err, "invoices")
	}
	if len(ids) > maxZipDocuments {
		return nil, invalid("since", fmt.Sprintf("more than %d documents; narrow the period", maxZipDocuments))
	}
	name := "doklady"
	if in.Since != "" || in.Until != "" {
		name += "-" + strings.Trim(in.Since+"_"+in.Until, "_")
	}
	release, err := s.acquireExport(ctx)
	if err != nil {
		return nil, err
	}
	return &huma.StreamResponse{Body: func(hc huma.Context) {
		defer release()
		hc.SetHeader("Content-Type", "application/zip")
		hc.SetHeader("Content-Disposition", `attachment; filename="`+docFilenameSafe(name)+`.zip"`)
		zw := zip.NewWriter(hc.BodyWriter())
		defer zw.Close()
		used := map[string]int{}
		for _, id := range ids {
			inv, err := loadInvoice(ctx, s.db.WithContext(ctx), id)
			if err != nil {
				return
			}
			base := strings.TrimSuffix(pdfFilename(inv.Number), ".pdf")
			if n := used[base]; n > 0 { // numbers are unique per document type only
				used[base]++
				base = fmt.Sprintf("%s-%s", base, inv.DocumentType)
			} else {
				used[base] = 1
			}
			extendWriteDeadline(hc, 2*time.Minute)
			b, err := s.renderInvoicePDF(ctx, inv, pdf.Options{})
			if err != nil || zipFile(zw, base+".pdf", inv.UpdatedAt, b) != nil {
				return
			}
			if in.ISDOC {
				x, err := s.renderISDOC(ctx, inv)
				if err != nil || zipFile(zw, base+".isdoc", inv.UpdatedAt, x) != nil {
					return
				}
			}
		}
	}}, nil
}

func zipFile(zw *zip.Writer, name string, mod time.Time, b []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: mod})
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// docFilenameSafe keeps only [A-Za-z0-9._-] of s.
func docFilenameSafe(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(pdfFilename(s), "faktura-"), ".pdf")
}
