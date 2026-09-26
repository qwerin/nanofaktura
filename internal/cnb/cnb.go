// Package cnb provides the Czech National Bank (ČNB) daily exchange rates
// ("denní kurz devizového trhu"): a parser of the published text format, an
// HTTP client and a Service that caches the rates in the database.
//
// The text format (UTF-8, one list per working day, published after 14:30):
//
//	25.09.2026 #186
//	země|měna|množství|kód|kurz
//	EMU|euro|1|EUR|24,350
//	Japonsko|jen|100|JPY|13,546
//
// "množství" is the number of units the rate is quoted for (100 JPY = 13,546 CZK).
// Asking for a weekend, holiday or a day before publication returns the last
// published list, whose own date is in the first line.
package cnb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production ČNB endpoint; "?date=DD.MM.YYYY" is appended.
const DefaultBaseURL = "https://www.cnb.cz/cs/financni-trhy/devizovy-trh/kurzy-devizoveho-trhu/kurzy-devizoveho-trhu/denni_kurz.txt"

var (
	// ErrUnknownCurrency: ČNB does not publish a rate for the currency (on that day).
	ErrUnknownCurrency = errors.New("exchange rate not published for the currency")
	// ErrInvalidDate: the date is not YYYY-MM-DD.
	ErrInvalidDate = errors.New("invalid date, expected YYYY-MM-DD")
	// ErrInvalidCurrency: the currency is not a 3-letter ISO 4217 code.
	ErrInvalidCurrency = errors.New("invalid currency, expected 3-letter ISO 4217 code")
	// ErrFormat: the ČNB response could not be parsed.
	ErrFormat = errors.New("cnb: unexpected rate list format")
)

// Rate is one line of the list: Amount units of Currency cost Rate CZK.
type Rate struct {
	Currency string // ISO 4217, upper case
	Amount   int    // "množství", usually 1, 100 or 1000
	Rate     string // CZK per Amount units, decimal with a dot ("13.546")
}

// List is one published daily rate list.
type List struct {
	Date   string // YYYY-MM-DD of the list (may be before the requested day)
	Number int    // sequence number of the list within the year
	Rates  []Rate
}

// Find returns the rate of currency, or nil.
func (l *List) Find(currency string) *Rate {
	for i := range l.Rates {
		if l.Rates[i].Currency == currency {
			return &l.Rates[i]
		}
	}
	return nil
}

var (
	headerRe = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{4})\s+#(\d+)$`)
	rateRe   = regexp.MustCompile(`^\d+(,\d+)?$`)
	codeRe   = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ParseList parses the ČNB daily text format.
func ParseList(r io.Reader) (*List, error) {
	sc := bufio.NewScanner(r)
	var lines []string
	for sc.Scan() {
		if l := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF")); l != "" {
			lines = append(lines, l)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("%w: too short", ErrFormat)
	}
	m := headerRe.FindStringSubmatch(lines[0])
	if m == nil {
		return nil, fmt.Errorf("%w: header %q", ErrFormat, lines[0])
	}
	l := &List{Date: m[3] + "-" + m[2] + "-" + m[1]}
	l.Number, _ = strconv.Atoi(m[4])
	if _, err := time.Parse(time.DateOnly, l.Date); err != nil {
		return nil, fmt.Errorf("%w: header date %q", ErrFormat, lines[0])
	}
	for _, line := range lines[2:] { // lines[1] is the column header
		f := strings.Split(line, "|")
		if len(f) != 5 {
			return nil, fmt.Errorf("%w: line %q", ErrFormat, line)
		}
		amount, err := strconv.Atoi(strings.TrimSpace(f[2]))
		code := strings.TrimSpace(f[3])
		rate := strings.TrimSpace(f[4])
		if err != nil || amount <= 0 || !codeRe.MatchString(code) || !rateRe.MatchString(rate) {
			return nil, fmt.Errorf("%w: line %q", ErrFormat, line)
		}
		l.Rates = append(l.Rates, Rate{Currency: code, Amount: amount, Rate: strings.Replace(rate, ",", ".", 1)})
	}
	if len(l.Rates) == 0 {
		return nil, fmt.Errorf("%w: no rates", ErrFormat)
	}
	return l, nil
}

// PerUnit divides a rate quoted for amount units down to one unit and formats
// it with at least 3 and at most 12 decimal places ("13.546", 100 → "0.13546").
func PerUnit(rate string, amount int) (string, error) {
	r, ok := new(big.Rat).SetString(rate)
	if !ok || amount <= 0 {
		return "", fmt.Errorf("cnb: invalid rate %q / %d", rate, amount)
	}
	r.Quo(r, new(big.Rat).SetInt64(int64(amount)))
	for dp := 3; dp < 12; dp++ {
		s := r.FloatString(dp)
		if back, _ := new(big.Rat).SetString(s); back.Cmp(r) == 0 {
			return s, nil
		}
	}
	return r.FloatString(12), nil
}

// Fetcher returns the list valid on date (YYYY-MM-DD); the list's own Date
// may be earlier (weekend, holiday, not yet published).
type Fetcher interface {
	Fetch(ctx context.Context, date string) (*List, error)
}

// Client downloads the lists over HTTP.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for baseURL ("" = DefaultBaseURL) with a 10 s timeout.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Fetch downloads the list valid on date.
func (c *Client) Fetch(ctx context.Context, date string) (*List, error) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return nil, ErrInvalidDate
	}
	sep := "?"
	if strings.Contains(c.BaseURL, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+sep+"date="+d.Format("02.01.2006"), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cnb: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cnb: unexpected status %d", res.StatusCode)
	}
	return ParseList(io.LimitReader(res.Body, 1<<20))
}
