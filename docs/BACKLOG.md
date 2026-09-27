# Backlog — nalezené mezery a dluh

Položky nalezené během implementace, určené k vyřešení v úklidové vlně.

## Backend
- [ ] `SubjectPatch.due_days` nejde vrátit na NULL (výchozí splatnost účtu) — potřeba nullable PATCH semantika.
- [ ] 409 odpovědi nemají strojově čitelný důvod (`has_invoices`, `is_default`, `last_of_type` …) — frontend rozlišuje jen podle anglického `detail`. Zavést `code` v problem+json (huma error model extension) a použít všude.
- [ ] Příznak dokončeného onboardingu na účtu (`onboarded_at`) místo localStorage.
- [ ] ARES timeout → 504 (teď 502).
- [ ] Smazání dokladu, na který odkazuje jiný přes `related_id`, není blokováno.
- [ ] Smazání faktury/kontaktu nemaže přílohy.
- [ ] Změna hesla neinvaliduje ostatní sessions.
- [ ] Seznam faktur: součty za celý filtr (`sum_total`, `sum_remaining`) v odpovědi seznamu.
- [ ] Filtr `status=unpaid` (open+sent+overdue) — pro záložku „Neuhrazené“; obecně víc stavů najednou.
- [ ] **`identified_person`**: identifikovaná osoba tuzemsky DPH neúčtuje → backend musí vynutit sazbu 0 jako u neplátce (PDF už to tak zobrazuje, výpočet ne). Opravit v billing/invoices + frontend calc.
- [ ] Rozhodnout: platby na zamčené faktuře jsou povolené (zámek blokuje jen PATCH/DELETE) — ponechat, zdokumentovat v UI.
- [ ] Seznam nákladů: součty za filtr (per měna) — stejně jako u faktur.
- [ ] Náklad: PATCH nejde odpojit dodavatele (`subject_id` → null) při přechodu na volný text.
- [ ] Počáteční skladový pohyb má anglickou poznámku „initial stock“ → česky / prázdná.
- [ ] Chybové `detail` texty jsou anglicky a propadají k uživateli — zavést `code` (viz 409) a překládat na frontendu, nebo lokalizovat.
- [ ] Output nákladu bez `attachments[]` (SPEC §7.2) — sjednotit s fakturou.
- [ ] `ExpenseCreate` bez `due_days`.
- [ ] Generické CSV s mapováním sloupců přes API (parser existuje).
- [ ] Mazání kontaktu použitého jen v šabloně není blokováno → recurring selže.
- [ ] Per-account SMTP nastavení; text pozvánky jen česky.

## Dev
- [ ] Agenti sdílí Chrome i cookie `nf_session` na `localhost` — pro paralelní vizuální testy používat `127.0.0.1`/různé porty; procesy ukončovat podle PID, ne `pkill -f`.
