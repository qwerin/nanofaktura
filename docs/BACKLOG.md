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
- [ ] Per-account SMTP nastavení; text pozvánky jen česky.

## Dev
- [ ] Agenti sdílí Chrome i cookie `nf_session` na `localhost` — pro paralelní vizuální testy používat `127.0.0.1`/různé porty; procesy ukončovat podle PID, ne `pkill -f`.
