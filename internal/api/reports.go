package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/reports"
)

// Tax reports (SPEC §7.11). Amounts are CZK minor units; documents in a
// foreign currency are converted with their exchange rate.

// reportRoles: tax reports are for the people doing the bookkeeping, not for
// members who only issue documents.
var reportRoles = auth.Allow(model.RoleOwner, model.RoleAdmin, model.RoleAccountant)

// VatReport is the VAT return and control statement of a period.
type VatReport struct {
	Period    string                   `json:"period" example:"2026-Q3"`
	From      string                   `json:"from"`
	To        string                   `json:"to"`
	VatPeriod string                   `json:"vat_period" enum:"month,quarter" doc:"Account setting"`
	Currency  string                   `json:"currency" example:"CZK"`
	Return    reports.VatReturn        `json:"return" doc:"Rows of the VAT return (DPHDP3)"`
	Control   reports.ControlStatement `json:"control" doc:"Control statement (DPHKH1)"`
	Warnings  []reports.Warning        `json:"warnings" nullable:"false" doc:"Documents that could not be classified"`
}

type vatReportInput struct {
	Period string `query:"period" doc:"YYYY-MM or YYYY-Qn; default: the previous period per the account's vat_period"`
}

func (s *server) registerReports(g huma.API) {
	huma.Get(g, "/reports/vat", s.getVatReport, reportRoles)
	for _, x := range []struct{ name, summary string }{
		{"dphdp3", "VAT return as EPO XML (DPHDP3)"},
		{"dphkh1", "VAT control statement as EPO XML (DPHKH1)"},
	} {
		op := huma.Operation{
			OperationID: "get-report-vat-" + x.name,
			Method:      http.MethodGet,
			Path:        "/reports/vat/" + x.name + ".xml",
			Summary:     x.summary,
			Tags:        []string{"Reports"},
			Responses:   fileResponses("application/xml", x.summary),
		}
		reportRoles(&op)
		name := x.name
		huma.Register(g, op, func(ctx context.Context, in *vatReportInput) (*FileOutput, error) {
			return s.getVatXML(ctx, in, name)
		})
	}
	huma.Get(g, "/reports/overview", s.getOverview, reportRoles)
}

// vatReport computes the report of the requested period (409 for accounts
// that are not VAT payers).
func (s *server) vatReport(ctx context.Context, period string) (*reports.VatReport, error) {
	acc := auth.AccountFrom(ctx)
	if acc.VatMode != model.VatModePayer {
		return nil, conflict(CodeNotVatPayer, "VAT reports are available only for VAT payers (vat_mode = vat_payer)")
	}
	var p reports.Period
	if period == "" {
		p = reports.PreviousPeriod(s.deps.Now(), acc.VatPeriod == model.VatPeriodQuarter)
	} else {
		var err error
		if p, err = reports.ParsePeriod(period); err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
				&huma.ErrorDetail{Location: "query.period", Message: err.Error(), Value: period})
		}
	}
	r := reports.NewVatReport(p)
	from, to := p.Range()
	for _, src := range []func(context.Context, *reports.VatReport, string, string) error{s.vatSales, s.vatPurchases} {
		if err := src(ctx, r, from, to); err != nil {
			return nil, err
		}
	}
	r.Finish()
	return r, nil
}

// vatSales adds issued invoices and corrections with DUZP in the period
// (cancelled documents and documents issued as a non-payer excluded).
func (s *server) vatSales(ctx context.Context, r *reports.VatReport, from, to string) error {
	var invs []model.Invoice
	err := s.scoped(ctx).Preload("Lines").
		Where("document_type IN ? AND status <> ? AND your_vat_mode = ? AND taxable_fulfillment_due BETWEEN ? AND ?",
			[]string{model.DocInvoice, model.DocCorrection}, model.StatusCancelled, model.VatModePayer, from, to).
		Order("taxable_fulfillment_due, number, id").Find(&invs).Error
	if err != nil {
		return dbErr(err, "invoices")
	}
	// Totals of corrected invoices decide A.4 / A.5 of their corrections.
	relIDs := []uint{}
	for _, m := range invs {
		if m.DocumentType == model.DocCorrection && m.RelatedID != nil {
			relIDs = append(relIDs, *m.RelatedID)
		}
	}
	related := map[uint]int64{}
	if len(relIDs) > 0 {
		var rels []model.Invoice
		if err := s.scoped(ctx).Select("id", "total", "currency", "exchange_rate").Where("id IN ?", relIDs).Find(&rels).Error; err != nil {
			return dbErr(err, "invoices")
		}
		for _, m := range rels {
			related[m.ID] = toCZK(m.Total, m.Currency, m.ExchangeRate)
		}
	}
	for i := range invs {
		m := &invs[i]
		recap, total, err := czkRecap(billingLines(m.Lines), billingOptions(m), m.Currency, m.ExchangeRate)
		if err != nil {
			r.Warn(reports.WarnCalculation, m.Number, nil, "%s", err.Error())
			continue
		}
		sale := reports.Sale{
			Number: m.Number, TaxPointDate: m.TaxableFulfillmentDue, CustomerVatNo: m.ClientVatNo,
			CustomerCountry: m.ClientCountry, ReverseCharge: m.ReverseCharge, Recap: recap, Total: total,
		}
		if m.RelatedID != nil {
			sale.ControlTotal = related[*m.RelatedID]
		}
		r.AddSale(sale)
	}
	return nil
}

// vatPurchases adds tax-deductible expenses with DUZP (or, without it, the
// issue date) in the period.
func (s *server) vatPurchases(ctx context.Context, r *reports.VatReport, from, to string) error {
	var exps []model.Expense
	err := s.scoped(ctx).Preload("Lines").
		Where("tax_deductible = ? AND (taxable_fulfillment_due BETWEEN ? AND ? OR (taxable_fulfillment_due = '' AND issued_on BETWEEN ? AND ?))",
			true, from, to, from, to).
		Order("issued_on, number, id").Find(&exps).Error
	if err != nil {
		return dbErr(err, "expenses")
	}
	for i := range exps {
		m := &exps[i]
		recap, total, err := czkRecap(expenseBillingLines(m.Lines), expenseOptions(m), m.Currency, m.ExchangeRate)
		if err != nil {
			r.Warn(reports.WarnCalculation, m.Number, nil, "%s", err.Error())
			continue
		}
		r.AddPurchase(reports.Purchase{
			Number: defaultStr(strings.TrimSpace(m.OriginalNumber), m.Number), TaxPointDate: defaultStr(m.TaxableFulfillmentDue, m.IssuedOn),
			SupplierVatNo: m.SupplierVatNo, Recap: recap, Total: total,
		})
	}
	return nil
}

// czkRecap is the VAT recap of a document converted to CZK, and its total
// (Σ base + VAT, without rounding).
func czkRecap(lines []billing.Line, opt billing.Options, currency, rate string) ([]reports.RateAmount, int64, error) {
	t, err := billing.Calculate(lines, opt)
	if err != nil {
		return nil, 0, err
	}
	r, err := czkRate(currency, rate)
	if err != nil {
		return nil, 0, err
	}
	out := make([]reports.RateAmount, len(t.VatRecap))
	var total int64
	for i, v := range t.VatRecap {
		out[i] = reports.RateAmount{RateBps: v.VatRateBps, Base: billing.ToLocal(v.Base, r), Vat: billing.ToLocal(v.Vat, r)}
		total += out[i].Base + out[i].Vat
	}
	return out, total, nil
}

func czkRate(currency, rate string) (int64, error) {
	if currency == "" || strings.EqualFold(currency, "CZK") {
		return billing.RateScale, nil
	}
	return billing.ParseRate(rate)
}

// toCZK converts an amount of a document to CZK (0 for an invalid rate).
func toCZK(amount int64, currency, rate string) int64 {
	r, err := czkRate(currency, rate)
	if err != nil {
		return 0
	}
	return billing.ToLocal(amount, r)
}

func toVatReport(r *reports.VatReport, acc *model.Account) VatReport {
	from, to := r.Period.Range()
	return VatReport{
		Period: r.Period.String(), From: from, To: to, VatPeriod: defaultStr(acc.VatPeriod, model.VatPeriodMonth),
		Currency: "CZK", Return: r.Return, Control: withCZPrefix(r.Control), Warnings: r.Warnings,
	}
}

// withCZPrefix shows the DIČ of the control statement rows with the "CZ"
// prefix (the EPO XML has the numeric part only).
func withCZPrefix(c reports.ControlStatement) reports.ControlStatement {
	cz := func(d string) string {
		if d == "" || strings.HasPrefix(d, "CZ") {
			return d
		}
		return "CZ" + d
	}
	out := c
	out.A1 = make([]reports.A1Row, len(c.A1))
	for i, r := range c.A1 {
		r.CustomerVatNo = cz(r.CustomerVatNo)
		out.A1[i] = r
	}
	out.A4 = make([]reports.DocumentRow, len(c.A4))
	for i, r := range c.A4 {
		r.VatNo = cz(r.VatNo)
		out.A4[i] = r
	}
	out.B2 = make([]reports.DocumentRow, len(c.B2))
	for i, r := range c.B2 {
		r.VatNo = cz(r.VatNo)
		out.B2[i] = r
	}
	return out
}

func (s *server) getVatReport(ctx context.Context, in *vatReportInput) (*Out[VatReport], error) {
	r, err := s.vatReport(ctx, in.Period)
	if err != nil {
		return nil, err
	}
	return &Out[VatReport]{Body: toVatReport(r, auth.AccountFrom(ctx))}, nil
}

func (s *server) getVatXML(ctx context.Context, in *vatReportInput, form string) (*FileOutput, error) {
	r, err := s.vatReport(ctx, in.Period)
	if err != nil {
		return nil, err
	}
	acc := auth.AccountFrom(ctx)
	tp := reports.Taxpayer{
		VatNo: acc.VatNo, Name: acc.Name, Street: acc.Street, City: acc.City, Zip: acc.Zip, Country: acc.Country,
		Email: acc.Email, Phone: acc.Phone, TaxOffice: acc.TaxOffice, TaxOfficeBranch: acc.TaxOfficeBranch,
	}
	opt := reports.EPOOptions{Filed: s.deps.Now(), Software: "NanoFaktura", Version: "1.0"}
	var b []byte
	if form == "dphdp3" {
		b, err = reports.DPHDP3(r, tp, opt)
	} else {
		b, err = reports.DPHKH1(r, tp, opt)
	}
	if errors.Is(err, reports.ErrTaxpayer) {
		return nil, conflict(CodeMissingTaxOffice, "set the tax office code (c_ufo) and the DIČ in the account settings first")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("xml generation failed", err)
	}
	return &FileOutput{
		ContentType:        "application/xml",
		ContentDisposition: fmt.Sprintf(`attachment; filename="%s-%s.xml"`, form, r.Period.String()),
		Body:               b,
	}, nil
}

// ---- overview ----

// TopCustomer is a customer with the sum of its documents in the year.
type TopCustomer struct {
	SubjectID uint   `json:"subject_id"`
	Name      string `json:"name"`
	Total     int64  `json:"total" doc:"Σ total of invoices and corrections (CZK)"`
	Count     int    `json:"count"`
}

// IncomeTax is the income-tax overview of a sole trader (cash basis).
type IncomeTax struct {
	Income       int64              `json:"income" doc:"Payments received in the year (without VAT for VAT payers)"`
	RealExpenses int64              `json:"real_expenses" doc:"Tax-deductible expenses paid in the year (without deductible VAT for VAT payers)"`
	RealTaxBase  int64              `json:"real_tax_base" doc:"income − real_expenses"`
	FlatRates    []reports.FlatRate `json:"flat_rates" nullable:"false" doc:"Flat-rate expense options (paušál) with statutory caps"`
}

// Overview are the yearly statistics (CZK; foreign documents converted).
type Overview struct {
	Year             int           `json:"year"`
	Currency         string        `json:"currency" example:"CZK"`
	RevenueByMonth   []int64       `json:"revenue_by_month" nullable:"false" minItems:"12" maxItems:"12" doc:"Σ total of invoices and corrections by issued_on (not cancelled)"`
	ExpensesByMonth  []int64       `json:"expenses_by_month" nullable:"false" minItems:"12" maxItems:"12" doc:"Σ total of expenses by issued_on"`
	ProfitByMonth    []int64       `json:"profit_by_month" nullable:"false" minItems:"12" maxItems:"12"`
	RevenueTotal     int64         `json:"revenue_total"`
	ExpensesTotal    int64         `json:"expenses_total"`
	ProfitTotal      int64         `json:"profit_total"`
	TopCustomers     []TopCustomer `json:"top_customers" nullable:"false" maxItems:"10"`
	AverageDaysToPay *float64      `json:"average_days_to_pay" doc:"Mean of paid_on − issued_on of invoices issued in the year and paid; null without data"`
	PaidCount        int           `json:"paid_count" doc:"Invoices in average_days_to_pay"`
	IncomeTax        IncomeTax     `json:"income_tax"`
}

func (s *server) getOverview(ctx context.Context, in *struct {
	Year int `query:"year" minimum:"2000" maximum:"2999" doc:"Default: current year"`
}) (*Out[Overview], error) {
	o := Overview{Year: in.Year, Currency: "CZK", RevenueByMonth: make([]int64, 12), ExpensesByMonth: make([]int64, 12),
		ProfitByMonth: make([]int64, 12), TopCustomers: []TopCustomer{}}
	if o.Year == 0 {
		o.Year = s.deps.Now().Year()
	}
	from, to := fmt.Sprintf("%04d-01-01", o.Year), fmt.Sprintf("%04d-12-31", o.Year)
	payer := auth.AccountFrom(ctx).VatMode == model.VatModePayer

	var invs []model.Invoice
	err := s.scoped(ctx).
		Select("id", "document_type", "status", "subject_id", "client_name", "issued_on", "paid_on", "currency", "exchange_rate", "total").
		Where("document_type IN ? AND status <> ? AND issued_on BETWEEN ? AND ?",
			[]string{model.DocInvoice, model.DocCorrection}, model.StatusCancelled, from, to).
		Order("issued_on, id").Find(&invs).Error
	if err != nil {
		return nil, dbErr(err, "invoices")
	}
	customers := map[uint]*TopCustomer{}
	var days, paid int
	for _, m := range invs {
		total := toCZK(m.Total, m.Currency, m.ExchangeRate)
		o.RevenueByMonth[month(m.IssuedOn)] += total
		c := customers[m.SubjectID]
		if c == nil {
			c = &TopCustomer{SubjectID: m.SubjectID}
			customers[m.SubjectID] = c
		}
		c.Name, c.Total, c.Count = m.ClientName, c.Total+total, c.Count+1 // latest name wins
		if m.DocumentType == model.DocInvoice && m.Status == model.StatusPaid && m.PaidOn != "" {
			if d, ok := daysBetween(m.IssuedOn, m.PaidOn); ok {
				days += d
				paid++
			}
		}
	}
	for _, c := range customers {
		o.TopCustomers = append(o.TopCustomers, *c)
	}
	sort.Slice(o.TopCustomers, func(i, j int) bool {
		a, b := o.TopCustomers[i], o.TopCustomers[j]
		return a.Total > b.Total || (a.Total == b.Total && a.SubjectID < b.SubjectID)
	})
	o.TopCustomers = o.TopCustomers[:min(10, len(o.TopCustomers))]
	if paid > 0 {
		avg := math.Round(float64(days)/float64(paid)*10) / 10
		o.AverageDaysToPay, o.PaidCount = &avg, paid
	}

	var exps []model.Expense
	if err := s.scoped(ctx).Select("id", "issued_on", "currency", "exchange_rate", "total").
		Where("issued_on BETWEEN ? AND ?", from, to).Find(&exps).Error; err != nil {
		return nil, dbErr(err, "expenses")
	}
	for _, m := range exps {
		o.ExpensesByMonth[month(m.IssuedOn)] += toCZK(m.Total, m.Currency, m.ExchangeRate)
	}
	for i := range 12 {
		o.ProfitByMonth[i] = o.RevenueByMonth[i] - o.ExpensesByMonth[i]
		o.RevenueTotal += o.RevenueByMonth[i]
		o.ExpensesTotal += o.ExpensesByMonth[i]
	}
	o.ProfitTotal = o.RevenueTotal - o.ExpensesTotal

	if o.IncomeTax, err = s.incomeTax(ctx, from, to, payer); err != nil {
		return nil, err
	}
	return &Out[Overview]{Body: o}, nil
}

// incomeTax: cash basis (daňová evidence) — payments received on invoices
// and corrections (proforma payments are counted through their final
// invoice) and payments of tax-deductible expenses, both without VAT when
// the account is a VAT payer (VAT share of a payment = its share of the total).
func (s *server) incomeTax(ctx context.Context, from, to string, payer bool) (IncomeTax, error) {
	type pay struct {
		Amount       int64
		Currency     string
		ExchangeRate string
		Subtotal     int64
		Total        int64
	}
	net := func(p pay) int64 {
		amount := toCZK(p.Amount, p.Currency, p.ExchangeRate)
		if payer && p.Total != 0 {
			amount, _ = billing.MulDivRound(amount, p.Subtotal, p.Total)
		}
		return amount
	}
	accountID := auth.AccountFrom(ctx).ID
	var t IncomeTax
	var in []pay
	err := s.db.WithContext(ctx).Table("payments").
		Select("payments.amount, invoices.currency, invoices.exchange_rate, invoices.subtotal, invoices.total").
		Joins("JOIN invoices ON invoices.id = payments.invoice_id").
		Where("payments.account_id = ? AND invoices.account_id = ? AND invoices.document_type IN ? AND payments.paid_on BETWEEN ? AND ?",
			accountID, accountID, []string{model.DocInvoice, model.DocCorrection}, from, to).
		Scan(&in).Error
	if err != nil {
		return t, dbErr(err, "payments")
	}
	for _, p := range in {
		t.Income += net(p)
	}
	var out []pay
	err = s.db.WithContext(ctx).Table("expense_payments").
		Select("expense_payments.amount, expenses.currency, expenses.exchange_rate, expenses.subtotal, expenses.total").
		Joins("JOIN expenses ON expenses.id = expense_payments.expense_id").
		Where("expense_payments.account_id = ? AND expenses.account_id = ? AND expenses.tax_deductible = ? AND expense_payments.paid_on BETWEEN ? AND ?",
			accountID, accountID, true, from, to).
		Scan(&out).Error
	if err != nil {
		return t, dbErr(err, "expense payments")
	}
	for _, p := range out {
		t.RealExpenses += net(p)
	}
	t.RealTaxBase = t.Income - t.RealExpenses
	t.FlatRates = reports.FlatRates(t.Income)
	return t, nil
}

// month returns the 0-based month of "YYYY-MM-DD".
func month(date string) int {
	if t, err := time.Parse(time.DateOnly, date); err == nil {
		return int(t.Month()) - 1
	}
	return 0
}

func daysBetween(from, to string) (int, bool) {
	a, err1 := time.Parse(time.DateOnly, from)
	b, err2 := time.Parse(time.DateOnly, to)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return int(b.Sub(a).Hours() / 24), true
}
