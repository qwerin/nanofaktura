import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { isApiError } from '../errors'
import { keys } from './keys'

export const aresQueries = {
  /** Firma z ARES podle IČO. 404 = nenalezeno, 422 = neplatné IČO, 502 = ARES nedostupný. */
  lookup: (ico: string) =>
    queryOptions({
      queryKey: keys.ares(ico),
      queryFn: () => unwrap(api.GET('/api/ares/{ico}', { params: { path: { ico } } })),
      staleTime: 60 * 60_000,
      retry: false,
    }),
}

/**
 * Načtení z ARES na vyžádání (tlačítko / automaticky po zadání IČO).
 * Výsledek se cachuje, opakované dotazy na stejné IČO nejdou na server.
 */
export function useAresLookup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ico: string) => qc.fetchQuery(aresQueries.lookup(ico)),
    meta: { silent: true },
  })
}

/** Česká hláška pro chybu ARES. */
export function aresErrorMessage(err: unknown): string {
  if (isApiError(err)) {
    if (err.status === 404) return 'Subjekt s tímto IČO v ARES není.'
    if (err.status === 422) return 'Neplatné IČO.'
    if (err.status === 502 || err.status === 504) return 'ARES je momentálně nedostupný, vyplňte údaje ručně.'
    if (err.isNetworkError) return 'Server je nedostupný. Zkontrolujte připojení.'
  }
  return 'Načtení z ARES se nezdařilo.'
}
