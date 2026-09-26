# NanoFaktura — specifikace (kontrakt pro implementaci)

Open-source, self-hosted fakturace pro české OSVČ a malé firmy. Funkčně inspirováno
Fakturoidem (https://www.fakturoid.cz/api/v3), API je ale vlastní a čistší.
**Tento dokument je závazný.** Když implementace potřebuje něco, co tu není, drž se
ducha dokumentu a odchylku zapiš do sekce „Otevřené otázky“ na konci.

## 1. Stack a struktura

- Go 1.26, `huma/v2` (OpenAPI 3.1) nad `chi/v5`, GORM.
- DB: SQLite přes **pure-Go** driver `github.com/glebarez/sqlite` (bez CGO) + PostgreSQL (`gorm.io/driver/postgres`).
  Schéma přes GORM AutoMigrate.
- Frontend: React 19 + Vite + TypeScript + Tailwind v4 + shadcn/ui, React Router,
  TanStack Query, `openapi-fetch` s typy generovanými `openapi-typescript` z OpenAPI backendu.
- UI je česky. Kód, identifikátory a JSON pole anglicky. Komentáře v kódu stručně česky nebo anglicky, konzistentně v rámci souboru.

```
cmd/server/          main: config → db → api.New() → http.Server (+ statické soubory SPA)
cmd/gen-schema/      vypíše OpenAPI JSON (pro make gen-types), bez DB
internal/config/     env konfigurace
internal/db/         Open(driver, dsn) + Migrate()
internal/model/      GORM struktury (jen persistence, žádné huma tagy)
internal/billing/    čistá doménová logika: výpočty součtů, číslování, stavy — bez DB/HTTP, 100% unit testy
internal/auth/       hesla (bcrypt), sessions, API tokeny, middleware, context helpery
internal/api/        huma registrace; soubor na zdroj (subjects.go, invoices.go …); DTO in/out typy vedle
internal/ares/       klient ARES
internal/pdf/        generování PDF (Maroto v2), fonty DejaVu v internal/pdf/fonts
internal/spayd/      QR Platba (SPAYD) řetězec + validace čísla účtu (mod 11) + převod na IBAN
web/                 SPA
```

`api.New(db *gorm.DB, cfg config.Config, deps Deps) (http.Handler, huma.API)` — jediný vstupní bod,
používá ho main i testy. `Deps` obsahuje vyměnitelné závislosti (ARES klient, hodiny `Now func() time.Time`).

## 2. Konvence

- **Peníze**: `int64` v nejmenší jednotce (haléře/centy). V API i DB. Nikdy float.
- **Množství**: v API decimální string (`"1.5"`, max 3 desetinná místa, smí být záporné),
  v DB `int64` tisíciny (`quantity_milli`). Parsování/formátování v `internal/billing`.
- **Sazba DPH**: `int32` v basis points (`2100` = 21 %) — pole `vat_rate_bps`.
- **Kurz**: decimální string (`"24.355"`), výchozí `"1"`.
- **Datumy**: string `YYYY-MM-DD` (pole `*_on`). **Časy**: RFC3339 (`*_at`, `time.Time`).
- **ID**: `uint` autoincrement. Všechny doménové záznamy mají `account_id` a každý dotaz je jím filtrován (multi-tenant izolace — test na to je povinný).
- **Huma DTO**: GORM modely se nikdy nepoužívají přímo jako huma input/output. Input DTO pro create
  mají volitelná pole `omitempty`; PATCH DTO mají pointery (nil = neměnit). Output DTO jsou explicitní struktury.
- **JSON odpověď** je přímo tělo (žádné obalování). Seznamy: `{ "items": [...], "page": 1, "per_page": 50, "total": 123 }`,
  query `?page=&per_page=` (max 200).
- **Chyby**: huma RFC 9457 problem+json. 404 pro cizí/neexistující záznam (nikdy 403 u cizího účtu — neprozrazovat existenci),
  409 pro nepovolenou akci ve stavu (zamčená faktura…), 422 validace.
- **Testy**: každý endpoint má test v `internal/api/*_test.go` (package `api_test`) přes `httptest` a sdílený harness
  `internal/api/testutil_test.go` (in-memory SQLite, helper pro registraci+login, `do(method, path, body)`).
  Doménová logika v `internal/billing` má tabulkové unit testy.
- Po změně API: `make gen-types` přegeneruje `web/src/api/schema.gen.ts` (needitovat ručně).

## 3. Auth a účty

Instance je multi-user a multi-account (jako Fakturoid): uživatel může mít přístup k více účtům (firmám/OSVČ).

- `User`: id, email (unique, lowercase), name, password_hash, created_at, updated_at.
- `Account`: id, slug (unique, z názvu, `[a-z0-9-]`), + firemní profil (viz 4.1).
- `Membership`: user_id, account_id, role `owner|member` (unique pár).
- `Session`: id, user_id, token_hash (sha256), expires_at (30 dní, posuvně), created_at. Cookie `nf_session`, HttpOnly, SameSite=Lax, Secure dle configu.
- `APIToken`: id, user_id, name, token_hash, prefix (prvních 8 znaků pro zobrazení), last_used_at, created_at. Token `nf_` + 32 náhodných bajtů base64url; plaintext vrácen jen při vytvoření. Header `Authorization: Bearer nf_…`.

Endpointy:
| Metoda | Cesta | Popis |
|---|---|---|
| GET | `/api/health` | `{status:"ok"}` bez auth |
| GET | `/api/auth/status` | `{signup_allowed: bool, has_users: bool}` bez auth |
| POST | `/api/auth/register` | `{email,name,password,account_name}` → vytvoří user+account(owner)+session. Povoleno, pokud v DB není žádný uživatel, nebo `NANOFAKTURA_ALLOW_SIGNUP=true`. Jinak 403. Heslo min. 8 znaků. |
| POST | `/api/auth/login` | `{email,password}` → set cookie, vrací `Me` |
| POST | `/api/auth/logout` | smaže session |
| GET | `/api/auth/me` | `Me = {user:{id,email,name}, accounts:[{slug,name,role}]}` |
| PATCH | `/api/auth/me` | změna jména / hesla (`current_password` povinné při změně hesla) |
| GET/POST | `/api/auth/tokens` | seznam / vytvoření API tokenu |
| DELETE | `/api/auth/tokens/{id}` | revokace |
| GET/POST | `/api/accounts` | účty uživatele / založení nového účtu (tvůrce = owner) |
| GET/PATCH | `/api/accounts/{slug}` | firemní profil a nastavení (PATCH jen owner) |

Všechny doménové zdroje žijí pod `/api/accounts/{slug}/…`. Middleware ověří membership a vloží account do contextu;
nečlen → 404.

## 4. Doména

### 4.1 Account (firemní profil + nastavení)
`name, registration_no (IČO), vat_no (DIČ), street, city, zip, country (ISO2, default CZ), email, phone, web,
vat_mode: non_vat_payer | vat_payer | identified_person (default non_vat_payer),
registered_by (text „Zapsán v ž. rejstříku…“), default_currency (CZK), default_due_days (14),
default_payment_method (bank), default_language (cs|en), default_note (text nad položkami),
default_footer_note, round_total (bool, default false), default_vat_rate_bps (2100), created_at, updated_at`.

### 4.2 BankAccount
`id, account_id, name, currency, number (české č. ú. „123456789/0800“, volitelné), iban, swift_bic, is_default, created_at…`
Při zadání českého čísla dopočítat IBAN (`internal/spayd`), validovat mod 11 → 422 při chybě. Max jeden `is_default` na měnu.
CRUD: `/api/accounts/{slug}/bank-accounts`.

### 4.3 NumberFormat (číselné řady)
`id, account_id, document_type (invoice|proforma|correction), format, is_default, created_at…`
+ `NumberCounter`: `number_format_id, period (např. "2026" nebo "" pokud formát neobsahuje rok), last_number` (unique pár).
Formát: placeholdery `{YYYY}`, `{YY}`, `{MM}`, `{N}` … `{NNNNNN}` (počet N = zero-pad). Např. `{YYYY}-{NNNN}` → `2026-0001`.
Období čítače = kombinace roku/měsíce obsaženého ve formátu (rok+měsíc pokud obsahuje `{MM}`), řada se tak resetuje sama.
Číslo se přiděluje **při vytvoření** dokladu podle `issued_on`, v transakci (`SELECT … FOR UPDATE` na Postgresu / serializováno na SQLite).
Při založení účtu se vytvoří výchozí řady: invoice `{YYYY}-{NNNN}`, proforma `Z{YYYY}-{NNNN}`, correction `D{YYYY}-{NNNN}`.
Klient smí poslat vlastní `number` (pak se čítač neposouvá). `(account_id, document_type, number)` unique → 409.
CRUD: `/api/accounts/{slug}/number-formats` + `GET …/number-formats/{id}/preview?date=` → `{number}` (bez posunu čítače).

### 4.4 Subject (kontakt)
`id, account_id, type (customer|supplier|both, default customer), custom_id (*string, unique v rámci účtu když ne-NULL),
name (povinné), full_name (kontaktní osoba), registration_no, vat_no, local_vat_no, street, city, zip, country (default CZ),
email, email_copy, phone, web, bank_account, iban, swift_bic, due_days (*int, přebije default účtu),
note (private), created_at, updated_at`.
Endpointy: CRUD `/api/accounts/{slug}/subjects`, list s `?query=` (name/IČO/email, case-insensitive) a `?type=`.
Smazání subjektu s fakturami → 409 (faktury mají snapshot, ale nechceme sirotky ve filtrech).
`GET /api/ares/{ico}` (auth) → `{registration_no, vat_no, name, street, city, zip, country}`; 404 nenalezeno, 502 ARES nedostupný. IČO validovat (8 číslic, kontrolní součet) → 422.

### 4.5 Invoice
`document_type`: `invoice` (faktura), `proforma` (zálohová), `correction` (opravný daňový doklad / dobropis).

Pole (model i output):
```
id, account_id, document_type, number, variable_symbol, status,
subject_id, related_id (*uint: dobropis→opravovaná faktura, finální faktura→proforma),
client_name, client_full_name, client_registration_no, client_vat_no, client_street, client_city, client_zip, client_country, client_email,
your_name, your_registration_no, your_vat_no, your_street, your_city, your_zip, your_country, your_registered_by, your_vat_mode,
issued_on, taxable_fulfillment_due (DUZP; u non_vat_payer smí být prázdné), due_days, due_on,
sent_at, paid_on, cancelled_at, uncollectible_at, locked_at,
currency, exchange_rate, language, payment_method (bank|cash|card|cod|paypal|custom), custom_payment_method,
bank_account_id, bank_account, iban, swift_bic,
order_number, note (text nad položkami), footer_note, private_note, tags ([]string, v DB JSON),
prices_include_vat (bool), round_total (bool), reverse_charge (bool, přenesená daňová povinnost),
lines[], payments[],
subtotal (základ), vat_total, rounding, total, paid_amount, remaining_amount,
vat_recap: [{vat_rate_bps, base, vat, total}],
created_at, updated_at
```
`InvoiceLine`: `id, invoice_id, position, name, quantity (string), unit_name, unit_price, vat_rate_bps` + vypočtené `base, vat, total`.

**Vytvoření (`POST /api/accounts/{slug}/invoices`)**: povinné `subject_id` a `lines` (≥1). Defaulty:
`document_type=invoice`, `issued_on=dnes`, `taxable_fulfillment_due=issued_on` (jen plátce), `due_days` = subject.due_days → account.default_due_days,
`due_on=issued_on+due_days`, měna/jazyk/platba/round_total/note/footer z účtu, `bank_account_id` = výchozí pro měnu.
Snapshot `client_*` ze subjektu, `your_*` z účtu, `bank_account/iban/swift_bic` z bankovního účtu — vše při vytvoření;
klient smí jednotlivá snapshot pole přepsat. `variable_symbol` default = číslice z čísla dokladu (max 10, zprava).
Pro `non_vat_payer` se všechny `vat_rate_bps` vynutí na 0. `correction` vyžaduje `related_id` na fakturu téhož účtu.

**Výpočet (internal/billing, čistá funkce)**:
- `prices_include_vat=false`: řádek `base = round_half_away(unit_price × qty)`; rekapitulace per sazba:
  `base = Σ base`, `vat = round(base × rate / 10000)`, `total = base + vat`. Řádkové `vat/total` informativně stejným vzorcem.
- `prices_include_vat=true`: řádek `total = round(unit_price × qty)`; per sazba `total = Σ`, `vat = round(total × rate / (10000 + rate))`, `base = total − vat`.
- `reverse_charge=true`: DPH se nepočítá (vat=0), sazby zůstávají pro zobrazení.
- `subtotal = Σ base`, `vat_total = Σ vat`, `total_before = subtotal + vat_total`;
  `round_total` → `total` zaokrouhlen na celé jednotky (100) half-away, `rounding = total − total_before`.
- `paid_amount = Σ payments.amount`, `remaining_amount = total − paid_amount`.
- Součty se ukládají do DB (kvůli filtrům/statistikám) a přepočítávají při každé změně řádků/plateb.

**Stavy**: uloženo `open | sent | paid | cancelled | uncollectible`; `overdue` se odvozuje při čtení
(`open|sent` a `due_on < dnes`) a jde podle něj filtrovat. Proforma/correction mají stejné stavy.
Dobropis se zápornou částkou: „zaplaceno“ = vráceno.

**Úpravy (`PATCH`)**: zakázané (409), pokud `locked_at != nil` nebo `status ∈ {cancelled, uncollectible}`.
`lines` v PATCH = úplná náhrada seznamu (řádky s `id` se aktualizují, bez `id` vloží, chybějící smažou).
Změna `issued_on`/`due_days` přepočítá `due_on`. Změna `subject_id` znovu nasnapshotuje `client_*` (pokud nejsou poslána explicitně).
**Smazání (`DELETE`)**: zakázané pokud zamčená nebo má platby → 409.

**Akce `POST /api/accounts/{slug}/invoices/{id}/actions/{action}`** (vrací aktualizovanou fakturu):
| akce | z | do |
|---|---|---|
| `mark_as_sent` | open | sent (`sent_at=now`) |
| `cancel` | open, sent (bez plateb) | cancelled |
| `undo_cancel` | cancelled | open/sent (podle `sent_at`) |
| `mark_as_uncollectible` | open, sent | uncollectible |
| `undo_uncollectible` | uncollectible | open/sent |
| `lock` / `unlock` | jakýkoli | nastaví/smaže `locked_at` |
Nepovolený přechod → 409 s lidsky čitelnou zprávou.

**Platby** `POST /api/accounts/{slug}/invoices/{id}/payments` `{paid_on (default dnes), amount (default remaining), note, create_final_invoice (bool, jen proforma)}`,
`DELETE …/payments/{payment_id}`. Model `Payment`: `id, account_id, invoice_id, paid_on, amount, note, created_at`.
Po změně: `paid_amount ≥ total` (u záporných totalů `≤`) → `status=paid`, `paid_on` = datum poslední platby; jinak návrat na `sent`/`open` a `paid_on=""`.
Platba na cancelled/uncollectible → 409.
`create_final_invoice` u proformy vytvoří `invoice` se stejnými řádky, `related_id=proforma.id`, rovnou zaplacenou (platba se stejným datem a částkou);
id nové faktury vrátit v odpovědi (`final_invoice_id`).

**Dobropis**: `POST …/invoices/{id}/correction` → vytvoří `correction` k faktuře s řádky zkopírovanými a zápornými množstvími
(klient ho pak upraví přes PATCH). Vrací novou fakturu.

**Duplikace**: `POST …/invoices/{id}/duplicate` → nová faktura (open, nové číslo, dnešní datum, stejné řádky a subjekt).

**Seznam** `GET …/invoices`: filtry `status` (vč. `overdue`), `document_type`, `subject_id`, `since`/`until` (issued_on),
`query` (číslo, client_name, VS), `sort` (`-issued_on` default, `issued_on`, `-number`, `due_on`, `-total`). Položky seznamu bez `lines`/`payments`.
Detail vrací vše.

**PDF** `GET …/invoices/{id}/pdf` → `application/pdf`, `Content-Disposition: inline; filename="faktura-<number>.pdf"`.
Jazyk dle `language`. Obsah: hlavička s typem dokladu a číslem, dodavatel/odběratel (IČO, DIČ, adresa, „Neplátce DPH“ / zápis v rejstříku),
data (vystavení, DUZP pro plátce, splatnost), platební údaje (účet, IBAN, SWIFT, VS, způsob platby), položky, rekapitulace DPH (plátce),
celkem k úhradě, poznámky, QR Platba (SPAYD) pokud měna CZK a IBAN je CZ a `remaining_amount > 0`.
U `correction` odkaz na původní doklad; u proformy text „Nejedná se o daňový doklad“.

### 4.6 Statistiky
`GET /api/accounts/{slug}/dashboard?year=2026` →
`{ year, currency, revenue_by_month: [12× int64 (součet total vydaných faktur typu invoice+correction dle issued_on, ne cancelled)],
unpaid_total, unpaid_count, overdue_total, overdue_count, revenue_total }` (jen doklady ve výchozí měně účtu).

## 5. Konfigurace (env)
`NANOFAKTURA_LISTEN_ADDR` (:8080), `NANOFAKTURA_DB_DRIVER` (sqlite|postgres, default sqlite), `NANOFAKTURA_DB_DSN`
(default `nanofaktura.db`), `NANOFAKTURA_STATIC_DIR` (volitelné, servíruje SPA s fallbackem na index.html),
`NANOFAKTURA_ALLOW_SIGNUP` (false), `NANOFAKTURA_SECURE_COOKIES` (false), `NANOFAKTURA_ARES_URL` (pro testy).

## 6. Frontend
Routy: `/login`, `/register`, `/` → redirect na `/a/{slug}` (první účet), pod `/a/:slug/`:
`dashboard`, `invoices`, `invoices/new`, `invoices/:id`, `invoices/:id/edit`, `subjects`, `subjects/new`, `subjects/:id`,
`settings` (záložky: Firma, Bankovní účty, Číselné řady, Můj profil, API tokeny). Přepínač účtů v sidebaru.
Formulář faktury: výběr subjektu s hledáním + „Nový kontakt“ (s ARES), editovatelné řádky, živý přepočet
(zobrazovací duplikát logiky; zdrojem pravdy je backend), klávesová efektivita. Detail faktury: náhled údajů, stav, akce,
platby, PDF (otevřít/stáhnout), dobropis, duplikovat. Peníze formátovat `Intl.NumberFormat('cs-CZ', {style:'currency'})`.

## 7. Mimo první milník
Náklady (expenses), sklad, šablony/pravidelné faktury, odesílání e-mailem, webhooky, události, úkoly, ISDOC, EET, importy.

## Otevřené otázky
(sem zapisují implementátoři odchylky a nejasnosti)
