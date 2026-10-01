package api_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
)

const fixtureFioToken = "fioTokenSECRET1234567890abcdefghijklmnopqrstuvwxyz0123456789abcd"

// importBackup posts a backup ZIP to POST /api/accounts/import.
func (c *client) importBackup(name string, data []byte) (*http.Response, []byte) {
	c.ts.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if name != "" {
		_ = mw.WriteField("name", name)
	}
	fw, _ := mw.CreateFormFile("file", "backup.zip")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/accounts/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func accountOf(t *testing.T, ts *testServer, slug string) model.Account {
	t.Helper()
	var acc model.Account
	if err := ts.db.Where("slug = ?", slug).First(&acc).Error; err != nil {
		t.Fatal(err)
	}
	return acc
}

// richFixture fills a's account with every kind of data and returns a
// secret that must never appear in a backup (the webhook secret).
func richFixture(t *testing.T, ts *testServer, a *client) (webhookSecret string) {
	t.Helper()
	acc := accountOf(t, ts, a.slug)

	logo := uploadOK(a, "account", 0, "logo.png", pngData)
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{
		"street": "Dlouhá 1", "city": "Praha", "zip": "11000", "email": "firma@example.cz", "vat_mode": "vat_payer",
		"vat_no": "CZ12345678", "default_note": "Děkujeme", "pdf_accent": "#112233", "pdf_template": "modern",
		"email_signature": "S pozdravem", "reminders_enabled": true, "reminder_days_after_due": []int{5, 20},
		"paid_thanks_enabled": true, "onboarded": true, "logo_attachment_id": logo.ID, "c_ufo": "451",
	})

	czk := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"), api.BankAccountCreate{
		Name: "Fio", Number: "2000145399/2010", SyncProvider: "fio", FioToken: fixtureFioToken, SyncFrom: "2026-01-01"})
	doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "EUR", Currency: "EUR", IBAN: "DE89370400440532013000", SwiftBIC: "COBADEFFXXX"})

	doJSON[api.NumberFormat](a, http.StatusCreated, "POST", a.acct("/number-formats"),
		api.NumberFormatCreate{DocumentType: "invoice", Format: "FV{YY}{MM}-{NNN}", IsDefault: true})

	cust := newSubject(a, api.SubjectCreate{Name: "ACME s.r.o.", Email: "acme@example.cz", CustomID: strPtr("K-1"), DueDays: intPtr(10)})
	supp := newSubject(a, api.SubjectCreate{Name: "Dodavatel a.s.", Type: "supplier"})

	item := doJSON[api.PriceItem](a, http.StatusCreated, "POST", a.acct("/price-items"), api.PriceItemCreate{
		Name: "Krabice", SKU: "KR-1", UnitName: "ks", UnitPrice: 10000, TrackStock: true, StockQuantity: "10", MinStock: "2"})
	a.mustDo(http.StatusCreated, "POST", fmt.Sprintf("%s/%d/stock-moves", a.acct("/price-items"), item.ID),
		api.StockMoveCreate{Direction: "out", Quantity: "1.5", Note: "rozbité"})

	pl := line("Krabice", "2", 10000, i32(2100))
	pl.PriceItemID = &item.ID
	inv1 := createInv(a, api.InvoiceCreate{SubjectID: cust.ID, Lines: []api.InvoiceLineInput{pl, line("Práce", "1.25", 80000, i32(1200))},
		Tags: []string{"web"}, OrderNumber: "OBJ-1", BankAccountID: &czk.ID})
	pay(a, inv1.ID, api.PaymentCreate{Note: "převodem"})
	corr := doJSON[api.Invoice](a, http.StatusCreated, "POST", invURL(a, inv1.ID, "/correction"), map[string]any{"correction_reason": "Vrácení zboží"})
	_ = corr
	proforma := createInv(a, api.InvoiceCreate{DocumentType: "proforma", SubjectID: cust.ID, Lines: []api.InvoiceLineInput{line("Záloha", "", 50000, nil)}})
	pay(a, proforma.ID, api.PaymentCreate{CreateFinalInvoice: true})
	cancelled := createInv(a, api.InvoiceCreate{SubjectID: cust.ID, Lines: []api.InvoiceLineInput{line("Storno", "", 1000, nil)}})
	action(a, cancelled.ID, "cancel")
	locked := createInv(a, api.InvoiceCreate{SubjectID: cust.ID, Lines: []api.InvoiceLineInput{line("Zamčená", "", 2000, nil)}})
	action(a, locked.ID, "mark_as_sent")
	action(a, locked.ID, "lock")
	pay(a, locked.ID, api.PaymentCreate{Amount: ptr64(500)})
	deleted := createInv(a, api.InvoiceCreate{SubjectID: cust.ID, Lines: []api.InvoiceLineInput{line("Smazat", "", 1, nil)}})
	a.mustDo(http.StatusNoContent, "DELETE", invURL(a, deleted.ID, ""), nil) // leaves an invoice.deleted event

	exp := createExp(a, api.ExpenseCreate{SubjectID: &supp.ID, OriginalNumber: "DF-77", Category: "kancelář",
		Lines: []api.InvoiceLineInput{pl}})
	expPay := payExp(a, exp.ID, api.ExpensePaymentCreate{})

	uploadOK(a, "invoice", inv1.ID, "smlouva.pdf", pdfData)
	uploadOK(a, "expense", exp.ID, "uctenka.jpg", jpegData)
	uploadOK(a, "subject", cust.ID, "../../vizitka.png", pngData)

	tpl := doJSON[api.Template](a, http.StatusCreated, "POST", a.acct("/templates"), api.TemplateCreate{Name: "Hosting", SubjectID: cust.ID,
		Note: strPtr("Hosting {month}"), BankAccountID: &czk.ID,
		Lines: []api.TemplateLineInput{{Name: "Hosting", UnitPrice: 30000, PriceItemID: &item.ID}}})
	rec := doJSON[api.Recurring](a, http.StatusCreated, "POST", a.acct("/recurring"), api.RecurringCreate{
		Name: "Měsíční hosting", TemplateID: tpl.ID, StartOn: "2026-03-15", DayOfMonth: intPtr(15)})
	a.mustDo(http.StatusCreated, "POST", fmt.Sprintf("%s/%d/run-now", a.acct("/recurring"), rec.ID), nil)

	a.mustDo(http.StatusCreated, "POST", a.acct("/todos"), api.TodoCreate{Text: "Zavolat klientovi", DueOn: "2026-04-01",
		RelatedType: "invoice", RelatedID: &inv1.ID})
	hook := doJSON[api.Webhook](a, http.StatusCreated, "POST", a.acct("/webhooks"),
		api.WebhookCreate{URL: "http://127.0.0.1:9/hook", Description: "ERP", Events: []string{"invoice.*"}})

	// records only the scheduler / bank import create (direct setup)
	var inv1Pay model.Payment
	if err := ts.db.Where("invoice_id = ?", inv1.ID).First(&inv1Pay).Error; err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("invoice.overdue:%d", locked.ID)
	for _, m := range []any{
		&model.BankTransaction{AccountID: acc.ID, BankAccountID: czk.ID, ExternalID: "T1", BookedOn: "2026-03-15", Amount: inv1.Total,
			Currency: "CZK", VariableSymbol: inv1.VariableSymbol, MatchedInvoiceID: &inv1.ID, PaymentID: &inv1Pay.ID, AutoMatched: true},
		&model.BankTransaction{AccountID: acc.ID, BankAccountID: czk.ID, ExternalID: "T2", BookedOn: "2026-03-15", Amount: -exp.Total,
			Currency: "CZK", MatchedExpenseID: &exp.ID, PaymentID: &expPay.Payment.ID},
		&model.BankTransaction{AccountID: acc.ID, BankAccountID: czk.ID, ExternalID: "T3", BookedOn: "2026-03-14", Amount: 1500,
			Currency: "CZK", CounterpartyName: "Někdo", Message: "platba", SuggestionCount: 1,
			Suggestions: []model.MatchSuggestion{{InvoiceID: &locked.ID, Number: locked.Number, Name: "ACME", Remaining: 1500, Score: 60, Reasons: []string{"amount"}}}},
		&model.Todo{AccountID: acc.ID, Key: &key, Name: model.TodoInvoiceOverdue, Text: "Po splatnosti", RelatedType: "invoice", RelatedID: &locked.ID},
		&model.EmailLog{AccountID: acc.ID, InvoiceID: inv1.ID, Kind: model.EmailReminder, To: []string{"acme@example.cz"}, Subject: "Upomínka",
			Body: "Zaplaťte", Attachments: []string{"FV.pdf"}, ReminderStep: 5, Automatic: true, SentAt: &ts.now},
	} {
		if err := ts.db.Create(m).Error; err != nil {
			t.Fatal(err)
		}
	}
	ts.memberOf(a, "ucetni@example.cz", "accountant")
	return hook.Secret
}

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// dumpAccount is a canonical, ID-free dump of all data of an account:
// records per table with references replaced by the ordinal of the target
// (by ID order, children by parent), sorted. Instance-specific values (IDs,
// tokens, secrets, storage keys, users) are left out.
func dumpAccount(t *testing.T, ts *testServer, accID uint) map[string][]string {
	t.Helper()
	load := func(dst any, where string, args ...any) []map[string]any {
		if err := ts.db.Where(where, args...).Order("id").Find(dst).Error; err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(dst)
		var rows []map[string]any
		_ = json.Unmarshal(b, &rows)
		return rows
	}
	tables := map[string][]map[string]any{}
	byAcc := func(name string, dst any) { tables[name] = load(dst, "account_id = ?", accID) }
	var acc model.Account
	if err := ts.db.First(&acc, accID).Error; err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(acc)
	var accRow map[string]any
	_ = json.Unmarshal(b, &accRow)
	tables["accounts"] = []map[string]any{accRow}
	byAcc("bank_accounts", &[]model.BankAccount{})
	byAcc("number_formats", &[]model.NumberFormat{})
	byAcc("subjects", &[]model.Subject{})
	byAcc("price_items", &[]model.PriceItem{})
	byAcc("stock_moves", &[]model.StockMove{})
	byAcc("invoices", &[]model.Invoice{})
	byAcc("expenses", &[]model.Expense{})
	byAcc("templates", &[]model.InvoiceTemplate{})
	byAcc("recurring", &[]model.Recurring{})
	byAcc("bank_transactions", &[]model.BankTransaction{})
	byAcc("todos", &[]model.Todo{})
	byAcc("events", &[]model.Event{})
	byAcc("email_logs", &[]model.EmailLog{})
	byAcc("webhooks", &[]model.Webhook{})
	byAcc("attachments", &[]model.Attachment{})
	ids := func(table string) []any {
		var out []any
		for _, r := range tables[table] {
			out = append(out, r["ID"])
		}
		return out
	}
	tables["invoice_lines"] = load(&[]model.InvoiceLine{}, "invoice_id IN ?", ids("invoices"))
	tables["payments"] = load(&[]model.Payment{}, "account_id = ?", accID)
	tables["expense_lines"] = load(&[]model.ExpenseLine{}, "expense_id IN ?", ids("expenses"))
	tables["expense_payments"] = load(&[]model.ExpensePayment{}, "account_id = ?", accID)
	tables["number_counters"] = load(&[]model.NumberCounter{}, "number_format_id IN ?", ids("number_formats"))

	ord := map[string]map[float64]int{}
	index := func(table string) {
		ord[table] = map[float64]int{}
		for i, r := range tables[table] {
			ord[table][r["ID"].(float64)] = i + 1
		}
	}
	for _, tb := range []string{"accounts", "bank_accounts", "number_formats", "subjects", "price_items", "invoices", "expenses",
		"templates", "recurring", "bank_transactions", "webhooks", "attachments"} {
		index(tb)
	}
	// payments: by (invoice, id) — the order of the backup
	for _, p := range [][2]string{{"payments", "InvoiceID"}, {"expense_payments", "ExpenseID"}} {
		rows := tables[p[0]]
		sort.SliceStable(rows, func(i, j int) bool {
			return ord[map[string]string{"InvoiceID": "invoices", "ExpenseID": "expenses"}[p[1]]][rows[i][p[1]].(float64)] <
				ord[map[string]string{"InvoiceID": "invoices", "ExpenseID": "expenses"}[p[1]]][rows[j][p[1]].(float64)]
		})
		index(p[0])
	}
	ref := func(table string, v any) any {
		f, ok := v.(float64)
		if !ok {
			return v // nil
		}
		return ord[table][f] // 0 = not found
	}
	typed := map[string]string{"invoice": "invoices", "expense": "expenses", "subject": "subjects", "price_item": "price_items",
		"bank_transaction": "bank_transactions", "bank_account": "bank_accounts", "recurring": "recurring", "webhook": "webhooks",
		"account": "accounts"}
	refs := map[string]string{"SubjectID": "subjects", "RelatedID": "invoices", "RecurringID": "recurring", "BankAccountID": "bank_accounts",
		"PriceItemID": "price_items", "InvoiceID": "invoices", "ExpenseID": "expenses", "TemplateID": "templates",
		"LastInvoiceID": "invoices", "MatchedInvoiceID": "invoices", "MatchedExpenseID": "expenses",
		"LogoAttachmentID": "attachments", "StampAttachmentID": "attachments", "NumberFormatID": "number_formats",
		"TaxDocumentID": "invoices", "SourcePaymentID": "payments"}
	out := map[string][]string{}
	for table, rows := range tables {
		for _, r := range rows {
			for _, k := range []string{"ID", "AccountID", "Slug", "PublicToken", "StorageKey", "FioToken", "Secret", "UserID"} {
				delete(r, k)
			}
			for k, target := range refs {
				if v, ok := r[k]; ok {
					switch {
					case table == "todos" && k == "RelatedID":
						r[k] = ref(typed[r["RelatedType"].(string)], v)
					case table == "events" && k == "SubjectID":
					default:
						r[k] = ref(target, v)
					}
				}
			}
			switch table {
			case "events":
				r["SubjectID"] = ref(typed[r["SubjectType"].(string)], r["SubjectID"])
			case "attachments":
				r["OwnerID"] = ref(typed[r["OwnerType"].(string)], r["OwnerID"])
			case "todos":
				if r["Key"] != nil {
					r["Key"] = fmt.Sprintf("%s:%v", r["Name"], r["RelatedID"])
				}
			case "templates":
				if ls, ok := r["Lines"].([]any); ok {
					for _, x := range ls {
						m := x.(map[string]any)
						if v, ok := m["price_item_id"]; ok {
							m["price_item_id"] = ref("price_items", v)
						}
					}
				}
			case "bank_transactions":
				if r["MatchedInvoiceID"] != nil {
					r["PaymentID"] = ref("payments", r["PaymentID"])
				} else {
					r["PaymentID"] = ref("expense_payments", r["PaymentID"])
				}
				if s, ok := r["Suggestions"].([]any); ok {
					for _, x := range s {
						m := x.(map[string]any)
						if v, ok := m["invoice_id"]; ok {
							m["invoice_id"] = ref("invoices", v)
						}
						if v, ok := m["expense_id"]; ok {
							m["expense_id"] = ref("expenses", v)
						}
					}
				}
			}
			b, _ := json.Marshal(r)
			out[table] = append(out[table], string(b))
		}
		sort.Strings(out[table])
	}
	return out
}

func downloadBackup(t *testing.T, c *client) []byte {
	t.Helper()
	res, body := c.do("GET", c.acct("/backup"), nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("backup: %d %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content type %q", ct)
	}
	return body
}

func TestBackupRoundtrip(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	webhookSecret := richFixture(t, ts, a)
	srcAcc := accountOf(t, ts, a.slug)
	before := dumpAccount(t, ts, srcAcc.ID)

	data := downloadBackup(t, a)
	res, _ := a.do("GET", a.acct("/backup"), nil)
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="nanofaktura-firma-a-2026-03-15.zip"` {
		t.Fatalf("content disposition %q", cd)
	}

	// no secret in the archive, neither plain nor encrypted
	var ba model.BankAccount
	var hook model.Webhook
	ts.db.Where("account_id = ? AND fio_token <> ''", srcAcc.ID).First(&ba)
	ts.db.Where("account_id = ?", srcAcc.ID).First(&hook)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		rc.Close()
		for _, s := range []string{fixtureFioToken, ba.FioToken, webhookSecret, hook.Secret} {
			if s == "" || bytes.Contains(content, []byte(s)) {
				t.Fatalf("secret %q… found in %s (or empty)", s[:min(len(s), 8)], f.Name)
			}
		}
	}
	for _, n := range []string{"manifest.json", "account.json", "invoices.json", "attachments.json", "members.json", "number_formats.json"} {
		if !names[n] {
			t.Errorf("%s missing in the archive: %v", n, names)
		}
	}
	var events int64
	ts.db.Model(&model.Event{}).Where("account_id = ? AND name = ?", srcAcc.ID, "account.exported").Count(&events)
	if events != 2 {
		t.Errorf("account.exported events: %d", events)
	}

	// import by another user → a new account owned by b
	res, body := b.importBackup("", data)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("import: %d %s", res.StatusCode, body)
	}
	out := decodeJSON[api.ImportResult](t, body)
	if out.Account.Slug == a.slug || out.Account.Role != "owner" || out.Account.Name != "Firma A" {
		t.Fatalf("imported account: %+v", out.Account)
	}
	codes := map[string]int{}
	for _, w := range out.Warnings {
		codes[w.Code] = w.Count
	}
	for code, n := range map[string]int{"recurring_deactivated": 1, "reminders_disabled": 0, "paid_thanks_disabled": 0,
		"webhooks_inactive": 1, "bank_tokens_removed": 1, "public_links_regenerated": 8, "members_not_imported": 1} {
		if got, ok := codes[code]; !ok || got != n {
			t.Errorf("warning %s: %d (present %v), want %d; all %+v", code, got, ok, n, out.Warnings)
		}
	}
	dst := accountOf(t, ts, out.Account.Slug)
	after := dumpAccount(t, ts, dst.ID)

	// documented differences
	fix := func(table, from, to string) {
		for i, r := range before[table] {
			before[table][i] = strings.ReplaceAll(r, from, to)
		}
		sort.Strings(before[table])
	}
	fix("accounts", `"RemindersEnabled":true`, `"RemindersEnabled":false`)
	fix("accounts", `"PaidThanksEnabled":true`, `"PaidThanksEnabled":false`)
	fix("recurring", `"Active":true`, `"Active":false`)
	fix("webhooks", `"Active":true`, `"Active":false`)
	var evs []string
	for _, e := range after["events"] {
		if !strings.Contains(e, `"Name":"account.imported"`) {
			evs = append(evs, e)
		}
	}
	if len(evs) != len(after["events"])-1 {
		t.Fatalf("want exactly one account.imported event: %v", after["events"])
	}
	after["events"] = evs
	for table := range before {
		if !reflect.DeepEqual(before[table], after[table]) {
			t.Errorf("%s differs after roundtrip:\nbefore: %v\nafter:  %v", table, before[table], after[table])
		}
	}
	if len(before["invoices"]) != 8 || len(before["attachments"]) != 4 || len(before["bank_transactions"]) != 3 {
		t.Fatalf("fixture too small: %d invoices, %d attachments", len(before["invoices"]), len(before["attachments"]))
	}

	// public tokens are new, attachment contents are there, logo points to the copy
	var srcInv, dstInv []model.Invoice
	ts.db.Where("account_id = ?", srcAcc.ID).Order("id").Find(&srcInv)
	ts.db.Where("account_id = ?", dst.ID).Order("id").Find(&dstInv)
	for i := range srcInv {
		if srcInv[i].PublicToken == dstInv[i].PublicToken || len(dstInv[i].PublicToken) != 32 {
			t.Errorf("public token of %s not regenerated", dstInv[i].Number)
		}
	}
	bc := &client{ts: ts, session: b.session, slug: dst.Slug}
	atts := doJSON[api.ListResponse[api.Attachment]](bc, http.StatusOK, "GET", bc.acct("/attachments?per_page=100"), nil)
	if atts.Total != 4 {
		t.Fatalf("attachments: %+v", atts)
	}
	for _, at := range atts.Items {
		content := bc.mustDo(http.StatusOK, "GET", attURL(bc, at.ID, "/download"), nil)
		if len(content) != int(at.Size) {
			t.Errorf("attachment %s: %d bytes, want %d", at.Filename, len(content), at.Size)
		}
	}
	if dst.LogoAttachmentID == nil || *dst.LogoAttachmentID == *srcAcc.LogoAttachmentID {
		t.Errorf("logo: %v", dst.LogoAttachmentID)
	}

	// numbering continues in both accounts identically
	subj := doJSON[api.ListResponse[api.Subject]](bc, http.StatusOK, "GET", bc.acct("/subjects?query=ACME"), nil).Items[0]
	next := createInv(bc, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Další", "", 100, nil)}})
	srcSubj := doJSON[api.ListResponse[api.Subject]](a, http.StatusOK, "GET", a.acct("/subjects?query=ACME"), nil).Items[0]
	nextSrc := createInv(a, api.InvoiceCreate{SubjectID: srcSubj.ID, Lines: []api.InvoiceLineInput{line("Další", "", 100, nil)}})
	if next.Number != nextSrc.Number || !strings.HasPrefix(next.Number, "FV2603-") || next.Number == "FV2603-001" {
		t.Errorf("next number %q (source %q)", next.Number, nextSrc.Number)
	}

	// the imported webhook gets a fresh secret when activated
	var dstHook model.Webhook
	ts.db.Where("account_id = ?", dst.ID).First(&dstHook)
	wh := doJSON[api.Webhook](bc, http.StatusOK, "PATCH", fmt.Sprintf("%s/%d", bc.acct("/webhooks"), dstHook.ID), map[string]any{"active": true})
	if !wh.Active || !wh.HasSecret || !strings.HasPrefix(wh.Secret, "whsec_") {
		t.Errorf("activated webhook: %+v", wh)
	}

	// the source account and a's access are untouched; a cannot see the new account
	res, body = a.do("GET", "/api/accounts/"+dst.Slug, nil)
	assertError(t, res, body, http.StatusNotFound, "not found")

	// a second import of the same backup gets another slug and a custom name
	res, body = b.importBackup("Kopie firmy", data)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("second import: %d %s", res.StatusCode, body)
	}
	if o := decodeJSON[api.ImportResult](t, body); o.Account.Name != "Kopie firmy" || o.Account.Slug != "kopie-firmy" {
		t.Errorf("second import: %+v", o.Account)
	}
}

// rewriteZip copies a backup, replacing (or removing with nil) some files and
// optionally recomputing the manifest checksums.
func rewriteZip(t *testing.T, data []byte, replace map[string][]byte, fixManifest bool) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	var order []string
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = b
		order = append(order, f.Name)
	}
	for k, v := range replace {
		if _, ok := files[k]; !ok {
			order = append(order, k)
		}
		files[k] = v
	}
	if fixManifest {
		var man map[string]any
		_ = json.Unmarshal(files["manifest.json"], &man)
		fm := man["files"].(map[string]any)
		for name, b := range files {
			if name == "manifest.json" {
				continue
			}
			if _, listed := fm[name]; listed || replace[name] != nil {
				fm[name] = map[string]any{"sha256": sha256hex(b), "size": len(b)}
			}
		}
		files["manifest.json"], _ = json.Marshal(man)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		if files[name] == nil {
			continue
		}
		w, _ := zw.Create(name)
		_, _ = w.Write(files[name])
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestBackupImportRejections(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.ImportMaxMB = 1 })
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("X", "", 100, nil)}})
	data := downloadBackup(t, a)

	var accounts int64
	countAccounts := func() int64 {
		var n int64
		ts.db.Model(&model.Account{}).Count(&n)
		return n
	}
	accounts = countAccounts()

	// b's own subject must not be reachable from a crafted backup
	bSubj := newSubject(b, api.SubjectCreate{Name: "Cizí"})
	var invs []map[string]any
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	for _, f := range zr.File {
		if f.Name == "invoices.json" {
			rc, _ := f.Open()
			raw, _ := io.ReadAll(rc)
			rc.Close()
			_ = json.Unmarshal(raw, &invs)
		}
	}
	invs[0]["subject_id"] = bSubj.ID
	foreign, _ := json.Marshal(invs)

	var man map[string]any
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			rc, _ := f.Open()
			raw, _ := io.ReadAll(rc)
			rc.Close()
			_ = json.Unmarshal(raw, &man)
		}
	}
	man["version"] = 2
	newer, _ := json.Marshal(man)

	for _, c := range []struct {
		name   string
		zip    []byte
		status int
		code   string
	}{
		{"not a zip", []byte("PK nope"), http.StatusUnprocessableEntity, "corrupt_backup"},
		{"checksum", rewriteZip(t, data, map[string][]byte{"subjects.json": []byte("[]")}, false), http.StatusUnprocessableEntity, "corrupt_backup"},
		{"newer version", rewriteZip(t, data, map[string][]byte{"manifest.json": newer}, false), http.StatusUnprocessableEntity, "unsupported_backup_version"},
		{"zip slip", rewriteZip(t, data, map[string][]byte{"../../etc/cron.d/x": []byte("x")}, false), http.StatusUnprocessableEntity, "corrupt_backup"},
		{"foreign reference", rewriteZip(t, data, map[string][]byte{"invoices.json": foreign}, true), http.StatusUnprocessableEntity, "corrupt_backup"},
		{"too big", rewriteZip(t, data, map[string][]byte{"padding.bin": bytes.Repeat([]byte{0}, 2<<20)}, false), http.StatusRequestEntityTooLarge, ""},
	} {
		res, body := b.importBackup("", c.zip)
		if c.code != "" {
			assertCode(t, res, body, c.status, c.code)
		} else if res.StatusCode != c.status {
			t.Errorf("%s: %d %s", c.name, res.StatusCode, body)
		}
		if n := countAccounts(); n != accounts {
			t.Fatalf("%s: an account was created (%d → %d)", c.name, accounts, n)
		}
	}
	// a zip bomb whose compressed size fits: rejected by the uncompressed limit
	bomb := rewriteZip(t, data, map[string][]byte{"bomb.bin": bytes.Repeat([]byte{0}, 3<<20)}, false)
	if len(bomb) > 1<<20 {
		t.Fatalf("bomb not compressed: %d", len(bomb))
	}
	res, body := b.importBackup("", bomb)
	assertCode(t, res, body, http.StatusRequestEntityTooLarge, "backup_too_large")

	// unknown files are ignored
	res, body = b.importBackup("", rewriteZip(t, data, map[string][]byte{"future.json": []byte(`{"x":1}`)}, false))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("unknown file: %d %s", res.StatusCode, body)
	}

	// no file / anonymous
	res, body = b.importBackup("", nil)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("empty file: %d %s", res.StatusCode, body)
	}
	res, body = ts.anon().do("POST", "/api/accounts/import", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous: %d %s", res.StatusCode, body)
	}
}
