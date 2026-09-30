# NanoFaktura

Open-source fakturace pro české OSVČ a malé firmy, kterou si provozujete sami. Jedna Go binárka s webovou aplikací
(React, navržená nejdřív pro mobil, instalovatelná jako PWA), SQLite pro lokální provoz nebo PostgreSQL v Dockeru.

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
SETUP_TOKEN=nahodny_retezec            # openssl rand -hex 16 — zadá se při první registraci
SECRET_KEY=                           # openssl rand -base64 32 (prázdný = vygeneruje se do /data)
TRUSTED_PROXIES=172.18.0.0/16         # podsíť Traefiku (výchozí: privátní rozsahy)
SMTP_HOST=smtp.example.cz
SMTP_PORT=587
SMTP_TLS=starttls
SMTP_USER=faktury@example.cz
SMTP_PASSWORD=…
MAIL_FROM="Moje firma <faktury@example.cz>"
```

Overlay nastaví `NANOFAKTURA_PUBLIC_URL=https://${DOMAIN}`, Secure cookie a důvěru v `X-Forwarded-*` od Traefiku
a **zruší publikovaný port 8080** (aplikace je dostupná jen přes HTTPS; vyžaduje Docker Compose ≥ 2.24 kvůli `!reset`).
Ověření: `docker compose -f docker-compose.yml -f docker-compose.traefik.yml config` nesmí u `app` vypsat `ports`.

### Produkční checklist

- **HTTPS** — aplikaci provozujte jen za reverzní proxy s TLS a nastavte `NANOFAKTURA_PUBLIC_URL=https://…`
  (zapne `Secure` cookie a HSTS). Port aplikace nepublikujte veřejně (základní compose ho váže jen na `127.0.0.1`).
- **`NANOFAKTURA_SETUP_TOKEN`** — nastavte náhodný řetězec (`openssl rand -hex 16`) ještě před prvním spuštěním
  na veřejné adrese; při registraci prvního účtu ho zadáte. Bez něj se může jako první zaregistrovat kdokoli.
- **Logy** — výchozí `NANOFAKTURA_DB_LOG=error` vypisuje jen chyby databáze; při ladění `warn` (+ pomalé dotazy nad `NANOFAKTURA_DB_SLOW_MS`) nebo `info` (všechny dotazy). Hodnoty parametrů se do logu nedostanou.
- **`NANOFAKTURA_TRUSTED_PROXIES`** — adresy/podsíť vaší reverzní proxy (např. síť Traefiku). Jen od nich se věří
  `X-Forwarded-For` (limity pokusů podle IP klienta) a `X-Forwarded-Proto`. Nikdy nezadávejte adresy, ze kterých
  se k aplikaci dostane kdokoli přímo.
- **SMTP** — bez `NANOFAKTURA_SMTP_HOST` se e-maily (i odkazy pozvánek) jen vypisují do logu.
- **Zálohujte** databázi i datový volume (`/data`: přílohy a `secret.key`), nebo nastavte `NANOFAKTURA_SECRET_KEY`.
- Server při startu varuje, pokud některé z výše uvedeného chybí.

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
| `NANOFAKTURA_DATA_DIR`        | `./data`         | Přílohy a `secret.key` (v Dockeru volume `/data`)   |
| `NANOFAKTURA_SECRET_KEY`      | *(prázdné)*      | Klíč šifrující uložená tajemství (tokeny Fio); prázdný = `DATA_DIR/secret.key` |
| `NANOFAKTURA_ALLOW_SIGNUP`    | `false`          | Povolit registraci i po vytvoření prvního uživatele |
| `NANOFAKTURA_PUBLIC_URL`      | `http://localhost:8080` | Veřejná adresa (odkazy v e-mailech; `https://` zapne Secure cookie a HSTS) |
| `NANOFAKTURA_SECURE_COOKIES`  | `auto`           | `Secure` u session cookie: `auto` podle HTTPS, `true`/`false` vynutí |
| `NANOFAKTURA_TRUSTED_PROXIES` | *(prázdné)*      | IP/CIDR reverzních proxy, kterým se věří `X-Forwarded-For`/`-Proto` |
| `NANOFAKTURA_SETUP_TOKEN`     | *(prázdné)*      | Token vyžadovaný při první registraci prázdné instance |
| `NANOFAKTURA_DISABLE_RATE_LIMIT` | `false`       | Vypne limity pokusů (přihlášení, registrace, e-maily …) |
| `NANOFAKTURA_DISABLE_API_DOCS` | `false`         | Skryje `/api/docs` a `/api/openapi.json`            |
| `NANOFAKTURA_IMPORT_MAX_MB`   | `512`            | Limit velikosti zálohy při obnově účtu              |
| `NANOFAKTURA_SMTP_HOST`       | *(prázdné)*      | SMTP server; prázdný = e-maily se jen vypíšou do logu |
| `NANOFAKTURA_SMTP_PORT`       | `587`            | Port (465 pro `tls`)                                |
| `NANOFAKTURA_SMTP_TLS`        | `starttls`       | `starttls`, `tls` (implicitní) nebo `none`          |
| `NANOFAKTURA_SMTP_USER` / `_PASSWORD` | *(prázdné)* | Přihlášení k SMTP (prázdné = bez AUTH)        |
| `NANOFAKTURA_MAIL_FROM`       | `NanoFaktura <nanofaktura@localhost>` | Odesílatel e-mailů             |
| `NANOFAKTURA_DB_LOG`          | `error`          | Logování SQL: `silent`, `error`, `warn` (+ pomalé dotazy), `info` (vše); hodnoty se nelogují |
| `NANOFAKTURA_DB_SLOW_MS`      | `1000`           | Hranice pomalého dotazu pro `warn`                  |
| `NANOFAKTURA_WEBHOOKS_ALLOW_PRIVATE` | `false`   | Povolí webhooky na privátní/lokální adresy (jen LAN, testy) |
| `NANOFAKTURA_ARES_URL`, `_CNB_URL`, `_VIES_URL`, `_VATREG_URL`, `_FIO_URL` | oficiální služby | Přesměrování externích služeb (testy, proxy) |
| `NANOFAKTURA_WEBAUTHN_ORIGINS`| –                | Další adresy pro bezpečnostní klíče (čárkou), vývoj |

### Záloha a přenos účtu

Zálohu účtu (ZIP se všemi daty a přílohami, bez tajných tokenů) stáhne vlastník/administrátor v
*Nastavení → Záloha a přenos*; obnova v přepínači účtů *Nový účet → Obnovit ze zálohy* vždy založí nový účet.
Správce instance může totéž z příkazové řádky (používá stejné `NANOFAKTURA_*` proměnné jako server, třeba
pro přenos ze SQLite na PostgreSQL):

```bash
nanofaktura backup export --account moje-firma --out zaloha.zip
NANOFAKTURA_DB_DRIVER=postgres NANOFAKTURA_DB_DSN="host=…" \
  nanofaktura backup import --owner jan@example.cz [--name "Moje firma"] zaloha.zip
```

Bez argumentů (nebo `nanofaktura serve`) se spustí server. Po obnově jsou pravidelné faktury, upomínky
a webhooky vypnuté a tokeny Fio je třeba zadat znovu.

### Zapomenuté heslo a dvoufázové ověření

Odkaz pro obnovu hesla chodí e-mailem, takže musí být nastavené SMTP a `NANOFAKTURA_PUBLIC_URL`. Uživatelé si
v *Nastavení → Zabezpečení* zapnou druhý faktor: ověřovací aplikaci (TOTP) nebo bezpečnostní klíč / passkey
(WebAuthn, např. YubiKey). Klíče jsou vázané na doménu z `NANOFAKTURA_PUBLIC_URL` a prohlížeč je dovolí jen přes
HTTPS (výjimkou je `localhost`); po změně domény je třeba je přidat znovu. Kdo ztratí druhý faktor i záložní
kódy, tomu ho správce instance vypne:

```bash
nanofaktura user reset-2fa --email jan@example.cz
```

### Migrace z jiných fakturačních aplikací

Data lze do NanoFaktury nahrát přes REST API (API token z *Nastavení → API tokeny*) — kontakty, doklady
s položkami, platbami a přílohami. Před importem historických dokladů vypněte v *Nastavení → E-maily a upomínky*
automatické upomínky i poděkování za platbu, jinak by klientům odešly e-maily ke starým fakturám.

### PostgreSQL DSN

```
host=localhost user=nanofaktura password=tajne dbname=nanofaktura sslmode=disable
```

## Vývoj

```bash
make dev-backend     # backend na :8080
make dev-frontend    # Vite na :5173, proxuje /api → :8080
make test            # go vet + Go testy + TypeScript typecheck + frontend testy (vitest)
make gen-types       # přegeneruje TS typy z OpenAPI
```

Závazná specifikace (datový model, API, konvence) je v [`docs/SPEC.md`](docs/SPEC.md).
Interaktivní dokumentace API běží na `/api/docs`, OpenAPI schéma na `/api/openapi.json`.

## Tech stack

| Vrstva    | Technologie                                                   |
|-----------|---------------------------------------------------------------|
| Backend   | Go 1.26 (≥ 1.26.8), [chi](https://github.com/go-chi/chi), [Huma v2](https://github.com/danielgtaylor/huma), GORM |
| Databáze  | SQLite (pure Go, bez CGO) nebo PostgreSQL                     |
| Frontend  | React 19, TypeScript, Vite, TanStack Router + Query, Tailwind v4, shadcn/ui, PWA |
| Container | Docker (distroless), docker compose, PostgreSQL 17            |
