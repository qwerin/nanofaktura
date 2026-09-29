package events

import "testing"

func TestMatches(t *testing.T) {
	for _, c := range []struct {
		patterns []string
		name     string
		want     bool
	}{
		{[]string{"*"}, "invoice.paid", true},
		{[]string{"invoice.*"}, "invoice.paid", true},
		{[]string{"invoice.*"}, "invoices.x", false},
		{[]string{"invoice.paid"}, "invoice.paid", true},
		{[]string{"invoice.paid"}, "invoice.sent", false},
		{nil, "invoice.paid", false},
	} {
		if got := Matches(c.patterns, c.name); got != c.want {
			t.Errorf("Matches(%v, %s) = %v", c.patterns, c.name, got)
		}
	}
	for p, want := range map[string]bool{"*": true, "invoice.*": true, "invoice.paid": true, "foo.*": false, "invoice.nope": false} {
		if ValidPattern(p) != want {
			t.Errorf("ValidPattern(%s) != %v", p, want)
		}
	}
}
