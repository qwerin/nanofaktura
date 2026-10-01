package reports

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPeriod(t *testing.T) {
	for in, want := range map[string]struct{ from, to string }{
		"2026-09": {"2026-09-01", "2026-09-30"},
		"2026-02": {"2026-02-01", "2026-02-28"},
		"2026-Q3": {"2026-07-01", "2026-09-30"},
		"2026-q1": {"2026-01-01", "2026-03-31"},
		"2028-Q4": {"2028-10-01", "2028-12-31"},
	} {
		p, err := ParsePeriod(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if from, to := p.Range(); from != want.from || to != want.to {
			t.Errorf("%s: %s–%s", in, from, to)
		}
	}
	for _, bad := range []string{"", "2026", "2026-13", "2026-Q5", "26-01", "2026-9"} {
		if _, err := ParsePeriod(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	p, _ := ParsePeriod("2026-Q3")
	if p.String() != "2026-Q3" || !p.Contains("2026-08-31") || p.Contains("2026-10-01") {
		t.Error("String/Contains")
	}
	if got := PreviousPeriod(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), false).String(); got != "2025-12" {
		t.Errorf("previous month %s", got)
	}
	if got := PreviousPeriod(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), true).String(); got != "2025-Q4" {
		t.Errorf("previous quarter %s", got)
	}
	if got := PreviousPeriod(time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), true).String(); got != "2026-Q2" {
		t.Errorf("previous quarter %s", got)
	}
}

// mixedReport builds a report from a mixed dataset used by several tests.
func mixedReport() *VatReport {
	p, _ := ParsePeriod("2026-09")
	r := NewVatReport(p)
	// big domestic invoice → A.4
	r.AddSale(Sale{Number: "2026-0001", TaxPointDate: "2026-09-02", CustomerVatNo: "CZ12345678", CustomerCountry: "CZ",
		Recap: []RateAmount{{2100, 100_000_00, 21_000_00}, {1200, 10_000_00, 1_200_00}}, Total: 132_200_00})
	// small domestic invoice → A.5
	r.AddSale(Sale{Number: "2026-0002", TaxPointDate: "2026-09-03", CustomerVatNo: "CZ12345678",
		Recap: []RateAmount{{2100, 5_000_00, 1_050_00}}, Total: 6_050_00})
	// consumer (no DIČ), big → A.5
	r.AddSale(Sale{Number: "2026-0003", TaxPointDate: "2026-09-04",
		Recap: []RateAmount{{2100, 50_000_00, 10_500_00}}, Total: 60_500_00})
	// correction of the big invoice (its original was in A.4) → A.4 even though small
	r.AddSale(Sale{Number: "D2026-0001", TaxPointDate: "2026-09-05", CustomerVatNo: "CZ12345678",
		Recap: []RateAmount{{2100, -1_000_00, -210_00}}, Total: -1_210_00, ControlTotal: 132_200_00})
	// domestic reverse charge → ř. 25, A.1
	r.AddSale(Sale{Number: "2026-0004", TaxPointDate: "2026-09-06", CustomerVatNo: "CZ87654321", ReverseCharge: true,
		Recap: []RateAmount{{2100, 40_000_00, 0}}, Total: 40_000_00})
	// EU B2B services → ř. 21
	r.AddSale(Sale{Number: "2026-0005", TaxPointDate: "2026-09-07", CustomerVatNo: "DE811907980", CustomerCountry: "DE", ReverseCharge: true,
		Recap: []RateAmount{{2100, 30_000_00, 0}}, Total: 30_000_00})
	// services outside the EU → ř. 26
	r.AddSale(Sale{Number: "2026-0006", TaxPointDate: "2026-09-08", CustomerCountry: "US",
		Recap: []RateAmount{{0, 20_000_00, 0}}, Total: 20_000_00})
	// purchases: big → B.2, small → B.3, foreign → skipped, without VAT → ignored
	r.AddPurchase(Purchase{Deductible: true, Number: "FA-778", TaxPointDate: "2026-09-10", SupplierVatNo: "CZ27074358",
		Recap: []RateAmount{{2100, 20_000_00, 4_200_00}}, Total: 24_200_00})
	r.AddPurchase(Purchase{Deductible: true, Number: "UCT-1", TaxPointDate: "2026-09-11", SupplierVatNo: "CZ27074358",
		Recap: []RateAmount{{2100, 1_000_00, 210_00}, {1200, 500_00, 60_00}}, Total: 1_770_00})
	r.AddPurchase(Purchase{Deductible: true, Number: "INV-9", SupplierVatNo: "DE811907980", Recap: []RateAmount{{2100, 100_00, 21_00}}, Total: 121_00})
	r.AddPurchase(Purchase{Deductible: true, Number: "NP-1", SupplierVatNo: "CZ12345678", Recap: []RateAmount{{0, 100_00, 0}}, Total: 100_00})
	r.Finish()
	return r
}

func TestVatReportMixed(t *testing.T) {
	r := mixedReport()
	v := r.Return
	if v.R1 != (Pair{154_000_00, 32_340_00}) || v.R2 != (Pair{10_000_00, 1_200_00}) {
		t.Errorf("r1 %v r2 %v", v.R1, v.R2)
	}
	if v.R21 != 30_000_00 || v.R25 != 40_000_00 || v.R26 != 20_000_00 {
		t.Errorf("r21 %d r25 %d r26 %d", v.R21, v.R25, v.R26)
	}
	if v.R40 != (Pair{21_000_00, 4_410_00}) || v.R41 != (Pair{500_00, 60_00}) || v.R46 != 4_470_00 {
		t.Errorf("r40 %v r41 %v r46 %d", v.R40, v.R41, v.R46)
	}
	if v.R62 != 33_540_00 || v.R63 != 4_470_00 || v.R64 != 29_070_00 || v.R65 != 0 {
		t.Errorf("r62–65 %d %d %d %d", v.R62, v.R63, v.R64, v.R65)
	}
	c := r.Control
	if len(c.A4) != 2 || c.A4[0].Number != "2026-0001" || c.A4[0].VatNo != "12345678" || c.A4[1].Number != "D2026-0001" {
		t.Fatalf("A.4 %+v", c.A4)
	}
	if c.A5.Basic != (Pair{55_000_00, 11_550_00}) {
		t.Errorf("A.5 %+v", c.A5)
	}
	if len(c.A1) != 1 || c.A1[0].CustomerVatNo != "87654321" || c.A1[0].Base != 40_000_00 {
		t.Errorf("A.1 %+v", c.A1)
	}
	if len(c.B2) != 1 || c.B2[0].Number != "FA-778" || c.B3.Basic != (Pair{1_000_00, 210_00}) || c.B3.Reduced != (Pair{500_00, 60_00}) {
		t.Errorf("B %+v %+v", c.B2, c.B3)
	}
	// INV-9: VAT from a supplier without CZ DIČ; NP-1: a CZ VAT payer charged no VAT (exempt, or § 92a self-assessment?)
	if len(r.Warnings) != 2 || r.Warnings[0].Document != "INV-9" || r.Warnings[0].Code != WarnSupplierNoDIC ||
		r.Warnings[1].Document != "NP-1" || r.Warnings[1].Code != WarnPossibleRC {
		t.Errorf("warnings %v", r.Warnings)
	}

	// excess deduction
	p, _ := ParsePeriod("2026-Q1")
	r2 := NewVatReport(p)
	r2.AddPurchase(Purchase{Deductible: true, Number: "X", SupplierVatNo: "CZ27074358", Recap: []RateAmount{{2100, 100_00, 21_00}, {1500, 100_00, 15_00}}, Total: 236_00})
	r2.Finish()
	if r2.Return.R65 != 21_00 || r2.Return.R64 != 0 || len(r2.Warnings) != 1 {
		t.Errorf("excess deduction %+v %v", r2.Return, r2.Warnings)
	}
}

// validate checks doc against an EPO schema with xmllint when installed.
func validate(t *testing.T, xsd string, doc []byte) {
	t.Helper()
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Log("xmllint not installed; XSD validation skipped")
		return
	}
	f := filepath.Join(t.TempDir(), "doc.xml")
	if err := os.WriteFile(f, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("xmllint", "--noout", "--schema", filepath.Join("testdata", xsd), f).CombinedOutput(); err != nil {
		t.Fatalf("XSD validation failed: %v\n%s\n%s", err, out, doc)
	}
}

var company = Taxpayer{VatNo: "CZ27074358", Name: "Studio Kovář s.r.o.", Street: "Vinohradská 1597/174", City: "Praha 3",
	Zip: "130 00", Country: "CZ", Email: "a@b.cz", Phone: "603123456", TaxOffice: "451", TaxOfficeBranch: "2003"}

func TestDPHDP3(t *testing.T) {
	opt := EPOOptions{Filed: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), Software: "NanoFaktura", Version: "1"}
	b, err := DPHDP3(mixedReport(), company, opt)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "dphdp3_epo2.xsd", b)
	v := vetaAttrs(t, b)
	for veta, want := range map[string]map[string]string{
		"VetaD": {"mesic": "9", "rok": "2026", "dokument": "DP3", "d_poddp": "20.10.2026", "k_uladis": "DPH", "typ_platce": "P", "trans": "A"},
		"VetaP": {"dic": "27074358", "typ_ds": "P", "zkrobchjm": "Studio Kovář s.r.o.", "ulice": "Vinohradská", "c_pop": "1597", "c_orient": "174",
			"psc": "13000", "c_ufo": "451", "c_pracufo": "2003", "naz_obce": "Praha 3"},
		"Veta1": {"obrat23": "154000", "dan23": "32340", "obrat5": "10000", "dan5": "1200"},
		"Veta2": {"pln_sluzby": "30000", "pln_rez_pren": "40000", "pln_ost": "20000"},
		"Veta4": {"pln23": "21000", "odp_tuz23_nar": "4410", "pln5": "500", "odp_tuz5_nar": "60", "odp_sum_nar": "4470"},
		"Veta6": {"dan_zocelk": "33540", "odp_zocelk": "4470", "dano_da": "29070"},
	} {
		for k, w := range want {
			if got := v[veta][k]; got != w {
				t.Errorf("%s/@%s = %q, want %q", veta, k, got, w)
			}
		}
	}

	// quarterly, natural person, missing tax office
	p, _ := ParsePeriod("2026-Q3")
	r := NewVatReport(p)
	r.Finish()
	person := company
	person.VatNo, person.Name = "CZ8001011234", "Jan Novák"
	b, err = DPHDP3(r, person, opt)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "dphdp3_epo2.xsd", b)
	if s := string(b); !strings.Contains(s, `ctvrt="3"`) || !strings.Contains(s, `typ_ds="F"`) || !strings.Contains(s, `prijmeni="Novák"`) || !strings.Contains(s, `trans="N"`) {
		t.Errorf("quarterly person: %s", s)
	}
	person.TaxOffice = ""
	if _, err := DPHDP3(r, person, opt); err != ErrTaxpayer {
		t.Errorf("missing c_ufo: %v", err)
	}
}

func TestDPHKH1(t *testing.T) {
	opt := EPOOptions{Filed: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	b, err := DPHKH1(mixedReport(), company, opt)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "dphkh1_epo2.xsd", b)
	s := string(b)
	for _, want := range []string{
		`<VetaA4 c_radku="1" dic_odb="12345678" c_evid_dd="2026-0001" dppd="02.09.2026" zakl_dane1="100000.00" dan1="21000.00" zakl_dane2="10000.00" dan2="1200.00" kod_rezim_pl="0" zdph_44="N">`,
		`<VetaA5 zakl_dane1="55000.00" dan1="11550.00">`,
		`<VetaA1 c_radku="1" dic_odb="87654321" c_evid_dd="2026-0004" duzp="06.09.2026" zakl_dane1="40000.00" kod_pred_pl="">`,
		`<VetaB2 c_radku="1" dic_dod="27074358" c_evid_dd="FA-778" dppd="10.09.2026" zakl_dane1="20000.00" dan1="4200.00" pomer="N" zdph_44="N">`,
		`<VetaB3 zakl_dane1="1000.00" dan1="210.00" zakl_dane2="500.00" dan2="60.00">`,
		`<VetaC obrat23="154000.00" obrat5="10000.00" pln23="21000.00" pln5="500.00" pln_rez_pren="40000.00">`,
		`khdph_forma="B"`, `verzePis="03.01.14"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
}

func TestFlatRates(t *testing.T) {
	fr := FlatRates(1_000_000_00)
	if fr[0].Percent != 80 || fr[0].Expenses != 800_000_00 || fr[0].TaxBase != 200_000_00 {
		t.Errorf("80 %% %+v", fr[0])
	}
	fr = FlatRates(3_000_000_00)
	for i, want := range []int64{1_600_000_00, 1_200_000_00, 800_000_00, 600_000_00} {
		if fr[i].Expenses != want || fr[i].Cap != want {
			t.Errorf("capped %+v", fr[i])
		}
	}
	if fr := FlatRates(-5); fr[0].Expenses != 0 || fr[0].TaxBase != -5 {
		t.Errorf("negative %+v", fr[0])
	}
	if roundKc(150) != 2 || roundKc(149) != 1 || roundKc(-150) != -2 || epoDate("2026-01-05") != "05.01.2026" {
		t.Error("helpers")
	}
}

// vetaAttrs returns the attributes of every element by element name.
func vetaAttrs(t *testing.T, b []byte) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			m := map[string]string{}
			for _, a := range se.Attr {
				m[a.Name.Local] = a.Value
			}
			out[se.Name.Local] = m
		}
	}
	return out
}
