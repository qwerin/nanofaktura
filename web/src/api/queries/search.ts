import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { keys } from './keys'

export const searchQueries = {
  /** Globální hledání (command palette). Prázdný dotaz se neposílá. */
  global: (slug: string, q: string, limit = 5) =>
    queryOptions({
      queryKey: [...keys.search(slug, q), limit],
      queryFn: ({ signal }) =>
        unwrap(api.GET('/api/accounts/{slug}/search', { params: { path: { slug }, query: { q, limit } }, signal })),
      enabled: q.trim().length > 0,
      placeholderData: keepPreviousData,
      staleTime: 10_000,
    }),
}
