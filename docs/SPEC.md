# NanoFaktura — specifikace (kontrakt pro implementaci)

Open-source, self-hosted fakturace pro české OSVČ a malé firmy.
**Tento dokument je závazný.** Když implementace potřebuje něco, co tu není, drž se
ducha dokumentu a odchylku zapiš do sekce „Otevřené otázky“ na konci.

## 1. Stack a struktura

- Go 1.26, `huma/v2` (OpenAPI 3.1) nad `chi/v5`, GORM.
- DB: SQLite přes **pure-Go** driver `github.com/glebarez/sqlite` (bez CGO) + PostgreSQL (`gorm.io/driver/postgres`).
  Schéma přes GORM AutoMigrate.
- Frontend: čistá SPA (bez SSR) — React 19 + Vite + TypeScript (strict) + Tailwind v4 + shadcn/ui (CLI v4, Base UI primitives),
  **TanStack Router** (file-based, typované params i search params — filtry seznamů žijí v URL), TanStack Query,
  React Hook Form + Zod, `openapi-fetch` s typy generovanými `openapi-typescript` z OpenAPI backendu. PWA (`vite-plugin-pwa`, instalovatelné, bez offline zápisu).
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

Instance je multi-user a multi-account: uživatel může mít přístup k více účtům (firmám/OSVČ).

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
Routy (TanStack Router, `web/src/routes/`): `/login`, `/register`, `/` → redirect na `/a/{slug}` (první účet), pod `/a/$slug/`:
`dashboard`, `invoices`, `invoices/new`, `invoices/:id`, `invoices/:id/edit`, `subjects`, `subjects/new`, `subjects/:id`,
`settings` (záložky: Firma, Bankovní účty, Číselné řady, Můj profil, API tokeny). Přepínač účtů v sidebaru.
Formulář faktury: výběr subjektu s hledáním + „Nový kontakt“ (s ARES), editovatelné řádky, živý přepočet
(zobrazovací duplikát logiky; zdrojem pravdy je backend), klávesová efektivita. Detail faktury: náhled údajů, stav, akce,
platby, PDF (otevřít/stáhnout), dobropis, duplikovat. Peníze formátovat `Intl.NumberFormat('cs-CZ', {style:'currency'})`.

**Mobile-first (povinné od první obrazovky, ne dodatečně):**
- Návrh začíná na 360 px šířky, desktop je rozšíření (`md:`/`lg:` breakpointy). Žádný horizontální scroll stránky.
- Navigace: < `md` spodní tab bar (Přehled, Faktury, **+ Nová**, Kontakty, Více) s `env(safe-area-inset-bottom)`;
  ≥ `md` shadcn Sidebar (sbalitelný na ikony). Přepínač účtů v „Více“ / v hlavičce sidebaru.
- Seznamy: na mobilu karty (číslo, klient, částka, stav, splatnost), na desktopu tabulka. Filtry na mobilu v Drawer (bottom sheet).
  Pull-to-refresh není nutný; nekonečné načítání nebo stránkování tlačítkem „Načíst další“.
- Formuláře: jeden sloupec na mobilu, touch targety ≥ 44 px, `inputmode="decimal"`/`numeric` pro částky, množství a IČO,
  `type="email"`/`tel`, nativní date input. Řádky faktury na mobilu jako skládací karty, ne tabulka.
  Primární akce ve sticky spodní liště (Uložit / Vystavit), respektuje klávesnici a safe area.
- Detail faktury: akce v Drawer menu na mobilu, v toolbaru na desktopu. PDF na mobilu otevírat v nové záložce (ne iframe).
- Dialogy: na mobilu Drawer, na desktopu Dialog (vzor „responsive dialog“).
- `viewport-fit=cover`, `theme-color`, podpora světlého/tmavého režimu (prefers-color-scheme + přepínač).
- Ověřovat v Chrome DevTools / Playwright na 390×844 i 1440×900.

## 7. Milník 2 — kompletní nástroj

Stejné konvence jako výše (account scope, int64 peníze, testy na každý endpoint, mobile-first UI).
Všechny nové zdroje pod `/api/accounts/{slug}/…`, pokud není řečeno jinak.

### 7.1 Ceník (price items)
`PriceItem`: `id, account_id, name, sku, unit_name, unit_price, vat_rate_bps, prices_include_vat, currency, track_stock (bool),
stock_quantity (string, jen při track_stock), min_stock (string), archived_at, note`. CRUD `/price-items`, `?query=`, `?archived=`.
`InvoiceLine.price_item_id` (*uint) — ve formuláři faktury našeptávač z ceníku (název/SKU).
**Sklad**: pokud položka `track_stock`, vystavení faktury (create; ne proforma) odepíše množství, zrušení/smazání vrátí.
`StockMove`: `id, account_id, price_item_id, direction (in|out), quantity, moved_on, note, invoice_id?, expense_id?`.
`GET/POST /price-items/{id}/stock-moves`, ruční příjem/výdej. Nízký stav → úkol (7.9).

### 7.2 Náklady (expenses)
`Expense`: `id, account_id, number (interní, vlastní řada document_type=expense `N{YYYY}-{NNNN}`), original_number (číslo dokladu dodavatele),
variable_symbol, subject_id (dodavatel), supplier_* snapshot, issued_on, taxable_fulfillment_due, due_on, paid_on, status (open|paid|overdue odvozené),
currency, exchange_rate, payment_method, category (text, našeptávání z existujících), description, private_note, tags,
tax_deductible (bool, default true), prices_include_vat, lines (stejná struktura a výpočet jako InvoiceLine, sdílený kód v billing),
totals jako Invoice, attachments[], created_at…`.
CRUD `/expenses` + filtry (status, category, subject_id, since/until, query), platby `POST/DELETE /expenses/{id}/payments`,
akce `lock/unlock`. Přílohy viz 7.12. Dashboard doplnit o `expenses_by_month`, `profit_total`.

### 7.3 Pravidelné faktury (recurring) a šablony
`InvoiceTemplate` (`Generator`): všechna „obsahová“ pole faktury (subject, lines, note, měna, platba, due_days, tags…) + `name`.
`Recurring`: šablona + `start_on, next_occurrence_on, end_on?, months_period (1=měsíčně, 3, 12…), day_of_month?,
issue_as (invoice|proforma), send_email (bool), active (bool), last_invoice_id`.
CRUD `/templates`, `/recurring`, `POST /templates/{id}/create-invoice`, `POST /recurring/{id}/run-now`.
Plánovač: goroutine v serveru, 1× za hodinu (+ při startu) vytvoří faktury pro `next_occurrence_on <= dnes` (idempotentně, v transakci),
posune `next_occurrence_on`. Placeholdery v textech řádků: `{MONTH}`, `{MONTH_NAME}`, `{YEAR}`, `{PREV_MONTH_NAME}` apod. (podle data vystavení, česky/anglicky dle jazyka).
Scheduler má interface s `Now()` → testovatelný.

### 7.4 E-maily a upomínky
SMTP konfigurace per instance (env `NANOFAKTURA_SMTP_*`, `NANOFAKTURA_MAIL_FROM`) + per account override (`smtp_*`, reply-to, podpis).
`POST /invoices/{id}/send` `{to[], cc[], subject?, body?, attach_pdf=true, attach_isdoc=false, kind: invoice|reminder|paid_thanks}` →
odešle, zapíše `EmailLog` (`id, account_id, invoice_id, kind, to, subject, sent_at, error`), při kind=invoice provede `mark_as_sent`.
Šablony textů v nastavení účtu (`email_templates`: invoice/reminder/paid_thanks × cs/en) s placeholdery `{number} {total} {due_on} {public_url} {account_name}…`.
**Automatické upomínky**: nastavení účtu `reminders_enabled`, `reminder_days_after_due: [3, 14, 30]` — plánovač posílá.
`GET /invoices/{id}/emails` historie. Mailer je interface (`Mailer.Send`), v testech fake; v dev režimu bez SMTP loguje do stdout.

### 7.5 Veřejný odkaz pro klienta
Každá faktura má `public_token` (náhodný, 24+ znaků). `GET /api/public/invoices/{token}` (bez auth) → omezený výpis (údaje z PDF),
`GET /api/public/invoices/{token}/pdf`, `…/isdoc`. Frontend `/p/$token` — mobile-first stránka s fakturou, QR platbou, tlačítkem PDF.
Zaznamenat první zobrazení (`public_viewed_at`) → událost. Token lze přegenerovat (`POST /invoices/{id}/regenerate-public-token`).

### 7.6 Banka — import a párování plateb
`BankAccount` doplnit o `sync_provider (none|fio)`, `fio_token` (šifrovaně, AES-GCM klíčem z `NANOFAKTURA_SECRET_KEY`), `last_synced_at`.
`BankTransaction`: `id, account_id, bank_account_id, external_id (unique per bank account), booked_on, amount (±), currency, counterparty_account,
counterparty_name, variable_symbol, constant_symbol, specific_symbol, message, matched_invoice_id?, matched_expense_id?, payment_id?, ignored (bool)`.
Zdroje: **Fio API** (`/sync` ruční + plánovač každé 2 h), **import CSV** (Fio, ČSOB, KB, Air Bank, generický s mapováním sloupců) a **ABO/GPC** formát.
**Párování**: příchozí platba → neuhrazená faktura se stejným VS a přesnou částkou (remaining) → automaticky vytvoří Payment;
jinak návrhy (VS sedí/částka sedí/jméno) k ručnímu potvrzení. Odchozí → náklady obdobně.
Endpointy `/bank-accounts/{id}/sync`, `/bank-accounts/{id}/import` (multipart), `/bank-transactions` (filtry: unmatched…),
`POST /bank-transactions/{id}/match {invoice_id|expense_id}`, `/unmatch`, `/ignore`. Fio klient za interfacem (testy s fake serverem).

### 7.7 Měny a kurzy ČNB
`GET /api/exchange-rates?date=&currency=` → kurz ČNB (denní kurzovní lístek, cache v DB `ExchangeRate(date, currency, rate, amount)`).
Při vytvoření dokladu v cizí měně a nezadaném kurzu se použije kurz ČNB ke dni DUZP (resp. vystavení). Klient za interfacem.
PDF u plátce v cizí měně zobrazí rekapitulaci DPH i v CZK.

### 7.8 Ověřování subjektů
- ARES (4.4) + **registr plátců DPH** (MFČR, služba „nespolehlivý plátce“ / zveřejněné účty): `GET /api/vat-registry/{dic}` →
  `{reliable: bool|null, published_accounts: [...]}`. Při vytváření nákladu upozornit, pokud je dodavatel nespolehlivý nebo účet není zveřejněný.
- **VIES** pro EU DIČ: `GET /api/vies/{vat_no}` → `{valid, name, address}`.
- Oba klienti za interfacem, výsledek cache 24 h.

### 7.9 Události (activity log) a úkoly
`Event`: `id, account_id, user_id?, name (invoice.created, invoice.sent, invoice.paid, invoice.overdue, payment.created, expense.created,
recurring.generated, email.sent, public.viewed, bank.matched, stock.low …), subject_type, subject_id, text, data (JSON), created_at`.
Zapisuje doménová vrstva (jedna funkce `events.Record`). `GET /events` (filtry typu/subjektu, stránkování), na detailu faktury/kontaktu timeline.
`Todo`: `id, account_id, name, text, related (type,id), due_on?, completed_at?`, auto-generované (faktura po splatnosti, nespárovaná platba,
nízký sklad, nepotvrzený návrh párování) + ruční. `GET/POST/PATCH /todos`, `POST /todos/{id}/toggle`.

### 7.10 Webhooky
`Webhook`: `id, account_id, url, events ([]string, "*"), secret, active, last_status, last_delivered_at`.
Doručení asynchronně (fronta v DB `WebhookDelivery` s retry 1m/5m/30m/2h), podpis `X-NanoFaktura-Signature: sha256=HMAC(secret, body)`.
CRUD `/webhooks`, `POST /webhooks/{id}/test`, `GET /webhooks/{id}/deliveries`.

### 7.11 Exporty a reporty
- **ISDOC 6.0.2** XML pro faktury (`GET /invoices/{id}/isdoc`), volitelně přílohou e-mailu; hromadně ZIP.
- **CSV/XLSX export** seznamů faktur, nákladů, kontaktů (se stejnými filtry jako seznam) `GET /exports/{kind}.csv|.xlsx`.
- **Hromadný ZIP PDF** za období (pro účetní) `GET /exports/pdf.zip?since=&until=`.
- **DPH (jen plátci)**: `GET /reports/vat?period=2026-09|2026-Q3` → podklad přiznání (řádky 1–2, 40–41, 46…) a **kontrolní hlášení** (A.4/A.5/B.2/B.3)
  + export XML ve formátu EPO (DPHDP3, DPHKH1). Periodicita v nastavení účtu (`vat_period: month|quarter`).
- **Přehledy**: tržby/náklady/zisk po měsících, top odběratelé, průměrná doba úhrady, přehled pro daňové přiznání OSVČ (příjmy, výdaje, paušál 60/40/80/30 %).

### 7.12 Přílohy a úložiště
`Attachment`: `id, account_id, owner_type (invoice|expense|subject), owner_id, filename, content_type, size, storage_key, created_at`.
Úložiště přes interface (lokální disk `NANOFAKTURA_DATA_DIR/attachments`, volitelně S3-kompatibilní). Limit 20 MB, povolené typy pdf/obrázky/xml.
`POST /attachments` (multipart), `GET /attachments/{id}` (download), `DELETE`. Upload na mobilu umožní fotit (`accept="image/*,application/pdf" capture`).
Logo účtu a podpis/razítko (obrázek) do PDF — přes stejné úložiště, pole `logo_attachment_id`, `stamp_attachment_id` na Account.

### 7.13 Uživatelé účtu a role
Pozvánky: `POST /members/invite {email, role}` → e-mail s odkazem (`/invite/$token`), registrace/přihlášení a přijetí.
Role: `owner` (vše), `admin` (vše kromě mazání účtu a správy ownerů), `accountant` (čtení vše + exporty + reporty, bez úprav dokladů),
`member` (doklady, kontakty, náklady; bez nastavení). `GET/PATCH/DELETE /members`. Kontrola rolí centrálně (deklarativně při registraci operace).

### 7.14 Vzhled a jazyk dokladů
Nastavení účtu: PDF šablona (`classic|modern|minimal`), barva akcentu, logo, razítko, zobrazení QR, jazyk (cs/en/sk/de), vlastní patička.
Náhled PDF v nastavení (`GET /accounts/{slug}/pdf-preview?template=` s ukázkovými daty).

### 7.15 UX navíc
- Globální hledání + command palette (`⌘K`/tlačítko na mobilu): faktury, kontakty, náklady, akce („Nová faktura pro …“). `GET /search?q=`.
- Klávesové zkratky na desktopu (N = nová faktura, / = hledat).
- Onboarding po registraci: průvodce (IČO → ARES předvyplní firmu, bankovní účet, plátcovství DPH, logo) + ukázková první faktura.
- Dashboard: tržby vs. náklady graf, neuhrazené/po splatnosti, cashflow očekávaných příjmů (dle due_on), úkoly, poslední aktivita.
- Kopírování údajů klientovi jedním klepnutím, sdílení veřejného odkazu přes Web Share API na mobilu.

### 7.16 Mimo rozsah
EET (zrušeno), účetnictví (podvojné), mzdy, OCR účtenek (jen příprava: přílohy nákladu).

## Otevřené otázky
(sem zapisují implementátoři odchylky a nejasnosti)

**Backend základ (auth, účty) — rozhodnutí a odchylky:**
- Seznamy tokenů (`GET /api/auth/tokens`) a účtů (`GET /api/accounts`) používají také stránkovanou obálku `{items,page,per_page,total}` (jednotnost; `Me.accounts` zůstává plné pole).
- Výstup `Account` nemá `id` (identifikátor je `slug`) a obsahuje `role` aktuálního uživatele. `POST /api/accounts` přijímá jen `{name}`, profil se doplní přes PATCH. PATCH účtu členem (ne ownerem) → 403 (člen existenci účtu zná). Slug: max 50 znaků, prázdný → `ucet`, kolize → `-2`, `-3`…
- Statusy: register 201, login 200, špatné přihlašovací údaje 401 (`invalid email or password`), bez přihlášení 401, logout 204 a nevyžaduje přihlášení (idempotentní; maže session z cookie). Změna hesla bez/špatné `current_password` → 422 (`body.current_password`). Změna hesla zatím nezneplatňuje ostatní sessions.
- Session: expirace se posouvá nejvýš jednou za 24 h (při posunu server pošle obnovenou cookie). Bearer token má přednost před cookie.
- Texty chyb (`detail`) jsou anglicky, stejně jako validační chyby z huma; frontend si je případně přeloží.
- OpenAPI na `/api/openapi.json`, dokumentace `/api/docs` (pod `/api`, aby nekolidovaly se SPA). Pole `$schema` se do odpovědí nepřidává.
- SQLite běží s jedním připojením (`MaxOpenConns=1`) → zápisy (i přidělování čísel) jsou serializované; uvnitř transakce se smí používat jen `tx`.
- `NumberCounter` a `InvoiceLine` nemají `account_id` (patří pod `NumberFormat` resp. `Invoice`, které ho mají). Výchozí řady se zakládají s `is_default=true`.
- `go.mod` obsahuje `ignore ./web` (v `web/node_modules` je Go balíček, který by jinak spadl do `./...`).

**Kontakty, bankovní účty, číselné řady, ARES — rozhodnutí a odchylky:**
- Číselné řady: `document_type` navíc `expense` (milník 2); nový účet dostane i výchozí řadu `N{YYYY}-{NNNN}`. Konstanta `model.DocExpense` je v `model/account.go`.
- Formát: max 50 znaků, `{N}` smí být víckrát (všechny dostanou stejné číslo), jiné `{…}` → 422 (`body.format`). Období: `{MM}` → `YYYY-MM` (i bez roku ve formátu), jen rok → `YYYY`, jinak `""`. Číslo širší než padding se nezkracuje.
- `numbering.Next(tx, accountID, docType, issuedOn)` bere výchozí řadu typu; bez výchozí řady `numbering.ErrNoFormat`, špatné datum `numbering.ErrInvalidDate` (volající mapuje na 422/409). Čítač se zakládá přes `INSERT … ON CONFLICT DO NOTHING` a pak zamyká `SELECT … FOR UPDATE`.
- Řady: stejný `format` dvakrát u téhož typu → 409 (vydávaly by duplicitní čísla). První řada typu je vždy výchozí. `PATCH is_default=false` u výchozí řady → 409 (výchozí se mění nastavením jiné řady). Smazání výchozí nebo poslední řady typu → 409; smazání maže i její čítače. `document_type` se PATCHem měnit nedá. List má filtr `?document_type=`.
- Preview: `?date=` volitelné (default dnes), špatné datum → 422.
- Zápisy (POST/PATCH/DELETE) bankovních účtů a číselných řad smí jen owner (403 pro membera), čtení kdokoli z účtu — jde o nastavení, stejně jako PATCH účtu. Kontakty smí spravovat i member.
- Bankovní účet: `currency` volitelná (default `default_currency` účtu); povinné je `number` nebo `iban` (jinak 422 `body.number`). Číslo účtu se ukládá bez mezer; IBAN normalizovaný (velká písmena, bez mezer), SWIFT velkými písmeny a validovaný (8/11 znaků). Zadaný IBAN, který nesedí k číslu účtu → 422 `body.iban`. Změna `number` v PATCH přepočítá IBAN/SWIFT, pokud nejsou poslány zároveň. `is_default=false` je povoleno (max jeden výchozí na měnu, ne právě jeden); první účet v měně je výchozí automaticky, po smazání výchozího se výchozím stane nejstarší zbývající účet téže měny. Smazání účtu použitého na fakturách je povoleno (faktury mají snapshot). List má filtr `?currency=`.
- Kontakty: `?type=customer|supplier` vrací i `both`, `?type=both` jen `both`. `query` hledá v name, registration_no a email (LIKE s escapovanými `%`/`_`), řazení podle `LOWER(name)`. IČO se normalizuje (mezery pryč, 7 číslic doplněno nulou) a kontroluje mod 11; `vat_no` velkými bez mezer; `country` se převádí na velká písmena a musí být 2 písmena; IBAN/SWIFT kontaktu se validují, `bank_account` kontaktu ne (může být zahraniční). Duplicitní `custom_id` → 409 s textem `custom_id is already used…`; prázdný `custom_id` = NULL (PATCH s `""` ho smaže). `due_days` nelze PATCHem vrátit na NULL (jen nastavit číslo) — případně doplnit později.
- ARES: `GET /api/ares/{ico}` → výstup `AresSubject`; `zip` jako 5 číslic bez mezery, `city` = městská část/obvod, pokud rozšiřuje název obce („Praha 4“, „Brno-střed“). Obec bez ulic: místo ulice část obce (nebo obec) + číslo; číslo evidenční jako „č. ev. N“. Bez strukturované adresy se parsuje `textovaAdresa`. ARES 400 i 404 → 404; jiná chyba / nedostupnost → 502 (bez detailu upstream chyby). `NANOFAKTURA_ARES_URL` je base URL včetně `/ekonomicke-subjekty` (IČO se připojí jako další segment cesty).

**Faktury, platby, dashboard — rozhodnutí a odchylky:**
- Výstup `status` je efektivní (může být `overdue`); uložený open/sent je poznat podle `sent_at`. Filtr `status=open|sent` vrací jen doklady, které nejsou po splatnosti.
- Seznam vrací `InvoiceSummary` (bez `lines`, `payments`, `vat_recap`); detail `Invoice` = summary + `lines`, `payments`, `vat_recap`. `vat_recap` se neukládá, počítá se při čtení z řádků; řazení od nejvyšší sazby. Volitelná pole výstupu (`related_id`, `bank_account_id`, `sent_at`, `cancelled_at`, `uncollectible_at`, `locked_at`, `price_item_id`) se při null vynechávají.
- Nová pole už teď: `public_token` (32 znaků base64url, unique, vzniká při vytvoření; `POST …/regenerate-public-token` → 200 + faktura, v jakémkoli stavu) a `InvoiceLine.price_item_id` (volitelné, zatím bez validace).
- Odkazy v těle (`subject_id`, `bank_account_id`, `related_id`) na neexistující/cizí záznam → 422 (ne 404). `related_id` je u ne-dobropisů volitelné, ale musí existovat v účtu; dobropis musí odkazovat na doklad typu `invoice`; odkaz sám na sebe → 422.
- DUZP default = `issued_on`, pokud `your_vat_mode != non_vat_payer` (tj. i identifikovaná osoba). Vynucení sazeb na 0 i výpočet se řídí snapshotem `your_vat_mode` faktury, ne aktuálním nastavením účtu (PATCH staré faktury tak počítá stejně). Klient smí přepsat i `your_*` a `bank_account/iban/swift_bic`.
- Bankovní účet: výchozí pro měnu (`is_default DESC, id`, tj. fallback na jiný účet téže měny); bez účtu v měně zůstanou bankovní pole prázdná.
- Řádky: `quantity` default `"1"`, `vat_rate_bps` default `default_vat_rate_bps` účtu (0–10000), `|unit_price| ≤ 10^14`, max 500 řádků; přetečení int64 ve výpočtu → 422 `body.lines`. Řádek v PATCH je úplný objekt (vynechaná volitelná pole = defaulty, ne původní hodnoty), `id` jen určuje, který řádek se aktualizuje; cizí nebo opakované `id` → 422. `position` = pořadí v poli.
- `variable_symbol` default = `spayd.Digits(number, 10)`; změna `number` PATCHem VS nepřepočítá. Bez výchozí číselné řady → 409 (vlastní `number` řadu nepotřebuje). Přidělení čísla je vyměnitelné přes `api.Deps.NextNumber` (default `numbering.Next`).
- PATCH: `document_type` měnit nelze. Změna `currency` bez `bank_account_id` znovu vybere výchozí bankovní účet nové měny. Změna `issued_on` nepřepočítává DUZP. Po PATCH se přepočítá i stav z plateb (zaplacená faktura s vyšší částkou → zpět `open`/`sent`). `tags` nahrazují seznam (trim, bez duplicit).
- Akce vrací 200 + fakturu, neznámá akce → 422 (enum v cestě). Zámek blokuje jen PATCH/DELETE (akce a platby na zamčené faktuře jsou povolené).
- Platby: `POST` → 201 `{payment, invoice, final_invoice_id?}`, `DELETE` → 204. `amount: 0` → 422; bez `amount` při nulovém zůstatku → 409. `paid` vyžaduje aspoň jednu platbu (faktura s total 0 bez plateb zůstává open). U `uncollectible` lze platby mazat, stav se nemění.
- `create_final_invoice`: jen proforma (jinak 422) a jen jednou (už existuje `invoice` s `related_id` = proforma → 409, vrátí se celá transakce vč. platby). Finální faktura: `issued_on` (a DUZP u plátce) = datum platby, `client_*` z proformy, `your_*` z aktuálního účtu, řádky/měna/poznámky z proformy.
- Dobropis (`/correction`) jen k dokladu typu `invoice` (jinak 409), kopíruje `client_*` i `your_*` originálu, datum dnes. Duplikace zachová typ dokladu (dobropis i s `related_id`), snapshoty bere znovu ze subjektu a účtu.
- Smazat lze i zrušenou fakturu bez plateb. Smazání dokladu, na který ukazuje cizí `related_id`, zatím blokované není (otevřené).
- Dashboard: `revenue_by_month` obsahuje i `uncollectible` (vyřazen je jen `cancelled`) a dobropisy záporně. `unpaid_*`/`overdue_*` = doklady open/sent všech typů (vč. proforem) ve výchozí měně bez ohledu na `year`, částka `total − paid_amount`. `year` 2000–2999, default aktuální rok.
- **[frontend] Předpokládané tvary auth/accounts odpovědí** (dočasné ručně psané `web/src/api/schema.gen.ts`,
  frontend na ně odkazuje jen přes `paths` v `web/src/api/types.ts`):
  `GET /api/auth/tokens` → `{items: [{id, name, prefix, last_used_at|null, created_at}]}`;
  `POST /api/auth/tokens` `{name}` → `{id, name, prefix, token, created_at}`; `DELETE /api/auth/tokens/{id}` → 204;
  `GET /api/accounts` → `{items: [{slug, name, role}]}`; `POST /api/accounts` `{name}` → celý `Account`;
  `GET/PATCH /api/accounts/{slug}` → `Account` = `slug` + všechna pole §4.1 (PATCH = všechna pole volitelná);
  `POST /api/auth/login|register` a `PATCH /api/auth/me` → `Me`; `POST /api/auth/logout` → 204;
  `PATCH /api/auth/me` `{name?, current_password?, new_password?}` (název pole nového hesla `new_password`).
  Špatné heslo při loginu → 401; špatné `current_password` → 401/403/422 (frontend zvládne všechny).
  Validační chyby 422 s `errors[].location = "body.<pole>"` frontend mapuje přímo na pole formuláře.
- **[frontend] Záložky nastavení jsou vnořené routy** `/a/$slug/settings/{company,bank-accounts,number-formats,profile,tokens}`;
  `/a/$slug/settings` je na mobilu seznam sekcí, na desktopu přesměruje na `company`.
- **[frontend] Vite dev proxy** předává `/api/*` beze změny cesty (backend servíruje API pod `/api`).

**PDF (internal/pdf, `GET …/invoices/{id}/pdf`) — rozhodnutí a odchylky:**
- API balíčku: `pdf.Render(inv *model.Invoice, acc *model.Account, opt pdf.Options) ([]byte, error)`; `Options{Template, Accent, Language, Logo, Stamp, ShowQR, RelatedNumber}`. Šablona/akcent/logo/razítko zatím nejsou na `Account` (§7.14, §7.12) — přicházejí přes `Options`; endpoint zatím bere jen `?template=classic|modern|minimal&lang=cs|en|sk|de` (neplatná hodnota → 422) a posílá `ShowQR=true`. Po doplnění polí na Account je stačí předat v `getInvoicePDF`.
- `pdf.Sample(pdf.SampleSpec)` vrací konzistentní ukázková data (pro budoucí `GET /pdf-preview` z §7.14); `go run ./cmd/pdf-sample [-png]` vygeneruje všechny šablony × typy dokladů do `tmp/pdf-samples/` (gitignored).
- Titulek neplátce je jen „Faktura“ (resp. „Opravná faktura“ u dobropisu neplátce); „Neplátce DPH“ je v bloku dodavatele. `identified_person` se zobrazuje jako neplátce (bez sloupců DPH) s poznámkou „Identifikovaná osoba k DPH“. Finální faktura k proformě (`related_id`) má podtitulek „Vyúčtování zálohové faktury č. …“.
- QR Platba jen při `payment_method=bank`, měně CZK, CZ IBAN, zbývající částce > 0 a stavu ≠ cancelled/uncollectible; částka v QR = zbývající částka. SWIFT se do SPAYD záměrně neposílá (některé bankovní aplikace pak platbu berou jako zahraniční).
- Rekapitulace DPH se v PDF počítá z řádků přes `billing.Calculate`; součty (`subtotal/vat_total/rounding/total/paid_amount`) se berou z uložené faktury.
- Stav `paid` → razítko „ZAPLACENO“ s datem `paid_on`, `cancelled` → „STORNO“. Plně zaplacená faktura ukáže „Celkem“ + řádek „Zaplaceno“, částečně zaplacená „Zaplaceno“ + „Zbývá uhradit“.
- Kontakty (e-mail, telefon, web) v patičce jsou z aktuálního `Account` (na faktuře nejsou snapshotované).
