// Typovaný HTTP klient nad openapi-fetch. Veškerá komunikace s backendem jde přes `api`.
//
//   const account = await unwrap(api.GET('/api/accounts/{slug}', { params: { path: { slug } } }))
//
// - Session je v HttpOnly cookie → `credentials: 'include'`.
// - Chyby: `unwrap` vyhodí `ApiError` (problem+json). Toasty řeší globálně QueryClient
//   (src/api/query-client.ts), ne jednotlivé komponenty.
// - 401 mimo „tiché“ endpointy → zavolá se handler registrovaný v main.tsx (redirect na /login).

import createClient, { type Middleware } from 'openapi-fetch'
import { ApiError, type Problem } from './errors'
import type { paths } from './schema.gen'

export const api = createClient<paths>({
  baseUrl: import.meta.env.VITE_API_BASE_URL ?? '',
  credentials: 'include',
  headers: { Accept: 'application/json, application/problem+json' },
})

/** Endpointy, kde je 401 očekávaný stav a nemá vést k přesměrování. */
const SILENT_401 = ['/api/auth/me', '/api/auth/login', '/api/auth/status', '/api/auth/register']

let onUnauthorized: (() => void) | null = null

/** Registruje reakci na vypršelou session (volá se z main.tsx, aby client nezávisel na routeru). */
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

const authMiddleware: Middleware = {
  onResponse({ response, schemaPath }) {
    if (response.status === 401 && !SILENT_401.includes(schemaPath)) {
      onUnauthorized?.()
    }
    return undefined
  },
}

api.use(authMiddleware)

type FetchResult<T> = { data?: T; error?: unknown; response: Response }

/**
 * Rozbalí výsledek openapi-fetch: vrátí `data`, nebo vyhodí `ApiError`.
 * Síťové chyby (backend neběží) se převedou na `ApiError` se statusem 0.
 */
export async function unwrap<T>(request: Promise<FetchResult<T>>): Promise<T> {
  let result: FetchResult<T>
  try {
    result = await request
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, { title: 'Network error' })
  }
  const { data, error, response } = result
  if (!response.ok) {
    const problem: Problem =
      error && typeof error === 'object' ? (error as Problem) : { title: response.statusText }
    throw new ApiError(response.status, problem)
  }
  return data as T
}
