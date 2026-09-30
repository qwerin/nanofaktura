# NanoFaktura

Open-source fakturace pro české OSVČ a malé firmy, kterou si provozujete sami. Jedna Go binárka s webovou
aplikací (React, navržená nejdřív pro mobil, instalovatelná jako PWA), SQLite pro lokální provoz nebo PostgreSQL
v Dockeru.

Co je nového pro uživatele, je v [`CHANGELOG.md`](CHANGELOG.md) — stejný text aplikace ukazuje v sekci *Novinky*.

## Funkce

- **Doklady** — faktury, zálohové faktury (→ konečná faktura), opravné doklady; číselné řady, výpočet DPH
  (plátce, neplátce, identifikovaná osoba, přenesená povinnost), cizí měny s kurzem ČNB, zamykání, storno
- **PDF a QR platba** — tři šablony, logo a razítko, čeština/angličtina/slovenština/němčina; ISDOC 6.0.2
- **Kontakty** s [ARES](https://ares.gov.cz), kontrolou nespolehlivého plátce DPH a VIES
- **Náklady** s přílohami (foto účtenky), **ceník a sklad**
- **Pravidelné faktury a šablony**, odesílání e-mailem, automatické upomínky, veřejný odkaz faktury pro klienta
- **Banka** — synchronizace s Fio API, import výpisů (ABO/GPC, CSV Fio/ČSOB/KB/Air Bank), automatické párování plateb
- **Přehledy a DPH** — tržby/náklady/zisk, srovnání s paušálem OSVČ, přiznání k DPH a kontrolní hlášení (XML pro EPO)
- **Exporty** — CSV/XLSX, ZIP s PDF za období, kompletní **záloha a obnova účtu**
- **Tým** — více uživatelů a firem, role (vlastník, administrátor, člen, účetní), pozvánky
- **Zabezpečení** — dvoufázové ověření (TOTP, bezpečnostní klíče/passkeys), obnova hesla, limity pokusů, CSP
- **Pro integrace** — REST API s API tokeny, OpenAPI na `/api/docs`, webhooky s HMAC podpisem
- Úkoly, historie změn, rychlé hledání (`Ctrl K`), tmavý režim

## Spuštění

### Lokálně (SQLite)

```bash
# Go 1.26+ a Node 22+
make build-web   # sestaví frontend do web/dist
make build       # sestaví Go binárku do bin/nanofaktura
NANOFAKTURA_STATIC_DIR=web/dist ./bin/nanofaktura
# → http://localhost:8080  (databáze nanofaktura.db v aktuálním adresáři)
```

### Docker Compose (PostgreSQL)

```bash
cp .env.example .env     # vyplň POSTGRES_PASSWORD a podle potřeby další proměnné
docker compose up -d
# → http://localhost:8080  (port je ve výchozím stavu dostupný jen z localhostu)
```

### Produkce — Portainer + Traefik

Repo obsahuje dva compose soubory:

- `docker-compose.yml` — základ (PostgreSQL + aplikace)
- `docker-compose.traefik.yml` — overlay pro reverzní proxy Traefik s HTTPS

**Portainer → Stacks → Add stack → Repository:**

| Pole | Hodnota |
|------|---------|
| Repository URL | `https://github.com/qwerin/nanofaktura` |
| Repository reference | `refs/heads/main` |
| Compose path | `docker-compose.yml` |
| Additional file | `docker-compose.traefik.yml` |

**Environment variables** (nebo nahraj `.env`):

```env
# infrastruktura
POSTGRES_PASSWORD=silne_heslo
DOMAIN=nanofaktura.cz
CERT_RESOLVER=letsencrypt
TRAEFIK_NETWORK=proxy
# aplikace (plné názvy, viz Konfigurace)
NANOFAKTURA_SETUP_TOKEN=nahodny_retezec        # openssl rand -hex 16
NANOFAKTURA_SECRET_KEY=                       # openssl rand -base64 32 (prázdný = vygeneruje se do /data)
NANOFAKTURA_SMTP_HOST=smtp.example.cz
NANOFAKTURA_SMTP_USER=faktury@example.cz
NANOFAKTURA_SMTP_PASSWORD=…
NANOFAKTURA_MAIL_FROM="Moje firma <faktury@example.cz>"
```

Overlay nastaví `NANOFAKTURA_PUBLIC_URL=https://${DOMAIN}`, Secure cookie a důvěru v `X-Forwarded-*` od Traefiku
a **zruší publikovaný port 8080** — aplikace je dostupná jen přes HTTPS. Vyžaduje Docker Compose ≥ 2.24
(kvůli `!reset`); ověření: `docker compose -f docker-compose.yml -f docker-compose.traefik.yml config` nesmí
u služby `app` vypsat `ports`.

> PostgreSQL volume se inicializuje heslem při prvním spuštění. Pokud heslo později změníš, musíš smazat volume
> `nanofaktura_pgdata` a nasadit znovu (nebo heslo změnit přímo v databázi).

## Konfigurace

Aplikace se nastavuje **jen proměnnými prostředí `NANOFAKTURA_*`**. Kde je zadat:

- **bez Dockeru** — v prostředí procesu (`NANOFAKTURA_SMTP_HOST=… ./bin/nanofaktura`);
- **v Dockeru** — do `.env` vedle `docker-compose.yml`, nebo do *Environment variables* stacku v Portaineru,
  **vždy pod stejným plným názvem**. Compose je do kontejneru předá beze změny.

Pro starší `.env` fungují u několika proměnných i krátké názvy (sloupec *Starší zkratka*); když jsou vyplněné oba,
vyhrává plný název. Nové nastavení pište plnými názvy.

### Proměnné aplikace

| Proměnná | Starší zkratka | Výchozí | Popis |
|---|---|---|---|
| **Základ** | | | |
| `NANOFAKTURA_PUBLIC_URL` | `PUBLIC_URL` | `http://localhost:8080` | Veřejná adresa (odkazy v e-mailech, bezpečnostní klíče); `https://` zapne Secure cookie a HSTS |
| `NANOFAKTURA_LISTEN_ADDR` | – | `:8080` | Adresa, na které server naslouchá |
| `NANOFAKTURA_DB_DRIVER` | – | `sqlite` | `sqlite` nebo `postgres` (compose nastavuje `postgres`) |
| `NANOFAKTURA_DB_DSN` | – | `nanofaktura.db` | Soubor SQLite nebo PostgreSQL DSN (compose nastavuje sám) |
| `NANOFAKTURA_STATIC_DIR` | – | *(prázdné)* | Adresář s buildem frontendu (v Dockeru `/app/dist`) |
| `NANOFAKTURA_DATA_DIR` | – | `./data` | Přílohy a `secret.key` (v Dockeru volume `/data`) |
| **Zabezpečení** | | | |
| `NANOFAKTURA_SETUP_TOKEN` | `SETUP_TOKEN` | *(prázdné)* | Token vyžadovaný při první registraci prázdné instance |
| `NANOFAKTURA_SECRET_KEY` | `SECRET_KEY` | *(prázdné)* | Klíč šifrující uložená tajemství (tokeny Fio); prázdný = `DATA_DIR/secret.key` |
| `NANOFAKTURA_ALLOW_SIGNUP` | – | `false` | Povolit registraci i po vytvoření prvního uživatele |
| `NANOFAKTURA_SECURE_COOKIES` | – | `auto` | `Secure` u session cookie: `auto` podle HTTPS, `true`/`false` vynutí |
| `NANOFAKTURA_TRUSTED_PROXIES` | `TRUSTED_PROXIES` | *(prázdné)* | IP/CIDR reverzních proxy, kterým se věří `X-Forwarded-For`/`-Proto` |
| `NANOFAKTURA_DISABLE_RATE_LIMIT` | – | `false` | Vypne limity pokusů (přihlášení, registrace, e-maily …) |
| `NANOFAKTURA_DISABLE_API_DOCS` | – | `false` | Skryje `/api/docs` a `/api/openapi.json` |
| `NANOFAKTURA_WEBAUTHN_ORIGINS` | – | *(prázdné)* | Další adresy pro bezpečnostní klíče (čárkou; vývoj s Vite) |
| **E-mail** | | | |
| `NANOFAKTURA_SMTP_HOST` | `SMTP_HOST` | *(prázdné)* | SMTP server; prázdný = e-maily se jen vypíšou do logu |
| `NANOFAKTURA_SMTP_PORT` | `SMTP_PORT` | `587` | Port (465 pro `tls`) |
| `NANOFAKTURA_SMTP_TLS` | `SMTP_TLS` | `starttls` | `starttls`, `tls` (implicitní) nebo `none` |
| `NANOFAKTURA_SMTP_USER` | `SMTP_USER` | *(prázdné)* | Přihlašovací jméno k SMTP (prázdné = bez přihlášení) |
| `NANOFAKTURA_SMTP_PASSWORD` | `SMTP_PASSWORD` | *(prázdné)* | Heslo k SMTP |
| `NANOFAKTURA_MAIL_FROM` | `MAIL_FROM` | `NanoFaktura <nanofaktura@localhost>` | Odesílatel e-mailů |
| **Databáze a logy** | | | |
| `NANOFAKTURA_DB_LOG` | – | `error` | Logování SQL: `silent`, `error`, `warn` (+ pomalé dotazy), `info` (vše); hodnoty se nelogují |
| `NANOFAKTURA_DB_SLOW_MS` | – | `1000` | Hranice pomalého dotazu pro `warn` |
| `NANOFAKTURA_IMPORT_MAX_MB` | – | `512` | Limit velikosti zálohy při obnově účtu |
| **Pokročilé** | | | |
| `NANOFAKTURA_WEBHOOKS_ALLOW_PRIVATE` | – | `false` | Povolí webhooky na privátní/lokální adresy (jen LAN, testy) |
| `NANOFAKTURA_ARES_URL`, `_CNB_URL`, `_VIES_URL`, `_VATREG_URL`, `_FIO_URL` | – | oficiální služby | Přesměrování externích služeb (testy, proxy) |

### Proměnné jen pro Docker Compose

Tyto nejsou pro aplikaci, ale pro compose soubory:

| Proměnná | Výchozí | Popis |
|---|---|---|
| `POSTGRES_PASSWORD` | – (povinné) | Heslo databáze PostgreSQL |
| `APP_PORT` | `8080` | Port, na kterém je aplikace publikovaná (jen bez Traefiku) |
| `APP_BIND` | `127.0.0.1` | Rozhraní publikovaného portu; `0.0.0.0` jen v důvěryhodné LAN |
| `DOMAIN` | – | Doména pro Traefik (overlay z ní odvodí `NANOFAKTURA_PUBLIC_URL`) |
| `CERT_RESOLVER` | – | Certificate resolver Traefiku (např. `letsencrypt`) |
| `TRAEFIK_NETWORK` | – | Externí síť Dockeru, ve které běží Traefik |

### PostgreSQL DSN (bez Dockeru)

```
host=localhost user=nanofaktura password=tajne dbname=nanofaktura sslmode=disable
```

## Produkční checklist

- **HTTPS** — aplikaci provozujte jen za reverzní proxy s TLS a nastavte `NANOFAKTURA_PUBLIC_URL=https://…`
  (zapne `Secure` cookie a HSTS). Port aplikace nepublikujte veřejně (základní compose ho váže jen na `127.0.0.1`).
- **`NANOFAKTURA_SETUP_TOKEN`** — nastavte náhodný řetězec ještě před prvním spuštěním na veřejné adrese;
  při registraci prvního účtu ho zadáte. Bez něj se může jako první zaregistrovat kdokoli.
- **`NANOFAKTURA_TRUSTED_PROXIES`** — adresy/podsíť vaší reverzní proxy. Jen od nich se věří `X-Forwarded-For`
  (limity pokusů podle IP klienta) a `X-Forwarded-Proto`. Nikdy nezadávejte adresy, ze kterých se k aplikaci
  dostane kdokoli přímo. Traefik overlay má výchozí privátní rozsahy (bezpečné jen proto, že port není publikovaný).
- **SMTP** — bez `NANOFAKTURA_SMTP_HOST` se e-maily (i pozvánky a obnova hesla) jen vypisují do logu.
- **Zálohujte** databázi i datový volume (`/data`: přílohy a `secret.key`), nebo nastavte `NANOFAKTURA_SECRET_KEY`.
- **Logy** — výchozí `NANOFAKTURA_DB_LOG=error` vypisuje jen chyby databáze; při ladění `warn` nebo `info`.
- Server při startu varuje, pokud něco z výše uvedeného chybí.

## Správa z příkazové řádky

Binárka má kromě serveru i příkazy pro správce. Používají stejné proměnné `NANOFAKTURA_*` jako server
(v Dockeru: `docker compose exec app /app/nanofaktura …`).

```bash
nanofaktura                    # (nebo `nanofaktura serve`) spustí server

# Záloha a obnova účtu — třeba i pro přenos ze SQLite na PostgreSQL
nanofaktura backup export --account moje-firma --out zaloha.zip
NANOFAKTURA_DB_DRIVER=postgres NANOFAKTURA_DB_DSN="host=…" \
  nanofaktura backup import --owner jan@example.cz [--name "Moje firma"] zaloha.zip

# Uživatel ztratil druhý faktor i záložní kódy
nanofaktura user reset-2fa --email jan@example.cz
```

### Záloha a přenos účtu

Zálohu účtu (ZIP se všemi daty a přílohami, bez hesel a tajných tokenů) stáhne vlastník/administrátor
v *Nastavení → Záloha a přenos*; obnova v přepínači účtů *Nový účet → Obnovit ze zálohy* vždy založí nový účet.
Po obnově jsou pravidelné faktury, upomínky a webhooky vypnuté a tokeny Fio je třeba zadat znovu.

### Zapomenuté heslo a dvoufázové ověření

Odkaz pro obnovu hesla chodí e-mailem, takže musí být nastavené SMTP a `NANOFAKTURA_PUBLIC_URL`. Uživatelé si
v *Nastavení → Zabezpečení* zapnou druhý faktor: ověřovací aplikaci (TOTP) nebo bezpečnostní klíč / passkey
(WebAuthn, např. YubiKey). Klíče jsou vázané na doménu z `NANOFAKTURA_PUBLIC_URL` a prohlížeč je dovolí jen přes
HTTPS (výjimkou je `localhost`); po změně domény je třeba je přidat znovu.

### Migrace z jiných fakturačních aplikací

Data lze do NanoFaktury nahrát přes REST API (API token z *Nastavení → API tokeny*) — kontakty, doklady
s položkami, platbami a přílohami. Před importem historických dokladů vypněte v *Nastavení → E-maily
a upomínky* automatické upomínky i poděkování za platbu, jinak by klientům odešly e-maily ke starým fakturám.

## Vývoj

```bash
make dev-backend     # backend na :8080
make dev-frontend    # Vite na :5173, proxuje /api → :8080
make test            # go vet + Go testy + TypeScript typecheck + frontend testy (vitest)
make gen-types       # přegeneruje TS typy z OpenAPI (po každé změně API)
```

Závazná specifikace (datový model, API, konvence) je v [`docs/SPEC.md`](docs/SPEC.md), pravidla pro přispěvatele
v [`CLAUDE.md`](CLAUDE.md). Interaktivní dokumentace API běží na `/api/docs`, OpenAPI schéma na `/api/openapi.json`.

## Tech stack

| Vrstva    | Technologie |
|-----------|-------------|
| Backend   | Go 1.26 (≥ 1.26.8), [chi](https://github.com/go-chi/chi), [Huma v2](https://github.com/danielgtaylor/huma), GORM |
| Databáze  | SQLite (pure Go, bez CGO) nebo PostgreSQL |
| Frontend  | React 19, TypeScript, Vite, TanStack Router + Query, Tailwind v4, shadcn/ui, PWA |
| Container | Docker (distroless), docker compose, PostgreSQL 17 |
