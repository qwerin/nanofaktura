package api_test

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

// randomFixture fills a's account with n subjects, ~2n invoices (random
// lines, VAT rates, partial/full payments, some cancelled) and ~n expenses.
func randomFixture(t *testing.T, a *client, rng *rand.Rand, n int) {
	t.Helper()
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"vat_mode": "vat_payer", "vat_no": "CZ12345678"})
	rates := []*int32{nil, i32(0), i32(1200), i32(2100)}
	var subjects []api.Subject
	for i := range n {
		subjects = append(subjects, newSubject(a, api.SubjectCreate{Name: fmt.Sprintf("Klient %d %c", i, 'A'+rune(rng.IntN(26))),
			Email: fmt.Sprintf("k%d@example.cz", i)}))
	}
	if n == 0 {
		return
	}
	lines := func() []api.InvoiceLineInput {
		var ls []api.InvoiceLineInput
		for j := range 1 + rng.IntN(4) {
			qty := fmt.Sprintf("%d.%03d", rng.IntN(20), rng.IntN(1000))
			if qty == "0.000" {
				qty = "1"
			}
			ls = append(ls, line(fmt.Sprintf("Položka %d", j), qty, int64(1+rng.IntN(500_000)), rates[rng.IntN(len(rates))]))
		}
		return ls
	}
	for i := range 2 * n {
		inv := createInv(a, api.InvoiceCreate{SubjectID: subjects[rng.IntN(len(subjects))].ID, Lines: lines(),
			Tags: []string{fmt.Sprintf("t%d", rng.IntN(3))}})
		switch rng.IntN(4) {
		case 0:
			pay(a, inv.ID, api.PaymentCreate{})
		case 1:
			if inv.Total > 1 {
				pay(a, inv.ID, api.PaymentCreate{Amount: ptr64(inv.Total / 2)})
			}
		case 2:
			if i%2 == 0 {
				action(a, inv.ID, "cancel")
			}
		}
	}
	for range n {
		exp := createExp(a, api.ExpenseCreate{SubjectID: &subjects[rng.IntN(len(subjects))].ID,
			OriginalNumber: fmt.Sprintf("DF-%d", rng.IntN(1000)), Lines: lines()})
		if rng.IntN(2) == 0 {
			payExp(a, exp.ID, api.ExpensePaymentCreate{})
		}
	}
}

// normalizeDump removes what legitimately differs between an account and
// its restored copy: backup events, and the switches Import turns off.
func normalizeDump(d map[string][]string) map[string][]string {
	out := map[string][]string{}
	for table, rows := range d {
		var rs []string
		for _, r := range rows {
			if table == "events" && (strings.Contains(r, `"Name":"account.imported"`) || strings.Contains(r, `"Name":"account.exported"`)) {
				continue
			}
			r = strings.ReplaceAll(r, `"RemindersEnabled":true`, `"RemindersEnabled":false`)
			r = strings.ReplaceAll(r, `"PaidThanksEnabled":true`, `"PaidThanksEnabled":false`)
			rs = append(rs, r)
		}
		sort.Strings(rs)
		out[table] = rs
	}
	return out
}

// TestBackupRoundtripRandomized: for randomized fixture sizes,
// export → import → export → import yields the same canonical data each
// time (backup is lossless and idempotent up to IDs/tokens).
func TestBackupRoundtripRandomized(t *testing.T) {
	for _, n := range []int{0, 1, 4, 9} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			ts := newTestServer(t)
			a := ts.signup("a@example.cz", "Firma A")
			b := ts.signup("b@example.cz", "Firma B")
			randomFixture(t, a, rand.New(rand.NewPCG(uint64(n), 42)), n)

			src := accountOf(t, ts, a.slug)
			want := normalizeDump(dumpAccount(t, ts, src.ID))
			data := downloadBackup(t, a)

			cur := a
			for gen := 1; gen <= 2; gen++ {
				res, body := b.importBackup("", data)
				if res.StatusCode != http.StatusCreated {
					t.Fatalf("gen %d import: %d %s", gen, res.StatusCode, body)
				}
				out := decodeJSON[api.ImportResult](t, body)
				dst := accountOf(t, ts, out.Account.Slug)
				got := normalizeDump(dumpAccount(t, ts, dst.ID))
				for table := range want {
					if !reflect.DeepEqual(want[table], got[table]) {
						t.Fatalf("gen %d: %s differs:\nwant: %v\ngot:  %v", gen, table, want[table], got[table])
					}
				}
				cur = &client{ts: ts, session: b.session, slug: dst.Slug}
				data = downloadBackup(t, cur)
			}
			if n > 0 && len(want["invoices"]) != 2*n {
				t.Fatalf("fixture: %d invoices", len(want["invoices"]))
			}
		})
	}
}
