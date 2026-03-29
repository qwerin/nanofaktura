// Package ares poskytuje klienta pro veřejné REST API ARES (Administrativní registr
// ekonomických subjektů) Ministerstva financí ČR.
// Endpoint: https://ares.gov.cz/ekonomicke-subjekty-v-be/rest/ekonomicke-subjekty/{ico}
package ares

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const aresBaseURL = "https://ares.gov.cz/ekonomicke-subjekty-v-be/rest/ekonomicke-subjekty"

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
type aresResponse struct {
	ICO            string `json:"ico"`
	ObchodniJmeno  string `json:"obchodniJmeno"`
	DIC            string `json:"dic"`
	Sidlo          struct {
		PSC             string `json:"psc"`
		ObecNazev       string `json:"nazevObce"`
		UliceNazev      string `json:"nazevUlice"`
		CisloDomovniOrientacni string `json:"cisloDomovniOrientacni"`
		CisloOrientacni string `json:"cisloOrientacni"`
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
		return nil, fmt.Errorf("ares: IČO %s nenalezeno", ic)
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
	if r.Sidlo.CisloDomovniOrientacni != "" {
		street += " " + r.Sidlo.CisloDomovniOrientacni
	}
	if r.Sidlo.CisloOrientacni != "" {
		street += "/" + r.Sidlo.CisloOrientacni
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
		ZIP:         r.Sidlo.PSC,
		CountryCode: "CZ",
	}
}
