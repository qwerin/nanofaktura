// Package ares poskytuje klienta pro veřejné REST API ARES (Administrativní registr
// ekonomických subjektů) Ministerstva financí ČR.
// Endpoint: https://ares.gov.cz/ekonomicke-subjekty-v-be/rest/ekonomicke-subjekty/{ico}
package ares

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const aresBaseURL = "https://ares.gov.cz/ekonomicke-subjekty-v-be/rest/ekonomicke-subjekty"

// ErrNotFound je vrácen, pokud ARES subjekt s daným IČO nezná.
var ErrNotFound = errors.New("ares: subjekt nenalezen")

// Lookup jsou data vrácená z ARES pro jedno IČO.
// Mapujeme pouze pole, která jsou potřeba pro předvyplnění faktury.
type Lookup struct {
	IC          string `json:"ic"`
	Name        string `json:"name"`
	DIC         string `json:"dic"`          // může být prázdné pro neplátce
	Street      string `json:"street"`       // sestavujeme z adresních komponent
	City        string `json:"city"`
	ZIP         string `json:"zip"`
	CountryCode string `json:"country_code"` // vždy "CZ" pro tuzemské subjekty
}

// aresResponse je interní struct pro parsování JSON odpovědi ARES.
// Pozor: ARES vrací číselná pole (psc, cisloDomovni, cisloOrientacni) jako JSON number, ne string.
type aresResponse struct {
	ICO           string `json:"ico"`
	ObchodniJmeno string `json:"obchodniJmeno"`
	DIC           string `json:"dic"`
	Sidlo         struct {
		PSC             int    `json:"psc"`
		ObecNazev       string `json:"nazevObce"`
		UliceNazev      string `json:"nazevUlice"`
		CisloDomovni    int    `json:"cisloDomovni"`
		CisloOrientacni int    `json:"cisloOrientacni"`
	} `json:"sidlo"`
}

// Client je HTTP klient pro ARES API s nastaveným timeoutem.
type Client struct {
	http *http.Client
}

// New vytvoří nový ARES klienta. Pokud není předán vlastní http.Client,
// použije se výchozí s rozumným timeoutem.
func New(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{http: hc}
}

// FetchByIC stáhne a vrátí data subjektu z ARES podle IČO.
// IČO musí být 8místné číslo (s případnými vedoucími nulami).
// Vrací ErrNotFound pokud ARES IČO nezná.
func (c *Client) FetchByIC(ctx context.Context, ic string) (*Lookup, error) {
	if len(ic) != 8 {
		return nil, fmt.Errorf("ares: IČO musí mít přesně 8 znaků, dostali jsme %q", ic)
	}

	url := fmt.Sprintf("%s/%s", aresBaseURL, ic)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ares: sestavení požadavku: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ares: HTTP požadavek: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ares: neočekávaný status %d", resp.StatusCode)
	}

	var raw aresResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("ares: parsování odpovědi: %w", err)
	}

	return mapToLookup(raw), nil
}

// mapToLookup převede surovou ARES odpověď na naši doménovou strukturu.
func mapToLookup(r aresResponse) *Lookup {
	street := r.Sidlo.UliceNazev
	if r.Sidlo.CisloDomovni != 0 {
		street += fmt.Sprintf(" %d", r.Sidlo.CisloDomovni)
	}
	if r.Sidlo.CisloOrientacni != 0 {
		street += fmt.Sprintf("/%d", r.Sidlo.CisloOrientacni)
	}
	// Pokud ulice chybí, použijeme jen název obce (typické pro vesnice)
	if street == "" {
		street = r.Sidlo.ObecNazev
	}

	return &Lookup{
		IC:          r.ICO,
		Name:        r.ObchodniJmeno,
		DIC:         r.DIC,
		Street:      street,
		City:        r.Sidlo.ObecNazev,
		ZIP:         fmt.Sprintf("%05d", r.Sidlo.PSC),
		CountryCode: "CZ",
	}
}
