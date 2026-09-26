# NanoFaktura

Jednoduchý fakturační nástroj pro OSVČ. Go backend + React SPA, SQLite pro lokální provoz nebo PostgreSQL v Dockeru.

## Funkce

- Faktury, zálohové faktury, opravné daňové doklady
- Evidence odběratelů s napojením na [ARES](https://ares.gov.cz) (dohledání firmy podle IČO)
- Generování PDF faktur
- Číselné řady s automatickým číslováním
- Více uživatelů a více firem (účtů), přihlášení heslem, API tokeny
- OpenAPI dokumentace na `/api/docs`

## Spuštění

### Lokální binárka (SQLite)

```bash
# Nainstaluj Go 1.26+ a Node 22+
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

### Produkce — Portainer + Traefik

Repo obsahuje dva compose soubory:
- `docker-compose.yml` — univerzální základ (PostgreSQL + app)
- `docker-compose.traefik.yml` — overlay pro Traefik reverse proxy

**Portainer → Stacks → Add stack → Repository:**

| Pole | Hodnota |
|------|---------|
| Repository URL | `https://github.com/qwerin/nanofaktura` |
| Repository reference | `refs/heads/main` |
| Compose path | `docker-compose.yml` |
| Additional file | `docker-compose.traefik.yml` |

**Environment variables** (nebo nahraj `.env` soubor):

```env
POSTGRES_PASSWORD=silne_heslo
DOMAIN=nanofaktura.cz
CERT_RESOLVER=letsencrypt
TRAEFIK_NETWORK=proxy
```

> Při prvním spuštění se PostgreSQL volume inicializuje s zadaným heslem.
> Pokud změníš heslo, musíš smazat volume `nanofaktura_pgdata` a znovu nasadit.

## Konfigurace

Konfigurace se načítá z env proměnných.

| Env proměnná                  | Výchozí          | Popis                                               |
|-------------------------------|------------------|-----------------------------------------------------|
| `NANOFAKTURA_LISTEN_ADDR`     | `:8080`          | Adresa pro naslouchání                              |
| `NANOFAKTURA_DB_DRIVER`       | `sqlite`         | `sqlite` nebo `postgres`                            |
| `NANOFAKTURA_DB_DSN`          | `nanofaktura.db` | Cesta k SQLite souboru nebo PostgreSQL DSN          |
| `NANOFAKTURA_STATIC_DIR`      | *(prázdné)*      | Adresář s buildem frontendu (SPA)                   |
| `NANOFAKTURA_ALLOW_SIGNUP`    | `false`          | Povolit registraci i po vytvoření prvního uživatele |
| `NANOFAKTURA_SECURE_COOKIES`  | `false`          | Session cookie s příznakem `Secure` (za HTTPS)      |

### PostgreSQL DSN

```
host=localhost user=nanofaktura password=tajne dbname=nanofaktura sslmode=disable
```

## Vývoj

```bash
make dev-backend     # backend na :8080
make dev-frontend    # Vite na :5173, proxuje /api → :8080
make test            # go vet + Go testy + TypeScript typecheck
make gen-types       # přegeneruje TS typy z OpenAPI
```

Závazná specifikace (datový model, API, konvence) je v [`docs/SPEC.md`](docs/SPEC.md).
Interaktivní dokumentace API běží na `/api/docs`, OpenAPI schéma na `/api/openapi.json`.

## Tech stack

| Vrstva    | Technologie                                                   |
|-----------|---------------------------------------------------------------|
| Backend   | Go 1.26, [chi](https://github.com/go-chi/chi), [Huma v2](https://github.com/danielgtaylor/huma), GORM |
| Databáze  | SQLite (pure Go, bez CGO) nebo PostgreSQL                     |
| Frontend  | React 19, TypeScript, Vite, Tailwind v4, shadcn/ui            |
| Container | Docker (distroless), docker compose, PostgreSQL 17            |
