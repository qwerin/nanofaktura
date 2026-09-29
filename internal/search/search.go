// Package search normalizes texts for diacritics- and case-insensitive
// search that works the same on SQLite and PostgreSQL: searchable records
// store Text(<their searchable fields>) in a search_text column (GORM
// BeforeSave hooks in internal/model) and queries compare it with LIKE
// against LikePattern(query).
package search

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Fold lower-cases s, removes diacritics and collapses whitespace
// ("Žluťoučký  KŮŇ" → "zlutoucky kun").
func Fold(s string) string {
	s, _, _ = transform.String(stripMarks, strings.ToLower(s))
	return strings.Join(strings.Fields(s), " ")
}

// Text folds and joins the non-empty parts with " | " (the separator keeps
// a query from matching across two fields).
func Text(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = Fold(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " | ")
}

// LikePattern is the LIKE pattern matching Fold(q) anywhere, with % and _
// escaped by backslash (use it with ESCAPE '\').
func LikePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(Fold(q)) + "%"
}
