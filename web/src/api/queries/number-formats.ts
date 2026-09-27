import { keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateNumberFormatInput, NumberFormat, UpdateNumberFormatInput } from '../types'
import { keys } from './keys'

export const numberFormatQueries = {
  list: (slug: string) =>
    queryOptions({
      queryKey: keys.numberFormats(slug),
      queryFn: async () =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/number-formats', { params: { path: { slug }, query: { per_page: 200 } } }),
          )
        ).items ?? [],
    }),

  /**
   * Příští číslo řady (bez posunu čítače). `format` je v klíči, aby se náhled
   * po úpravě formátu načetl znovu.
   */
  preview: (slug: string, format: Pick<NumberFormat, 'id' | 'format'>) =>
    queryOptions({
      queryKey: keys.numberFormatPreview(slug, format.id, format.format),
      queryFn: async () =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/number-formats/{id}/preview', {
              params: { path: { slug, id: format.id } },
            }),
          )
        ).number,
      placeholderData: keepPreviousData,
    }),
}

export function useCreateNumberFormat(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationKey: [...keys.numberFormats(slug), 'save'],
    mutationFn: (body: CreateNumberFormatInput) =>
      unwrap(api.POST('/api/accounts/{slug}/number-formats', { params: { path: { slug } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.numberFormats(slug) }),
    meta: { silent: true },
  })
}

export function useUpdateNumberFormat(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateNumberFormatInput }) =>
      unwrap(api.PATCH('/api/accounts/{slug}/number-formats/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.numberFormats(slug) }),
    meta: { silent: true },
  })
}

export function useDeleteNumberFormat(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (format: Pick<NumberFormat, 'id'>) =>
      unwrap(api.DELETE('/api/accounts/{slug}/number-formats/{id}', { params: { path: { slug, id: format.id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.numberFormats(slug) }),
    meta: { silent: true },
  })
}
