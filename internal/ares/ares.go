// Package ares is a client of ARES, the Czech register of economic subjects
// (REST API "ekonomicke-subjekty-v-be"), plus IČO validation.
package ares

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production ARES endpoint; the IČO is appended as a path segment.
const DefaultBaseURL = "https://ares.gov.cz/ekonomicke-subjekty-v-be/rest/ekonomicke-subjekty"

var (
	// ErrNotFound: ARES has no subject with the IČO.
	ErrNotFound = errors.New("subject not found in ARES")
	// ErrInvalidICO: the IČO is not 7–8 digits or fails the checksum.
	ErrInvalidICO = errors.New("invalid IČO")
)

// Result is a subject found in ARES, mapped to NanoFaktura's address fields.
type Result struct {
	RegistrationNo string
	VatNo          string
	Name           string
	Street         string
	City           string
	Zip            string
	Country        string // ISO 3166-1 alpha-2
}

var icoRe = regexp.MustCompile(`^\d{7,8}$`)

// NormalizeICO trims spaces, left-pads a 7-digit IČO to 8 digits and checks
// the mod-11 checksum. Errors wrap ErrInvalidICO.
func NormalizeICO(s string) (string, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if !icoRe.MatchString(s) {
		return "", fmt.Errorf("%w: expected 8 digits", ErrInvalidICO)
	}
	if len(s) == 7 {
		s = "0" + s
	}
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(s[i]-'0') * (8 - i)
	}
	if want := (11 - sum%11) % 10; int(s[7]-'0') != want {
		return "", fmt.Errorf("%w: checksum mismatch", ErrInvalidICO)
	}
	return s, nil
}

// Client queries ARES over HTTP.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for baseURL ("" = DefaultBaseURL) with a 10 s timeout.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Lookup fetches the subject with the IČO. Errors: ErrInvalidICO, ErrNotFound,
// otherwise a transport/upstream error (ARES unavailable).
func (c *Client) Lookup(ctx context.Context, ico string) (*Result, error) {
	ico, err := NormalizeICO(ico)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/"+ico, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ares: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound, res.StatusCode == http.StatusBadRequest:
		return nil, ErrNotFound
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("ares: unexpected status %d", res.StatusCode)
	}
	var s subject
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&s); err != nil {
		return nil, fmt.Errorf("ares: decode: %w", err)
	}
	if s.ICO == "" && s.ObchodniJmeno == "" {
		return nil, ErrNotFound
	}
	return s.result(ico), nil
}

// subject is the part of the ARES "EkonomickySubjekt" we use.
type subject struct {
	ICO           string  `json:"ico"`
	ObchodniJmeno string  `json:"obchodniJmeno"`
	DIC           string  `json:"dic"`
	Sidlo         address `json:"sidlo"`
}

type address struct {
	KodStatu                string `json:"kodStatu"`
	NazevObce               string `json:"nazevObce"`
	NazevCastiObce          string `json:"nazevCastiObce"`
	NazevMestskeCastiObvodu string `json:"nazevMestskeCastiObvodu"`
	NazevUlice              string `json:"nazevUlice"`
	CisloDomovni            int    `json:"cisloDomovni"`
	TypCisloDomovni         int    `json:"typCisloDomovni"` // 2 = číslo evidenční
	CisloOrientacni         int    `json:"cisloOrientacni"`
	CisloOrientacniPismeno  string `json:"cisloOrientacniPismeno"`
	PSC                     int    `json:"psc"`
	TextovaAdresa           string `json:"textovaAdresa"`
}

func (s *subject) result(ico string) *Result {
	r := &Result{
		RegistrationNo: ico,
		VatNo:          strings.TrimSpace(s.DIC),
		Name:           strings.TrimSpace(s.ObchodniJmeno),
		Country:        strings.ToUpper(s.Sidlo.KodStatu),
	}
	if r.Country == "" {
		r.Country = "CZ"
	}
	a := s.Sidlo
	if a.NazevObce != "" {
		r.Street = a.street()
		r.City = a.city()
		if a.PSC > 0 {
			r.Zip = fmt.Sprintf("%05d", a.PSC)
		}
	} else {
		r.Street, r.City, r.Zip = parseTextAddress(a.TextovaAdresa)
	}
	return r
}

// street builds "Ulice 12/3a"; in municipalities without streets the part of
// the municipality (or the municipality) stands for the street.
func (a address) street() string {
	name := a.NazevUlice
	if name == "" {
		name = a.NazevCastiObce
	}
	if name == "" {
		name = a.NazevObce
	}
	var num string
	if a.CisloDomovni > 0 {
		num = strconv.Itoa(a.CisloDomovni)
		if a.TypCisloDomovni == 2 {
			num = "č. ev. " + num
		}
	}
	if a.CisloOrientacni > 0 {
		o := strconv.Itoa(a.CisloOrientacni) + a.CisloOrientacniPismeno
		if num == "" {
			num = o
		} else {
			num += "/" + o
		}
	}
	return strings.TrimSpace(name + " " + num)
}

// city prefers the city district when it extends the city name ("Praha 4", "Brno-střed").
func (a address) city() string {
	if d := a.NazevMestskeCastiObvodu; d != "" && strings.HasPrefix(d, a.NazevObce) {
		return d
	}
	return a.NazevObce
}

var zipCityRe = regexp.MustCompile(`^(\d{3}) ?(\d{2})\s+(.+)$`)

// parseTextAddress splits "Ulice 1, Část, 14000 Praha 4" into street, city and zip.
func parseTextAddress(s string) (street, city, zip string) {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 0 || parts[0] == "" {
		return "", "", ""
	}
	last := parts[len(parts)-1]
	if m := zipCityRe.FindStringSubmatch(last); m != nil {
		zip, city = m[1]+m[2], m[3]
		if len(parts) > 1 {
			street = parts[0]
		}
		return street, city, zip
	}
	return parts[0], strings.Join(parts[1:], ", "), ""
}
