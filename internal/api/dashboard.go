package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Dashboard are the account statistics (SPEC §4.6), only documents in the
// account's default currency.
type Dashboard struct {
	Year           int     `json:"year"`
	Currency       string  `json:"currency"`
	RevenueByMonth []int64 `json:"revenue_by_month" nullable:"false" minItems:"12" maxItems:"12" doc:"Σ total of invoices and corrections issued in each month of year (not cancelled)"`
	RevenueTotal   int64   `json:"revenue_total" doc:"Σ revenue_by_month"`
	UnpaidTotal    int64   `json:"unpaid_total" doc:"Σ remaining amount of open/sent documents (all types, all years)"`
	UnpaidCount    int64   `json:"unpaid_count"`
	OverdueTotal   int64   `json:"overdue_total" doc:"The part of unpaid that is past due"`
	OverdueCount   int64   `json:"overdue_count"`
}

func (s *server) registerDashboard(g huma.API) {
	huma.Get(g, "/dashboard", s.getDashboard)
}

func (s *server) getDashboard(ctx context.Context, in *struct {
	Year int `query:"year" minimum:"2000" maximum:"2999" doc:"Default: current year"`
}) (*Out[Dashboard], error) {
	acc := auth.AccountFrom(ctx)
	today := s.today()
	d := Dashboard{Year: in.Year, Currency: acc.DefaultCurrency, RevenueByMonth: make([]int64, 12)}
	if d.Year == 0 {
		d.Year = s.deps.Now().Year()
	}

	// substr() and CAST … AS BIGINT work on both SQLite and PostgreSQL.
	var months []struct {
		Month string
		Sum   int64
	}
	err := s.scoped(ctx).Model(&model.Invoice{}).
		Select("substr(issued_on, 6, 2) AS month, CAST(COALESCE(SUM(total), 0) AS BIGINT) AS sum").
		Where("currency = ? AND document_type IN ? AND status <> ? AND issued_on BETWEEN ? AND ?",
			d.Currency, []string{model.DocInvoice, model.DocCorrection}, model.StatusCancelled,
			fmt.Sprintf("%04d-01-01", d.Year), fmt.Sprintf("%04d-12-31", d.Year)).
		Group("substr(issued_on, 6, 2)").
		Scan(&months).Error
	if err != nil {
		return nil, dbErr(err, "statistics")
	}
	for _, m := range months {
		if n, err := strconv.Atoi(m.Month); err == nil && n >= 1 && n <= 12 {
			d.RevenueByMonth[n-1] = m.Sum
			d.RevenueTotal += m.Sum
		}
	}

	type agg struct {
		Count int64
		Sum   int64
	}
	unpaid := func(extra string, args ...any) (agg, error) {
		var a agg
		q := s.scoped(ctx).Model(&model.Invoice{}).
			Select("COUNT(*) AS count, CAST(COALESCE(SUM(total - paid_amount), 0) AS BIGINT) AS sum").
			Where("currency = ? AND status IN ?", d.Currency, []string{model.StatusOpen, model.StatusSent})
		if extra != "" {
			q = q.Where(extra, args...)
		}
		err := q.Scan(&a).Error
		return a, err
	}
	u, err := unpaid("")
	if err != nil {
		return nil, dbErr(err, "statistics")
	}
	o, err := unpaid("due_on <> '' AND due_on < ?", today)
	if err != nil {
		return nil, dbErr(err, "statistics")
	}
	d.UnpaidCount, d.UnpaidTotal, d.OverdueCount, d.OverdueTotal = u.Count, u.Sum, o.Count, o.Sum
	return &Out[Dashboard]{Body: d}, nil
}
