package slug

import (
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

func TestMake(t *testing.T) {
	for in, want := range map[string]string{
		"Účetní s.r.o.":           "ucetni-s-r-o",
		"  Firma   ABC  ":         "firma-abc",
		"ŽLUŤOUČKÝ kůň 2026":      "zlutoucky-kun-2026",
		"---":                     "ucet",
		"":                        "ucet",
		"日本":                      "ucet",
		"a&b":                     "a-b",
		strings.Repeat("x", 60):   strings.Repeat("x", MaxLen),
		strings.Repeat("ab ", 30): strings.TrimRight(strings.Repeat("ab-", 17)[:MaxLen], "-"),
	} {
		if got := Make(in); got != want {
			t.Errorf("Make(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMakeProperties: for random input the slug is always non-empty,
// [a-z0-9-], at most MaxLen, without leading/trailing/double dashes, and
// Make is idempotent.
func TestMakeProperties(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	alphabet := []rune("aáäbcčdďeéěfghiíjklľmnňoóôpqrřsštťuúůvwxyýzžAÁČŽ0123456789 -_.,&/@ščř\t日")
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		n := rng.IntN(80)
		rs := make([]rune, n)
		for i := range rs {
			rs[i] = alphabet[rng.IntN(len(alphabet))]
		}
		in := string(rs)
		got := Make(in)
		if len(got) > MaxLen || !valid.MatchString(got) {
			t.Fatalf("Make(%q) = %q invalid", in, got)
		}
		if again := Make(got); again != got {
			t.Fatalf("Make not idempotent: %q → %q → %q", in, got, again)
		}
	}
}

func TestUnique(t *testing.T) {
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"firma", "firma-2", "firma-3"} {
		got, err := Unique(gdb, "Firma")
		if err != nil || got != want {
			t.Fatalf("#%d: %q %v, want %q", i, got, err, want)
		}
		if err := gdb.Create(&model.Account{Slug: got, Name: "Firma"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, _ := gdb.DB()
	sqlDB.Close()
	if _, err := Unique(gdb, "Firma"); err == nil {
		t.Fatal("closed DB: no error")
	}
}
