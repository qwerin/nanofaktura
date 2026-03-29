# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Backend
make build          # go build -o bin/nanofaktura ./cmd/server/
make run            # build + run on :8080
make dev-backend    # go run ./cmd/server/ (no rebuild on change)

# Tests
make test           # go test ./cmd/... ./internal/... + frontend tsc --noEmit
make test-verbose   # same with -v
make test-api       # go test ./internal/handler/... -v -count=1
make test-models    # go test ./internal/models/... ./internal/ares/... -v -count=1

# Run a single test by name
go test ./internal/handler/... -run TestInvoice_Create -v -count=1

# Frontend
make dev-frontend   # cd web && npm run dev  (Vite on :5173, proxies /api → :8080)
make build-web      # cd web && npm run build

# Cleanup
make clean          # removes bin/, nanofaktura.db*, web/dist/
```

## Architecture

### Big picture

Single Go binary (`cmd/server/main.go`) serves a JSON REST API on `:8080`. The React SPA lives in `web/` and in dev mode proxies `/api/*` to the backend via Vite. In production the frontend is bundled into `web/dist/` and served statically.

```
cmd/server/main.go        → bootstrap: DB open, chi router, Huma API, register handlers
internal/database/        → SQLite Open() + AutoMigrate + ensureSettings()
internal/models/          → GORM structs (Invoice, InvoiceLine, Subject, Settings, NumberFormat)
internal/handler/         → Huma endpoint registration (one file per resource)
internal/handler/dto.go   → API input DTOs (separate from GORM models)
internal/ares/            → HTTP client for ARES (Czech business registry)
internal/pdf/             → Maroto v2 PDF generation
web/src/api/client.ts     → typed fetch wrapper for all backend endpoints
web/src/pages/            → React page components (one per route)
```

### Critical design decisions

**DTO pattern (handler/dto.go)** — Huma v2 marks every non-pointer Go field as `required` in JSON Schema. GORM models have many non-pointer computed/readonly fields (ID, timestamps, totals), so they cannot be used as Huma input types directly. `InvoiceInput`, `SubjectInput`, `SettingsInput`, `NumberFormatInput` are separate structs where all optional API fields have `omitempty`. The `toModel()` methods on each DTO handle mapping and apply defaults via `orDefault()` / `orDefaultInt()`.

**Money as int64 haléře** — All monetary values are stored and passed as `int64` in haléře (1 Kč = 100 hal). Never use `float64` for money. Quantities are `string` decimal (e.g. `"1.5"`). VAT rates are `int32` basis points (2100 = 21%). Frontend converts on display only.

**Dates as strings** — Date fields (`issued_on`, `taxable_fulfillment_due`, `paid_on`) are stored as `string` in `"YYYY-MM-DD"` format, not `time.Time`. Reason: Huma generates `date-time` format for `time.Time` and rejects date-only strings from the frontend.

**Settings singleton** — `Settings` is always ID=1, created automatically by `ensureSettings()` on startup. The `NumberFormat` records reference it via `SettingsID`. `Settings.NumberFormat.Generate()` formats and increments `NextNumber` — always call inside a DB transaction.

**Subject.CustomID is \*string** — Nullable unique index. Using `string` would cause a unique constraint violation when two subjects have empty CustomID; NULL values are excluded from unique index checks in SQLite.

**`your_*` fields on Invoice** — Company info is denormalized onto each invoice at creation time (for archival integrity, like Fakturoid). The frontend prefills them from Settings on mount; they are not shown in the form. `toModel()` in dto.go maps them from the hidden form state.

**Name collision: ares.Lookup** — The ARES result struct is named `Lookup` (not `Subject`) to avoid a name collision in Huma's OpenAPI schema registry with `models.Subject`.

### Handler pattern

Each resource gets its own `Register*` function in `internal/handler/`. Input/output types are defined as local structs inside the function to keep them scoped. Handlers call `db.WithContext(ctx)` for every query. Always `Preload("Lines")` when reading invoices. After `Create`, reload with `First` to get generated ID and timestamps.

### Testing

Tests live in `internal/handler/handler_test.go` (package `handler_test`). Each test calls `newTestAPIChi(t)` which opens a fresh `:memory:` SQLite — no cleanup needed. Use the `do()` helper to fire HTTP requests and `decodeBody()` to unmarshal Huma's wrapped response. Huma wraps all responses in `{"$schemaKey": {...}}` — the client normalizes this by taking `json[Object.keys(json)[0]]`.

### Frontend routing

`App.tsx` defines routes: `/invoices`, `/invoices/new`, `/invoices/:id`, `/subjects`, `/subjects/new`, `/settings`. The sidebar is dark slate-900 with violet accent. Components use shadcn/ui (Nova preset, Geist font, oklch CSS variables, Tailwind v4).

## Key constraints

- **huma/v2 pinned to v2.19.0** — newer versions require Go 1.25+. Do not upgrade without upgrading Go first.
- **SQLite WAL mode** — enabled in `database.Open()`. The DB files are `nanofaktura.db`, `nanofaktura.db-wal`, `nanofaktura.db-shm`.
- **GORM AutoMigrate only adds columns, never removes them.** If you rename or remove a model field that had a `NOT NULL` constraint, delete the DB file and let it recreate.
- **No CGO required** — using the modernc SQLite driver (`gorm.io/driver/sqlite` backed by `mattn/go-sqlite3` actually does require CGO — check `go.mod` if issues arise).
