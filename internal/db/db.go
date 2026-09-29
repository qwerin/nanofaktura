// Package db opens the database connection and migrates the schema.
package db

import (
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/search"
)

// Open connects to "sqlite" (pure Go) or "postgres".
//
// SQLite runs in WAL mode with foreign keys on and a single connection, which
// serializes all access (writes cannot conflict, and ":memory:" databases are
// shared by the whole process). Consequence: inside a transaction always use
// the tx handle — a query on the outer *gorm.DB would wait forever.
//
// Duplicate-key errors are translated to gorm.ErrDuplicatedKey for both drivers.
func Open(driver, dsn string) (*gorm.DB, error) {
	cfg := &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Warn)}
	switch driver {
	case "sqlite":
		db, err := gorm.Open(sqlite.Open(sqliteDSN(dsn)), cfg)
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
		return db, nil
	case "postgres":
		db, err := gorm.Open(postgres.Open(dsn), cfg)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		return db, nil
	default:
		return nil, fmt.Errorf("unsupported db driver %q", driver)
	}
}

func sqliteDSN(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

// Migrate creates/updates the schema for all models and backfills derived
// columns (search_text of rows written before it existed).
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(model.All()...); err != nil {
		return err
	}
	return backfillSearchText(db)
}

// backfillSearchText fills search_text where it is empty (idempotent; rows
// with nothing searchable stay empty and are simply revisited next start).
func backfillSearchText(db *gorm.DB) error {
	if err := backfill[model.Invoice](db); err != nil {
		return err
	}
	if err := backfill[model.Expense](db); err != nil {
		return err
	}
	if err := backfill[model.Subject](db); err != nil {
		return err
	}
	return backfill[model.PriceItem](db)
}

func backfill[T any, PT interface {
	*T
	model.Searchable
}](db *gorm.DB) error {
	var rows []T
	return db.Model(new(T)).Where("search_text = '' OR search_text IS NULL").
		FindInBatches(&rows, 500, func(tx *gorm.DB, _ int) error {
			for i := range rows {
				p := PT(&rows[i])
				text := search.Text(p.SearchSource()...)
				if text == "" {
					continue
				}
				if err := db.Model(p).UpdateColumn("search_text", text).Error; err != nil {
					return fmt.Errorf("backfill search_text: %w", err)
				}
			}
			return nil
		}).Error
}
