// Package eet is a client of the Czech registration of sales "EET 2.0"
// (zákon č. 180/2026 Sb., data interface 4.1): one synchronous SOAP 1.1
// operation OdeslaniTrzby whose Body is signed with the taxpayer's
// cash-register certificate (WS-Security, XML-DSig RSA-SHA256, exclusive
// C14N). The tax authority answers with a confirmation code (POK) or an
// error; see SPEC §7.19 and the documents at https://eet.gov.cz/cs/pro-vyvojare.
//
// The package knows nothing about invoices: callers hand it a Sale and keep
// the retry state themselves (a resent sale keeps its data and gets a new
// UUID, sending time and first=false).
package eet

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Prague without system tzdata
)

// Environments of the tax authority's receiving system.
const (
	EnvPlayground = "playground" // developers only; its POK ends with "-ff" and is not valid
	EnvProduction = "production"

	PlaygroundURL = "https://pg.trzbyeet.gov.cz/eet/services/EETServiceSOAP/v4"
	ProductionURL = "https://trzbyeet.gov.cz/eet/services/EETServiceSOAP/v4"

	soapAction = "http://fs.gov.cz/eet/OdeslaniTrzby"
	// MaxMessageBytes is the documented limit of a whole message (critical check 5).
	MaxMessageBytes = 12 * 1024
	// MaxAmount: amounts are below 100 million CZK in absolute value (minor units).
	MaxAmount = 100_000_000_00 - 1

	// Timeout of one attempt. The law's response limit ("mezní doba odezvy")
	// is at least 2 s; afterwards the sale is sent again later.
	Timeout = 10 * time.Second
	// Deadline is the legal limit for a sale that could not be sent at once.
	Deadline = 48 * time.Hour
)

// ValidEnv reports whether env is a known environment.
func ValidEnv(env string) bool { return env == EnvPlayground || env == EnvProduction }

var (
	prague, _ = time.LoadLocation("Europe/Prague")

	eicRe      = regexp.MustCompile(`^CZ[0-9]{8,10}$`)
	unitRe     = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	registerRe = regexp.MustCompile(`^[0-9a-zA-Z.,:;/#\-_ ]{1,20}$`)
	serialRe   = regexp.MustCompile(`^[0-9a-zA-Z.,:;/#\-_ ]{1,25}$`)
	uuidRe     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

// ValidEIC: "evidenční identifikační číslo" = CZ + 8–10 digits (the DIČ; a
// person without one uses CZ + birth number or the identifier from DIS+).
func ValidEIC(s string) bool { return eicRe.MatchString(s) }

// ValidUnitID: the number of the "evidenční jednotka" assigned in DIS+ (1–999999999).
func ValidUnitID(s string) bool { return unitRe.MatchString(s) }

// ValidRegisterID: the taxpayer's own name of the cash register (id_pokl).
func ValidRegisterID(s string) bool { return registerRe.MatchString(s) }

// ValidSerialNo: the taxpayer's serial number of the sale (porad_cis).
func ValidSerialNo(s string) bool { return serialRe.MatchString(s) }

// Sale is the registered data of one sale (element Data). Together the
// fields identify the sale: a message with the same values is the same sale.
type Sale struct {
	EIC        string    // eic_popl
	UnitID     string    // id_jednotky
	RegisterID string    // id_pokl
	SerialNo   string    // porad_cis
	SoldAt     time.Time // dat_trzby
	Amount     int64     // celk_trzba in haléře; negative = cancellation (storno)
}

// Message is one transmission of a sale (element Hlavicka + Data).
type Message struct {
	UUID   string    // new for every transmission, also for a resend
	SentAt time.Time // dat_odesl
	First  bool      // prvni_zaslani: false when the same sale is sent again
	Verify bool      // overeni: only checks the message, registers nothing
	Sale   Sale
}

// Validate checks the message against the formats of the XML schema.
func (m Message) Validate() error {
	s := m.Sale
	switch {
	case !uuidRe.MatchString(m.UUID):
		return errors.New("eet: invalid message UUID")
	case m.SentAt.IsZero() || s.SoldAt.IsZero():
		return errors.New("eet: missing date")
	case !ValidEIC(s.EIC):
		return errors.New("eet: invalid EIC")
	case !ValidUnitID(s.UnitID):
		return errors.New("eet: invalid unit id")
	case !ValidRegisterID(s.RegisterID):
		return errors.New("eet: invalid register id")
	case !ValidSerialNo(s.SerialNo):
		return errors.New("eet: invalid serial number")
	case s.Amount > MaxAmount || s.Amount < -MaxAmount:
		return errors.New("eet: amount out of range")
	}
	return nil
}

// FormatAmount renders minor units as the schema's decimal: exactly two
// decimals, no leading zeros, no negative zero ("250.00", "-187.20", "0.56").
func FormatAmount(minor int64) string {
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/100, minor%100)
}

// FormatTime renders an instant as Czech local time with its zone offset
// ("2027-01-09T04:25:28+01:00").
func FormatTime(t time.Time) string { return t.In(prague).Format("2006-01-02T15:04:05-07:00") }

// Location is Europe/Prague.
func Location() *time.Location { return prague }

// Warning is the outcome of a failed non-critical check; the sale is
// registered anyway.
type Warning struct {
	Code int    `json:"code"`
	Text string `json:"text"`
}

// Result is the tax authority's answer to one message.
type Result struct {
	// POK is the confirmation code of a registered sale ("" otherwise).
	POK string
	// Test: the answer came from a non-production environment.
	Test bool
	// Verified: a message sent with Verify passed all critical checks (error code 0).
	Verified bool
	// ErrCode/ErrText: the message was refused (critical error > 0) or could
	// not be processed now (code < 0, send it again later).
	ErrCode  int
	ErrText  string
	Warnings []Warning
	// TransactionID is the X-Global-Transaction-Id header for support requests.
	TransactionID string
}

// Confirmed: the sale is registered.
func (r Result) Confirmed() bool { return r.POK != "" }

// Rejected: the message has a critical error; sending the same data again
// cannot succeed until the cause (certificate, EIC …) is fixed.
func (r Result) Rejected() bool { return !r.Confirmed() && !r.Verified && r.ErrCode > 0 }

// Sender sends messages (implemented by *Client; faked in tests).
type Sender interface {
	Send(ctx context.Context, env string, cert *Certificate, m Message) (Result, error)
}

// Client talks to the receiving system over HTTPS.
type Client struct {
	// URLs by environment; New fills in the official endpoints.
	URLs map[string]string
	HTTP *http.Client
}

// New returns a client of the official endpoints. Connections are not
// reused: production uses DNS balancing and requires a fresh lookup for
// every connection.
func New() *Client {
	return &Client{
		URLs: map[string]string{EnvPlayground: PlaygroundURL, EnvProduction: ProductionURL},
		HTTP: &http.Client{Timeout: Timeout, Transport: &http.Transport{
			DisableKeepAlives:   true,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: Timeout,
			ForceAttemptHTTP2:   false, // HTTP/1.1 is mandatory
			TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
		}},
	}
}

// Send signs and posts one message. A returned error means "no answer"
// (network, timeout, HTTP error, unreadable answer): the sale stays
// unregistered and must be sent again. Otherwise the Result says whether it
// was confirmed, verified or refused.
func (c *Client) Send(ctx context.Context, env string, cert *Certificate, m Message) (Result, error) {
	url := c.URLs[env]
	if url == "" {
		return Result{}, fmt.Errorf("eet: unknown environment %q", env)
	}
	body, err := Envelope(cert, m)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"`+soapAction+`"`)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("eet: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return Result{}, fmt.Errorf("eet: reading the answer: %w", err)
	}
	out, err := parseResponse(raw)
	if err != nil {
		return Result{}, fmt.Errorf("eet: HTTP %d: %w", res.StatusCode, err)
	}
	out.TransactionID = res.Header.Get("X-Global-Transaction-Id")
	if len(out.TransactionID) > 64 {
		out.TransactionID = out.TransactionID[:64]
	}
	if out.Confirmed() && !m.Verify && (env == EnvProduction) == out.Test {
		// a production POK must not carry the test flag and vice versa
		return Result{}, errors.New("eet: the answer does not match the environment")
	}
	return out, nil
}

type soapEnvelope struct {
	Body struct {
		Odpoved *struct {
			Potvrzeni *struct {
				POK  string `xml:"pok,attr"`
				Test bool   `xml:"test,attr"`
			} `xml:"http://fs.gov.cz/eet/schema/v4 Potvrzeni"`
			Chyba *struct {
				Kod  string `xml:"kod,attr"`
				Test bool   `xml:"test,attr"`
				Text string `xml:",chardata"`
			} `xml:"http://fs.gov.cz/eet/schema/v4 Chyba"`
			Varovani []struct {
				Kod  int    `xml:"kod_varov,attr"`
				Text string `xml:",chardata"`
			} `xml:"http://fs.gov.cz/eet/schema/v4 Varovani"`
		} `xml:"http://fs.gov.cz/eet/schema/v4 Odpoved"`
		Fault *struct {
			String string `xml:"faultstring"`
		} `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`
	} `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

var pokRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}-[0-9a-fA-F]{2}$`)

func parseResponse(raw []byte) (Result, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return Result{}, errors.New("the answer is not a SOAP message")
	}
	if f := env.Body.Fault; f != nil {
		return Result{}, fmt.Errorf("SOAP fault: %s", clip(f.String))
	}
	o := env.Body.Odpoved
	if o == nil || (o.Potvrzeni == nil) == (o.Chyba == nil) {
		return Result{}, errors.New("the answer has no confirmation or error")
	}
	var out Result
	for _, w := range o.Varovani {
		out.Warnings = append(out.Warnings, Warning{Code: w.Kod, Text: clip(w.Text)})
	}
	if p := o.Potvrzeni; p != nil {
		if !pokRe.MatchString(p.POK) {
			return Result{}, errors.New("the answer has an invalid confirmation code")
		}
		out.POK, out.Test = strings.ToLower(p.POK), p.Test
		return out, nil
	}
	code, err := strconv.Atoi(strings.TrimSpace(o.Chyba.Kod))
	if err != nil {
		return Result{}, errors.New("the answer has an invalid error code")
	}
	out.ErrCode, out.ErrText, out.Test = code, clip(o.Chyba.Text), o.Chyba.Test
	out.Verified = code == 0
	return out, nil
}

// clip trims a text of the answer to the documented 100 characters.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
