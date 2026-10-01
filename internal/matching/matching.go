// Package matching pairs bank transactions with unpaid documents (SPEC §7.6).
// It is pure: the caller loads the candidates (open invoices for incoming
// payments, unpaid expenses for outgoing ones) and stores the outcome.
package matching

import (
	"sort"
	"strings"
	"unicode"

	"github.com/qwerin/nanofaktura/internal/search"
	"github.com/qwerin/nanofaktura/internal/spayd"
)

// Reasons shown to the user (UI is Czech).
const (
	ReasonVS            = "VS sedí"
	ReasonAmount        = "Částka sedí"
	ReasonAmountTotal   = "Částka odpovídá celkové částce dokladu"
	ReasonPartial       = "Částečná úhrada"
	ReasonName          = "Jméno protistrany sedí"
	ReasonAccount       = "Účet protistrany sedí"
	maxSuggestions      = 3
	minSuggestionScore  = 20
	scoreVS             = 50
	scoreAmount         = 40
	scoreAmountTotal    = 20
	scorePartial        = 10
	scoreName           = 15
	scoreAccount        = 30
	nameSimilarityLimit = 0.5
)

// Tx is the transaction being matched.
type Tx struct {
	Amount              int64  // signed minor units; the absolute value is compared
	Currency            string // ISO 4217
	VS                  string
	CounterpartyAccount string // "prefix-number/bank" or IBAN
	CounterpartyName    string
}

// Candidate is an unpaid document (invoice, proforma or expense).
type Candidate struct {
	ID        uint
	Number    string
	Currency  string
	VS        string
	Total     int64    // document total (absolute value is compared)
	Remaining int64    // total − paid; candidates with Remaining ≤ 0 are skipped
	Name      string   // client_name / supplier_name
	Accounts  []string // known accounts of the counterparty (Czech number or IBAN)
}

// Suggestion is a scored candidate.
type Suggestion struct {
	Candidate Candidate
	Score     int
	Reasons   []string
	vs, exact bool
}

// Result of matching one transaction.
type Result struct {
	// Auto is the candidate to pay automatically: the only candidate whose
	// VS matches exactly and whose remaining amount equals the transaction.
	Auto *Candidate
	// Suggestions are the best (max 3) candidates by score (Auto included,
	// so a caller that must not auto-match can offer it instead).
	Suggestions []Suggestion
}

// Match scores the candidates for tx. Only candidates in tx's currency with
// a positive remaining amount are considered.
func Match(tx Tx, cands []Candidate) Result {
	amount := abs(tx.Amount)
	vs := NormalizeVS(tx.VS)
	txAcc := NormalizeAccount(tx.CounterpartyAccount)
	txName := NormalizeName(tx.CounterpartyName)

	var scored []Suggestion
	for _, c := range cands {
		if c.Remaining <= 0 || !strings.EqualFold(c.Currency, tx.Currency) || amount == 0 {
			continue
		}
		s := Suggestion{Candidate: c}
		if vs != "" && vs == NormalizeVS(c.VS) {
			s.vs = true
			s.Score += scoreVS
			s.Reasons = append(s.Reasons, ReasonVS)
		}
		switch {
		case amount == c.Remaining:
			s.exact = true
			s.Score += scoreAmount
			s.Reasons = append(s.Reasons, ReasonAmount)
		case amount == abs(c.Total):
			s.Score += scoreAmountTotal
			s.Reasons = append(s.Reasons, ReasonAmountTotal)
		case s.vs && amount < c.Remaining:
			s.Score += scorePartial
			s.Reasons = append(s.Reasons, ReasonPartial)
		}
		if txAcc != "" {
			for _, a := range c.Accounts {
				if NormalizeAccount(a) == txAcc {
					s.Score += scoreAccount
					s.Reasons = append(s.Reasons, ReasonAccount)
					break
				}
			}
		}
		if txName != "" && NameSimilarity(txName, NormalizeName(c.Name)) >= nameSimilarityLimit {
			s.Score += scoreName
			s.Reasons = append(s.Reasons, ReasonName)
		}
		if s.Score >= minSuggestionScore {
			scored = append(scored, s)
		}
	}

	var auto []int
	for i, s := range scored {
		if s.vs && s.exact {
			auto = append(auto, i)
		}
	}
	var res Result
	if len(auto) == 1 {
		c := scored[auto[0]].Candidate
		res.Auto = &c
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Candidate.ID < scored[j].Candidate.ID
	})
	if len(scored) > maxSuggestions {
		scored = scored[:maxSuggestions]
	}
	res.Suggestions = scored
	return res
}

// NormalizeVS strips white space and leading zeros.
func NormalizeVS(s string) string {
	return strings.TrimLeft(strings.Join(strings.Fields(s), ""), "0")
}

// NormalizeAccount converts a Czech account number to its IBAN and returns
// IBANs / other numbers upper-case without spaces; leading zeros of the
// Czech parts do not matter.
func NormalizeAccount(s string) string {
	s = strings.ToUpper(strings.Join(strings.Fields(s), ""))
	if s == "" {
		return ""
	}
	if a, err := spayd.ParseAccount(s); err == nil {
		return a.IBAN()
	}
	// Czech-looking number failing the checksum: compare without leading zeros
	if num, bank, ok := strings.Cut(s, "/"); ok {
		prefix, n, hasPrefix := strings.Cut(num, "-")
		if !hasPrefix {
			prefix, n = "", num
		}
		prefix, n = strings.TrimLeft(prefix, "0"), strings.TrimLeft(n, "0")
		if prefix != "" {
			n = prefix + "-" + n
		}
		return n + "/" + bank
	}
	return s
}

// legal-form tokens ignored when comparing names
var legalForms = map[string]bool{
	"sro": true, "spol": true, "s": true, "r": true, "o": true, "as": true, "a": true, "vos": true, "v": true,
	"ks": true, "k": true, "zs": true, "z": true, "ops": true, "p": true, "se": true, "gmbh": true, "ltd": true,
	"inc": true, "llc": true, "ag": true, "sa": true, "bv": true, "co": true, "ing": true, "mgr": true, "bc": true,
	"sp": true, "zoo": true, "ro": true,
}

// NormalizeName lower-cases, removes diacritics and punctuation and drops
// legal forms and titles ("Žluťoučký kůň s.r.o." → "zlutoucky kun").
func NormalizeName(s string) string {
	s = search.StripMarks(strings.ToLower(s))
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if r == '.' {
			return -1 // "s.r.o." → "sro"
		}
		return ' '
	}, s)
	var out []string
	for _, w := range strings.Fields(s) {
		if !legalForms[w] {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}

// NameSimilarity is the share of the shorter name's words found in the
// other name (0–1); both arguments are NormalizeName outputs. Word order does
// not matter ("Novák Jan" = "Jan Novák").
func NameSimilarity(a, b string) float64 {
	wa, wb := strings.Fields(a), strings.Fields(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	if len(wa) > len(wb) {
		wa, wb = wb, wa
	}
	set := make(map[string]bool, len(wb))
	for _, w := range wb {
		set[w] = true
	}
	hit := 0
	for _, w := range wa {
		if set[w] {
			hit++
		}
	}
	return float64(hit) / float64(len(wa))
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
