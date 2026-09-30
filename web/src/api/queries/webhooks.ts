import { infiniteQueryOptions, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateWebhookInput, UpdateWebhookInput, Webhook } from '../types'
import { keys } from './keys'

export const webhookQueries = {
  /** Webhooky účtu (jen owner/admin). */
  list: (slug: string) =>
    queryOptions({
      queryKey: [...keys.webhooks(slug), 'list'],
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/webhooks', { params: { path: { slug }, query: { per_page: 100 } }, signal }),
          )
        ).items ?? [],
    }),

  /** Doručení jednoho webhooku, nejnovější první. */
  deliveries: (slug: string, id: number) =>
    infiniteQueryOptions({
      queryKey: keys.webhookDeliveries(slug, id),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/webhooks/{id}/deliveries', {
            params: { path: { slug, id }, query: { page: pageParam, per_page: 20 } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      // Čekající pokusy se mění na pozadí.
      refetchInterval: 30_000,
    }),
}

export function useCreateWebhook(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateWebhookInput) =>
      unwrap(api.POST('/api/accounts/{slug}/webhooks', { params: { path: { slug } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.webhooks(slug) }),
    meta: { silent: true },
  })
}

export function useUpdateWebhook(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateWebhookInput }) =>
      unwrap(api.PATCH('/api/accounts/{slug}/webhooks/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.webhooks(slug) }),
    meta: { silent: true },
  })
}

export function useDeleteWebhook(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (webhook: Pick<Webhook, 'id'>) =>
      unwrap(api.DELETE('/api/accounts/{slug}/webhooks/{id}', { params: { path: { slug, id: webhook.id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.webhooks(slug) }),
  })
}

/** Synchronní „ping“ — výsledek (status, odpověď) zobrazí volající. */
export function useTestWebhook(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (webhook: Pick<Webhook, 'id'>) =>
      unwrap(api.POST('/api/accounts/{slug}/webhooks/{id}/test', { params: { path: { slug, id: webhook.id } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.webhooks(slug) }),
  })
}

export function useRedeliverWebhook(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ webhookId, deliveryId }: { webhookId: number; deliveryId: number }) =>
      unwrap(
        api.POST('/api/accounts/{slug}/webhooks/{id}/deliveries/{delivery_id}/redeliver', {
          params: { path: { slug, id: webhookId, delivery_id: deliveryId } },
        }),
      ),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.webhooks(slug) }),
  })
}
