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

// Migrate creates/updates the schema for all models.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(model.All()...)
}
