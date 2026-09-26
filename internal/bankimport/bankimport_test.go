package bankimport

import (
	"errors"
	"os"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseAmount(t *testing.T) {
	for _, tc := range []struct {
		in   string
		sep  rune
		cur  string
		want int64
	}{
		{"12100,00", 0, "CZK", 1210000},
		{"-2 500,50", 0, "CZK", -250050},
		{"-2 500,5", 0, "CZK", -250050},
		{"1.234,56", 0, "EUR", 123456},
		{"1,234.56", 0, "USD", 123456},
		{"+12.5", 0, "CZK", 1250},
		{"-130.0", '.', "CZK", -13000},
		{"100.00-", 0, "CZK", -10000},
		{"1,234,567", 0, "CZK", 123456700},
		{"431", 0, "CZK", 43100},
		{"1500", 0, "JPY", 1500},
		{"1500,00", 0, "JPY", 1500},
		{"1,5 CZK", 0, "CZK", 150},
		{"12,345", 0, "KWD", 12345},
		{",5", 0, "CZK", 50},
	} {
		got, err := ParseAmount(tc.in, tc.sep, tc.cur)
		if err != nil || got != tc.want {
			t.Errorf("ParseAmount(%q) = %d, %v; want %d", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "abc", "1,234", "12.345.6,7,8", "1,001", "--5"} {
		if v, err := ParseAmount(bad, 0, "CZK"); err == nil {
			t.Errorf("ParseAmount(%q) = %d, want error", bad, v)
		}
	}
}

func TestParseDate(t *testing.T) {
	for in, want := range map[string]string{
		"2016-08-03+0200": "2016-08-03", "02.09.2026": "2026-09-02", "2.9.2026": "2026-09-02", "2. 9. 2026": "2026-09-02",
		"31/01/2019": "2019-01-31", "31/01/2019 00:00:00": "2019-01-31", "20260902": "2026-09-02", "2026-09-02T10:00:00Z": "2026-09-02",
	} {
		if got, err := ParseDate(in, ""); err != nil || got != want {
			t.Errorf("ParseDate(%q) = %q %v", in, got, err)
		}
	}
	if got, err := ParseDate("09/02/2026", "01/02/2006"); err != nil || got != "2026-09-02" {
		t.Errorf("layout: %q %v", got, err)
	}
	if _, err := ParseDate("32.13.2026", ""); err == nil {
		t.Error("expected error")
	}
}

func TestFormatAccount(t *testing.T) {
	for _, tc := range []struct{ num, bank, want string }{
		{"0000192000145399", "0800", "19-2000145399/0800"},
		{"0000000123456789", "0100", "123456789/0100"},
		{"0000000000000000", "0000", ""},
		{"123456789", "0100", "123456789/0100"},
		{"000019-0002000145399", "0800", "19-2000145399/0800"},
		{"2000000000/2010", "", "2000000000/2010"},
		{"DE89370400440532013000", "", "DE89370400440532013000"},
		{"123456789", "", "123456789"},
		{"", "0100", ""},
	} {
		if got := FormatAccount(tc.num, tc.bank); got != tc.want {
			t.Errorf("FormatAccount(%q, %q) = %q, want %q", tc.num, tc.bank, got, tc.want)
		}
	}
}

func TestDecode(t *testing.T) {
	const s = "Žluťoučký kůň úpěl ďábelské ódy"
	for _, enc := range []Encoding{EncodingWindows1250, EncodingCP852} {
		gpc := fixture(t, "vypis.gpc")
		if enc == EncodingCP852 {
			gpc = fixture(t, "vypis_cp852.gpc")
		}
		_, got, err := Decode(gpc, EncodingAuto)
		if err != nil || got != enc {
			t.Errorf("detected %q, want %q (%v)", got, enc, err)
		}
	}
	if out, enc, _ := Decode([]byte("\xef\xbb\xbf"+s), EncodingAuto); out != s || enc != EncodingUTF8 {
		t.Errorf("utf-8: %q %q", out, enc)
	}
	if _, _, err := Decode(nil, "latin1"); err == nil {
		t.Error("unsupported encoding accepted")
	}
}

func checkTx(t *testing.T, got Transaction, want Transaction) {
	t.Helper()
	if got != want {
		t.Errorf("transaction\n got %+v\nwant %+v", got, want)
	}
}

func TestParseGPC(t *testing.T) {
	for _, file := range []string{"vypis.gpc", "vypis_cp852.gpc"} {
		st, err := ParseGPC(fixture(t, file), EncodingAuto)
		if err != nil {
			t.Fatal(file, err)
		}
		if st.Account != "19-2000145399" || *st.OpeningBalance != 1543210 || *st.ClosingBalance != 2610710 || len(st.Transactions) != 5 {
			t.Fatalf("%s: statement %+v", file, st)
		}
		tx := st.Transactions
		checkTx(t, tx[0], Transaction{ExternalID: "1000000123456", BookedOn: "2026-09-25", Amount: 1200000, Currency: "CZK",
			CounterpartyAccount: "123456789/0800", CounterpartyName: "Žluťoučký kůň s.r.o., Příčná 1, Praha",
			VS: "2026001", KS: "308", Message: "Faktura 2026001 - platba za webhosting a domenu nanofaktura."})
		checkTx(t, tx[1], Transaction{ExternalID: "1000000123457", BookedOn: "2026-09-25", Amount: -32500, Currency: "CZK",
			CounterpartyAccount: "35-123457/0100", CounterpartyName: "Úhrada nájmu", VS: "1234567890", KS: "558", SS: "77"})
		checkTx(t, tx[2], Transaction{ExternalID: "1000000123458", BookedOn: "2026-09-25", Amount: -100000, Currency: "CZK",
			CounterpartyName: "Poplatek za vedení"})
		checkTx(t, tx[3], Transaction{ExternalID: "1000000123459", BookedOn: "2026-09-25", Amount: 25000, Currency: "EUR",
			CounterpartyAccount: "DE89370400440532013000", CounterpartyName: "ACME GMBH", Message: "INVOICE 2026-17"})
		if tx[4].Amount != -5000 || tx[4].VS != "2026001" { // storno of a credit
			t.Errorf("storno %+v", tx[4])
		}
	}
	if _, err := ParseGPC([]byte("hello\nworld\n"), EncodingAuto); !errors.Is(err, ErrFormat) {
		t.Errorf("garbage: %v", err)
	}
	if _, err := ParseGPC([]byte("0750000192000145399\r\n"), EncodingAuto); !errors.Is(err, ErrFormat) {
		t.Errorf("short 075: %v", err)
	}
}

func TestParseFioJSON(t *testing.T) {
	st, err := ParseFioJSON(fixture(t, "fio_transactions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Account != "2000000000/2010" || st.IBAN != "CZ7920100000002000000000" || st.Currency != "CZK" ||
		*st.OpeningBalance != 1543210 || *st.ClosingBalance != 1025432100 || len(st.Transactions) != 4 {
		t.Fatalf("statement %+v", st)
	}
	tx := st.Transactions
	checkTx(t, tx[0], Transaction{ExternalID: "26543210001", BookedOn: "2026-09-02", Amount: 1210000, Currency: "CZK",
		CounterpartyAccount: "123456789/0100", CounterpartyName: "Žluťoučký kůň s.r.o.", VS: "2026001", KS: "308",
		Message: "Faktura 2026001"})
	checkTx(t, tx[1], Transaction{ExternalID: "26543210002", BookedOn: "2026-09-03", Amount: -13000, Currency: "CZK",
		Message: "Nákup: ORDR, PRAGUE, CZ, dne 1.9.2026, částka 130.00 CZK"})
	checkTx(t, tx[2], Transaction{ExternalID: "26543210003", BookedOn: "2026-09-10", Amount: -250050, Currency: "CZK",
		CounterpartyAccount: "19-2000145399/0800", CounterpartyName: "Jan Novák", VS: "1234", SS: "42", Message: "Nájem září"})
	checkTx(t, tx[3], Transaction{ExternalID: "26543210004", BookedOn: "2026-09-25", Amount: 1000000000, Currency: "CZK",
		CounterpartyAccount: "DE89370400440532013000", CounterpartyName: "ACME GmbH", Message: "INVOICE 2026-17"})

	empty, err := ParseFioJSON(fixture(t, "fio_empty.json"))
	if err != nil || len(empty.Transactions) != 0 || empty.Transactions == nil {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	for _, bad := range []string{"", "{}", "[1]", `{"accountStatement":{"transactionList":{"transaction":[{"column0":{"value":"x"},"column1":{"value":1}}]}}}`} {
		if _, err := ParseFioJSON([]byte(bad)); !errors.Is(err, ErrFormat) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestParseFioCSV(t *testing.T) {
	data := fixture(t, "fio.csv")
	if f := Detect(data); f != FormatFioCSV {
		t.Fatalf("detected %q", f)
	}
	st, err := ParseFioCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Transactions) != 4 {
		t.Fatalf("%d transactions", len(st.Transactions))
	}
	tx := st.Transactions
	checkTx(t, tx[0], Transaction{ExternalID: "26543210001", BookedOn: "2026-09-02", Amount: 1210000, Currency: "CZK",
		CounterpartyAccount: "123456789/0100", CounterpartyName: "Žluťoučký kůň s.r.o.", VS: "2026001", KS: "308",
		Message: "Faktura 2026001"})
	checkTx(t, tx[2], Transaction{ExternalID: "26543210003", BookedOn: "2026-09-10", Amount: -250050, Currency: "CZK",
		CounterpartyAccount: "19-2000145399/0800", CounterpartyName: "Jan Novák", VS: "1234", SS: "42", Message: "Nájem září"})
	if tx[1].Amount != -13000 || tx[1].CounterpartyAccount != "" || tx[3].Amount != 43100 {
		t.Errorf("card/interest: %+v %+v", tx[1], tx[3])
	}
}

func TestParseCSOBCSV(t *testing.T) {
	data := fixture(t, "csob.csv")
	if f := Detect(data); f != FormatCSOBCSV {
		t.Fatalf("detected %q", f)
	}
	st, err := ParseCSOBCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Transactions) != 3 {
		t.Fatalf("%d transactions", len(st.Transactions))
	}
	checkTx(t, st.Transactions[0], Transaction{ExternalID: "2026090200001", BookedOn: "2026-09-02", Amount: 1210000, Currency: "CZK",
		CounterpartyAccount: "2000000000/2010", CounterpartyName: "Žluťoučký kůň s.r.o.", VS: "2026001", KS: "308",
		Message: "Faktura 2026001"})
	checkTx(t, st.Transactions[1], Transaction{ExternalID: "2026090300002", BookedOn: "2026-09-03", Amount: -125000, Currency: "CZK",
		CounterpartyAccount: "35-123457/0100", CounterpartyName: "Pronajímatel s.r.o.", VS: "1234567890", KS: "558", SS: "77",
		Message: "Nájem 09/2026"})
	if st.Transactions[2].Amount != -9900 || st.Transactions[2].CounterpartyAccount != "" {
		t.Errorf("fee %+v", st.Transactions[2])
	}

	// newer header: no "ID transakce" → content hash IDs, identical rows stay distinct and stable
	data = fixture(t, "csob_new.csv")
	if f := Detect(data); f != FormatCSOBCSV {
		t.Fatalf("new header detected %q", f)
	}
	st, err = ParseCSOBCSV(data)
	if err != nil || len(st.Transactions) != 2 {
		t.Fatalf("new: %+v %v", st, err)
	}
	a, b := st.Transactions[0].ExternalID, st.Transactions[1].ExternalID
	if a == b || len(a) != 26 || a[:2] != "h:" {
		t.Fatalf("hash ids %q %q", a, b)
	}
	again, _ := ParseCSOBCSV(data)
	if again.Transactions[0].ExternalID != a || again.Transactions[1].ExternalID != b {
		t.Fatal("hash ids are not stable")
	}
	if st.Transactions[0].CounterpartyAccount != "2000000000/2010" || st.Transactions[0].CounterpartyName != "Žluťoučký kůň s.r.o." {
		t.Fatalf("new header columns %+v", st.Transactions[0])
	}
}

func TestParseKBCSV(t *testing.T) {
	data := fixture(t, "kb.csv")
	if f := Detect(data); f != FormatKBCSV {
		t.Fatalf("detected %q", f)
	}
	st, err := ParseKBCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Transactions) != 2 {
		t.Fatalf("%d transactions", len(st.Transactions))
	}
	checkTx(t, st.Transactions[0], Transaction{ExternalID: "123-02092026 1602 010111 000123", BookedOn: "2026-09-02", Amount: 1210000,
		Currency: "CZK", CounterpartyAccount: "2000000000/2010", CounterpartyName: "Žluťoučký kůň s.r.o.", VS: "2026001", KS: "308",
		Message: "Faktura 2026001 Platba za webhosting"})
	checkTx(t, st.Transactions[1], Transaction{ExternalID: "123-03092026 1602 010111 000456", BookedOn: "2026-09-03", Amount: -250050,
		Currency: "CZK", CounterpartyAccount: "19-2000145399/0800", CounterpartyName: "Jan Novák", VS: "1234", SS: "42", Message: "Nájem září"})

	data = fixture(t, "kb_business.csv")
	if f := Detect(data); f != FormatKBCSV {
		t.Fatalf("business detected %q", f)
	}
	st, err = ParseKBCSV(data)
	if err != nil || len(st.Transactions) != 2 {
		t.Fatalf("business: %+v %v", st, err)
	}
	checkTx(t, st.Transactions[0], Transaction{ExternalID: "123-25092026 1602 010111 000123", BookedOn: "2026-09-25", Amount: 1210000,
		Currency: "CZK", CounterpartyAccount: "2000000000/2010", CounterpartyName: "Žluťoučký kůň s.r.o.", VS: "2026001", KS: "308",
		Message: "Faktura 2026001"})
	if tx := st.Transactions[1]; tx.Amount != -250050 || tx.CounterpartyAccount != "19-2000145399/0800" || tx.SS != "42" {
		t.Errorf("business debit %+v", tx)
	}
	if _, err := ParseKBCSV(fixture(t, "csob.csv")); !errors.Is(err, ErrFormat) {
		t.Errorf("ČSOB file as KB: %v", err)
	}
}

func TestParseAirBankCSV(t *testing.T) {
	data := fixture(t, "airbank.csv")
	if f := Detect(data); f != FormatAirBankCSV {
		t.Fatalf("detected %q", f)
	}
	st, err := ParseAirBankCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Transactions) != 2 {
		t.Fatalf("%d transactions", len(st.Transactions))
	}
	checkTx(t, st.Transactions[0], Transaction{ExternalID: "36546164942", BookedOn: "2026-09-02", Amount: 1210000, Currency: "CZK",
		CounterpartyAccount: "2000000000/2010", CounterpartyName: "Žluťoučký kůň s.r.o.", VS: "2026001", KS: "308",
		Message: "Faktura 2026001"})
	checkTx(t, st.Transactions[1], Transaction{ExternalID: "26546164942", BookedOn: "2026-01-31", Amount: -4660, Currency: "CZK",
		CounterpartyName: "LIDL DEKUJE ZA NAKUP"})
}

func TestParseGenericCSV(t *testing.T) {
	data := fixture(t, "generic.csv")
	if f := Detect(data); f != FormatUnknown {
		t.Fatalf("generic file detected as %q", f)
	}
	m := CSVMapping{BookedOn: "date", Credit: "credit", Debit: "debit", CounterpartyAccount: "counterparty", VS: "vs",
		Message: []string{"description"}, DefaultCurrency: "eur"}
	st, err := ParseCSV(data, m, CSVOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Transactions) != 2 || st.Currency != "EUR" {
		t.Fatalf("%+v", st)
	}
	checkTx(t, st.Transactions[0], Transaction{ExternalID: st.Transactions[0].ExternalID, BookedOn: "2026-09-02", Amount: 12100,
		Currency: "EUR", CounterpartyAccount: "CZ6508000000192000145399", VS: "2026001", Message: "Invoice 2026001, thanks"})
	if st.Transactions[1].Amount != -125040 {
		t.Errorf("debit %+v", st.Transactions[1])
	}

	// positional mapping, explicit delimiter/decimal separator, and errors
	pos := CSVMapping{NoHeader: true, BookedOn: "#1", Amount: "#2", DateFormat: "02.01.2006"}
	st, err = ParseCSV([]byte("01.09.2026;1.500,25\n\n02.09.2026;-3\nCelkem;;\n"), pos, CSVOptions{Delimiter: ';', DecimalSep: ','})
	if err != nil || len(st.Transactions) != 2 || st.Transactions[0].Amount != 150025 || st.Transactions[1].Amount != -300 {
		t.Fatalf("positional %+v %v", st, err)
	}
	if _, err := ParseCSV([]byte("a;b\n1;2\n"), m, CSVOptions{}); !errors.Is(err, ErrFormat) {
		t.Errorf("missing header: %v", err)
	}
	if _, err := ParseCSV([]byte("date,credit\n2026-09-02,abc\n"), m, CSVOptions{}); !errors.Is(err, ErrFormat) {
		t.Errorf("bad amount: %v", err)
	}
	if _, err := ParseCSV([]byte("date,credit\nyesterday,1\n"), m, CSVOptions{}); !errors.Is(err, ErrFormat) {
		t.Errorf("bad date: %v", err)
	}
}

func TestDetectDelimiter(t *testing.T) {
	for text, want := range map[string]rune{
		"a;b;c\n1;2,5;3\n":                  ';',
		"a,b,c\n1,\"2;5\",3\n":              ',',
		"a\tb\tc\n1\t2\t3\n":                '\t',
		"\"x;y\"\na|b|c\n1|2|3\n4|5|6\n":    '|',
		"\"12,5\";\"3,2\"\n\"1,0\";\"2,0\"": ';',
	} {
		if got := DetectDelimiter(text); got != want {
			t.Errorf("%q: got %q want %q", text, got, want)
		}
	}
}

func TestParseAuto(t *testing.T) {
	for file, want := range map[string]Format{
		"vypis.gpc": FormatGPC, "vypis_cp852.gpc": FormatGPC, "fio_transactions.json": FormatFioJSON, "fio.csv": FormatFioCSV,
		"csob.csv": FormatCSOBCSV, "kb.csv": FormatKBCSV, "kb_business.csv": FormatKBCSV, "airbank.csv": FormatAirBankCSV,
	} {
		f, st, err := ParseAuto(fixture(t, file))
		if err != nil || f != want || len(st.Transactions) == 0 {
			t.Errorf("%s: %q %v", file, f, err)
		}
	}
	if _, _, err := ParseAuto(fixture(t, "generic.csv")); !errors.Is(err, ErrFormat) {
		t.Errorf("generic: %v", err)
	}
	if _, err := Parse("xml", nil); !errors.Is(err, ErrFormat) {
		t.Errorf("unknown format: %v", err)
	}
}
