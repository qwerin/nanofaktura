package bankimport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeFgHiJkLmNoPqRsTuVwXyZ01"

// fakeFio mimics the Fio API: valid token → recorded JSON, other tokens →
// 500 with an empty body (as the real API does), "rate" token → 409.
func fakeFio(t *testing.T) (*httptest.Server, *[]string) {
	var paths []string
	body := fixture(t, "fio_transactions.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/rest"), "/"), "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		switch token := parts[1]; {
		case strings.HasPrefix(token, "rate"):
			w.WriteHeader(http.StatusConflict)
		case strings.HasPrefix(token, "big"):
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		case token != testToken:
			w.WriteHeader(http.StatusInternalServerError)
		case parts[0] == "set-last-date":
			w.WriteHeader(http.StatusOK)
		default:
			w.Header().Set("Content-Type", "application/json;charset=UTF-8")
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func TestFioClient(t *testing.T) {
	srv, paths := fakeFio(t)
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	c := NewFioClient(srv.URL + "/v1/rest/")
	c.Now = func() time.Time { return now }
	ctx := context.Background()

	st, err := c.Periods(ctx, testToken, "2026-09-01", "2026-09-25")
	if err != nil || len(st.Transactions) != 4 || st.Account != "2000000000/2010" {
		t.Fatalf("periods: %+v %v", st, err)
	}
	if (*paths)[0] != "/v1/rest/periods/"+testToken+"/2026-09-01/2026-09-25/transactions.json" {
		t.Fatalf("path %s", (*paths)[0])
	}

	// local 30 s limit per token: no request is sent
	_, err = c.Last(ctx, testToken)
	if !errors.Is(err, ErrFioRateLimited) || len(*paths) != 1 || !strings.Contains(err.Error(), "retry in 30s") {
		t.Fatalf("local limit: %v, %d requests", err, len(*paths))
	}
	now = now.Add(FioMinInterval)
	if st, err = c.Last(ctx, testToken); err != nil || len(st.Transactions) != 4 {
		t.Fatalf("last: %v", err)
	}
	if (*paths)[1] != "/v1/rest/last/"+testToken+"/transactions.json" {
		t.Fatalf("path %s", (*paths)[1])
	}
	now = now.Add(FioMinInterval)
	if err := c.SetLastDate(ctx, testToken, "2026-09-20"); err != nil || (*paths)[2] != "/v1/rest/set-last-date/"+testToken+"/2026-09-20/" {
		t.Fatalf("set-last-date: %v %v", err, *paths)
	}

	c.MinInterval = 0
	bad := strings.Repeat("x", 64)
	if _, err := c.Last(ctx, bad); !errors.Is(err, ErrFioToken) || strings.Contains(err.Error(), bad) {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := c.Last(ctx, "rate"+strings.Repeat("0", 60)); !errors.Is(err, ErrFioRateLimited) {
		t.Fatalf("409: %v", err)
	}
	if _, err := c.Periods(ctx, "big"+strings.Repeat("0", 61), "2020-01-01", "2026-01-01"); !errors.Is(err, ErrFioTooMany) {
		t.Fatalf("413: %v", err)
	}
	if _, err := c.Last(ctx, "short/../x"); !errors.Is(err, ErrFioToken) {
		t.Fatalf("malformed token: %v", err)
	}
	if _, err := c.Periods(ctx, testToken, "1.9.2026", "2026-09-25"); err == nil {
		t.Fatal("bad date accepted")
	}

	// transport errors must not leak the token (it is part of the URL)
	down := NewFioClient("http://127.0.0.1:1")
	_, err = down.Last(ctx, testToken)
	if err == nil || strings.Contains(err.Error(), testToken) {
		t.Fatalf("transport error leaks token: %v", err)
	}
	var _ Fio = down
}
