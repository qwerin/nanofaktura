package numbering_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/numbering"
)

var errRollback = errors.New("rollback")

// concurrentNext runs workers×perWorker Next calls, each in its own
// transaction; every third transaction of a worker rolls back. It returns
// the numbers of the committed transactions.
func concurrentNext(t *testing.T, gdb *gorm.DB, accountID uint, workers, perWorker int) []string {
	t.Helper()
	var (
		mu  sync.Mutex
		got []string
		wg  sync.WaitGroup
	)
	errs := make(chan error, workers*perWorker)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWorker {
				var n string
				err := gdb.Transaction(func(tx *gorm.DB) (err error) {
					n, err = numbering.Next(tx, accountID, model.DocInvoice, "2026-03-15")
					if err != nil {
						return err
					}
					if (w+i)%3 == 0 {
						return errRollback // the counter must not move
					}
					return nil
				})
				switch {
				case errors.Is(err, errRollback):
				case err != nil:
					errs <- err
				default:
					mu.Lock()
					got = append(got, n)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Next: %v", err)
	}
	return got
}

// assertGapless: the committed numbers are exactly 2026-0001 … 2026-00NN.
func assertGapless(t *testing.T, got []string) {
	t.Helper()
	sort.Strings(got)
	for i, n := range got {
		if want := fmt.Sprintf("2026-%04d", i+1); n != want {
			t.Fatalf("numbers not unique/gapless at %d: %q, want %q (all: %v)", i, n, want, got)
		}
	}
}

func seedSeries(t *testing.T, gdb *gorm.DB, slug string) uint {
	t.Helper()
	acc := model.Account{Slug: slug, Name: slug}
	if err := gdb.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	nf := model.NumberFormat{AccountID: acc.ID, DocumentType: model.DocInvoice, Format: "{YYYY}-{NNNN}", IsDefault: true}
	if err := gdb.Create(&nf).Error; err != nil {
		t.Fatal(err)
	}
	return acc.ID
}

// TestNextConcurrentSQLite: on SQLite every transaction is serialized by the
// single connection (db.Open), so concurrent Next calls can neither collide
// nor skip a number, and rolled back transactions leave no gap.
func TestNextConcurrentSQLite(t *testing.T) {
	gdb, err := db.Open("sqlite", filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	a := seedSeries(t, gdb, "a")
	b := seedSeries(t, gdb, "b")
	var ga, gb []string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ga = concurrentNext(t, gdb, a, 8, 15) }()
	go func() { defer wg.Done(); gb = concurrentNext(t, gdb, b, 4, 15) }()
	wg.Wait()
	if len(ga) != 80 || len(gb) != 40 {
		t.Fatalf("committed %d / %d", len(ga), len(gb))
	}
	assertGapless(t, ga) // accounts have independent series
	assertGapless(t, gb)
}

// TestNextConcurrentPostgres runs the same check against a real Postgres
// when NANOFAKTURA_TEST_POSTGRES_DSN is set (e.g. in a local docker). There
// the transactions do run in parallel: the counter row is created with
// INSERT … ON CONFLICT DO NOTHING and locked with SELECT … FOR UPDATE, so
// concurrent Next calls on the same series wait for each other.
func TestNextConcurrentPostgres(t *testing.T) {
	dsn := os.Getenv("NANOFAKTURA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NANOFAKTURA_TEST_POSTGRES_DSN not set")
	}
	gdb, err := db.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	a := seedSeries(t, gdb, fmt.Sprintf("numbering-test-%d", os.Getpid()))
	t.Cleanup(func() {
		gdb.Exec("DELETE FROM number_counters WHERE number_format_id IN (SELECT id FROM number_formats WHERE account_id = ?)", a)
		gdb.Exec("DELETE FROM number_formats WHERE account_id = ?", a)
		gdb.Exec("DELETE FROM accounts WHERE id = ?", a)
	})
	got := concurrentNext(t, gdb, a, 16, 10)
	if len(got) == 0 {
		t.Fatal("nothing committed")
	}
	assertGapless(t, got)
}
