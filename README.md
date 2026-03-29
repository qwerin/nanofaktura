# NanoFaktura

Jednoduchý fakturační nástroj pro OSVČ. Go backend + React SPA, SQLite pro lokální provoz nebo PostgreSQL v Dockeru.

## Funkce

- Faktury, zálohové faktury, opravné daňové doklady
- Evidece odběratelů s napojením na [ARES](https://ares.gov.cz) (dohledání firmy podle IČO)
- Generování PDF faktur
- Číselné řady s automatickým číslováním
- Volitelný multi-user mód s přihlašováním a session cookies
- OpenAPI dokumentace na `/api/docs`

## Spuštění

### Lokální binárka (SQLite)

```bash
# Nainstaluj Go 1.25+ a Node 22+
make build-web   # sestav frontend
make build       # sestav Go binárku
./bin/nanofaktura
# → http://localhost:8080
```

Ve výchozím nastavení se vytvoří `nanofaktura.db` v aktuálním adresáři.

### Docker (PostgreSQL)

```bash
cp .env.example .env
# Nastav POSTGRES_PASSWORD v .env
docker compose up -d
# → http://localhost:8080
```

## Konfigurace

Konfigurace se načítá v pořadí: výchozí hodnoty → `nanofaktura.json` → env proměnné → CLI flags.

| Env proměnná                | Flag            | Výchozí          | Popis                                      |
|-----------------------------|-----------------|------------------|--------------------------------------------|
| `NANOFAKTURA_DB_DRIVER`     | `--db-driver`   | `sqlite`         | `sqlite` nebo `postgres`                   |
| `NANOFAKTURA_DB_PATH`       | `--db`          | `nanofaktura.db` | Cesta k SQLite souboru nebo postgres DSN   |
| `NANOFAKTURA_LISTEN_ADDR`   | `--addr`        | `:8080`          | Adresa pro naslouchání                     |
| `NANOFAKTURA_MULTI_USER`    | `--multi-user`  | `false`          | Zapnout multi-user mód s přihlašováním     |
| `NANOFAKTURA_SESSION_TTL`   | `--session-ttl` | `720`            | Platnost session v hodinách (30 dní)       |
| `NANOFAKTURA_STATIC_DIR`    | `--static-dir`  | *(prázdné)*      | Adresář s frontend buildem (SPA serving)   |

### Příklad `nanofaktura.json`

```json
{
  "db_driver": "sqlite",
  "db_path": "/data/nanofaktura.db",
  "listen_addr": ":8080",
  "multi_user": false
}
```

### PostgreSQL DSN

```
host=localhost user=nanofaktura password=tajne dbname=nanofaktura sslmode=disable
```

## Vývoj

```bash
# Backend (port 8080, SQLite :memory: pro testy)
make dev-backend     # go run ./cmd/server/

# Frontend (port 5173, proxuje /api → :8080)
make dev-frontend    # cd web && npm run dev

# Testy
make test            # Go testy + TypeScript typecheck
make test-api        # jen handler testy
make test-verbose    # testy s výpisem

# Spustit konkrétní test
go test ./internal/handler/... -run TestInvoice_Create -v -count=1
```

## API

Všechny endpointy jsou pod prefixem `/api`. Interaktivní dokumentace (Swagger UI) je dostupná na `/api/docs`.

| Metoda   | Cesta                                  | Popis                              |
|----------|----------------------------------------|------------------------------------|
| `GET`    | `/api/health`                          | Health check                       |
| `POST`   | `/api/auth/login`                      | Přihlášení (multi-user)            |
| `POST`   | `/api/auth/logout`                     | Odhlášení                          |
| `GET`    | `/api/auth/me`                         | Info o přihlášeném uživateli       |
| `POST`   | `/api/setup/init`                      | First-run wizard                   |
| `GET`    | `/api/invoices`                        | Seznam faktur                      |
| `POST`   | `/api/invoices`                        | Nová faktura                       |
| `GET`    | `/api/invoices/{id}`                   | Detail faktury                     |
| `PUT`    | `/api/invoices/{id}`                   | Úprava faktury                     |
| `DELETE` | `/api/invoices/{id}`                   | Smazání faktury                    |
| `GET`    | `/api/invoices/{id}/pdf`               | Stažení PDF                        |
| `POST`   | `/api/invoices/{id}/duplicate`         | Duplikace faktury                  |
| `PUT`    | `/api/invoices/{id}/status`            | Změna stavu faktury                |
| `GET`    | `/api/subjects`                        | Seznam odběratelů                  |
| `POST`   | `/api/subjects`                        | Nový odběratel                     |
| `GET`    | `/api/subjects/{id}`                   | Detail odběratele                  |
| `PUT`    | `/api/subjects/{id}`                   | Úprava odběratele                  |
| `DELETE` | `/api/subjects/{id}`                   | Smazání odběratele                 |
| `GET`    | `/api/ares/{ic}`                       | Vyhledání v ARES podle IČO         |
| `GET`    | `/api/settings`                        | Nastavení firmy                    |
| `PUT`    | `/api/settings`                        | Uložení nastavení                  |
| `GET`    | `/api/settings/number-formats`         | Číselné řady                       |
| `POST`   | `/api/settings/number-formats`         | Nová číselná řada                  |
| `PUT`    | `/api/settings/number-formats/{id}`    | Úprava číselné řady                |
| `DELETE` | `/api/settings/number-formats/{id}`    | Smazání číselné řady               |
| `GET`    | `/api/users`                           | Seznam uživatelů (multi-user)      |
| `POST`   | `/api/users`                           | Nový uživatel                      |
| `PUT`    | `/api/users/{id}`                      | Úprava uživatele                   |
| `POST`   | `/api/users/{id}/reset-password`       | Reset hesla                        |

## Architektura

```
cmd/server/main.go        → bootstrap: DB, chi router, Huma API, handlery
internal/config/          → konfigurace (výchozí → JSON → env → flags)
internal/database/        → GORM Open() + AutoMigrate (SQLite nebo PostgreSQL)
internal/models/          → GORM modely (Invoice, Subject, Settings, ...)
internal/handler/         → Huma endpoint handlery + DTO typy
internal/ares/            → HTTP klient pro ARES
internal/auth/            → session autentizace (multi-user mód)
internal/pdf/             → generování PDF přes Maroto v2
web/src/                  → React SPA (React Router, shadcn/ui, Tailwind v4)
```

**Peníze jako int64 haléře** — všechny peněžní hodnoty jsou v haléřích (1 Kč = 100 hal), nikdy `float64`.

## Tech stack

| Vrstva    | Technologie                                                   |
|-----------|---------------------------------------------------------------|
| Backend   | Go 1.25, [chi](https://github.com/go-chi/chi), [Huma v2](https://github.com/danielgtaylor/huma), GORM |
| Databáze  | SQLite (mattn/go-sqlite3) nebo PostgreSQL (pgx v5)            |
| PDF       | [Maroto v2](https://github.com/johnfercher/maroto)            |
| Frontend  | React 19, TypeScript, Vite, Tailwind v4, shadcn/ui            |
| Container | Docker, docker compose, PostgreSQL 17                         |
