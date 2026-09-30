import { infiniteQueryOptions, keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { EventFilters } from '../types'
import { keys } from './keys'

/** Prefix cache zdroje, pod kterým leží timeline záznamu (zneplatní ji mutace dokladu). */
const timelineResource: Record<string, string> = {
  invoice: 'invoices',
  expense: 'expenses',
  subject: 'subjects',
  price_item: 'price-items',
  recurring: 'recurring',
}

export const eventQueries = {
  /** Události účtu (nejnovější první, „Načíst starší“). */
  list: (slug: string, filters: EventFilters, perPage = 20) => {
    const record = filters.subject_type && filters.subject_id ? filters.subject_type : undefined
    const queryKey =
      record && timelineResource[record]
        ? [...keys.recordTimeline(slug, timelineResource[record], filters.subject_id ?? 0), filters, perPage]
        : keys.eventList(slug, { ...filters, perPage })
    return infiniteQueryOptions({
      queryKey,
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/events', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: perPage } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      placeholderData: keepPreviousData,
    })
  },

  /** Katalog názvů událostí (pro výběr u webhooků). Mění se jen s verzí aplikace. */
  catalog: (slug: string) =>
    queryOptions({
      queryKey: keys.eventCatalog(slug),
      queryFn: async ({ signal }) =>
        (await unwrap(api.GET('/api/accounts/{slug}/events/catalog', { params: { path: { slug } }, signal }))) ?? [],
      staleTime: Infinity,
    }),
}
