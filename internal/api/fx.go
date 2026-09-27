package api

import (
	"context"
	"fmt"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/cnb"
)

// defaultExchangeRate returns the exchange rate a new document gets (SPEC §7.7):
// rate when the client sent one; the ČNB rate of currency on the first valid
// of dates (DUZP, issued_on; default today) when the document is in a foreign
// currency and the account keeps its books in CZK; otherwise "" (= the
// default "1"). ČNB rates are CZK per unit, so for accounts with another
// default currency the rate must be entered manually.
//
// Call it before opening a transaction: the ČNB service uses its own DB
// connection for its cache (SQLite has only one).
func (s *server) defaultExchangeRate(ctx context.Context, currency, rate string, dates ...string) (string, error) {
	acc := auth.AccountFrom(ctx)
	if rate != "" || currency == "" || currency == acc.DefaultCurrency || acc.DefaultCurrency != "CZK" {
		return rate, nil
	}
	date := cnb.Today(s.deps.Now())
	for _, d := range dates {
		if d != "" {
			if billing.ValidDate(d) {
				date = d
			}
			break
		}
	}
	r, _, err := s.deps.CNB.Rate(ctx, currency, date)
	if err != nil {
		return "", invalid("exchange_rate",
			fmt.Sprintf("ČNB exchange rate of %s on %s is not available; enter exchange_rate manually", currency, date))
	}
	return r, nil
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
