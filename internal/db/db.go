// Package db opens the database connection and migrates the schema.
package db

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

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
func Open(driver, dsn string, opts ...Option) (*gorm.DB, error) {
	o := options{logLevel: "error", slow: time.Second, out: os.Stderr}
	for _, opt := range opts {
		opt(&o)
	}
	lg, err := newLogger(o)
	if err != nil {
		return nil, err
	}
	cfg := &gorm.Config{TranslateError: true, Logger: lg}
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

// Option configures Open.
type Option func(*options)

type options struct {
	logLevel string
	slow     time.Duration
	out      io.Writer
}

// WithLogging sets the SQL log level (silent|error|warn|info; "" = error) and
// the slow-query threshold reported at warn level (0 = default 1 s).
func WithLogging(level string, slow time.Duration) Option {
	return func(o *options) {
		if level != "" {
			o.logLevel = level
		}
		if slow > 0 {
			o.slow = slow
		}
	}
}

// newLogger logs to stderr without colors and without query parameters
// (values such as e-mails or names never reach the log); "record not found"
// is a normal outcome of lookups and is never logged.
func newLogger(o options) (logger.Interface, error) {
	levels := map[string]logger.LogLevel{"silent": logger.Silent, "error": logger.Error, "warn": logger.Warn, "info": logger.Info}
	lvl, ok := levels[strings.ToLower(o.logLevel)]
	if !ok {
		return nil, fmt.Errorf("NANOFAKTURA_DB_LOG: unknown level %q (silent|error|warn|info)", o.logLevel)
	}
	return logger.New(log.New(o.out, "", log.LstdFlags), logger.Config{
		SlowThreshold:             o.slow,
		LogLevel:                  lvl,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	}), nil
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
