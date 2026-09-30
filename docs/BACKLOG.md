# Backlog — nalezené mezery a dluh

Položky nalezené během implementace, určené k vyřešení v úklidové vlně.

## Backend
- [x] `SubjectPatch.due_days` nejde vrátit na NULL (výchozí splatnost účtu) — potřeba nullable PATCH semantika.
- [x] 409 odpovědi nemají strojově čitelný důvod (`has_invoices`, `is_default`, `last_of_type` …) — frontend rozlišuje jen podle anglického `detail`. Zavést `code` v problem+json (huma error model extension) a použít všude.
- [x] Příznak dokončeného onboardingu na účtu (`onboarded_at`) místo localStorage.
- [x] ARES timeout → 504 (teď 502).
- [x] Smazání dokladu, na který odkazuje jiný přes `related_id`, není blokováno.
- [x] Smazání faktury/kontaktu nemaže přílohy.
- [x] Změna hesla neinvaliduje ostatní sessions.
- [x] Seznam faktur: součty za celý filtr (`sum_total`, `sum_remaining`) v odpovědi seznamu.
- [x] Filtr `status=unpaid` (open+sent+overdue) — pro záložku „Neuhrazené“; obecně víc stavů najednou.
- [x] **`identified_person`**: identifikovaná osoba tuzemsky DPH neúčtuje → backend musí vynutit sazbu 0 jako u neplátce (PDF už to tak zobrazuje, výpočet ne). Opravit v billing/invoices + frontend calc.
- [ ] Rozhodnout: platby na zamčené faktuře jsou povolené (zámek blokuje jen PATCH/DELETE) — ponechat, zdokumentovat v UI. *(backend beze změny, zůstává na UI)*
- [x] Seznam nákladů: součty za filtr (per měna) — stejně jako u faktur.
- [x] Náklad: PATCH nejde odpojit dodavatele (`subject_id` → null) při přechodu na volný text.
- [x] Počáteční skladový pohyb má anglickou poznámku „initial stock“ → česky / prázdná.
- [x] Chybové `detail` texty jsou anglicky a propadají k uživateli — zavést `code` (viz 409) a překládat na frontendu, nebo lokalizovat.
- [x] Output nákladu bez `attachments[]` (SPEC §7.2) — sjednotit s fakturou.
- [x] `ExpenseCreate` bez `due_days`.
- [ ] Generické CSV s mapováním sloupců přes API (parser existuje). *(odloženo: vyžaduje návrh UI mapování)*
- [x] Mazání kontaktu použitého jen v šabloně není blokováno → recurring selže.
- [x] Account: uložit PDF šablonu, barvu akcentu, zobrazení QR, vlastní patičku (§7.14) + použít v PDF; `default_language` rozšířit na cs/en/sk/de (PDF to umí).
- [x] Pozvánky: vracet odkaz (pro kopírování) a jméno zvoucího; „Poslat znovu“ bez zneplatnění.
- [x] `GET /email-templates/preview` přijímat `subject`/`body` override + endpoint s výchozími texty (tlačítko „Obnovit výchozí“).
- [x] Output `Template`/`Recurring` bez jména odběratele a součtu → frontend dotahuje zvlášť.
- [x] Filtr faktur podle `recurring_id` (historie pravidelné faktury).
- [x] `BankTransaction` vracet číslo a název spárovaného dokladu (teď N+1 dotazů z frontendu).
- [x] Seznamy faktur/nákladů: filtr podle částky a „jen neuhrazené“ (ruční párování).
- [x] Zůstatek bankovního účtu z výpisů (opening/closing balance).
- [x] Veřejný DTO faktury: URL loga dodavatele (veřejně dostupný endpoint pro logo přes token).
- [x] Varování VAT reportu anglicky → `code` + česky; DIČ v KH A.4/B.2 zobrazovat s prefixem CZ v JSON (XML dle schématu bez).
- [x] Faktura v měně bez bankovního účtu v té měně → žádné platební údaje; zvážit fallback (výchozí účet s IBAN) nebo varování při vystavení.
- [x] `Me`/account výstup: capability flagy (`can_view_reports`, `can_manage_settings` …) místo odvozování rolí na frontendu.
- [ ] Webhook test ping neaktualizuje `last_status`/`last_delivered_at`.
- [ ] Hledání: `search_text` faktury neobsahuje typ dokladu („faktura“, „zálohová“…).
- [ ] Filtr událostí: víc prefixů najednou (`expense.*,expense_payment.*`); úkoly bez vazby (`related_type=none`).
- [ ] Per-account SMTP nastavení; text pozvánky jen česky. *(odloženo: šifrované SMTP heslo + test připojení, samostatná vlna)*

## Dev
- [ ] Agenti sdílí Chrome i cookie `nf_session` na `localhost` — pro paralelní vizuální testy používat `127.0.0.1`/různé porty; procesy ukončovat podle PID, ne `pkill -f`.
