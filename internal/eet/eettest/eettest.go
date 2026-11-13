// Package eettest fakes the tax authority's receiving system of EET 2.0 and
// makes throw-away cash-register certificates for tests.
package eettest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/qwerin/nanofaktura/internal/eet"
)

// Modes of the fake server.
const (
	ModeOK        = "ok"        // confirms with a POK
	ModeDown      = "down"      // HTTP 503 without a SOAP answer
	ModeTemporary = "temporary" // error -1: send again later
	ModeReject    = "reject"    // error 4: invalid signature
)

// Request is one received, signature-checked message.
type Request struct {
	Production bool
	CertEIC    string
	UUID       string `xml:"-"`
	SentAt     string
	First      bool
	Verify     bool
	EIC        string
	UnitID     string
	RegisterID string
	SerialNo   string
	SoldAt     string
	Amount     string
}

// Server is the fake receiving system; requests with a broken signature get
// error 4 like the real one.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	mode     string
	warn     bool
	requests []Request
}

// Start runs a fake server until the test ends.
func Start(t testing.TB) *Server {
	s := &Server{mode: ModeOK}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

// Client returns an eet.Client whose environments point at the server.
func (s *Server) Client() *eet.Client {
	return &eet.Client{
		URLs: map[string]string{eet.EnvPlayground: s.URL + "/pg", eet.EnvProduction: s.URL + "/prod"},
		HTTP: &http.Client{Timeout: 5 * time.Second},
	}
}

// SetMode switches the behaviour (Mode* constants).
func (s *Server) SetMode(mode string) { s.mu.Lock(); s.mode = mode; s.mu.Unlock() }

// SetWarning makes confirmations carry warning 1 (EIC differs from the certificate).
func (s *Server) SetWarning(on bool) { s.mu.Lock(); s.warn = on; s.mu.Unlock() }

// Requests returns the messages received so far (also the refused ones).
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

type trzba struct {
	Body struct {
		Trzba struct {
			Hlavicka struct {
				UUID   string `xml:"uuid_zpravy,attr"`
				SentAt string `xml:"dat_odesl,attr"`
				First  bool   `xml:"prvni_zaslani,attr"`
				Verify bool   `xml:"overeni,attr"`
			} `xml:"Hlavicka"`
			Data struct {
				EIC        string `xml:"eic_popl,attr"`
				UnitID     string `xml:"id_jednotky,attr"`
				RegisterID string `xml:"id_pokl,attr"`
				SerialNo   string `xml:"porad_cis,attr"`
				SoldAt     string `xml:"dat_trzby,attr"`
				Amount     string `xml:"celk_trzba,attr"`
			} `xml:"Data"`
		} `xml:"http://fs.gov.cz/eet/schema/v4 Trzba"`
	} `xml:"Body"`
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	prod := strings.HasSuffix(r.URL.Path, "/prod")
	s.mu.Lock()
	mode, warn := s.mode, s.warn
	s.mu.Unlock()
	if mode == ModeDown {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	test := ""
	if !prod {
		test = ` test="true"`
	}
	now := eet.FormatTime(time.Now())
	answer := func(inner string) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Header().Set("X-Global-Transaction-Id", "tx-"+uuid.NewString()[:8])
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><eet:Odpoved xmlns:eet="http://fs.gov.cz/eet/schema/v4">%s</eet:Odpoved></soapenv:Body></soapenv:Envelope>`, inner)
	}
	fail := func(code int, text string) {
		answer(fmt.Sprintf(`<eet:Hlavicka dat_odmit="%s"/><eet:Chyba kod="%d"%s>%s</eet:Chyba>`, now, code, test, text))
	}
	var req trzba
	cert, err := eet.VerifyEnvelope(raw)
	if err == nil {
		err = xml.Unmarshal(raw, &req)
	}
	if err != nil || r.Header.Get("SOAPAction") == "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/xml") {
		fail(4, "Neplatny podpis SOAP zpravy")
		return
	}
	h, d := req.Body.Trzba.Hlavicka, req.Body.Trzba.Data
	s.mu.Lock()
	s.requests = append(s.requests, Request{Production: prod, CertEIC: cert.Subject.CommonName, UUID: h.UUID, SentAt: h.SentAt,
		First: h.First, Verify: h.Verify, EIC: d.EIC, UnitID: d.UnitID, RegisterID: d.RegisterID, SerialNo: d.SerialNo,
		SoldAt: d.SoldAt, Amount: d.Amount})
	s.mu.Unlock()
	switch {
	case mode == ModeTemporary:
		fail(-1, "Docasna technicka chyba zpracovani - odeslete prosim datovou zpravu pozdeji")
	case mode == ModeReject:
		fail(4, "Neplatny podpis SOAP zpravy")
	case h.Verify:
		fail(0, "Datovou zpravu evidovane trzby v overovacim modu se podarilo zpracovat")
	default:
		suffix, warning := "01", ""
		if !prod {
			suffix = "ff"
		}
		if warn {
			warning = `<eet:Varovani kod_varov="1">EIC poplatnika v datove zprave se neshoduje s EIC v certifikatu</eet:Varovani>`
		}
		answer(fmt.Sprintf(`<eet:Hlavicka uuid_zpravy="%s" dat_prij="%s"/><eet:Potvrzeni pok="%s-%s"%s/>%s`,
			h.UUID, now, uuid.NewString(), suffix, test, warning))
	}
}

// PKCS12 makes a self-signed cash-register certificate for eic valid until
// notAfter and returns it as a .p12 file protected by password.
func PKCS12(t testing.TB, eic, password string, notAfter time.Time) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: eic, Country: []string{"CZ"}},
		NotBefore:    notAfter.AddDate(-1, 0, -1), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	p12, err := pkcs12.Modern.Encode(key, cert, nil, password)
	if err != nil {
		t.Fatal(err)
	}
	return p12
}
