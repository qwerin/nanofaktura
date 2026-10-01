package bankimport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/qwerin/nanofaktura/internal/search"
)

// CSVMapping says which CSV column holds which Transaction field.
//
// Each column reference is either a header name — compared case- and
// diacritics-insensitively, alternatives separated by "|" ("VS|Variabilní
// symbol") — or "#N", the N-th column (1-based). Empty = not present.
type CSVMapping struct {
	ExternalID          string
	BookedOn            string // required
	Amount              string // signed amount; or use Credit + Debit
	Credit              string // incoming amount column (unsigned)
	Debit               string // outgoing amount column (unsigned or negative)
	Currency            string
	CounterpartyAccount string
	CounterpartyBank    string // bank code appended as "/bank" when the account column lacks it
	CounterpartyName    string
	VS, KS, SS          string
	Message             []string // non-empty values are joined with " "

	DateFormat      string // Go layout, "" = auto (Czech/ISO formats)
	DefaultCurrency string // when there is no currency column, default "CZK"
	NoHeader        bool   // the file has no header row; all references must be "#N"
}

// CSVOptions are the file-level settings; zero values mean auto-detect.
type CSVOptions struct {
	Delimiter  rune     // 0 = detect among ; , TAB |
	DecimalSep rune     // 0 = detect per value (',' or '.')
	Encoding   Encoding // "" = detect UTF-8 / Windows-1250 / CP852
}

// Bank export profiles. The header names come from the banks' internet
// banking CSV exports; alternatives cover older/newer versions.
var (
	// FioCSVMapping: Fio internet banking "Pohyby na účtu" CSV (UTF-8, ';', preamble lines before the header).
	FioCSVMapping = CSVMapping{
		ExternalID: "ID operace|ID pohybu", BookedOn: "Datum", Amount: "Objem", Currency: "Měna",
		CounterpartyAccount: "Protiúčet", CounterpartyBank: "Kód banky", CounterpartyName: "Název protiúčtu",
		VS: "VS", KS: "KS", SS: "SS", Message: []string{"Zpráva pro příjemce"},
	}
	// CSOBCSVMapping: ČSOB internet banking CSV (Windows-1250, ';'); both the
	// older header ("číslo účtu protiúčtu", "ID transakce") and the newer one.
	CSOBCSVMapping = CSVMapping{
		ExternalID: "ID transakce", BookedOn: "datum zaúčtování", Amount: "částka", Currency: "měna",
		CounterpartyAccount: "číslo účtu protiúčtu|číslo protiúčtu", CounterpartyBank: "kód banky protiúčtu",
		CounterpartyName: "název účtu protiúčtu|název protiúčtu",
		VS:               "variabilní symbol", KS: "konstantní symbol", SS: "specifický symbol", Message: []string{"poznámka"},
	}
	// KBCSVMapping: Komerční banka MojeBanka "export transakční historie" (Windows-1250, ';').
	KBCSVMapping = CSVMapping{
		ExternalID: "Identifikace transakce", BookedOn: "Datum splatnosti|Datum zaúčtování", Amount: "Částka",
		CounterpartyAccount: "Protistrana|Protiúčet|Protiúčet a kód banky", CounterpartyBank: "Kód banky protiúčtu|Kód banky",
		CounterpartyName: "Název protiúčtu|Název protistrany",
		VS:               "VS|Variabilní symbol", KS: "KS|Konstantní symbol", SS: "SS|Specifický symbol",
		Message: []string{"Popis pro příjemce|Zpráva pro příjemce", "AV pole 1", "AV pole 2", "AV pole 3", "AV pole 4"},
	}
	// KBBusinessCSVMapping: KB MojeBanka Business "Klientský formát CSV" —
	// no header, 27 fixed columns (14 due date, 16 counter-account, 17 bank,
	// 18 name, 19 amount, 20–22 VS/KS/SS, 23 transaction ID, 26–27 messages).
	KBBusinessCSVMapping = CSVMapping{
		NoHeader: true, ExternalID: "#23", BookedOn: "#14", Amount: "#19", Currency: "#3",
		CounterpartyAccount: "#16", CounterpartyBank: "#17", CounterpartyName: "#18",
		VS: "#20", KS: "#21", SS: "#22", Message: []string{"#26", "#27"},
	}
	// AirBankCSVMapping: Air Bank CSV export (';', dates DD/MM/YYYY).
	AirBankCSVMapping = CSVMapping{
		ExternalID: "Referenční číslo", BookedOn: "Datum provedení", Amount: "Částka v měně účtu", Currency: "Měna účtu",
		CounterpartyAccount: "Číslo účtu protistrany", CounterpartyName: "Název protistrany|Název účtu protistrany",
		VS: "Variabilní symbol", KS: "Konstantní symbol", SS: "Specifický symbol", Message: []string{"Zpráva pro příjemce"},
	}
)

// csvProfiles is the detection order; each needs its distinctive columns.
var csvProfiles = []struct {
	format   Format
	mapping  CSVMapping
	requires []string
}{
	{FormatFioCSV, FioCSVMapping, []string{"Objem", "Protiúčet", "Datum"}},
	{FormatAirBankCSV, AirBankCSVMapping, []string{"Datum provedení", "Částka v měně účtu"}},
	{FormatCSOBCSV, CSOBCSVMapping, []string{"datum zaúčtování", "částka", "variabilní symbol"}},
	{FormatKBCSV, KBCSVMapping, []string{"Identifikace transakce", "Částka"}},
}

// ParseCSV parses a CSV export with the given mapping.
func ParseCSV(data []byte, m CSVMapping, opt CSVOptions) (*Statement, error) {
	rows, err := readCSV(data, opt)
	if err != nil {
		return nil, err
	}
	return parseRows(rows, m, opt)
}

// ParseFioCSV parses a Fio internet banking CSV export.
func ParseFioCSV(data []byte) (*Statement, error) {
	return ParseCSV(data, FioCSVMapping, CSVOptions{})
}

// ParseCSOBCSV parses a ČSOB CSV export.
func ParseCSOBCSV(data []byte) (*Statement, error) {
	return ParseCSV(data, CSOBCSVMapping, CSVOptions{})
}

// ParseAirBankCSV parses an Air Bank CSV export.
func ParseAirBankCSV(data []byte) (*Statement, error) {
	return ParseCSV(data, AirBankCSVMapping, CSVOptions{})
}

// ParseKBCSV parses a Komerční banka CSV: the MojeBanka export with a header,
// or the headerless MojeBanka Business client format.
func ParseKBCSV(data []byte) (*Statement, error) {
	rows, err := readCSV(data, CSVOptions{})
	if err != nil {
		return nil, err
	}
	if hasColumns(rows, []string{"Identifikace transakce", "Částka"}) {
		return parseRows(rows, KBCSVMapping, CSVOptions{})
	}
	if isKBBusiness(rows) {
		return parseRows(rows, KBBusinessCSVMapping, CSVOptions{})
	}
	return nil, fmt.Errorf("%w: not a KB CSV export", ErrFormat)
}

func isKBBusiness(rows [][]string) bool {
	for _, r := range rows {
		if len(r) == 1 && strings.TrimSpace(r[0]) == "" {
			continue
		}
		if len(r) < 27 {
			return false
		}
		_, err1 := ParseDate(r[0], "02.01.2006")
		_, err2 := ParseDate(r[13], "02.01.2006")
		return err1 == nil && err2 == nil
	}
	return false
}

// DetectCSV returns the bank CSV format of data, or FormatUnknown.
func DetectCSV(data []byte) Format {
	rows, err := readCSV(data, CSVOptions{})
	if err != nil {
		return FormatUnknown
	}
	for _, p := range csvProfiles {
		if hasColumns(rows, p.requires) {
			return p.format
		}
	}
	if isKBBusiness(rows) {
		return FormatKBCSV
	}
	return FormatUnknown
}

// Detect recognizes the format of a statement file.
func Detect(data []byte) Format {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte(bom)))
	switch {
	case bytes.HasPrefix(trimmed, []byte("{")) && bytes.Contains(trimmed, []byte(`"accountStatement"`)):
		return FormatFioJSON
	case bytes.HasPrefix(trimmed, []byte("074")) || bytes.HasPrefix(trimmed, []byte("075")):
		return FormatGPC
	}
	return DetectCSV(data)
}

// Parse parses data in the given format.
func Parse(format Format, data []byte) (*Statement, error) {
	switch format {
	case FormatFioJSON:
		return ParseFioJSON(data)
	case FormatGPC:
		return ParseGPC(data, EncodingAuto)
	case FormatFioCSV:
		return ParseFioCSV(data)
	case FormatCSOBCSV:
		return ParseCSOBCSV(data)
	case FormatKBCSV:
		return ParseKBCSV(data)
	case FormatAirBankCSV:
		return ParseAirBankCSV(data)
	}
	return nil, ErrFormat
}

// ParseAuto detects the format and parses data.
func ParseAuto(data []byte) (Format, *Statement, error) {
	f := Detect(data)
	if f == FormatUnknown {
		return f, nil, ErrFormat
	}
	st, err := Parse(f, data)
	return f, st, err
}

// readCSV decodes data and splits it into records.
func readCSV(data []byte, opt CSVOptions) ([][]string, error) {
	text, _, err := Decode(data, opt.Encoding)
	if err != nil {
		return nil, err
	}
	delim := opt.Delimiter
	if delim == 0 {
		delim = DetectDelimiter(text)
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	var rows [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: CSV: %v", ErrFormat, err)
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty CSV", ErrFormat)
	}
	return rows, nil
}

// DetectDelimiter picks the delimiter among ';', ',', TAB and '|' that splits
// the most lines (of the first 30) into the same number of fields; quoted
// text is ignored.
func DetectDelimiter(text string) rune {
	lines := strings.Split(text, "\n")
	if len(lines) > 30 {
		lines = lines[:30]
	}
	best, bestScore := ';', -1
	for _, d := range []rune{';', ',', '\t', '|'} {
		counts := map[int]int{}
		for _, l := range lines {
			if n := countUnquoted(l, d); n > 0 {
				counts[n]++
			}
		}
		score := 0
		for n, lines := range counts {
			if s := n * lines * lines; s > score {
				score = s
			}
		}
		if score > bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

func countUnquoted(line string, d rune) int {
	n, quoted := 0, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == d && !quoted:
			n++
		}
	}
	return n
}

// fold normalizes a header name: lower case, no diacritics, single spaces.
func fold(s string) string {
	out := search.StripMarks(s)
	return strings.ToLower(squash(strings.Trim(squash(out), "\"'")))
}

// resolve finds the index of a column reference in header ("#N" needs no header).
func resolve(ref string, header []string) (int, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return -1, false
	}
	if strings.HasPrefix(ref, "#") {
		n, err := strconv.Atoi(ref[1:])
		return n - 1, err == nil && n > 0
	}
	for _, alt := range strings.Split(ref, "|") {
		want := fold(alt)
		for i, h := range header {
			if fold(h) == want {
				return i, true
			}
		}
	}
	return -1, false
}

func hasColumns(rows [][]string, names []string) bool {
	for _, r := range rows[:min(len(rows), 50)] {
		ok := true
		for _, n := range names {
			if _, found := resolve(n, r); !found {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// findHeader returns the index of the header row (the first row resolving the
// date and amount columns) and the resolved column indexes.
func findHeader(rows [][]string, m CSVMapping) (int, *columns, bool) {
	if m.NoHeader {
		c, ok := m.columns(nil)
		return -1, c, ok
	}
	for i, r := range rows[:min(len(rows), 50)] {
		if c, ok := m.columns(r); ok {
			return i, c, true
		}
	}
	return 0, nil, false
}

type columns struct {
	id, date, amount, credit, debit, currency, account, bank, name, vs, ks, ss int
	message                                                                    []int
}

// columns resolves the mapping against a header row; ok requires the date
// and the amount (or credit/debit) columns.
func (m CSVMapping) columns(header []string) (*columns, bool) {
	idx := func(ref string) int {
		if i, ok := resolve(ref, header); ok {
			return i
		}
		return -1
	}
	c := &columns{
		id: idx(m.ExternalID), date: idx(m.BookedOn), amount: idx(m.Amount), credit: idx(m.Credit), debit: idx(m.Debit),
		currency: idx(m.Currency), account: idx(m.CounterpartyAccount), bank: idx(m.CounterpartyBank),
		name: idx(m.CounterpartyName), vs: idx(m.VS), ks: idx(m.KS), ss: idx(m.SS),
	}
	for _, ref := range m.Message {
		if i := idx(ref); i >= 0 {
			c.message = append(c.message, i)
		}
	}
	ok := c.date >= 0 && (c.amount >= 0 || c.credit >= 0 || c.debit >= 0)
	return c, ok
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseRows(rows [][]string, m CSVMapping, opt CSVOptions) (*Statement, error) {
	h, c, ok := findHeader(rows, m)
	if !ok {
		return nil, fmt.Errorf("%w: CSV header with the date and amount columns not found", ErrFormat)
	}
	defCur := strings.ToUpper(m.DefaultCurrency)
	if defCur == "" {
		defCur = "CZK"
	}
	st := &Statement{Transactions: []Transaction{}}
	for i := h + 1; i < len(rows); i++ {
		row := rows[i]
		date := cell(row, c.date)
		amt, cr, dr := cell(row, c.amount), cell(row, c.credit), cell(row, c.debit)
		if date == "" && amt == "" && cr == "" && dr == "" {
			continue // blank line
		}
		bookedOn, err := ParseDate(date, m.DateFormat)
		if err != nil {
			if amt == "" && cr == "" && dr == "" {
				continue // footer / summary line
			}
			return nil, fmt.Errorf("%w: CSV line %d: %v", ErrFormat, i+1, err)
		}
		t := Transaction{
			ExternalID: cell(row, c.id), BookedOn: bookedOn, Currency: strings.ToUpper(cell(row, c.currency)),
			CounterpartyName: squash(cell(row, c.name)),
			VS:               symbol(cell(row, c.vs)), KS: symbol(cell(row, c.ks)), SS: symbol(cell(row, c.ss)),
		}
		if t.Currency == "" {
			t.Currency = defCur
		}
		t.CounterpartyAccount = FormatAccount(cell(row, c.account), cell(row, c.bank))
		var msg []string
		for _, mi := range c.message {
			if v := squash(cell(row, mi)); v != "" {
				msg = append(msg, v)
			}
		}
		t.Message = strings.Join(msg, " ")
		if t.Amount, err = rowAmount(amt, cr, dr, opt.DecimalSep, t.Currency); err != nil {
			return nil, fmt.Errorf("%w: CSV line %d: %v", ErrFormat, i+1, err)
		}
		st.Transactions = append(st.Transactions, t)
	}
	if len(st.Transactions) > 0 {
		st.Currency = st.Transactions[0].Currency
	}
	fillHashIDs(st.Transactions)
	return st, nil
}

func rowAmount(amount, credit, debit string, sep rune, currency string) (int64, error) {
	if amount != "" {
		return ParseAmount(amount, sep, currency)
	}
	var v int64
	if credit != "" {
		c, err := ParseAmount(credit, sep, currency)
		if err != nil {
			return 0, err
		}
		v += abs(c)
	}
	if debit != "" {
		d, err := ParseAmount(debit, sep, currency)
		if err != nil {
			return 0, err
		}
		v -= abs(d)
	}
	return v, nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
