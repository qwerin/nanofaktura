package model

import "time"

// ExchangeRate caches one ČNB daily rate (global, not account-scoped).
//
// Date is the day the rate is valid for and ListDate the date of the ČNB list
// it comes from: rows fetched for a weekend/holiday have Date = that day and
// ListDate = the last published working day before it. Rate is CZK per
// Amount units, a decimal string ("13.546" for 100 JPY).
type ExchangeRate struct {
	ID        uint
	Date      string `gorm:"not null;size:10;uniqueIndex:idx_exchange_rates_date_currency"`
	Currency  string `gorm:"not null;size:3;uniqueIndex:idx_exchange_rates_date_currency"`
	ListDate  string `gorm:"not null;size:10"`
	Amount    int    `gorm:"not null"`
	Rate      string `gorm:"not null"`
	CreatedAt time.Time
}
