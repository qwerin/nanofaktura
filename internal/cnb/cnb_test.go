package cnb_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/cnb"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseList(t *testing.T) {
	l, err := cnb.ParseList(strings.NewReader(string(readFixture(t, "denni_kurz_2026-09-25.txt"))))
	if err != nil {
		t.Fatal(err)
	}
	if l.Date != "2026-09-25" || l.Number != 186 || len(l.Rates) != 30 {
		t.Fatalf("list %s #%d, %d rates", l.Date, l.Number, len(l.Rates))
	}
	if r := l.Find("EUR"); r == nil || *r != (cnb.Rate{Currency: "EUR", Amount: 1, Rate: "24.350"}) {
		t.Fatalf("EUR %+v", r)
	}
	if r := l.Find("JPY"); r == nil || r.Amount != 100 || r.Rate != "13.546" {
		t.Fatalf("JPY %+v", r)
	}
	if r := l.Find("IDR"); r == nil || r.Amount != 1000 {
		t.Fatalf("IDR %+v", r)
	}
	if l.Find("BGN") != nil {
		t.Fatal("BGN is not in the 2026 list")
	}
	old, err := cnb.ParseList(strings.NewReader(string(readFixture(t, "denni_kurz_2024-01-02.txt"))))
	if err != nil || old.Date != "2024-01-02" || old.Number != 1 || old.Find("BGN") == nil {
		t.Fatalf("2024 list: %+v %v", old, err)
	}

	for _, bad := range []string{"", "<html>maintenance</html>\n", "25.09.2026 #186\nzemě|měna|množství|kód|kurz\n",
		"25.09.2026 #186\nzemě|měna|množství|kód|kurz\nEMU|euro|1|EUR|24.350\n",
		"25.09.2026 #186\nzemě|měna|množství|kód|kurz\nEMU|euro|x|EUR|24,350\n"} {
		if _, err := cnb.ParseList(strings.NewReader(bad)); !errors.Is(err, cnb.ErrFormat) {
			t.Errorf("%q: err %v", bad, err)
		}
	}
}

func TestPerUnit(t *testing.T) {
	for _, tc := range []struct {
		rate   string
		amount int
		want   string
	}{
		{"24.350", 1, "24.350"},
		{"24.35", 1, "24.350"},
		{"21.359", 1, "21.359"},
		{"13.546", 100, "0.13546"},
		{"1.194", 1000, "0.001194"},
		{"6.663", 100, "0.06663"},
		{"1", 3, "0.333333333333"},
	} {
		got, err := cnb.PerUnit(tc.rate, tc.amount)
		if err != nil || got != tc.want {
			t.Errorf("PerUnit(%s, %d) = %q, %v; want %q", tc.rate, tc.amount, got, err, tc.want)
		}
	}
	if _, err := cnb.PerUnit("abc", 1); err == nil {
		t.Error("expected error")
	}
}

// fakeCNB serves the recorded 25.09.2026 list for any date from 25.09.2026
// on (like ČNB on the weekend of 26–27.09.) and counts requests.
func fakeCNB(t *testing.T) (*httptest.Server, *int) {
	body := readFixture(t, "denni_kurz_2026-09-25.txt")
	old := readFixture(t, "denni_kurz_2024-01-02.txt")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Query().Get("date") {
		case "25.09.2026", "26.09.2026", "27.09.2026":
			_, _ = w.Write(body)
		case "02.01.2024":
			_, _ = w.Write(old)
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestClientFetch(t *testing.T) {
	srv, _ := fakeCNB(t)
	c := cnb.New(srv.URL + "/denni_kurz.txt")
	l, err := c.Fetch(context.Background(), "2026-09-26")
	if err != nil || l.Date != "2026-09-25" {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := c.Fetch(context.Background(), "2026-01-01"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err %v", err)
	}
	if _, err := c.Fetch(context.Background(), "26.09.2026"); !errors.Is(err, cnb.ErrInvalidDate) {
		t.Fatalf("err %v", err)
	}
}

func TestServiceRate(t *testing.T) {
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := gdb.DB(); err == nil {
			_ = s.Close()
		}
	})
	srv, calls := fakeCNB(t)
	now := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC) // Monday morning, before publication
	svc := cnb.NewService(gdb, cnb.New(srv.URL), func() time.Time { return now })
	ctx := context.Background()

	check := func(cur, date, wantRate, wantDate string) {
		t.Helper()
		rate, rd, err := svc.Rate(ctx, cur, date)
		if err != nil || rate != wantRate || rd != wantDate {
			t.Fatalf("Rate(%s, %s) = %s, %s, %v; want %s, %s", cur, date, rate, rd, err, wantRate, wantDate)
		}
	}

	check("EUR", "2026-09-26", "24.350", "2026-09-25") // Saturday → Friday's list
	if *calls != 1 {
		t.Fatalf("calls %d", *calls)
	}
	check("jpy", "2026-09-26", "0.13546", "2026-09-25") // cached, per 1 unit
	check("USD", "2026-09-25", "21.359", "2026-09-25")  // Friday rows were stored too
	if *calls != 1 {
		t.Fatalf("expected cache hits, calls %d", *calls)
	}
	if _, _, err := svc.Rate(ctx, "BGN", "2026-09-26"); !errors.Is(err, cnb.ErrUnknownCurrency) {
		t.Fatalf("BGN: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("unknown currency on a cached day must not refetch, calls %d", *calls)
	}
	check("BGN", "2024-01-02", "12.621", "2024-01-02")
	check("CZK", "2026-09-26", "1.000", "2026-09-26")

	var n int64
	gdb.Model(&model.ExchangeRate{}).Where("date = ? AND list_date = ?", "2026-09-26", "2026-09-25").Count(&n)
	if n != 30 {
		t.Fatalf("alias rows %d", n)
	}

	// Today's list not published yet: answer with the last list, but don't
	// cache the mapping so a later call picks up today's list.
	before := *calls
	_, _, err = svc.Rate(ctx, "EUR", "2026-09-28")
	if err == nil {
		t.Fatal("fake has no list for 28.09 (500) → expected error")
	}
	// A future date is clamped to today.
	now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	check("EUR", "2030-01-01", "24.350", "2026-09-25")
	check("EUR", "2026-09-27", "24.350", "2026-09-25")
	if *calls != before+3 { // 28.09 failed; 27.09 (today) is fetched each time until published
		t.Fatalf("calls %d, before %d", *calls, before)
	}
	gdb.Model(&model.ExchangeRate{}).Where("date = ?", "2026-09-27").Count(&n)
	if n != 0 {
		t.Fatalf("today's unpublished mapping must not be cached, got %d rows", n)
	}

	for _, tc := range []struct{ cur, date string }{{"EU", "2026-09-26"}, {"EUR1", "2026-09-26"}} {
		if _, _, err := svc.Rate(ctx, tc.cur, tc.date); !errors.Is(err, cnb.ErrInvalidCurrency) {
			t.Errorf("%v: %v", tc, err)
		}
	}
	if _, _, err := svc.Rate(ctx, "EUR", "26.09.2026"); !errors.Is(err, cnb.ErrInvalidDate) {
		t.Errorf("date: %v", err)
	}
}
