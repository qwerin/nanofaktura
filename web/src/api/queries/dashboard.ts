import { queryOptions } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { keys } from './keys'

export const dashboardQueries = {
  /** Statistiky účtu za rok (bez `year` = aktuální rok podle serveru). */
  get: (slug: string, year?: number) =>
    queryOptions({
      queryKey: keys.dashboard(slug, year),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/dashboard', {
            params: { path: { slug }, query: year ? { year } : {} },
            signal,
          }),
        ),
    }),
}
