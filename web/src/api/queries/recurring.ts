import { queryOptions, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateRecurringInput, Recurring, UpdateRecurringInput } from '../types'
import { keys } from './keys'

export const RECURRING_PER_PAGE = 100

export interface RecurringFilters {
  active?: 'true' | 'false'
  template_id?: number
}

export const recurringQueries = {
  list: (slug: string, filters: RecurringFilters = {}) =>
    queryOptions({
      queryKey: keys.recurringList(slug, filters),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/recurring', {
            params: { path: { slug }, query: { ...filters, per_page: RECURRING_PER_PAGE } },
            signal,
          }),
        ),
    }),

  detail: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.recurringDetail(slug, id),
      queryFn: ({ signal }) =>
        unwrap(api.GET('/api/accounts/{slug}/recurring/{id}', { params: { path: { slug, id } }, signal })),
    }),
}

function afterChange(qc: QueryClient, slug: string, r?: Recurring) {
  if (r) qc.setQueryData(keys.recurringDetail(slug, r.id), r)
  void qc.invalidateQueries({ queryKey: [...keys.recurring(slug), 'list'] })
}

export function useCreateRecurring(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateRecurringInput) =>
      unwrap(api.POST('/api/accounts/{slug}/recurring', { params: { path: { slug } }, body })),
    onSuccess: (r) => afterChange(qc, slug, r),
    meta: { silent: true },
  })
}

export function useUpdateRecurring(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateRecurringInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/recurring/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: (r) => afterChange(qc, slug, r),
    meta: { silent: true },
  })
}

export function useDeleteRecurring(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/accounts/{slug}/recurring/{id}', { params: { path: { slug, id } } })),
    onSuccess: (_d, id) => {
      qc.removeQueries({ queryKey: keys.recurringDetail(slug, id) })
      afterChange(qc, slug)
    },
  })
}

/** Zapnout / vypnout (aktivace přeskočí zameškané výskyty). */
export function useSetRecurringActive(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) =>
      active
        ? unwrap(api.POST('/api/accounts/{slug}/recurring/{id}/activate', { params: { path: { slug, id } } }))
        : unwrap(api.POST('/api/accounts/{slug}/recurring/{id}/deactivate', { params: { path: { slug, id } } })),
    onSuccess: (r) => afterChange(qc, slug, r),
  })
}

/** Vystavit fakturu hned (datum dnes), posune příští výskyt. */
export function useRunRecurringNow(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      unwrap(api.POST('/api/accounts/{slug}/recurring/{id}/run-now', { params: { path: { slug, id } } })),
    onSuccess: (inv) => {
      qc.setQueryData(keys.invoiceDetail(slug, inv.id), inv)
      void qc.invalidateQueries({ queryKey: keys.recurring(slug) })
      void qc.invalidateQueries({ queryKey: [...keys.invoices(slug), 'list'] })
      void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'dashboard'] })
    },
  })
}
