// msw server pro komponentové testy. Výchozí handlery pokrývají to, co potřebuje každá stránka
// (session, detail účtu, odznaky v navigaci); testy přidávají vlastní přes `server.use(...)`.
import { http, HttpResponse, type RequestHandler } from 'msw'
import { setupServer } from 'msw/node'
import type { Account, AccountMembership, Me } from '@/api/types'
import { emptyList, makeAccount, makeMe } from './fixtures'

/** Origin jsdom (vitest environmentOptions) = baseUrl API klienta v testech. */
export const API = 'http://localhost:3000'

/** Stav, ze kterého čtou výchozí handlery — testy ho mění přes `setSession`. */
export const state: { me: Me | null; account: Account } = { me: makeMe(), account: makeAccount() }

/** Přihlášený uživatel s rolí v účtu `firma` (null = nepřihlášen). */
export function setSession(role: AccountMembership['role'] | null, account: Partial<Account> = {}, me: Partial<Me> = {}) {
  state.me = role ? makeMe(me, role) : null
  state.account = makeAccount({ role: role ?? 'owner', ...account })
}

export function resetState() {
  state.me = makeMe()
  state.account = makeAccount()
}

/** problem+json odpověď jako z huma (errors[].location = "body.<pole>"). */
export function problem(
  status: number,
  detail: string,
  extra: { errors?: { location: string; message: string; value?: unknown }[]; code?: string } = {},
) {
  return HttpResponse.json(
    { title: detail, status, detail, ...extra },
    { status, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

export const defaultHandlers: RequestHandler[] = [
  http.get(`${API}/api/auth/me`, () => (state.me ? HttpResponse.json(state.me) : problem(401, 'unauthorized'))),
  http.get(`${API}/api/auth/status`, () => HttpResponse.json({ has_users: true, signup_allowed: true })),
  http.get(`${API}/api/accounts/:slug`, () => HttpResponse.json(state.account)),
  http.get(`${API}/api/accounts/:slug/todos`, () => HttpResponse.json(emptyList())),
  // Vše ostatní, co test nenamockoval (vedlejší widgety stránek), odpoví 404 → komponenty ukážou prázdný stav
  // a test neběží proti síti. Handlery z `server.use(...)` mají vždy přednost.
  http.all(`${API}/api/*`, () => problem(404, 'not mocked in test')),
]

export const server = setupServer(...defaultHandlers)
