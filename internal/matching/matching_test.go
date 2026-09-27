package matching

import (
	"reflect"
	"testing"
)

func TestMatch(t *testing.T) {
	inv := func(id uint, vs string, total, remaining int64, name string, accounts ...string) Candidate {
		return Candidate{ID: id, Number: "2026-000" + vs, Currency: "CZK", VS: vs, Total: total, Remaining: remaining, Name: name, Accounts: accounts}
	}
	type want struct {
		auto    uint // 0 = none
		ids     []uint
		reasons [][]string
	}
	for _, tc := range []struct {
		name  string
		tx    Tx
		cands []Candidate
		want  want
	}{
		{
			name:  "VS and remaining amount → auto",
			tx:    Tx{Amount: 12100, Currency: "CZK", VS: "0020261"},
			cands: []Candidate{inv(1, "20261", 12100, 12100, "ACME"), inv(2, "20262", 12100, 12100, "ACME")},
			want:  want{auto: 1},
		},
		{
			name:  "auto after partial payment (remaining matches)",
			tx:    Tx{Amount: 2100, Currency: "CZK", VS: "1"},
			cands: []Candidate{inv(1, "1", 12100, 2100, "ACME")},
			want:  want{auto: 1},
		},
		{
			name:  "outgoing amounts are compared by absolute value",
			tx:    Tx{Amount: -500, Currency: "CZK", VS: "77"},
			cands: []Candidate{inv(5, "77", 500, 500, "Dodavatel")},
			want:  want{auto: 5},
		},
		{
			name:  "two candidates with same VS and amount → suggestions only",
			tx:    Tx{Amount: 100, Currency: "CZK", VS: "5"},
			cands: []Candidate{inv(1, "5", 100, 100, "A"), inv(2, "5", 100, 100, "B")},
			want:  want{ids: []uint{1, 2}, reasons: [][]string{{ReasonVS, ReasonAmount}, {ReasonVS, ReasonAmount}}},
		},
		{
			name:  "partial payment with VS → suggestion",
			tx:    Tx{Amount: 5000, Currency: "CZK", VS: "9"},
			cands: []Candidate{inv(1, "9", 12100, 12100, "ACME")},
			want:  want{ids: []uint{1}, reasons: [][]string{{ReasonVS, ReasonPartial}}},
		},
		{
			name:  "amount equals total but not remaining",
			tx:    Tx{Amount: 12100, Currency: "CZK", VS: "9"},
			cands: []Candidate{inv(1, "9", 12100, 2100, "ACME")},
			want:  want{ids: []uint{1}, reasons: [][]string{{ReasonVS, ReasonAmountTotal}}},
		},
		{
			name: "amount only, name and account ranking",
			tx: Tx{Amount: 300, Currency: "CZK", CounterpartyName: "ŽLUŤOUČKÝ KŮŇ, s.r.o.",
				CounterpartyAccount: "0000123456789/0100"},
			cands: []Candidate{
				inv(1, "1", 300, 300, "Jiná firma"),
				inv(2, "2", 300, 300, "Žluťoučký kůň s.r.o."),
				inv(3, "3", 300, 300, "Někdo", "123456789/0100"),
				inv(4, "4", 300, 300, "Další"),
			},
			want: want{ids: []uint{3, 2, 1}, reasons: [][]string{{ReasonAmount, ReasonAccount}, {ReasonAmount, ReasonName}, {ReasonAmount}}},
		},
		{
			name: "IBAN of counterparty matches Czech number",
			tx:   Tx{Amount: 1, Currency: "CZK", VS: "8", CounterpartyAccount: "CZ65 0800 0000 1920 0014 5399"},
			cands: []Candidate{
				inv(1, "8", 1000, 1000, "X", "19-2000145399/0800"),
			},
			want: want{ids: []uint{1}, reasons: [][]string{{ReasonVS, ReasonPartial, ReasonAccount}}},
		},
		{
			name:  "other currency, paid or VS-less weak candidates ignored",
			tx:    Tx{Amount: 100, Currency: "EUR", VS: "1", CounterpartyName: "ACME"},
			cands: []Candidate{inv(1, "1", 100, 100, "ACME"), {ID: 2, Currency: "EUR", VS: "1", Total: 100, Remaining: 0}, {ID: 3, Currency: "EUR", Total: 999, Remaining: 999, Name: "ACME"}},
			want:  want{},
		},
		{
			name:  "no VS in transaction → never auto",
			tx:    Tx{Amount: 100, Currency: "CZK"},
			cands: []Candidate{inv(1, "", 100, 100, "A")},
			want:  want{ids: []uint{1}, reasons: [][]string{{ReasonAmount}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Match(tc.tx, tc.cands)
			if tc.want.auto != 0 {
				if r.Auto == nil || r.Auto.ID != tc.want.auto || len(r.Suggestions) == 0 || r.Suggestions[0].Candidate.ID != tc.want.auto {
					t.Fatalf("auto: %+v", r)
				}
				return
			}
			if r.Auto != nil {
				t.Fatalf("unexpected auto %+v", r.Auto)
			}
			var ids []uint
			var reasons [][]string
			for _, s := range r.Suggestions {
				ids = append(ids, s.Candidate.ID)
				reasons = append(reasons, s.Reasons)
			}
			if !reflect.DeepEqual(ids, tc.want.ids) || !reflect.DeepEqual(reasons, tc.want.reasons) {
				t.Fatalf("got %v %v, want %v %v", ids, reasons, tc.want.ids, tc.want.reasons)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Žluťoučký kůň s.r.o.":        "zlutoucky kun",
		"ŽLUŤOUČKÝ KŮŇ, spol. s r.o.": "zlutoucky kun",
		"Ing. Jan Novák":              "jan novak",
		"ACME GmbH":                   "acme",
		"":                            "",
	} {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", in, got, want)
		}
	}
	if s := NameSimilarity("novak jan", "jan novak"); s != 1 {
		t.Errorf("similarity %v", s)
	}
	if s := NameSimilarity("jan novak", "petr novak"); s != 0.5 {
		t.Errorf("similarity %v", s)
	}
	if NormalizeVS(" 000123 ") != "123" || NormalizeVS("0") != "" {
		t.Error("NormalizeVS")
	}
	if NormalizeAccount("19-2000145399/0800") != "CZ6508000000192000145399" || NormalizeAccount("de89 3704 0044 0532 0130 00") != "DE89370400440532013000" {
		t.Error("NormalizeAccount")
	}
}
