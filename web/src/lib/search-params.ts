import { defaultParseSearch, defaultStringifySearch } from '@tanstack/react-router'

/**
 * Parametry OAuth žádosti (/oauth/authorize) jsou neprůhledné řetězce klienta. Výchozí serializace routeru
 * je převádí přes JSON (`state=123` → číslo, `1e3` → 1000, při zápisu zpět `"123"` v uvozovkách),
 * což by rozbilo kontrolu `state` / PKCE na straně aplikace — tyhle klíče proto čteme a píšeme beze změny.
 */
const RAW_KEYS = new Set([
  'client_id',
  'redirect_uri',
  'response_type',
  'state',
  'code_challenge',
  'code_challenge_method',
  'scope',
  'resource',
])

export function parseSearch(searchStr: string): Record<string, unknown> {
  const search: Record<string, unknown> = defaultParseSearch(searchStr)
  const raw = new URLSearchParams(searchStr)
  for (const key of RAW_KEYS) {
    const value = raw.get(key)
    if (value !== null) search[key] = value
  }
  return search
}

export function stringifySearch(search: Record<string, unknown>): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(search)) {
    if (value === undefined) continue
    if (RAW_KEYS.has(key) && typeof value === 'string') {
      params.set(key, value)
      continue
    }
    const encoded = new URLSearchParams(defaultStringifySearch({ [key]: value })).get(key)
    if (encoded !== null) params.set(key, encoded)
  }
  const str = params.toString()
  return str ? `?${str}` : ''
}
