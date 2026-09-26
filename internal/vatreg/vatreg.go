// Package vatreg queries the Czech register of VAT payers ("Registr plátců
// DPH", Ministry of Finance / GFŘ): whether a payer is unreliable
// ("nespolehlivý plátce") and which bank accounts it has published.
//
// It uses the public SOAP service rozhraniCRPDPH, operation
// getStatusNespolehlivyPlatceRozsireny (the "extended" variant that also
// returns the payer's name and address).
package vatreg

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/qwerin/nanofaktura/internal/spayd"
	"github.com/qwerin/nanofaktura/internal/ttlcache"
)

// DefaultURL is the production SOAP endpoint (from the service WSDL).
const DefaultURL = "https://mojedane.gov.cz/dpr/axis2/services/rozhraniCRPDPH.rozhraniCRPDPHSOAP"

const soapAction = "http://adis.mfcr.cz/rozhraniCRPDPH/getStatusNespolehlivyPlatceRozsireny"

var (
	// ErrInvalidDIC: not a Czech DIČ (optional "CZ" + 8–10 digits).
	ErrInvalidDIC = errors.New("invalid Czech VAT number (DIČ)")
	// ErrUnavailable: the registry did not answer (outage, maintenance 0:00–0:10 …).
	ErrUnavailable = errors.New("VAT payer registry is unavailable")
)

// Account is a bank account the payer published in the registry.
type Account struct {
	Number      string // "35-3355550267/0100" for Czech accounts, else as published (usually IBAN)
	IBAN        string // "" when it cannot be derived
	PublishedOn string // YYYY-MM-DD
}

// Result describes one DIČ.
type Result struct {
	VatNo           string // "CZ27082440"
	Registered      bool   // false = the DIČ is not in the register of VAT payers
	Reliable        *bool  // nil when not registered
	UnreliableSince string // YYYY-MM-DD, only when Reliable is false
	TaxOffice       string // "cisloFu", number of the tax office
	Name            string
	Street          string
	CityPart        string
	City            string
	Zip             string
	Country         string // as published, e.g. "Česká republika"
	Accounts        []Account
}

// Address formats the address on one line ("Jankovcova 1522/53, 170 00 Praha 7").
func (r Result) Address() string {
	var parts []string
	if r.Street != "" {
		parts = append(parts, r.Street)
	}
	zc := strings.TrimSpace(formatZip(r.Zip) + " " + r.City)
	if zc != "" {
		parts = append(parts, zc)
	}
	return strings.Join(parts, ", ")
}

func formatZip(z string) string {
	if len(z) == 5 {
		return z[:3] + " " + z[3:]
	}
	return z
}

// HasAccount reports whether account (Czech "prefix-number/bank" or IBAN) is
// among the published accounts.
func (r Result) HasAccount(account string) bool {
	a := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(account), " ", ""))
	if a == "" {
		return false
	}
	if p, err := spayd.ParseAccount(a); err == nil {
		a = p.IBAN()
	}
	for _, pa := range r.Accounts {
		if pa.IBAN == a || strings.EqualFold(pa.Number, a) {
			return true
		}
	}
	return false
}

var dicRe = regexp.MustCompile(`^\d{8,10}$`)

// NormalizeDIC strips spaces and the "CZ" prefix and checks the digits.
func NormalizeDIC(s string) (string, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	s = strings.TrimPrefix(s, "CZ")
	if !dicRe.MatchString(s) {
		return "", ErrInvalidDIC
	}
	return s, nil
}

// Checker looks up a DIČ.
type Checker interface {
	Check(ctx context.Context, dic string) (*Result, error)
}

// Client calls the SOAP service.
type Client struct {
	URL  string
	HTTP *http.Client
}

// New returns a client for url ("" = DefaultURL) with a 15 s timeout.
func New(url string) *Client {
	if url == "" {
		url = DefaultURL
	}
	return &Client{URL: url, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

const requestTmpl = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:roz="http://adis.mfcr.cz/rozhraniCRPDPH/">` +
	`<soapenv:Body><roz:StatusNespolehlivyPlatceRozsirenyRequest><roz:dic>%s</roz:dic></roz:StatusNespolehlivyPlatceRozsirenyRequest></soapenv:Body>` +
	`</soapenv:Envelope>`

// Check looks up dic. Errors: ErrInvalidDIC, ErrUnavailable (wrapped).
func (c *Client) Check(ctx context.Context, dic string) (*Result, error) {
	dic, err := NormalizeDIC(dic)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, strings.NewReader(fmt.Sprintf(requestTmpl, dic)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"`+soapAction+`"`)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, res.StatusCode)
	}
	results, err := ParseResponse(body)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if r.VatNo == "CZ"+dic {
			return r, nil
		}
	}
	return nil, fmt.Errorf("%w: DIČ missing in the response", ErrUnavailable)
}

type envelope struct {
	Body struct {
		Fault *struct {
			String string `xml:"faultstring"`
		} `xml:"Fault"`
		Response *struct {
			Status struct {
				Code int    `xml:"statusCode,attr"`
				Text string `xml:"statusText,attr"`
			} `xml:"status"`
			Payers []payer `xml:"statusPlatceDPH"`
		} `xml:"StatusNespolehlivyPlatceRozsirenyResponse"`
	} `xml:"Body"`
}

type payer struct {
	DIC             string `xml:"dic,attr"`
	Unreliable      string `xml:"nespolehlivyPlatce,attr"` // ANO | NE | NENALEZEN
	UnreliableSince string `xml:"datumZverejneniNespolehlivosti,attr"`
	TaxOffice       string `xml:"cisloFu,attr"`
	Accounts        []struct {
		PublishedOn string `xml:"datumZverejneni,attr"`
		Standard    *struct {
			Prefix string `xml:"predcisli,attr"`
			Number string `xml:"cislo,attr"`
			Bank   string `xml:"kodBanky,attr"`
		} `xml:"standardniUcet"`
		Other *struct {
			Number string `xml:"cislo,attr"`
		} `xml:"nestandardniUcet"`
	} `xml:"zverejneneUcty>ucet"`
	Name    string `xml:"nazevSubjektu"`
	Address struct {
		Street   string `xml:"uliceCislo"`
		CityPart string `xml:"castObce"`
		City     string `xml:"mesto"`
		Zip      string `xml:"psc"`
		Country  string `xml:"stat"`
	} `xml:"adresa"`
}

// ParseResponse parses a getStatusNespolehlivyPlatceRozsireny SOAP response.
// A SOAP fault or a non-zero status code yields ErrUnavailable.
func ParseResponse(body []byte) ([]*Result, error) {
	var env envelope
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&env); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	if f := env.Body.Fault; f != nil {
		return nil, fmt.Errorf("%w: SOAP fault: %s", ErrUnavailable, strings.TrimSpace(f.String))
	}
	resp := env.Body.Response
	if resp == nil {
		return nil, fmt.Errorf("%w: unexpected response", ErrUnavailable)
	}
	if resp.Status.Code != 0 && resp.Status.Code != 1 { // 1 = OK, only first 100 DIČ returned
		return nil, fmt.Errorf("%w: status %d %s", ErrUnavailable, resp.Status.Code, resp.Status.Text)
	}
	out := make([]*Result, 0, len(resp.Payers))
	for _, p := range resp.Payers {
		out = append(out, p.result())
	}
	return out, nil
}

func (p payer) result() *Result {
	r := &Result{VatNo: "CZ" + strings.TrimSpace(p.DIC), Accounts: []Account{}}
	switch p.Unreliable {
	case "NE", "ANO":
		reliable := p.Unreliable == "NE"
		r.Registered, r.Reliable = true, &reliable
		if !reliable {
			r.UnreliableSince = p.UnreliableSince
		}
	default: // NENALEZEN
		return r
	}
	r.TaxOffice = p.TaxOffice
	r.Name = squash(p.Name)
	r.Street, r.CityPart, r.City = squash(p.Address.Street), squash(p.Address.CityPart), squash(p.Address.City)
	r.Zip, r.Country = strings.ReplaceAll(squash(p.Address.Zip), " ", ""), squash(p.Address.Country)
	for _, a := range p.Accounts {
		acc := Account{PublishedOn: a.PublishedOn}
		switch {
		case a.Standard != nil:
			acc.Number = a.Standard.Number + "/" + a.Standard.Bank
			if pre := strings.TrimLeft(a.Standard.Prefix, "0"); pre != "" {
				acc.Number = pre + "-" + acc.Number
			}
			if pa, err := spayd.ParseAccount(acc.Number); err == nil {
				acc.IBAN = pa.IBAN()
			}
		case a.Other != nil:
			acc.Number = strings.ReplaceAll(strings.TrimSpace(a.Other.Number), " ", "")
			if spayd.ValidIBAN(acc.Number) {
				acc.IBAN = spayd.NormalizeIBAN(acc.Number)
			}
		default:
			continue
		}
		r.Accounts = append(r.Accounts, acc)
	}
	return r
}

// squash trims and collapses runs of white space (the registry pads names).
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// Cached wraps a Checker with an in-memory cache; errors are not cached.
type Cached struct {
	inner Checker
	cache *ttlcache.Cache[string, Result]
}

// NewCached caches results of inner for ttl; now may be nil (time.Now).
func NewCached(inner Checker, ttl time.Duration, now func() time.Time) *Cached {
	return &Cached{inner: inner, cache: ttlcache.New[string, Result](ttl, now)}
}

// Check implements Checker.
func (c *Cached) Check(ctx context.Context, dic string) (*Result, error) {
	dic, err := NormalizeDIC(dic)
	if err != nil {
		return nil, err
	}
	if r, ok := c.cache.Get(dic); ok {
		return &r, nil
	}
	r, err := c.inner.Check(ctx, dic)
	if err != nil {
		return nil, err
	}
	c.cache.Set(dic, *r)
	return r, nil
}
