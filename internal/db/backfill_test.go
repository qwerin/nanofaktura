package db

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/model"
)

func TestWithLoggingAndDSN(t *testing.T) {
	o := options{logLevel: "error", slow: time.Second}
	WithLogging("", 0)(&o)
	if o.logLevel != "error" || o.slow != time.Second {
		t.Fatalf("empty options changed defaults: %+v", o)
	}
	WithLogging("warn", 5*time.Millisecond)(&o)
	if o.logLevel != "warn" || o.slow != 5*time.Millisecond {
		t.Fatalf("options %+v", o)
	}
	if _, err := Open("sqlite", ":memory:", WithLogging("bogus", 0)); err == nil {
		t.Fatal("unknown log level accepted")
	}
	if dsn := sqliteDSN("file:x.db?cache=shared"); !strings.HasPrefix(dsn, "file:x.db?cache=shared&_pragma=foreign_keys(1)") {
		t.Fatalf("dsn %q", dsn)
	}
	var buf bytes.Buffer
	d, err := Open("sqlite", ":memory:", WithLogging("silent", 0), func(o *options) { o.out = &buf })
	if err != nil {
		t.Fatal(err)
	}
	d.Exec("SELECT * FROM missing_table")
	if buf.Len() != 0 {
		t.Fatalf("silent level logged: %s", buf.String())
	}
}

func TestOpenPostgresUnreachable(t *testing.T) {
	if _, err := Open("postgres", "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1"); err == nil ||
		!strings.Contains(err.Error(), "open postgres") {
		t.Fatalf("unreachable postgres: %v", err)
	}
}

// TestMigrateBackfillsSearchText: rows written before search_text existed
// get it on the next Migrate; rows with nothing searchable stay empty.
func TestMigrateBackfillsSearchText(t *testing.T) {
	d, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	subj := model.Subject{AccountID: 1, Name: "Žluťoučký Kůň s.r.o."}
	if err := d.Create(&subj).Error; err != nil {
		t.Fatal(err)
	}
	item := model.PriceItem{AccountID: 1, Name: "Konzultace"}
	if err := d.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&subj, &item} {
		if err := d.Model(m).UpdateColumn("search_text", "").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var gotS model.Subject
	d.First(&gotS, subj.ID)
	if !strings.Contains(gotS.SearchText, "zlutoucky kun") {
		t.Fatalf("subject search_text %q", gotS.SearchText)
	}
	var gotP model.PriceItem
	d.First(&gotP, item.ID)
	if !strings.Contains(gotP.SearchText, "konzultace") {
		t.Fatalf("price item search_text %q", gotP.SearchText)
	}
	// idempotent
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
}
