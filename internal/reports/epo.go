package reports

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EPO form versions (structure descriptions DPHDP3 / DPHKH1 on the tax
// portal, schemas dphdp3_epo2.xsd / dphkh1_epo2.xsd).
const (
	VersionDP3 = "03.01.03"
	VersionKH1 = "03.01.14"
)

// Taxpayer identifies the filer in the EPO header (věta P).
type Taxpayer struct {
	VatNo           string // "CZ12345678" or the numeric part
	Name            string
	Street          string
	City            string
	Zip             string
	Country         string // ISO code
	Email           string
	Phone           string
	TaxOffice       string // c_ufo
	TaxOfficeBranch string // c_pracufo
}

// ErrTaxpayer is returned when the header lacks mandatory data.
var ErrTaxpayer = errors.New("the tax office code (c_ufo) and a Czech DIČ are required for the EPO export")

// EPOOptions set the header of an EPO document.
type EPOOptions struct {
	Filed    time.Time // d_poddp
	Software string    // nazevSW
	Version  string    // verzeSW
}

type pisemnost struct {
	XMLName  xml.Name `xml:"Pisemnost"`
	NazevSW  string   `xml:"nazevSW,attr,omitempty"`
	VerzeSW  string   `xml:"verzeSW,attr,omitempty"`
	Document any
}

type attrs []xml.Attr

func (a *attrs) set(name, value string) {
	if value != "" {
		*a = append(*a, xml.Attr{Name: xml.Name{Local: name}, Value: value})
	}
}

// setKc writes whole crowns (DPHDP3), omitting zero.
func (a *attrs) setKc(name string, haler int64) {
	if v := roundKc(haler); v != 0 {
		a.set(name, strconv.FormatInt(v, 10))
	}
}

// setAmt writes crowns with two decimals (DPHKH1), omitting zero.
func (a *attrs) setAmt(name string, haler int64) {
	if haler != 0 {
		a.set(name, kc(haler))
	}
}

// veta is an element with attributes only.
type veta struct {
	name  string
	attrs attrs
}

func (v veta) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: xml.Name{Local: v.name}, Attr: v.attrs}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}

type form struct {
	XMLName  xml.Name
	VerzePis string `xml:"verzePis,attr"`
	Vety     []veta
}

func (f form) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: f.XMLName, Attr: []xml.Attr{{Name: xml.Name{Local: "verzePis"}, Value: f.VerzePis}}}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, v := range f.Vety {
		if err := e.Encode(v); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

func encode(f form, opt EPOOptions) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(pisemnost{NazevSW: opt.Software, VerzeSW: opt.Version, Document: f}); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func periodAttrs(a *attrs, p Period) {
	a.set("rok", strconv.Itoa(p.Year))
	if p.Quarterly() {
		a.set("ctvrt", strconv.Itoa(p.Quarter))
	} else {
		a.set("mesic", strconv.Itoa(p.Month))
	}
}

// DPHDP3 renders the VAT return in the EPO XML format (řádné přiznání).
func DPHDP3(r *VatReport, tp Taxpayer, opt EPOOptions) ([]byte, error) {
	p, err := vetaP(tp)
	if err != nil {
		return nil, err
	}
	d := veta{name: "VetaD"}
	d.attrs.set("k_uladis", "DPH")
	d.attrs.set("dokument", "DP3")
	periodAttrs(&d.attrs, r.Period)
	d.attrs.set("typ_platce", "P")
	d.attrs.set("dapdph_forma", "B")
	d.attrs.set("d_poddp", opt.Filed.Format("02.01.2006"))
	v := r.Return
	trans := "N"
	if v.R1.Base != 0 || v.R2.Base != 0 || v.R20 != 0 || v.R21 != 0 || v.R25 != 0 || v.R26 != 0 {
		trans = "A"
	}
	d.attrs.set("trans", trans)

	vety := []veta{d, p}
	v1 := veta{name: "Veta1"}
	pair := func(base, vat string, x Pair) {
		v1.attrs.setKc(base, x.Base)
		v1.attrs.setKc(vat, x.Vat)
	}
	pair("obrat23", "dan23", v.R1)
	pair("obrat5", "dan5", v.R2)
	pair("p_zb23", "dan_pzb23", v.R3)
	pair("p_zb5", "dan_pzb5", v.R4)
	pair("p_sl23_e", "dan_psl23_e", v.R5)
	pair("p_sl5_e", "dan_psl5_e", v.R6)
	pair("rez_pren23", "dan_rpren23", v.R10)
	pair("rez_pren5", "dan_rpren5", v.R11)
	pair("p_sl23_z", "dan_psl23_z", v.R12)
	pair("p_sl5_z", "dan_psl5_z", v.R13)
	v2 := veta{name: "Veta2"}
	v2.attrs.setKc("dod_zb", v.R20)
	v2.attrs.setKc("pln_sluzby", v.R21)
	v2.attrs.setKc("pln_rez_pren", v.R25)
	v2.attrs.setKc("pln_ost", v.R26)
	// Row 46 and 62–65 from the rounded rows, as the form computes them.
	odp := roundKc(v.R40.Vat) + roundKc(v.R41.Vat) + roundKc(v.R43.Vat) + roundKc(v.R44.Vat)
	v4 := veta{name: "Veta4"}
	v4.attrs.setKc("pln23", v.R40.Base)
	v4.attrs.setKc("odp_tuz23_nar", v.R40.Vat)
	v4.attrs.setKc("pln5", v.R41.Base)
	v4.attrs.setKc("odp_tuz5_nar", v.R41.Vat)
	v4.attrs.setKc("nar_zdp23", v.R43.Base)
	v4.attrs.setKc("od_zdp23", v.R43.Vat)
	v4.attrs.setKc("nar_zdp5", v.R44.Base)
	v4.attrs.setKc("od_zdp5", v.R44.Vat)
	if odp != 0 {
		v4.attrs.set("odp_sum_nar", strconv.FormatInt(odp, 10))
	}
	for _, x := range []veta{v1, v2, v4} {
		if len(x.attrs) > 0 {
			vety = append(vety, x)
		}
	}
	var dan int64
	for _, x := range []Pair{v.R1, v.R2, v.R3, v.R4, v.R5, v.R6, v.R10, v.R11, v.R12, v.R13} {
		dan += roundKc(x.Vat)
	}
	v6 := veta{name: "Veta6"}
	v6.attrs.set("dan_zocelk", strconv.FormatInt(dan, 10))
	v6.attrs.set("odp_zocelk", strconv.FormatInt(odp, 10))
	if dan >= odp {
		v6.attrs.set("dano_da", strconv.FormatInt(dan-odp, 10))
	} else {
		v6.attrs.set("dano_no", strconv.FormatInt(odp-dan, 10))
	}
	vety = append(vety, v6)
	return encode(form{XMLName: xml.Name{Local: "DPHDP3"}, VerzePis: VersionDP3, Vety: vety}, opt)
}

// DPHKH1 renders the VAT control statement in the EPO XML format (řádné).
func DPHKH1(r *VatReport, tp Taxpayer, opt EPOOptions) ([]byte, error) {
	p, err := vetaP(tp)
	if err != nil {
		return nil, err
	}
	d := veta{name: "VetaD"}
	d.attrs.set("k_uladis", "DPH")
	d.attrs.set("dokument", "KH1")
	periodAttrs(&d.attrs, r.Period)
	d.attrs.set("khdph_forma", "B")
	d.attrs.set("d_poddp", opt.Filed.Format("02.01.2006"))
	vety := []veta{d, p}

	c := r.Control
	for i, a := range c.A1 {
		v := veta{name: "VetaA1"}
		v.attrs.set("c_radku", strconv.Itoa(i+1))
		v.attrs.set("dic_odb", a.CustomerVatNo)
		v.attrs.set("c_evid_dd", a.Number)
		v.attrs.set("duzp", epoDate(a.TaxPointDate))
		v.attrs.set("zakl_dane1", kc(a.Base))
		// kod_pred_pl (subject of the § 92a supply) is not recorded on
		// invoices; the empty value must be completed in the EPO form.
		v.attrs = append(v.attrs, xml.Attr{Name: xml.Name{Local: "kod_pred_pl"}, Value: ""})
		vety = append(vety, v)
	}
	for i, a := range c.A2 {
		v := veta{name: "VetaA2"}
		v.attrs.set("c_radku", strconv.Itoa(i+1))
		v.attrs.set("k_stat", a.Country)
		v.attrs.set("vatid_dod", a.VatID)
		v.attrs.set("c_evid_dd", a.Number)
		v.attrs.set("dppd", epoDate(a.TaxPointDate))
		v.attrs = append(v.attrs, sumsVeta("", a.RateSums).attrs...)
		vety = append(vety, v)
	}
	for i, a := range c.A4 {
		v := documentVeta("VetaA4", "dic_odb", i, a)
		v.attrs.set("kod_rezim_pl", "0")
		v.attrs.set("zdph_44", "N")
		vety = append(vety, v)
	}
	if a5 := sumsVeta("VetaA5", c.A5); len(a5.attrs) > 0 {
		vety = append(vety, a5)
	}
	for i, b := range c.B1 {
		v := veta{name: "VetaB1"}
		v.attrs.set("c_radku", strconv.Itoa(i+1))
		v.attrs.set("dic_dod", b.SupplierVatNo)
		v.attrs.set("c_evid_dd", b.Number)
		v.attrs.set("duzp", epoDate(b.TaxPointDate))
		v.attrs = append(v.attrs, sumsVeta("", b.RateSums).attrs...)
		// the subject code of the § 92a supply is not recorded; complete it in the EPO form
		v.attrs = append(v.attrs, xml.Attr{Name: xml.Name{Local: "kod_pred_pl"}, Value: ""})
		vety = append(vety, v)
	}
	for i, b := range c.B2 {
		v := documentVeta("VetaB2", "dic_dod", i, b)
		v.attrs.set("pomer", "N")
		v.attrs.set("zdph_44", "N")
		vety = append(vety, v)
	}
	if b3 := sumsVeta("VetaB3", c.B3); len(b3.attrs) > 0 {
		vety = append(vety, b3)
	}

	// Section C: control sums (A.4 + A.5, B.2 + B.3, A.1).
	var outBasic, outReduced, inBasic, inReduced, rc int64
	for _, a := range c.A4 {
		outBasic += a.Basic.Base
		outReduced += a.Reduced.Base
	}
	outBasic += c.A5.Basic.Base
	outReduced += c.A5.Reduced.Base
	for _, b := range c.B2 {
		inBasic += b.Basic.Base
		inReduced += b.Reduced.Base
	}
	inBasic += c.B3.Basic.Base
	inReduced += c.B3.Reduced.Base
	for _, a := range c.A1 {
		rc += a.Base
	}
	var b1Basic, b1Reduced, a2 int64
	for _, b := range c.B1 {
		b1Basic += b.Basic.Base
		b1Reduced += b.Reduced.Base
	}
	for _, a := range c.A2 {
		a2 += a.Basic.Base + a.Reduced.Base
	}
	vc := veta{name: "VetaC"}
	vc.attrs.setAmt("obrat23", outBasic)
	vc.attrs.setAmt("obrat5", outReduced)
	vc.attrs.setAmt("pln23", inBasic)
	vc.attrs.setAmt("pln5", inReduced)
	vc.attrs.setAmt("pln_rez_pren", rc)
	vc.attrs.setAmt("rez_pren23", b1Basic)
	vc.attrs.setAmt("rez_pren5", b1Reduced)
	vc.attrs.setAmt("celk_zd_a2", a2)
	if len(vc.attrs) > 0 {
		vety = append(vety, vc)
	}
	return encode(form{XMLName: xml.Name{Local: "DPHKH1"}, VerzePis: VersionKH1, Vety: vety}, opt)
}

func documentVeta(name, dicAttr string, i int, r DocumentRow) veta {
	v := veta{name: name}
	v.attrs.set("c_radku", strconv.Itoa(i+1))
	v.attrs.set(dicAttr, r.VatNo)
	v.attrs.set("c_evid_dd", r.Number)
	v.attrs.set("dppd", epoDate(r.TaxPointDate))
	v.attrs = append(v.attrs, sumsVeta("", r.RateSums).attrs...)
	return v
}

func sumsVeta(name string, s RateSums) veta {
	v := veta{name: name}
	v.attrs.setAmt("zakl_dane1", s.Basic.Base)
	v.attrs.setAmt("dan1", s.Basic.Vat)
	v.attrs.setAmt("zakl_dane2", s.Reduced.Base)
	v.attrs.setAmt("dan2", s.Reduced.Vat)
	return v
}

var buildingRe = regexp.MustCompile(`^(.*?)\s+(\d[\w/\-]*)$`)

// LegalPerson reports whether a Czech DIČ belongs to a legal person (8
// digits = IČO; natural persons have a 9–10 digit birth number). A legal
// person files the control statement for every month, also with a
// quarterly VAT period (§ 101e odst. 1 ZDPH).
func LegalPerson(vatNo string) bool {
	d := strings.TrimPrefix(normVatNo(vatNo), "CZ")
	return len(d) == 8 && strings.Trim(d, "0123456789") == ""
}

// vetaP is the taxpayer header. The subject type is derived from the DIČ:
// 8 digits = legal person (IČO), otherwise a natural person (birth number).
func vetaP(tp Taxpayer) (veta, error) {
	dic, ok := czDIC(normVatNo(tp.VatNo))
	if !ok {
		if d := normVatNo(tp.VatNo); len(d) >= 8 && len(d) <= 10 && strings.Trim(d, "0123456789") == "" {
			dic, ok = d, true
		}
	}
	if !ok || strings.TrimSpace(tp.TaxOffice) == "" {
		return veta{}, ErrTaxpayer
	}
	p := veta{name: "VetaP"}
	p.attrs.set("c_ufo", strings.TrimSpace(tp.TaxOffice))
	p.attrs.set("c_pracufo", strings.TrimSpace(tp.TaxOfficeBranch))
	p.attrs.set("dic", dic)
	name := strings.TrimSpace(tp.Name)
	if len(dic) == 8 {
		p.attrs.set("typ_ds", "P")
		p.attrs.set("zkrobchjm", name)
	} else {
		p.attrs.set("typ_ds", "F")
		first, last, _ := strings.Cut(name, " ")
		p.attrs.set("jmeno", cut(first, 30))
		p.attrs.set("prijmeni", cut(strings.TrimSpace(last), 36))
	}
	street, number := strings.TrimSpace(tp.Street), ""
	if m := buildingRe.FindStringSubmatch(street); m != nil {
		street, number = m[1], m[2]
	}
	p.attrs.set("ulice", cut(street, 38))
	// "1597/174" = číslo popisné / číslo orientační
	pop, orient, _ := strings.Cut(number, "/")
	if _, err := strconv.ParseUint(pop, 10, 32); err == nil {
		p.attrs.set("c_pop", pop)
	} else if orient == "" {
		orient = pop
	}
	p.attrs.set("c_orient", cut(orient, 4))
	p.attrs.set("naz_obce", cut(strings.TrimSpace(tp.City), 48))
	p.attrs.set("psc", cut(strings.ReplaceAll(tp.Zip, " ", ""), 10))
	if c := strings.ToUpper(tp.Country); c == "" || c == "CZ" {
		p.attrs.set("stat", "ČESKÁ REPUBLIKA")
	}
	p.attrs.set("email", cut(strings.TrimSpace(tp.Email), 255))
	p.attrs.set("c_telef", cut(strings.ReplaceAll(tp.Phone, " ", ""), 14))
	return p, nil
}

// cut truncates s to max runes (EPO attribute limits).
func cut(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// epoDate converts "YYYY-MM-DD" to "DD.MM.YYYY".
func epoDate(d string) string {
	t, err := time.Parse(time.DateOnly, d)
	if err != nil {
		return d
	}
	return t.Format("02.01.2006")
}

// roundKc rounds haléře to whole crowns, half away from zero.
func roundKc(h int64) int64 {
	if h < 0 {
		return -((-h + 50) / 100)
	}
	return (h + 50) / 100
}

// String helpers for tests and debugging.
func (p Pair) String() string { return fmt.Sprintf("%s/%s", kc(p.Base), kc(p.Vat)) }
