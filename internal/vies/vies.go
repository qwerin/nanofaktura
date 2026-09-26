// Package vies checks EU VAT numbers in VIES (VAT Information Exchange
// System) through its public REST API:
//
//	GET {base}/ms/{countryCode}/vat/{number}
//	→ {"isValid": true, "userError": "VALID", "name": "…", "address": "…", …}
//
// Member states that do not disclose trader details answer "---" for name and
// address. userError values other than VALID/INVALID mean the check could not
// be made (member state unavailable, rate limits …).
package vies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/qwerin/nanofaktura/internal/ttlcache"
)

// DefaultBaseURL is the production VIES REST API.
const DefaultBaseURL = "https://ec.europa.eu/taxation_customs/vies/rest-api"

var (
	// ErrInvalidInput: the VAT number is malformed or the country is not an EU member state.
	ErrInvalidInput = errors.New("invalid EU VAT number")
	// ErrUnavailable: VIES or the member state's system could not answer.
	ErrUnavailable = errors.New("VIES is unavailable")
)

// Result of a VIES check. Name and Address are empty when the member state
// does not disclose them.
type Result struct {
	CountryCode string // VIES member state code (EL for Greece, XI for Northern Ireland)
	VatNumber   string // the number without the country prefix
	Valid       bool
	Name        string
	Address     string // lines separated by "\n"
}

// VatNo returns the full VAT number with its country prefix ("CZ27082440").
func (r Result) VatNo() string { return r.CountryCode + r.VatNumber }

// memberStates are the VIES country codes.
var memberStates = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DE": true, "DK": true, "EE": true,
	"EL": true, "ES": true, "FI": true, "FR": true, "HR": true, "HU": true, "IE": true, "IT": true,
	"LT": true, "LU": true, "LV": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true,
	"SE": true, "SI": true, "SK": true, "XI": true,
}

var numberRe = regexp.MustCompile(`^[0-9A-Z+*]{2,12}$`)

// Split normalizes a VAT number ("cz 270 824 40", "GR094014201") into the VIES
// country code and the national number. Errors wrap ErrInvalidInput.
func Split(vatNo string) (country, number string, err error) {
	s := strings.ToUpper(strings.NewReplacer(" ", "", ".", "", "-", "", "\u00a0", "").Replace(strings.TrimSpace(vatNo)))
	if len(s) < 4 {
		return "", "", fmt.Errorf("%w: too short", ErrInvalidInput)
	}
	country, number = s[:2], s[2:]
	if country == "GR" {
		country = "EL"
	}
	if !memberStates[country] {
		return "", "", fmt.Errorf("%w: %q is not an EU member state code", ErrInvalidInput, country)
	}
	if !numberRe.MatchString(number) {
		return "", "", fmt.Errorf("%w: malformed number", ErrInvalidInput)
	}
	return country, number, nil
}

// Checker validates VAT numbers.
type Checker interface {
	Check(ctx context.Context, vatNo string) (*Result, error)
}

// Client queries VIES over HTTP.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for baseURL ("" = DefaultBaseURL) with a 15 s timeout.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 15 * time.Second}}
}

type response struct {
	IsValid       bool   `json:"isValid"`
	UserError     string `json:"userError"`
	Name          string `json:"name"`
	Address       string `json:"address"`
	VatNumber     string `json:"vatNumber"`
	ErrorWrappers []struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	} `json:"errorWrappers"`
}

// Check validates vatNo (with country prefix). Errors: ErrInvalidInput,
// ErrUnavailable (wrapped with the VIES error code).
func (c *Client) Check(ctx context.Context, vatNo string) (*Result, error) {
	cc, num, err := Split(vatNo)
	if err != nil {
		return nil, err
	}
	u := c.BaseURL + "/ms/" + cc + "/vat/" + url.PathEscape(num)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	var r response
	decErr := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r)
	switch {
	case res.StatusCode == http.StatusBadRequest:
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, wrapperCode(r))
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: status %d %s", ErrUnavailable, res.StatusCode, wrapperCode(r))
	case decErr != nil:
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, decErr)
	}
	switch r.UserError {
	case "VALID", "INVALID":
	case "INVALID_INPUT":
		return nil, fmt.Errorf("%w: rejected by VIES", ErrInvalidInput)
	default: // MS_UNAVAILABLE, TIMEOUT, SERVICE_UNAVAILABLE, *_MAX_CONCURRENT_REQ …
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, r.UserError)
	}
	out := &Result{CountryCode: cc, VatNumber: num, Valid: r.IsValid && r.UserError == "VALID"}
	if r.VatNumber != "" {
		out.VatNumber = r.VatNumber
	}
	if out.Valid {
		out.Name = clean(r.Name)
		out.Address = clean(r.Address)
	}
	return out, nil
}

func wrapperCode(r response) string {
	if len(r.ErrorWrappers) > 0 {
		return r.ErrorWrappers[0].Error
	}
	return ""
}

// clean drops the "---" placeholder and trims every line.
func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "---" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// Cached wraps a Checker with an in-memory cache of definitive answers
// (valid or invalid); errors are never cached.
type Cached struct {
	inner Checker
	cache *ttlcache.Cache[string, Result]
}

// NewCached caches results of inner for ttl; now may be nil (time.Now).
func NewCached(inner Checker, ttl time.Duration, now func() time.Time) *Cached {
	return &Cached{inner: inner, cache: ttlcache.New[string, Result](ttl, now)}
}

// Check implements Checker.
func (c *Cached) Check(ctx context.Context, vatNo string) (*Result, error) {
	cc, num, err := Split(vatNo)
	if err != nil {
		return nil, err
	}
	key := cc + num
	if r, ok := c.cache.Get(key); ok {
		return &r, nil
	}
	r, err := c.inner.Check(ctx, key)
	if err != nil {
		return nil, err
	}
	c.cache.Set(key, *r)
	return r, nil
}
