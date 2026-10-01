package search

import (
	"strings"
	"sync"
	"testing"
)

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Žluťoučký  KŮŇ": "zlutoucky kun",
		" 2026-0001 ":    "2026-0001",
		"":               "",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Text("Nováková", "", "12345678"); got != "novakova | 12345678" {
		t.Errorf("Text: %q", got)
	}
	if got := LikePattern("50%_x"); got != `%50\%\_x%` {
		t.Errorf("LikePattern: %q", got)
	}
}

// Fold is called from GORM hooks and request handlers concurrently; a shared
// transform chain corrupted its buffers (run with -race).
func TestFoldConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			in := strings.Repeat("Žluťoučký kůň ", 50+i)
			want := strings.TrimSpace(strings.Repeat("zlutoucky kun ", 50+i))
			for range 200 {
				if got := Fold(in); got != want {
					t.Errorf("Fold = %q", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}
