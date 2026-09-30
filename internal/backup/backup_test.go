package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/model"
)

// coverage classifies every model of model.All(): the backup file it is
// exported to, or why it is not exported. A new model must be added here
// (and to Export/Import) — the test fails otherwise.
var coverage = map[string]string{
	"User":            "excluded: users are global (instance-wide); members.json lists them informatively",
	"Account":         FileAccount,
	"Membership":      FileMembers + " (informative; the importer becomes the only owner)",
	"Session":         "excluded: login sessions (secrets)",
	"APIToken":        "excluded: API tokens (secrets)",
	"BankAccount":     FileBankAccounts,
	"NumberFormat":    FileNumberFormats,
	"NumberCounter":   FileNumberFormats + " (counters of a format)",
	"Subject":         FileSubjects,
	"Invoice":         FileInvoices,
	"InvoiceLine":     FileInvoices + " (lines)",
	"Payment":         FileInvoices + " (payments)",
	"Invitation":      "excluded: invitation tokens (secrets); invite members again",
	"Attachment":      FileAttachments + " + attachments/<id>/<filename>",
	"PriceItem":       FilePriceItems,
	"StockMove":       FileStockMoves,
	"Expense":         FileExpenses,
	"ExpenseLine":     FileExpenses + " (lines)",
	"ExpensePayment":  FileExpenses + " (payments)",
	"ExchangeRate":    "excluded: global ČNB rate cache, refetched on demand",
	"InvoiceTemplate": FileTemplates,
	"Recurring":       FileRecurring,
	"EmailLog":        FileEmailLogs,
	"BankTransaction": FileBankTransactions,
	"Event":           FileEvents,
	"Todo":            FileTodos,
	"Webhook":         FileWebhooks,
	"WebhookDelivery": "excluded: delivery queue/log of the old instance (payloads with old IDs)",
}

func TestEveryModelIsCovered(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range model.All() {
		name := reflect.TypeOf(m).Elem().Name()
		seen[name] = true
		if _, ok := coverage[name]; !ok {
			t.Errorf("model %s is not covered by the backup: export/import it or add it to coverage with a reason", name)
		}
	}
	for name := range coverage {
		if !seen[name] {
			t.Errorf("coverage lists %s which is not in model.All()", name)
		}
	}
}

// fieldSkips are model fields deliberately absent from the DTOs.
var fieldSkips = map[string]string{
	"*.AccountID":                  "implied by the archive",
	"*.SearchText":                 "derived (BeforeSave hooks)",
	"BankAccount.FioToken":         "secret",
	"Webhook.Secret":               "secret",
	"Invoice.PublicToken":          "regenerated on import",
	"Attachment.StorageKey":        "instance storage detail; content is in the ZIP",
	"Event.UserID":                 "users are not transferred",
	"Todo.UserID":                  "users are not transferred",
	"InvoiceLine.InvoiceID":        "nested in the invoice",
	"ExpenseLine.ExpenseID":        "nested in the expense",
	"Payment.InvoiceID":            "nested in the invoice",
	"ExpensePayment.ExpenseID":     "nested in the expense",
	"NumberCounter.ID":             "nested in the number format",
	"NumberCounter.NumberFormatID": "nested in the number format",
}

// TestDTOsCoverModelFields fails when a model gets a field the backup DTO
// does not carry (it would silently get lost on export/import).
func TestDTOsCoverModelFields(t *testing.T) {
	pairs := []struct{ m, d any }{
		{model.Account{}, Account{}}, {model.MailTemplate{}, MailTemplate{}},
		{model.BankAccount{}, BankAccount{}}, {model.NumberFormat{}, NumberFormat{}}, {model.NumberCounter{}, NumberCounter{}},
		{model.Subject{}, Subject{}}, {model.PriceItem{}, PriceItem{}}, {model.StockMove{}, StockMove{}},
		{model.Invoice{}, Invoice{}}, {model.InvoiceLine{}, Line{}}, {model.Payment{}, Payment{}},
		{model.Expense{}, Expense{}}, {model.ExpenseLine{}, Line{}}, {model.ExpensePayment{}, Payment{}},
		{model.InvoiceTemplate{}, Template{}}, {model.TemplateLine{}, TemplateLine{}}, {model.Recurring{}, Recurring{}},
		{model.BankTransaction{}, BankTransaction{}}, {model.MatchSuggestion{}, MatchSuggestion{}},
		{model.Todo{}, Todo{}}, {model.Event{}, Event{}}, {model.EmailLog{}, EmailLog{}}, {model.Webhook{}, Webhook{}},
		{model.Attachment{}, Attachment{}},
	}
	for _, p := range pairs {
		mt, dt := reflect.TypeOf(p.m), reflect.TypeOf(p.d)
		var check func(t2 reflect.Type)
		check = func(t2 reflect.Type) {
			for i := 0; i < t2.NumField(); i++ {
				f := t2.Field(i)
				if f.Anonymous && f.Type.Kind() == reflect.Struct {
					check(f.Type)
					continue
				}
				if _, skip := fieldSkips[mt.Name()+"."+f.Name]; skip {
					continue
				}
				if _, skip := fieldSkips["*."+f.Name]; skip {
					continue
				}
				df, ok := dt.FieldByName(f.Name)
				if !ok {
					t.Errorf("%s.%s is missing in backup DTO %s (add it, or to fieldSkips with a reason)", mt.Name(), f.Name, dt.Name())
					continue
				}
				if tag := df.Tag.Get("json"); tag == "" || tag == "-" {
					t.Errorf("%s.%s has no json name", dt.Name(), df.Name)
				}
			}
		}
		check(mt)
	}
}

func TestConvertRoundtrip(t *testing.T) {
	rs := true
	vat := int32(1200)
	m := model.InvoiceTemplate{ID: 3, Name: "T", RoundTotal: &rs, Tags: []string{"a"},
		Lines: []model.TemplateLine{{Name: "L", QuantityMilli: 1500, VatRateBps: &vat}}}
	var d Template
	convert(&d, &m)
	if d.ID != 3 || *d.RoundTotal != true || d.Lines[0].QuantityMilli != 1500 || *d.Lines[0].VatRateBps != 1200 {
		t.Fatalf("to DTO: %+v", d)
	}
	var back model.InvoiceTemplate
	convert(&back, &d)
	if !reflect.DeepEqual(back, m) {
		t.Fatalf("roundtrip:\n%+v\n%+v", back, m)
	}

	acc := model.Account{Name: "A", AccountMailSettings: model.AccountMailSettings{EmailSignature: "S", RemindersEnabled: true,
		EmailTemplates: map[string]model.MailTemplate{"invoice:cs": {Subject: "x"}}}}
	var ad Account
	convert(&ad, &acc)
	if ad.EmailSignature != "S" || !ad.RemindersEnabled || ad.EmailTemplates["invoice:cs"].Subject != "x" {
		t.Fatalf("embedded to DTO: %+v", ad)
	}
	var accBack model.Account
	convert(&accBack, &ad)
	if !reflect.DeepEqual(accBack, acc) {
		t.Fatalf("embedded roundtrip:\n%+v\n%+v", accBack, acc)
	}
}

func TestSafeName(t *testing.T) {
	for name, want := range map[string]bool{
		"manifest.json": true, "attachments/3/faktura.pdf": true, "attachments/3/Účtenka (1).jpg": true,
		"../etc/passwd": false, "/abs": false, "a/../b": false, "a//b": false, "a\\b": false, "C:x": false, "./a": false, "": false,
	} {
		if got := safeName(name); got != want {
			t.Errorf("safeName(%q) = %v, want %v", name, got, want)
		}
	}
	for in, want := range map[string]string{
		"faktura.pdf": "attachments/7/faktura.pdf", "../../x.pdf": "attachments/7/x.pdf", "a\\b.pdf": "attachments/7/b.pdf",
		"..": "attachments/7/file", "": "attachments/7/file",
	} {
		if got := attachmentPath(7, in); got != want || !safeName(got) {
			t.Errorf("attachmentPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// zipOf builds an archive; manifest "files" is computed unless manifest is given raw.
func zipOf(t *testing.T, files map[string]string, man *Manifest) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if man != nil {
		if man.Files == nil {
			man.Files = map[string]FileInfo{}
			for name, content := range files {
				s := sha256.Sum256([]byte(content))
				man.Files[name] = FileInfo{SHA256: hex.EncodeToString(s[:]), Size: int64(len(content))}
			}
		}
		b, _ := json.Marshal(man)
		files = mergeFile(files, FileManifest, string(b))
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mergeFile(files map[string]string, name, content string) map[string]string {
	out := map[string]string{name: content}
	for k, v := range files {
		out[k] = v
	}
	return out
}

func TestReadArchiveValidation(t *testing.T) {
	good := map[string]string{FileAccount: `{"id":1,"name":"A"}`, "unknown.txt": "ignored"}
	read := func(b []byte, max int64) error {
		_, err := readArchive(bytes.NewReader(b), int64(len(b)), max)
		return err
	}
	if err := read(zipOf(t, good, &Manifest{Format: Format, Version: 1}), DefaultMaxBytes); err != nil {
		t.Fatalf("valid archive: %v", err)
	}
	cases := []struct {
		name string
		zip  []byte
		max  int64
		want error
		msg  string
	}{
		{"not a zip", []byte("hello"), DefaultMaxBytes, ErrCorrupt, "not a ZIP"},
		{"no manifest", zipOf(t, good, nil), DefaultMaxBytes, ErrCorrupt, "manifest.json is missing"},
		{"wrong format", zipOf(t, good, &Manifest{Format: "other", Version: 1}), DefaultMaxBytes, ErrCorrupt, "format"},
		{"newer version", zipOf(t, good, &Manifest{Format: Format, Version: 2}), DefaultMaxBytes, ErrUnsupportedVersion, "newer"},
		{"version 0", zipOf(t, good, &Manifest{Format: Format}), DefaultMaxBytes, ErrCorrupt, "invalid version"},
		{"checksum", zipOf(t, good, &Manifest{Format: Format, Version: 1, Files: map[string]FileInfo{FileAccount: {SHA256: strings.Repeat("0", 64), Size: 19}}}),
			DefaultMaxBytes, ErrCorrupt, "checksum"},
		{"listed file missing", zipOf(t, good, &Manifest{Format: Format, Version: 1, Files: map[string]FileInfo{"invoices.json": {}}}),
			DefaultMaxBytes, ErrCorrupt, "missing"},
		{"no account", zipOf(t, map[string]string{"subjects.json": "[]"}, &Manifest{Format: Format, Version: 1}), DefaultMaxBytes, ErrCorrupt, "account.json"},
		{"zip slip", zipOf(t, mergeFile(good, "../evil.txt", "x"), &Manifest{Format: Format, Version: 1}), DefaultMaxBytes, ErrCorrupt, "unsafe path"},
		{"absolute", zipOf(t, mergeFile(good, "/etc/x", "x"), &Manifest{Format: Format, Version: 1}), DefaultMaxBytes, ErrCorrupt, "unsafe path"},
		{"too big", zipOf(t, mergeFile(good, "big.bin", strings.Repeat("a", 2048)), &Manifest{Format: Format, Version: 1}), 1024, ErrTooLarge, "uncompressed"},
		{"bad json", zipOf(t, map[string]string{FileAccount: "{"}, &Manifest{Format: Format, Version: 1}), DefaultMaxBytes, ErrCorrupt, "account.json"},
		{"attachment outside dir", zipOf(t, mergeFile(good, FileAttachments, `[{"id":1,"path":"account.json"}]`), &Manifest{Format: Format, Version: 1}),
			DefaultMaxBytes, ErrCorrupt, "invalid path"},
	}
	for _, c := range cases {
		err := read(c.zip, c.max)
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %v containing %q", c.name, err, c.want, c.msg)
		}
	}
}
