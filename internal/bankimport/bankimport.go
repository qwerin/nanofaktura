// Package bankimport parses Czech bank statements into a common Transaction
// shape: Fio API JSON (plus an API client), ABO/GPC files and CSV exports of
// Fio, ČSOB, Komerční banka and Air Bank, or any CSV with a column mapping.
//
// It is pure parsing: storing transactions and matching them to invoices is
// the caller's job. Amounts are int64 in minor units (haléře), signed:
// positive = incoming (credit), negative = outgoing (debit).
package bankimport

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Transaction is one booked movement on the account.
type Transaction struct {
	ExternalID          string // bank's movement ID; "h:<hash>" when the source has none (stable across re-imports)
	BookedOn            string // YYYY-MM-DD
	Amount              int64  // minor units, + incoming / − outgoing
	Currency            string // ISO 4217
	CounterpartyAccount string // "prefix-number/bank", IBAN, or "" (card payments, fees)
	CounterpartyName    string
	VS                  string // variable symbol, without leading zeros
	KS                  string // constant symbol
	SS                  string // specific symbol
	Message             string // message for the recipient / description
}

// Statement is the result of parsing one file or API response. Account data
// and balances are filled only when the source carries them.
type Statement struct {
	Account        string // own account ("2000000000/2010"), may be ""
	IBAN           string
	Currency       string
	OpeningBalance *int64
	ClosingBalance *int64
	Transactions   []Transaction
}

// Format identifies a statement format.
type Format string

// Supported formats.
const (
	FormatUnknown    Format = ""
	FormatFioJSON    Format = "fio_json"
	FormatGPC        Format = "gpc"
	FormatFioCSV     Format = "fio_csv"
	FormatCSOBCSV    Format = "csob_csv"
	FormatKBCSV      Format = "kb_csv"
	FormatAirBankCSV Format = "airbank_csv"
)

// ErrFormat: the data is not in the expected (or any recognized) format.
var ErrFormat = errors.New("bankimport: unrecognized statement format")

// Encoding of a text file.
type Encoding string

// Supported text encodings; EncodingAuto detects UTF-8, Windows-1250 or CP852.
const (
	EncodingAuto        Encoding = ""
	EncodingUTF8        Encoding = "utf-8"
	EncodingWindows1250 Encoding = "windows-1250"
	EncodingCP852       Encoding = "cp852"
)

const bom = "\xef\xbb\xbf"

// Decode converts data to a Go string. With EncodingAuto, valid UTF-8 wins;
// otherwise Windows-1250 and CP852 (DOS Latin 2) are both tried and the one
// yielding more Czech letters (and fewer box-drawing/control characters) wins.
func Decode(data []byte, enc Encoding) (string, Encoding, error) {
	switch enc {
	case EncodingUTF8:
		return strings.TrimPrefix(string(data), bom), enc, nil
	case EncodingWindows1250:
		s, err := charmap.Windows1250.NewDecoder().Bytes(data)
		return string(s), enc, err
	case EncodingCP852:
		s, err := charmap.CodePage852.NewDecoder().Bytes(data)
		return string(s), enc, err
	case EncodingAuto:
	default:
		return "", enc, fmt.Errorf("bankimport: unsupported encoding %q", enc)
	}
	if utf8.Valid(data) {
		return strings.TrimPrefix(string(data), bom), EncodingUTF8, nil
	}
	w, _ := charmap.Windows1250.NewDecoder().Bytes(data)
	d, _ := charmap.CodePage852.NewDecoder().Bytes(data)
	if czechScore(string(d)) > czechScore(string(w)) {
		return string(d), EncodingCP852, nil
	}
	return string(w), EncodingWindows1250, nil
}

const czechLetters = "áčďéěíňóřšťúůýžÁČĎÉĚÍŇÓŘŠŤÚŮÝŽ"

func czechScore(s string) int {
	score := 0
	for _, r := range s {
		switch {
		case strings.ContainsRune(czechLetters, r):
			score++
		case r >= 0x2500 && r <= 0x25ff, r >= 0x80 && r <= 0x9f: // box drawing, C1 controls
			score -= 2
		}
	}
	return score
}

// minorDigits returns the number of decimal places of a currency (ISO 4217).
func minorDigits(currency string) int {
	switch currency {
	case "JPY", "KRW", "ISK", "CLP", "VND", "PYG", "UGX", "XAF", "XOF", "XPF", "KMF", "GNF", "RWF", "VUV", "DJF", "BIF":
		return 0
	case "BHD", "KWD", "OMR", "JOD", "TND", "LYD", "IQD":
		return 3
	}
	return 2
}

// ParseAmount converts a decimal amount ("-1 234,50", "1,234.50", "+12.5",
// "100.00-") into minor units of currency. decimalSep is ',' or '.'; 0 detects
// it per value (the last of ',' / '.' is decimal when both occur, a single
// separator occurring once is decimal). Extra non-zero decimals are an error
// (money is never rounded silently).
func ParseAmount(s string, decimalSep rune, currency string) (int64, error) {
	orig := s
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\'' || r == '\u00a0' || r == '\u202f' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimFunc(s, unicode.IsLetter) // "CZK", "Kč"
	neg := false
	switch {
	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "\u2212"):
		neg, s = true, strings.TrimPrefix(strings.TrimPrefix(s, "-"), "\u2212")
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	case strings.HasSuffix(s, "-"):
		neg, s = true, strings.TrimSuffix(s, "-")
	}
	if s == "" {
		return 0, fmt.Errorf("bankimport: empty amount")
	}
	if decimalSep == 0 {
		lc, ld := strings.LastIndexByte(s, ','), strings.LastIndexByte(s, '.')
		switch {
		case lc >= 0 && ld >= 0:
			decimalSep = ','
			if ld > lc {
				decimalSep = '.'
			}
		case lc >= 0 && strings.Count(s, ",") == 1:
			decimalSep = ','
		case ld >= 0 && strings.Count(s, ".") == 1:
			decimalSep = '.'
		case lc >= 0:
			decimalSep = '.' // "1,234,567" → thousands only
		default:
			decimalSep = ','
		}
	}
	thousands := "."
	if decimalSep == '.' {
		thousands = ","
	}
	s = strings.ReplaceAll(s, thousands, "")
	intPart, frac, _ := strings.Cut(s, string(decimalSep))
	if intPart == "" {
		intPart = "0"
	}
	if !allDigits(intPart) || !allDigits(frac) {
		return 0, fmt.Errorf("bankimport: invalid amount %q", orig)
	}
	digits := minorDigits(currency)
	if len(frac) > digits {
		if strings.Trim(frac[digits:], "0") != "" {
			return 0, fmt.Errorf("bankimport: amount %q has more than %d decimal places", orig, digits)
		}
		frac = frac[:digits]
	}
	frac += strings.Repeat("0", digits-len(frac))
	v, err := strconv.ParseInt(intPart+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bankimport: invalid amount %q", orig)
	}
	if neg {
		v = -v
	}
	return v, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

var dateLayouts = []string{"2006-01-02", "02.01.2006", "2.1.2006", "02/01/2006", "2/1/2006", "20060102", "02.01.06", "2.1.06", "02-01-2006"}

var isoPrefixRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// ParseDate parses a bank date into YYYY-MM-DD. layout is a Go time layout;
// "" tries the usual Czech and ISO formats and ignores a trailing time or
// time zone ("2016-08-03+0200", "31/01/2019 00:00:00").
func ParseDate(s, layout string) (string, error) {
	s = strings.TrimSpace(s)
	if layout != "" {
		t, err := time.Parse(layout, s)
		if err != nil {
			return "", fmt.Errorf("bankimport: invalid date %q", s)
		}
		return t.Format(time.DateOnly), nil
	}
	if m := isoPrefixRe.FindString(s); m != "" {
		s = m
	}
	s = strings.ReplaceAll(s, ". ", ".")
	if i := strings.IndexAny(s, " T"); i > 0 {
		s = s[:i]
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format(time.DateOnly), nil
		}
	}
	return "", fmt.Errorf("bankimport: invalid date %q", s)
}

// symbol strips white space and leading zeros ("0000001234" → "1234", "0" → "").
func symbol(s string) string {
	return strings.TrimLeft(strings.Join(strings.Fields(s), ""), "0")
}

// FormatAccount joins a Czech account number and bank code into
// "prefix-number/bank". A 16-digit zero-padded number (ABO) is split into the
// 6-digit prefix and the 10-digit number; an all-zero number gives "".
// Numbers that already contain "/" or are IBANs are returned as they are.
func FormatAccount(number, bank string) string {
	number = strings.Join(strings.Fields(number), "")
	bank = strings.TrimSpace(bank)
	if number == "" || strings.Trim(number, "0-") == "" {
		return ""
	}
	if strings.Contains(number, "/") || !isCzechNumber(number) {
		return number
	}
	if len(number) == 16 && allDigits(number) {
		prefix, num := strings.TrimLeft(number[:6], "0"), strings.TrimLeft(number[6:], "0")
		number = num
		if prefix != "" {
			number = prefix + "-" + num
		}
	} else if p, n, ok := strings.Cut(number, "-"); ok {
		number = strings.TrimLeft(n, "0")
		if p = strings.TrimLeft(p, "0"); p != "" {
			number = p + "-" + number
		}
	} else {
		number = strings.TrimLeft(number, "0")
	}
	if bank != "" && strings.Trim(bank, "0") != "" {
		return number + "/" + bank
	}
	return number
}

func isCzechNumber(s string) bool {
	p, n, ok := strings.Cut(s, "-")
	if !ok {
		return allDigits(s)
	}
	return allDigits(p) && allDigits(n)
}

// squash trims and collapses white space.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// fillHashIDs gives transactions without an ExternalID a deterministic one
// derived from their content; identical rows in one file get an occurrence
// counter so they stay distinct but re-importing the file yields the same IDs.
func fillHashIDs(txs []Transaction) {
	seen := map[string]int{}
	for i := range txs {
		t := &txs[i]
		if t.ExternalID != "" {
			continue
		}
		key := strings.Join([]string{t.BookedOn, strconv.FormatInt(t.Amount, 10), t.Currency, t.CounterpartyAccount,
			t.CounterpartyName, t.VS, t.KS, t.SS, t.Message}, "\x1f")
		seen[key]++
		sum := sha256.Sum256([]byte(key + "\x1f" + strconv.Itoa(seen[key])))
		t.ExternalID = "h:" + hex.EncodeToString(sum[:12])
	}
}
