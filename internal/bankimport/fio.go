package bankimport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Fio banka API (https://www.fio.cz/bankovni-sluzby/api-bankovnictvi):
//
//	GET {base}/periods/{token}/{YYYY-MM-DD}/{YYYY-MM-DD}/transactions.json
//	GET {base}/last/{token}/transactions.json   (since the last download mark)
//	GET {base}/set-last-date/{token}/{YYYY-MM-DD}/
//
// The token is a 64-character secret. Fio allows one request per token per
// 30 seconds and answers 409 Conflict otherwise; an invalid or expired token
// yields 500 with an empty body.

// FioBaseURL is the production API base.
const FioBaseURL = "https://fioapi.fio.cz/v1/rest"

// FioMinInterval is Fio's limit: one request per token per 30 s.
const FioMinInterval = 30 * time.Second

var (
	// ErrFioRateLimited: less than 30 s since the previous request with the token (HTTP 409).
	ErrFioRateLimited = errors.New("fio: rate limited, only one request per 30 s per token is allowed")
	// ErrFioToken: the token is malformed or Fio rejected it (HTTP 500/401/403, the way Fio reports bad tokens).
	ErrFioToken = errors.New("fio: token is invalid, expired or lacks permissions")
	// ErrFioTooMany: the period holds more transactions than Fio returns at once (HTTP 413).
	ErrFioTooMany = errors.New("fio: too many transactions, request a shorter period")
)

// Fio is the Fio banka API (implemented by FioClient; tests use fakes).
type Fio interface {
	Periods(ctx context.Context, token, from, to string) (*Statement, error)
	Last(ctx context.Context, token string) (*Statement, error)
	SetLastDate(ctx context.Context, token, date string) error
}

// FioClient calls the Fio API over HTTP. It also enforces the 30 s
// per-token interval locally, so a sync button pressed twice fails fast with
// ErrFioRateLimited instead of burning Fio's limit.
type FioClient struct {
	BaseURL     string
	HTTP        *http.Client
	MinInterval time.Duration    // default FioMinInterval; 0 after construction disables the local limit
	Now         func() time.Time // default time.Now

	mu   sync.Mutex
	last map[string]time.Time
}

// NewFioClient returns a client for baseURL ("" = FioBaseURL) with a 60 s timeout.
func NewFioClient(baseURL string) *FioClient {
	if baseURL == "" {
		baseURL = FioBaseURL
	}
	return &FioClient{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 60 * time.Second},
		MinInterval: FioMinInterval, Now: time.Now, last: map[string]time.Time{}}
}

var fioTokenRe = regexp.MustCompile(`^[A-Za-z0-9]{16,128}$`)

// Periods returns the transactions booked between from and to (YYYY-MM-DD, inclusive).
func (c *FioClient) Periods(ctx context.Context, token, from, to string) (*Statement, error) {
	for _, d := range []string{from, to} {
		if _, err := time.Parse(time.DateOnly, d); err != nil {
			return nil, fmt.Errorf("fio: invalid date %q", d)
		}
	}
	body, err := c.get(ctx, token, "/periods/%s/"+from+"/"+to+"/transactions.json")
	if err != nil {
		return nil, err
	}
	return ParseFioJSON(body)
}

// Last returns the transactions since the last download mark and moves the mark.
func (c *FioClient) Last(ctx context.Context, token string) (*Statement, error) {
	body, err := c.get(ctx, token, "/last/%s/transactions.json")
	if err != nil {
		return nil, err
	}
	return ParseFioJSON(body)
}

// SetLastDate moves the download mark so that Last returns transactions after date.
func (c *FioClient) SetLastDate(ctx context.Context, token, date string) error {
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return fmt.Errorf("fio: invalid date %q", date)
	}
	_, err := c.get(ctx, token, "/set-last-date/%s/"+date+"/")
	return err
}

// get performs the request; pathFmt contains one %s for the token. Errors
// never contain the token.
func (c *FioClient) get(ctx context.Context, token, pathFmt string) ([]byte, error) {
	if !fioTokenRe.MatchString(token) {
		return nil, fmt.Errorf("%w: malformed token", ErrFioToken)
	}
	if err := c.reserve(token); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+fmt.Sprintf(pathFmt, token), nil)
	if err != nil {
		return nil, errors.New("fio: cannot build request")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // *url.Error would print the URL including the token
		}
		return nil, fmt.Errorf("fio: request failed: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("fio: read: %v", err)
	}
	switch res.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusConflict:
		return nil, ErrFioRateLimited
	case http.StatusInternalServerError, http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%w (HTTP %d)", ErrFioToken, res.StatusCode)
	case http.StatusRequestEntityTooLarge:
		return nil, ErrFioTooMany
	default:
		return nil, fmt.Errorf("fio: unexpected status %d", res.StatusCode)
	}
}

func (c *FioClient) reserve(token string) error {
	if c.MinInterval <= 0 {
		return nil
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		c.last = map[string]time.Time{}
	}
	t := now()
	if prev, ok := c.last[token]; ok && t.Sub(prev) < c.MinInterval {
		return fmt.Errorf("%w (retry in %s)", ErrFioRateLimited, (c.MinInterval - t.Sub(prev)).Round(time.Second))
	}
	c.last[token] = t
	return nil
}

// fioValue is {"value": …, "name": "…", "id": N}; value is a string or a number.
type fioValue struct {
	Value json.RawMessage `json:"value"`
}

func (v *fioValue) String() string {
	if v == nil || len(v.Value) == 0 || string(v.Value) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(v.Value)) // number, kept verbatim (no float conversion)
}

type fioResponse struct {
	AccountStatement *struct {
		Info struct {
			AccountID      string      `json:"accountId"`
			BankID         string      `json:"bankId"`
			Currency       string      `json:"currency"`
			IBAN           string      `json:"iban"`
			OpeningBalance json.Number `json:"openingBalance"`
			ClosingBalance json.Number `json:"closingBalance"`
		} `json:"info"`
		TransactionList struct {
			Transaction []map[string]*fioValue `json:"transaction"`
		} `json:"transactionList"`
	} `json:"accountStatement"`
}

// ParseFioJSON parses a Fio API transactions.json response.
//
// Columns used: 22 ID pohybu, 0 Datum, 1 Objem, 14 Měna, 2 Protiúčet,
// 3 Kód banky, 10 Název protiúčtu, 5 VS, 4 KS, 6 SS, 16 Zpráva pro příjemce
// (fallback 7 Uživatelská identifikace).
func ParseFioJSON(data []byte) (*Statement, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var r fioResponse
	if err := dec.Decode(&r); err != nil || r.AccountStatement == nil {
		return nil, fmt.Errorf("%w: not a Fio transactions.json", ErrFormat)
	}
	info := r.AccountStatement.Info
	st := &Statement{IBAN: info.IBAN, Currency: info.Currency, Transactions: []Transaction{}}
	if info.AccountID != "" {
		st.Account = FormatAccount(info.AccountID, info.BankID)
	}
	if info.OpeningBalance != "" {
		if v, err := fioAmount(info.OpeningBalance.String(), info.Currency); err == nil {
			st.OpeningBalance = &v
		}
	}
	if info.ClosingBalance != "" {
		if v, err := fioAmount(info.ClosingBalance.String(), info.Currency); err == nil {
			st.ClosingBalance = &v
		}
	}
	for i, row := range r.AccountStatement.TransactionList.Transaction {
		col := func(n int) string { return row[fmt.Sprintf("column%d", n)].String() }
		t := Transaction{
			ExternalID:       col(22),
			Currency:         strings.ToUpper(col(14)),
			CounterpartyName: squash(col(10)),
			VS:               symbol(col(5)),
			KS:               symbol(col(4)),
			SS:               symbol(col(6)),
			Message:          squash(col(16)),
		}
		if t.Currency == "" {
			t.Currency = info.Currency
		}
		if t.Message == "" {
			t.Message = squash(col(7))
		}
		bank := col(3)
		if len(bank) != 4 || !allDigits(bank) {
			bank = "" // foreign payments carry a BIC here
		}
		t.CounterpartyAccount = FormatAccount(col(2), bank)
		var err error
		if t.BookedOn, err = ParseDate(col(0), ""); err != nil {
			return nil, fmt.Errorf("%w: Fio transaction %d: %v", ErrFormat, i+1, err)
		}
		if t.Amount, err = fioAmount(col(1), t.Currency); err != nil {
			return nil, fmt.Errorf("%w: Fio transaction %d: %v", ErrFormat, i+1, err)
		}
		st.Transactions = append(st.Transactions, t)
	}
	fillHashIDs(st.Transactions)
	return st, nil
}

// fioAmount parses a JSON number; Fio (Java doubles) may use exponent
// notation for large values ("1.0E7"), which is expanded exactly.
func fioAmount(s, currency string) (int64, error) {
	if strings.ContainsAny(s, "eE") {
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			return 0, fmt.Errorf("bankimport: invalid amount %q", s)
		}
		s = r.FloatString(minorDigits(currency) + 3)
	}
	return ParseAmount(s, '.', currency)
}
