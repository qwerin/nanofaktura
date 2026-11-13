import { infiniteQueryOptions, keepPreviousData, queryOptions, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { EETCertificateInput, EETSale, EETSaleStatus, EETSettings, UpdateEETSettingsInput } from '../types'
import { keys } from './keys'

export const EET_SALES_PER_PAGE = 25

export const eetQueries = {
  /** Nastavení a stav evidence tržeb (čte každý člen účtu). */
  settings: (slug: string) =>
    queryOptions({
      queryKey: keys.eetSettings(slug),
      queryFn: ({ signal }) => unwrap(api.GET('/api/accounts/{slug}/eet', { params: { path: { slug } }, signal })),
      // Dotaz běží i na pozadí dialogů (platba) — chybu řeší volající, žádný globální toast.
      meta: { silent: true },
    }),

  /** Tržby, nejnovější první. */
  sales: (slug: string, filters: { status?: EETSaleStatus }) =>
    infiniteQueryOptions({
      queryKey: keys.eetSales(slug, filters),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/eet-sales', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: EET_SALES_PER_PAGE } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      placeholderData: keepPreviousData,
    }),
}

/** Uložené nastavení rovnou do cache; seznam tržeb se načte znovu. */
function afterSettings(qc: QueryClient, slug: string, settings?: EETSettings) {
  if (settings) qc.setQueryData(keys.eetSettings(slug), settings)
  else void qc.invalidateQueries({ queryKey: keys.eetSettings(slug) })
}

export function useUpdateEETSettings(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateEETSettingsInput) => unwrap(api.PATCH('/api/accounts/{slug}/eet', { params: { path: { slug } }, body })),
    onSuccess: (settings) => afterSettings(qc, slug, settings),
    meta: { silent: true },
  })
}

export function useUploadEETCertificate(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: EETCertificateInput) =>
      unwrap(api.PUT('/api/accounts/{slug}/eet/certificate', { params: { path: { slug } }, body })),
    onSuccess: (settings) => afterSettings(qc, slug, settings),
    meta: { silent: true },
  })
}

/** Odebrání certifikátu evidenci zároveň vypne. */
export function useDeleteEETCertificate(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE('/api/accounts/{slug}/eet/certificate', { params: { path: { slug } } })),
    onSuccess: () => afterSettings(qc, slug),
  })
}

/** Ověřovací zpráva Finanční správě — nic se neeviduje, výsledek zobrazí volající. */
export function useTestEET(slug: string) {
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/accounts/{slug}/eet/actions/test', { params: { path: { slug } } })),
    meta: { silent: true },
  })
}

/** Nový pokus o odeslání čekající/odmítnuté tržby. Obnoví počty, seznam tržeb i detail faktury. */
export function useRetryEETSale(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (sale: Pick<EETSale, 'id' | 'invoice_id'>) =>
      unwrap(api.POST('/api/accounts/{slug}/eet-sales/{id}/actions/retry', { params: { path: { slug, id: sale.id } } })),
    onSettled: (_data, _err, sale) => {
      void qc.invalidateQueries({ queryKey: keys.eet(slug) })
      void qc.invalidateQueries({ queryKey: keys.invoiceDetail(slug, sale.invoice_id) })
    },
    meta: { silent: true },
  })
}
