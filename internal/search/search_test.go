package search

import "testing"

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Žluťoučký  KŮŇ": "zlutoucky kun",
		" 2026-0001 ":     "2026-0001",
		"":                "",
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
