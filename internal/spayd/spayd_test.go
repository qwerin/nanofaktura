package spayd

import "testing"

func TestParseAccount(t *testing.T) {
	tests := []struct {
		in, iban string
		ok       bool
	}{
		{"19-2000145399/0800", "CZ6508000000192000145399", true},
		{"2000145399/0800", "CZ7908000000002000145399", true},
		{" 2000145399 / 0800 ", "CZ7908000000002000145399", true},
		{"2000145398/0800", "", false}, // mod-11
		{"20-2000145399/0800", "", false},
		{"2000145399/80", "", false},
		{"abc", "", false},
	}
	for _, tt := range tests {
		a, err := ParseAccount(tt.in)
		if (err == nil) != tt.ok {
			t.Fatalf("%q: err=%v, want ok=%v", tt.in, err, tt.ok)
		}
		if tt.ok && a.IBAN() != tt.iban {
			t.Errorf("%q: IBAN=%s, want %s", tt.in, a.IBAN(), tt.iban)
		}
		if tt.ok && !ValidIBAN(a.IBAN()) {
			t.Errorf("%q: generated IBAN fails validation", tt.in)
		}
	}
	if a, _ := ParseAccount("2000145399/0800"); a.SWIFT() != "GIBACZPX" {
		t.Errorf("SWIFT=%q", a.SWIFT())
	}
}

func TestValidIBAN(t *testing.T) {
	for in, want := range map[string]bool{
		"CZ65 0800 0000 1920 0014 5399": true,
		"DE89370400440532013000":        true,
		"DE89370400440532013001":        false,
		"CZ65":                          false,
		"CZ65080000001920001453!9":      false,
	} {
		if got := ValidIBAN(in); got != want {
			t.Errorf("ValidIBAN(%q)=%v, want %v", in, got, want)
		}
	}
}

func TestBuild(t *testing.T) {
	got := Build(Payment{
		IBAN: "CZ6508000000192000145399", SWIFT: "gibaczpx", Amount: 123456,
		VariableSymbol: "2026-0042", DueOn: "2026-10-10", Message: "Faktura *2026-0042", RecipientName: "Jan Novák",
	})
	want := "SPD*1.0*ACC:CZ6508000000192000145399+GIBACZPX*AM:1234.56*CC:CZK*DT:20261010*MSG:Faktura 2026-0042*RN:Jan Novák*X-VS:20260042"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	for name, p := range map[string]Payment{
		"foreign IBAN": {IBAN: "DE89370400440532013000", Amount: 100},
		"zero amount":  {IBAN: "CZ6508000000192000145399"},
		"bad IBAN":     {IBAN: "CZ6508000000192000145390", Amount: 100},
	} {
		if s := Build(p); s != "" {
			t.Errorf("%s: want empty, got %s", name, s)
		}
	}
}

// Money M-09 / tax L-05: AM has at most 10 characters (9 999 999.99).
func TestBuildAmountLimit(t *testing.T) {
	p := Payment{IBAN: "CZ6508000000192000145399", Amount: MaxAmount}
	if Build(p) == "" {
		t.Fatal("max amount rejected")
	}
	p.Amount++
	if s := Build(p); s != "" {
		t.Fatalf("over the limit: %s", s)
	}
}

func TestDigits(t *testing.T) {
	if d := Digits("FV-2026/000123456", 10); d != "6000123456" {
		t.Errorf("got %s", d)
	}
}
