package api

import (
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/slug"
)

// slugify / uniqueSlug: see internal/slug (shared with internal/backup).
func slugify(name string) string { return slug.Make(name) }

func uniqueSlug(tx *gorm.DB, name string) (string, error) { return slug.Unique(tx, name) }
