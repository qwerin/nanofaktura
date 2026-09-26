package api

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/model"
)

const maxSlugLen = 50

// slugify turns a name into [a-z0-9-]: diacritics are stripped ("Účetní s.r.o."
// → "ucetni-s-r-o"), every other character run becomes a single dash.
func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// combining mark left over from NFD decomposition
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	slug := strings.TrimRight(b.String()[:min(b.Len(), maxSlugLen)], "-")
	if slug == "" {
		return "ucet"
	}
	return slug
}

// uniqueSlug returns slugify(name), suffixed with -2, -3… if already taken.
func uniqueSlug(tx *gorm.DB, name string) (string, error) {
	base := slugify(name)
	for i := 1; ; i++ {
		slug := base
		if i > 1 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		var n int64
		if err := tx.Model(&model.Account{}).Where("slug = ?", slug).Count(&n).Error; err != nil {
			return "", err
		}
		if n == 0 {
			return slug, nil
		}
	}
}
