package api

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Účetní kancelář Žluťoučký s.r.o.", "ucetni-kancelar-zlutoucky-s-r-o"},
		{"  ACME, a.s.  ", "acme-a-s"},
		{"Jan Novák 2", "jan-novak-2"},
		{"ŘŠČĚÝÁÍÉŮÚŤĎŇ", "rsceyaieuutdn"},
		{"!!!", "ucet"},
		{"", "ucet"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-b", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	for _, tt := range tests {
		if got := slugify(tt.in); got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
