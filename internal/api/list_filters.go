package api

import (
	"strings"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

// InvoiceFilter are the query parameters shared by the invoice list and the
// invoice exports (CSV, XLSX, PDF ZIP), so an export contains exactly what
// the list shows.
type InvoiceFilter struct {
	Status       string `query:"status" enum:"open,sent,overdue,paid,cancelled,uncollectible" doc:"Effective status: open/sent exclude overdue documents"`
	DocumentType string `query:"document_type" enum:"invoice,proforma,correction"`
	SubjectID    uint   `query:"subject_id"`
	Since        string `query:"since" format:"date" doc:"issued_on ≥ since"`
	Until        string `query:"until" format:"date" doc:"issued_on ≤ until"`
	Query        string `query:"query" doc:"Number, client name or variable symbol (case-insensitive substring)"`
	Sort         string `query:"sort" enum:"-issued_on,issued_on,-number,due_on,-total" default:"-issued_on"`
}

var invoiceSorts = map[string]string{
	"-issued_on": "issued_on DESC, id DESC",
	"issued_on":  "issued_on, id",
	"-number":    "number DESC, id DESC",
	"due_on":     "due_on, id",
	"-total":     "total DESC, id DESC",
}

// query applies the filter and the ordering to q (already scoped to the
// account); today decides what is overdue.
func (f *InvoiceFilter) query(q *gorm.DB, today string) *gorm.DB {
	q = q.Model(&model.Invoice{})
	switch f.Status {
	case "":
	case billing.StatusOverdue:
		q = q.Where("status IN ? AND due_on <> '' AND due_on < ?", []string{model.StatusOpen, model.StatusSent}, today)
	case model.StatusOpen, model.StatusSent:
		q = q.Where("status = ? AND (due_on = '' OR due_on >= ?)", f.Status, today)
	default:
		q = q.Where("status = ?", f.Status)
	}
	if f.DocumentType != "" {
		q = q.Where("document_type = ?", f.DocumentType)
	}
	if f.SubjectID != 0 {
		q = q.Where("subject_id = ?", f.SubjectID)
	}
	if f.Since != "" {
		q = q.Where("issued_on >= ?", f.Since)
	}
	if f.Until != "" {
		q = q.Where("issued_on <= ?", f.Until)
	}
	if qs := strings.TrimSpace(f.Query); qs != "" {
		like := likePattern(qs)
		q = q.Where(`(LOWER(number) LIKE ? ESCAPE '\' OR LOWER(client_name) LIKE ? ESCAPE '\' OR variable_symbol LIKE ? ESCAPE '\')`, like, like, like)
	}
	order, ok := invoiceSorts[f.Sort]
	if !ok {
		order = invoiceSorts["-issued_on"]
	}
	return q.Order(order)
}

// SubjectFilter are the query parameters shared by the subject list and the
// subject exports.
type SubjectFilter struct {
	Query string `query:"query" doc:"Case-insensitive search in name, IČO and email"`
	Type  string `query:"type" enum:"customer,supplier,both" doc:"customer/supplier also match subjects of type both"`
}

// query applies the filter and the ordering to q (already scoped to the account).
func (f *SubjectFilter) query(q *gorm.DB) *gorm.DB {
	q = q.Model(&model.Subject{}).Order("LOWER(name), id")
	if qs := strings.TrimSpace(f.Query); qs != "" {
		like := likePattern(qs)
		q = q.Where(`(LOWER(name) LIKE ? ESCAPE '\' OR registration_no LIKE ? ESCAPE '\' OR LOWER(email) LIKE ? ESCAPE '\')`, like, like, like)
	}
	switch f.Type {
	case model.SubjectCustomer, model.SubjectSupplier:
		q = q.Where("type IN ?", []string{f.Type, model.SubjectBoth})
	case model.SubjectBoth:
		q = q.Where("type = ?", model.SubjectBoth)
	}
	return q
}

// ExpenseFilter are the query parameters shared by the expense list and the
// expense exports.
type ExpenseFilter struct {
	Status    string `query:"status" enum:"open,overdue,paid" doc:"Effective status: open excludes overdue expenses"`
	Category  string `query:"category" doc:"Exact category"`
	SubjectID uint   `query:"subject_id"`
	Since     string `query:"since" format:"date" doc:"issued_on ≥ since"`
	Until     string `query:"until" format:"date" doc:"issued_on ≤ until"`
	Query     string `query:"query" doc:"Number, original number, supplier name, variable symbol or description (case-insensitive substring)"`
	Sort      string `query:"sort" enum:"-issued_on,issued_on,-number,due_on,-total" default:"-issued_on"`
}

var expenseSorts = map[string]string{
	"-issued_on": "issued_on DESC, id DESC",
	"issued_on":  "issued_on, id",
	"-number":    "number DESC, id DESC",
	"due_on":     "due_on, id",
	"-total":     "total DESC, id DESC",
}

// query applies the filter and the ordering to q (already scoped to the
// account); today decides what is overdue.
func (f *ExpenseFilter) query(q *gorm.DB, today string) *gorm.DB {
	q = q.Model(&model.Expense{})
	switch f.Status {
	case "":
	case billing.StatusOverdue:
		q = q.Where("status = ? AND due_on <> '' AND due_on < ?", model.StatusOpen, today)
	case model.StatusOpen:
		q = q.Where("status = ? AND (due_on = '' OR due_on >= ?)", model.StatusOpen, today)
	default:
		q = q.Where("status = ?", f.Status)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.SubjectID != 0 {
		q = q.Where("subject_id = ?", f.SubjectID)
	}
	if f.Since != "" {
		q = q.Where("issued_on >= ?", f.Since)
	}
	if f.Until != "" {
		q = q.Where("issued_on <= ?", f.Until)
	}
	if qs := strings.TrimSpace(f.Query); qs != "" {
		like := likePattern(qs)
		q = q.Where(`(LOWER(number) LIKE ? ESCAPE '\' OR LOWER(original_number) LIKE ? ESCAPE '\' OR LOWER(supplier_name) LIKE ? ESCAPE '\'`+
			` OR variable_symbol LIKE ? ESCAPE '\' OR LOWER(description) LIKE ? ESCAPE '\')`, like, like, like, like, like)
	}
	order, ok := expenseSorts[f.Sort]
	if !ok {
		order = expenseSorts["-issued_on"]
	}
	return q.Order(order)
}
