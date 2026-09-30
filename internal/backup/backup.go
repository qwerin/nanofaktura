// Package backup exports a whole account into a versioned ZIP archive and
// imports such an archive as a new account (SPEC §7.16). The API
// (GET /api/accounts/{slug}/backup, POST /api/accounts/import) and the CLI
// (nanofaktura backup export|import) both call Export and Import.
//
// Archive (version 1): manifest.json (format, version, counts, SHA-256 of
// every other file), account.json, one JSON array per entity (Files), and
// attachments.json + attachments/<id>/<filename>. The DTOs in dto.go are the
// file format; secrets (Fio tokens, webhook secrets, invitation/session/API
// tokens) are never exported.
package backup

import (
	"errors"
	"reflect"
	"runtime/debug"
)

const (
	// Format is manifest.format of every NanoFaktura backup.
	Format = "nanofaktura-backup"
	// Version is the archive format version written by Export; Import reads
	// versions 1..Version.
	Version = 1
)

// Errors of Import (wrapped with details; test with errors.Is).
var (
	ErrUnsupportedVersion = errors.New("unsupported backup version")
	ErrCorrupt            = errors.New("corrupt backup")
	ErrTooLarge           = errors.New("backup too large")
)

// DefaultMaxBytes is the default limit of the total uncompressed size of an
// imported archive (NANOFAKTURA_IMPORT_MAX_MB).
const DefaultMaxBytes = 512 << 20

// Per-entry limits of Import (on top of the total limit).
const (
	maxManifest   = 16 << 20
	maxAttachment = 64 << 20
	maxEntries    = 500_000
)

// File names of the entity files (JSON arrays), in the order they are written.
const (
	FileManifest         = "manifest.json"
	FileAccount          = "account.json"
	FileBankAccounts     = "bank_accounts.json"
	FileNumberFormats    = "number_formats.json"
	FileSubjects         = "subjects.json"
	FilePriceItems       = "price_items.json"
	FileStockMoves       = "stock_moves.json"
	FileInvoices         = "invoices.json"
	FileExpenses         = "expenses.json"
	FileTemplates        = "templates.json"
	FileRecurring        = "recurring.json"
	FileBankTransactions = "bank_transactions.json"
	FileTodos            = "todos.json"
	FileEvents           = "events.json"
	FileEmailLogs        = "email_logs.json"
	FileWebhooks         = "webhooks.json"
	FileMembers          = "members.json"
	FileAttachments      = "attachments.json"
	attachmentsDir       = "attachments/"
)

// AppVersion is the version of the running binary (module version or VCS
// revision from the build info), "dev" when unknown.
func AppVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	v := bi.Main.Version
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 12 {
			if v == "" || v == "(devel)" {
				v = s.Value[:12]
			} else {
				v += "+" + s.Value[:12]
			}
		}
	}
	if v == "" || v == "(devel)" {
		return "dev"
	}
	return v
}

// convert copies src into dst field by field by Go field name, recursing
// into structs, pointers, slices and maps of different types (model ↔ DTO).
// Embedded structs of dst missing in src are filled from src itself (the
// DTOs are flat). Fields without a counterpart stay zero.
func convert(dst, src any) {
	conv(reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem())
}

func conv(dst, src reflect.Value) {
	if !src.IsValid() {
		return
	}
	if src.Type() == dst.Type() {
		dst.Set(src)
		return
	}
	switch dst.Kind() {
	case reflect.Struct:
		if src.Kind() != reflect.Struct {
			return
		}
		t := dst.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if sf := src.FieldByName(f.Name); sf.IsValid() {
				conv(dst.Field(i), sf)
			} else if f.Anonymous && f.Type.Kind() == reflect.Struct {
				conv(dst.Field(i), src)
			}
		}
	case reflect.Pointer:
		if src.Kind() != reflect.Pointer || src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return
		}
		n := reflect.New(dst.Type().Elem())
		conv(n.Elem(), src.Elem())
		dst.Set(n)
	case reflect.Slice:
		if src.Kind() != reflect.Slice || src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return
		}
		s := reflect.MakeSlice(dst.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			conv(s.Index(i), src.Index(i))
		}
		dst.Set(s)
	case reflect.Map:
		if src.Kind() != reflect.Map || src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return
		}
		m := reflect.MakeMapWithSize(dst.Type(), src.Len())
		it := src.MapRange()
		for it.Next() {
			k := reflect.New(dst.Type().Key()).Elem()
			conv(k, it.Key())
			v := reflect.New(dst.Type().Elem()).Elem()
			conv(v, it.Value())
			m.SetMapIndex(k, v)
		}
		dst.Set(m)
	default:
		if src.Type().ConvertibleTo(dst.Type()) {
			dst.Set(src.Convert(dst.Type()))
		}
	}
}
