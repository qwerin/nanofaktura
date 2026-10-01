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
  409 pro nepovolenou akci ve stavu (zamčená faktura…), 422 validace. Každá chyba má navíc strojově čitelný `code`
  (`has_invoices`, `locked`, `is_default`, … — seznam v `internal/api/errors.go`; bez specifického důvodu obecný podle
  statusu: `not_found`, `conflict`, `validation_failed`, `upstream_unavailable`, `upstream_timeout` …). Klienti se řídí
  `code`, anglický `detail` je jen pro vývojáře; frontend překládá kódy centrálně (`web/src/api/errors.ts`).
- **Testy**: každý endpoint má test v `internal/api/*_test.go` (package `api_test`) přes `httptest` a sdílený harness
  `internal/api/testutil_test.go` (in-memory SQLite, helper pro registraci+login, `do(method, path, body)`).
  Doménová logika v `internal/billing` má tabulkové unit testy.
- Po změně API: `make gen-types` přegeneruje `web/src/api/schema.gen.ts` (needitovat ručně).

## 3. Auth a účty

Instance je multi-user a multi-account: uživatel může mít přístup k více účtům (firmám/OSVČ).

- `User`: id, email (unique, lowercase), name, password_hash, created_at, updated_at.
- `Account`: id, slug (unique, z názvu, `[a-z0-9-]`), + firemní profil (viz 4.1).
- `Membership`: user_id, account_id, role `owner|member` (unique pár).
- `Session`: id, user_id, token_hash (sha256), expires_at (30 dní, posuvně), created_at. Cookie `nf_session`, HttpOnly, SameSite=Lax, Secure automaticky při https `NANOFAKTURA_PUBLIC_URL` nebo HTTPS požadavku (přepis `NANOFAKTURA_SECURE_COOKIES`).
- `APIToken`: id, user_id, name, token_hash, prefix (prvních 8 znaků pro zobrazení), last_used_at (s přesností na minutu), expires_at (volitelné, `expires_in_days` při vytvoření), created_at. Změna hesla tokeny uživatele zneplatní. Token `nf_` + 32 náhodných bajtů base64url; plaintext vrácen jen při vytvoření. Header `Authorization: Bearer nf_…`.

Endpointy:
| Metoda | Cesta | Popis |
|---|---|---|
| GET | `/api/health` | `{status:"ok"}` bez auth |
| GET | `/api/auth/status` | `{signup_allowed: bool, has_users: bool, setup_token_required: bool}` bez auth |
| POST | `/api/auth/register` | `{email,name,password,account_name}` → vytvoří user+account(owner)+session. Povoleno, pokud v DB není žádný uživatel, nebo `NANOFAKTURA_ALLOW_SIGNUP=true`. Jinak 403. Heslo 8–72 znaků a nejvýš 72 bajtů (jinak 422 `password_too_long`). Při nastaveném `NANOFAKTURA_SETUP_TOKEN` musí první registrace poslat `setup_token` (jinak 403 `setup_token_invalid`). |
| POST | `/api/auth/login` | `{email,password}` → `LoginResult {me?, two_factor?}`; bez 2FA set cookie + `me`, jinak výzva k druhému faktoru (§3.1) |
| POST | `/api/auth/logout` | smaže session |
| GET | `/api/auth/me` | `Me = {user:{id,email,name}, accounts:[{slug,name,role}]}` |
| PATCH | `/api/auth/me` | změna jména / hesla (`current_password` povinné při změně hesla) |
| GET/POST | `/api/auth/tokens` | seznam / vytvoření API tokenu |
| DELETE | `/api/auth/tokens/{id}` | revokace |
| GET/POST | `/api/accounts` | účty uživatele / založení nového účtu (tvůrce = owner) |
| GET/PATCH | `/api/accounts/{slug}` | firemní profil a nastavení (PATCH jen owner) |

Všechny doménové zdroje žijí pod `/api/accounts/{slug}/…`. Middleware ověří membership a vloží account do contextu;
nečlen → 404.

### 3.1 Obnova hesla a dvoufázové ověření (2FA)

Týká se uživatele (instance-wide), ne účtu; nic z toho není v záloze účtu (§7.16).

**Zapomenuté heslo** (vše `public`):
- `POST /api/auth/password-reset {email}` → vždy 204 (neprozradí, zda e-mail existuje). Existujícímu uživateli pošle
  e-mail s odkazem `{PUBLIC_URL}/reset-password/{token}`, platnost 1 h. Nejvýš jeden e-mail za 5 min na uživatele
  (další žádosti v té době se tiše ignorují). `PasswordReset`: user_id, token_hash (sha256), expires_at, used_at.
- `GET /api/auth/password-reset/{token}` → `{email, two_factor}`; neznámý 404, použitý/expirovaný 410 `reset_expired`.
- `POST /api/auth/password-reset/{token} {password}` → 204: nové heslo, smaže **všechny** sessions a všechny reset tokeny
  uživatele. Nepřihlašuje a **nevypíná 2FA** (po resetu se přihlásí heslem + druhým faktorem). API tokeny zůstávají.

**Druhý faktor**: TOTP (aplikace, RFC 6238: SHA-1, 6 číslic, 30 s, tolerance ±1 krok, znovupoužití stejného kroku odmítnuto)
a bezpečnostní klíče WebAuthn (YubiKey, passkey v telefonu/počítači) — libovolně kombinovatelné. S prvním zapnutým
faktorem vznikne 10 záložních kódů (`xxxxx-xxxxx`, jednorázové, uložen jen hash); po odebrání posledního faktoru se smažou.
- User: `totp_secret_enc` (šifrováno `secret.Box`), `totp_pending_enc` (rozpracované nastavení), `totp_last_step`.
  `RecoveryCode`: user_id, code_hash, used_at. `WebAuthnCredential`: user_id, name, credential_id (base64url, unique),
  data (JSON credential knihovny `go-webauthn`, vč. sign counteru), last_used_at.
  `AuthChallenge`: user_id, purpose `login|webauthn_register`, token_hash, webauthn_session (JSON), attempts, expires_at.
- WebAuthn RP ID = hostname `NANOFAKTURA_PUBLIC_URL`, povolené origins = origin `PUBLIC_URL` + `NANOFAKTURA_WEBAUTHN_ORIGINS`.
  Klíče jsou vázané na doménu — po změně domény je nutné je přidat znovu. Prohlížeč vyžaduje HTTPS (výjimka `localhost`).

**Přihlášení**: `POST /api/auth/login` vrací `LoginResult = {me?, two_factor?}`. Bez 2FA `me` + session cookie (jako dřív).
S 2FA jen `two_factor = {token, methods: [totp|webauthn|recovery], expires_at}` a **žádnou** cookie; token platí 10 min
a snese 5 neúspěšných pokusů (pak 401 `two_factor_expired`). Dokončení (vrací `Me` + cookie):
- `POST /api/auth/login/2fa {token, code}` — TOTP kód nebo záložní kód; špatný → 401 `invalid_code`.
- `POST /api/auth/login/webauthn/options {token}` → `{options}` (`PublicKeyCredentialRequestOptions` jako JSON s base64url),
  pak `POST /api/auth/login/webauthn {token, credential}` (JSON `PublicKeyCredential`); neověřený → 401 `webauthn_failed`.
API tokeny 2FA neobcházejí ani nevyžadují (vytvořit je lze jen po plném přihlášení).

**Správa** (`authed`; citlivé kroky vyžadují aktuální heslo, špatné → 422 `wrong_password` v `body.password`):
- `GET /api/auth/2fa` → `{enabled, totp, webauthn: [{id, name, created_at, last_used_at}], recovery_codes_left}`.
- `POST /api/auth/2fa/totp/setup {password}` → `{secret, otpauth_url}` (QR kreslí frontend); TOTP už zapnuté → 409 `totp_enabled`.
  `POST /api/auth/2fa/totp/enable {code}` → `{recovery_codes}` (prázdné, pokud už kódy existují); bez setupu 409
  `totp_not_pending`, špatný kód 422 `invalid_code`. `DELETE /api/auth/2fa/totp {password}` → 204.
- `POST /api/auth/2fa/webauthn/options {password}` → `{token, options}` (`PublicKeyCredentialCreationOptions`, vyloučí už
  registrované klíče), `POST /api/auth/2fa/webauthn {token, name, credential}` → 201 `{key, recovery_codes}`;
  neověřený → 422 `webauthn_failed`. `DELETE /api/auth/2fa/webauthn/{id} {password}` → 204.
- `POST /api/auth/2fa/recovery-codes {password}` → nové kódy (staré zneplatní); bez 2FA 409 `two_factor_disabled`.
- Zapnutí faktoru (aplikace i klíče) odhlásí ostatní sessions (aktuální zůstává).
- Ztracený telefon/klíč i záložní kódy: správce instance `nanofaktura user reset-2fa --email <e-mail>` (vypne 2FA).

Úklid: job `auth-cleanup` (1× za hodinu) maže expirované sessions, výzvy a reset tokeny.

Frontend: na `/login` odkaz „Zapomenuté heslo?“ → `/forgot-password`; `/reset-password/$token`; po hesle druhý krok na
téže stránce (kód / „Použít bezpečnostní klíč“ / záložní kód). Nastavení → nová záložka „Zabezpečení“ (2FA: aplikace s QR,
bezpečnostní klíče, záložní kódy ke stažení/zkopírování).

### 3.2 Správa instance a ověření e-mailu

**Správce instance** = uživatel, jehož e-mail je v `NANOFAKTURA_ADMIN_EMAILS` (čárkami, bez ohledu na velikost písmen)
**a zároveň** má ověřený e-mail (`users.email_verified_at`). Ověření je nutné: vlastník účtu může pozvat libovolnou
adresu a ta se pak zaregistruje — bez ověření by tak kdokoli získal správu instance. Neověřený uvedený e-mail práva nedává.

Ověření e-mailu: nastaví ho dokončená obnova hesla (odkaz přišel do schránky), odkaz z e-mailu nebo CLI.
- `POST /api/auth/me/verify-email` (přihlášený) → 204, pošle odkaz `{PUBLIC_URL}/verify-email/{token}` (starší neužité
  odkazy zneplatní); už ověřený 409 `email_already_verified`; limit 3/h na uživatele.
- `POST /api/auth/verify-email {token}` (veřejné) → `{email, verified}`; token uložen jako hash, platí 24 h, jednorázový;
  neznámý 404, vypršelý/použitý nebo mezitím změněná adresa 410 `verification_expired`; limit 20/10 min na IP.
- CLI `nanofaktura user verify-email --email <e-mail>` (např. když instance ještě neumí posílat e-maily).
- `GET /api/auth/me` vrací navíc `email_verified`, `instance_admin`, `instance_admin_pending` (uveden, ale neověřen).

Skupina `/api/admin/*` (authed + kontrola správce → jinak 403 `not_instance_admin`); každá změna se zapíše do logu
(`admin action`: operace, správce, IP).
- `GET /api/admin/status` → verze, Go, DB driver, start a uptime, počty uživatelů a účtů, souhrn konfigurace **bez tajemství**
  (veřejná URL/https, SMTP host/port/TLS, zda je nastaveno přihlášení, odesílatel, DKIM doména/selektor/typ klíče + TXT
  záznam s veřejným klíčem, limity pokusů, setup token nastaven?, registrace, trusted proxies, datový adresář a zda jde
  zapisovat, zdroj klíče tajemství env|file, seznam správců, API docs) a česká varování (`warnings`).
- `POST /api/admin/email-test {to?, dkim_selector?}` (výchozí příjemce = vlastní e-mail; limit 10/h na správce) →
  report `{status, sent, queue_id, server_reply, from, to, smtp_host, smtp_port, tls_mode, dkim_signed, smtp[], dns[]}`.
  Každý krok `{id, title, status ok|info|warning|error, message (česky), hint, details[], duration_ms}`.
  SMTP kroky: `config` (SMTP nenastaveno → jen tento krok + DNS), `resolve`, `connect`, `tls` (implicitní TLS),
  `banner`, `ehlo`, `starttls`, `ehlo_tls`, `auth` (mechanismus, nikdy heslo), `mail_from`, `rcpt_to` (relay denied),
  `data` (queue id). TLS: verze, šifra, subjekt/vydavatel/platnost certifikátu, shoda s názvem hostitele.
  DNS kontroly domény odesílatele: `mx` (i null MX), `spf` (právě jeden záznam, kvalifikátor `all`, limit 10 DNS
  dotazů, best-effort vyhodnocení IP SMTP serveru — jinak „nelze ověřit“), `dmarc` (`p=`, `rua`), `dkim`
  (nastavený selektor: záznam existuje a klíč odpovídá; jinak volitelně zadaný selektor). Pak se pošle skutečný
  český testovací e-mail (URL instance, čas, odesílatel, návod „Zobrazit originál“ v Gmailu → spf/dkim/dmarc=pass).
  CLI `nanofaktura mail test --to <e-mail> [--dkim-selector <s>]` (exit 1 při chybě).
- `GET /api/admin/users?query=` (stránkováno) → `{id, email, name, created_at, email_verified_at, two_factor, accounts,
  instance_admin, admin_listed}`; `POST /api/admin/users/{id}/reset-2fa|verify-email|send-verification` → 204
  (`send-verification` u ověřeného 409 `email_already_verified`).

**DKIM** (volitelné): `NANOFAKTURA_DKIM_DOMAIN` + `_SELECTOR` + `_PRIVATE_KEY` (PEM, `\n` povoleno) nebo `_PRIVATE_KEY_FILE`
(jen všechny najednou; RSA ≥ 2048 b nebo Ed25519; kanonikalizace relaxed/relaxed, SHA-256). Podepisuje se každá odchozí
zpráva; klíč se nikdy neloguje. Každá zpráva má `Date` a `Message-ID`. SMTP přihlášení PLAIN, nebo LOGIN, když server
nabízí jen ten.

Frontend: `/a/$slug/admin` (`/admin` přesměruje do prvního účtu) — záložky Stav instance, Test e-mailu (výsledek ve
skupinách Připojení / TLS / Přihlášení / Odeslání / DNS: SPF · DKIM · DMARC · MX), Uživatelé; položka v nabídce
uživatele, v mobilním „Více“ a v ⌘K jen pro `instance_admin`. `instance_admin_pending` → výzva „Ověřit e-mail“
v Můj profil / Zabezpečení; stránka `/verify-email/$token`.

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
`document_type`: `invoice` (faktura), `proforma` (zálohová), `correction` (opravný daňový doklad / dobropis),
`tax_document` (daňový doklad k přijaté platbě — vzniká automaticky z platby proformy plátce, viz „Zálohy“ níže; ručně ho vytvořit nelze).

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
supply_type (services|goods — u RC do EU: služba § 9/1 „daň odvede zákazník“, nebo zboží § 64 „osvobozeno“),
correction_reason (důvod opravy, u plátce povinný pro correction), client_local_vat_no (IČ DPH odběratele, snapshot `subjects.local_vat_no`),
lines[], payments[] (+ `tax_document_id`, `source_payment_id`), related_documents[], deposits[],
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
**Hromadná úhrada** `POST /api/accounts/{slug}/invoices/mark-paid` (a `/expenses/mark-paid`) s query parametry filtru seznamu
`{paid_on?, dry_run?}` → 200 `{count, sums}`: každý neuhrazený doklad filtru (faktury `open|sent`, náklady `open`; `total ≠ paid_amount`)
dostane platbu zbývající částky, v jedné transakci. Datum: `paid_on`, jinak splatnost (bez ní vystavení), nejpozději dnes.
`dry_run` jen spočítá (`sums[].sum_remaining` = částka k úhradě). Poděkování za úhradu se neposílá (staré doklady zadané zpětně).
`create_final_invoice` u proformy vytvoří vyúčtovací `invoice` se stejnými řádky, `related_id=proforma.id`, s datem platby, a převezme
**všechny** platby proformy (kopie se `source_payment_id`) — při částečné platbě tedy zbytek dluží vyúčtovací faktura; proforma se tím
**vyúčtuje** (status `paid`, další platby/úpravy → 409 `proforma_settled`). id nové faktury vrátit v odpovědi (`final_invoice_id`).
Samostatně: `POST …/invoices/{id}/final-invoice {issued_on?, taxable_fulfillment_due?}` → 201 vyúčtovací faktura (DUZP = datum dodání).

**Zálohy a daňové doklady k přijaté platbě (§ 21, § 28 ZDPH)**: každá platba proformy plátce (bez přenesení daňové povinnosti)
vytvoří `tax_document`: vlastní řada (výchozí `ZD{YYYY}-{NNNN}`), vystavení = DUZP = datum platby, řádky „Přijatá platba k zálohové faktuře č. …“
s cenou vč. DPH rozdělenou poměrem sazeb proformy (DPH koeficientem z přijaté částky), kurz ČNB ke dni platby; doklad je „zaplacen“ kopií
platby (`source_payment_id`), není pohledávkou a nejde upravit částkou/měnou/daty ani smazat (409 `tax_document_fixed`) — maže se
spolu s platbou proformy. Platí i pro platby z bankovního párování a hromadné úhrady. DPH vyúčtovací faktury se v přiznání/KH
snižuje o daňové doklady její proformy (`deposits`, PDF „Odpočet záloh“, ISDOC `TaxedDeposits`). Kopie plateb nejdou smazat samostatně
(409 `advance_payment`); smazání vyúčtovací faktury, která má jen převzaté platby, proformu znovu otevře.

**Dobropis**: `POST …/invoices/{id}/correction {correction_reason?}` → vytvoří `correction` k faktuře s řádky zkopírovanými a zápornými množstvími
(klient ho pak upraví přes PATCH). U plátce je důvod opravy povinný (422 `body.correction_reason`). Vrací novou fakturu.

**Duplikace**: `POST …/invoices/{id}/duplicate` → nová faktura (open, nové číslo, dnešní datum, stejné řádky a subjekt; cizí měna → kurz ČNB nového data).

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
`NANOFAKTURA_PUBLIC_URL` (veřejná adresa pro odkazy v e-mailech, default `http://localhost:8080`), `NANOFAKTURA_DATA_DIR` (`./data`),
`NANOFAKTURA_SMTP_HOST` (prázdné = e-maily se jen vypisují na stdout), `NANOFAKTURA_SMTP_PORT` (587, u `tls` 465),
`NANOFAKTURA_SMTP_USER`, `NANOFAKTURA_SMTP_PASSWORD`, `NANOFAKTURA_SMTP_TLS` (`starttls`|`tls`|`none`, default `starttls`),
`NANOFAKTURA_MAIL_FROM` (odesílatel, default `NanoFaktura <nanofaktura@localhost>`).
`NANOFAKTURA_CNB_URL`, `NANOFAKTURA_VIES_URL`, `NANOFAKTURA_VATREG_URL` (přepis adres externích služeb, pro testy).
`NANOFAKTURA_SECRET_KEY` (32 B base64/hex, šifruje uložená tajemství — Fio tokeny; nezadaný → vygeneruje se a uloží do `NANOFAKTURA_DATA_DIR/secret.key` s varováním v logu), `NANOFAKTURA_FIO_URL` (přepis Fio API, pro testy).
`NANOFAKTURA_WEBHOOKS_ALLOW_PRIVATE` (`true` = webhooky smí volat privátní/loopback/link-local adresy; výchozí `false`, ochrana proti SSRF).
`NANOFAKTURA_DB_LOG` (`error` — logování SQL: `silent|error|warn|info`; hodnoty parametrů se nelogují nikdy, „record not found“ také ne),
`NANOFAKTURA_DB_SLOW_MS` (1000 — hranice pomalého dotazu pro `warn`),
`NANOFAKTURA_IMPORT_MAX_MB` (512 — limit nahrané zálohy i celkové rozbalené velikosti při importu, §7.16).
`NANOFAKTURA_WEBAUTHN_ORIGINS` (další povolené origins pro bezpečnostní klíče, čárkou oddělené, např. `http://localhost:5173` při vývoji; §3.1).

**Bezpečnost (§5.1):** `NANOFAKTURA_SECURE_COOKIES` (`auto` = výchozí: Secure při https `PUBLIC_URL` nebo HTTPS požadavku; `true`/`false` vynutí),
`NANOFAKTURA_TRUSTED_PROXIES` (čárkami oddělené IP/CIDR reverzních proxy, kterým se věří `X-Forwarded-For`/`-Proto`; výchozí žádné),
`NANOFAKTURA_SETUP_TOKEN` (je-li nastaven, první registrace prázdné instance ho musí zadat), `NANOFAKTURA_DISABLE_RATE_LIMIT`
(false; vypne limity pokusů), `NANOFAKTURA_DISABLE_API_DOCS` (false; skryje `/api/docs`, `/api/openapi.json`, `/api/schemas`).

**Správa instance a e-mail (§3.2):** `NANOFAKTURA_ADMIN_EMAILS` (čárkami oddělené e-maily správců instance; práva jen
s ověřeným e-mailem), `NANOFAKTURA_DKIM_DOMAIN`, `NANOFAKTURA_DKIM_SELECTOR`, `NANOFAKTURA_DKIM_PRIVATE_KEY` /
`NANOFAKTURA_DKIM_PRIVATE_KEY_FILE` (DKIM podpis odchozích e-mailů; všechny tři, nebo žádný).

### 5.1 Zabezpečení HTTP
- **Hlavičky** (`internal/httpsec`, na API i SPA): SPA má CSP `default-src 'self'; script-src 'self'` (žádný inline skript ani eval —
  motiv před vykreslením nastaví `/theme-init.js`, Zod běží `jitless`), `style-src 'self' 'unsafe-inline'`, `img-src 'self' data: blob:`,
  `frame-src 'self' blob:` (náhled PDF), `frame-ancestors 'none'`; API `frame-ancestors 'self'`. Dále `X-Content-Type-Options: nosniff`,
  `Referrer-Policy` (`strict-origin-when-cross-origin`, u `/p/*`, `/invite/*` a API `no-referrer`), `Permissions-Policy`, HSTS
  (`max-age=31536000`) při HTTPS; `/api/public/*` `Cache-Control: private, no-store`.
- **CSRF:** nebezpečné metody autentizované cookie musí být same-origin (`Sec-Fetch-Site`/`Origin`, `http.CrossOriginProtection`) → jinak
  403 `cross_origin_request`; požadavky s hlavičkou `Authorization` (API tokeny) jsou vyjmuté. Tělo musí mít `Content-Type`
  `application/json` (`+json`) nebo `multipart/form-data`, jinak 415.
- **Limity pokusů** (v paměti procesu, 429 `rate_limited` + `Retry-After`): login 20/IP (pak 2/min) a 5 neúspěchů/e-mail za 15 min,
  registrace 5/IP (10/h), pozvánky (náhled/přijetí) 20/IP za 10 min, veřejné odkazy faktur 60/IP/min, špatné současné heslo 5/uživatel
  za 15 min, e-maily odeslané uživatelem (faktury, pozvánky) 50/účet/h a max. 10 příjemců, těžké exporty (ZIP PDF, záloha) 10/účet/h,
  souběžně jeden na účet a dva na instanci (429 `export_in_progress`). Obnova hesla (§3.1): žádost 5/IP i 5/e-mail za hodinu,
  náhled/potvrzení odkazu 20/IP za 10 min. 2FA: kroky přihlášení se počítají do loginu per IP, špatný druhý faktor (login
  i potvrzení TOTP) 10/uživatel za hodinu napříč výzvami; špatné heslo ve správě 2FA sdílí limit „současné heslo“. IP klienta z `X-Forwarded-For` jen od `NANOFAKTURA_TRUSTED_PROXIES`.
- **Obrázky** (logo, razítko): PNG/JPEG max. 2 MB a 4000×4000 px (kontrola hlavičky, 422). Každý obrázek pro PDF se před vložením
  znovu ověří a překóduje (zmenšení na 1200 px, cache podle obsahu); nevyhovující se do PDF nevloží. Souběžné rendery PDF jsou omezené.
- **Chyby 5xx** neobsahují interní příčinu (loguje se); validační chyby hesel/tokenů nevracejí zadanou hodnotu.
- Server má `ReadTimeout` 2 min, `WriteTimeout` 5 min, `IdleTimeout` 2 min; uploady a streamované exporty si lhůty prodlužují.

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
tax_deductible (bool, default true — daň z příjmů), vat_deductible (bool, default true — odpočet DPH, nezávislý na tax_deductible),
reverse_charge (bool, příjemce přiznává daň: služby/zboží z EU, § 92a, služby ze třetích zemí), supply_type (services|goods),
prices_include_vat, lines (stejná struktura a výpočet jako InvoiceLine, sdílený kód v billing),
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
Šablony textů v nastavení účtu (`email_templates`: invoice/reminder/paid_thanks × cs/en/sk/de; jazyk e-mailu = jazyk dokladu, jiný → cs) s placeholdery `{number} {total} {due_on} {public_url} {account_name}…`.
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
Při vytvoření dokladu v cizí měně a nezadaném kurzu se použije kurz ČNB ke dni DUZP (resp. vystavení) — i u faktur ze šablony,
pravidelných faktur, duplikátů, vyúčtovacích faktur (kurz jejich DUZP) a daňových dokladů k platbě (kurz dne platby). Kurz musí být > 0 (422).
Klient za interfacem.
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
- **DPH (jen plátci)**: `GET /reports/vat?period=2026-09|2026-Q3` → podklad přiznání (řádky 1–6, 10–13, 20–26, 40–46…) a **kontrolní hlášení** (A.1/A.2/A.4/A.5/B.1/B.2/B.3)
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
- Instalace jako aplikace (PWA, `display: standalone` — bez adresního řádku): položka „Nainstalovat aplikaci“ v menu (mobil „Více“,
  desktop uživatelské menu). Chromium → systémový dialog (`beforeinstallprompt`), iOS a ostatní mobilní prohlížeče → návod
  (Sdílet → Přidat na plochu); v nainstalované aplikaci se nenabízí.

### 7.16 Záloha, export a import účtu
Kompletní přenos účtu mezi instancemi (a SQLite → Postgres) a záloha.
- **Formát**: ZIP `nanofaktura-<slug>-<YYYY-MM-DD>.zip`: `manifest.json` (`format: "nanofaktura-backup"`, `version: 1`,
  `exported_at`, `app_version`, `account: {slug, name}`, `counts` per entita, SHA-256 každého souboru), `account.json`
  (profil + všechna nastavení), a JSON pole per entita: `bank_accounts`, `number_formats` (+ čítače), `subjects`,
  `price_items`, `stock_moves`, `invoices` (s `lines`, `payments`), `expenses` (s `lines`, `payments`), `templates`,
  `recurring`, `bank_transactions`, `todos`, `events`, `email_logs`, `webhooks`, `members` (jen e-mail, jméno, role —
  informativně). Přílohy: `attachments.json` + soubory `attachments/<id>/<filename>`.
  Exportní DTO jsou vlastní, stabilní a verzované (ne GORM modely ani API výstupy), s původními ID pro vazby.
- **Nikdy se neexportují tajemství**: Fio tokeny, secrety webhooků, tokeny pozvánek/sessions/API. Webhooky se po importu
  založí neaktivní bez secretu, bankovní účty bez tokenu, veřejné odkazy faktur dostanou nové tokeny.
- **Export**: `GET /api/accounts/{slug}/backup` (owner, admin) streamuje ZIP; událost `account.exported`.
- **Import**: `POST /api/accounts/import` (authed, multipart `file`, volitelně `name`) → **vždy nový účet** (volající = owner,
  nový slug), vše v jedné transakci (vše nebo nic), přemapování ID, soubory příloh zapsány po commitu (při chybě uklizeny).
  Validace: formát + verze manifestu (novější verze → 422 `unsupported_backup_version`), kontrolní součty, ochrana proti
  zip-slip a zip bomb (limit velikosti `NANOFAKTURA_IMPORT_MAX_MB`, default 512), neznámé soubory ignorovat.
  Číselné řady a čítače se přenesou (další číslo navazuje), recurring se naimportují **neaktivní** (aby nová instance
  nevystavovala duplicitně), upomínky a poděkování za platbu se po importu vypnou — s upozorněním v odpovědi `warnings[]`.
- **CLI** (pro správce instance): `nanofaktura backup export --account <slug> [--out soubor.zip]` a
  `nanofaktura backup import --owner <email> soubor.zip` (bez HTTP, stejný kód jako API).
- **UI**: Nastavení → „Záloha a přenos“ (stáhnout zálohu, co obsahuje, co ne); v přepínači účtů „Nový účet“ → „Obnovit ze zálohy“.

### 7.17 Mimo rozsah
EET (zrušeno), účetnictví (podvojné), mzdy, OCR účtenek (jen příprava: přílohy nákladu).

## Otevřené otázky
(sem zapisují implementátoři odchylky a nejasnosti)

**Peníze a DPH (2026-10-01, opravy z auditu) — rozhodnutí a odchylky:**
- Souběh: každá transakce, která čte a mění platby/součty/stav dokladu (platby, PATCH, akce, smazání, hromadná úhrada, párování,
  vyúčtování), nejdřív zamkne řádek dokladu (`loadInvoiceForUpdate` / `loadExpenseForUpdate`, `SELECT … FOR UPDATE`); bankovní
  transakce se zamyká před dokladem. `paid_amount`/`status`/`paid_on` se počítají ze součtu uložených plateb a zapisují samostatnými
  sloupci (`refreshInvoicePayments`). Auto-párování po zamčení znovu ověří, že transakce není spárovaná a zbývá přesně její částka.
  Testy souběhu běží na PostgreSQL s `NANOFAKTURA_TEST_PG_DSN` (jinak se přeskočí).
- Zálohy (viz §4.5 „Zálohy“): přijatá platba proformy je peníz (daň z příjmů ji počítá, dokud proforma nemá vyúčtovací fakturu; pak
  ji nese převzatá platba vyúčtovací faktury se stejným datem a částkou). Daňové doklady k platbě nejsou tržba ani pohledávka a do
  daně z příjmů nevstupují. Daňový doklad se nevytváří u proformy s přenesením daňové povinnosti (zálohy na plnění do EU řeší
  uživatel ručně) ani u neplátce/identifikované osoby; záporná platba proformy vytvoří opravný daňový doklad k přijaté platbě
  (záporné částky). Odeslaný daňový doklad k platbě se smaže spolu s platbou (zamčený → 409). Kurz chybějící v cache ČNB → kurz proformy.
- Vyúčtovaná proforma má `status=paid` i při částečné úhradě (zbytek je na vyúčtovací faktuře); dashboard, seznamy, upomínky
  a párování ji tak nepočítají jako pohledávku. Starší data: proformy, ke kterým existuje vyúčtovací faktura vytvořená dříve
  z částečné platby, zůstávají `open` — nová logika je nezmění (viz poznámka k produkci v popisu opravy).
- DPH: DUZP je u daňového dokladu plátce (a RC dokladu identifikované osoby) povinné (422); staré doklady bez DUZP vstupují do
  přiznání podle data vystavení s varováním `missing_taxable_date`. Sazby plátce od 2024 jen 0/12/21 % (starší DUZP smí 10/15 %).
  Důvod opravy (`correction_reason`) je u opravného dokladu plátce povinný. Odeslaný (nebo klientem zobrazený) daňový doklad nelze
  stornovat → 409 `correction_required` (neplátce a neodeslané doklady ano). Evidenční čísla faktur, dobropisů a daňových dokladů
  k platbě jsou unikátní společně (409 `already_exists`).
- Přenesená daňová povinnost na výstupu: `supply_type=goods` → ř. 20 a text „Osvobozeno … § 64“, jinak služba ř. 21 a „Daň odvede
  zákazník“; u EU RC se vyžaduje VAT ID s prefixem státu (slovenské IČ DPH z `local_vat_no` má přednost) a report připomene souhrnné
  hlášení (`ec_sales_list`) — export DPHSHV zatím není. Identifikovaná osoba: RC doklad = „Faktura – daňový doklad“ s DUZP.
- Náklady: `vat_deductible` (odpočet DPH) je oddělený od `tax_deductible` (daň z příjmů); při migraci se převezme z `tax_deductible`.
  `reverse_charge` na nákladu = samovyměření: dodavatel z EU → ř. 3/4 (zboží) nebo 5/6 (služby) + KH A.2; CZ dodavatel → ř. 10/11 + KH B.1
  (kód předmětu plnění `kod_pred_pl` je nutné doplnit v EPO, varování); mimo EU → ř. 12/13 + A.2 (zboží mimo EU = dovoz, nevykazuje se,
  varování); s odpočtem ř. 43/44. Nulová DPH od zahraničního dodavatele nebo CZ plátce bez RC → varování `possible_reverse_charge`.
- KH: právnická osoba (DIČ = 8 číslic) podává KH měsíčně — `dphkh1.xml` za čtvrtletí → 409 `monthly_control_statement`, JSON report varuje.
  DP3 ř. 46 i 62–65 se sčítají ze zaokrouhlených řádků.
- PDF plátce v cizí měně ukazuje „DPH v Kč (kurz ČNB …)“ přepočtenou po sazbách stejně jako přiznání; u RC je ve sloupci sazby „PDP“.
  Veřejný odkaz (JSON) rekapitulaci v Kč zatím neukazuje (PDF ke stažení ano).
- Ostatní: změna měny dokladu s platbami → 409 `currency_has_payments`; ruční platba se znaménkem opačným k zbývající částce → 422;
  `sum_remaining` v seznamech jen z otevřených dokladů (přeplatky se nesčítají); dashboard „neuhrazeno“ jen kladné pohledávky
  (dobropisy k vrácení ne). Bankovní import vždy v setinách (i JPY/BHD; třetí nenulové desetinné místo → chyba). QR Platba nad
  9 999 999,99 se negeneruje. Import zálohy přepočítá součty dokladů z řádků a plateb (varování `totals_recomputed`).
  Daň z příjmů v cizí měně počítá kurzem dokladu (`income_tax.rate_basis = document`); doklady s neplatným kurzem jsou v
  `invalid_rate_documents` / `invalid_rates`, nikdy tiše jako 0.
- Neřešeno (zdůvodnění v popisu opravy): export souhrnného hlášení DPHSHV, kód předmětu plnění § 92a na dokladu, `c_okec` a
  jméno/příjmení FO v EPO, VS proforem odlišný od faktur, kontrola plátcovství odběratele (A.4 vs A.5), zjednodušený daňový doklad.

**Zabezpečení (2026-09-30) — rozhodnutí:**
- Odkaz pozvánky (`invite_url`) vidí jen ten, kdo smí danou roli udělit (owner pozvánky jen owner). Pozvánka propadá (410
  `invitation_revoked`), pokud pozvávající už není členem s právem roli udělit (kontrola při náhledu, registraci i přijetí).
- Přílohy s `owner_type=account` (logo, razítko) nahrává a maže jen owner/admin (jde o nastavení).
- Změna hesla zneplatní i všechny API tokeny uživatele (dosud jen ostatní sessions); tokeny bez expirace fungují dál, dokud se heslo nezmění.
- Hesla nad 72 bajtů se odmítají (422), nepředhashovávají — existující bcrypt hashe zůstávají platné.
- Úlohou `auth-cleanup` (denně) se mažou prošlé sessions, API tokeny a pozvánky prošlé déle než 7 dní.
- Události `webhook.*` obsahují jen `scheme://host` webhooku (dříve celé URL) a čtou je jen owner/admin.
- Blokace SSRF webhooků pokrývá všechny IANA special-purpose rozsahy (NAT64/6to4 podle vložené IPv4, Teredo) a porty mimo 80, 443, 1024–65535.
- CSV exporty předřazují `'` textovým buňkám začínajícím `= + - @ tab CR`.
- Limity pokusů jsou v paměti procesu (restart je vynuluje, víc instancí je nesdílí) — pro jednu self-hosted instanci dostačující.
- Neřešeno: `secret.Box` bez AAD (prohození šifrovaných hodnot mezi řádky vyžaduje zápis do DB), prefix `__Host-` u cookie
  (odhlásil by všechny uživatele a nefunguje na http v dev), dashboard ukazuje tržby i roli member (ta vidí i všechny faktury).

**Backend základ (auth, účty) — rozhodnutí a odchylky:**
- Seznamy tokenů (`GET /api/auth/tokens`) a účtů (`GET /api/accounts`) používají také stránkovanou obálku `{items,page,per_page,total}` (jednotnost; `Me.accounts` zůstává plné pole).
- Výstup `Account` nemá `id` (identifikátor je `slug`) a obsahuje `role` aktuálního uživatele. `POST /api/accounts` přijímá jen `{name}`, profil se doplní přes PATCH. PATCH účtu členem (ne ownerem) → 403 (člen existenci účtu zná). Slug: max 50 znaků, prázdný → `ucet`, kolize → `-2`, `-3`…
- Statusy: register 201, login 200, špatné přihlašovací údaje 401 (`invalid email or password`), bez přihlášení 401, logout 204 a nevyžaduje přihlášení (idempotentní; maže session z cookie). Změna hesla bez/špatné `current_password` → 422 (`body.current_password`). Změna hesla zatím nezneplatňuje ostatní sessions.
- Session: expirace se posouvá nejvýš jednou za 24 h (při posunu server pošle obnovenou cookie). Bearer token má přednost před cookie.
- 2FA (§3.1): odpověď loginu se změnila z `Me` na `LoginResult` (jediný konzument je SPA). Token výzvy chodí v těle, ne v cookie
  (jednodušší pro API klienty; je krátkodobý a použitelný jen s druhým faktorem). WebAuthn slouží jen jako druhý faktor,
  přihlášení bez hesla (discoverable passkeys) zatím není. Obnova hesla neposílá e-mail o změně hesla a není rate-limitovaná
  jinak než 1 e-mail / 5 min na uživatele (zbytek na reverse proxy).
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
- Nová pole už teď: `public_token` (32 znaků base64url, unique, vzniká při vytvoření; `POST …/regenerate-public-token` → 200 + faktura, v jakémkoli stavu) a `InvoiceLine.price_item_id` (volitelné; od milníku 2 validované, viz Ceník níže).
- Odkazy v těle (`subject_id`, `bank_account_id`, `related_id`) na neexistující/cizí záznam → 422 (ne 404). `related_id` je u ne-dobropisů volitelné, ale musí existovat v účtu; dobropis musí odkazovat na doklad typu `invoice`; odkaz sám na sebe → 422.
- DUZP default = `issued_on`, pokud `your_vat_mode != non_vat_payer` (tj. i identifikovaná osoba). Vynucení sazeb na 0 i výpočet se řídí snapshotem `your_vat_mode` faktury, ne aktuálním nastavením účtu (PATCH staré faktury tak počítá stejně). Klient smí přepsat i `your_*` a `bank_account/iban/swift_bic`.
- Bankovní účet: výchozí pro měnu (`is_default DESC, id`, tj. fallback na jiný účet téže měny); bez účtu v měně zůstanou bankovní pole prázdná.
- Řádky: `quantity` default `"1"`, `vat_rate_bps` default `default_vat_rate_bps` účtu (0–10000), `|unit_price| ≤ 10^14`, max 500 řádků; přetečení int64 ve výpočtu → 422 `body.lines`. Řádek v PATCH je úplný objekt (vynechaná volitelná pole = defaulty, ne původní hodnoty), `id` jen určuje, který řádek se aktualizuje; cizí nebo opakované `id` → 422. `position` = pořadí v poli.
- `variable_symbol` default = `spayd.Digits(number, 10)`; změna `number` PATCHem VS nepřepočítá. Bez výchozí číselné řady → 409 (vlastní `number` řadu nepotřebuje). Přidělení čísla je vyměnitelné přes `api.Deps.NextNumber` (default `numbering.Next`).
- PATCH: `document_type` měnit nelze. Změna `currency` bez `bank_account_id` znovu vybere výchozí bankovní účet nové měny. Změna `issued_on` nepřepočítává DUZP. Po PATCH se přepočítá i stav z plateb (zaplacená faktura s vyšší částkou → zpět `open`/`sent`). `tags` nahrazují seznam (trim, bez duplicit).
- Akce vrací 200 + fakturu, neznámá akce → 422 (enum v cestě). Zámek blokuje jen PATCH/DELETE (akce a platby na zamčené faktuře jsou povolené).
- Platby: `POST` → 201 `{payment, invoice, final_invoice_id?}`, `DELETE` → 204. `amount: 0` → 422; bez `amount` při nulovém zůstatku → 409. `paid` vyžaduje aspoň jednu platbu (faktura s total 0 bez plateb zůstává open). U `uncollectible` lze platby mazat, stav se nemění.
- `create_final_invoice`: jen proforma (jinak 422) a jen jednou (vyúčtovaná proforma → 409 `proforma_settled`, existující vyúčtování → 409 `final_exists`; vrátí se celá transakce vč. platby). Finální faktura: `issued_on` (a DUZP u plátce) = datum platby, `client_*` z proformy, `your_*` z aktuálního účtu, řádky/měna/poznámky z proformy.
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

**Role, pozvánky, e-mail, přílohy — rozhodnutí a odchylky:**
- Role `owner|admin|accountant|member` (Membership.role). Kontrola deklarativně: operace účtu nese `auth.ForEditors` (owner, admin, member), `auth.ForManagers` (owner, admin) nebo `auth.Allow(...)`; middleware účtu vrací 403 (`your role (x) cannot do this; allowed roles: …`) ještě před validací vstupu. Nedeklarovaná operace = smí každý člen. Role jsou v OpenAPI jako `x-roles`. Test `TestEveryAccountMutationDeclaresRoles` hlídá, že každá mutace pod `/api/accounts/{slug}` roli deklaruje (výjimka: `DELETE /members/{user_id}`).
- Matice: čtení všichni (včetně `GET /members`); doklady, platby, akce faktur, kontakty, přílohy: owner/admin/member; nastavení (PATCH účtu, bankovní účty, číselné řady), správa členů a pozvánek (`PATCH/DELETE /members`, `POST /members/invite`, `GET/DELETE /invitations`): owner/admin. Oproti dřívějšku smí nastavení měnit i admin (dříve jen owner). Accountant je zatím čistě read-only (exporty/reporty zatím nejsou).
- Admin nesmí spravovat ownery: měnit/odebrat ownera, udělit roli owner, pozvat ownera, zrušit pozvánku ownera → 403 `only an owner can manage owners`. Poslední owner nejde degradovat ani odebrat (ani sám odejít) → 409. `DELETE /members/{user_id}` na sebe = opuštění účtu (smí kdokoli). Mazání účtu zatím neexistuje.
- `Member` výstup: `{user_id, email, name, role, joined_at}`; `PATCH /members/{user_id}` `{role}`. Člen se adresuje `user_id` (ne id membershipu).
- Pozvánky: `POST /members/invite {email, role}` → 201 `Invitation {id, email, role, invited_by, expires_at, created_at}`, platnost 7 dní, token v DB jen jako sha256. Nová pozvánka na stejný e-mail nahradí nepřijatou předchozí. Pozvat existujícího člena → 409. Selže-li odeslání e-mailu → 502 a pozvánka se neuloží. `GET /invitations` vrací jen čekající (nepřijaté, neexpirované); `DELETE /invitations/{id}` jen nepřijaté.
- Veřejné `GET /api/invitations/{token}` → `{account_name, email, role, expires_at, user_exists}` (`user_exists` říká frontendu, zda nabídnout přihlášení nebo registraci). Neznámý token 404, použitý/expirovaný 410. `POST /api/invitations/{token}/accept` (přihlášený) → `MeAccount`; e-mail uživatele musí sedět (bez ohledu na velikost písmen), jinak 403; už člen → 409.
- Registrace s `invitation_token`: obchází zákaz registrace, nezakládá účet (pole `account_name` je pak volitelné; bez tokenu je povinné → 422 `body.account_name`), e-mail musí sedět s pozvánkou (422 `body.email`). Odkaz v e-mailu: `{NANOFAKTURA_PUBLIC_URL}/invite/{token}`. Text pozvánky je zatím pevný (česky), šablony e-mailů z §7.4 přijdou s odesíláním faktur.
- Mailer: `internal/mail` (`Mailer.Send(ctx, Message)`), SMTP přes `net/smtp` (STARTTLS povinné v režimu `starttls`, implicitní TLS v `tls`, AUTH PLAIN jen přes TLS nebo na localhost), bez SMTP hostu log na stdout; per-account SMTP override (§7.4) zatím není.
- Přílohy: `owner_type` navíc `account` (logo, razítko, dokumenty firmy; `owner_id` = id účtu, při uploadu může být 0). Upload `POST /attachments` (multipart: `file`, `owner_type`, `owner_id`) jen owner/admin/member; vlastník musí existovat v účtu (jinak 404 `owner not found`). Limit 20 MB → 413. Typ se určuje z obsahu (ne z názvu/hlavičky): pdf, png, jpeg, webp, heic, xml; jiné → 415. Název souboru se ořízne na základní jméno bez řídicích znaků (max 200 znaků).
- `GET /attachments/{id}` vrací metadata, stahování je `GET /attachments/{id}/download` (odchylka od §7.12, kde je download přímo na `/{id}`), `?inline=true` → `Content-Disposition: inline`; odpověď má `X-Content-Type-Options: nosniff` a `Content-Security-Policy: sandbox`. `GET /attachments?owner_type=&owner_id=` stránkovaný seznam.
- Úložiště: `internal/storage` (`Put/Get/Delete` podle klíče), lokálně `NANOFAKTURA_DATA_DIR/attachments/{account_id}/{random}`; S3 zatím není.
- Account `logo_attachment_id`, `stamp_attachment_id` (PATCH; `0` = odebrat): příloha musí patřit účtu a být PNG nebo JPEG (jen ty umí PDF) → jinak 422. Smazání přílohy nastavené jako logo/razítko pole vynuluje. PDF faktury logo i razítko používá. Smazání vlastníka (kontakt, faktura) zatím jeho přílohy nemaže.

**Ceník, sklad, náklady — rozhodnutí a odchylky:**
- Ceník `/price-items`: výstup navíc `archived` (bool) a `low_stock` (bool = `track_stock` a `stock_quantity ≤ min_stock`; bez `min_stock` vždy false). `min_stock` je string, `""` = bez limitu. Archivace/obnovení přes `PATCH {archived: true|false}` (žádné zvláštní akce). List: `?archived=true` vrací jen archivované, default jen aktivní; navíc `?low_stock=true`; `query` hledá v názvu a SKU; řazení podle `LOWER(name)`. SKU není unikátní. `vat_rate_bps` default = výchozí sazba účtu, `currency` = výchozí měna účtu. `stock_quantity` je vždy ve výstupu (i bez `track_stock`).
- `stock_quantity` (v DB `stock_quantity_milli`) = Σ pohybů vždy; mění se jen spolu se `StockMove` ve stejné transakci. PATCH ho nemění; `stock_quantity` v create (jen s `track_stock`, jinak 422) vytvoří ruční pohyb „initial stock“. Stav smí jít do záporu (výdej se neblokuje).
- `StockMove.quantity` je vždy kladná, znaménko určuje `direction`. Ruční pohyby: `POST /price-items/{id}/stock-moves` → 201 `{move, price_item}` (jen u `track_stock`, jinak 409; `moved_on` default dnes), `DELETE …/stock-moves/{move_id}` → 204 jen pro ruční pohyby (vygenerované z dokladu → 409). List je stránkovaný, `moved_on DESC`.
- Doklady a sklad: pohyby dokladu se při každé změně celé smažou a zapíšou znovu z aktuálních řádků (create, PATCH, `cancel`/`undo_cancel`, smazání). Pohyb vzniká jen pro řádky s `price_item_id` na položku s `track_stock` a nenulovým množstvím; `moved_on` = `issued_on`. Faktura (`invoice`) a dobropis (`correction`) vydávají (`out`), záporné množství = příjem (`in`), takže dobropis zboží vrací; proforma nikdy; `cancelled` nemá pohyby, `uncollectible` je ponechává (zboží bylo dodáno). Platí i pro faktury vzniklé přes `/duplicate`, `/correction` a `create_final_invoice`. Zapnutí `track_stock` dodatečně nevytváří pohyby starým dokladům (vzniknou až při jejich PATCH).
- Náklady s řádky na ceníkovou položku s `track_stock` zboží přijímají (`in`, záporné množství `out`); `StockMove.expense_id` se tak už používá.
- `price_item_id` na řádcích faktur i nákladů musí patřit účtu (jinak 422 `lines[i].price_item_id`); archivovaná položka je povolená. Smazání položky ceníku smaže její pohyby a u řádků dokladů `price_item_id` vynuluje (řádky zůstanou).
- Náklady `/expenses`: `subject_id` je volitelné (účtenky bez kontaktu), ale pak je povinné `supplier_name` (422). Snapshot `supplier_*` (name, full_name, registration_no, vat_no, street, city, zip, country, bank_account, iban, swift_bic) ze subjektu při create a při změně `subject_id`; poslaná pole mají přednost. Defaulty: `issued_on` dnes, `taxable_fulfillment_due = issued_on`, `due_on = issued_on + default_due_days účtu` (náklad nemá `due_days`), měna/platba z účtu, `exchange_rate` 1, `tax_deductible` true, `variable_symbol` = číslice z `original_number` (max 10). Číslo z řady `expense` (`numbering.Next`), vlastní `number` smí; duplicita → 409.
- Řádky nákladů mají stejný vstup jako řádky faktur (`InvoiceLineInput`, stejná pravidla PATCH), výpočet `billing.Calculate` s `prices_include_vat` a `round_total` (nové pole nákladu, default false). DPH dodavatele se počítá vždy — nezávisle na tom, zda je účet plátce (sazby se nenulují); `reverse_charge` u nákladů = samovyměření DPH příjemcem (viz „Peníze a DPH (2026-10-01)“).
- Stav nákladu: uloženo `open|paid` (`billing.PaymentStatus`), výstup `overdue` pro `open` po splatnosti; filtr `status=open` vrací jen náklady, které nejsou po splatnosti. Zrušení nákladů neexistuje (smazat). Platby: samostatný model `ExpensePayment` (zrcadlí `Payment`), `POST /expenses/{id}/payments` → 201 `{payment, expense}`, `DELETE …/payments/{payment_id}` → 204; stejné chování jako u faktur (0 → 422, bez částky a nulový zůstatek → 409, platby i na zamčeném nákladu).
- Akce nákladu `POST /expenses/{id}/actions/{lock|unlock}` → 200 + náklad; zámek blokuje PATCH/DELETE (409). Smazání s platbami → 409.
- List nákladů: filtry `status`, `category` (přesná shoda), `subject_id`, `since`/`until` (issued_on), `query` (number, original_number, supplier_name, variable_symbol, description), `sort` jako u faktur. `GET /expenses/categories?query=` → `{items: [...]}` (distinct neprázdné kategorie, abecedně, max 200, bez stránkování).
- Dashboard: `expenses_by_month` (Σ `total` nákladů dle `issued_on` v roce, jen výchozí měna, včetně `tax_deductible=false`), `expenses_total`, `profit_total = revenue_total − expenses_total` (vše s DPH, stejně jako tržby).

**Integrace: kurzy ČNB, VIES, registr plátců DPH, parsery bankovních výpisů — rozhodnutí a odchylky:**
- Kurzy: `GET /api/exchange-rates?currency=&date=` (authed, ne pod účtem — data jsou globální) → `{currency, date, rate_date, rate}`; `rate` = CZK za **1 jednotku** (dělí se „množstvím“, např. 100 JPY → `0.13546`), decimální string s min. 3 des. místy (CZK → `1.000`). `date` default dnes (pražský kalendář), budoucí datum = dnešek. Víkend/svátek → kurz posledního vyhlášeného dne (`rate_date`). Neplatná měna/datum → 422 (`query.currency`/`query.date`), měnu ČNB nevyhlašuje → 404 `exchange rate not found`, ČNB nedostupná → 502.
- Cache `ExchangeRate` má navíc sloupec `list_date` (datum lístku ČNB); `date` = den, pro který kurz platí (unikátní `date+currency`). Pro minulý víkend/svátek se ukládají i řádky s `date` = požadovaný den → druhý dotaz už nejde na ČNB. Pro dnešek před vyhlášením (14:30) se mapování neukládá (zkusí se znovu). Kurz se ukládá tak, jak ho ČNB vyhlásila (`amount` + `rate` za `amount` jednotek). Služba: `cnb.Service.Rate(ctx, currency, date) (rate, rateDate, err)`, `api.Deps.CNB`; použití při tvorbě dokladu v cizí měně (§7.7) doplní faktury.
- VIES: `GET /api/vies/{vat_no}` → `{vat_no, country_code, valid, name, address}`; DIČ s prefixem státu, `GR` se převádí na `EL`, mezery/tečky se ignorují. Neznámý stát / špatný formát (i HTTP 400 z VIES) → 422 `path.vat_no`; `MS_UNAVAILABLE`, `TIMEOUT` apod. → 502. `name`/`address` jsou prázdné, když je členský stát nezveřejňuje (VIES vrací `---`); `address` má řádky oddělené `\n`. Cache 24 h v paměti procesu, jen definitivní odpovědi (chyby se necachují).
- Registr plátců DPH: `GET /api/vat-registry/{dic}` → `{vat_no, registered, reliable, unreliable_since?, name, address, street, city, zip, published_accounts: [{number, iban, published_on}]}`. Oproti §7.8 navíc `registered` (DIČ v registru není → `registered=false`, `reliable=null`, 200 — ne 404) a adresa. Přijímá jen česká DIČ (`CZ` volitelně + 8–10 číslic), jinak 422 `path.dic`. SOAP `getStatusNespolehlivyPlatceRozsireny` (endpoint `https://mojedane.gov.cz/dpr/axis2/services/rozhraniCRPDPH.rozhraniCRPDPHSOAP`); statusCode ≠ 0/1 (odstávka 0:00–0:10), SOAP fault nebo HTTP chyba → 502. České účty jako `předčíslí-číslo/banka` + dopočtený IBAN, nestandardní (zahraniční) účty jako zveřejněné číslo (IBAN, pokud je validní). `vatreg.Result.HasAccount(acc)` pro kontrolu „účet není zveřejněný“ u nákladů. Cache 24 h v paměti.
- Cache VIES/registru je per proces (restart ji smaže) — záměrně bez DB.
- `internal/bankimport` je čistý parser (bez DB/API): `Parse(format, data)`, `ParseAuto(data)`, `Detect(data)`; formáty `fio_json`, `gpc`, `fio_csv`, `csob_csv`, `kb_csv`, `airbank_csv` + `ParseCSV(data, CSVMapping, CSVOptions)` pro generické CSV (sloupce jménem bez ohledu na diakritiku/velikost písmen, alternativy `a|b`, nebo `#N`; buď `amount` se znaménkem, nebo `credit`/`debit`). Výstup `Statement{account, iban, currency, opening/closing balance?, transactions}`; `Transaction.amount` v haléřích se znaménkem (+ příchozí), `counterparty_account` jako `předčíslí-číslo/banka` nebo IBAN, symboly bez úvodních nul.
- Když zdroj nemá ID pohybu (novější CSV ČSOB, generické CSV), `external_id` = `h:` + hash obsahu řádku (+ pořadí shodných řádků v souboru) → opakovaný import téhož souboru dá stejná ID (deduplikace přes unikátní `external_id`).
- Kódování se detekuje automaticky: UTF-8 (i s BOM), jinak Windows-1250 vs. CP852 podle počtu českých znaků. Oddělovač CSV (`;` `,` TAB `|`) a desetinná čárka/tečka se detekují, lze je zadat.
- GPC: pozice dle dokumentace bank (ČSOB/Fio), podporuje 074/075/076/078/079, více výpisů v jednom souboru, storna (kód 4 = kladně, 5 = záporně), měnu z numerického kódu (neznámý → CZK), zahraniční platby (IBAN protistrany z 078). Číslo účtu se čte v „klientském“ pořadí číslic (interní ABO pořadí s přeházenými číslicemi, které některé banky nabízejí, se nepodporuje).
- CSV profily bank jsou dle veřejných ukázek/specifikací exportů: KB „export transakční historie“ (s hlavičkou) i MojeBanka Business „Klientský formát CSV“ (27 sloupců bez hlavičky; částka se čte se znaménkem — bez reálného vzorku ověřeno jen dle specifikace). Fixtures v `internal/bankimport/testdata` jsou syntetické ve formátu bank (GPC dle ČSOB/Fio specifikace, Air Bank dle hlavičky reálného exportu); ČNB a VIES fixtures jsou nahrané odpovědi reálných služeb, registr DPH = reálná odpověď + syntetické (nespolehlivý plátce, odstávka, fault) dle WSDL.
- Fio klient (`bankimport.NewFioClient`, interface `bankimport.Fio`): `Periods(token, from, to)`, `Last(token)`, `SetLastDate(token, date)`; base `https://fioapi.fio.cz/v1/rest`. Lokálně hlídá limit 1 požadavek / 30 s na token (`ErrFioRateLimited` bez volání Fio), HTTP 409 → `ErrFioRateLimited`, 500/401/403 → `ErrFioToken` (Fio vrací 500 s prázdným tělem pro neplatný token), 413 → `ErrFioTooMany`. Chyby nikdy neobsahují token (je v URL). Částky z JSON (Java double, i `1.0E7`) se převádějí přesně, bez floatu.

**Šablony, pravidelné faktury, plánovač, e-maily, upomínky (§7.3, §7.4) — rozhodnutí a odchylky:**
- `InvoiceTemplate` (tabulka `invoice_templates`): obsahová pole faktury + `name`, `document_type` (invoice|proforma, výchozí typ pro create-invoice). Řádky šablony jsou JSON sloupec (`[]TemplateLine`, nikdy se nedotazují samostatně, vždy se nahrazují celé). Prázdná/vynechaná pole = defaulty nové faktury v okamžiku vystavení (`currency`, `language`, `payment_method` prázdné → účet; `note`/`footer_note`/`round_total`/`due_days`/`vat_rate_bps` řádku vynechané → účet/subjekt). Výstup `Template` má `lines[].quantity` jako string.
- `Recurring` **odkazuje na šablonu** (`template_id`), neobsahuje obsah faktury sám (šablona je znovupoužitelná, úprava šablony se projeví v dalších fakturách). Smazání šablony používané pravidelnou fakturou → 409. Smazání subjektu použitého jen v šabloně blokované není — generování pak selže (422 v `last_error`).
- Rozvrh: `next_occurrence_on` = datum vystavení příští faktury. Den výskytu = `day_of_month` (1–31), jinak den `start_on`; vždy se ořízne na délku měsíce (31. 1. + 1 měsíc → 28./29. 2., další výskyt zase 31. 3. — počítá se od kotvícího dne, nedriftuje). S `day_of_month` je první výskyt první takový den ≥ `start_on`. `end_on` = poslední možné datum vystavení; po jeho překročení se pravidelná faktura sama deaktivuje. Navíc pole `last_run_at`, `last_error`.
- Plánovač (`internal/scheduler`): gorutina v `cmd/server`, běh hned po startu a pak každou hodinu, zastaví se zrušením kontextu při shutdownu. Job = `func(ctx, now) error`, volitelné `Every` (min. interval). Chyby a paniky jobů se logují a nezastaví ostatní joby.
- Generování (`recurring` job): pro každou aktivní s `next_occurrence_on ≤ dnes` v **jedné transakci na jeden výskyt**: zamknout řádek (`FOR UPDATE`, na SQLite serializováno), znovu ověřit aktivitu/termín, vytvořit fakturu (stejná cesta jako `POST /invoices` vč. číslování a skladu), posunout `next_occurrence_on`. Zameškaná období se dohánějí po jednom (max 1000 za běh), **každá faktura je vystavena k datu svého výskytu** (i zpětně) a placeholdery se počítají z tohoto data. Opakovaný běh nic nevytvoří (idempotence). Chyba (např. chybí číselná řada) → `last_error`, termín se neposune, zkusí se při dalším běhu.
- `POST /recurring/{id}/run-now` → 201 + faktura: vystaví hned s datem dnes (bez ohledu na `active` a termín) a posune `next_occurrence_on` o jednu periodu. `POST /recurring/{id}/activate` přeskočí výskyty zmeškané během deaktivace (posune `next_occurrence_on` na první výskyt ≥ dnes, nic zpětně nevygeneruje); aktivace po `end_on` → 409. `POST …/deactivate`. Obě vrací `Recurring`. PATCH změny `start_on`/`day_of_month` přepočítají `next_occurrence_on` (pokud ho neposíláš): bez vygenerované faktury od `start_on`, jinak jen posun dne v měsíci příštího výskytu.
- `POST /templates/{id}/create-invoice` (tělo volitelné `{issued_on?, document_type?}`) → 201 faktura. `POST /invoices/{id}/save-as-template` (tělo volitelné `{name?}`, default „<klient> <číslo>“) → 201 šablona; dobropis → 409. Obě cesty (i generování) mají roli editors.
- Placeholdery (v názvech řádků, `note`, `footer_note`, `order_number`, `private_note`; i ve výchozí poznámce účtu): `{MONTH}` (MM), `{MONTH_NAME}`, `{PREV_MONTH}`, `{PREV_MONTH_NAME}`, `{NEXT_MONTH}`, `{NEXT_MONTH_NAME}`, `{YEAR}`, `{PREV_YEAR}`, `{NEXT_YEAR}`, `{QUARTER}` (1–4), z data vystavení, jazyk šablony → účtu. Názvy měsíců cs/sk/en/de **jen v 1. pádě** (cs/sk malými: „za měsíc {MONTH_NAME}“ → „za měsíc září“; formulace typu „za {MONTH_NAME}“ by potřebovala 4. pád, ten nepodporujeme). Neznámé `{…}` zůstávají beze změny. Implementace `billing.RenderDatePlaceholders`, `billing.AddMonths`, `billing.FirstOccurrence`.
- E-mailová nastavení jsou na `Account` (embedded `AccountMailSettings`) a v `GET/PATCH /accounts/{slug}`: `email_reply_to` (prázdné → `email` účtu; neplatná adresa → 422), `email_signature` (připojí se pod text ze šablony), `email_templates` (výstup: všech 6 efektivních textů kind × cs/en s `custom`; PATCH: pole přepisů, **nahrazuje všechny přepisy**, prázdné nebo shodné s defaultem se neukládá, prázdný subject/body = default), `reminders_enabled`, `reminder_days_after_due` (default `[3,14,30]`, 1–365, seřadí se a deduplikuje), `paid_thanks_enabled` (default false). Per-account SMTP override zatím není (jen instance).
- `POST /invoices/{id}/send` `{to?, cc?, subject?, body?, attach_pdf=true, kind=invoice}` → 200 `EmailLog`. Bez `to` i `cc` = `client_email` + `email_copy` subjektu jako cc (více adres oddělených `,`/`;`). Žádný příjemce → 422 `body.to`; neplatná adresa → 422 `body.to[i]`. Chyba maileru → 502 a `EmailLog` s `error` (bez `sent_at`). `kind=invoice` úspěšně odeslaný → `open` faktura se označí `sent`. `subject`/`body` z požadavku se také renderují, ale podpis se k nim nepřidává (frontend předvyplní text z preview, který podpis obsahuje). `attach_isdoc=true` přiloží i ISDOC (`faktura-<number>.isdoc`, `application/xml`).
- Placeholdery e-mailů: `{number} {document} {document_title} {total} {remaining} {issued_on} {due_on} {days_overdue} {public_url} {account_name} {client_name} {vs} {iban} {bank_account} {payment_info}` (částky a data formátované jako v PDF v jazyce faktury; `{public_url}` = `{NANOFAKTURA_PUBLIC_URL}/p/{public_token}`; `{payment_info}` = řádky účet/IBAN/VS jen u platby převodem). Výchozí české texty se vyhýbají skloňování placeholderů.
- `EmailLog`: `id, account_id, invoice_id, kind, to[], cc[], subject, body, attachments[] (názvy), reminder_step, automatic, sent_at?, error, created_at`. `GET /invoices/{id}/emails` stránkovaně, nejnovější první. `GET /email-templates/preview?kind=&lang=&invoice_id=` → `{kind, lang, to, cc, subject, body}`; bez `invoice_id` s ukázkovými daty.
- Upomínky (`reminders` job): účty s `reminders_enabled`; doklady typu invoice/proforma ve stavu open/sent, `due_on < dnes`, s `client_email` a `total > paid_amount` (tj. ne cancelled/uncollectible/paid, ne dobropisy). Pošle se upomínka jen pro **nejvyšší dosažený krok** (zmeškané nižší kroky se neposílají); krok je vyřízený, když existuje úspěšný `EmailLog` s daným `reminder_step`; neúspěšný pokus se opakuje nejdřív po 24 h. Upomínka má PDF v příloze a neoznačuje fakturu jako odeslanou. Ruční `kind=reminder` má `reminder_step=0` a automatiku neovlivní.
- Poděkování za úhradu: `paid_thanks_enabled` → po `POST /payments`, které fakturu převede do `paid` (a jen pokud předtím zaplacená nebyla), se odešle `paid_thanks` bez přílohy (best effort; chyba jen v `EmailLog`, platbu neruší). Jiné cesty vzniku plateb (bankovní párování) musí zavolat `s.sendPaidThanks(ctx, invoiceID)` samy.
- Pravidelná faktura se `send_email` posílá po commitu výchozím příjemcům; bez e-mailu klienta vznikne `EmailLog` s chybou „no recipient“.

**Veřejný odkaz, ISDOC, exporty, DPH a přehledy (§7.5, §7.11, §7.14) — rozhodnutí a odchylky:**
- Veřejný odkaz: `GET /api/public/invoices/{token}` → `PublicInvoice` (jen obsah PDF: dodavatel vč. kontaktů účtu, odběratel bez e-mailu, data, platební údaje, `spayd` pro QR, řádky bez id, rekapitulace jen u plátce, součty, `related_number`); nikdy id, `subject_id`, `public_token`, `private_note`, `tags`, časy změn. `…/pdf` a `…/isdoc` jako u přihlášených. Token kratší než 24 nebo delší než 64 znaků → 404 bez dotazu do DB; lookup přes unique index. Zrušený doklad je dostupný s `cancelled: true` (bez QR). První zobrazení (kterýkoli ze tří endpointů) nastaví `Invoice.public_viewed_at` (i ve výstupu faktury); událost `public.viewed` se zapíše až s §7.9. Přegenerování tokenu `public_viewed_at` nemaže. Rate limiting zatím není (na reverse proxy).
- `auth.WithAccount(ctx, acc, "")` se používá i pro veřejné endpointy (účet dohledaný podle tokenu), aby šly použít account-scoped helpery (PDF s logem/razítkem, ISDOC).
- `GET /pdf-preview?template=&lang=&document_type=` (pod účtem, kdokoli z účtu) → ukázková data `pdf.Sample` s firemními údaji, logem a razítkem účtu; plátcovství podle účtu.
- ISDOC (`internal/isdoc`, `GET /invoices/{id}/isdoc`, `application/xml`, `attachment; filename="faktura-<number>.isdoc"`): validní vůči oficiálnímu `isdoc-invoice-6.0.2.xsd` (test přes `xmllint`, schéma v `internal/isdoc/testdata`). Typy: invoice → 1, proforma → 4, correction se záporným (nulovým) součtem → 2 (dobropis, **kladné částky i množství** dle pravidla A.6 ISDOC), correction s kladným součtem → 3 (vrubopis). UUID je deterministické UUID v5 z `account_id` + `invoice_id` (neukládá se; opakovaný export = stejný doklad). Cizí měna: částky bez `Curr` v CZK přepočtené kurzem dokladu (rekapitulace po sazbách, součty = součty rekapitulace), `*Curr` v měně dokladu. Přenesená daňová povinnost a neplátce: `Percent` 0, `TaxAmount` 0 (+ poznámka „Daň odvede zákazník“). `TaxPointDate` jen u typu 1 u plátce. `PaymentMeans` (kód 42) jen u převodu s účtem a kladnou částkou k úhradě, ne u dobropisu. Finální faktura k zaplacené proformě má `NonTaxedDeposits` a `PaidDepositsAmount`. Lokální RC kód (§92a) se neuvádí. Přílohou e-mailu přes `attach_isdoc`.
- Exporty: `GET /exports/{invoices|subjects|expenses}.{csv|xlsx}` se stejnými filtry jako seznamy (filtry vytaženy do `InvoiceFilter`/`SubjectFilter`/`ExpenseFilter` v `list_filters.go`, seznamy je používají také; hledání kontaktů má nově OR v závorce, takže se správně kombinuje s `type`). CSV: UTF-8 s BOM, `;`, částky s desetinnou čárkou bez oddělovače tisíců, data ISO `YYYY-MM-DD`, texty stavů/typů česky. XLSX (excelize, StreamWriter): číselné buňky s formátem `#,##0.00`, data jako datum `d.m.yyyy`, tučná barevná hlavička, ukotvený první řádek. Streamuje se po stránkách 500 řádků. `GET /exports/pdf.zip` bere celý `InvoiceFilter` (tedy i `since/until/document_type`) + `isdoc=true`; nejdřív se načtou jen id (max 5000, jinak 422 „narrow the period“), dokumenty se renderují a zapisují do ZIPu postupně; kolize názvů (stejné číslo u různých typů) dostane příponu `-<document_type>`. Chyba uprostřed streamu už nemůže změnit status (odpověď je useknutá).
- Role: exporty, ISDOC a náhled PDF smí všechny role (stejná data jako seznamy); reporty (`/reports/*`) jen owner, admin, accountant (member ne).
- Účet: nová pole `vat_period` (`month|quarter`, výchozí month), `c_ufo` (0–3 číslice), `c_pracufo` (0–4 číslice); v DB `vat_period`, `tax_office`, `tax_office_branch`. PATCH jako ostatní nastavení (owner/admin).
- `GET /reports/vat?period=YYYY-MM|YYYY-Qn` (bez `period` = předchozí měsíc/čtvrtletí dle `vat_period`; neplátce → 409; špatné období → 422 `query.period`). Zdroj: faktury a dobropisy s `taxable_fulfillment_due` v období, ne `cancelled`, vystavené jako plátce (`your_vat_mode`); náklady s `tax_deductible` a DUZP (bez DUZP datum vystavení) v období. Cizí měna přepočtená kurzem dokladu po sazbách. Proformy se nezapočítávají (daňové doklady k přijaté platbě nejsou). Klasifikace výstupů: RC + odběratel CZ → ř. 25 + A.1 (`kod_pred_pl` prázdný — na faktuře se neeviduje, doplní se v EPO); RC + odběratel z EU → ř. 21 (služby; dodání zboží ř. 20 se nerozlišuje); bez DPH + odběratel mimo EU → ř. 26; jinak ř. 1/2 a KH A.4 (odběratel s CZ DIČ a doklad > 10 000 Kč s DPH; dobropis se řídí součtem opravovaného dokladu) nebo A.5. Vstupy: dodavatel s CZ DIČ a nenulová DPH → ř. 40/41 (plný odpočet) a B.2 (> 10 000 Kč, evidenční číslo = `original_number`, jinak interní číslo) nebo B.3; dodavatel bez CZ DIČ se přeskočí s varováním (pořízení z EU/dovoz, B.1 a ř. 3–13/43–45 nejsou). Jiné sazby než 21/12 % a nulová sazba u tuzemského plnění → `warnings`. Výstup v haléřích (`reports.VatReturn` řádky r1, r2, r21, r25, r26, r40, r41, r46, r62–r65; `ControlStatement` a1, a4, a5, b2, b3).
- EPO XML: `GET /reports/vat/dphdp3.xml` a `/dphkh1.xml` (řádné podání, `verzePis` 03.01.03 / 03.01.14 dle popisu struktur z 9. 3. 2026; validní vůči `dphdp3_epo2.xsd`/`dphkh1_epo2.xsd` v `internal/reports/testdata`). DP3 v celých Kč (zaokrouhlení každého řádku, ř. 62–65 z zaokrouhlených řádků), KH na haléře. Bez `c_ufo` nebo CZ DIČ → 409. Typ subjektu z DIČ: 8 číslic = právnická osoba (`zkrobchjm`), jinak fyzická (jméno = první slovo názvu účtu, příjmení zbytek). Adresa: „ulice č.p./č.o.“ se dělí na `ulice`, `c_pop`, `c_orient`. KH se generuje za zvolené období (i čtvrtletí) — měsíční povinnost právnických osob hlídá uživatel.
- `GET /reports/overview?year=` → tržby/náklady/zisk po měsících (CZK, přepočet kurzem; tržby = invoice+correction mimo cancelled dle `issued_on`, náklady = všechny náklady dle `issued_on`), top 10 odběratelů podle součtu, průměrná doba úhrady (`paid_on − issued_on` zaplacených faktur vystavených v roce, 1 desetinné místo, `null` bez dat) a `income_tax` pro OSVČ v daňové evidenci: příjmy = platby přijaté v roce na faktury/dobropisy (platby proforem se nepočítají, započte je finální faktura), skutečné výdaje = platby daňově uznatelných nákladů v roce; u plátce bez DPH (poměrem základ/celkem). Paušály 80/60/40/30 % se stropy 1 600 000 / 1 200 000 / 800 000 / 600 000 Kč (platné od 2021, stále 2026).
- Typy výstupu reportů (`reports.VatReturn`, `ControlStatement`, `FlatRate`) jsou přímo z balíčku `internal/reports` (čistá doménová logika bez DB), ne duplicitní DTO v `internal/api`.
- Frontend (§7.5/§7.11): `/p/$token` bez app shellu, jazyk dle `language` dokladu (cs; sk → cs; ostatní → en), QR Platba se kreslí v prohlížeči ze `spayd` (knihovna `uqr`); logo dodavatele `PublicInvoice` nemá, zobrazují se iniciály. Varování DPH reportu vrací backend anglicky — frontend je překládá podle vzorů (`components/reports/vat-rows.ts`), neznámé zobrazí beze změny. Číselník `c_ufo`/`c_pracufo` je v `web/src/lib/tax-offices.ts` (číselník GFŘ platný od 1. 1. 2013; při změně upravit ručně). Exporty stahuje `fetch → blob` (spinner, čitelná chyba 422 u ZIPu).

**Banka (import, Fio sync, párování), kurzy při vytvoření dokladu, varování z registru DPH — rozhodnutí a odchylky:**
- `BankAccount` navíc `sync_provider` (`none|fio`, default `none`), `fio_token` (jen zápis: v create/PATCH, `""` v PATCH token smaže; v DB šifrovaně AES-256-GCM přes `internal/secret`, formát `v1:<base64url(nonce|ciphertext)>`; výstup má jen `has_fio_token`), `sync_from` (datum první synchronizace, default 30 dní zpět), `last_synced_at`. `sync_provider=fio` bez tokenu → 422 `body.fio_token`; token musí být 16–128 alfanumerických znaků. Smazání bankovního účtu smaže i jeho importované transakce (platby z nich vzniklé zůstávají).
- Import/sync/párování smí owner, admin i member (`auth.ForEditors` — zakládají platby, jako `POST /payments`); nastavení synchronizace je součástí bankovního účtu (owner/admin).
- `POST /bank-accounts/{id}/import` (multipart `file` max 10 MB + volitelné `format` `fio_json|gpc|fio_csv|csob_csv|kb_csv|airbank_csv`, prázdné = autodetekce) → 200 `{format, imported, duplicates, matched, suggestions}`. Neznámý formát → 422 `body.format`, nečitelný soubor → 422 `body.file`. Výpis jiného účtu (IBAN, resp. číslo účtu bez úvodních nul; kód banky jen pokud ho mají obě strany — GPC ho nenese) → 422 `body.file`. Generické CSV s mapováním sloupců zatím přes API nejde (jen parser v `bankimport`).
- `POST /bank-accounts/{id}/sync` (Fio `periods`): od dne před `last_synced_at` (překryv kvůli pozdě zaúčtovaným pohybům, duplicity se přeskočí), jinak od `sync_from`, jinak 30 dní zpět; do dneška. Nenastavený sync → 409; `ErrFioRateLimited` → 429 + `Retry-After: 30`; špatný token → 422; příliš mnoho pohybů → 422 (nastavit pozdější `sync_from`); jiná chyba Fio → 502. Nedešifrovatelný token (změněný klíč) → 422. Síťové volání běží mimo DB transakci.
- Plánovač: job `bank-sync` (`s.RunBankSync`, `Every: 2h`) synchronizuje všechny účty s `sync_provider=fio` postupně; limit 30 s na token hlídá Fio klient (sdílený mezi API a joby v `cmd/server`), účet narážející na limit se přeskočí do dalšího běhu (jen log), ostatní chyby se logují a vrací spojené.
- `BankTransaction` navíc `auto_matched`, `suggestions` (JSON, max 3: `{invoice_id|expense_id, number, name, remaining, score, reasons[]}`, snapshot v okamžiku párování) a `suggestion_count` (pro filtr). Výstup má odvozené `state`: `matched` (má `payment_id`), `ignored`, `suggested` (nespárováno s návrhy), `unmatched`. Filtr `?state=unmatched` zahrnuje i `suggested`. `external_id` je unikátní v rámci bankovního účtu; stejný výpis v jiném účtu je nezávislý.
- Párování (`internal/matching`, čistá funkce): příchozí → faktury a proformy `open|sent` (vč. po splatnosti) se `total > paid_amount` ve stejné měně; odchozí → náklady `open` se zbývající částkou. Body: VS přesně (bez úvodních nul) +50 „VS sedí“; částka = zbývající +40 „Částka sedí“, jinak = celková částka +20, jinak (při shodě VS a menší částce) +10 „Částečná úhrada“; účet protistrany = účet/IBAN kontaktu (u nákladu `supplier_bank_account/iban`) +30 (české číslo i IBAN se porovnávají přes IBAN); jméno (bez diakritiky, interpunkce, právních forem a titulů, pořadí slov nehraje roli, ≥ 50 % slov kratšího jména) +15. Návrh jen od 20 bodů. **Automaticky** jen když právě jeden kandidát má VS přesně a částku = zbývající; jinak se uloží návrhy. Při importu se po automatickém spárování sníží zbývající částka kandidáta v paměti (dvě stejné platby nezaplatí fakturu dvakrát). Dobropisy (záporné součty) se zatím automaticky nepárují, ručně ano.
- `POST /bank-transactions/{id}/match {invoice_id|expense_id}` (právě jedno, jinak 422) vytvoří platbu s `paid_on = booked_on` a **částkou transakce** (u faktury se znaménkem transakce, u nákladu opačným), tj. i částečnou úhradu nebo přeplatek; zruší `ignored`. Už spárovaná → 409, jiná měna → 422, cizí/neexistující doklad → 422, faktura cancelled/uncollectible → 409, nulová částka → 409. `unmatch` smaže vytvořenou platbu, přepočte stav dokladu a znovu spočítá návrhy (bez automatického spárování). Smazání platby přes `DELETE …/payments/{id}` transakci odpáruje. `ignore` spárované → 409. `POST /bank-transactions/rematch` → `{processed, matched, suggestions}` pro nespárované neignorované (s automatickým párováním). Seznam řazen `booked_on DESC, id DESC`; `query` hledá ve jméně/účtu protistrany, zprávě a VS.
- Platba z párování, která fakturu plně uhradí, pošle po commitu `paid_thanks` (pokud je zapnuté), stejně jako `POST /payments`.
- Kurzy (§7.7): při `POST /invoices` a `POST /expenses` v cizí měně bez `exchange_rate` se použije ČNB kurz ke dni DUZP (`taxable_fulfillment_due`), jinak `issued_on`, jinak dnes. Jen pokud je výchozí měna účtu CZK (ČNB kurzy jsou CZK za jednotku; jinak zůstává `1` a kurz je nutné zadat). Nedostupný/neznámý kurz → 422 `body.exchange_rate` s výzvou zadat kurz ručně (ne 502). Duplikace, dobropisy a finální faktury kurz přebírají z originálu. Kurz se zjišťuje před otevřením transakce (ČNB cache používá vlastní DB spojení).
- Registr DPH (§7.8): `Expense.warnings` (jen `GET /expenses/{id}`, jinak pole chybí): „Dodavatel je nespolehlivý plátce DPH (od …)“ a „Bankovní účet … není zveřejněný v registru plátců DPH“ (jen registrovaný plátce, český DIČ — prefix `CZ`, nebo číslice u dodavatele z CZ/bez země — a náklad s `supplier_bank_account` nebo `supplier_iban`). Timeout 3 s, chyba registru = žádné varování (log). Texty varování jsou česky (zobrazují se přímo). `GET /subjects/{id}/vat-status` → `VatRegistryResult` pro DIČ kontaktu; bez českého DIČ → 422, registr nedostupný → 502.

**[frontend] Tým, pozvánky, vzhled dokladů (§7.13, §7.14) — rozhodnutí a odchylky:**
- Nastavení → „Tým“ (`/a/$slug/settings/members`) a „Vzhled dokladů“ (`/a/$slug/settings/appearance`). Pravidla, která UI hlídá předem (zdrojem pravdy zůstává 403/409 backendu), jsou v `components/team/permissions.ts` (+ testy): admin nespravuje ownery, poslední owner nejde degradovat/odebrat ani odejít, opustit účet smí každý. Chyby týmu se překládají do češtiny podle `code` (`codeMessages` v `api/errors.ts`).
- `Invitation` vrací `invite_url` (token je v DB navíc šifrovaně přes `secret.Box`, `invitations.token_enc`; pozvánky z doby před touto změnou mají `invite_url` prázdné) a `invited_by_name`. UI nabízí „Kopírovat odkaz“ a „Poslat znovu“ = `POST /invitations/{id}/resend` (stejný odkaz znovu e-mailem, platnost se prodlouží o 7 dní; pozvánka bez uloženého odkazu dostane nový).
- Veřejná stránka `/invite/$token`: 404/410 mají vlastní stavy; přihlášený se stejným e-mailem → „Přijmout“ (`POST …/accept`, pak refetch `/auth/me` a přechod na `/a/{slug}/dashboard`); jiný e-mail → vysvětlení + odhlášení; nepřihlášený: `user_exists` → `/login?redirect=/invite/$token` (e-mail se na loginu předvyplní z `GET /api/invitations/{token}`, ne z URL), jinak registrační formulář přímo na stránce pozvánky (`invitation_token`, bez `account_name`). Registrace z pozvánky funguje i při vypnuté registraci.
- Logo/razítko: nahrání `POST /attachments` (`owner_type=account`, `owner_id=0`) → `PATCH` účtu → smazání předchozí přílohy (best-effort); při chybě PATCH se nová příloha smaže. Klient povoluje jen PNG/JPEG do 5 MB (PDF jiné neumí).
- Náhled PDF: `GET /pdf-preview` jako blob v iframe (desktop), na mobilu odkaz do nové záložky. Vzhled se ukládá na `Account`: `pdf_template` (`classic|modern|minimal`), `pdf_accent` (`#RRGGBB`, prázdné = barva šablony), `pdf_show_qr` (default true; v DB `pdf_hide_qr`), `pdf_footer` (text za zápisem v rejstříku v patičce každé strany). Platí pro všechna PDF (detail, e-maily, veřejný odkaz, ZIP export); `?template=` u `GET /invoices/{id}/pdf` má přednost. Náhled přijímá neuložené `accent`, `show_qr`, `footer` v query (prázdná hodnota = uložená, tj. „vrátit na výchozí barvu“ se v náhledu projeví až po uložení).

**Události, úkoly, webhooky, globální hledání (§7.9, §7.10, §7.15) — rozhodnutí a odchylky:**
- `events.Record(tx, meta, event)` (balíček `internal/events`) se volá **v transakci změny**; ve stejné transakci zakládá `WebhookDelivery` pro odpovídající aktivní webhooky. Katalog názvů: `GET /events/catalog`. Navíc oproti SPEC: `invoice.updated|deleted|cancelled|cancel_undone|uncollectible|uncollectible_undone|locked|unlocked|public_link_regenerated`, `payment.deleted`, `email.failed`, `expense.updated|deleted|locked|unlocked|paid`, `expense_payment.created|deleted`, `subject.*`, `price_item.*`, `stock.moved` (ruční pohyb), `recurring.failed`, `bank.imported|unmatched`, `webhook.failed|disabled`. Payload/`data` má klíčová pole dokladu. `user_id` je prázdné u plánovače a veřejného odkazu. Timeline detailu = `GET /events?subject_type=invoice&subject_id=` (samostatně, ne ve výstupu faktury). Filtry: `name` (přesně, nebo prefix `invoice`/`invoice.*`), `subject_type`+`subject_id`, `since` (RFC 3339 nebo datum); nejnovější první.
- `invoice.overdue` zapisuje job `todos` (hodinově) **jednou za fakturu** (existuje-li už událost, znovu se nezapíše). `stock.low` se zapíše při každém propadu pod minimum (přechod úkolu do otevřeného stavu), ne při každém pohybu. `public.viewed` jen při prvním zobrazení.
- `Todo` navíc `key` (unikátní v účtu, jen automatické: `invoice.overdue:<id>`, `bank.suggested:<id>`, `stock.low:<id>`, `recurring.failed:<id>`), `auto_completed`, `user_id`. Výstup: `automatic`, `completed`. Automatické úkoly se přepočítávají při každé události svého záznamu a jobem `todos`; vyřešené se samy dokončí, znovu vzniklý stav je znovu otevře (jen pokud je dokončil systém; ručně dokončené zůstávají). Smazaný záznam → jeho automatický úkol se smaže. Automatický úkol nejde upravit ani smazat (409), jen dokončit/otevřít. `DELETE /todos/{id}` navíc pro ruční. Varování z registru DPH u nákladů úkol **nevytváří** (vyžadovalo by síťové dotazy).
- Webhooky: jen owner/admin (i čtení). Secret `whsec_…` šifrovaně (`secret.Box`), vrací se jen při vytvoření a `PATCH {rotate_secret: true}`. Filtr `events`: přesné názvy, `prefix.*`, `*` (neznámé → 422). Payload `{id (události), event, created_at, account (slug), subject {type,id}, text, data}`, hlavičky `X-NanoFaktura-Event|Delivery|Signature`. Pokusy: hned, pak +1 min/5 min/30 min/2 h/12 h (celkem 6), pak `failed` + událost `webhook.failed`; 20 neúspěšných doručení za sebou → webhook se vypne (`disabled_at`, `webhook.disabled`), `PATCH active=true` čítač nuluje. Webhook nedostává `webhook.*` události o sobě samém. Přesměrování se nesledují (3xx = neúspěch). SSRF: URL i připojení (kontrola IP po DNS v dialeru) odmítá privátní/loopback/link-local/CGNAT adresy, pokud není `NANOFAKTURA_WEBHOOKS_ALLOW_PRIVATE=true`. `POST /webhooks/{id}/test` pošle synchronně `ping` (zaloguje se jako doručení bez opakování). `POST …/deliveries/{id}/redeliver` založí kopii se stejným payloadem a hned ji zkusí (201). Plánovač nyní tiká každou minutu (job `webhooks` každý tik, ostatní mají `Every`); běží sekvenčně, takže dlouhý job (bank-sync) doručení zdrží.
- Globální hledání `GET /search?q=&limit=5` → `{invoices, expenses, subjects, price_items}` s `{type, id, title, subtitle, status, url_hint}` (`/a/{slug}/invoices/{id}` …). Bez ohledu na velikost písmen a diakritiku na SQLite i Postgres přes sloupec `search_text` (normalizovaný text, plní GORM hook `BeforeSave`, existující řádky doplní `db.Migrate`); hledání `LIKE %…%` v rámci indexu `account_id` (bez fulltextu).

**Úklidová vlna backlogu (backend) — rozhodnutí a odchylky:**
- Kódy chyb viz §2. VAT report: `warnings` jsou objekty `{code, document, message, params}` (`unsupported_rate` {rate}, `missing_taxable_date`, `possible_reverse_charge`, `reverse_charge_import`, `reverse_charge_subject_code`, `ec_sales_list`, `correction_of_cancelled`, `control_statement_monthly`, `reverse_charge_no_dic`, `eu_reverse_charge_no_vat`, `zero_rate_not_reported` {amount}, `supplier_no_dic`, `calculation_error`); DIČ v `control.a1/a4/b2` JSONu je s prefixem `CZ` (EPO XML bez něj).
- **Identifikovaná osoba** účtuje tuzemská plnění bez DPH: sazby se vynutí na 0 jako u neplátce (`billing.ChargesNoVAT(vatMode, reverseCharge)`, zrcadlo `chargesNoVat` ve `web/src/components/invoice/calc.ts`); u přenesené daňové povinnosti (služby do EU) sazby zůstávají pro zobrazení, DPH je 0. Platí i pro ISDOC a součty šablon.
- Faktura v měně bez bankovního účtu v té měně (a bez `bank_account_id`) zůstává **bez platebních údajů** (žádný fallback na účet v jiné měně — číslo účtu v jiné měně by klienta mátlo). `Invoice.warnings[]` (detail i odpověď create/patch) pak obsahuje `{code: "no_bank_account"}` (jen `payment_method=bank`, ne u dobropisu); frontend po vystavení ukáže varování.
- Mazání: faktura, na kterou odkazuje jiný doklad přes `related_id` (dobropis, vyúčtování zálohy) → 409 `referenced`. Kontakt použitý v šabloně → 409 `used_by_template`; náklady kontaktu se při smazání odpojí (`subject_id` → null, snapshot dodavatele zůstává). Smazání faktury/nákladu/kontaktu smaže jeho přílohy: řádky v transakci, soubory až po commitu (best-effort, chyba jen log).
- Změna hesla (`PATCH /auth/me`) smaže všechny ostatní sessions uživatele, aktuální zůstává. API tokeny zůstávají platné. Špatné současné heslo → 422 `wrong_password`.
- Seznamy `GET /invoices` a `GET /expenses` vrací navíc `sums: [{currency, count, sum_total, sum_remaining}]` za celý filtr (všechny stránky; `sum_remaining` bez stornovaných/nedobytných). `status` přijímá více hodnot oddělených čárkou a pseudo-stav `unpaid` (faktury open+sent+overdue, náklady open+overdue); neznámý stav → 422. `amount=` hledá shodu s `total` nebo zbývající částkou (ruční párování). Faktury navíc `recurring_id=` — generované faktury mají `recurring_id` (starší ne). Exporty sdílejí filtr, sumy nevrací.
- `BankTransaction` navíc `matched_number` a `matched_name` (u nákladu původní číslo dodavatele, je-li). `BankAccount` navíc `balance` + `balance_on` — konečný zůstatek posledního importovaného výpisu, který ho nese (Fio JSON/API, GPC), k poslednímu dni pohybů výpisu; starší výpis novější zůstatek nepřepíše.
- `Template` navíc `subject_name`, `total`, `total_currency` (součet faktury vystavené teď — výchozí sazba a zaokrouhlení účtu); `Recurring` navíc `template_name`, `subject_id`, `subject_name`, `total`, `total_currency`.
- `ExpenseCreate.due_days` (když chybí `due_on`; default výchozí splatnost účtu). `ExpensePatch.clear_subject: true` odpojí dodavatele (s `subject_id` zároveň → 422). `SubjectPatch.clear_due_days: true` vrátí splatnost na výchozí účtu. (JSON `null` u pointeru huma od vynechaného pole neodliší, proto explicitní příznaky.) Detail faktury i nákladu vrací `attachments[]` (stejný tvar jako `GET /attachments`).
- `Account` navíc `onboarded_at` (PATCH `onboarded: true|false`; frontend podle něj nabízí průvodce místo localStorage) a `capabilities {edit, manage_settings, manage_members, manage_owners, view_reports, export}` odvozené z role (vynucuje je dál backend per operace). `default_language` a jazyk faktur/šablon nově `cs|en|sk|de` (PDF umí všechny; e-maily jen cs/en — sk dostane české, de anglické texty).
- ARES timeout → 504 `upstream_timeout` (ostatní výpadky 502). Počáteční skladový pohyb má poznámku „Počáteční stav“.
- `GET /email-templates/preview` přijímá neuložené `subject`/`body` (vykreslí se jako uložená šablona vč. podpisu); `GET /email-templates/defaults` vrací vestavěné texty (`[{kind, lang, subject, body}]`) pro „Obnovit výchozí“.
- Veřejná faktura má `logo_url` (jen s logem) → `GET /api/public/invoices/{token}/logo` (bez přihlášení, jen přes token faktury).
- Neřešeno (zůstává v backlogu): SMTP nastavení per účet, generické CSV s mapováním sloupců přes API.

**Záloha, export a import účtu (§7.16) — rozhodnutí a odchylky:**
- Balíček `internal/backup` (`Export`, `Import`, `RecordExported`); API (`internal/api/backup.go`) i CLI (`cmd/server/backup.go`) volají stejný kód. Exportní DTO (`internal/backup/dto.go`) mají Go názvy polí shodné s modely (převod reflexí podle jména), JSON snake_case. Test `TestEveryModelIsCovered` selže pro model v `model.All()` bez zařazení (soubor zálohy nebo výjimka s důvodem), `TestDTOsCoverModelFields` pro pole modelu, které DTO nenese (výjimky: `account_id`, `search_text`, tajemství, `public_token`, `storage_key`, `user_id` událostí/úkolů, ID rodiče u vnořených záznamů).
- Nezálohuje se: `User` (jen `members.json` informativně), `Session`, `APIToken`, `Invitation`, `ExchangeRate` (globální cache), `WebhookDelivery` (fronta staré instance). `user_id` událostí a úkolů se po importu vynuluje (uživatelé se nepřenáší).
- ZIP: nejdřív se v jedné čtecí transakci načtou všechny záznamy (konzistentní snímek, DB se nedrží během streamování), pak se streamují JSONy, přílohy ze storage, `attachments.json` a nakonec `manifest.json` (obsahuje SHA-256 a velikost každého jiného souboru). Příloha, jejíž soubor ve storage chybí, se exportuje bez obsahu (`path: ""`) a při importu se vynechá (upozornění `attachments_missing`). Název souboru v ZIPu je očištěný (`attachments/<id>/<jméno>` bez oddělovačů).
- `manifest.version` > 1 → 422 `unsupported_backup_version`; chybějící/jiný `format`, nesedící kontrolní součet, chybějící soubor z manifestu, nebezpečná cesta (absolutní, `..`, `\`, `:`) nebo odkaz na neexistující povinný záznam (kontakt faktury/šablony, šablona recurring) → 422 `corrupt_backup`; součet rozbalených velikostí (a skutečně přečtená data) nad limit → 413 `backup_too_large` (per-entry: manifest 16 MB, příloha 64 MB). Soubory mimo manifest se ignorují.
- Odkazy se přemapují výhradně přes ID ze zálohy — odkaz na ID, které v záloze není, se nikdy nevyhodnotí proti DB (nelze sáhnout na jiný účet): nepovinné vazby → `null`, povinné → `corrupt_backup`; osiřelé pohyby skladu, bankovní pohyby a e-mailové logy se vynechají (upozornění `orphans_skipped`); automatické úkoly dostanou klíč s novým ID. Události o smazaných záznamech mají po importu `subject_id` 0. Data událostí (`data`) se nepřepisují (mohou obsahovat stará ID).
- Import: nový slug z `name` (nebo názvu ze zálohy), volající = jediný owner, `created_at`/`updated_at` záznamů zůstávají; chybí-li v záloze číselná řada typu dokladu, založí se výchozí. Změny s `warnings[]` (`{code, message, count}`): `recurring_deactivated`, `reminders_disabled`, `paid_thanks_disabled`, `webhooks_inactive`, `bank_tokens_removed` (sync_provider zůstane, token chybí → sync se přeskakuje), `public_links_regenerated`, `members_not_imported`, `attachments_missing`, `orphans_skipped`. Zapnutí webhooku bez secretu (`PATCH active=true`) vygeneruje nový secret a vrátí ho v odpovědi.
- Odchylka: soubory příloh se zapisují do storage **na konci importní transakce** (ne až po commitu) — při chybě zápisu se transakce vrátí, při chybě transakce/commitu se zapsané soubory smažou. Výsledek je „vše nebo nic“ i pro soubory; cenou je delší transakce u velkých záloh (SQLite je po tu dobu blokované).
- Nevytváří se události pro jednotlivé záznamy; jen `account.imported` (data: `source_slug`, `exported_at`, `app_version`, `counts`) a při stažení `account.exported` (po úspěšném dokončení streamu; při chybě uprostřed dostane klient useknutý ZIP a chyba se jen zaloguje). `app_version` = verze modulu nebo VCS revize z build info (`dev`).
- CLI: `nanofaktura backup export --account <slug> [--out soubor.zip|-]` (nepřepíše existující soubor; událost bez uživatele) a `nanofaktura backup import --owner <email> [--name …] soubor.zip` (uživatel musí existovat). Bez argumentů (nebo `serve`) běží server jako dřív.
- `POST /api/accounts/import` je v `authed` (vytvořit účet smí každý přihlášený, stejně jako `POST /api/accounts`); `GET …/backup` jen owner/admin.

