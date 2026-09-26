# NanoFaktura — web

SPA pro NanoFakturu: React 19 + Vite + TypeScript (strict), Tailwind v4, shadcn/ui (Base UI),
TanStack Router (file-based) + TanStack Query, React Hook Form + Zod, openapi-fetch, PWA.
Závazný kontrakt je v [`docs/SPEC.md`](../docs/SPEC.md) (hlavně §6 Frontend).

```bash
npm install
npm run dev        # Vite na :5173, /api → http://localhost:8080 (backend: make dev-backend)
npm run build      # tsr generate && tsc -b && vite build → dist/
npm run lint       # ESLint
npm run typecheck  # tsr generate && tsc -b
npm test           # Vitest (src/**/*.test.ts)
make gen-types     # (z kořene repa) přegeneruje src/api/schema.gen.ts z OpenAPI backendu
```

## Struktura

```
src/
  main.tsx               providery (Theme, Query, Tooltip, Router, Toaster), 401 handler
  router.tsx             createRouter + typ kontextu { queryClient }
  routeTree.gen.ts       GENEROVANÉ (TanStack Router plugin / `npm run routes`) — commitovat, needitovat
  routes/                stránky, file-based routing (viz níže)
  api/
    schema.gen.ts        GENEROVANÉ openapi-typescript (`make gen-types`) — nikdy needitovat
    types.ts             doménové typy odvozené z `paths` (Me, Account, …) — importujte odsud
    client.ts            `api` (openapi-fetch) + `unwrap()` → data nebo ApiError
    errors.ts            ApiError, errorMessage(), applyProblemToForm() (422 → pole formuláře)
    query-client.ts      QueryClient + globální toasty chyb
    queries/keys.ts      konvence query klíčů
    queries/<zdroj>.ts   queryOptions + mutační hooky pro jeden zdroj
  components/
    ui/                  shadcn komponenty (přidávat `npx shadcn@latest add …`)
    layout/              AppShell, AppSidebar (desktop), BottomTabBar + „Více“ (mobil), nav.ts
    form/fields.tsx      TextField / TextareaField / SelectField / SwitchField (RHF ↔ shadcn Field)
    page-header.tsx      PageHeader (+ PageAction), PageBody
    responsive-dialog.tsx, responsive-list.tsx, sticky-action-bar.tsx, empty-state.tsx,
    skeletons.tsx, page-states.tsx, settings-page.tsx, button-link.tsx, …
  hooks/                 useIsMobile, useCurrentAccount, useThemeChoice
  lib/                   money.ts, quantity.ts, date.ts, validation.ts (+ *.test.ts)
```

## Routy

| Soubor | URL |
|---|---|
| `routes/login.tsx`, `register.tsx` | veřejné |
| `routes/index.tsx` | `/` → první účet (`/a/$slug/dashboard`) nebo `/login` |
| `routes/a/$slug/route.tsx` | layout přihlášené části + **auth guard** (`beforeLoad`: `/api/auth/me` + členství v účtu) |
| `routes/a/$slug/invoices/index.tsx` | `/a/$slug/invoices` (typované search params) |
| `routes/a/$slug/invoices/$invoiceId/index.tsx` | detail (`params.parse` → číslo) |
| `routes/a/$slug/settings/route.tsx` | layout nastavení (záložky na desktopu, seznam na mobilu) |

## Jak přidat stránku

1. Vytvořte soubor v `src/routes/…` — plugin při `npm run dev` sám doplní kostru a přegeneruje `routeTree.gen.ts`.
2. Data: `loader: ({ context, params }) => context.queryClient.prefetchQuery(xxxQueries.detail(…))`
   a v komponentě `useQuery(xxxQueries.detail(…))` (skeleton při `isPending`, `PageError` při chybě).
3. Filtry seznamů patří do URL: `validateSearch: z.object({ … .optional().catch(undefined) })`, číst `Route.useSearch()`,
   měnit `navigate({ search: (prev) => ({ ...prev, status }) })`.
4. Kostra stránky:
   ```tsx
   <PageHeader title="Faktury" back={{ to: '/a/$slug/invoices', params: { slug } }} actions={[…]} />
   <PageBody>…</PageBody>
   ```
   Podstránka nastavení: `<SettingsPage title=…>` + `<FormSection>`.
5. Navigační položky přidávejte jen do `components/layout/nav.ts`.

## Jak přidat API zdroj (queries)

- Po změně backendu `make gen-types`, pak doplňte typy do `api/types.ts` (přes `ResponseBody<'/api/…', 'get'>`).
- Klíče do `api/queries/keys.ts` — vše pro účet začíná `keys.account(slug)` = `['a', slug]`.
- `api/queries/<zdroj>.ts`: objekt `xxxQueries` s `queryOptions(...)` (použitelný v loaderu i v komponentě)
  a `useCreateXxx` / `useUpdateXxx` / … mutace, které po úspěchu invalidují/aktualizují cache.
- Volání vždy `unwrap(api.GET('/api/…', { params: { path: { slug } } }))`.
- Chyby: globálně se toastují (mutace vždy, query jen při chybě refetche). Formulář, který chyby řeší sám
  (`applyProblemToForm` → pole, `setError('root')`), dá mutaci `meta: { silent: true }`.
  401 → automaticky redirect na `/login?redirect=…`.

## Mobile-first pravidla (povinné, SPEC §6)

- Navrhujte od 360 px, desktop přidávejte přes `md:` / `lg:`. Žádný horizontální scroll stránky.
- Touch targety ≥ 44 px: `Button`, `Input`, `SelectTrigger` jsou na mobilu `h-11`, na desktopu kompaktní (`md:h-8`).
- Pole: správný `type` / `inputMode` / `autoComplete` / `enterKeyHint` — `inputMode="numeric"` pro IČO a PSČ,
  `"decimal"` pro částky a množství, `type="email"` / `"tel"`, nativní `type="date"`.
- Formuláře: jeden sloupec na mobilu (`grid sm:grid-cols-2`), primární akce v `<StickyActionBar>`.
- Seznamy: `<ResponsiveList>` (karty < md, tabulka ≥ md), filtry na mobilu v Draweru, stránkování „Načíst další“.
- Dialogy: `<ResponsiveDialog>` (Drawer na mobilu, Dialog na desktopu). Akce stránky přes `PageHeader.actions`
  (na mobilu `primary` akce s ikonou v hlavičce, ostatní v Drawer menu „…“).
- Safe area: utility `pt-safe` / `pb-safe` / `px-safe`; tab bar a sticky lišta to řeší samy.
  Při otevřené klávesnici je na `<html>` `data-keyboard="open"` (varianta `keyboard-open:`).
- Ověřujte na 390×844 i 1440×900, ve světlém i tmavém režimu.

## Konvence

- Peníze jsou vždy `number` v haléřích (celé číslo). Zobrazení `formatMoney(amount, currency)`,
  vstup `parseMoney("1 234,50") → 123450`. Množství je string (`"1.5"`) — `parseQuantity`, `formatQuantity`.
  Data `YYYY-MM-DD` — `formatDate`, `todayISO`, `addDays` (`lib/date.ts`). Nikdy nepočítejte peníze ve floatu.
- UI texty česky, kód a identifikátory anglicky.
- Barvy jen přes tokeny (`bg-primary`, `text-muted-foreground`, `text-success`, `bg-warning` …) z `src/index.css`.
- Odkaz vzhledu tlačítka: `<ButtonLink to=… params=…>`; tlačítko renderující odkaz: `<Button nativeButton={false} render={<Link …/>}>`.
