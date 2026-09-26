package bankimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ABO/GPC ("Gemini Payment Center") is the fixed-width Czech statement format:
// 128-character lines (CRLF), Windows-1250 (older exports CP852). Positions
// below are 1-based and inclusive, as in the banks' documentation.
//
//	074 header:      4–19 own account, 20–39 name, 40–45 old balance date DDMMYY,
//	                 46–59 old balance, 60 sign, 61–74 new balance, 75 sign,
//	                 76–89 debit turnover, 90 sign, 91–104 credit turnover, 105 sign,
//	                 106–108 statement no., 109–114 statement date DDMMYY
//	075 transaction: 4–19 own account, 20–35 counter-account (6 prefix + 10 number),
//	                 36–48 transaction ID, 49–60 amount (2 decimals, unsigned),
//	                 61 posting code (1 debit, 2 credit, 4 storno debit, 5 storno credit),
//	                 62–71 VS, 72–81 "00"+bank code(4)+KS(4), 82–91 SS,
//	                 92–97 value date, 98–117 counterparty name / description,
//	                 118–122 numeric currency code ("00203"), 123–128 booking date
//	076 (optional):  4–29 bank reference, 30–35 date, 36–127 counterparty name / note
//	078, 079 (opt.): message for the recipient in 35-char chunks at 4–38 and 39–73;
//	                 for foreign payments 078 holds "amount currency rate" + IBAN
//	                 (39–73) + BIC, and 079 the message (4–38) and name (39–73).

var numericCurrency = map[string]string{
	"203": "CZK", "978": "EUR", "840": "USD", "826": "GBP", "756": "CHF", "985": "PLN", "348": "HUF",
	"392": "JPY", "036": "AUD", "124": "CAD", "156": "CNY", "208": "DKK", "578": "NOK", "752": "SEK",
	"946": "RON", "949": "TRY", "191": "HRK", "643": "RUB", "975": "BGN",
}

var foreignInfoRe = regexp.MustCompile(`^\d+,\d{2} [A-Z]{3}\b`)

type gpcLine []rune

// f returns the trimmed field at 1-based inclusive positions from–to.
func (l gpcLine) f(from, to int) string {
	if from > len(l) {
		return ""
	}
	if to > len(l) {
		to = len(l)
	}
	return strings.TrimSpace(string(l[from-1 : to]))
}

// raw is like f without trimming (message chunks are concatenated as-is).
func (l gpcLine) raw(from, to int) string {
	if from > len(l) {
		return ""
	}
	if to > len(l) {
		to = len(l)
	}
	return string(l[from-1 : to])
}

func gpcDate(s string) (string, bool) {
	if len(s) != 6 || strings.Trim(s, "0") == "" {
		return "", false
	}
	t, err := time.Parse("020106", s)
	if err != nil {
		return "", false
	}
	return t.Format(time.DateOnly), true
}

// gpcMoney converts an unsigned amount with 2 implied decimals into minor units of currency.
func gpcMoney(s, currency string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	switch minorDigits(currency) {
	case 0:
		return v / 100, nil
	case 3:
		return v * 10, nil
	}
	return v, nil
}

// ParseGPC parses an ABO/GPC statement file (several merged statements are
// allowed). enc is usually EncodingAuto.
func ParseGPC(data []byte, enc Encoding) (*Statement, error) {
	text, _, err := Decode(data, enc)
	if err != nil {
		return nil, err
	}
	st := &Statement{Transactions: []Transaction{}}
	var cur *Transaction // last 075, receiving 076/078/079
	var msg078, msg079 gpcLine
	headerDate := ""
	flush := func() {
		if cur == nil {
			return
		}
		applyGPCMessages(cur, msg078, msg079)
		st.Transactions = append(st.Transactions, *cur)
		cur, msg078, msg079 = nil, nil, nil
	}
	seenHeader := false
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		l := gpcLine(line)
		switch l.f(1, 3) {
		case "074":
			flush()
			seenHeader = true
			if st.Account == "" {
				st.Account = FormatAccount(l.f(4, 19), "")
				if v, err := strconv.ParseInt(l.f(46, 59), 10, 64); err == nil {
					if l.f(60, 60) == "-" {
						v = -v
					}
					st.OpeningBalance = &v
				}
			}
			if v, err := strconv.ParseInt(l.f(61, 74), 10, 64); err == nil {
				if l.f(75, 75) == "-" {
					v = -v
				}
				st.ClosingBalance = &v
			}
			headerDate, _ = gpcDate(l.f(109, 114))
		case "075":
			flush()
			t, err := parseGPC075(l, headerDate)
			if err != nil {
				return nil, fmt.Errorf("%w: GPC line %d: %v", ErrFormat, n+1, err)
			}
			cur = t
		case "076":
			if cur != nil {
				if name := squash(l.f(36, 127)); name != "" {
					cur.CounterpartyName = name
				}
			}
		case "078":
			msg078 = l
		case "079":
			msg079 = l
		default:
			if !seenHeader {
				return nil, fmt.Errorf("%w: not a GPC file", ErrFormat)
			}
		}
	}
	flush()
	if !seenHeader && len(st.Transactions) == 0 {
		return nil, fmt.Errorf("%w: not a GPC file", ErrFormat)
	}
	if len(st.Transactions) > 0 {
		st.Currency = st.Transactions[0].Currency
	}
	fillHashIDs(st.Transactions)
	return st, nil
}

func parseGPC075(l gpcLine, headerDate string) (*Transaction, error) {
	if len(l) < 122 {
		return nil, fmt.Errorf("075 record too short (%d characters)", len(l))
	}
	t := &Transaction{Currency: "CZK"}
	if c, ok := numericCurrency[strings.TrimLeft(l.f(118, 122), "0")]; ok {
		t.Currency = c
	} else if c, ok := numericCurrency[l.f(120, 122)]; ok {
		t.Currency = c
	}
	amount, err := gpcMoney(l.f(49, 60), t.Currency)
	if err != nil {
		return nil, fmt.Errorf("invalid amount %q", l.f(49, 60))
	}
	switch l.f(61, 61) {
	case "1", "5": // debit, storno of a credit
		amount = -amount
	case "2", "4": // credit, storno of a debit
	default:
		return nil, fmt.Errorf("unknown posting code %q", l.f(61, 61))
	}
	t.Amount = amount
	ks := l.f(72, 81)
	bank := ""
	if len(ks) == 10 {
		bank, ks = ks[2:6], ks[6:]
	}
	t.CounterpartyAccount = FormatAccount(l.f(20, 35), bank)
	t.ExternalID = strings.TrimLeft(l.f(36, 48), "0")
	t.VS, t.KS, t.SS = symbol(l.f(62, 71)), symbol(ks), symbol(l.f(82, 91))
	t.CounterpartyName = squash(l.f(98, 117))
	var ok bool
	if t.BookedOn, ok = gpcDate(l.f(123, 128)); !ok {
		if t.BookedOn, ok = gpcDate(l.f(92, 97)); !ok {
			t.BookedOn = headerDate
		}
	}
	if t.BookedOn == "" {
		return nil, fmt.Errorf("missing booking date")
	}
	return t, nil
}

func applyGPCMessages(t *Transaction, m078, m079 gpcLine) {
	if m078 != nil && t.CounterpartyAccount == "" && foreignInfoRe.MatchString(m078.f(4, 38)) {
		// foreign / SEPA payment
		t.CounterpartyAccount = strings.ReplaceAll(m078.f(39, 73), " ", "")
		if m079 != nil {
			t.Message = squash(m079.f(4, 38))
			if name := squash(m079.f(39, 73)); name != "" && t.CounterpartyName == "" {
				t.CounterpartyName = name
			}
		}
		return
	}
	var b strings.Builder
	for _, m := range []gpcLine{m078, m079} {
		if m != nil {
			b.WriteString(m.raw(4, 38))
			b.WriteString(m.raw(39, 73))
		}
	}
	t.Message = squash(b.String())
}
