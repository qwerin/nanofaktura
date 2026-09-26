package cnb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Prague must resolve even without system tzdata

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/model"
)

var prague = mustLoad("Europe/Prague")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Today returns the current date in Prague ("YYYY-MM-DD"), the calendar ČNB uses.
func Today(now time.Time) string { return now.In(prague).Format(time.DateOnly) }

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// Service answers rate queries from the model.ExchangeRate cache, fetching
// missing lists from src.
type Service struct {
	db  *gorm.DB
	src Fetcher
	now func() time.Time
}

// NewService returns a caching service; now may be nil (time.Now).
func NewService(db *gorm.DB, src Fetcher, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, src: src, now: now}
}

// Rate returns the ČNB rate of currency valid on date (YYYY-MM-DD) as CZK per
// one unit (decimal string, ≥ 3 dp) and the date of the ČNB list it comes
// from. A future date is treated as today. CZK itself is "1" on date.
//
// Errors: ErrInvalidCurrency, ErrInvalidDate, ErrUnknownCurrency, otherwise
// the source is unavailable or returned garbage.
func (s *Service) Rate(ctx context.Context, currency, date string) (rate, rateDate string, err error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !currencyRe.MatchString(currency) {
		return "", "", ErrInvalidCurrency
	}
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", "", ErrInvalidDate
	}
	today := Today(s.now())
	if date = d.Format(time.DateOnly); date > today {
		date = today
	}
	if currency == "CZK" {
		return "1.000", date, nil
	}

	db := s.db.WithContext(ctx)
	var rows []model.ExchangeRate
	if err := db.Where("date = ? AND currency = ?", date, currency).Limit(1).Find(&rows).Error; err != nil {
		return "", "", err
	}
	if len(rows) == 1 {
		return perUnitRow(rows[0])
	}
	var n int64
	if err := db.Model(&model.ExchangeRate{}).Where("date = ?", date).Count(&n).Error; err != nil {
		return "", "", err
	}
	if n > 0 { // the day's list is cached, it just does not contain the currency
		return "", "", ErrUnknownCurrency
	}

	list, err := s.src.Fetch(ctx, date)
	if err != nil {
		return "", "", err
	}
	if list.Date > date {
		return "", "", fmt.Errorf("%w: list %s returned for %s", ErrFormat, list.Date, date)
	}
	rows = toRows(list, list.Date)
	// The mapping date → older list is final for past days; for today the
	// list may still be published later (after 14:30), so don't cache it.
	if list.Date != date && date < today {
		rows = append(rows, toRows(list, date)...)
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return "", "", err
	}
	r := list.Find(currency)
	if r == nil {
		return "", "", ErrUnknownCurrency
	}
	rate, err = PerUnit(r.Rate, r.Amount)
	return rate, list.Date, err
}

func toRows(l *List, date string) []model.ExchangeRate {
	rows := make([]model.ExchangeRate, len(l.Rates))
	for i, r := range l.Rates {
		rows[i] = model.ExchangeRate{Date: date, ListDate: l.Date, Currency: r.Currency, Amount: r.Amount, Rate: r.Rate}
	}
	return rows
}

func perUnitRow(r model.ExchangeRate) (string, string, error) {
	rate, err := PerUnit(r.Rate, r.Amount)
	return rate, r.ListDate, err
}
